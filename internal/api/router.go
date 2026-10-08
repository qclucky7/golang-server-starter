// Package api 负责 HTTP 路由装配。
package api

import (
	"golang-server-starter/internal/api/v1"
	"golang-server-starter/internal/apperr"
	"golang-server-starter/internal/middleware"
	"golang-server-starter/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// Dependencies 路由层所需依赖。
//
// 就是 v1.Dependencies 的别名 —— 这样 bootstrap 只依赖 api 包，
// 不必为了拼装依赖而 import api/v1。
type Dependencies = v1.Dependencies

// NewRouter 构建 gin 引擎并注册全部路由
func NewRouter(deps Dependencies) *gin.Engine {
	gin.SetMode(ginMode(deps.Config.Server.Mode))

	engine := gin.New()
	engine.RedirectTrailingSlash = true
	engine.HandleMethodNotAllowed = true

	middleware.Setup(engine, middleware.Options{
		Config: deps.Config,
		Logger: deps.Logger,
	})

	registerFallbacks(engine)
	registerSwagger(engine)

	v1.Register(engine, deps)
	return engine
}

// registerFallbacks 统一 404 / 405 响应格式，避免返回 gin 默认的纯文本
func registerFallbacks(engine *gin.Engine) {
	engine.NoRoute(func(c *gin.Context) {
		response.Abort(c, apperr.ErrRouteNotFound)
	})
	engine.NoMethod(func(c *gin.Context) {
		response.Abort(c, apperr.ErrMethodNotAllowed)
	})
}

func ginMode(mode string) string {
	switch mode {
	case gin.ReleaseMode, gin.TestMode, gin.DebugMode:
		return mode
	default:
		return gin.DebugMode
	}
}
