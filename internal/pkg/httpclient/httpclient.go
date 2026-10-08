// Package httpclient 封装「本服务作为客户端」发往外部服务的 HTTP 调用（基于 resty）。
//
// 与 internal/middleware 的访问日志分工不同：访问日志记录入站请求，本包记录出站请求。
// 出站日志默认按 debug 级别输出，且：
//   - 请求/响应头中的凭据（Authorization / Cookie / X-Api-Key 等）一律打码；
//   - 请求体与响应体按长度截断，避免大报文淹没日志或泄漏密钥。
//
// 用法：
//
//	client := httpclient.New("payment", logger)
//	resp, err := client.R().SetBody(req).Post("https://api.example.com/pay")
package httpclient

import (
	"net/http"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

const (
	// defaultBodyLimit 请求/响应体日志的默认截断长度（字节）
	defaultBodyLimit = 4096
	// maskedValue 脱敏后的占位值
	maskedValue = "****"
)

// sensitiveHeaders 需要打码的请求/响应头，匹配大小写不敏感。
//
// 这些头一旦原文落盘即等于凭据泄漏（日志常被集中采集、长期留存），
// 因此默认全部打码，不提供运行时开关。
var sensitiveHeaders = []string{
	"authorization", "cookie", "set-cookie", "proxy-authorization", "x-api-key",
}

// Client 出站 HTTP 客户端，内嵌 resty.Client 以保留其全部链式 API。
type Client struct {
	*resty.Client

	name      string
	log       *logrus.Logger
	bodyLimit int
}

// Option 客户端选项
type Option func(*Client)

// WithBodyLimit 设置请求/响应体日志的截断长度，limit <= 0 时忽略。
func WithBodyLimit(limit int) Option {
	return func(c *Client) {
		if limit > 0 {
			c.bodyLimit = limit
		}
	}
}

// New 创建一个带日志的出站客户端。
//
// name 仅用于日志区分不同下游（如 "payment" / "sms"），不参与请求。
// log 为 nil 时回落到 logrus.StandardLogger()。
func New(name string, log *logrus.Logger, opts ...Option) *Client {
	if log == nil {
		log = logrus.StandardLogger()
	}

	c := &Client{
		Client:    resty.New(),
		name:      name,
		log:       log,
		bodyLimit: defaultBodyLimit,
	}
	for _, opt := range opts {
		opt(c)
	}

	c.OnBeforeRequest(func(_ *resty.Client, r *resty.Request) error {
		c.log.WithFields(logrus.Fields{
			"client":  c.name,
			"method":  r.Method,
			"url":     r.URL,
			"query":   r.QueryParam.Encode(),
			"headers": maskHeaders(r.Header),
			"body":    truncate(marshalBody(c.Client, r.Body), c.bodyLimit),
		}).Debug("出站请求")
		return nil
	})

	c.OnAfterResponse(func(_ *resty.Client, r *resty.Response) error {
		entry := c.log.WithFields(logrus.Fields{
			"client":     c.name,
			"method":     r.Request.Method,
			"url":        r.Request.URL,
			"status":     r.StatusCode(),
			"latency_ms": r.Time().Milliseconds(),
			"headers":    maskHeaders(r.Header()),
			"body":       truncate(r.String(), c.bodyLimit),
		})
		if r.IsError() {
			entry.Warn("出站响应异常")
			return nil
		}
		entry.Debug("出站响应")
		return nil
	})

	return c
}

// Name 返回客户端名称
func (c *Client) Name() string { return c.name }

// marshalBody 尽力把请求体序列化成字符串用于日志；失败时返回占位说明。
func marshalBody(client *resty.Client, body any) string {
	if body == nil {
		return ""
	}
	if raw, ok := body.(string); ok {
		return raw
	}
	if raw, ok := body.([]byte); ok {
		return string(raw)
	}
	data, err := client.JSONMarshal(body)
	if err != nil {
		return "(请求体无法序列化)"
	}
	return string(data)
}

// maskHeaders 把 http.Header 摊平成单值 map，敏感头打码。
func maskHeaders(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	out := make(map[string]string, len(header))
	for name, values := range header {
		joined := strings.Join(values, ", ")
		if isSensitiveHeader(name) {
			joined = maskedValue
		}
		out[name] = joined
	}
	return out
}

// isSensitiveHeader 判断请求头是否需要打码，忽略大小写与 -/_ 差异。
func isSensitiveHeader(name string) bool {
	normalized := strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "-", ""), "_", "")
	for _, key := range sensitiveHeaders {
		if strings.ReplaceAll(key, "-", "") == normalized {
			return true
		}
	}
	return false
}

// truncate 按字节截断并保持 UTF-8 合法，避免日志里出现半个汉字。
func truncate(raw string, limit int) string {
	if limit <= 0 || len(raw) <= limit {
		return raw
	}
	return strings.ToValidUTF8(raw[:limit], "") + "...(truncated)"
}
