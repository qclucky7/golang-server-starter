package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"gin-quick-start/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// newRateLimitEngine 构造只挂 ErrorHandler + RateLimit 的最小引擎。
// ErrorHandler 必须在外层，才能接住 RateLimit panic 出来的 429。
func newRateLimitEngine(cfg config.RateLimit) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorHandler(ErrorHandlerOptions{Logger: logrus.New()}))
	r.Use(RateLimit(cfg))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	return r
}

// doRequest 发起一次请求，返回状态码与 X-RateLimit-Remaining。
//
// 注意：中间件为保留大小写是直接写 Header map 的，
// Go 的 Header.Get 会做规范化查找因而取不到，这里必须按原始 key 读取。
func doRequest(r *gin.Engine) (int, string) {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	remaining := ""
	if values := w.Header()["X-RateLimit-Remaining"]; len(values) > 0 {
		remaining = values[0]
	}
	return w.Code, remaining
}

// TestRateLimitAllowsThenRejects 验证令牌桶放行 burst 次后开始 429。
func TestRateLimitAllowsThenRejects(t *testing.T) {
	const burst = 3
	r := newRateLimitEngine(config.RateLimit{Enabled: true, RPS: 1, Burst: burst})

	for i := 0; i < burst+3; i++ {
		code, _ := doRequest(r)
		want := http.StatusOK
		if i >= burst {
			want = http.StatusTooManyRequests
		}
		if code != want {
			t.Fatalf("第 %d 次请求：期望 %d，实际 %d", i+1, want, code)
		}
	}
}

// TestRateLimitRemainingHeader 验证 X-RateLimit-Remaining 反映的是「消费后」的余额。
//
// 回归点：Allow() 与 Tokens() 的顺序写反的话，响应头会永远多报 1。
func TestRateLimitRemainingHeader(t *testing.T) {
	const burst = 3
	r := newRateLimitEngine(config.RateLimit{Enabled: true, RPS: 1, Burst: burst})

	for i := 0; i < burst; i++ {
		_, remaining := doRequest(r)
		want := strconv.Itoa(burst - 1 - i)
		if remaining != want {
			t.Fatalf("第 %d 次请求：期望剩余 %s，实际 %s", i+1, want, remaining)
		}
	}
}

// TestRateLimitConcurrent 并发打同一 IP。
// 该用例主要用于配合 -race 检出 rate.Limiter.Tokens() 的数据竞争。
func TestRateLimitConcurrent(t *testing.T) {
	r := newRateLimitEngine(config.RateLimit{Enabled: true, RPS: 100, Burst: 50})

	var wg sync.WaitGroup
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			doRequest(r)
		}()
	}
	wg.Wait()
}

// TestRateLimitDefaultsWhenUnset 未配置 rps/burst 时回落到默认值，不应 panic。
func TestRateLimitDefaultsWhenUnset(t *testing.T) {
	r := newRateLimitEngine(config.RateLimit{Enabled: true})
	if code, _ := doRequest(r); code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", code)
	}
}
