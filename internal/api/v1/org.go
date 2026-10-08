package v1

import (
	"golang-server-starter/internal/dto"
	"golang-server-starter/internal/middleware"
	"golang-server-starter/internal/pkg/contextx"
	"golang-server-starter/internal/pkg/convert"
	"golang-server-starter/internal/pkg/request"
	"golang-server-starter/internal/pkg/response"
	"golang-server-starter/internal/repository"
	"golang-server-starter/internal/service"

	"github.com/gin-gonic/gin"
)

// orgModule 组织域：分页查询当前登录账号拥有的组织。
//
// 这是模板里唯一的列表 / 分页接口，作用是把分页链路完整串一遍，供新业务照抄：
//
//	query.Page（入参）→ repository.Page（查询）→ dto.NewPageResult（分页信息）→ response.Page（渲染）
//
// 响应形状是固定的 grid.result 契约：
//
//	{"model":"grid.result","page":{...},"result":{"model":"data.set","datas":[...],"total":N},"request_id":"..."}
//
// 与账号域一样，所有受保护接口只操作当前登录账号自己，路径里不带目标账号 ID。
type orgModule struct {
	orgs *service.OrgService
	auth middleware.Authenticator
}

// newOrgModule 装配组织域
func newOrgModule(deps Dependencies) *orgModule {
	accountRepo := repository.NewAccountRepository(deps.DB)
	orgRepo := repository.NewOrgRepository(deps.DB)
	return &orgModule{
		orgs: service.NewOrgService(orgRepo),
		auth: service.NewAuthService(accountRepo, orgRepo, deps.Token),
	}
}

// Register 挂载组织路由
func (m *orgModule) Register(engine *gin.Engine) {
	g := engine.Group("/api/v1/orgs").Use(middleware.Auth(m.auth))
	g.GET("", m.list)
}

// list 组织列表
//
//	@Tags			Org
//	@Summary		组织列表
//	@Description	分页查询当前登录账号拥有的组织。current/size 越界会静默归一化；order_by 仅接受模型真实字段，非法值回落 id
//	@Produce		json
//	@Security		Authorization
//	@Param			current		query		int		false	"当前页码，从 1 开始"			default(1)
//	@Param			size		query		int		false	"每页条数，上限 200"			default(10)
//	@Param			order_by	query		string	false	"排序字段，默认 id"
//	@Param			order		query		string	false	"排序方向"	Enums(asc, desc)	default(desc)
//	@Success		200			{object}	response.PageBody	"查询成功"
//	@Failure		401			{object}	model.SystemErrorResult	"未登录或令牌失效"
//	@Router			/api/v1/orgs [get]
func (m *orgModule) list(c *gin.Context) {
	var req dto.OrgQuery
	request.BindQuery(c, &req)

	// ownerID 取自令牌解析出的当前账号，不接受请求参数传入
	items, total, err := m.orgs.ListByOwner(c.Request.Context(), contextx.AccountID(c), &req.Page)
	if err != nil {
		panic(err)
	}

	// req.Page 已被仓储层就地归一化，直接用它构造分页信息
	response.Page(c, convert.AnySlice(dto.NewOrgs(items)), dto.NewPageResult(req.Page, int(total)))
}
