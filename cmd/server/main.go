// Package main 服务入口。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "gin-quick-start/docs" // swag 生成的接口文档
	"gin-quick-start/internal/bootstrap"
	"gin-quick-start/internal/config"
	"gin-quick-start/internal/pkg/logger"
)

// @title						Gin Quick Start API
// @version					1.0.0
// @description				基于 Gin + GORM 的 Go 服务端模板：统一响应体、统一错误处理、JWT 鉴权、泛型 CRUD 仓储、Swagger 文档。
// @description
// @description				鉴权方式：请求头 `Authorization: Bearer <token>`，不兼容其他写法。
// @description				主键为雪花算法生成的 19 位整数，JSON 中以字符串返回（避免 JS 精度丢失）。
// @BasePath					/
//
// @securityDefinitions.apikey	Authorization
// @in							header
// @name						Authorization
// @description				标准鉴权头，格式：Bearer <access_token>
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "服务启动失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath string
		env        string
	)
	flag.StringVar(&configPath, "c", "", "基线配置文件路径（默认 "+config.DefaultPath+"）")
	flag.StringVar(&configPath, "config", "", "同 -c")
	flag.StringVar(&env, "e", "", "运行环境，加载 configs/config-{env}.yaml（默认取 APP_ENV，再兜底 dev）")
	flag.StringVar(&env, "env", "", "同 -e")
	flag.Parse()

	cfg, err := config.Load(configPath, env)
	if err != nil {
		return err
	}

	container, err := bootstrap.New(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := container.Close(); err != nil {
			container.Logger.WithError(err).Error("关闭数据库连接失败")
		}
		// 日志 hook 持有文件句柄，必须最后关闭；此时已不能再写日志。
		if err := logger.Close(container.Logger); err != nil {
			fmt.Fprintf(os.Stderr, "关闭日志文件失败: %v\n", err)
		}
	}()

	server := bootstrap.NewServer(cfg, container.Logger, container.Engine)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		container.Logger.Info("收到退出信号，开始优雅关停")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("优雅关停超时: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	container.Logger.Info("服务已退出")
	return nil
}
