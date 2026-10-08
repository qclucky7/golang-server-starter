// Package logger 初始化全局日志实例（logrus + lumberjack 滚动切割）。
package logger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gin-quick-start/internal/config"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Setup 初始化全局日志，同时接管 logrus.StandardLogger，
// 使第三方库与 contextx.Logger 的兜底路径共用同一套输出。
func Setup(cfg config.Log) (*logrus.Logger, error) {
	l := logrus.StandardLogger()

	level, err := logrus.ParseLevel(strings.ToLower(cfg.Level))
	if err != nil {
		level = logrus.InfoLevel
	}
	l.SetLevel(level)
	l.SetReportCaller(true)
	l.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02 15:04:05.000",
		DisableQuote:    true,
		CallerPrettyfier: func(frame *runtime.Frame) (string, string) {
			return "", fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)
		},
	})

	if cfg.Console {
		l.SetOutput(os.Stdout)
	} else {
		l.SetOutput(io.Discard)
	}

	if strings.TrimSpace(cfg.Dir) == "" {
		return l, nil
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建日志目录 %s 失败: %w", cfg.Dir, err)
	}

	// info 文件记录全部级别，error 文件只记录 Error 及以上
	l.AddHook(newFileHook(
		logrus.InfoLevel,
		filepath.Join(cfg.Dir, cfg.Filename),
		cfg,
	))
	l.AddHook(newFileHook(
		logrus.ErrorLevel,
		filepath.Join(cfg.Dir, cfg.ErrorFile),
		cfg,
	))
	return l, nil
}

// fileHook 按级别阈值写入独立文件
//
// 注意：logrus 对 hook 的匹配是「Levels() 里是否包含该条目的级别」这种精确匹配，
// 不是阈值比较。所以必须把所有 >= 阈值严重程度的级别都列出来，
// 否则 Warn 这类中间级别会既不落 info 文件也不落 error 文件，直接丢失。
type fileHook struct {
	writer   *lumberjack.Logger
	minLevel logrus.Level
}

func newFileHook(minLevel logrus.Level, filename string, cfg config.Log) *fileHook {
	return &fileHook{
		writer: &lumberjack.Logger{
			Filename:   filename,
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
		},
		minLevel: minLevel,
	}
}

// Levels 返回严重程度不低于 minLevel 的全部级别。
// logrus 级别数值越小越严重（Panic=0 ... Trace=6），故用 level <= minLevel 判断。
func (h *fileHook) Levels() []logrus.Level {
	levels := make([]logrus.Level, 0, len(logrus.AllLevels))
	for _, level := range logrus.AllLevels {
		if level <= h.minLevel {
			levels = append(levels, level)
		}
	}
	return levels
}

func (h *fileHook) Fire(entry *logrus.Entry) error {
	line, err := entry.String()
	if err != nil {
		return err
	}
	_, err = h.writer.Write([]byte(line))
	return err
}

// Close 关闭底层文件句柄。
func (h *fileHook) Close() error {
	return h.writer.Close()
}

// Close 关闭 Setup 注册的全部文件 hook。
//
// lumberjack 会持有文件句柄，进程退出前必须显式关闭，否则：
// 1. 缓冲区内容可能未落盘；
// 2. Windows 下文件被占用，删除/滚动日志目录会失败。
// 未调用过 Setup 或未配置日志目录时为空操作。
func Close(l *logrus.Logger) error {
	if l == nil {
		return nil
	}
	var errs []error
	for _, hooks := range l.Hooks {
		for _, hook := range hooks {
			if fh, ok := hook.(*fileHook); ok {
				if err := fh.Close(); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}
