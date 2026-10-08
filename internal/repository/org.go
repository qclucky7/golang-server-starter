package repository

import (
	"context"

	"golang-server-starter/internal/model"
	"golang-server-starter/internal/pkg/query"

	"gorm.io/gorm"
)

// OrgRepository 组织仓储，在泛型 CRUD 基础上追加组织域专属查询。
type OrgRepository struct {
	*Repository[model.Org]
	db *gorm.DB
}

// NewOrgRepository 创建组织仓储
func NewOrgRepository(db *gorm.DB) *OrgRepository {
	return &OrgRepository{
		Repository: New[model.Org](db),
		db:         db,
	}
}

// FindDefaultByOwner 查询账号的默认组织，未找到返回 (nil, nil)
func (r *OrgRepository) FindDefaultByOwner(ctx context.Context, ownerID model.ID) (*model.Org, error) {
	return r.Get(ctx, WhereEq("owner_id", ownerID), WhereEq("is_default", true))
}

// ListByOwner 查询账号拥有的全部组织
func (r *OrgRepository) ListByOwner(ctx context.Context, ownerID model.ID) ([]model.Org, error) {
	return r.List(ctx, WhereEq("owner_id", ownerID))
}

// PageByOwner 分页查询账号拥有的组织，返回列表与总条数。
//
// 分页参数归一化（越界回落默认值）与 order_by 白名单校验都由
// 泛型仓储的 Page 完成，这里只负责追加 owner_id 过滤条件。
func (r *OrgRepository) PageByOwner(ctx context.Context, ownerID model.ID, page *query.Page) ([]model.Org, int64, error) {
	return r.Page(ctx, page, WhereEq("owner_id", ownerID))
}
