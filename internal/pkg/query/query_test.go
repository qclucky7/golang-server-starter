package query_test

import (
	"testing"

	"golang-server-starter/internal/pkg/query"
)

func TestPageNormalize(t *testing.T) {
	cases := []struct {
		name       string
		in         query.Page
		wantCur    int
		wantSize   int
		wantOrder  string
		wantOrderB string
	}{
		{"零值回落默认", query.Page{}, 1, 10, "desc", "id"},
		{"负数回落默认", query.Page{Current: -3, Size: -1}, 1, 10, "desc", "id"},
		{"超上限截断", query.Page{Size: 9999}, 1, 200, "desc", "id"},
		{"合法值保留", query.Page{Current: 3, Size: 20, OrderBy: "created_time", Order: "ASC"}, 3, 20, "asc", "created_time"},
		{"非法排序方向回落", query.Page{Order: "drop table"}, 1, 10, "desc", "id"},
		{"注入字段名回落", query.Page{OrderBy: "id; drop table users"}, 1, 10, "desc", "id"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := tc.in
			page.Normalize()
			if page.Current != tc.wantCur || page.Size != tc.wantSize {
				t.Fatalf("分页归一化不符: current=%d size=%d", page.Current, page.Size)
			}
			if page.Order != tc.wantOrder || page.OrderBy != tc.wantOrderB {
				t.Fatalf("排序归一化不符: order_by=%s order=%s", page.OrderBy, page.Order)
			}
		})
	}
}

func TestPageOffsetAndOrderClause(t *testing.T) {
	page := query.Page{Current: 3, Size: 20, OrderBy: "created_time", Order: "desc"}
	page.Normalize()

	if page.Offset() != 40 {
		t.Fatalf("Offset 计算错误: %d", page.Offset())
	}
	if page.OrderClause() != "created_time desc" {
		t.Fatalf("OrderClause 错误: %s", page.OrderClause())
	}
}

func TestPageAcceptTableQualifiedColumn(t *testing.T) {
	page := query.Page{OrderBy: "users.created_time", Order: "asc"}
	page.Normalize()

	if page.OrderBy != "users.created_time" {
		t.Fatalf("带表名的字段应被接受: %s", page.OrderBy)
	}
}

// TestPageCurrentClamped 页码必须有上限。
// 回归点：不夹住 Current 时 (Current-1)*Size 会整数溢出，
// 溢出成负数后 SQLite 把负 OFFSET 当 0，静默返回第 1 页数据。
func TestPageCurrentClamped(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"零值回落", 0, query.DefaultCurrent},
		{"负数回落", -7, query.DefaultCurrent},
		{"正常值保留", 42, 42},
		{"上限值保留", query.MaxCurrent, query.MaxCurrent},
		{"超上限截断", query.MaxCurrent + 1, query.MaxCurrent},
		{"极大值截断", 1 << 62, query.MaxCurrent},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := query.Page{Current: tc.in}
			page.Normalize()
			if page.Current != tc.want {
				t.Fatalf("Current 期望 %d，实际 %d", tc.want, page.Current)
			}
		})
	}
}

// TestPageOffsetNeverNegative 夹住上限后 Offset 必须为正。
func TestPageOffsetNeverNegative(t *testing.T) {
	cases := []struct {
		name    string
		current int
		size    int
	}{
		{"上限页码", query.MaxCurrent, query.MaxSize},
		{"极大页码", 1 << 62, query.MaxSize},
		{"极大每页", query.MaxCurrent, 1 << 30},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := query.Page{Current: tc.current, Size: tc.size}
			page.Normalize()
			if offset := page.Offset(); offset < 0 {
				t.Fatalf("Offset 不应为负: %d", offset)
			}
		})
	}
}

// TestPageValidateOrderBy 排序字段必须落在模型真实字段内，且被规整成真实列名。
//
// 回归点一：列名格式合法但列不存在时会打到数据库报错，最终以 500 返回。
// 回归点二：白名单若原样放行 Go 字段名（CreatedTime），GORM 会把它拼进 SQL，
// 数据库并不认，同样报 no such column —— 必须翻译成列名。
func TestPageValidateOrderBy(t *testing.T) {
	// 与 repository.orderableColumns 构造的映射同构：列名 + Go 字段名，值都是列名
	cols := map[string]string{
		"id":           "id",
		"username":     "username",
		"created_time": "created_time",
		"CreatedTime":  "created_time",
	}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"命中列名", "username", "username"},
		{"Go 字段名翻译成列名", "CreatedTime", "created_time"},
		{"带表前缀命中", "users.username", "users.username"},
		{"带表前缀 + Go 字段名", "users.CreatedTime", "users.created_time"},
		{"不在白名单回落", "nonexistent_column", "id"},
		{"SQL 保留字回落", "select", "id"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := query.Page{OrderBy: tc.in, Order: "asc"}
			page.Normalize()
			page.ValidateOrderBy(cols)

			if page.OrderBy != tc.want {
				t.Fatalf("order_by 期望 %q，实际 %q", tc.want, page.OrderBy)
			}
		})
	}
}

// TestPageValidateOrderByEmptyCols 拿不到字段集时应保持原值，不能误伤成 id。
func TestPageValidateOrderByEmptyCols(t *testing.T) {
	page := query.Page{OrderBy: "created_time", Order: "desc"}
	page.Normalize()
	page.ValidateOrderBy(nil)

	if page.OrderBy != "created_time" {
		t.Fatalf("无法校验时不应改动 order_by，实际 %q", page.OrderBy)
	}
}
