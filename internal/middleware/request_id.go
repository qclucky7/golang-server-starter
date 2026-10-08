package middleware

import (
	"gin-quick-start/internal/constant"
	"gin-quick-start/internal/pkg/contextx"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestID 为每个请求分配唯一 ID：
//   - 优先沿用调用方传入的 X-Request-Id，便于全链路追踪
//   - 写入上下文，并在响应头回写
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(constant.HeaderRequestID)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		contextx.SetRequestID(c, requestID)
		c.Writer.Header().Set(constant.HeaderRequestID, requestID)
		c.Next()
	}
}
