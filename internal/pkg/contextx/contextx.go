// Package contextx 封装 gin.Context 上的取值/存值，避免各处硬编码字符串 key。
//
// 定位：只服务于 HTTP 层（api / middleware）。它本质是「请求级的、带锁的
// map[string]any」，属于 gin 生态的惯用做法，但**不是** Java ThreadLocal 的对应物。
// 关键差别在于 goroutine 之间没有隔离：*gin.Context 由 gin 从 sync.Pool 取出，
// 请求结束后归还并清空 Keys。
//
// 因此有一条硬规则：
//
//	不要在 go func() 里直接持有 c。请求结束后 c 可能已被复用，
//	此时读到的是别的请求的数据。确实需要跨 goroutine 时：
//	  1. 先把需要的值取出来（值拷贝）再传进 goroutine；或
//	  2. 用 c.Copy() 取得一份独立的 *gin.Context（它会复制 Keys）。
//
// 业务层（service / repository）不应 import 本包，只接收 context.Context。
package contextx

import (
	"golang-server-starter/internal/model"
	"golang-server-starter/internal/pkg/i18n"
	"golang-server-starter/internal/pkg/token"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

const (
	keyRequestID = "ctx.request_id"
	keyAccount   = "ctx.account"
	keyClaims    = "ctx.claims"
	keyLogger    = "ctx.logger"
	keyLocale    = "ctx.locale"
)

// SetRequestID 写入请求 ID
func SetRequestID(c *gin.Context, id string) { c.Set(keyRequestID, id) }

// RequestID 读取请求 ID
func RequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.GetString(keyRequestID)
}

// SetAccount 写入当前登录账号
func SetAccount(c *gin.Context, account *model.Account) { c.Set(keyAccount, account) }

// Account 读取当前登录账号，未登录返回 nil
func Account(c *gin.Context) *model.Account {
	if c == nil {
		return nil
	}
	value, ok := c.Get(keyAccount)
	if !ok {
		return nil
	}
	account, _ := value.(*model.Account)
	return account
}

// AccountID 读取当前登录账号 ID，未登录返回 0
func AccountID(c *gin.Context) model.ID {
	if account := Account(c); account != nil {
		return account.ID
	}
	return 0
}

// SetClaims 写入令牌声明
func SetClaims(c *gin.Context, claims *token.Claims) { c.Set(keyClaims, claims) }

// Claims 读取令牌声明，不存在返回 nil
func Claims(c *gin.Context) *token.Claims {
	if c == nil {
		return nil
	}
	value, ok := c.Get(keyClaims)
	if !ok {
		return nil
	}
	claims, _ := value.(*token.Claims)
	return claims
}

// SetLocale 写入当前请求语言
func SetLocale(c *gin.Context, locale string) { c.Set(keyLocale, locale) }

// Locale 读取当前请求语言，未设置时返回 i18n 的兜底语言
func Locale(c *gin.Context) string {
	if c == nil {
		return i18n.Fallback()
	}
	if locale := c.GetString(keyLocale); locale != "" {
		return locale
	}
	return i18n.Fallback()
}

// SetLogger 写入带请求上下文的日志实例
func SetLogger(c *gin.Context, entry *logrus.Entry) { c.Set(keyLogger, entry) }

// Logger 读取带请求上下文的日志实例，不存在时回落到默认实例
func Logger(c *gin.Context) *logrus.Entry {
	if c == nil {
		return logrus.NewEntry(logrus.StandardLogger())
	}
	value, ok := c.Get(keyLogger)
	if !ok {
		return logrus.NewEntry(logrus.StandardLogger())
	}
	entry, ok := value.(*logrus.Entry)
	if !ok || entry == nil {
		return logrus.NewEntry(logrus.StandardLogger())
	}
	return entry
}
