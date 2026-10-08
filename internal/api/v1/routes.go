// Package v1 第一版 API：模块契约、模块清单与路由注册入口。
//
// 本包不含任何具体接口的知识 —— 每个业务域在自己的文件里实现 Module，
// 自己装配依赖、自己挂路由。新增业务域只需新建一个文件，并在 Modules 清单里加一行。
package v1

import "github.com/gin-gonic/gin"

// Register 装配 v1 全部业务模块并挂载路由
//
// 本函数不认识任何具体接口：遍历清单，每个模块挂自己的路由。
// 新增业务域时这里不需要改动。
func Register(engine *gin.Engine, deps Dependencies) {
	for _, m := range Modules(deps) {
		m.Register(engine)
	}
}
