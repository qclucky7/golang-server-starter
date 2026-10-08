package middleware

import (
	"strings"

	"golang-server-starter/internal/config"
	"golang-server-starter/internal/pkg/contextx"
	"golang-server-starter/internal/pkg/i18n"

	"github.com/gin-gonic/gin"
)

// Locale 语言协商中间件。
//
// 取值优先级：
//  1. 自定义请求头（config.i18n.alt_header，默认 X-Lang）—— 便于前端显式指定
//  2. 标准请求头（config.i18n.header，默认 Accept-Language）
//  3. 查询参数（config.i18n.query，默认 lang）
//
// 协商结果写入上下文供 response / request 渲染文案，同时通过 Content-Language
// 与 Vary 响应头告知客户端与中间缓存。
func Locale(cfg config.I18N) gin.HandlerFunc {
	return func(c *gin.Context) {
		locale := i18n.Match(pickLanguage(c, cfg))
		contextx.SetLocale(c, locale)

		c.Writer.Header().Set("Content-Language", locale)
		for _, name := range []string{cfg.AltHeader, cfg.Header} {
			if name = strings.TrimSpace(name); name != "" {
				c.Writer.Header().Add("Vary", name)
			}
		}
		c.Next()
	}
}

// pickLanguage 按优先级取出原始的语言声明
func pickLanguage(c *gin.Context, cfg config.I18N) string {
	if name := strings.TrimSpace(cfg.AltHeader); name != "" {
		if value := strings.TrimSpace(c.GetHeader(name)); value != "" {
			return value
		}
	}
	if name := strings.TrimSpace(cfg.Header); name != "" {
		if value := strings.TrimSpace(c.GetHeader(name)); value != "" {
			return value
		}
	}
	if name := strings.TrimSpace(cfg.Query); name != "" {
		if value := strings.TrimSpace(c.Query(name)); value != "" {
			return value
		}
	}
	return ""
}
