package v1

import (
	"gin-quick-start/internal/config"
	"gin-quick-start/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

// systemModule 系统域：存活探针与健康检查。
type systemModule struct {
	cfg *config.Config
}

// newSystemModule 装配系统域
func newSystemModule(deps Dependencies) *systemModule {
	return &systemModule{cfg: deps.Config}
}

// Register 挂载系统路由
//
// 存活探针挂在根路径 /healthz（不带 /api/v1 前缀），供容器编排直接探测。
func (m *systemModule) Register(engine *gin.Engine) {
	engine.GET("/healthz", m.liveness)
	engine.Group("/api/v1").GET("/system/health-check", m.healthCheck)
}

// HealthCheckResponse 健康检查响应
type HealthCheckResponse struct {
	Available bool   `json:"available" example:"true"`
	Name      string `json:"name" example:"gin-quick-start"`
	Version   string `json:"version" example:"1.0.0"`
	Env       string `json:"env" example:"dev"`
}

// healthCheck 服务健康检查
//
//	@Tags			System
//	@Summary		健康检查
//	@Description	检测服务是否可用，并返回应用基础信息
//	@Produce		json
//	@Success		200	{object}	response.Body{data=v1.HealthCheckResponse}	"服务可用"
//	@Router			/api/v1/system/health-check [get]
func (m *systemModule) healthCheck(c *gin.Context) {
	response.OK(c, "system.health.check", HealthCheckResponse{
		Available: true,
		Name:      m.cfg.App.Name,
		Version:   m.cfg.App.Version,
		Env:       m.cfg.App.Env,
	})
}

// liveness 存活探针
//
//	@Tags			System
//	@Summary		存活探针
//	@Description	仅用于容器编排探测，不返回业务信息
//	@Produce		plain
//	@Success		200	{string}	string	"ok"
//	@Router			/healthz [get]
func (m *systemModule) liveness(c *gin.Context) {
	c.String(200, "ok")
}
