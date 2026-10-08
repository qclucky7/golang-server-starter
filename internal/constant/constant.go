// Package constant 集中定义请求头、上下文键等常量，避免散落在各文件中。
package constant

// 标准请求/响应头
const (
	// HeaderRequestID 请求追踪 ID
	HeaderRequestID = "X-Request-Id"
	// HeaderAuthorization 标准鉴权头，本项目只认这一种，格式 Authorization: Bearer <token>
	HeaderAuthorization = "Authorization"
	// HeaderRealIP 反向代理下真实客户端 IP
	HeaderRealIP = "X-Real-IP"
	// HeaderForwardedFor 反向代理转发链
	HeaderForwardedFor = "X-Forwarded-For"
	// HeaderUserAgent 客户端标识
	HeaderUserAgent = "User-Agent"
	// HeaderContentType 报文内容类型
	HeaderContentType = "Content-Type"
)

// 业务常量
const (
	// DefaultPageSize 默认每页条数
	DefaultPageSize = 10
	// MaxPageSize 每页条数上限
	MaxPageSize = 200
)
