// Package apperr 定义全局统一错误类型与错误码。
//
// 设计约定：
//  1. 业务代码直接 panic(apperr.XXX) 或在 service 层返回 apperr.APIError，
//     由 middleware.ErrorHandler 统一捕获并渲染成标准错误响应体。
//  2. 错误码使用「域.子域.原因」的小写点分格式，例如 auth.token.expired。
//  3. Code 同时是 i18n 的翻译 key，词条位于 internal/pkg/i18n/locales/*.yaml。
//     Message 是中文兜底模板，仅在词条缺失或占位符数量对不上时生效。
//  4. Message 支持 fmt 占位符 %s，可变参数通过 Extra 按顺序传入。
package apperr

import (
	"errors"
	"fmt"
	"net/http"

	"gin-quick-start/internal/pkg/i18n"
)

// APIError 统一业务错误
type APIError struct {
	Status  int      // HTTP 状态码
	Code    string   // 业务错误码，同时作为 i18n 翻译 key
	Message string   // 中文兜底模板，支持 %s 占位符
	Extra   []string // Message 的格式化参数

	// noTranslate 为 true 时 Localize 直接使用 Message，不再按 Code 查词条。
	// 由 WithMessage 置位：调用方显式覆盖文案时，说明它比词条更精确
	// （例如调试模式下拼接的原始错误、动态生成的提示），不应被词条掩盖。
	noTranslate bool
}

// Error 实现 error 接口
func (e APIError) Error() string {
	return fmt.Sprintf("code: %s, message: %s", e.Code, e.Localize(i18n.Fallback()))
}

// Localize 按语言渲染错误描述。
//
// 优先使用 i18n 词条（key = Code），未命中或译文占位符数量与 Extra 不匹配时，
// 回落到 Message 这份中文兜底模板，保证任何情况下都不会输出 %!s(MISSING)。
// 若错误由 WithMessage 显式覆盖过文案，则直接返回该文案。
func (e APIError) Localize(locale string) string {
	if e.noTranslate {
		return e.Message
	}
	args := make([]any, 0, len(e.Extra))
	for _, v := range e.Extra {
		args = append(args, v)
	}
	return i18n.Render(locale, e.Code, e.Message, args...)
}

// Message 渲染中文兜底描述，自动应用 Extra 中的格式化参数
func Message(e APIError) string {
	return e.Localize(i18n.Fallback())
}

// WithExtra 附加格式化参数，返回副本
func (e APIError) WithExtra(extra ...string) APIError {
	clone := e
	clone.Extra = append(append([]string{}, e.Extra...), extra...)
	return clone
}

// WithMessage 覆盖错误描述，返回副本。
// 覆盖后不再按 Code 查 i18n 词条，适用于调用方比词条更清楚文案的场景。
func (e APIError) WithMessage(message string) APIError {
	clone := e
	clone.Message = message
	clone.Extra = nil
	clone.noTranslate = true
	return clone
}

// Is 支持 errors.Is 按错误码比较
func (e APIError) Is(target error) bool {
	var other APIError
	if errors.As(target, &other) {
		return e.Code == other.Code
	}
	return false
}

// New 创建自定义错误
func New(status int, code, message string) APIError {
	return APIError{Status: status, Code: code, Message: message}
}

// ---------------------------------------------------------------------------
// 通用错误
// ---------------------------------------------------------------------------

var (
	// ErrBadRequest 请求参数错误
	ErrBadRequest = APIError{Status: http.StatusBadRequest, Code: "common.bad.request", Message: "请求参数错误"}
	// ErrValidate 参数校验失败，Extra 由校验器自动填充
	ErrValidate = APIError{Status: http.StatusBadRequest, Code: "common.validate.error", Message: "参数校验失败: %s"}
	// ErrNotFound 资源不存在
	ErrNotFound = APIError{Status: http.StatusNotFound, Code: "common.not.found", Message: "资源不存在"}
	// ErrConflict 资源冲突
	ErrConflict = APIError{Status: http.StatusConflict, Code: "common.conflict", Message: "资源已存在"}
	// ErrForbidden 无权限
	ErrForbidden = APIError{Status: http.StatusForbidden, Code: "common.forbidden", Message: "无访问权限"}
	// ErrTooManyRequests 触发限流
	ErrTooManyRequests = APIError{Status: http.StatusTooManyRequests, Code: "common.rate.limit", Message: "请求过于频繁，请稍后重试"}
	// ErrInternal 未知服务异常
	ErrInternal = APIError{Status: http.StatusInternalServerError, Code: "common.internal.error", Message: "服务内部异常"}
	// ErrDatabase 数据库操作异常
	ErrDatabase = APIError{Status: http.StatusInternalServerError, Code: "common.database.error", Message: "数据库操作异常"}
	// ErrRouteNotFound 路由不存在，由 router 的 NoRoute 使用
	ErrRouteNotFound = APIError{Status: http.StatusNotFound, Code: "common.route.not.found", Message: "接口不存在"}
	// ErrMethodNotAllowed 请求方法不被允许，由 router 的 NoMethod 使用
	ErrMethodNotAllowed = APIError{Status: http.StatusMethodNotAllowed, Code: "common.method.not.allowed", Message: "请求方法不被允许"}
	// ErrValidateJSONSyntax 请求体不是合法 JSON
	ErrValidateJSONSyntax = APIError{Status: http.StatusBadRequest, Code: "validate.json.syntax", Message: "请求体不是合法的 JSON"}
	// ErrValidateTypeMismatch 字段类型不匹配，Extra 依次为字段名、期望类型
	ErrValidateTypeMismatch = APIError{Status: http.StatusBadRequest, Code: "validate.type.mismatch", Message: "字段 %s 类型应为 %s"}
)

// ---------------------------------------------------------------------------
// 鉴权与账号
// ---------------------------------------------------------------------------

var (
	// ErrAuthHeaderMissing 请求头未携带令牌，Extra 为鉴权请求头名称
	ErrAuthHeaderMissing = APIError{Status: http.StatusUnauthorized, Code: "auth.token.missing", Message: "缺少鉴权令牌，请在 %s 请求头中携带"}
	// ErrAuthTokenMalformed 令牌格式非法，Extra 为期望的令牌前缀
	ErrAuthTokenMalformed = APIError{Status: http.StatusUnauthorized, Code: "auth.token.malformed", Message: "鉴权令牌格式非法，期望 %s <token>"}
	// ErrAuthTokenInvalid 令牌无效
	ErrAuthTokenInvalid = APIError{Status: http.StatusUnauthorized, Code: "auth.token.invalid", Message: "鉴权令牌无效"}
	// ErrAuthTokenExpired 令牌已过期
	ErrAuthTokenExpired = APIError{Status: http.StatusUnauthorized, Code: "auth.token.expired", Message: "鉴权令牌已过期"}
	// ErrAuthTokenRevoked 令牌已被吊销
	ErrAuthTokenRevoked = APIError{Status: http.StatusUnauthorized, Code: "auth.token.revoked", Message: "鉴权令牌已失效，请重新登录"}
	// ErrAuthTokenWrongType 令牌类型不正确（例如用刷新令牌访问接口）
	ErrAuthTokenWrongType = APIError{Status: http.StatusUnauthorized, Code: "auth.token.wrong.type", Message: "令牌类型不正确"}
	// ErrAuthAccountNotFound 令牌对应的账号不存在
	ErrAuthAccountNotFound = APIError{Status: http.StatusUnauthorized, Code: "auth.account.not.found", Message: "账号不存在或已注销"}
	// ErrAuthAccountDisabled 账号被禁用
	ErrAuthAccountDisabled = APIError{Status: http.StatusForbidden, Code: "auth.account.disabled", Message: "账号已被禁用"}
	// ErrAuthPasswordWrong 账号或密码错误
	ErrAuthPasswordWrong = APIError{Status: http.StatusUnauthorized, Code: "auth.password.wrong", Message: "账号或密码错误"}
	// ErrAuthOldPasswordWrong 原密码错误
	ErrAuthOldPasswordWrong = APIError{Status: http.StatusBadRequest, Code: "auth.old.password.wrong", Message: "原密码错误"}
	// ErrAuthTokenSignFailed 令牌签发失败
	ErrAuthTokenSignFailed = APIError{Status: http.StatusInternalServerError, Code: "auth.token.sign.failed", Message: "令牌签发失败"}
)

// ---------------------------------------------------------------------------
// 账号域
// ---------------------------------------------------------------------------

var (
	// ErrAccountNotFound 账号不存在
	ErrAccountNotFound = APIError{Status: http.StatusNotFound, Code: "account.not.found", Message: "账号不存在"}
	// ErrUsernameExists 用户名已占用
	ErrUsernameExists = APIError{Status: http.StatusConflict, Code: "account.username.exists", Message: "用户名已被占用"}
	// ErrEmailExists 邮箱已占用
	ErrEmailExists = APIError{Status: http.StatusConflict, Code: "account.email.exists", Message: "邮箱已被注册"}
	// ErrAccountCreateFailed 创建账号失败
	ErrAccountCreateFailed = APIError{Status: http.StatusInternalServerError, Code: "account.create.failed", Message: "创建账号失败"}
	// ErrAccountUpdateFailed 更新账号失败
	ErrAccountUpdateFailed = APIError{Status: http.StatusInternalServerError, Code: "account.update.failed", Message: "更新账号失败"}
	// ErrAccountPasswordHashFailed 密码加密失败
	ErrAccountPasswordHashFailed = APIError{Status: http.StatusInternalServerError, Code: "account.password.hash.failed", Message: "密码加密失败"}
)

// ---------------------------------------------------------------------------
// 组织域
// ---------------------------------------------------------------------------

var (
	// ErrOrgNotFound 组织不存在。当前组织只作为账号的默认组织自动创建，
	// 尚无对外接口；等组织管理接口落地后直接复用。
	ErrOrgNotFound = APIError{Status: http.StatusNotFound, Code: "org.not.found", Message: "组织不存在"}
	// ErrOrgCreateFailed 创建组织失败（注册时初始化默认组织失败会返回它）
	ErrOrgCreateFailed = APIError{Status: http.StatusInternalServerError, Code: "org.create.failed", Message: "创建组织失败"}
)
