// Package request 统一请求参数绑定与校验错误的转换。
//
// 绑定失败时直接 panic(apperr.ErrValidate)，由 middleware.ErrorHandler 统一渲染，
// 业务代码无需重复写 if err != nil 分支。
//
// 校验失败的具体原因会按请求语言（contextx.Locale）本地化后拼进错误描述，
// 例如：参数校验失败: password 需为 8-64 位且同时包含字母和数字
package request

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"golang-server-starter/internal/apperr"
	"golang-server-starter/internal/pkg/contextx"
	"golang-server-starter/internal/pkg/i18n"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var (
	usernamePattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
	passwordHasLetter = regexp.MustCompile(`[A-Za-z]`)
	passwordHasDigit  = regexp.MustCompile(`[0-9]`)
)

// 校验提示的兜底模板（中文），与 internal/pkg/i18n/locales 中的 validate.* 词条对应。
// 词条缺失或占位符数量不匹配时使用这里的模板。
const (
	defaultRequired       = "%s 为必填项"
	defaultEmail          = "%s 邮箱格式不正确"
	defaultUsername       = "%s 需为 3-32 位字母、数字、下划线或短横线"
	defaultPassword       = "%s 需为 8-64 位且同时包含字母和数字"
	defaultMin            = "%s 长度不能小于 %s"
	defaultMax            = "%s 长度不能大于 %s"
	defaultGte            = "%s 不能小于 %s"
	defaultLte            = "%s 不能大于 %s"
	defaultOneOf          = "%s 只能是 %s 之一"
	defaultURL            = "%s 需为合法 URL"
	defaultE164           = "%s 需为合法国际手机号"
	defaultValidationRule = "%s 不满足校验规则 %s"
)

// Bind 绑定 JSON 请求体
func Bind(c *gin.Context, obj any) {
	if err := c.ShouldBindJSON(obj); err != nil {
		panic(translate(c, err))
	}
}

// BindQuery 绑定 URL 查询参数
func BindQuery(c *gin.Context, obj any) {
	if err := c.ShouldBindQuery(obj); err != nil {
		panic(translate(c, err))
	}
}

// BindURI 绑定路径参数
func BindURI(c *gin.Context, obj any) {
	if err := c.ShouldBindUri(obj); err != nil {
		panic(translate(c, err))
	}
}

// RegisterValidation 注册自定义校验规则与字段名解析，应在服务启动前调用一次。
func RegisterValidation() error {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return errors.New("无法获取 gin 默认 validator 实例")
	}

	// 让校验错误里显示 json tag 名（前端字段名）而不是 Go 结构体字段名
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		if name == "" {
			return field.Name
		}
		return name
	})

	rules := map[string]validator.Func{
		"username": func(fl validator.FieldLevel) bool {
			return usernamePattern.MatchString(fl.Field().String())
		},
		"password": func(fl validator.FieldLevel) bool {
			value := fl.Field().String()
			if len(value) < 8 || len(value) > 64 {
				return false
			}
			return passwordHasLetter.MatchString(value) && passwordHasDigit.MatchString(value)
		},
	}
	for name, fn := range rules {
		if err := v.RegisterValidation(name, fn); err != nil {
			return fmt.Errorf("注册校验规则 %s 失败: %w", name, err)
		}
	}
	return nil
}

// translate 将绑定/校验错误转换为统一的 APIError，并按请求语言本地化细节
func translate(c *gin.Context, err error) apperr.APIError {
	locale := contextx.Locale(c)

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		return apperr.ErrValidate.WithExtra(joinFieldMessages(locale, validationErrors))
	}

	// 注意：下面两个分支必须使用各自的错误码，不能复用 ErrBadRequest。
	// 因为 Localize 是按 Code 查词条的，复用同一个 Code 会让自定义文案被词条掩盖。
	var unmarshalTypeError *json.UnmarshalTypeError
	if errors.As(err, &unmarshalTypeError) {
		return apperr.ErrValidateTypeMismatch.WithExtra(unmarshalTypeError.Field, unmarshalTypeError.Type.String())
	}

	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) {
		return apperr.ErrValidateJSONSyntax
	}

	return apperr.ErrBadRequest.WithMessage(err.Error())
}

func joinFieldMessages(locale string, errs validator.ValidationErrors) string {
	messages := make([]string, 0, len(errs))
	for _, fieldError := range errs {
		messages = append(messages, fieldMessage(locale, fieldError))
	}
	return strings.Join(messages, "; ")
}

func fieldMessage(locale string, fe validator.FieldError) string {
	field := fe.Field()
	switch fe.Tag() {
	case "required":
		return i18n.Render(locale, "validate.required", defaultRequired, field)
	case "email":
		return i18n.Render(locale, "validate.email", defaultEmail, field)
	case "username":
		return i18n.Render(locale, "validate.username", defaultUsername, field)
	case "password":
		return i18n.Render(locale, "validate.password", defaultPassword, field)
	case "min":
		return i18n.Render(locale, "validate.min", defaultMin, field, fe.Param())
	case "max":
		return i18n.Render(locale, "validate.max", defaultMax, field, fe.Param())
	case "gte":
		return i18n.Render(locale, "validate.gte", defaultGte, field, fe.Param())
	case "lte":
		return i18n.Render(locale, "validate.lte", defaultLte, field, fe.Param())
	case "oneof":
		return i18n.Render(locale, "validate.oneof", defaultOneOf, field, fe.Param())
	case "url":
		return i18n.Render(locale, "validate.url", defaultURL, field)
	case "e164":
		return i18n.Render(locale, "validate.e164", defaultE164, field)
	default:
		return i18n.Render(locale, "validate.default", defaultValidationRule, field, fe.Tag())
	}
}
