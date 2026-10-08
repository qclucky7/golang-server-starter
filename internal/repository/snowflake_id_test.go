package repository_test

import (
	"context"
	"testing"

	"golang-server-starter/internal/model"
	"golang-server-starter/internal/repository"
)

// TestSnowflakePrimaryKey 主键必须是应用层生成的雪花 ID，而不是数据库自增。
//
// 回归点：GORM 对 int64 类型的主键默认按自增处理（省略该列、用 LastInsertId 回读），
// 少了 `autoIncrement:false` 这个 tag，BeforeCreate 里赋的值会被丢掉。
func TestSnowflakePrimaryKey(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	first := newAccount(t, db, "sf1")
	second := newAccount(t, db, "sf2")

	if first.ID.IsZero() || second.ID.IsZero() {
		t.Fatalf("主键未被分配: first=%d second=%d", first.ID, second.ID)
	}
	if first.ID == second.ID {
		t.Fatalf("主键重复: %d", first.ID)
	}
	// 自增会给出 1、2；雪花 ID 是 19 位十进制数
	if len(first.ID.String()) < 18 {
		t.Fatalf("主键看起来像自增而非雪花: %d", first.ID)
	}
	if second.ID <= first.ID {
		t.Fatalf("雪花 ID 应趋势递增: %d -> %d", first.ID, second.ID)
	}

	// 主键真的落库了（而不是被数据库改写成自增值）
	got, err := repo.GetByID(ctx, first.ID)
	if err != nil || got == nil {
		t.Fatalf("按雪花主键查不到记录: err=%v", err)
	}
	if got.ID != first.ID {
		t.Fatalf("落库主键与分配值不一致: %d != %d", got.ID, first.ID)
	}
}

// TestBaseRespectsExplicitID 调用方显式指定的主键不能被覆盖（数据导入 / 测试固定 ID）。
func TestBaseRespectsExplicitID(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	const fixed model.ID = 9999
	account := &model.Account{Username: "fixed", Email: "fixed@example.com", Password: "x"}
	account.ID = fixed
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if account.ID != fixed {
		t.Fatalf("显式主键被覆盖: 期望 %d，实际 %d", fixed, account.ID)
	}

	got, err := repo.GetByID(ctx, fixed)
	if err != nil || got == nil {
		t.Fatalf("按固定主键查不到: err=%v", err)
	}
}

// TestSnowflakePrimaryKeyInWhereClause 主键参与 WHERE / IN 查询时类型要能被正确编码。
func TestSnowflakePrimaryKeyInWhereClause(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	first := newAccount(t, db, "w1")
	second := newAccount(t, db, "w2")
	newAccount(t, db, "w3")

	total, err := repo.Count(ctx, repository.WhereID(first.ID))
	if err != nil || total != 1 {
		t.Fatalf("WhereID 结果不符: total=%d err=%v", total, err)
	}

	total, err = repo.Count(ctx, repository.WhereIDs(first.ID, second.ID))
	if err != nil || total != 2 {
		t.Fatalf("WhereIDs 结果不符: total=%d err=%v", total, err)
	}

	if err := repo.DeleteBatch(ctx, first.ID, second.ID); err != nil {
		t.Fatalf("DeleteBatch 失败: %v", err)
	}
	total, err = repo.Count(ctx)
	if err != nil || total != 1 {
		t.Fatalf("批量软删后应剩 1 条: total=%d err=%v", total, err)
	}
}
