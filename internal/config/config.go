// Package config 负责应用配置的定义、加载与校验。
//
// 多环境（对齐 Spring Boot 的 profile 机制）：
//
//	configs/config.yaml          基线配置，所有环境共享
//	configs/config-{env}.yaml    环境专属配置，只写与基线不同的项
//	APP_* 环境变量                最高优先级，用于 CI/CD 与容器注入
//
// 加载顺序即覆盖顺序：基线 → 环境文件 → 环境变量。后两者只需写差异项，
// 未出现的键自动继承上一层（深合并，不是整体替换）。
//
// 环境名（profile）的确定顺序：
//
//  1. 启动参数 --env / -e
//  2. 环境变量 APP_ENV（对齐 SPRING_PROFILES_ACTIVE 的习惯）
//  3. 基线配置里的 app.env
//  4. 兜底 dev
//
// 环境变量统一使用 APP_ 前缀，层级以 "_" 连接，例如：
//
//	APP_ENV=prod
//	APP_SERVER_PORT=9090
//	APP_DATABASE_DSN=root:pwd@tcp(127.0.0.1:3306)/demo?charset=utf8mb4&parseTime=true
//	APP_JWT_SECRET=xxxxx
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	// DefaultPath 默认基线配置文件路径
	DefaultPath = "configs/config.yaml"
	// DefaultEnv 未指定环境时使用的 profile
	DefaultEnv = "dev"
	// EnvPrefix 环境变量前缀
	EnvPrefix = "APP"
	// EnvKey 指定 profile 的环境变量名
	EnvKey = "APP_ENV"
)

// Config 应用总配置
type Config struct {
	App       App       `mapstructure:"app"`
	Server    Server    `mapstructure:"server"`
	Database  Database  `mapstructure:"database"`
	JWT       JWT       `mapstructure:"jwt"`
	I18N      I18N      `mapstructure:"i18n"`
	Log       Log       `mapstructure:"log"`
	RateLimit RateLimit `mapstructure:"rate_limit"`
	CORS      CORS      `mapstructure:"cors"`

	// LoadedFiles 实际加载成功的配置文件（按覆盖顺序），仅供启动日志展示。
	LoadedFiles []string `mapstructure:"-"`
}

// App 应用元信息
type App struct {
	Name    string `mapstructure:"name"`
	Version string `mapstructure:"version"`
	Env     string `mapstructure:"env"`
	// NodeID 雪花算法工作节点 ID，0-1023；传 -1 表示按主机名自动推导。
	// 多实例部署必须显式配置且互不相同。
	NodeID int64 `mapstructure:"node_id"`
}

// IsProd 是否生产环境
func (a App) IsProd() bool { return strings.EqualFold(a.Env, "prod") }

// IsDev 是否开发环境
func (a App) IsDev() bool { return strings.EqualFold(a.Env, "dev") }

// IsTest 是否测试环境
func (a App) IsTest() bool { return strings.EqualFold(a.Env, "test") }

// Server HTTP 服务配置
type Server struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Mode            string        `mapstructure:"mode"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// Addr 监听地址
func (s Server) Addr() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

// Database 数据库配置
type Database struct {
	Driver          string        `mapstructure:"driver"`
	DSN             string        `mapstructure:"dsn"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	LogLevel        string        `mapstructure:"log_level"`
	AutoMigrate     bool          `mapstructure:"auto_migrate"`
}

// JWT 令牌签发配置
type JWT struct {
	Secret     string        `mapstructure:"secret"`
	Issuer     string        `mapstructure:"issuer"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

// I18N 多语言配置
type I18N struct {
	// Enabled 关闭时不加载词条，所有文案使用代码内置的中文兜底模板
	Enabled bool `mapstructure:"enabled"`
	// Fallback 语言协商失败时的兜底语言
	Fallback string `mapstructure:"fallback"`
	// Support 支持的语言列表，留空则自动使用 internal/pkg/i18n/locales 下发现的全部语言
	Support []string `mapstructure:"support"`
	// Header 标准语言协商请求头，默认 Accept-Language
	Header string `mapstructure:"header"`
	// AltHeader 优先级更高的自定义语言请求头，便于前端显式指定，默认 X-Lang
	AltHeader string `mapstructure:"alt_header"`
	// Query 通过查询参数指定语言，留空表示不启用
	Query string `mapstructure:"query"`
}

// Log 日志配置
type Log struct {
	Level      string `mapstructure:"level"`
	Dir        string `mapstructure:"dir"`
	Filename   string `mapstructure:"filename"`
	ErrorFile  string `mapstructure:"error_file"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAgeDays int    `mapstructure:"max_age_days"`
	Compress   bool   `mapstructure:"compress"`
	Console    bool   `mapstructure:"console"`
	// LogBody 是否在访问日志中打印请求体/响应体（有性能开销，建议仅在开发/排障时开启）
	LogBody bool `mapstructure:"log_body"`
	// LogHeader 是否在访问日志中打印请求头/响应头（体积较小，可按需长期开启）
	LogHeader bool `mapstructure:"log_header"`
	// BodyLimit 请求体/响应体日志的截断长度（字节），0 表示使用默认值 4096
	BodyLimit int `mapstructure:"body_limit"`
	// SensitiveKeys 在内置脱敏名单之外追加的字段名/请求头名（大小写不敏感）。
	// 内置名单见 middleware.defaultSensitiveKeys，覆盖 password / token / Authorization / Cookie 等。
	SensitiveKeys []string `mapstructure:"sensitive_keys"`
}

// RateLimit 限流配置
type RateLimit struct {
	Enabled bool    `mapstructure:"enabled"`
	RPS     float64 `mapstructure:"rps"`
	Burst   int     `mapstructure:"burst"`
}

// CORS 跨域配置
type CORS struct {
	Enabled          bool          `mapstructure:"enabled"`
	AllowOrigins     []string      `mapstructure:"allow_origins"`
	AllowMethods     []string      `mapstructure:"allow_methods"`
	AllowHeaders     []string      `mapstructure:"allow_headers"`
	ExposeHeaders    []string      `mapstructure:"expose_headers"`
	AllowCredentials bool          `mapstructure:"allow_credentials"`
	MaxAge           time.Duration `mapstructure:"max_age"`
}

// ProfilePath 由基线路径推导某个环境的配置文件路径。
//
//	configs/config.yaml + prod -> configs/config-prod.yaml
func ProfilePath(basePath, env string) string {
	dir := filepath.Dir(basePath)
	ext := filepath.Ext(basePath)
	if ext == "" {
		ext = ".yaml"
	}
	name := strings.TrimSuffix(filepath.Base(basePath), filepath.Ext(basePath))
	return filepath.Join(dir, name+"-"+env+ext)
}

// Load 加载配置。path 为空时使用 DefaultPath，env 为空时按优先级自行推导。
//
// 基线文件或环境文件不存在都不算错误（可以完全依赖默认值 + 环境变量），
// 但**存在却解析失败**会直接报错，避免带着半份配置启动。
func Load(path, env string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultPath
	}

	v := viper.New()
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	// APP_ENV 单独绑定：AutomaticEnv 会把 app.env 映射成 APP_APP_ENV，
	// 那名字太别扭，这里显式对齐业界惯例。
	// node_id 同理，额外接受更顺手的 APP_NODE_ID（APP_APP_NODE_ID 仍然可用且优先）。
	_ = v.BindEnv("app.env", EnvKey)
	_ = v.BindEnv("app.node_id", "APP_NODE_ID")
	setDefaults(v)

	loaded := make([]string, 0, 2)

	// 1. 基线配置
	used, err := readInto(v, path)
	if err != nil {
		return nil, err
	}
	if used {
		loaded = append(loaded, path)
	}

	// 2. 环境名：参数 > APP_ENV > 基线里的 app.env > 兜底
	if strings.TrimSpace(env) == "" {
		env = v.GetString("app.env")
	}
	if strings.TrimSpace(env) == "" {
		env = DefaultEnv
	}

	// 3. 环境专属配置，深合并覆盖基线
	profilePath := ProfilePath(path, env)
	profile := viper.New()
	used, err = readInto(profile, profilePath)
	if err != nil {
		return nil, err
	}
	if used {
		loaded = append(loaded, profilePath)
		// MergeConfigMap 是深合并：环境文件只写差异项即可，嵌套键不会被整体替换
		if err := v.MergeConfigMap(profile.AllSettings()); err != nil {
			return nil, fmt.Errorf("合并环境配置 %s 失败: %w", profilePath, err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	// 生效环境以实际加载的 profile 为准，避免配置文件里的 app.env 与实际不符
	cfg.App.Env = env
	cfg.LoadedFiles = loaded

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// readInto 读取配置文件到 viper，返回该文件是否真的被加载。
//
// 注意不能用 v.ConfigFileUsed() 判断：viper 的 SetConfigFile 会立刻把路径记下来，
// 即使随后 ReadInConfig 因为文件不存在而失败，ConfigFileUsed() 依然返回这个路径。
//
// 文件不存在视为正常（可以完全依赖默认值 + 环境变量）；存在却读不了（权限、格式错误）
// 直接报错，避免带着半份配置启动。
func readInto(v *viper.Viper, path string) (bool, error) {
	v.SetConfigFile(path)
	err := v.ReadInConfig()
	switch {
	case err == nil:
		return true, nil
	case isConfigMissing(err):
		return false, nil
	default:
		return false, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}
}

// isConfigMissing 判断错误是否为「文件不存在」。
//
// viper 在不同路径下可能返回 ConfigFileNotFoundError 或裸的 *fs.PathError，
// 两种都要认。
func isConfigMissing(err error) bool {
	var notFound viper.ConfigFileNotFoundError
	if errors.As(err, &notFound) {
		return true
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return errors.Is(pathErr, fs.ErrNotExist)
	}
	return errors.Is(err, fs.ErrNotExist)
}

// Validate 配置校验
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port 非法: %d", c.Server.Port)
	}
	switch strings.ToLower(c.Database.Driver) {
	case "sqlite", "mysql", "postgres":
	default:
		return fmt.Errorf("database.driver 仅支持 sqlite/mysql/postgres，当前: %s", c.Database.Driver)
	}
	if strings.TrimSpace(c.Database.DSN) == "" {
		return errors.New("database.dsn 不能为空")
	}
	// MySQL 的 DATETIME 必须靠 parseTime 才能扫进 time.Time，漏了会在第一条查询上炸，
	// 错误信息（unsupported Scan ... into *time.Time）跟配置八竿子打不着，所以在这里拦下来
	if strings.EqualFold(c.Database.Driver, "mysql") &&
		!strings.Contains(strings.ToLower(c.Database.DSN), "parsetime=true") {
		return errors.New("database.dsn 缺少 parseTime=true，MySQL 的时间列无法扫描进 time.Time")
	}
	if len(c.JWT.Secret) < 16 {
		return errors.New("jwt.secret 长度需 >= 16，请通过 APP_JWT_SECRET 覆盖")
	}
	if c.JWT.AccessTTL <= 0 || c.JWT.RefreshTTL <= 0 {
		return errors.New("jwt.access_ttl / jwt.refresh_ttl 必须大于 0")
	}
	if c.I18N.Enabled {
		if strings.TrimSpace(c.I18N.Fallback) == "" {
			return errors.New("i18n.fallback 不能为空")
		}
		if strings.TrimSpace(c.I18N.Header) == "" && strings.TrimSpace(c.I18N.AltHeader) == "" {
			return errors.New("i18n.header 与 i18n.alt_header 不能同时为空")
		}
	}
	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "golang-server-starter")
	v.SetDefault("app.version", "1.0.0")
	v.SetDefault("app.env", DefaultEnv)
	v.SetDefault("app.node_id", -1)

	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "debug")
	v.SetDefault("server.read_timeout", "15s")
	v.SetDefault("server.write_timeout", "15s")
	v.SetDefault("server.idle_timeout", "60s")
	v.SetDefault("server.shutdown_timeout", "10s")

	// 默认数据库是 MySQL；postgres / sqlite 同样支持，改 database.driver 即可
	// （本地开发用 sqlite，见 configs/config-dev.yaml）
	v.SetDefault("database.driver", "mysql")
	v.SetDefault("database.dsn", "root:123456@tcp(127.0.0.1:3306)/golang_server_starter?charset=utf8mb4&parseTime=True&loc=Local")
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.conn_max_lifetime", "1h")
	v.SetDefault("database.log_level", "warn")
	v.SetDefault("database.auto_migrate", true)

	v.SetDefault("jwt.issuer", "golang-server-starter")
	v.SetDefault("jwt.access_ttl", "2h")
	v.SetDefault("jwt.refresh_ttl", "168h")
	// 没有默认值也必须登记：viper 的 Unmarshal 只遍历「已知键」（默认值 + 配置文件键），
	// 纯靠 APP_JWT_SECRET 注入的键若不登记就永远读不到，校验会以「密钥过短」失败。
	// 空值会在 Validate 里被拦下，所以这样既支持纯环境变量注入，又不会静默用空密钥启动。
	v.SetDefault("jwt.secret", "")

	v.SetDefault("i18n.enabled", true)
	v.SetDefault("i18n.fallback", "zh-CN")
	v.SetDefault("i18n.header", "Accept-Language")
	v.SetDefault("i18n.alt_header", "X-Lang")
	v.SetDefault("i18n.query", "lang")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.dir", "logs")
	v.SetDefault("log.filename", "app.log")
	v.SetDefault("log.error_file", "error.log")
	v.SetDefault("log.max_size_mb", 32)
	v.SetDefault("log.max_backups", 7)
	v.SetDefault("log.max_age_days", 30)
	v.SetDefault("log.compress", true)
	v.SetDefault("log.console", true)
	v.SetDefault("log.log_body", false)

	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.rps", 50)
	v.SetDefault("rate_limit.burst", 100)

	v.SetDefault("cors.enabled", true)
	v.SetDefault("cors.allow_origins", []string{"*"})
	v.SetDefault("cors.allow_methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
	v.SetDefault("cors.allow_headers", []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-Id"})
	v.SetDefault("cors.expose_headers", []string{"X-Request-Id"})
	v.SetDefault("cors.allow_credentials", false)
	v.SetDefault("cors.max_age", "12h")
}
