package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"gin-quick-start/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Server HTTP 服务封装，负责启动与优雅退出
type Server struct {
	http     *http.Server
	logger   *logrus.Logger
	listener net.Listener
}

// NewServer 创建 HTTP 服务
func NewServer(cfg *config.Config, logger *logrus.Logger, handler *gin.Engine) *Server {
	return &Server{
		http: &http.Server{
			Addr:         cfg.Server.Addr(),
			Handler:      handler,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
			IdleTimeout:  cfg.Server.IdleTimeout,
		},
		logger: logger,
	}
}

// Listen 预先绑定监听端口。
//
// 提前绑定有两个好处：
//  1. 端口被占用时立即返回错误，不会先打出「服务已启动」的假日志；
//  2. 配置 port=0 时可拿到系统分配的真实端口（供测试使用）。
func (s *Server) Listen() error {
	if s.listener != nil {
		return nil
	}
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", s.http.Addr, err)
	}
	s.listener = ln
	return nil
}

// Addr 返回实际监听地址；未调用 Listen 时返回配置地址。
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.http.Addr
}

// logAddr 返回用于日志展示的地址。
// 配置了固定端口时用配置值——否则 Go 会把 0.0.0.0 解析成双栈 [::]，日志里看着莫名其妙；
// port=0 时才回退到系统实际分配的地址。
func (s *Server) logAddr() string {
	if _, port, err := net.SplitHostPort(s.http.Addr); err == nil && port != "0" {
		return s.http.Addr
	}
	return s.Addr()
}

// Start 阻塞式启动服务，返回 nil 表示已正常关闭。
// 未提前调用 Listen 时会自行绑定。
func (s *Server) Start() error {
	if err := s.Listen(); err != nil {
		return err
	}
	s.logger.WithField("addr", s.logAddr()).Info("HTTP 服务已启动")
	if err := s.http.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown 优雅退出：停止接收新请求并等待在途请求完成
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
