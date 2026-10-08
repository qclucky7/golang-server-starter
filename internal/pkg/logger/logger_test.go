package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang-server-starter/internal/config"

	"github.com/sirupsen/logrus"
)

// newTestConfig 构造一份写入临时目录的日志配置。
func newTestConfig(t *testing.T) config.Log {
	t.Helper()
	return config.Log{
		Level:      "info",
		Dir:        t.TempDir(),
		Filename:   "app.log",
		ErrorFile:  "error.log",
		MaxSizeMB:  1,
		MaxBackups: 1,
		MaxAgeDays: 1,
		Console:    false,
	}
}

// readFileOrEmpty 读取文件内容，文件不存在时返回空串。
func readFileOrEmpty(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return string(data)
}

// TestFileHookLevelRouting 验证级别路由：
// app.log 记录全部级别；error.log 只记录 Error 及以上。
// 回归点：hook.Levels() 必须返回「阈值及以上的全部级别」，
// 否则 Warn 会两级都不落，静默丢失。
func TestFileHookLevelRouting(t *testing.T) {
	cfg := newTestConfig(t)

	// Setup 会接管 logrus.StandardLogger，测试结束后恢复，避免影响同包其他用例。
	origin := logrus.StandardLogger().Hooks
	logrus.StandardLogger().ReplaceHooks(logrus.LevelHooks{})
	t.Cleanup(func() {
		// 先关文件句柄，再恢复 hook，最后 t.TempDir 才能删掉目录（Windows 会锁文件）。
		if err := Close(logrus.StandardLogger()); err != nil {
			t.Errorf("关闭日志文件失败: %v", err)
		}
		logrus.StandardLogger().ReplaceHooks(origin)
	})

	l, err := Setup(cfg)
	if err != nil {
		t.Fatalf("Setup 失败: %v", err)
	}

	l.Info("info-entry")
	l.Warn("warn-entry")
	l.Error("error-entry")

	appLog := readFileOrEmpty(t, filepath.Join(cfg.Dir, cfg.Filename))
	errLog := readFileOrEmpty(t, filepath.Join(cfg.Dir, cfg.ErrorFile))

	cases := []struct {
		name    string
		content string
		want    []string
		notWant []string
	}{
		{
			name:    "app.log 记录全部级别",
			content: appLog,
			want:    []string{"info-entry", "warn-entry", "error-entry"},
		},
		{
			name:    "error.log 只记录 Error 及以上",
			content: errLog,
			want:    []string{"error-entry"},
			notWant: []string{"info-entry", "warn-entry"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, want := range tc.want {
				if !strings.Contains(tc.content, want) {
					t.Errorf("期望包含 %q，实际内容:\n%s", want, tc.content)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(tc.content, notWant) {
					t.Errorf("不应包含 %q，实际内容:\n%s", notWant, tc.content)
				}
			}
		})
	}
}

// TestFileHookLevels 直接校验 Levels() 的集合语义。
func TestFileHookLevels(t *testing.T) {
	infoHook := newFileHook(logrus.InfoLevel, "app.log", config.Log{})
	got := map[logrus.Level]bool{}
	for _, lv := range infoHook.Levels() {
		got[lv] = true
	}

	// Info 及以上（数值 <= InfoLevel）必须包含。
	for _, lv := range []logrus.Level{
		logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel,
		logrus.WarnLevel, logrus.InfoLevel,
	} {
		if !got[lv] {
			t.Errorf("info hook 应包含级别 %s", lv)
		}
	}
	// Debug / Trace 必须排除。
	for _, lv := range []logrus.Level{logrus.DebugLevel, logrus.TraceLevel} {
		if got[lv] {
			t.Errorf("info hook 不应包含级别 %s", lv)
		}
	}

	errHook := newFileHook(logrus.ErrorLevel, "error.log", config.Log{})
	errGot := map[logrus.Level]bool{}
	for _, lv := range errHook.Levels() {
		errGot[lv] = true
	}
	if !errGot[logrus.ErrorLevel] || !errGot[logrus.FatalLevel] || !errGot[logrus.PanicLevel] {
		t.Error("error hook 应包含 Error / Fatal / Panic")
	}
	if errGot[logrus.WarnLevel] || errGot[logrus.InfoLevel] {
		t.Error("error hook 不应包含 Warn / Info")
	}
}

// TestSetupDisabledDir 未配置目录时不写文件，且不报错。
func TestSetupDisabledDir(t *testing.T) {
	l, err := Setup(config.Log{Level: "debug", Dir: "", Console: false})
	if err != nil {
		t.Fatalf("Setup 失败: %v", err)
	}
	if l.Level != logrus.DebugLevel {
		t.Errorf("期望级别 debug，实际 %s", l.Level)
	}
}
