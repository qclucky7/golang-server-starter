package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"golang-server-starter/internal/apperr"
	"golang-server-starter/internal/dto"
	"golang-server-starter/internal/model"
	"golang-server-starter/internal/pkg/query"
	"golang-server-starter/internal/pkg/snowflake"
	"golang-server-starter/internal/pkg/token"
	"golang-server-starter/internal/repository"
	"golang-server-starter/internal/service"

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

// TestListOrgsByOwnerPagination 分页查询只返回当前账号自己的组织。
//
// 这是模板里唯一的分页接口，覆盖三件事：总条数正确、分页切片正确、
// 不串到别人的组织（owner_id 过滤必须生效）。
func TestListOrgsByOwnerPagination(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	demo := registerDemo(t, env)
	orgs := service.NewOrgService(env.orgRepo)

	// 注册时已自动创建 1 个默认组织，这里再补 2 个
	for _, name := range []string{"研发部", "市场部"} {
		org := &model.Org{Name: name, OwnerID: demo.ID}
		if err := env.orgRepo.Create(ctx, org); err != nil {
			t.Fatalf("创建组织失败: %v", err)
		}
	}
	// 另一个账号也会有自己的组织，它不应出现在 demo 的结果里
	if _, err := env.auth.Register(ctx, &dto.RegisterRequest{
		Username: "other", Email: "other@example.com", Password: "demo1234",
	}); err != nil {
		t.Fatalf("注册第二个账号失败: %v", err)
	}

	// 第一页：total 应为 3（若 owner_id 过滤失效则是 4）
	page := query.Page{Current: 1, Size: 2}
	items, total, err := orgs.ListByOwner(ctx, demo.ID, &page)
	if err != nil {
		t.Fatalf("查询组织列表失败: %v", err)
	}
	if total != 3 {
		t.Fatalf("demo 的组织总数应为 3，实际 %d（owner_id 过滤可能失效）", total)
	}
	if len(items) != 2 {
		t.Fatalf("size=2 应返回 2 条，实际 %d", len(items))
	}
	for _, item := range items {
		if item.OwnerID != demo.ID {
			t.Fatalf("不应返回他人组织: owner=%d", item.OwnerID)
		}
	}

	// 第二页只剩 1 条
	page2 := query.Page{Current: 2, Size: 2}
	items2, total2, err := orgs.ListByOwner(ctx, demo.ID, &page2)
	if err != nil {
		t.Fatalf("查询第二页失败: %v", err)
	}
	if total2 != 3 || len(items2) != 1 {
		t.Fatalf("第二页应为 1 条 / 共 3 条，实际 %d / %d", len(items2), total2)
	}

	// 越界参数应被归一化，并回写到入参，供调用方构造分页信息
	page3 := query.Page{Current: 0, Size: 99999}
	if _, _, err := orgs.ListByOwner(ctx, demo.ID, &page3); err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if page3.Current != query.DefaultCurrent || page3.Size != query.MaxSize {
		t.Fatalf("分页参数应被归一化，实际 %+v", page3)
	}
}
