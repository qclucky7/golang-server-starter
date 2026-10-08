package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gin-quick-start/internal/config"
)

// baseConfig 一份能通过校验的最小基线配置
const baseConfig = `
app:
  name: base-app
  node_id: 7
server:
  host: 1.2.3.4
  port: 8080
database:
  driver: mysql
  dsn: root:p@tcp(127.0.0.1:3306)/demo?parseTime=True
jwt:
  secret: test-secret-at-least-16-chars
`

// devConfig 环境文件只写差异项
const devConfig = `
server:
  mode: debug
database:
  driver: sqlite
  dsn: data/app.db
log:
  level: debug
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入 %s 失败: %v", name, err)
	}
	return path
}

// TestProfilePath 由基线路径推导环境文件路径。
func TestProfilePath(t *testing.T) {
	cases := []struct {
		base string
		env  string
		want string
	}{
		{"configs/config.yaml", "prod", filepath.Join("configs", "config-prod.yaml")},
		{"configs/config.yaml", "dev", filepath.Join("configs", "config-dev.yaml")},
		{"a/b/c.yml", "test", filepath.Join("a", "b", "c-test.yml")},
		{"config.yaml", "prod", "config-prod.yaml"},
	}
	for _, tc := range cases {
		if got := config.ProfilePath(tc.base, tc.env); got != tc.want {
			t.Errorf("ProfilePath(%q, %q) = %q，期望 %q", tc.base, tc.env, got, tc.want)
		}
	}
}

// TestLoadMergesProfileOverBase 环境文件必须**深合并**覆盖基线：
// 只写差异项，未出现的键（含嵌套键）要原样继承。
//
// 回归点：如果实现成整体替换，config-dev.yaml 里只写 server.mode
// 会把 server.host / server.port 一起抹掉，回落到代码默认值 ——
// 配置看起来「生效了」，但基线被悄悄丢弃。
func TestLoadMergesProfileOverBase(t *testing.T) {
	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", baseConfig)
	writeFile(t, dir, "config-dev.yaml", devConfig)

	cfg, err := config.Load(base, "dev")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	// 基线里未被环境文件触碰的键必须保留
	if cfg.Server.Host != "1.2.3.4" {
		t.Fatalf("server.host 应继承基线，实际 %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("server.port 应继承基线，实际 %d", cfg.Server.Port)
	}
	if cfg.App.Name != "base-app" {
		t.Fatalf("app.name 应继承基线，实际 %q", cfg.App.Name)
	}
	if cfg.App.NodeID != 7 {
		t.Fatalf("app.node_id 应继承基线，实际 %d", cfg.App.NodeID)
	}
	// 基线里 jwt 只有 secret，其余键回落到代码默认值
	if cfg.JWT.AccessTTL.String() != "2h0m0s" {
		t.Fatalf("jwt.access_ttl 应回落到默认值 2h，实际 %s", cfg.JWT.AccessTTL)
	}

	// 环境文件里出现的键必须覆盖基线
	if cfg.Server.Mode != "debug" {
		t.Fatalf("server.mode 应被环境文件覆盖，实际 %q", cfg.Server.Mode)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("database.driver 应被环境文件覆盖，实际 %q", cfg.Database.Driver)
	}
	if cfg.Database.DSN != "data/app.db" {
		t.Fatalf("database.dsn 应被环境文件覆盖，实际 %q", cfg.Database.DSN)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("log.level 应被环境文件覆盖，实际 %q", cfg.Log.Level)
	}

	if cfg.App.Env != "dev" {
		t.Fatalf("生效环境应为 dev，实际 %q", cfg.App.Env)
	}
	if len(cfg.LoadedFiles) != 2 {
		t.Fatalf("应加载 2 个配置文件，实际 %v", cfg.LoadedFiles)
	}
}

// TestLoadMissingProfileIsTolerated 环境文件不存在时只加载基线，不算错误。
func TestLoadMissingProfileIsTolerated(t *testing.T) {
	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", baseConfig)

	cfg, err := config.Load(base, "staging")
	if err != nil {
		t.Fatalf("环境文件缺失不应报错: %v", err)
	}
	if cfg.Database.Driver != "mysql" {
		t.Fatalf("应使用基线配置，实际 driver=%q", cfg.Database.Driver)
	}
	if len(cfg.LoadedFiles) != 1 {
		t.Fatalf("只应加载基线，实际 %v", cfg.LoadedFiles)
	}
	if cfg.App.Env != "staging" {
		t.Fatalf("生效环境应为 staging，实际 %q", cfg.App.Env)
	}
}

// TestLoadMissingBaseIsTolerated 基线也不存在时完全依赖默认值 + 环境变量。
//
// jwt.secret 没有默认值（生产必须显式注入），所以这里要提供 APP_JWT_SECRET，
// 否则会在校验阶段被拦下 —— 这是有意为之，不是缺陷。
func TestLoadMissingBaseIsTolerated(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APP_JWT_SECRET", "test-secret-at-least-16-chars")

	cfg, err := config.Load(filepath.Join(dir, "nope.yaml"), "dev")
	if err != nil {
		t.Fatalf("基线缺失不应报错: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("应回落到默认端口，实际 %d", cfg.Server.Port)
	}
	if cfg.Database.Driver != "mysql" {
		t.Fatalf("默认数据库应为 mysql，实际 %q", cfg.Database.Driver)
	}
	if !strings.Contains(cfg.Database.DSN, "parseTime=True") {
		t.Fatalf("默认 MySQL DSN 应带 parseTime=True，实际 %q", cfg.Database.DSN)
	}
	if cfg.App.Env != "dev" {
		t.Fatalf("应使用兜底环境 dev，实际 %q", cfg.App.Env)
	}
	if len(cfg.LoadedFiles) != 0 {
		t.Fatalf("不应记录任何已加载文件，实际 %v", cfg.LoadedFiles)
	}
}

// TestLoadFailsWithoutSecret 没有可用密钥时必须启动失败，而不是用空密钥签发令牌。
func TestLoadFailsWithoutSecret(t *testing.T) {
	dir := t.TempDir()
	if _, err := config.Load(filepath.Join(dir, "nope.yaml"), "dev"); err == nil {
		t.Fatal("缺少 jwt.secret 应校验失败")
	}
}

// TestLoadEnvResolution APP_ENV 与显式参数决定加载哪个 profile，参数优先。
func TestLoadEnvResolution(t *testing.T) {
	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", baseConfig)
	writeFile(t, dir, "config-dev.yaml", devConfig)
	writeFile(t, dir, "config-prod.yaml", "server:\n  mode: release\n")

	t.Run("APP_ENV 生效", func(t *testing.T) {
		t.Setenv(config.EnvKey, "prod")
		cfg, err := config.Load(base, "")
		if err != nil {
			t.Fatalf("加载失败: %v", err)
		}
		if cfg.App.Env != "prod" || cfg.Server.Mode != "release" {
			t.Fatalf("APP_ENV 未生效: env=%q mode=%q", cfg.App.Env, cfg.Server.Mode)
		}
	})

	t.Run("显式参数优先于 APP_ENV", func(t *testing.T) {
		t.Setenv(config.EnvKey, "prod")
		cfg, err := config.Load(base, "dev")
		if err != nil {
			t.Fatalf("加载失败: %v", err)
		}
		if cfg.App.Env != "dev" || cfg.Server.Mode != "debug" {
			t.Fatalf("显式参数应优先: env=%q mode=%q", cfg.App.Env, cfg.Server.Mode)
		}
	})

	t.Run("都没给时用基线里的 app.env 兜底", func(t *testing.T) {
		// 必须放在独立目录且文件名叫 config.yaml：
		// 环境文件名由基线文件名推导，config2.yaml 会去找 config2-prod.yaml
		sub := t.TempDir()
		baseWithEnv := writeFile(t, sub, "config.yaml", strings.Replace(
			baseConfig, "  name: base-app\n", "  name: base-app\n  env: prod\n", 1))
		writeFile(t, sub, "config-prod.yaml", "server:\n  mode: release\n")

		cfg, err := config.Load(baseWithEnv, "")
		if err != nil {
			t.Fatalf("加载失败: %v", err)
		}
		if cfg.App.Env != "prod" {
			t.Fatalf("应取基线里的 app.env，实际 %q", cfg.App.Env)
		}
		if cfg.Server.Mode != "release" {
			t.Fatalf("应按 prod profile 加载，实际 mode=%q", cfg.Server.Mode)
		}
	})
}

// TestLoadEnvVarOverridesProfile 环境变量优先级最高，能盖掉 profile 里的值。
func TestLoadEnvVarOverridesProfile(t *testing.T) {
	dir := t.TempDir()
	base := writeFile(t, dir, "config.yaml", baseConfig)
	writeFile(t, dir, "config-dev.yaml", devConfig)

	t.Setenv("APP_SERVER_PORT", "19090")
	t.Setenv("APP_APP_NAME", "from-env")

	cfg, err := config.Load(base, "dev")
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if cfg.Server.Port != 19090 {
		t.Fatalf("APP_SERVER_PORT 未生效，实际 %d", cfg.Server.Port)
	}
	if cfg.App.Name != "from-env" {
		t.Fatalf("APP_APP_NAME 未生效，实际 %q", cfg.App.Name)
	}
	// profile 的值仍在，未被环境变量误伤
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("profile 覆盖应保留，实际 %q", cfg.Database.Driver)
	}
}

// TestValidateMySQLRequiresParseTime 漏掉 parseTime=true 会在启动时就拦下来。
//
// 不拦的话，错误会在第一条查询上以 "unsupported Scan ... into *time.Time"
// 的形式出现，跟「配置漏了个参数」八竿子打不着。
func TestValidateMySQLRequiresParseTime(t *testing.T) {
	dir := t.TempDir()
	bad := writeFile(t, dir, "config.yaml", `
database:
  driver: mysql
  dsn: root:p@tcp(127.0.0.1:3306)/demo?charset=utf8mb4
jwt:
  secret: test-secret-at-least-16-chars
`)
	_, err := config.Load(bad, "dev")
	if err == nil {
		t.Fatal("缺少 parseTime=true 应校验失败")
	}
	if !strings.Contains(err.Error(), "parseTime") {
		t.Fatalf("错误信息应点明 parseTime，实际: %v", err)
	}

	// sqlite 不受该约束
	ok := writeFile(t, dir, "config-sqlite.yaml", `
database:
  driver: sqlite
  dsn: data/app.db
jwt:
  secret: test-secret-at-least-16-chars
`)
	if _, err := config.Load(ok, "dev"); err != nil {
		t.Fatalf("sqlite 不应受 parseTime 约束: %v", err)
	}
}

// TestValidateRejectsBadInput 其它校验项。
func TestValidateRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantSub string
	}{
		{
			name:    "非法端口",
			content: "server:\n  port: 70000\njwt:\n  secret: test-secret-at-least-16-chars\n",
			wantSub: "server.port",
		},
		{
			name:    "未知驱动",
			content: "database:\n  driver: oracle\n  dsn: x\njwt:\n  secret: test-secret-at-least-16-chars\n",
			wantSub: "database.driver",
		},
		{
			name:    "密钥过短",
			content: "database:\n  driver: sqlite\n  dsn: x\njwt:\n  secret: short\n",
			wantSub: "jwt.secret",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, "config.yaml", tc.content)
			_, err := config.Load(path, "dev")
			if err == nil {
				t.Fatal("应校验失败")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("错误信息应包含 %q，实际: %v", tc.wantSub, err)
			}
		})
	}
}

// TestRepoConfigsLoadable 仓库里真实提供的三套 profile 必须都能加载通过。
//
// 这条测试盯着的是「改了 config.yaml 却忘了同步 profile」这类漂移。
func TestRepoConfigsLoadable(t *testing.T) {
	base := filepath.Join("..", "..", "configs", "config.yaml")
	if _, err := os.Stat(base); err != nil {
		t.Skipf("找不到仓库配置目录: %v", err)
	}

	t.Setenv("APP_JWT_SECRET", "test-secret-at-least-16-chars")

	// 各 profile 的数据库驱动是刻意设计的：
	// 基线（以及 test / prod）是 MySQL，dev 覆盖为 sqlite 让本地零依赖。
	// 有人误改这里会直接测试失败。
	cases := []struct {
		env    string
		driver string
	}{
		{"dev", "sqlite"},
		{"test", "mysql"},
		{"prod", "mysql"},
	}
	for _, tc := range cases {
		t.Run(tc.env, func(t *testing.T) {
			cfg, err := config.Load(base, tc.env)
			if err != nil {
				t.Fatalf("configs/config-%s.yaml 加载失败: %v", tc.env, err)
			}
			if cfg.App.Env != tc.env {
				t.Fatalf("生效环境应为 %s，实际 %s", tc.env, cfg.App.Env)
			}
			if cfg.Database.Driver != tc.driver {
				t.Fatalf("%s profile 的数据库应为 %s，实际 %q", tc.env, tc.driver, cfg.Database.Driver)
			}
		})
	}
}
