package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"gin-quick-start/internal/config"

	"github.com/gin-gonic/gin"
)

// CORS 跨域中间件。
//
// 注意：Access-Control-Allow-Origin: * 与 Allow-Credentials: true 不能同时生效，
// 因此当开启凭证时，回显请求方 Origin 而非使用通配符。
func CORS(cfg config.CORS) gin.HandlerFunc {
	allowAll := contains(cfg.AllowOrigins, "*")
	allowed := make(map[string]struct{}, len(cfg.AllowOrigins))
	for _, origin := range cfg.AllowOrigins {
		allowed[origin] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		switch {
		case cfg.AllowCredentials && allowAll:
			// 携带凭证时回显 Origin
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		case allowAll:
			c.Header("Access-Control-Allow-Origin", "*")
		default:
			if _, ok := allowed[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				if cfg.AllowCredentials {
					c.Header("Access-Control-Allow-Credentials", "true")
				}
			}
		}

		if len(cfg.ExposeHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ", "))
		}

		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
			c.Header("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
			if cfg.MaxAge > 0 {
				c.Header("Access-Control-Max-Age", strconv.Itoa(int(cfg.MaxAge.Seconds())))
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func contains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
