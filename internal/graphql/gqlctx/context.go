// Package gqlctx 把 HTTP 请求级信息搬运进 GraphQL resolver 的 context。
//
// 为什么需要这个包：
// gin 侧的值存在 *gin.Context 的 Keys 里（见 internal/pkg/contextx），
// 而 gqlgen 的 resolver 只拿得到标准 context.Context。两者不共享存储，
// 所以由 HTTP 边界（api/v1/graphql.go）显式搬运一次。
//
// 与 contextx 一样，业务层（service / repository）不应 import 本包。
package gqlctx

import (
	"context"

	"gin-quick-start/internal/model"

	"github.com/sirupsen/logrus"
)

// Meta GraphQL 请求级元信息
type Meta struct {
	// Account 当前登录账号，未登录为 nil（端点用可选鉴权，匿名请求也放行）
	Account *model.Account
	// RequestID 与响应头 X-Request-Id 一致
	RequestID string
	// Locale 请求语言，用于错误文案本地化
	Locale string
	// Logger 带请求上下文的日志实例
	Logger *logrus.Entry
}

type contextKey struct{}

// WithMeta 把元信息注入 context
func WithMeta(ctx context.Context, meta Meta) context.Context {
	return context.WithValue(ctx, contextKey{}, meta)
}

// From 读取元信息，缺失时返回零值
func From(ctx context.Context) Meta {
	meta, _ := ctx.Value(contextKey{}).(Meta)
	return meta
}

// Account 当前登录账号，未登录返回 nil
func Account(ctx context.Context) *model.Account { return From(ctx).Account }

// RequestID 请求 ID
func RequestID(ctx context.Context) string { return From(ctx).RequestID }

// Locale 请求语言
func Locale(ctx context.Context) string { return From(ctx).Locale }

// Logger 请求级日志实例，缺失时回落到标准 logger
func Logger(ctx context.Context) *logrus.Entry {
	if entry := From(ctx).Logger; entry != nil {
		return entry
	}
	return logrus.NewEntry(logrus.StandardLogger())
}
