// Package middleware 全局与路由级中间件。
package middleware

import (
	"golang-server-starter/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Options 中间件装配参数
type Options struct {
	Config *config.Config
	Logger *logrus.Logger
}

// Setup 装配全局中间件。
//
// 注册顺序 = 执行顺序的外层到内层，这个顺序不能随意调整：
//
//	RequestID   最外层：错误响应与全部日志都需要 request_id
//	Locale      次外层：错误文案、校验提示都要按请求语言渲染，必须早于它们执行
//	Logger      必须在 ErrorHandler 之外，否则业务错误 panic 展开时
//	            它的后置日志代码不会执行，且此时状态码尚未写出
//	ErrorHandler 兜住内层中间件与 handler 的 panic
//	CORS / RateLimit  需要在被兜住的范围内，保证它们的拒绝也能被正确渲染
//
// 业务错误统一用 panic(apperr.XXX) 抛出，因此 Logger 必须在 ErrorHandler 外层，
// 这样 panic 被 recover 并写出响应后，Logger 才能拿到真实状态码落访问日志。
func Setup(r *gin.Engine, opt Options) {
	cfg := opt.Config

	r.Use(RequestID())

	if cfg.I18N.Enabled {
		r.Use(Locale(cfg.I18N))
	}

	r.Use(Logger(LoggerOptions{
		Logger:        opt.Logger,
		LogBody:       cfg.Log.LogBody,
		LogHeader:     cfg.Log.LogHeader,
		BodyLimit:     cfg.Log.BodyLimit,
		SensitiveKeys: cfg.Log.SensitiveKeys,
	}))

	r.Use(ErrorHandler(ErrorHandlerOptions{
		Logger: opt.Logger,
		Debug:  cfg.App.IsDev(),
	}))

	if cfg.CORS.Enabled {
		r.Use(CORS(cfg.CORS))
	}
	if cfg.RateLimit.Enabled {
		r.Use(RateLimit(cfg.RateLimit))
	}
}
