package dto

import (
	"golang-server-starter/internal/model"
	"golang-server-starter/internal/pkg/query"
)

// OrgQuery 组织列表查询参数。
//
// 内嵌 query.Page，获得 current / size / order_by / order 四个查询参数。
// 这里**不加 binding tag**：分页参数越界（如 size=99999）属于客户端习惯问题，
// 由仓储层静默归一化到默认值，比直接 400 更友好；order_by 白名单校验同理。
type OrgQuery struct {
	query.Page
}

// NewOrgs 实体列表转响应列表，保持与入参一一对应
func NewOrgs(items []model.Org) []*Org {
	out := make([]*Org, 0, len(items))
	for i := range items {
		out = append(out, NewOrg(&items[i]))
	}
	return out
}
