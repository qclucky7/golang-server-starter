package service

import (
	"context"
	"strings"

	"golang-server-starter/internal/apperr"
	"golang-server-starter/internal/model"
	"golang-server-starter/internal/pkg/query"
	"golang-server-starter/internal/repository"
)

// 组织域：默认组织命名 + 组织列表查询。
//
// 后续扩展（账号加入他人组织、成员管理、邀请）时在 OrgService 上追加方法，
// 关联关系落到 org_user 表，model.Org 不需要改动。

// 默认组织名后缀
const defaultOrgNameSuffix = "的组织"

// DefaultOrgName 生成账号默认组织名。
//
// 优先用昵称（注册时昵称缺省会回落为用户名，所以这里拿到的一般不为空），
// 两者都为空时用「我的组织」兜底，避免落一个空名字进库。
func DefaultOrgName(nickname, username string) string {
	name := strings.TrimSpace(nickname)
	if name == "" {
		name = strings.TrimSpace(username)
	}
	if name == "" {
		return "我的组织"
	}
	return name + defaultOrgNameSuffix
}

// OrgService 组织查询业务。
type OrgService struct {
	orgs *repository.OrgRepository
}

// NewOrgService 创建组织业务实例
func NewOrgService(orgs *repository.OrgRepository) *OrgService {
	return &OrgService{orgs: orgs}
}

// ListByOwner 分页查询账号拥有的组织。
//
// ownerID 由调用方从令牌解析得到（contextx.AccountID），不接受请求参数传入 ——
// 因此不存在「传别人的 ID 查别人组织」的越权路径。
//
// page 会被仓储层就地归一化，返回后调用方可直接用它构造分页信息。
func (s *OrgService) ListByOwner(ctx context.Context, ownerID model.ID, page *query.Page) ([]model.Org, int64, error) {
	items, total, err := s.orgs.PageByOwner(ctx, ownerID, page)
	if err != nil {
		return nil, 0, apperr.ErrDatabase
	}
	return items, total, nil
}
