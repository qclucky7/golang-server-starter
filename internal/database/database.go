// Package database 负责数据库连接与结构迁移。
package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang-server-starter/internal/config"
	"golang-server-starter/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open 按配置建立数据库连接并完成连接池设置
func Open(cfg config.Database) (*gorm.DB, error) {
	dialector, err := buildDialector(cfg)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(logLevel(cfg.LogLevel)),
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接池失败: %w", err)
	}
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库连通性检测失败: %w", err)
	}
	return db, nil
}

// Migrate 执行自动建表
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		return fmt.Errorf("自动迁移失败: %w", err)
	}
	return nil
}

// Close 关闭数据库连接
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func buildDialector(cfg config.Database) (gorm.Dialector, error) {
	switch strings.ToLower(cfg.Driver) {
	case "sqlite":
		if err := ensureSQLiteDir(cfg.DSN); err != nil {
			return nil, err
		}
		return sqlite.Open(cfg.DSN), nil
	case "mysql":
		return mysql.Open(cfg.DSN), nil
	case "postgres":
		return postgres.Open(cfg.DSN), nil
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", cfg.Driver)
	}
}

// ensureSQLiteDir 保证 sqlite 文件所在目录存在
func ensureSQLiteDir(dsn string) error {
	if dsn == "" || dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return nil
	}
	dir := filepath.Dir(dsn)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建数据库目录 %s 失败: %w", dir, err)
	}
	return nil
}

func logLevel(level string) gormlogger.LogLevel {
	switch strings.ToLower(level) {
	case "silent":
		return gormlogger.Silent
	case "error":
		return gormlogger.Error
	case "info":
		return gormlogger.Info
	default:
		return gormlogger.Warn
	}
}

// DefaultDSN 返回各驱动的默认 DSN，便于测试与文档示例
func DefaultDSN(driver string) string {
	switch strings.ToLower(driver) {
	case "mysql":
		return "root:123456@tcp(127.0.0.1:3306)/golang_server_starter?charset=utf8mb4&parseTime=True&loc=Local"
	case "postgres":
		return "host=127.0.0.1 user=postgres password=123456 dbname=golang_server_starter port=5432 sslmode=disable TimeZone=Asia/Shanghai"
	default:
		return "data/app.db"
	}
}
