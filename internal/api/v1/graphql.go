package v1

import (
	"net/http"
	"net/url"

	"gin-quick-start/internal/config"
	"gin-quick-start/internal/graphql"
	"gin-quick-start/internal/graphql/gqlctx"
	"gin-quick-start/internal/middleware"
	"gin-quick-start/internal/pkg/contextx"
	"gin-quick-start/internal/repository"
	"gin-quick-start/internal/service"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"github.com/vektah/gqlparser/v2/ast"
)

// defaultPlaygroundQuery GraphiQL 打开时预填的示例查询。
//
// 为什么示例长这样：本项目**没有列表 / 分页接口**（模板里连商品订单都没有），
// 拿不出「分页查列表」这种典型例子，所以改用「一次请求取回两个资源」来体现
// GraphQL 相对 REST 的实际价值 —— 同样的信息 REST 下要打
// /api/v1/auth/profile 与 /api/v1/system/health-check 两个接口。
const defaultPlaygroundQuery = `# 示例查询：一次取回两个资源
# REST 下等价于 /api/v1/auth/profile + /api/v1/system/health-check 两次请求
#
# me 需要令牌。先在左侧 Headers 面板填：
#   { "Authorization": "Bearer <access_token>" }
# access_token 从 POST /api/v1/auth/login 获取。
query Example {
  health {
    name
    version
    env
  }
  me {
    account {
      id
      username
      nickname
    }
    org {
      name
      isDefault
    }
  }
}`

// graphqlModule GraphQL 只读查询层。
//
// 它与其他模块不同的地方：**不是业务域，而是传输层**。
// schema 是全局单例（GraphQL 的固有形态），没法按业务域拆成多个文件，
// 所以这里只挂端点、装执行引擎，具体字段的解析逻辑在 internal/graphql 里。
//
// 挂成模块的好处是路由仍然集中在 Modules() 清单里，看清单就知道
// 这个服务对外暴露了哪些入口，不会多出一条「藏在 router.go 里的隐式路由」。
type graphqlModule struct {
	cfg    *config.Config
	auth   middleware.Authenticator
	server *handler.Server
}

// newGraphQLModule 装配 GraphQL 查询层
func newGraphQLModule(deps Dependencies) *graphqlModule {
	accountRepo := repository.NewAccountRepository(deps.DB)
	orgRepo := repository.NewOrgRepository(deps.DB)

	resolver := &graphql.Resolver{
		Config:   deps.Config,
		Accounts: service.NewAccountService(accountRepo, orgRepo),
	}

	// 用 handler.New 而不是已废弃的 handler.NewDefaultServer：
	// 后者会一并挂上 Websocket / MultipartForm 传输与 APQ 缓存，
	// 对一个只读查询端点来说是无谓的攻击面。
	server := handler.New(graphql.NewExecutableSchema(graphql.Config{Resolvers: resolver}))
	server.AddTransport(transport.Options{})
	server.AddTransport(transport.POST{})
	server.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	server.SetErrorPresenter(graphql.ErrorPresenter)
	server.SetRecoverFunc(graphql.Recover)
	if deps.Config.GraphQL.Introspection {
		server.Use(extension.Introspection{})
	}

	return &graphqlModule{
		cfg:    deps.Config,
		auth:   service.NewAuthService(accountRepo, orgRepo, deps.Token),
		server: server,
	}
}

// Register 挂载 GraphQL 端点
func (m *graphqlModule) Register(engine *gin.Engine) {
	group := engine.Group(m.cfg.GraphQL.Path)

	// 可选鉴权而不是强制鉴权：GraphQL 是单端点，强制鉴权会让
	// health 这类公开查询也必须带令牌。需要登录的字段（me）由解析器自己判。
	group.Use(middleware.OptionalAuth(m.auth))
	group.POST("", m.serve)

	if m.cfg.GraphQL.Playground {
		// 浏览器调试 IDE。挂在 engine 上而不是 group 上，避免被鉴权中间件拦下。
		engine.GET(m.cfg.GraphQL.Path, m.servePlayground)
	}
}

// servePlayground 提供 GraphiQL 调试页，并预填一个可跑的示例查询。
//
// GraphiQL 从**浏览器地址栏**读初始查询，服务端改写请求 URL 没用，
// 所以这里在缺少 query 参数时做一次跳转，把示例塞进 URL。
// 跳转只发生一次：带上 query 之后再进来就直接渲染页面。
func (m *graphqlModule) servePlayground(c *gin.Context) {
	serve := playground.Handler(m.cfg.App.Name, m.cfg.GraphQL.Path,
		playground.WithGraphiqlPersistStateInURL(true))

	if c.Query("query") == "" {
		target := m.cfg.GraphQL.Path + "?" + url.Values{"query": {defaultPlaygroundQuery}}.Encode()
		c.Redirect(http.StatusFound, target)
		return
	}
	serve(c.Writer, c.Request)
}

// serve 把请求交给 gqlgen 执行引擎
//
// 关键的一步是上下文搬运：gin 把值存在 *gin.Context 的 Keys 里，
// 而 gqlgen 的解析器只拿得到标准 context.Context，两者不共享存储。
func (m *graphqlModule) serve(c *gin.Context) {
	ctx := gqlctx.WithMeta(c.Request.Context(), gqlctx.Meta{
		Account:   contextx.Account(c),
		RequestID: contextx.RequestID(c),
		Locale:    contextx.Locale(c),
		Logger:    contextx.Logger(c),
	})
	m.server.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
}
