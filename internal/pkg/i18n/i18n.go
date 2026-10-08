// Package i18n 提供多语言文案查表与语言协商。
//
// 设计约定：
//  1. 文案以「错误码 = key」的方式组织，见 locales/*.yaml，
//     因此 apperr.APIError.Code 可以直接当翻译 key 用。
//  2. 值支持 fmt 占位符 %s，按顺序对应 APIError.Extra。
//  3. 查不到译文、或译文占位符数量与参数对不上时，回落到调用方传入的默认模板，
//     保证任何情况下都不会输出 %!s(MISSING) 这类脏数据。
//  4. 未调用 Setup 时（或 config.i18n.enabled = false）不加载任何词条，
//     Render 会始终使用默认模板。
package i18n

import (
	"embed"
	"fmt"
	"path"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
)

//go:embed locales/*.yaml
var localeFS embed.FS

const (
	localeDir    = "locales"
	localeSuffix = ".yaml"
	// DefaultLocale 兜底语言
	DefaultLocale = "zh-CN"
)

// Manager 语言管理器，并发安全
type Manager struct {
	mu        sync.RWMutex
	fallback  string
	supported []string
	matcher   language.Matcher
	catalogs  map[string]map[string]string
}

var std = &Manager{
	fallback: DefaultLocale,
	catalogs: make(map[string]map[string]string),
}

// NewManager 创建独立的管理器（测试或多语言场景可用）
func NewManager() *Manager {
	return &Manager{
		fallback: DefaultLocale,
		catalogs: make(map[string]map[string]string),
	}
}

// Setup 初始化全局管理器：加载词条并构建语言匹配器。
//
// supported 为空时自动使用 locales/ 目录下发现的全部语言。
func Setup(supported []string, fallback string) error {
	return std.Setup(supported, fallback)
}

// Setup 加载词条并构建语言匹配器
func (m *Manager) Setup(supported []string, fallback string) error {
	available, err := discoverLocales()
	if err != nil {
		return err
	}
	if len(available) == 0 {
		return fmt.Errorf("i18n: %s 目录下未找到任何词条文件", localeDir)
	}

	catalogs := make(map[string]map[string]string, len(available))
	for _, locale := range available {
		messages, err := loadCatalog(locale)
		if err != nil {
			return err
		}
		catalogs[locale] = messages
	}

	if len(supported) == 0 {
		supported = available
	}

	tags := make([]language.Tag, 0, len(supported))
	locales := make([]string, 0, len(supported))
	for _, locale := range supported {
		normalized := Normalize(locale)
		if _, ok := catalogs[normalized]; !ok {
			return fmt.Errorf("i18n: 配置支持的语言 %s 缺少词条文件 %s/%s%s", locale, localeDir, normalized, localeSuffix)
		}
		tags = append(tags, language.Make(normalized))
		locales = append(locales, normalized)
	}

	fallback = Normalize(fallback)
	if fallback == "" {
		fallback = DefaultLocale
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.catalogs = catalogs
	m.supported = locales
	// 注意：matcher 返回的下标对应 tags 切片，因此 supported 必须与 tags 严格同序同长
	m.matcher = language.NewMatcher(tags)
	m.fallback = fallback
	return nil
}

// Match 按 Accept-Language 协商出最合适的受支持语言。
// 输入为空、非法或完全无法匹配时返回兜底语言。
func Match(acceptLanguage string) string { return std.Match(acceptLanguage) }

// Match 语言协商
func (m *Manager) Match(acceptLanguage string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.matcher == nil || strings.TrimSpace(acceptLanguage) == "" {
		return m.fallback
	}

	tags, _, err := language.ParseAcceptLanguage(sanitizeAcceptLanguage(acceptLanguage))
	if err != nil || len(tags) == 0 {
		return m.fallback
	}

	_, index, confidence := m.matcher.Match(tags...)
	if confidence == language.No || index < 0 || index >= len(m.supported) {
		return m.fallback
	}
	return m.supported[index]
}

// T 取翻译，未命中返回空串
func T(locale, key string) string { return std.T(locale, key) }

// T 取翻译，未命中返回空串
func (m *Manager) T(locale, key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	catalog, ok := m.catalogs[Normalize(locale)]
	if !ok {
		catalog = m.catalogs[m.fallback]
	}
	if catalog == nil {
		return ""
	}
	return catalog[key]
}

// Render 取翻译并渲染：
//   - 命中译文且占位符数量与 args 匹配 → 返回译文
//   - 否则回落到 defaultTemplate
//   - 占位符数量不匹配时不强行 Sprintf，避免输出 %!s(MISSING)
func Render(locale, key, defaultTemplate string, args ...any) string {
	return std.Render(locale, key, defaultTemplate, args...)
}

// Render 取翻译并渲染
func (m *Manager) Render(locale, key, defaultTemplate string, args ...any) string {
	template := m.T(locale, key)
	if template == "" || !placeholderMatched(template, len(args)) {
		template = defaultTemplate
	}
	if !placeholderMatched(template, len(args)) {
		return template
	}
	if len(args) == 0 {
		return template
	}
	return fmt.Sprintf(template, args...)
}

// Supported 返回已配置的受支持语言
func Supported() []string { return std.Supported() }

// Supported 返回已配置的受支持语言
func (m *Manager) Supported() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.supported...)
}

// Fallback 返回兜底语言
func Fallback() string { return std.Fallback() }

// Fallback 返回兜底语言
func (m *Manager) Fallback() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fallback
}

// IsSupported 判断语言是否已配置
func IsSupported(locale string) bool {
	for _, item := range Supported() {
		if item == Normalize(locale) {
			return true
		}
	}
	return false
}

// Normalize 归一化单个语言标签：zh_CN -> zh-CN，并去除空白与 q 值。
// 仅用于「单个语言」场景（配置项、词条文件名、T/Render 的 locale 参数）。
// 完整的 Accept-Language 串请使用 sanitizeAcceptLanguage。
func Normalize(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return ""
	}
	if index := strings.IndexByte(locale, ','); index >= 0 {
		locale = locale[:index]
	}
	if index := strings.IndexByte(locale, ';'); index >= 0 {
		locale = locale[:index]
	}
	locale = strings.TrimSpace(strings.ReplaceAll(locale, "_", "-"))
	return locale
}

// sanitizeAcceptLanguage 仅做最小清洗，保留完整的语言优先级列表。
// 注意不能复用 Normalize，否则会把 "zh-CN,zh;q=0.9,en;q=0.8" 截断成只剩 "zh-CN"。
func sanitizeAcceptLanguage(header string) string {
	return strings.TrimSpace(strings.ReplaceAll(header, "_", "-"))
}

// placeholderMatched 判断 %s 占位符数量是否与参数个数一致
func placeholderMatched(template string, count int) bool {
	return strings.Count(template, "%s") == count
}

// discoverLocales 扫描 embed 文件系统，返回全部词条文件名（不含扩展名）
func discoverLocales() ([]string, error) {
	entries, err := localeFS.ReadDir(localeDir)
	if err != nil {
		return nil, fmt.Errorf("i18n: 读取 %s 目录失败: %w", localeDir, err)
	}
	locales := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), localeSuffix) {
			continue
		}
		locales = append(locales, strings.TrimSuffix(entry.Name(), localeSuffix))
	}
	return locales, nil
}

// loadCatalog 读取单个语言的词条文件
func loadCatalog(locale string) (map[string]string, error) {
	raw, err := localeFS.ReadFile(path.Join(localeDir, locale+localeSuffix))
	if err != nil {
		return nil, fmt.Errorf("i18n: 读取词条文件 %s 失败: %w", locale, err)
	}
	messages := make(map[string]string)
	if err := yaml.Unmarshal(raw, &messages); err != nil {
		return nil, fmt.Errorf("i18n: 解析词条文件 %s 失败: %w", locale, err)
	}
	return messages, nil
}
