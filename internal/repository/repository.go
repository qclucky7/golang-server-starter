// Package repository 数据访问层。
//
// Repository 是基于 GORM 的泛型 CRUD 基类，覆盖绝大多数单表操作；
// 表专属的查询（如按用户名查找）在各自的仓储里以方法形式追加。
package repository

import (
	"context"
	"errors"
	"sync"

	"golang-server-starter/internal/model"
	"golang-server-starter/internal/pkg/query"

	"gorm.io/gorm"
)

// Scope GORM 查询作用域，用于在调用处追加条件。
// 使用类型别名（=）而非定义类型，保证可直接传给 gorm 的 Scopes()。
type Scope = func(*gorm.DB) *gorm.DB

// Repository 泛型 CRUD 仓储
type Repository[T model.Entity] struct {
	db  *gorm.DB
	set *columnSet
}

// columnSet 惰性解析并缓存模型的排序字段映射，供排序字段校验使用。
//
// 用指针持有，让 WithDB 派生的事务仓储共享同一份缓存，
// 避免每个事务都重新解析一次 schema。
type columnSet struct {
	once sync.Once
	cols map[string]string
}

// New 创建泛型仓储
func New[T model.Entity](db *gorm.DB) *Repository[T] {
	return &Repository[T]{db: db, set: &columnSet{}}
}

// DB 返回底层连接，便于编写复杂查询
func (r *Repository[T]) DB() *gorm.DB { return r.db }

// WithDB 基于给定连接派生新仓储（事务场景使用）
func (r *Repository[T]) WithDB(db *gorm.DB) *Repository[T] {
	return &Repository[T]{db: db, set: r.set}
}

func (r *Repository[T]) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// orderableColumns 返回「可排序名称 → 真实数据库列名」映射。
//
// 同时收录数据库列名与 Go 字段名，两者的目标值都是真实列名：
// GORM 把 Order 子句原样拼进 SQL，只认列名，所以 Go 字段名必须被翻译。
// 解析失败返回 nil，调用方应视为「无法校验」而不是「全部非法」。
func (r *Repository[T]) orderableColumns() map[string]string {
	r.set.once.Do(func() {
		var entity T
		stmt := &gorm.Statement{DB: r.db}
		if err := stmt.Parse(&entity); err != nil || stmt.Schema == nil {
			return
		}
		cols := make(map[string]string, len(stmt.Schema.Fields)*2)
		for _, field := range stmt.Schema.Fields {
			cols[field.DBName] = field.DBName
			cols[field.Name] = field.DBName
		}
		r.set.cols = cols
	})
	return r.set.cols
}

// ---------------------------------------------------------------------------
// 写操作
// ---------------------------------------------------------------------------

// Create 新增单条记录，回填主键
func (r *Repository[T]) Create(ctx context.Context, entity *T) error {
	return r.session(ctx).Create(entity).Error
}

// CreateBatch 批量新增
func (r *Repository[T]) CreateBatch(ctx context.Context, entities []*T) error {
	if len(entities) == 0 {
		return nil
	}
	return r.session(ctx).Create(&entities).Error
}

// Update 全量更新（基于主键，忽略零值字段的语义由 GORM 决定，推荐用 UpdateFields 做局部更新）
func (r *Repository[T]) Update(ctx context.Context, entity *T) error {
	return r.session(ctx).Save(entity).Error
}

// UpdateFields 按主键局部更新指定字段
func (r *Repository[T]) UpdateFields(ctx context.Context, id model.ID, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	var entity T
	return r.session(ctx).Model(&entity).Where("id = ?", id).Updates(fields).Error
}

// UpdateBy 按条件批量更新字段，返回受影响行数
func (r *Repository[T]) UpdateBy(ctx context.Context, fields map[string]any, scopes ...Scope) (int64, error) {
	if len(fields) == 0 {
		return 0, nil
	}
	var entity T
	result := r.session(ctx).Model(&entity).Scopes(scopes...).Updates(fields)
	return result.RowsAffected, result.Error
}

// Delete 按主键删除（软删除实体写入 deleted_time）
func (r *Repository[T]) Delete(ctx context.Context, id model.ID) error {
	var entity T
	return r.session(ctx).Where("id = ?", id).Delete(&entity).Error
}

// DeleteBatch 按主键批量删除
func (r *Repository[T]) DeleteBatch(ctx context.Context, ids ...model.ID) error {
	if len(ids) == 0 {
		return nil
	}
	var entity T
	return r.session(ctx).Where("id IN ?", ids).Delete(&entity).Error
}

// DeleteBy 按条件批量删除，返回受影响行数
func (r *Repository[T]) DeleteBy(ctx context.Context, scopes ...Scope) (int64, error) {
	if len(scopes) == 0 {
		return 0, errors.New("DeleteBy 需要至少一个查询条件")
	}
	var entity T
	result := r.session(ctx).Scopes(scopes...).Delete(&entity)
	return result.RowsAffected, result.Error
}

// ---------------------------------------------------------------------------
// 读操作
// ---------------------------------------------------------------------------

// GetByID 按主键查询，未找到返回 (nil, nil)
func (r *Repository[T]) GetByID(ctx context.Context, id model.ID) (*T, error) {
	var entity T
	if err := r.session(ctx).Where("id = ?", id).First(&entity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &entity, nil
}

// Get 按条件查询单条，未找到返回 (nil, nil)
func (r *Repository[T]) Get(ctx context.Context, scopes ...Scope) (*T, error) {
	var entity T
	if err := r.session(ctx).Scopes(scopes...).First(&entity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &entity, nil
}

// List 按条件查询列表
func (r *Repository[T]) List(ctx context.Context, scopes ...Scope) ([]T, error) {
	items := make([]T, 0)
	if err := r.session(ctx).Scopes(scopes...).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// Page 分页查询，返回列表与总条数。
//
// order_by 会按模型真实字段白名单校验：客户端传了不存在的列时静默回落到 id，
// 避免把「列不存在」这种客户端输入错误变成 500 + 数据库错误日志。
func (r *Repository[T]) Page(ctx context.Context, page *query.Page, scopes ...Scope) ([]T, int64, error) {
	page.Normalize()
	page.ValidateOrderBy(r.orderableColumns())
	var entity T

	var total int64
	countDB := r.session(ctx).Model(&entity).Scopes(scopes...)
	if err := countDB.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return make([]T, 0), 0, nil
	}

	items := make([]T, 0)
	listDB := r.session(ctx).Model(&entity).Scopes(scopes...)
	if err := listDB.
		Order(page.OrderClause()).
		Offset(page.Offset()).
		Limit(page.Size).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Count 统计条数
func (r *Repository[T]) Count(ctx context.Context, scopes ...Scope) (int64, error) {
	var entity T
	var total int64
	err := r.session(ctx).Model(&entity).Scopes(scopes...).Count(&total).Error
	return total, err
}

// Exists 判断是否存在
func (r *Repository[T]) Exists(ctx context.Context, scopes ...Scope) (bool, error) {
	total, err := r.Count(ctx, scopes...)
	return total > 0, err
}

// ---------------------------------------------------------------------------
// 事务
// ---------------------------------------------------------------------------

// Transaction 在事务中执行 fn，fn 内应使用传入的仓储实例以保证同事务
func (r *Repository[T]) Transaction(ctx context.Context, fn func(tx *Repository[T]) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(r.WithDB(tx))
	})
}

// TransactionDB 在事务中执行 fn，并把原始 *gorm.DB 交给 fn。
//
// 用于**跨仓储**的原子写入：fn 内必须用 WithDB(tx) 派生每个仓储，例如
//
//	err := accountRepo.TransactionDB(ctx, func(tx *gorm.DB) error {
//		if err := accountRepo.WithDB(tx).Create(ctx, account); err != nil {
//			return err
//		}
//		return orgRepo.WithDB(tx).Create(ctx, org)
//	})
//
// 直接用容器里注入的仓储会绕过事务、落到另一条连接上，回滚时不会生效。
func (r *Repository[T]) TransactionDB(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// ---------------------------------------------------------------------------
// 常用 Scope
// ---------------------------------------------------------------------------

// WhereID 主键条件
func WhereID(id model.ID) Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Where("id = ?", id) }
}

// WhereIDs 主键集合条件
func WhereIDs(ids ...model.ID) Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Where("id IN ?", ids) }
}

// WhereEq 等值条件
func WhereEq(column string, value any) Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Where(column+" = ?", value) }
}

// WhereLike 模糊匹配条件，value 会自动包裹 %%
func WhereLike(column, value string) Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Where(column+" LIKE ?", "%"+value+"%") }
}

// OrderBy 排序条件
func OrderBy(column string) Scope {
	return func(db *gorm.DB) *gorm.DB { return db.Order(column) }
}
