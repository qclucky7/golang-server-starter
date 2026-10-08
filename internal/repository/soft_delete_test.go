package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"golang-server-starter/internal/model"
	"golang-server-starter/internal/pkg/query"
	"golang-server-starter/internal/repository"

	"gorm.io/gorm"
)

// TestBaseAutoTimestamps 验证改名后自动时间戳仍然生效。
//
// 回归点：GORM 的自动时间戳按「字段名」识别，只认 CreatedAt / UpdatedAt
// （见 gorm/schema/field.go）。本项目把字段改名为 CreatedTime / UpdatedTime，
// 必须靠 autoCreateTime / autoUpdateTime 两个 tag 兜住；
// 少了 tag 不会报错，字段会静默保持零值。
func TestBaseAutoTimestamps(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	account := newAccount(t, db, "ts")

	// 插入后两个时间戳都必须已填充 —— 这一步就能同时检出两个 tag 是否缺失
	if account.CreatedTime.IsZero() {
		t.Fatal("CreatedTime 未被填充，检查 gorm:\"autoCreateTime\" tag")
	}
	if account.UpdatedTime.IsZero() {
		t.Fatal("UpdatedTime 未被填充，检查 gorm:\"autoUpdateTime\" tag")
	}
	if account.DeletedTime != 0 {
		t.Fatalf("新记录 DeletedTime 应为 0，实际 %d", account.DeletedTime)
	}

	// 把 updated_time 预置成一天前（UpdateColumn 不触发 autoUpdateTime），
	// 再走一次普通更新，验证 autoUpdateTime 真的会回填。
	old := time.Now().Add(-24 * time.Hour)
	if err := db.Model(&model.Account{}).Where("id = ?", account.ID).
		UpdateColumn("updated_time", old).Error; err != nil {
		t.Fatalf("预置 updated_time 失败: %v", err)
	}

	preset, err := repo.GetByID(ctx, account.ID)
	if err != nil || preset == nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !preset.UpdatedTime.Before(time.Now().Add(-12 * time.Hour)) {
		t.Fatalf("预置未生效，updated_time=%v", preset.UpdatedTime)
	}

	if err := repo.UpdateFields(ctx, account.ID, map[string]any{"nickname": "新昵称"}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	got, err := repo.GetByID(ctx, account.ID)
	if err != nil || got == nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !got.UpdatedTime.After(old) {
		t.Fatalf("autoUpdateTime 未回填: updated_time=%v（预置值 %v）", got.UpdatedTime, old)
	}
	if !got.CreatedTime.Equal(account.CreatedTime) {
		t.Fatalf("CreatedTime 不应被更新改变: %v -> %v", account.CreatedTime, got.CreatedTime)
	}
}

// TestSoftDeletePlugin 验证 soft_delete 插件（unix 秒格式）的完整语义：
//
//  1. 查询自动追加 deleted_time = 0
//  2. Delete 生成 UPDATE 而非 DELETE，数据仍在库里
//  3. deleted_time 写入的是 unix 秒
//  4. Unscoped 能读到已软删记录
//  5. Unscoped 删除才是物理删除
func TestSoftDeletePlugin(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	account := newAccount(t, db, "soft")
	accountID := account.ID

	// 软删前可见
	if got, err := repo.GetByID(ctx, accountID); err != nil || got == nil {
		t.Fatalf("软删前应能查到: err=%v", err)
	}

	// Delete 必须是 UPDATE，且写的是 deleted_time
	dryRun := db.Session(&gorm.Session{DryRun: true}).
		Where("id = ?", accountID).Delete(&model.Account{})
	sql := strings.TrimSpace(dryRun.Statement.SQL.String())
	if !strings.HasPrefix(strings.ToUpper(sql), "UPDATE") {
		t.Fatalf("软删除应生成 UPDATE，实际: %s", sql)
	}
	// DryRun 下参数是 ? 占位符，故只断言 deleted_time 出现两次：
	// 一次在 SET（写入删除时间戳），一次在 WHERE（守卫，避免重复删除）
	if strings.Count(sql, "deleted_time") < 2 {
		t.Fatalf("UPDATE 应同时写入 deleted_time 并带 deleted_time 守卫条件，实际: %s", sql)
	}

	// 执行软删
	if err := repo.Delete(ctx, accountID); err != nil {
		t.Fatalf("软删除失败: %v", err)
	}

	// 常规查询必须查不到
	if got, err := repo.GetByID(ctx, accountID); err != nil || got != nil {
		t.Fatalf("软删后常规查询应返回 nil: got=%v err=%v", got, err)
	}
	if _, total, err := repo.Page(ctx, &query.Page{}); err != nil || total != 0 {
		t.Fatalf("软删后分页应为 0 条: total=%d err=%v", total, err)
	}

	// 数据仍在库里，且 deleted_time 是 unix 秒
	var raw struct {
		DeletedTime int64
	}
	if err := db.Unscoped().Model(&model.Account{}).
		Select("deleted_time").Where("id = ?", accountID).Scan(&raw).Error; err != nil {
		t.Fatalf("Unscoped 查询失败: %v", err)
	}
	if raw.DeletedTime == 0 {
		t.Fatal("deleted_time 应写入 unix 秒，实际为 0（记录可能被物理删除）")
	}
	now := time.Now().Unix()
	if raw.DeletedTime < now-60 || raw.DeletedTime > now+60 {
		t.Fatalf("deleted_time 不像是 unix 秒: %d（当前 %d）", raw.DeletedTime, now)
	}

	// Unscoped 能读到记录本身
	var count int64
	if err := db.Unscoped().Model(&model.Account{}).
		Where("id = ?", accountID).Count(&count).Error; err != nil {
		t.Fatalf("Unscoped 计数失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("Unscoped 应能读到已软删记录，实际 %d 条", count)
	}

	// Unscoped 删除才是物理删除
	if err := db.Unscoped().Where("id = ?", accountID).Delete(&model.Account{}).Error; err != nil {
		t.Fatalf("物理删除失败: %v", err)
	}
	if err := db.Unscoped().Model(&model.Account{}).
		Where("id = ?", accountID).Count(&count).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("物理删除后应无记录，实际 %d 条", count)
	}
}

// TestSoftDeletedUniqueIndexStillBlocks 单列唯一索引在软删后仍占位（防抢注）。
// 这是当前的有意设计；若改成「软删后可复用」，需改用
// (唯一列, deleted_time) 复合唯一索引 —— 用 unix 秒而非 NULL 才能生效。
func TestSoftDeletedUniqueIndexStillBlocks(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	first := newAccount(t, db, "reuse")
	if err := repo.Delete(ctx, first.ID); err != nil {
		t.Fatalf("软删除失败: %v", err)
	}

	dup := &model.Account{
		Username: "reuse",
		Email:    "other@example.com",
		Password: "x",
	}
	if err := repo.Create(ctx, dup); err == nil {
		t.Fatal("软删记录仍应占用唯一用户名（当前设计），重复创建不应成功")
	}
}
