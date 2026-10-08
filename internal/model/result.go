package model

import "math"

// 本文件保留参考项目（browser-openapi-core）的公共数据结构约定：
// 每个响应体都带一个 model 字段用于自描述类型，便于客户端做分发与调试。
//
// 这四个结构是**固定的响应契约**，与「当前有没有接口在用」无关 ——
// 客户端按 model 字段分发，形状必须稳定，所以即使某个形状暂时没有调用点也保留。

// DataSetResult 数据集结果
type DataSetResult[T any] struct {
	Model string `json:"model"` // 固定为 data.set
	Datas []T    `json:"datas"` // 数据列表
	Total int    `json:"total"` // 数据总量
}

// NewDataSetResult 构造数据集结果
func NewDataSetResult[T any](datas []T) *DataSetResult[T] {
	if datas == nil {
		datas = make([]T, 0)
	}
	return &DataSetResult[T]{
		Model: "data.set",
		Datas: datas,
		Total: len(datas),
	}
}

// PageResult 分页信息
type PageResult struct {
	Model     string `json:"model"`      // 固定为 page
	Current   int    `json:"current"`    // 当前页码，从 1 开始
	Size      int    `json:"size"`       // 每页条数
	Total     int    `json:"total"`      // 总条数
	TotalPage int    `json:"total_page"` // 总页数
}

// NewPageResult 构造分页信息，totalPage 由 total 与 size 自动推导。
func NewPageResult(current, size, total int) *PageResult {
	if size <= 0 {
		size = 10
	}
	return &PageResult{
		Model:     "page",
		Current:   current,
		Size:      size,
		Total:     total,
		TotalPage: int(math.Ceil(float64(total) / float64(size))),
	}
}

// GridResult 分页数据集结果
type GridResult[T any] struct {
	Model  string            `json:"model"`  // 固定为 grid.result
	Page   *PageResult       `json:"page"`   // 分页信息
	Result *DataSetResult[T] `json:"result"` // 数据集
}

// NewGridResult 构造分页数据集结果
func NewGridResult[T any](datas []T, page *PageResult) *GridResult[T] {
	return &GridResult[T]{
		Model:  "grid.result",
		Page:   page,
		Result: NewDataSetResult(datas),
	}
}

// SystemErrorItemResult 单条错误
type SystemErrorItemResult struct {
	Model   string `json:"model"`   // 固定为 error
	Code    string `json:"code"`    // 错误码
	Message string `json:"message"` // 错误描述
}

// NewSystemErrorItemResult 构造单条错误
func NewSystemErrorItemResult(code, message string) SystemErrorItemResult {
	return SystemErrorItemResult{Model: "error", Code: code, Message: message}
}

// SystemErrorResult 统一错误响应体
type SystemErrorResult struct {
	Model     string                                `json:"model"` // 固定为 errors
	Errors    *DataSetResult[SystemErrorItemResult] `json:"errors"`
	RequestID string                                `json:"request_id,omitempty"`
}

// NewSystemErrorResult 构造统一错误响应体
func NewSystemErrorResult(items []SystemErrorItemResult) SystemErrorResult {
	return SystemErrorResult{
		Model:  "errors",
		Errors: NewDataSetResult(items),
	}
}
