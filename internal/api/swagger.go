package api

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// registerSwagger 注册 Swagger UI。
// 文档由 swag init 生成到 docs 目录，执行 `make docs` 即可刷新。
func registerSwagger(engine *gin.Engine) {
	engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler,
		ginSwagger.PersistAuthorization(true),
	))
}
