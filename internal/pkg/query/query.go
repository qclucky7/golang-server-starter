// Package query 定义列表查询的通用参数（分页 / 排序）。
package query

import (
	"regexp"
	"strings"
)

const (
	// DefaultCurrent 默认页码
	DefaultCurrent = 1
	// DefaultSize 默认每页条数
	DefaultSize = 10
	// MaxSize 每页条数上限
	MaxSize = 200
	// MaxCurrent 页码上限。
	// 两个作用：一是保证 (Current-1)*Size 不会整数溢出，
	// 二是挡住「超深分页」——大 OFFSET 会让数据库全表扫过前 N 行。
	MaxCurrent = 1_000_000
)

// safeColumn 只允许字母、数字、下划线、点，防止 SQL 注入
var safeColumn = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)*$`)

// Page 分页与排序参数，可直接作为 handler 的 query 绑定目标。
type Page struct {
	Current int    `form:"current" json:"current"`   // 当前页码，从 1 开始
	Size    int    `form:"size" json:"size"`         // 每页条数
	OrderBy string `form:"order_by" json:"order_by"` // 排序字段
	Order   string `form:"order" json:"order"`       // asc / desc
}

// Normalize 归一化参数，越界值回落到默认值
func (p *Page) Normalize() {
	if p.Current <= 0 {
		p.Current = DefaultCurrent
	}
	if p.Current > MaxCurrent {
		p.Current = MaxCurrent
	}
	if p.Size <= 0 {
		p.Size = DefaultSize
	}
	if p.Size > MaxSize {
		p.Size = MaxSize
	}
	if !strings.EqualFold(p.Order, "asc") && !strings.EqualFold(p.Order, "desc") {
		p.Order = "desc"
	}
	p.Order = strings.ToLower(p.Order)
	if p.OrderBy == "" || !safeColumn.MatchString(p.OrderBy) {
		p.OrderBy = "id"
	}
}

// ValidateOrderBy 按调用方给出的字段白名单校验并规整排序字段。
//
// Normalize 只挡得住「格式非法」（分号、空格、引号等注入载荷），
// 挡不住「格式合法但列不存在」——例如 order_by=nonexistent_column。
// 这种查询会一路打到数据库报错，最终以 500 + common.database.error 返回，
// 既用错状态码、又把 SQL 错误写进 error 日志（dev 环境还会透出原文）。
// 因此需要真实字段集才能判定，由仓储层传入「可排序名称 → 真实列名」映射。
//
// 映射里同时收录数据库列名与 Go 字段名，两者都会被规整成真实列名 ——
// GORM 是把 Order 子句原样拼进 SQL 的，传 Go 字段名（CreatedTime）数据库并不认，
// 必须在这里翻译成列名（created_time）。
//
// 不在白名单内回落到 id；cols 为空表示无法校验（如 schema 解析失败），保持原值。
// 允许 "accounts.created_time" 这类带表前缀写法，前缀原样保留、只校验并翻译末段。
func (p *Page) ValidateOrderBy(cols map[string]string) {
	if len(cols) == 0 {
		return
	}

	prefix := ""
	name := p.OrderBy
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		prefix = name[:idx+1]
		name = name[idx+1:]
	}

	column, ok := cols[name]
	if !ok {
		p.OrderBy = "id"
		return
	}
	p.OrderBy = prefix + column
}

// Offset 计算 SQL OFFSET。
//
// 用 int64 相乘再收敛回 int：Normalize 已把 Current 与 Size 夹在上限内，
// 正常不会溢出；这里是防御性写法，避免日后放宽 MaxSize 时静默算出负偏移
// （SQLite 会把负 OFFSET 当 0，从而返回第 1 页数据 —— 错得不声不响）。
func (p *Page) Offset() int {
	return int(int64(p.Current-1) * int64(p.Size))
}

// OrderClause 生成 GORM Order 子句，例如 "id desc"
func (p *Page) OrderClause() string { return p.OrderBy + " " + p.Order }
