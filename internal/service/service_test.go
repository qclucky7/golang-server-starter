package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gin-quick-start/internal/apperr"
	"gin-quick-start/internal/dto"
	"gin-quick-start/internal/model"
	"gin-quick-start/internal/pkg/snowflake"
	"gin-quick-start/internal/pkg/token"
	"gin-quick-start/internal/repository"
	"gin-quick-start/internal/service"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type testEnv struct {
	auth        *service.AuthService
	accounts    *service.AccountService
	accountRepo *repository.AccountRepository
	orgRepo     *repository.OrgRepository
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	// 主键由雪花算法在应用层生成，任何写入前都必须先初始化生成器
	if err := snowflake.Setup(snowflake.AutoNodeID); err != nil {
		t.Fatalf("初始化雪花 ID 生成器失败: %v", err)
	}
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	accounts := repository.NewAccountRepository(db)
	orgs := repository.NewOrgRepository(db)
	tokens := token.NewManager("test-secret-key-at-least-16-chars", "test", time.Hour, 24*time.Hour)
	return &testEnv{
		auth:        service.NewAuthService(accounts, orgs, tokens),
		accounts:    service.NewAccountService(accounts, orgs),
		accountRepo: accounts,
		orgRepo:     orgs,
	}
}

func registerDemo(t *testing.T, env *testEnv) *model.Account {
	t.Helper()
	account, err := env.auth.Register(context.Background(), &dto.RegisterRequest{
		Username: "demo",
		Email:    "Demo@Example.com",
		Password: "demo1234",
	})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	return account
}

func TestRegisterNormalizesInput(t *testing.T) {
	env := newTestEnv(t)
	account := registerDemo(t, env)

	if account.Email != "demo@example.com" {
		t.Fatalf("邮箱应被归一化为小写: %s", account.Email)
	}
	if account.Nickname != "demo" {
		t.Fatalf("昵称缺省时应回落为用户名: %s", account.Nickname)
	}
	if account.Status != model.AccountStatusEnabled {
		t.Fatalf("默认状态应为启用: %d", account.Status)
	}
	if account.Password == "demo1234" {
		t.Fatal("密码不应以明文存储")
	}
}

// TestRegisterCreatesDefaultOrg 注册必须同时落库账号与它的默认组织。
func TestRegisterCreatesDefaultOrg(t *testing.T) {
	env := newTestEnv(t)
	account := registerDemo(t, env)

	org, err := env.orgRepo.FindDefaultByOwner(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("查询默认组织失败: %v", err)
	}
	if org == nil {
		t.Fatal("注册后应存在默认组织")
	}
	if org.OwnerID != account.ID {
		t.Fatalf("默认组织的 owner_id 应等于账号 ID: org.owner_id=%d account.id=%d", org.OwnerID, account.ID)
	}
	if !org.IsDefault {
		t.Fatal("注册创建的默认组织 is_default 应为 true")
	}
	if org.Name != "demo的组织" {
		t.Fatalf("默认组织名不符: %s", org.Name)
	}
}

// TestRegisterDoesNotLeakOrgOnConflict 注册失败时不应留下孤儿组织。
func TestRegisterDoesNotLeakOrgOnConflict(t *testing.T) {
	env := newTestEnv(t)
	registerDemo(t, env)
	ctx := context.Background()

	if _, err := env.auth.Register(ctx, &dto.RegisterRequest{
		Username: "other",
		Email:    "demo@example.com", // 邮箱冲突
		Password: "demo1234",
	}); !errors.Is(err, apperr.ErrEmailExists) {
		t.Fatalf("重复邮箱应返回 ErrEmailExists: %v", err)
	}

	// 冲突账号并未落库，因此不应有 owner 指向不存在的账号
	items, err := env.orgRepo.List(ctx)
	if err != nil {
		t.Fatalf("查询组织列表失败: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("失败注册不应创建组织，实际组织数=%d", len(items))
	}
}

func TestRegisterRejectsDuplicate(t *testing.T) {
	env := newTestEnv(t)
	registerDemo(t, env)

	_, err := env.auth.Register(context.Background(), &dto.RegisterRequest{
		Username: "demo",
		Email:    "other@example.com",
		Password: "demo1234",
	})
	if !errors.Is(err, apperr.ErrUsernameExists) {
		t.Fatalf("重复用户名应返回 ErrUsernameExists: %v", err)
	}

	_, err = env.auth.Register(context.Background(), &dto.RegisterRequest{
		Username: "other",
		Email:    "demo@example.com",
		Password: "demo1234",
	})
	if !errors.Is(err, apperr.ErrEmailExists) {
		t.Fatalf("重复邮箱应返回 ErrEmailExists: %v", err)
	}
}

func TestLoginAndAuthenticate(t *testing.T) {
	env := newTestEnv(t)
	registerDemo(t, env)
	ctx := context.Background()

	// 用户名登录
	result, err := env.auth.Login(ctx, &dto.LoginRequest{Account: "demo", Password: "demo1234"}, dto.ClientMeta{IP: "127.0.0.1"})
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if result.Token.AccessToken == "" {
		t.Fatal("未返回访问令牌")
	}

	account, claims, err := env.auth.Authenticate(ctx, result.Token.AccessToken)
	if err != nil {
		t.Fatalf("令牌校验失败: %v", err)
	}
	if account.Username != "demo" || claims.AccountID != account.ID.Int64() {
		t.Fatalf("鉴权结果不符: account=%s claims=%+v", account.Username, claims)
	}

	// 邮箱登录
	if _, err := env.auth.Login(ctx, &dto.LoginRequest{Account: "demo@example.com", Password: "demo1234"}, dto.ClientMeta{}); err != nil {
		t.Fatalf("邮箱登录失败: %v", err)
	}

	// 密码错误
	if _, err := env.auth.Login(ctx, &dto.LoginRequest{Account: "demo", Password: "wrong1234"}, dto.ClientMeta{}); !errors.Is(err, apperr.ErrAuthPasswordWrong) {
		t.Fatalf("密码错误应返回 ErrAuthPasswordWrong: %v", err)
	}

	// 账号不存在与密码错误返回同一错误，避免账号枚举
	if _, err := env.auth.Login(ctx, &dto.LoginRequest{Account: "ghost", Password: "demo1234"}, dto.ClientMeta{}); !errors.Is(err, apperr.ErrAuthPasswordWrong) {
		t.Fatalf("账号不存在应返回 ErrAuthPasswordWrong: %v", err)
	}
}

func TestLogoutRevokesToken(t *testing.T) {
	env := newTestEnv(t)
	registerDemo(t, env)
	ctx := context.Background()

	result, err := env.auth.Login(ctx, &dto.LoginRequest{Account: "demo", Password: "demo1234"}, dto.ClientMeta{})
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if err := env.auth.Logout(ctx, result.Account.ID); err != nil {
		t.Fatalf("登出失败: %v", err)
	}

	_, _, err = env.auth.Authenticate(ctx, result.Token.AccessToken)
	if !errors.Is(err, apperr.ErrAuthTokenRevoked) {
		t.Fatalf("登出后令牌应失效: %v", err)
	}
	// 刷新令牌同样失效
	if _, err := env.auth.Refresh(ctx, result.Token.RefreshToken); !errors.Is(err, apperr.ErrAuthTokenRevoked) {
		t.Fatalf("登出后刷新令牌应失效: %v", err)
	}
}

func TestDisabledAccountCannotLogin(t *testing.T) {
	env := newTestEnv(t)
	account := registerDemo(t, env)
	ctx := context.Background()

	// 管理端已移除，这里直接改库来构造「被禁用」的状态
	disabled := int8(model.AccountStatusDisabled)
	if err := env.accountRepo.UpdateFields(ctx, account.ID, map[string]any{"status": disabled}); err != nil {
		t.Fatalf("禁用账号失败: %v", err)
	}

	if _, err := env.auth.Login(ctx, &dto.LoginRequest{Account: "demo", Password: "demo1234"}, dto.ClientMeta{}); !errors.Is(err, apperr.ErrAuthAccountDisabled) {
		t.Fatalf("禁用账号登录应返回 ErrAuthAccountDisabled: %v", err)
	}
}

func TestChangePasswordRevokesToken(t *testing.T) {
	env := newTestEnv(t)
	account := registerDemo(t, env)
	ctx := context.Background()

	result, err := env.auth.Login(ctx, &dto.LoginRequest{Account: "demo", Password: "demo1234"}, dto.ClientMeta{})
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}

	if err := env.accounts.ChangePassword(ctx, account.ID, "wrong1234", "demo5678"); !errors.Is(err, apperr.ErrAuthOldPasswordWrong) {
		t.Fatalf("原密码错误应返回 ErrAuthOldPasswordWrong: %v", err)
	}
	if err := env.accounts.ChangePassword(ctx, account.ID, "demo1234", "demo5678"); err != nil {
		t.Fatalf("改密失败: %v", err)
	}

	if _, _, err := env.auth.Authenticate(ctx, result.Token.AccessToken); !errors.Is(err, apperr.ErrAuthTokenRevoked) {
		t.Fatalf("改密后旧令牌应失效: %v", err)
	}
	if _, err := env.auth.Login(ctx, &dto.LoginRequest{Account: "demo", Password: "demo5678"}, dto.ClientMeta{}); err != nil {
		t.Fatalf("新密码登录失败: %v", err)
	}
}

// TestDefaultOrgLookup 注册后能查到默认组织，且属于自己。
//
// 管理端账号 CRUD 已移除，DefaultOrg 现在是「账号自服务」的一部分：
// 只按传入的账号 ID 查它自己的默认组织，不提供跨账号查询。
func TestDefaultOrgLookup(t *testing.T) {
	env := newTestEnv(t)
	account := registerDemo(t, env)
	ctx := context.Background()

	org, err := env.accounts.DefaultOrg(ctx, account.ID)
	if err != nil {
		t.Fatalf("查询默认组织失败: %v", err)
	}
	if org == nil {
		t.Fatal("注册后应存在默认组织")
	}
	if org.OwnerID != account.ID {
		t.Fatalf("默认组织的 owner 应为本人: %d != %d", org.OwnerID, account.ID)
	}
	if !org.IsDefault {
		t.Fatal("默认组织的 is_default 应为 true")
	}

	// 不存在的账号不应凭空返回组织
	ghost, err := env.accounts.DefaultOrg(ctx, model.ID(999999999999999999))
	if err != nil {
		t.Fatalf("查询不存在的账号不应报错: %v", err)
	}
	if ghost != nil {
		t.Fatalf("不存在的账号不应有默认组织: %+v", ghost)
	}
}
