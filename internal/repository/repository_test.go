package repository_test

import (
	"context"
	"fmt"
	"testing"

	"golang-server-starter/internal/model"
	"golang-server-starter/internal/pkg/query"
	"golang-server-starter/internal/pkg/snowflake"
	"golang-server-starter/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newTestDB 为每个测试创建独立的内存库
func newTestDB(t *testing.T) *gorm.DB {
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
	return db
}

func newAccount(t *testing.T, db *gorm.DB, name string) *model.Account {
	t.Helper()
	repo := repository.NewAccountRepository(db)
	account := &model.Account{
		Username: name,
		Email:    name + "@example.com",
		Password: "hashed",
	}
	if err := repo.Create(context.Background(), account); err != nil {
		t.Fatalf("创建账号失败: %v", err)
	}
	return account
}

func TestRepositoryCRUD(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	account := &model.Account{Username: "u1", Email: "u1@example.com", Password: "x"}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if account.ID == 0 {
		t.Fatal("Create 未回填主键")
	}

	got, err := repo.GetByID(ctx, account.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID 失败: entity=%v err=%v", got, err)
	}
	if got.Username != "u1" {
		t.Fatalf("GetByID 结果不符: %s", got.Username)
	}

	if err := repo.UpdateFields(ctx, account.ID, map[string]any{"nickname": "nick"}); err != nil {
		t.Fatalf("UpdateFields 失败: %v", err)
	}
	got, _ = repo.GetByID(ctx, account.ID)
	if got.Nickname != "nick" {
		t.Fatalf("UpdateFields 未生效: %s", got.Nickname)
	}

	exists, err := repo.Exists(ctx, repository.WhereEq("username", "u1"))
	if err != nil || !exists {
		t.Fatalf("Exists 结果不符: %v err=%v", exists, err)
	}

	if err := repo.Delete(ctx, account.ID); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}
	got, err = repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("删除后查询报错: %v", err)
	}
	if got != nil {
		t.Fatal("软删除后仍能查到记录")
	}
}

func TestRepositoryGetNotFoundReturnsNil(t *testing.T) {
	db := newTestDB(t)
	repo := repository.New[model.Account](db)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf("未找到不应返回错误: %v", err)
	}
	if got != nil {
		t.Fatal("未找到应返回 nil")
	}
}

func TestRepositoryPage(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	for i := 0; i < 25; i++ {
		newAccount(t, db, fmt.Sprintf("account%02d", i))
	}

	page := &query.Page{Current: 2, Size: 10, OrderBy: "id", Order: "asc"}
	items, total, err := repo.Page(ctx, page)
	if err != nil {
		t.Fatalf("Page 失败: %v", err)
	}
	if total != 25 {
		t.Fatalf("总数不符: %d", total)
	}
	if len(items) != 10 {
		t.Fatalf("第二页条数不符: %d", len(items))
	}
	if items[0].Username != "account10" {
		t.Fatalf("排序或偏移不符: %s", items[0].Username)
	}

	// 越界页应返回空列表而非报错
	items, total, err = repo.Page(ctx, &query.Page{Current: 99, Size: 10})
	if err != nil {
		t.Fatalf("越界分页报错: %v", err)
	}
	if len(items) != 0 || total != 25 {
		t.Fatalf("越界分页结果不符: len=%d total=%d", len(items), total)
	}
}

func TestAccountRepositoryDomainQueries(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.NewAccountRepository(db)

	account := newAccount(t, db, "alice")

	byName, err := repo.FindByUsernameOrEmail(ctx, "alice")
	if err != nil || byName == nil || byName.ID != account.ID {
		t.Fatalf("按用户名查询失败: %v", err)
	}
	byEmail, err := repo.FindByUsernameOrEmail(ctx, "alice@example.com")
	if err != nil || byEmail == nil || byEmail.ID != account.ID {
		t.Fatalf("按邮箱查询失败: %v", err)
	}

	occupied, err := repo.ExistsUsername(ctx, "alice")
	if err != nil || !occupied {
		t.Fatalf("ExistsUsername 应为 true: %v", err)
	}
	occupied, err = repo.ExistsUsername(ctx, "alice", account.ID)
	if err != nil || occupied {
		t.Fatalf("排除自身后 ExistsUsername 应为 false: %v", err)
	}

	if err := repo.BumpTokenVersion(ctx, account.ID); err != nil {
		t.Fatalf("BumpTokenVersion 失败: %v", err)
	}
	after, _ := repo.GetByID(ctx, account.ID)
	if after.TokenVersion != account.TokenVersion+1 {
		t.Fatalf("令牌版本未递增: %d", after.TokenVersion)
	}
}
