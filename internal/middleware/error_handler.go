package middleware

import (
	"errors"
	"net/http"

	"gin-quick-start/internal/apperr"
	"gin-quick-start/internal/pkg/contextx"
	"gin-quick-start/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// ErrorHandlerOptions 错误处理中间件参数
type ErrorHandlerOptions struct {
	Logger *logrus.Logger
	// Debug 为 true 时会把原始错误信息透出到响应，便于本地排查
	Debug bool
}

// ErrorHandler 统一错误处理：
//  1. recover 捕获 panic（业务代码 panic(apperr.XXX) 即由此渲染）
//  2. 兜底处理通过 c.Error(err) 注入的错误
//  3. 5xx 记 error 级日志，4xx 记 warn 级日志
func ErrorHandler(opt ErrorHandlerOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			apiErr := normalize(rec, opt.Debug)
			logRecover(c, opt.Logger, apiErr, rec)

			if c.Writer.Written() {
				// 响应已开始写出，无法再改状态码，只能中断后续处理
				c.Abort()
				return
			}
			response.Abort(c, apiErr)
		}()

		c.Next()

		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		response.Abort(c, response.Resolve(c.Errors.Last().Err))
	}
}

// normalize 把任意 panic 值归一化为 APIError，未知错误不向外泄漏内部细节
func normalize(rec any, debug bool) apperr.APIError {
	switch value := rec.(type) {
	case apperr.APIError:
		return value
	case *apperr.APIError:
		if value != nil {
			return *value
		}
	case error:
		var apiErr apperr.APIError
		if errors.As(value, &apiErr) {
			return apiErr
		}
		if debug {
			return apperr.ErrInternal.WithMessage(apperr.ErrInternal.Message + ": " + value.Error())
		}
	default:
		if debug {
			return apperr.ErrInternal.WithMessage(apperr.ErrInternal.Message + ": " + toMessage(rec))
		}
	}
	return apperr.ErrInternal
}

func toMessage(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return "unknown panic"
}

func logRecover(c *gin.Context, logger *logrus.Logger, apiErr apperr.APIError, rec any) {
	entry := contextx.Logger(c)
	if logger != nil {
		entry = logger.WithField("request_id", contextx.RequestID(c))
	}
	fields := logrus.Fields{
		"method":     c.Request.Method,
		"path":       c.Request.URL.Path,
		"error_code": apiErr.Code,
		"panic":      rec,
	}
	if apiErr.Status >= http.StatusInternalServerError {
		entry.WithFields(fields).Error("请求处理发生异常")
		return
	}
	entry.WithFields(fields).Warn("请求被拒绝")
}
