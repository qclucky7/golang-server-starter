package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang-server-starter/internal/constant"
	"golang-server-starter/internal/pkg/contextx"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// defaultBodyLimit 请求/响应体日志的默认截断长度
const defaultBodyLimit = 4096

// maskedValue 脱敏后的占位值
const maskedValue = "****"

// defaultSensitiveKeys 内置脱敏名单，同时用于请求头名与请求/响应体字段名，
// 匹配时大小写不敏感、忽略 - 与 _ 差异。
//
// 这些字段一旦原文落盘就等于凭据泄漏（日志常被集中采集、长期留存、权限宽松），
// 所以默认全部打码，不提供运行时开关；确实需要明文时直接改这个名单。
var defaultSensitiveKeys = []string{
	// 凭据类请求/响应头
	"auth", "authorization", "cookie", "set-cookie", "proxy-authorization", "x-api-key",
	// 凭据类字段
	"password", "oldpassword", "newpassword", "confirmpassword",
	"token", "accesstoken", "refreshtoken", "secret", "privatekey", "apikey",
}

// LoggerOptions 访问日志中间件参数
type LoggerOptions struct {
	Logger *logrus.Logger
	// LogBody 记录请求体与响应体（按 BodyLimit 截断，敏感字段自动脱敏）
	LogBody bool
	// LogHeader 记录请求头与响应头（敏感头自动脱敏）
	LogHeader bool
	// BodyLimit 请求/响应体截断长度，0 表示使用 defaultBodyLimit
	BodyLimit int
	// SensitiveKeys 在内置名单基础上追加的脱敏字段名
	SensitiveKeys []string
}

// skipLogPaths 无需记录访问日志的路径前缀
var skipLogPaths = []string{"/swagger", "/favicon.ico", "/healthz"}

// Logger 访问日志中间件：输出请求 ID、客户端 IP、耗时、状态码等结构化字段。
//
// 注意：必须注册在 ErrorHandler 之外（见 Setup 的说明）。业务错误通过 panic 抛出，
// panic 会沿 c.Next() 向上展开，因此这里的日志写在 defer 中，保证异常路径也能落盘。
//
// LogBody / LogHeader 为 true 时额外记录请求体、响应体、请求头、响应头。
// 敏感内容（密码、令牌、Auth / Cookie 头等）会自动替换为 ****，见 defaultSensitiveKeys。
func Logger(opt LoggerOptions) gin.HandlerFunc {
	sensitive := buildSensitiveSet(opt.SensitiveKeys)
	limit := opt.BodyLimit
	if limit <= 0 {
		limit = defaultBodyLimit
	}

	return func(c *gin.Context) {
		if shouldSkipLog(c.Request.URL.Path) {
			c.Next()
			return
		}

		start := time.Now()
		entry := opt.Logger.WithFields(logrus.Fields{
			"request_id": contextx.RequestID(c),
			"client_ip":  c.ClientIP(),
		})
		contextx.SetLogger(c, entry)

		var requestBody string
		var responseWriter *bodyCaptureWriter
		if opt.LogBody {
			requestBody = readRequestBody(c, limit)
			responseWriter = newBodyCaptureWriter(c.Writer, limit)
			c.Writer = responseWriter
		}

		defer func() {
			status := c.Writer.Status()
			fields := logrus.Fields{
				"method":     c.Request.Method,
				"path":       c.Request.URL.Path,
				"query":      c.Request.URL.RawQuery,
				"status":     status,
				"latency_ms": time.Since(start).Milliseconds(),
				"user_agent": c.GetHeader(constant.HeaderUserAgent),
			}
			if account := contextx.Account(c); account != nil {
				fields["account_id"] = account.ID
			}
			if opt.LogHeader {
				fields["request_headers"] = flattenHeaders(c.Request.Header, sensitive)
				fields["response_headers"] = flattenHeaders(c.Writer.Header(), sensitive)
			}
			if opt.LogBody {
				fields["request_body"] = maskBody(requestBody, c.ContentType(), sensitive)
				fields["response_body"] = maskBody(
					responseWriter.Body(),
					c.Writer.Header().Get(constant.HeaderContentType),
					sensitive,
				)
			}

			switch {
			case status >= http.StatusInternalServerError:
				entry.WithFields(fields).Error("请求处理失败")
			case status >= http.StatusBadRequest:
				entry.WithFields(fields).Warn("请求返回异常状态码")
			default:
				entry.WithFields(fields).Info("请求完成")
			}
		}()

		c.Next()
	}
}

func shouldSkipLog(path string) bool {
	for _, prefix := range skipLogPaths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 脱敏
// ---------------------------------------------------------------------------

// normalizeKey 归一化字段名，使 AccessToken / access-token / access_token 命中同一条规则。
func normalizeKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, "_", "")
	return key
}

func buildSensitiveSet(extra []string) map[string]struct{} {
	set := make(map[string]struct{}, len(defaultSensitiveKeys)+len(extra))
	for _, key := range defaultSensitiveKeys {
		set[normalizeKey(key)] = struct{}{}
	}
	for _, key := range extra {
		if normalized := normalizeKey(key); normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return set
}

func isSensitive(key string, sensitive map[string]struct{}) bool {
	_, ok := sensitive[normalizeKey(key)]
	return ok
}

// flattenHeaders 把 http.Header 摊平成单值 map，敏感头打码。
// 摊平后 logrus 输出更紧凑；Go 的 fmt 会按 key 排序打印 map，日志顺序稳定。
func flattenHeaders(header http.Header, sensitive map[string]struct{}) map[string]string {
	if len(header) == 0 {
		return nil
	}
	out := make(map[string]string, len(header))
	for name, values := range header {
		joined := strings.Join(values, ", ")
		if isSensitive(name, sensitive) {
			joined = maskedValue
		}
		out[name] = joined
	}
	return out
}

// maskBody 按 Content-Type 对请求/响应体脱敏。无法识别的类型原样返回。
func maskBody(raw, contentType string, sensitive map[string]struct{}) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	switch {
	case strings.Contains(contentType, "json"):
		return maskJSON(raw, sensitive)
	case strings.Contains(contentType, "x-www-form-urlencoded"):
		return maskForm(raw, sensitive)
	default:
		return raw
	}
}

// maskJSON 解析 JSON 后递归打码敏感字段再序列化。
// 解析失败（截断、非法 JSON）时原样返回，保证日志不会因为脱敏而丢内容。
func maskJSON(raw string, sensitive map[string]struct{}) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
		return raw
	}

	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		return raw
	}
	maskValue(value, sensitive)

	masked, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return string(masked)
}

func maskValue(value any, sensitive map[string]struct{}) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if isSensitive(key, sensitive) {
				node[key] = maskedValue
				continue
			}
			maskValue(child, sensitive)
		}
	case []any:
		for _, child := range node {
			maskValue(child, sensitive)
		}
	}
}

// maskForm 对 application/x-www-form-urlencoded 文本打码敏感字段。
func maskForm(raw string, sensitive map[string]struct{}) string {
	parts := strings.Split(raw, "&")
	changed := false
	for i, part := range parts {
		key, _, found := strings.Cut(part, "=")
		if !found || !isSensitive(key, sensitive) {
			continue
		}
		parts[i] = key + "=" + maskedValue
		changed = true
	}
	if !changed {
		return raw
	}
	return strings.Join(parts, "&")
}

// ---------------------------------------------------------------------------
// 请求体读取与响应体捕获
// ---------------------------------------------------------------------------

// maxBufferedBody 请求体日志最多缓冲的字节数。
//
// 超过这个长度的请求体（典型场景是文件上传）不再整体读进内存，
// 只保留头部用于日志，剩余部分原样交回 handler，
// 否则「打开日志」就等于给服务端加了一个内存放大器。
const maxBufferedBody = 1 << 20 // 1MiB

// readRequestBody 读取请求体用于日志，并把 Body 完整还原给后续 handler。
//
// 不能用 io.LimitReader 直接替换 Body —— 那会把大请求体截断后交给业务代码。
// 这里的做法是最多缓冲 maxBufferedBody+1 字节：
//   - 未超出：把缓冲内容重新塞回 Body，等价于完整 ReadAll；
//   - 超出：把「已缓冲的头部 + 尚未读取的剩余」用 MultiReader 拼回 Body，日志只记头部。
func readRequestBody(c *gin.Context, limit int) string {
	if c.Request.Body == nil {
		return ""
	}

	head, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBufferedBody+1))
	if err != nil || len(head) > maxBufferedBody {
		// 读取失败或超大请求体：原样拼回 Body，不因日志影响业务
		c.Request.Body = &multiReadCloser{
			Reader: io.MultiReader(bytes.NewReader(head), c.Request.Body),
			Closer: c.Request.Body,
		}
		if err != nil {
			return ""
		}
		return "(请求体超过 " + strconv.Itoa(maxBufferedBody) + " 字节，未记录)"
	}

	c.Request.Body = io.NopCloser(bytes.NewReader(head))
	return truncateBody(head, limit)
}

// truncateBody 按字节截断并保持 UTF-8 合法，避免日志里出现半个汉字。
func truncateBody(raw []byte, limit int) string {
	if limit <= 0 || len(raw) <= limit {
		return string(raw)
	}
	return strings.ToValidUTF8(string(raw[:limit]), "") + "...(truncated)"
}

// multiReadCloser 组合「已缓冲头部 + 原始剩余」，并保留 Close 语义
type multiReadCloser struct {
	io.Reader
	io.Closer
}

// bodyCaptureWriter 复制响应体用于日志，同时保持原始写入行为
type bodyCaptureWriter struct {
	gin.ResponseWriter
	body  bytes.Buffer
	limit int
}

func newBodyCaptureWriter(inner gin.ResponseWriter, limit int) *bodyCaptureWriter {
	return &bodyCaptureWriter{ResponseWriter: inner, limit: limit}
}

func (w *bodyCaptureWriter) capture(data []byte) {
	if w.body.Len() >= w.limit {
		return
	}
	remaining := w.limit - w.body.Len()
	if remaining > len(data) {
		remaining = len(data)
	}
	w.body.Write(data[:remaining])
}

func (w *bodyCaptureWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

// WriteString 必须显式实现。
//
// gin.ResponseWriter 同时实现了 io.StringWriter，而 gin 的 responseWriter.WriteString
// 直接 io.WriteString 到底层 writer、**不经过 Write**；gin 的 render.String（c.String）
// 正是走这条路。若只实现 Write，所有 c.String(...) 的响应体都不会进日志。
func (w *bodyCaptureWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *bodyCaptureWriter) Body() string {
	if w.body.Len() == 0 {
		return ""
	}
	// 截断点可能落在多字节字符中间，ToValidUTF8 把残留的半个字符丢掉
	text := strings.ToValidUTF8(w.body.String(), "")
	if w.body.Len() >= w.limit {
		return text + "...(truncated)"
	}
	return text
}
