// Package bootstrap 负责依赖装配与应用生命周期管理。
package bootstrap

import (
	"fmt"

	"golang-server-starter/internal/api"
	"golang-server-starter/internal/config"
	"golang-server-starter/internal/database"
	"golang-server-starter/internal/pkg/i18n"
	"golang-server-starter/internal/pkg/logger"
	"golang-server-starter/internal/pkg/request"
	"golang-server-starter/internal/pkg/snowflake"
	"golang-server-starter/internal/pkg/token"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Container 依赖容器：集中创建并持有**进程级单例**。
//
// 只装两类东西：需要管理生命周期的（DB / Logger）、进程唯一入口（Engine / Config）。
//
// 业务对象（repository / service / handler）不进这里 —— 它们由
// internal/api/v1 下各业务模块在自己的文件里装配，新增模块不需要动本文件。
//
// 依赖方向（单向，无环）：
//
//	cmd -> bootstrap -> api -> api/v1 -> {业务模块} -> service -> repository -> database
//	                              -> middleware -> pkg/*
type Container struct {
	Config *config.Config
	Logger *logrus.Logger
	DB     *gorm.DB
	Engine *gin.Engine
}

// New 按配置装配基础设施并构建路由
func New(cfg *config.Config) (*Container, error) {
	log, err := logger.Setup(cfg.Log)
	if err != nil {
		return nil, err
	}

	// 主键由雪花算法在应用层生成，必须在任何写库动作之前初始化
	if err := snowflake.Setup(cfg.App.NodeID); err != nil {
		return nil, err
	}

	log.WithFields(logrus.Fields{
		"env":     cfg.App.Env,
		"configs": cfg.LoadedFiles,
		"node_id": snowflake.NodeID(),
	}).Info("配置加载完成")

	if err := request.RegisterValidation(); err != nil {
		return nil, fmt.Errorf("初始化参数校验失败: %w", err)
	}

	if cfg.I18N.Enabled {
		if err := i18n.Setup(cfg.I18N.Support, cfg.I18N.Fallback); err != nil {
			return nil, fmt.Errorf("初始化多语言词条失败: %w", err)
		}
		log.WithField("locales", i18n.Supported()).Info("多语言词条加载完成")
	}

	db, err := database.Open(cfg.Database)
	if err != nil {
		return nil, err
	}
	if cfg.Database.AutoMigrate {
		if err := database.Migrate(db); err != nil {
			_ = database.Close(db)
			return nil, err
		}
		log.Info("数据库自动迁移完成")
	}

	tokenManager := token.NewManager(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)

	engine := api.NewRouter(api.Dependencies{
		Config: cfg,
		Logger: log,
		DB:     db,
		Token:  tokenManager,
	})

	return &Container{
		Config: cfg,
		Logger: log,
		DB:     db,
		Engine: engine,
	}, nil
}

// Close 释放资源，按依赖的反向顺序关闭
func (c *Container) Close() error {
	if c.DB == nil {
		return nil
	}
	return database.Close(c.DB)
}
