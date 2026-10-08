package bootstrap

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"golang-server-starter/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// newTestServer 构造一个监听随机端口（port=0）的测试服务。
func newTestServer(t *testing.T, register func(r *gin.Engine)) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0

	l := logrus.New()
	l.SetOutput(io.Discard)

	engine := gin.New()
	register(engine)

	srv := NewServer(cfg, l, engine)
	if err := srv.Listen(); err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	return srv
}

// waitReachable 轮询直到服务可访问，避免测试里的启动竞态。
func waitReachable(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("服务在 3s 内未就绪: %s", url)
}

// TestServerStartThenShutdown 验证正常启动、优雅关停、关停后拒绝连接。
func TestServerStartThenShutdown(t *testing.T) {
	srv := newTestServer(t, func(r *gin.Engine) {
		r.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })
	})

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	url := fmt.Sprintf("http://%s/healthz", srv.Addr())
	waitReachable(t, url)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("优雅关停失败: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start 应返回 nil，实际: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown 后 Start 未返回")
	}

	if resp, err := http.Get(url); err == nil {
		_ = resp.Body.Close()
		t.Fatal("关停后仍可访问服务")
	}
}

// TestShutdownWaitsForInflight 优雅关停必须等在途请求处理完，而不是直接掐断。
func TestShutdownWaitsForInflight(t *testing.T) {
	const handlerDelay = 300 * time.Millisecond

	srv := newTestServer(t, func(r *gin.Engine) {
		r.GET("/slow", func(c *gin.Context) {
			time.Sleep(handlerDelay)
			c.String(http.StatusOK, "done")
		})
	})

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	base := fmt.Sprintf("http://%s", srv.Addr())
	waitReachable(t, base+"/slow")

	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := http.Get(base + "/slow")
		if err != nil {
			done <- result{err: err}
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		done <- result{code: resp.StatusCode}
	}()

	// 等请求进入 handler 后再关停
	time.Sleep(handlerDelay / 3)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("优雅关停失败: %v", err)
	}

	got := <-done
	if got.err != nil {
		t.Fatalf("在途请求被中断: %v", got.err)
	}
	if got.code != http.StatusOK {
		t.Fatalf("在途请求期望 200，实际 %d", got.code)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Start 应返回 nil，实际: %v", err)
	}
}

// TestListenFailsFastOnOccupiedPort 端口占用时应立即报错，
// 不能先打「服务已启动」的假日志。
func TestListenFailsFastOnOccupiedPort(t *testing.T) {
	first := newTestServer(t, func(r *gin.Engine) {})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = first.Shutdown(ctx)
	}()

	_, portStr, err := net.SplitHostPort(first.Addr())
	if err != nil {
		t.Fatalf("解析监听地址失败: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("解析端口失败: %v", err)
	}

	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = port

	l := logrus.New()
	l.SetOutput(io.Discard)
	second := NewServer(cfg, l, gin.New())

	if err := second.Listen(); err == nil {
		t.Fatal("端口被占用时 Listen 应返回错误")
	}
}
