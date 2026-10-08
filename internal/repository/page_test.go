package repository_test

import (
	"context"
	"testing"

	"golang-server-starter/internal/model"
	"golang-server-starter/internal/pkg/query"
	"golang-server-starter/internal/repository"
)

// TestPageOrderByFallsBackOnUnknownColumn 客户端传不存在的排序列时，
// 应静默回落到 id 并正常返回，而不是把「列不存在」变成数据库错误。
//
// 回归点：修复前 order_by=nonexistent_column 会打到数据库报错，
// 最终以 500 + common.database.error 返回，并把 SQL 错误写进 error 日志。
func TestPageOrderByFallsBackOnUnknownColumn(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	newAccount(t, db, "alice")
	newAccount(t, db, "bob")

	cases := []struct {
		name    string
		orderBy string
	}{
		{"不存在的列", "nonexistent_column"},
		{"SQL 片段", "id; DROP TABLE accounts"},
		{"SQL 保留字", "select"},
		{"空值", ""},
		{"带表前缀但列不存在", "accounts.nope"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := &query.Page{OrderBy: tc.orderBy, Order: "asc"}
			items, total, err := repo.Page(ctx, page)
			if err != nil {
				t.Fatalf("不应返回错误: %v", err)
			}
			if total != 2 || len(items) != 2 {
				t.Fatalf("期望 2 条，实际 total=%d len=%d", total, len(items))
			}
			if page.OrderBy != "id" {
				t.Fatalf("非法排序列应回落到 id，实际 %q", page.OrderBy)
			}
		})
	}
}

// TestPageOrderByValidColumn 模型真实字段应被接受，并被规整成真实数据库列名。
//
// 注意 CreatedTime 这类 Go 字段名：GORM 把 Order 子句原样拼进 SQL，
// 直接放行会报 no such column，所以必须翻译成 created_time。
func TestPageOrderByValidColumn(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	newAccount(t, db, "alice")
	newAccount(t, db, "bob")

	cases := []struct {
		in   string
		want string
	}{
		{"id", "id"},
		{"username", "username"},
		{"email", "email"},
		{"created_time", "created_time"},
		{"CreatedTime", "created_time"},
		{"LastLoginAt", "last_login_at"},
		{"accounts.username", "accounts.username"},
		{"accounts.CreatedTime", "accounts.created_time"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			page := &query.Page{OrderBy: tc.in, Order: "asc"}
			if _, _, err := repo.Page(ctx, page); err != nil {
				t.Fatalf("合法字段 %q 不应报错: %v", tc.in, err)
			}
			if page.OrderBy != tc.want {
				t.Fatalf("order_by %q 期望规整为 %q，实际 %q", tc.in, tc.want, page.OrderBy)
			}
		})
	}
}

// TestPageDeepPaginationNoOverflow 极大页码必须被夹住。
//
// 回归点：不夹 Current 时 (Current-1)*Size 溢出成负数，
// SQLite 把负 OFFSET 当 0，于是「深页码」静默返回第 1 页数据。
func TestPageDeepPaginationNoOverflow(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	newAccount(t, db, "alice")

	page := &query.Page{Current: 1 << 62, Size: query.MaxSize}
	items, total, err := repo.Page(ctx, page)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if page.Current != query.MaxCurrent {
		t.Fatalf("Current 应被夹到 %d，实际 %d", query.MaxCurrent, page.Current)
	}
	if total != 1 {
		t.Fatalf("total 期望 1，实际 %d", total)
	}
	if len(items) != 0 {
		t.Fatalf("深页码应返回空列表，实际返回 %d 条（疑似负偏移被当成 0）", len(items))
	}
}

// TestPageNormalPagination 常规分页行为不受影响。
func TestPageNormalPagination(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.New[model.Account](db)

	for _, name := range []string{"u1", "u2", "u3", "u4", "u5"} {
		newAccount(t, db, name)
	}

	page := &query.Page{Current: 2, Size: 2, OrderBy: "id", Order: "asc"}
	items, total, err := repo.Page(ctx, page)
	if err != nil {
		t.Fatalf("分页查询失败: %v", err)
	}
	if total != 5 {
		t.Fatalf("total 期望 5，实际 %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("第 2 页期望 2 条，实际 %d", len(items))
	}
	if items[0].Username != "u3" {
		t.Fatalf("第 2 页首条期望 u3，实际 %s", items[0].Username)
	}
}
