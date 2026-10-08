package v1

import (
	"gin-quick-start/internal/config"
	"gin-quick-start/internal/pkg/token"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Dependencies 业务模块可用的基础设施依赖。
//
// 新增基础设施（Redis、消息队列……）时在这里加字段。加字段不会破坏
// 既有模块，所以这是唯一需要全体模块感知的变化。
type Dependencies struct {
	Config *config.Config
	Logger *logrus.Logger
	DB     *gorm.DB
	Token  *token.Manager
}

// Module 一个业务模块。
//
// 路由前缀、中间件、鉴权分组全部由模块自己在 Register 里决定 ——
// 想知道某个域暴露了哪些接口，看它自己那个文件就够了，不用去别处找。
type Module interface {
	Register(engine *gin.Engine)
}

// Modules 返回 v1 启用的全部业务模块。
//
// 新增业务域时只改这里：新建一个同包文件实现 Module，然后在下面加一行。
// 既有模块的代码一行都不用动。
func Modules(deps Dependencies) []Module {
	modules := []Module{
		newAccountModule(deps),
		newSystemModule(deps),
	}
	// GraphQL 查询层可以按配置整体关闭，关闭时一条路由都不挂
	if deps.Config.GraphQL.Enabled {
		modules = append(modules, newGraphQLModule(deps))
	}
	return modules
}
