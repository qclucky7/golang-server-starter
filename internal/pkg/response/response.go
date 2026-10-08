// Package response 统一 HTTP 响应体构造。
//
// 成功响应（单对象）:
//
//	{"model":"account","data":{...},"request_id":"..."}
//
// 成功响应（数据集）:
//
//	{"model":"data.set","datas":[...],"total":10,"request_id":"..."}
//
// 成功响应（分页）:
//
//	{"model":"grid.result","page":{...},"result":{...},"request_id":"..."}
//
// 失败响应（保留参考项目 SystemErrorResult 结构）:
//
//	{"model":"errors","errors":{"model":"data.set","datas":[{"model":"error","code":"...","message":"..."}],"total":1},"request_id":"..."}
//
// 这四个形状是**固定的响应契约**，客户端按 model 字段分发，形状必须稳定。
// 某个形状暂时没有接口在用也保留 —— 它是模板的一部分，不是死代码。
package response

import (
	"errors"
	"net/http"

	"gin-quick-start/internal/apperr"
	"gin-quick-start/internal/model"
	"gin-quick-start/internal/pkg/contextx"

	"github.com/gin-gonic/gin"
)

// Body 单对象响应
type Body struct {
	Model     string `json:"model"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// ListBody 数据集响应
type ListBody struct {
	Model     string `json:"model"`
	Datas     []any  `json:"datas"`
	Total     int    `json:"total"`
	RequestID string `json:"request_id,omitempty"`
}

// PageBody 分页数据集响应
type PageBody struct {
	Model     string            `json:"model"`
	Page      *model.PageResult `json:"page"`
	Result    *ListBody         `json:"result"`
	RequestID string            `json:"request_id,omitempty"`
}

// OK 返回单对象成功响应
func OK(c *gin.Context, model string, data any) {
	c.JSON(http.StatusOK, Body{
		Model:     model,
		Data:      data,
		RequestID: contextx.RequestID(c),
	})
}

// List 返回数据集成功响应
func List(c *gin.Context, datas []any, total int) {
	if datas == nil {
		datas = make([]any, 0)
	}
	c.JSON(http.StatusOK, ListBody{
		Model:     "data.set",
		Datas:     datas,
		Total:     total,
		RequestID: contextx.RequestID(c),
	})
}

// Page 返回分页数据集成功响应
func Page(c *gin.Context, datas []any, page *model.PageResult) {
	if datas == nil {
		datas = make([]any, 0)
	}
	c.JSON(http.StatusOK, PageBody{
		Model:     "grid.result",
		Page:      page,
		Result:    &ListBody{Model: "data.set", Datas: datas, Total: page.Total},
		RequestID: contextx.RequestID(c),
	})
}

// NoContent 返回空成功响应
//
// 注意返回的是 HTTP 200 + {"model":"empty"}，不是裸 204 —— 统一响应体约定优先，
// 客户端只需一套解析逻辑。
func NoContent(c *gin.Context) {
	c.JSON(http.StatusOK, Body{
		Model:     "empty",
		RequestID: contextx.RequestID(c),
	})
}

// Fail 直接写出错误响应，不中断后续中间件
func Fail(c *gin.Context, apiErr apperr.APIError) {
	c.JSON(apiErr.Status, buildErrorBody(c, apiErr))
}

// Abort 写出错误响应并中断后续处理
func Abort(c *gin.Context, apiErr apperr.APIError) {
	c.AbortWithStatusJSON(apiErr.Status, buildErrorBody(c, apiErr))
}

// Render 将任意 error 渲染为错误响应，非 APIError 统一降级为服务内部异常
func Render(c *gin.Context, err error) {
	Abort(c, Resolve(err))
}

// Resolve 将任意 error 归一化为 APIError
func Resolve(err error) apperr.APIError {
	if err == nil {
		return apperr.ErrInternal
	}
	var apiErr apperr.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return apperr.ErrInternal.WithMessage(err.Error())
}

func buildErrorBody(c *gin.Context, apiErr apperr.APIError) model.SystemErrorResult {
	body := model.NewSystemErrorResult([]model.SystemErrorItemResult{
		model.NewSystemErrorItemResult(apiErr.Code, apiErr.Localize(contextx.Locale(c))),
	})
	body.RequestID = contextx.RequestID(c)
	return body
}
