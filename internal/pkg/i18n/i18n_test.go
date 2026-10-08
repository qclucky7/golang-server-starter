package i18n_test

import (
	"testing"

	"golang-server-starter/internal/pkg/i18n"
)

func setup(t *testing.T) {
	t.Helper()
	if err := i18n.Setup([]string{"zh-CN", "en-US"}, "zh-CN"); err != nil {
		t.Fatalf("初始化词条失败: %v", err)
	}
}

func TestMatch(t *testing.T) {
	setup(t)

	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"空串回落兜底语言", "", "zh-CN"},
		{"纯中文", "zh-CN", "zh-CN"},
		{"纯英文", "en-US", "en-US"},
		{"下划线写法", "en_US", "en-US"},
		{"语言族收敛到具体地区", "en", "en-US"},
		{"中文语言族收敛", "zh", "zh-CN"},
		{"带 q 值优先级", "en-US,en;q=0.9,zh-CN;q=0.8", "en-US"},
		{"按 q 值选次优", "zh-CN;q=0.8,en-US;q=0.9", "en-US"},
		{"不支持的语言回落兜底", "ja-JP", "zh-CN"},
		{"非法串回落兜底", "!!!not-a-locale!!!", "zh-CN"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := i18n.Match(tc.header); got != tc.want {
				t.Fatalf("Match(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestT(t *testing.T) {
	setup(t)

	if got := i18n.T("zh-CN", "auth.token.expired"); got != "鉴权令牌已过期" {
		t.Fatalf("中文词条不符: %q", got)
	}
	if got := i18n.T("en-US", "auth.token.expired"); got != "Token has expired" {
		t.Fatalf("英文词条不符: %q", got)
	}
	if got := i18n.T("en-US", "not.exist.key"); got != "" {
		t.Fatalf("未命中应返回空串: %q", got)
	}
	// 未知语言回落到兜底语言的词条
	if got := i18n.T("ja-JP", "auth.token.expired"); got != "鉴权令牌已过期" {
		t.Fatalf("未知语言应回落兜底词条: %q", got)
	}
}

func TestRender(t *testing.T) {
	setup(t)

	cases := []struct {
		name     string
		locale   string
		key      string
		fallback string
		args     []any
		want     string
	}{
		{
			name:   "命中译文并替换占位符",
			locale: "en-US", key: "validate.required",
			fallback: "%s 为必填项", args: []any{"username"},
			want: "username is required",
		},
		{
			name:   "多占位符",
			locale: "zh-CN", key: "validate.min",
			fallback: "%s 长度不能小于 %s", args: []any{"password", "8"},
			want: "password 长度不能小于 8",
		},
		{
			name:   "词条缺失时回落默认模板",
			locale: "en-US", key: "not.exist.key",
			fallback: "%s 兜底文案", args: []any{"x"},
			want: "x 兜底文案",
		},
		{
			name:   "无占位符",
			locale: "en-US", key: "common.internal.error",
			fallback: "服务内部异常",
			want:     "Internal server error",
		},
		{
			name:   "占位符数量不匹配时不输出 %!s(MISSING)",
			locale: "en-US", key: "validate.required",
			fallback: "%s 为必填项",
			want:     "%s 为必填项",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := i18n.Render(tc.locale, tc.key, tc.fallback, tc.args...)
			if got != tc.want {
				t.Fatalf("Render() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSetupRejectsLocaleWithoutCatalog(t *testing.T) {
	if err := i18n.Setup([]string{"zh-CN", "ja-JP"}, "zh-CN"); err == nil {
		t.Fatal("配置了缺少词条文件的语言时应返回错误")
	}
	// 恢复到可用状态，避免影响后续用例
	setup(t)
}

func TestSupportedAndFallback(t *testing.T) {
	setup(t)

	supported := i18n.Supported()
	if len(supported) != 2 || supported[0] != "zh-CN" || supported[1] != "en-US" {
		t.Fatalf("受支持语言列表不符: %v", supported)
	}
	if i18n.Fallback() != "zh-CN" {
		t.Fatalf("兜底语言不符: %s", i18n.Fallback())
	}
	if !i18n.IsSupported("zh-CN") || i18n.IsSupported("ja-JP") {
		t.Fatal("IsSupported 判断错误")
	}
}
