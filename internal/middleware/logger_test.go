package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"golang-server-starter/internal/apperr"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// newBufferedLogger 返回一个把日志写进内存的 logger，便于断言级别与内容。
func newBufferedLogger() (*logrus.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	l := logrus.New()
	l.SetOutput(buf)
	l.SetFormatter(&logrus.TextFormatter{DisableColors: true, DisableTimestamp: true})
	l.SetLevel(logrus.DebugLevel)
	return l, buf
}

// newLoggingEngine 构造 Logger -> ErrorHandler 的最小链路，顺序与 Setup 保持一致。
func newLoggingEngine(l *logrus.Logger) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Logger(LoggerOptions{Logger: l}))
	r.Use(ErrorHandler(ErrorHandlerOptions{Logger: l}))
	return r
}

// TestAccessLogSurvivesBusinessPanic 业务错误以 panic 抛出，
// 访问日志必须仍然落盘（回归点：日志若写在 c.Next() 之后就会整段丢失）。
func TestAccessLogSurvivesBusinessPanic(t *testing.T) {
	l, buf := newBufferedLogger()
	r := newLoggingEngine(l)
	r.GET("/boom", func(c *gin.Context) { panic(apperr.ErrAccountNotFound) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("期望 404，实际 %d", w.Code)
	}
	out := buf.String()
	if !strings.Contains(out, "请求返回异常状态码") {
		t.Fatalf("panic 路径缺少访问日志，实际输出:\n%s", out)
	}
	if !strings.Contains(out, "path=/boom") {
		t.Fatalf("访问日志缺少 path 字段，实际输出:\n%s", out)
	}
	if !strings.Contains(out, "status=404") {
		t.Fatalf("访问日志缺少 status 字段，实际输出:\n%s", out)
	}
}

// TestAccessLogLevelByStatus 校验状态码到日志级别的映射：
// 4xx -> warn，5xx -> error，2xx -> info。
func TestAccessLogLevelByStatus(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		wantLevel string
		wantMsg   string
	}{
		{name: "200 走 info", status: http.StatusOK, wantLevel: "level=info", wantMsg: "请求完成"},
		{name: "404 走 warn", status: http.StatusNotFound, wantLevel: "level=warning", wantMsg: "请求返回异常状态码"},
		{name: "500 走 error", status: http.StatusInternalServerError, wantLevel: "level=error", wantMsg: "请求处理失败"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, buf := newBufferedLogger()
			r := newLoggingEngine(l)
			status := tc.status
			r.GET("/x", func(c *gin.Context) {
				if status >= http.StatusInternalServerError {
					panic("boom")
				}
				c.Status(status)
			})

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

			out := buf.String()
			if !strings.Contains(out, tc.wantMsg) {
				t.Fatalf("期望访问日志含 %q，实际输出:\n%s", tc.wantMsg, out)
			}
			if !strings.Contains(out, tc.wantLevel) {
				t.Fatalf("期望日志级别 %q，实际输出:\n%s", tc.wantLevel, out)
			}
		})
	}
}

// TestAccessLogSkipsHealthz 探活路径不产生访问日志，避免噪声。
func TestAccessLogSkipsHealthz(t *testing.T) {
	l, buf := newBufferedLogger()
	r := newLoggingEngine(l)
	r.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if out := buf.String(); strings.Contains(out, "请求完成") {
		t.Fatalf("/healthz 不应产生访问日志，实际输出:\n%s", out)
	}
}

// TestAccessLogBodyCapture 开启 LogBody 时能记录请求体与响应体。
func TestAccessLogBodyCapture(t *testing.T) {
	l, buf := newBufferedLogger()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Logger(LoggerOptions{Logger: l, LogBody: true}))
	r.POST("/echo", func(c *gin.Context) { c.String(http.StatusOK, "hello-response") })

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("hello-request"))
	req.Header.Set("Content-Type", "text/plain")
	r.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if !strings.Contains(out, "hello-request") {
		t.Fatalf("缺少请求体，实际输出:\n%s", out)
	}
	if !strings.Contains(out, "hello-response") {
		t.Fatalf("缺少响应体，实际输出:\n%s", out)
	}
}

// TestAccessLogCapturesStringRenderedBody 回归：c.String 的响应体必须被捕获。
//
// gin.ResponseWriter 实现了 io.StringWriter，而 gin 的 responseWriter.WriteString
// 直接 io.WriteString 到底层 writer、不经过 Write；c.String 正是走这条路。
// 若 bodyCaptureWriter 只实现 Write，响应体日志会整段丢失。
func TestAccessLogCapturesStringRenderedBody(t *testing.T) {
	cases := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{"c.String", func(c *gin.Context) { c.String(http.StatusOK, "string-body-marker") }},
		{"c.JSON", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"note": "json-body-marker"}) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, buf := newBufferedLogger()
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(Logger(LoggerOptions{Logger: l, LogBody: true}))
			r.GET("/x", tc.handler)

			r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

			out := buf.String()
			if !strings.Contains(out, "body-marker") {
				t.Fatalf("响应体未被捕获，实际输出:\n%s", out)
			}
		})
	}
}

// TestAccessLogHeaderCapture 开启 LogHeader 时记录请求头与响应头。
func TestAccessLogHeaderCapture(t *testing.T) {
	l, buf := newBufferedLogger()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Logger(LoggerOptions{Logger: l, LogHeader: true}))
	r.GET("/h", func(c *gin.Context) {
		c.Header("X-Custom-Resp", "resp-value")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/h", nil)
	req.Header.Set("X-Custom-Req", "req-value")
	r.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if !strings.Contains(out, "request_headers") || !strings.Contains(out, "req-value") {
		t.Fatalf("请求头未记录，实际输出:\n%s", out)
	}
	if !strings.Contains(out, "response_headers") || !strings.Contains(out, "resp-value") {
		t.Fatalf("响应头未记录，实际输出:\n%s", out)
	}
}

// TestAccessLogMasksSensitiveContent 敏感内容（凭据头 + 请求/响应体字段）必须打码。
func TestAccessLogMasksSensitiveContent(t *testing.T) {
	l, buf := newBufferedLogger()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Logger(LoggerOptions{Logger: l, LogBody: true, LogHeader: true}))
	r.POST("/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"access_token": "response-token-leak",
			"username":     "alice",
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(
		`{"username":"alice","password":"request-password-leak","nested":{"refresh_token":"nested-leak"}}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Auth", "Bearer header-auth-leak")
	req.Header.Set("Cookie", "sid=cookie-leak")
	r.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	for _, leak := range []string{
		"request-password-leak", "nested-leak",
		"header-auth-leak", "cookie-leak", "response-token-leak",
	} {
		if strings.Contains(out, leak) {
			t.Fatalf("敏感值 %q 泄漏到日志:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "alice") {
		t.Fatalf("非敏感字段不应被抹掉，实际输出:\n%s", out)
	}
	if !strings.Contains(out, "****") {
		t.Fatalf("应出现脱敏占位，实际输出:\n%s", out)
	}
}

// TestAccessLogMasksSensitiveFormBody 表单请求体的敏感字段也要打码。
func TestAccessLogMasksSensitiveFormBody(t *testing.T) {
	l, buf := newBufferedLogger()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Logger(LoggerOptions{Logger: l, LogBody: true}))
	r.POST("/form", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/form",
		strings.NewReader("username=alice&password=p%40ssw0rd-leak"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if strings.Contains(out, "ssw0rd-leak") {
		t.Fatalf("表单密码泄漏到日志:\n%s", out)
	}
	if !strings.Contains(out, "username=alice") {
		t.Fatalf("非敏感表单字段应保留:\n%s", out)
	}
}

// TestBuildSensitiveSet 脱敏名单大小写不敏感，且忽略 - 与 _ 差异。
func TestBuildSensitiveSet(t *testing.T) {
	sensitive := buildSensitiveSet([]string{"id_card", "Phone"})

	cases := []struct {
		key  string
		want bool
	}{
		{"password", true},
		{"Password", true},
		{"NEW_PASSWORD", true},
		{"new-password", true},
		{"access_token", true},
		{"Access-Token", true},
		{"Auth", true},
		{"Authorization", true},
		{"Cookie", true},
		{"Set-Cookie", true},
		{"id_card", true},
		{"IdCard", true},
		{"phone", true},
		{"username", false},
		{"email", false},
		{"nickname", false},
	}

	for _, tc := range cases {
		if got := isSensitive(tc.key, sensitive); got != tc.want {
			t.Errorf("isSensitive(%q) = %v，期望 %v", tc.key, got, tc.want)
		}
	}
}

// TestMaskJSON 递归打码，非法 JSON 原样返回。
func TestMaskJSON(t *testing.T) {
	sensitive := buildSensitiveSet(nil)

	raw := `{"username":"alice","password":"pw","nested":{"refresh_token":"rt","keep":"kv"},"list":[{"secret":"sc"},"plain"]}`
	got := maskJSON(raw, sensitive)

	for _, want := range []string{`"password":"****"`, `"refresh_token":"****"`, `"secret":"****"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("缺少脱敏结果 %s，实际: %s", want, got)
		}
	}
	for _, keep := range []string{"alice", `"keep":"kv"`, "plain"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("非敏感内容丢失 %q，实际: %s", keep, got)
		}
	}

	// 非法 JSON / 被截断的 JSON 必须原样返回，不能因为脱敏把日志内容弄丢
	for _, broken := range []string{`{"a":`, `not json at all`, `{"a":1`} {
		if got := maskJSON(broken, sensitive); got != broken {
			t.Fatalf("非法 JSON 应原样返回，输入 %q 得到 %q", broken, got)
		}
	}
}

// TestReadRequestBodyKeepsBodyIntact 日志读取不能吃掉请求体。
// 无论是否超出缓冲上限，handler 都必须能读到完整原文。
func TestReadRequestBodyKeepsBodyIntact(t *testing.T) {
	cases := []struct {
		name string
		size int
	}{
		{"小请求体", 128},
		{"恰好缓冲上限", maxBufferedBody},
		{"超过缓冲上限", maxBufferedBody + 4096},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := bytes.Repeat([]byte("a"), tc.size)

			l, _ := newBufferedLogger()
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(Logger(LoggerOptions{Logger: l, LogBody: true}))

			var got int
			r.POST("/upload", func(c *gin.Context) {
				raw, err := io.ReadAll(c.Request.Body)
				if err != nil {
					t.Errorf("handler 读取请求体失败: %v", err)
				}
				got = len(raw)
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/octet-stream")
			r.ServeHTTP(httptest.NewRecorder(), req)

			if got != tc.size {
				t.Fatalf("handler 读到的请求体长度 = %d，期望 %d", got, tc.size)
			}
		})
	}
}

// TestAccessLogOversizedBodyNotBuffered 超过缓冲上限的请求体不落盘，只留一条提示。
func TestAccessLogOversizedBodyNotBuffered(t *testing.T) {
	marker := strings.Repeat("z", 512)
	payload := marker + strings.Repeat("y", maxBufferedBody)

	l, buf := newBufferedLogger()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Logger(LoggerOptions{Logger: l, LogBody: true}))
	r.POST("/upload", func(c *gin.Context) {
		if _, err := io.Copy(io.Discard, c.Request.Body); err != nil {
			t.Errorf("handler 读取请求体失败: %v", err)
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/octet-stream")
	r.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if !strings.Contains(out, "未记录") {
		t.Fatalf("超大请求体应记录提示信息，实际输出:\n%s", out)
	}
	if strings.Contains(out, strings.Repeat("y", 256)) {
		t.Fatalf("超大请求体不应落盘，实际输出:\n%s", out)
	}
}

// TestTruncateBodyKeepsValidUTF8 截断点落在多字节字符中间时不能产生乱码。
func TestTruncateBodyKeepsValidUTF8(t *testing.T) {
	raw := []byte("中文中文中文") // 每个汉字 3 字节，共 18 字节

	got := truncateBody(raw, 7) // 落在第二个「中」的中间
	if !utf8.ValidString(got) {
		t.Fatalf("截断结果不是合法 UTF-8: %q", got)
	}
	if !strings.HasPrefix(got, "中文") {
		t.Fatalf("截断结果丢失有效前缀: %q", got)
	}
	if !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("截断结果缺少标记: %q", got)
	}

	if got := truncateBody(raw, 0); got != string(raw) {
		t.Fatalf("limit<=0 时不应截断，实际: %q", got)
	}
	if got := truncateBody(raw, 64); got != string(raw) {
		t.Fatalf("未超限时不应截断，实际: %q", got)
	}
}

// TestMaskBodyByContentType 只对可识别的类型做脱敏，其他类型原样返回。
func TestMaskBodyByContentType(t *testing.T) {
	sensitive := buildSensitiveSet(nil)

	if got := maskBody(`{"password":"pw"}`, "application/json; charset=utf-8", sensitive); strings.Contains(got, "pw") {
		t.Fatalf("JSON 应脱敏，实际: %s", got)
	}
	if got := maskBody("password=pw", "application/x-www-form-urlencoded", sensitive); strings.Contains(got, "=pw") {
		t.Fatalf("表单应脱敏，实际: %s", got)
	}
	if got := maskBody("<password>pw</password>", "application/xml", sensitive); got != "<password>pw</password>" {
		t.Fatalf("未知类型应原样返回，实际: %s", got)
	}
	if got := maskBody("", "application/json", sensitive); got != "" {
		t.Fatalf("空体应返回空串，实际: %q", got)
	}
}
