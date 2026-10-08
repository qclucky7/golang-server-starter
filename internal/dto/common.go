package dto

import (
	"gin-quick-start/internal/model"
	"gin-quick-start/internal/pkg/query"
)

// NewPageResult 依据查询参数与总条数构造分页信息
//
// 属于固定的响应契约（配合 response.Page 使用），即使当前没有列表接口也保留。
func NewPageResult(page query.Page, total int) *model.PageResult {
	return model.NewPageResult(page.Current, page.Size, total)
}
