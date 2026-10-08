package apperr_test

import (
	"errors"
	"strings"
	"testing"

	"gin-quick-start/internal/apperr"
	"gin-quick-start/internal/pkg/i18n"
)

func setupI18N(t *testing.T) {
	t.Helper()
	if err := i18n.Setup([]string{"zh-CN", "en-US"}, "zh-CN"); err != nil {
		t.Fatalf("初始化词条失败: %v", err)
	}
}

func TestLocalizeUsesCatalog(t *testing.T) {
	setupI18N(t)

	if got := apperr.ErrAuthTokenExpired.Localize("zh-CN"); got != "鉴权令牌已过期" {
		t.Fatalf("中文文案不符: %q", got)
	}
	if got := apperr.ErrAuthTokenExpired.Localize("en-US"); got != "Token has expired" {
		t.Fatalf("英文文案不符: %q", got)
	}
}

func TestLocalizeAppliesExtra(t *testing.T) {
	setupI18N(t)

	err := apperr.ErrAuthHeaderMissing.WithExtra("Auth")
	if got := err.Localize("zh-CN"); got != "缺少鉴权令牌，请在 Auth 请求头中携带" {
		t.Fatalf("中文带参文案不符: %q", got)
	}
	if got := err.Localize("en-US"); got != "Missing credentials, please provide a token in the Auth header" {
		t.Fatalf("英文带参文案不符: %q", got)
	}
}

// WithMessage 是显式覆盖文案，必须绕过 i18n 查表，
// 否则 Code 命中词条时自定义文案会被静默丢弃。
func TestWithMessageBypassesCatalog(t *testing.T) {
	setupI18N(t)

	err := apperr.ErrBadRequest.WithMessage("自定义文案")
	if got := err.Localize("zh-CN"); got != "自定义文案" {
		t.Fatalf("中文覆盖失效: %q", got)
	}
	if got := err.Localize("en-US"); got != "自定义文案" {
		t.Fatalf("英文覆盖失效: %q", got)
	}
}

func TestLocalizeFallsBackWhenKeyMissing(t *testing.T) {
	setupI18N(t)

	custom := apperr.New(400, "custom.not.in.catalog", "兜底文案")
	if got := custom.Localize("en-US"); got != "兜底文案" {
		t.Fatalf("词条缺失时应使用 Message: %q", got)
	}
}

func TestLocalizeNeverLeaksFormatVerb(t *testing.T) {
	setupI18N(t)

	// Extra 数量与译文占位符不匹配时，不应输出 %!s(MISSING)
	err := apperr.ErrValidate.WithExtra("a", "b", "c")
	got := err.Localize("en-US")
	if got == "" {
		t.Fatal("文案不应为空")
	}
	for _, bad := range []string{"%!s", "%!d", "%!(EXTRA"} {
		if strings.Contains(got, bad) {
			t.Fatalf("输出包含格式化残留 %q: %q", bad, got)
		}
	}
}

func TestIsMatchesByCode(t *testing.T) {
	setupI18N(t)

	wrapped := apperr.ErrAccountNotFound.WithExtra()
	if !errors.Is(wrapped, apperr.ErrAccountNotFound) {
		t.Fatal("errors.Is 应按错误码匹配")
	}
	if errors.Is(wrapped, apperr.ErrEmailExists) {
		t.Fatal("不同错误码不应匹配")
	}
}
