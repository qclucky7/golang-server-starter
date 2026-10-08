# gin-quick-start

基于 **Gin + GORM** 的 Go 服务端模板：统一响应体、统一错误处理、JWT 鉴权、泛型 CRUD 仓储、雪花主键、多环境配置、Swagger 文档、GraphQL 只读查询层。

开箱即可注册 / 登录，按「基线 + 环境差异」组织配置，分层单向依赖，可直接作为新项目的起点。

> 本仓库由早期版本 `Simple-Gin-Example` 重构而来，架构与约定对齐参考模板 `golang-web-template`；
> 在其基础上额外保留了 `internal/pkg/httpclient`（出站 HTTP 客户端）。

> **REST 与 GraphQL 并行**：REST 负责全部命令操作（注册 / 登录 / 刷新 / 登出 / 改密）与探针；
> GraphQL 只提供 Query（`health` / `me`），不提供 Mutation。详见 [GraphQL 查询层](#graphql-查询层)。

---

## 快速开始

> 需要 Go >= 1.26（`go.mod` 的 go 指令由 gin v1.12 / quic-go 等依赖决定）。

```bash
go mod tidy                  # 首次执行
go run ./cmd/server -e dev   # 本地启动，sqlite，零依赖不用起数据库
```

| 环境 | 数据库 | 位置 |
|---|---|---|
| 基线 | MySQL | `configs/config.yaml`，test / prod 共用 |
| `dev` | sqlite | `configs/config-dev.yaml` → `data/app.db` |
| `test` | MySQL | `configs/config-test.yaml`，独立库名 `gin_quick_start_test` |
| `prod` | MySQL | `configs/config-prod.yaml`，不写 driver 即继承基线 |

PostgreSQL 同样支持（改 `database.driver` 即可），只是不作默认。启动日志会打印实际加载的配置文件：

```
env=dev configs=[configs/config.yaml configs/config-dev.yaml] node_id=837
```

| 地址 | 说明 |
|---|---|
| `http://127.0.0.1:8080/healthz` | 存活探针 |
| `http://127.0.0.1:8080/api/v1/system/health-check` | 健康检查 |
| `http://127.0.0.1:8080/swagger/index.html` | Swagger UI |
| `http://127.0.0.1:8080/graphql` | **GraphQL 查询端点**（POST），浏览器打开则进 GraphiQL 调试页 |

> 打开 `http://127.0.0.1:8080/graphql` 会**自动预填一个可跑的示例查询**，直接点运行即可。
> 示例同时取 `health` 与 `me` 两个资源 —— REST 下这要打两个接口。
> GraphiQL 仅在 `dev` 环境开启（`graphql.playground`）。

### 冒烟测试

```bash
# 注册（事务内同时创建默认组织）
curl -X POST http://127.0.0.1:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"demo","email":"demo@example.com","password":"demo1234"}'

# 登录，拿 access_token
curl -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"account":"demo","password":"demo1234"}'

# 访问受保护接口（只认标准 Authorization 头）
curl http://127.0.0.1:8080/api/v1/auth/profile \
  -H "Authorization: Bearer <access_token>"
# 返回当前账号 + 默认组织；ID 是字符串（雪花 19 位）

# GraphQL：匿名查询
curl -X POST http://127.0.0.1:8080/graphql \
  -H "Content-Type: application/json" \
  -d '{"query":"{ health { name version env } }"}'

# GraphQL：一次取回账号与组织（等价于 REST 的 /auth/profile）
curl -X POST http://127.0.0.1:8080/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"query":"{ me { account { id username } org { name isDefault } } }"}'
```

---

## 目录结构

```
.
├── cmd/server/main.go        # 入口：解析参数 → 装配 → 启动 → 优雅退出
├── configs/                  # 基线 + 环境差异（config-dev / -test / -prod.yaml）
├── docs/                     # swag init 生成的接口文档
├── gqlgen.yml                # GraphQL 代码生成配置（schema 路径 / 输出位置 / 标量映射）
├── tools.go                  # 把 gqlgen 等命令行工具锚在 go.mod 里（build tag 排除编译）
├── internal/
│   ├── api/                  # HTTP 装配
│   │   ├── router.go         #   引擎构建 + 404/405 兜底
│   │   ├── swagger.go        #   Swagger UI
│   │   └── v1/               #   v1 模块体系
│   │       ├── module.go     #     模块契约 + 模块清单（新增业务域改这里）
│   │       ├── routes.go     #     遍历清单挂载路由（不含具体接口知识）
│   │       ├── account.go    #     账号域：装配 + 路由 + 处理器（REST）
│   │       ├── graphql.go    #     GraphQL 端点（传输层模块，可按配置关闭）
│   │       └── system.go     #     系统域：装配 + 路由 + 处理器
│   ├── apperr/               # 统一错误类型与错误码
│   ├── bootstrap/            # 基础设施装配（Container）与 HTTP 生命周期（Server）
│   ├── config/               # 配置定义、加载、校验
│   ├── constant/             # 请求头、默认值等常量
│   ├── database/             # GORM 连接、连接池、自动迁移
│   ├── dto/                  # 请求/响应结构，与实体解耦
│   ├── graphql/              # GraphQL 查询层
│   │   ├── schema.graphqls   #   schema（唯一契约，改完跑 make gqlgen）
│   │   ├── generated.go      #   生成：执行引擎
│   │   ├── schema.resolvers.go # 生成：解析器方法（实现写在里面，重新生成会保留）
│   │   ├── resolver.go       #   手写：根解析器（持有 service）
│   │   ├── convert.go        #   手写：实体 → GraphQL 投影（白名单）
│   │   ├── error.go          #   手写：错误映射（apperr → GraphQL error）
│   │   ├── auth.go           #   手写：当前账号取值
│   │   ├── model/            #   生成：GraphQL 类型
│   │   ├── scalar/           #   手写：自定义标量（Time）
│   │   └── gqlctx/           #   手写：HTTP 请求信息搬进 resolver context
│   ├── middleware/           # request_id / locale / error_handler / cors / logger / ratelimit / auth
│   ├── model/                # 数据库实体 + 公共数据结构
│   ├── repository/           # 数据访问层（泛型 CRUD 基类 + 表专属方法）
│   ├── service/              # 业务逻辑层
│   └── pkg/                  # 与业务无关的基础设施
│       ├── contextx/         #   gin.Context 存取封装
│       ├── convert/          #   泛型转换
│       ├── hash/             #   bcrypt 密码哈希
│       ├── httpclient/       #   出站 HTTP 客户端（resty，带脱敏日志）
│       ├── i18n/             #   多语言词条 + 语言协商
│       ├── logger/           #   logrus + lumberjack
│       ├── query/            #   分页/排序参数
│       ├── request/          #   参数绑定与校验错误翻译
│       ├── response/         #   统一响应体构造
│       ├── snowflake/        #   雪花 ID 生成器
│       └── token/            #   JWT 签发与校验
├── Makefile / make.bat       # 开发命令（Windows 用 make.bat）
└── Dockerfile
```

### 依赖方向（单向，无环）

```
cmd  ->  bootstrap  ->  api  ->  api/v1  ->  {业务模块}  ->  service  ->  repository  ->  database
                                     ->  middleware  ->  pkg/*
```

- `service` 不感知 `gin.Context`，只接收 `context.Context` 与 DTO，便于单测与复用。
- `middleware` 通过接口（`middleware.Authenticator`）反向获取 service 能力，不直接依赖 `service` 包。
- **每个业务模块自己装配自己的 repository 与 service**（见[新增一个业务模块](#新增一个业务模块的完整流程)），
  所以 `bootstrap` 不需要知道任何仓储的存在 —— 它只创建进程级单例（Config / Logger / DB / Engine）。

### 中间件顺序（不可调整）

```
RequestID → Locale → Logger → ErrorHandler → CORS → RateLimit
```

`Logger` 必须在 `ErrorHandler` 外层，且日志写在 `defer` 中 —— 否则业务以 panic 抛错时，panic 展开会跳过 `Logger` 的后置代码，4xx / 5xx 全部没有访问日志。

---

## 统一响应格式

| 场景 | 响应体 |
|---|---|
| 单对象 | `{"model":"account","data":{...},"request_id":"3f1a..."}` |
| 数据集 | `{"model":"data.set","datas":[...],"total":10,"request_id":"..."}` |
| 分页 | `{"model":"grid.result","page":{"current":1,"size":10,"total":25,"total_page":3},"result":{"model":"data.set","datas":[...]},"request_id":"..."}` |
| 空结果 | `{"model":"empty","request_id":"..."}` |
| 失败 | `{"model":"errors","errors":{"model":"data.set","datas":[{"model":"error","code":"auth.token.expired","message":"鉴权令牌已过期"}],"total":1},"request_id":"..."}` |

- `model` 自描述响应类型，客户端据此分发；`request_id` 与响应头 `X-Request-Id` 一致，便于日志追踪。
- **`id` 一律是字符串。** 雪花 ID 为 19 位十进制，超过 JS `Number.MAX_SAFE_INTEGER`（2^53-1，16 位），按数字下发会在前端静默丢精度。所有实体 ID 与 JWT 的 `aid` 声明都按字符串序列化。
- **空结果返回 HTTP 200 + `{"model":"empty"}`**，不是裸 204 —— 统一响应体优先，客户端只需一套解析逻辑。

### 固定契约

上表五种形状是**固定的响应契约**，客户端按 `model` 字段分发，形状必须稳定。对应的构造件全部保留，即使当前暂无调用点：

`response.List` / `response.Page` / `model.PageResult` / `model.GridResult` / `dto.PageQuery` / `dto.NewPageResult`

新增列表 / 分页接口时直接接上：

| 环节 | 用什么 |
|---|---|
| 请求参数 | `dto.PageQuery`（内嵌 `query.Page`，自带分页参数 + 排序字段白名单校验） |
| 数据查询 | `Repository[T].Page(ctx, &q.Page, scopes...)` → `(items, total, err)` |
| 分页信息 | `dto.NewPageResult(q.Page, int(total))` |
| 响应渲染 | `response.Page(c, convert.AnySlice(items), page)`；不分页则用 `response.List` |

---

## 错误处理

`internal/apperr` 定义 `APIError{Status, Code, Message, Extra}`。service 层直接返回，handler 层直接 `panic`（免写 `if err != nil`），由 `middleware.ErrorHandler` 统一捕获渲染：

```go
// service：直接返回
return nil, apperr.ErrDatabase

// handler：直接 panic
org, err := h.accounts.DefaultOrg(c.Request.Context(), account.ID)
if err != nil {
    panic(err)
}
```

- 5xx 记 `error` 级日志，4xx 记 `warn` 级。
- 非 `APIError` 的未知错误统一降级为 `common.internal.error`，不泄漏内部细节；`app.env=dev` 时附加原始错误信息便于本地排查。
- 错误码格式 `域.子域.原因`（如 `auth.token.expired`），新增只改 `internal/apperr/apperr.go`。

---

## 鉴权

**只认标准 `Authorization` 头，其他写法一律拒绝。**

```
Authorization: Bearer <access_token>     ✅ 唯一合法写法
Authorization: bearer <access_token>     ✅ scheme 大小写不敏感（RFC 7235）
Authorization: <access_token>            ❌ 缺 Bearer 前缀 → auth.token.malformed
Auth: Bearer <access_token>              ❌ 自定义头 → auth.token.missing
?token=<access_token>                    ❌ 查询参数 → auth.token.missing
```

| 场景 | 错误码 |
|---|---|
| 请求头缺失或为空 | `auth.token.missing`（`Extra` 带合法头名） |
| 有头但 scheme 不对 / 令牌为空 | `auth.token.malformed`（`Extra` 带 `Bearer`） |

鉴权头**不可配置**，固定为 `Authorization`，因此没有 `auth.*` 配置项。不接受 URL 传令牌：令牌会进浏览器历史、`Referer` 头、Nginx access log 与 CDN 日志。下载 / SSE 场景的正确做法是前端先 `fetch` 拿数据再 `blob` 下载。

### 令牌机制

- 访问令牌（默认 2h）+ 刷新令牌（默认 7d），HS256 签名。
- `accounts.token_version`：**登出 / 改密 / 删除都会递增该字段，从而吊销该账号已签发的全部令牌**。
- 鉴权顺序：签名 → 有效期 → 令牌类型 → `token_version` 一致 → 账号未禁用。
- 令牌声明**不含任何权限信息**（只有 `aid` / `username` / `ver` / `typ`）。权限属于「账号在组织里的成员关系」，塞进令牌会随成员关系变化而失效，只能等令牌过期才纠正。
- `aid` 与响应体同理按字符串编码（`json:"aid,string"`）。

> 如需「只登出当前设备」或「在线设备管理」，新增 `account_sessions` 表记录 refresh_token 哈希，把 `token_version` 校验替换为 session 校验即可，改动集中在 `service/auth.go` 与 `repository/account.go`。

### 在路由上启用

在业务模块自己的 `Register` 里挂鉴权中间件：

```go
// internal/api/v1/account.go
authed := auth.Group("").Use(middleware.Auth(m.auth))
```

```go
// 处理器中读取当前账号
account := contextx.Account(c)     // *model.Account
id      := contextx.AccountID(c)   // model.ID
```

> `middleware.Auth` 只回答「这个请求是谁」，不回答「它能做什么」。授权（谁能操作哪个组织）等 `org_user` 表引入后再补授权中间件。

---

## 接口一览

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/healthz` | - | 存活探针 |
| GET | `/api/v1/system/health-check` | - | 健康检查 |
| POST | `/api/v1/auth/register` | - | 注册（同时创建默认组织） |
| POST | `/api/v1/auth/login` | - | 登录 |
| POST | `/api/v1/auth/refresh` | - | 刷新令牌 |
| POST | `/api/v1/auth/logout` | ✅ | 登出（吊销全部令牌） |
| GET | `/api/v1/auth/profile` | ✅ | 当前账号信息 + 默认组织 |
| PUT | `/api/v1/auth/password` | ✅ | 修改密码 |
| POST | `/graphql` | 可选 | GraphQL 查询端点（字段级鉴权） |
| GET | `/graphql` | - | GraphiQL 调试页（仅 `graphql.playground` 开启时） |

REST 接口只有**认证**与**账号自服务**两类。全部受保护接口都只操作当前登录账号自己，路径里不带目标账号 ID。

所有接口都支持语言协商（`X-Lang` / `Accept-Language` / `?lang=`），详见[多语言](#多语言i18n)。

> `internal/api/v1/routes_test.go` 断言了路由总数与路径前缀，接口面被无声扩大时会直接测试失败。
> `internal/api/v1/graphql_test.go` 另外钉住了 GraphQL 端点的挂载规则、错误码与 200/422 的分界。

---

## GraphQL 查询层

与 REST **并行**存在，只提供 Query，不提供 Mutation。

| | REST | GraphQL |
|---|---|---|
| 承担 | 全部命令操作（注册 / 登录 / 刷新 / 登出 / 改密）+ 探针 | 只读查询（`health` / `me`） |
| 响应体 | `{model, data, request_id}` 信封 | `{data, errors}`（GraphQL 规范形状，**不套信封**） |
| 失败表现 | HTTP 状态码 + 信封里的错误码 | 业务错误 **HTTP 200** + `errors[].extensions.code` |
| 文档 | Swagger UI（`make docs`） | GraphiQL 的 Docs 面板 + introspection |

### 为什么不做 Mutation

写操作需要事务边界、幂等与鉴权的一致语义，而 GraphQL 的业务错误一律返回 HTTP 200，
失败只能从 `errors[]` 里看 —— 对登录这类需要明确状态码的场景是净损失。
查询则相反：客户端能按需选字段、一次取多个资源，这才是 GraphQL 的价值所在。

### 错误处理：200 与 422 的分界

**并不是所有 GraphQL 错误都返回 200**，gqlgen 按错误类型分两档：

| 类型 | 例子 | HTTP | `extensions.code` |
|---|---|---|---|
| 业务错误（`apperr.APIError`） | 未登录、账号或密码错误 | **200** | 与 REST 同一套错误码，如 `auth.token.missing` |
| 协议错误（语法 / 校验） | 查询写错、字段不存在 | **422** | `GRAPHQL_PARSE_FAILED` / `GRAPHQL_VALIDATION_FAILED` |
| 未预期错误（panic、原始 error） | 空指针、未包装的 DB 错误 | **200** | `common.internal.error`，细节只进日志 |

所以客户端**不能只看状态码**：业务失败是 200，得看 `errors[].extensions.code`；
而 422 说明查询本身写错了，是开发期问题，不是业务分支。

错误映射集中在 `internal/graphql/error.go` 的 `ErrorPresenter` / `Recover`。
之所以不用 gqlgen 默认的 `DefaultRecover`：它把原始错误打到 `os.Stderr` 并统一换成
`internal system error`，业务错误码会整条丢掉。

### 怎么加一个新查询

项目里**没有列表 / 分页接口**可以照抄，下面是一个最小完整例子（加一个「按当前账号统计组织数」的查询）：

1. **改 schema** `internal/graphql/schema.graphqls`：

```graphql
type Query {
  health: Health!
  me: AccountDetail!
  # 新增
  myOrgCount: Int!
}
```

2. **重新生成**：

```bash
make gqlgen        # Windows: make.bat gqlgen
```

gqlgen 会在 `schema.resolvers.go` 里补一个 `panic("not implemented")` 的桩，
**已有的解析器实现会原样保留**（它按方法名比对，不会覆盖）。

3. **填实现**，并在 `internal/graphql/convert.go` 里做投影（如果返回的是实体）：

```go
func (r *queryResolver) MyOrgCount(ctx context.Context) (int, error) {
	account, err := currentAccount(ctx) // 未登录返回 auth.token.missing
	if err != nil {
		return 0, err
	}
	return r.Accounts.CountOrgs(ctx, account.ID)
}
```

4. **补测试**：`internal/api/v1/graphql_test.go` 里有现成的 `postGraphQL` 辅助函数，直接复用。

要点：

- **service 层不动**。GraphQL 只是多了一个入口，`service` / `repository` / `dto` 一行都不用改。
- **别直接返回实体**。GraphQL 类型是独立投影（`internal/graphql/model`），
  在 `convert.go` 里显式列字段 —— 这样数据库新增列不会自动对外暴露。
- **鉴权在解析器里做**。端点用可选鉴权（否则公开查询也得带令牌），
  需要登录的字段调 `currentAccount(ctx)`。
- **改了 schema 必须重新生成**，否则编译期就会报类型不匹配 —— 这正是选 gqlgen 的理由。

### 配置

```yaml
graphql:
  enabled: true          # 关闭则一条路由都不挂
  path: /graphql
  playground: false      # 浏览器调试页，dev 环境打开
  introspection: false   # 是否允许拉取完整 schema，dev 环境打开
```

生产建议两者都关：它们会把完整 schema 与数据形状暴露给任何人。

---

## 数据库与 CRUD

支持 **MySQL（基线默认）/ PostgreSQL / sqlite**，改 `database.driver` + DSN 即可切换。

| 驱动 | DSN 形态 | 注意 |
|---|---|---|
| `mysql` | `root:pwd@tcp(127.0.0.1:3306)/db?charset=utf8mb4&parseTime=True&loc=Local` | **必须带 `parseTime=True`**，否则 `DATETIME` 无法扫描成 `time.Time`，只在运行时以 `unsupported Scan ... into *time.Time` 暴露。启动校验会直接拦下缺该参数的 DSN |
| `postgres` | `host=... user=... password=... dbname=... port=5432 sslmode=disable TimeZone=Asia/Shanghai` | 本地/内网无 TLS **必须**写 `sslmode=disable`；`TimeZone` 建议显式指定 |
| `sqlite` | 文件路径，如 `data/app.db` | `github.com/glebarez/sqlite`，纯 Go 无需 CGO；目录不存在会自动创建 |

只有 `dev` 用 sqlite（本地零依赖），test / prod 与线上同构走 MySQL。要连真实库就把 `configs/config-dev.yaml` 的 `database` 段换成 mysql / postgres，文件里留了注释好的模板。

### 泛型仓储

`internal/repository.Repository[T]` 提供开箱即用的 CRUD：

```go
repo := repository.New[model.Order](db)

// 写
repo.Create(ctx, &order)      // 另有 CreateBatch / Update / Delete / Count / Exists
repo.UpdateFields(ctx, id, map[string]any{"status": 1})
repo.DeleteBy(ctx, repository.WhereEq("status", 0))
repo.Transaction(ctx, func(tx *repository.Repository[model.Order]) error { ... })
// 跨仓储原子写入：fn 内必须用 WithDB(tx) 派生仓储，否则会跑到事务外
repo.TransactionDB(ctx, func(tx *gorm.DB) error { return otherRepo.WithDB(tx).Create(ctx, &other) })

// 读（未找到返回 (nil, nil)，不把 gorm.ErrRecordNotFound 泄漏到上层）
entity, err       := repo.GetByID(ctx, id)
items, err        := repo.List(ctx, repository.WhereLike("name", "foo"))
items, total, err := repo.Page(ctx, &query.Page{Current: 1, Size: 10}, scopes...)
```

内置 Scope 助手：`WhereID` / `WhereIDs` / `WhereEq` / `WhereLike` / `OrderBy`。

### 实体基础字段

所有实体内嵌 `model.Base`：

```go
type Base struct {
    ID          ID                    `gorm:"primaryKey;autoIncrement:false;comment:主键（雪花算法）" json:"id"`
    CreatedTime time.Time             `gorm:"autoCreateTime;comment:创建时间" json:"created_time"`
    UpdatedTime time.Time             `gorm:"autoUpdateTime;comment:更新时间" json:"updated_time"`
    DeletedTime soft_delete.DeletedAt `gorm:"index;comment:软删除时间戳（unix 秒，0 表示未删除）" json:"-"`
}
```

### 主键：雪花算法

`github.com/bwmarrin/snowflake`：41 位毫秒时间戳 + 10 位节点号 + 12 位序列号，节点在应用层生成、DB 不参与。

相比自增：多实例 / 分库分表不需要全局发号器；不泄漏业务规模（`id=100` 等于暴露「你是第 100 个客户」）；INSERT 前即可拿到 ID；数值且趋势递增，InnoDB 聚簇索引不裂页（UUIDv4 完全随机，会频繁裂页）。代价是 19 位长度，必须按字符串下发。

```go
// internal/model/base.go
func (b *Base) BeforeCreate(*gorm.DB) error {
    if b.ID != 0 { return nil }        // 显式赋值优先，便于测试与数据导入
    id, err := snowflake.Next()
    if err != nil { return err }
    b.ID = ID(id)
    return nil
}
```

> **`autoIncrement:false` 不能省。** GORM 对 `int64` 主键**默认按自增处理**，INSERT 时会忽略该列并回读 `LastInsertId`，结果是 `BeforeCreate` 赋的值被静默丢弃、主键变成 1、2、3……`TestSnowflakePrimaryKey` 专门盯这一点。

节点号 `app.node_id`：`>= 0` 显式指定（0~1023，多实例 / K8s 用 StatefulSet 序号注入）；`-1`（默认）按主机名 FNV-1a 哈希取模 1024。

> 主机名哈希**不保证集群内唯一**，多实例部署必须显式指定 `APP_NODE_ID`。库内建了时钟回拨保护：可容忍范围内等待，超出则返回错误（**不会**生成重复 ID）。

`model.ID` 是 `int64` 命名类型，`MarshalJSON` 恒定输出带引号的十进制串，`UnmarshalJSON` 同时接受字符串与数字。

### 时间字段与软删除

**时间字段统一 `xxx_time` 命名，且必须显式写 tag。** GORM 的自动时间戳按**字段名**识别，只认 `CreatedAt` / `UpdatedAt`；本项目用 `CreatedTime` / `UpdatedTime`，靠 `autoCreateTime` / `autoUpdateTime` 兜住。少写 tag **不会报错**，字段会静默保持零值 —— 这是最容易踩的坑。

**软删除用 `gorm.io/plugin/soft_delete`，`DeletedTime` 存 unix 秒，`0` 表示未删除。** GORM 自动追加 `WHERE deleted_time = 0`，删除自动改写为 `UPDATE`，无需注册插件（纯类型实现）：

```go
db.Unscoped().Where("id = ?", id).Delete(&model.Account{})   // 物理删除
```

不用可空时间戳（`gorm.DeletedAt`）的原因：`NULL` 在唯一索引中互不冲突，会让 `(唯一列, deleted_time)` 这类复合唯一索引形同虚设；用 `0` 才能让复合唯一索引真正生效，也便于实现「软删后可复用唯一键」。

> 当前 `username` / `email` 用**单列**唯一索引，软删后仍占位（防抢注），这是有意设计。要改成「软删后可复用」，把唯一索引换成 `udx_username` 这样的复合索引（`Username` + `DeletedTime` 共用同名索引名）；注意同一秒内删除两条同名记录会冲突，那种场景改用 `gorm:"softDelete:milli"`。

### 账号与组织模型

实体划分为**账号（Account）** 与 **组织（Org）**。

```
accounts                          orgs
┌──────────────────┐         ┌──────────────────────┐
│ id               │◄────────│ owner_id             │
│ username (uniq)  │         │ name                 │
│ email    (uniq)  │         │ is_default           │
│ password         │         └──────────────────────┘
│ nickname         │          1 个账号至少 1 个默认组织
│ status           │
│ token_version    │          （未来）org_user
└──────────────────┘          ┌──────────────────────┐
                              │ org_id + account_id  │
                              │ 账号加入他人组织      │
                              └──────────────────────┘
```

- **注册即建组织**：`POST /auth/register` 在**同一事务**里写入账号 + 默认组织。只成功一半会留下「有账号没组织」的脏数据，所以两者必须原子（见 `AuthService.Register`）。
- **默认组织名**：`{昵称}的组织`（昵称缺省回落用户名），由 `service.DefaultOrgName` 生成。
- **`is_default` 是独立列，不能靠 `owner_id` 反推**：① `owner_id` 是普通索引，schema 允许一个账号拥有多个组织；② 「归属」与「登录后默认进入哪个组织」是两件事 —— 默认组织可以是别人拥有的。
- **账号表不含权限字段**：权限属于「账号在某个组织里的成员关系」，落到未来的 `org_user` 表。个人资料字段（`avatar` 等）同理，等有写入接口时再加。

`GET /auth/profile` 连同默认组织一起返回：

```json
{
  "model": "account",
  "data": {
    "account": { "id": "1859123456789012480", "username": "demo", "email": "demo@example.com" },
    "org": { "id": "1859123456789012481", "name": "demo的组织", "owner_id": "1859123456789012480", "is_default": true }
  }
}
```

### 新增一个业务模块的完整流程

以「订单」为例：

| 步骤 | 文件 | 内容 |
|---|---|---|
| 1 | `internal/model/order.go` | 定义 `Order`（内嵌 `model.Base`），加入 `model.AllModels()` |
| 2 | `internal/repository/order.go` | `type OrderRepository struct { *Repository[model.Order] }` + 构造函数 |
| 3 | `internal/dto/order.go` | 请求 / 响应结构 |
| 4 | `internal/service/order.go` | 业务逻辑，返回 `apperr.APIError` |
| 5 | `internal/api/v1/order.go` | **新建模块文件**：装配 + 路由 + 处理器 |
| 6 | `internal/api/v1/module.go` | 模块清单里**加一行** |

> 如果这个模块也要提供 GraphQL 查询，**不要**新建 `newXxxGraphQLModule` ——
> GraphQL 是单端点 + 全局 schema，加查询的做法是「改 schema + 在 `internal/graphql`
> 里补解析器」，见 [怎么加一个新查询](#怎么加一个新查询)。
> 上面这套流程加的是 **REST 模块**。

第 5 步长这样 —— 装配、路由、处理器全在一个文件里，本域的东西一眼看全：

```go
type orderModule struct {
	orders *service.OrderService
}

// 装配：repository 与 service 都在这里创建，不导出给外部
func newOrderModule(deps Dependencies) *orderModule {
	repo := repository.NewOrderRepository(deps.DB)
	return &orderModule{orders: service.NewOrderService(repo)}
}

// 路由：前缀、中间件、鉴权分组都由本模块自己决定
func (m *orderModule) Register(engine *gin.Engine) {
	g := engine.Group("/api/v1/orders").Use(middleware.Auth(m.authenticator))
	g.GET("", m.list)
	g.POST("", m.create)
}

// 处理器：swag 注释照常写
func (m *orderModule) list(c *gin.Context) { ... }
```

第 6 步就是在 `Modules()` 里加一行：

```go
func Modules(deps Dependencies) []Module {
	return []Module{
		newAccountModule(deps),
		newSystemModule(deps),
		newOrderModule(deps),      // ← 新增
	}
}
```

**`bootstrap/container.go`、`api/router.go`、`api/v1/routes.go` 都不需要改动，也不影响任何既有模块。**

需要新的基础设施（Redis、消息队列……）时，在 `api/v1/module.go` 的 `Dependencies` 里加字段即可 —— 加字段不会破坏既有模块。

> 清单那一行漏了是**静默失效**（接口全部 404，但不报错），所以 `api/v1/module_test.go`
> 用 AST 比对「定义了哪些模块构造函数」与「清单里注册了哪些」，漏了会直接测试失败。

> 生产建议关闭 `database.auto_migrate`，改用 golang-migrate / goose 管理表结构变更。

---

## 多语言（i18n）

**错误码本身就是翻译 key**，错误响应会自动按请求语言渲染，业务代码不需要写任何多语言逻辑。

语言协商优先级：`X-Lang`（自定义头）> `Accept-Language`（支持 `q` 权重）> `?lang=`。结果写入上下文并通过 `Content-Language` 返回，同时输出 `Vary` 便于中间缓存按语言分桶；匹配不到或取值非法时回落到 `i18n.fallback`。

```bash
curl http://127.0.0.1:8080/api/v1/auth/profile
# {"code":"auth.token.missing","message":"缺少鉴权令牌，请在 Authorization 请求头中携带"}

curl http://127.0.0.1:8080/api/v1/auth/profile -H 'X-Lang: en'
# {"code":"auth.token.missing","message":"Missing credentials, please provide a token in the Authorization header"}
```

词条在 `internal/pkg/i18n/locales/{zh-CN,en-US}.yaml`，平铺的 `key: 模板` 映射，值支持 `%s` 占位符（按顺序对应 `APIError.Extra`）。新增语言只需加一个 `<locale>.yaml` 并写进 `i18n.support`，无需改代码。

覆盖范围：全部 `apperr` 错误码、参数校验（字段名取 `json` tag，前端可直接定位）、`404 / 405`。

```go
// 新增错误码：apperr 声明（Message 为中文兜底模板）+ 两个词条文件补同名 key
ErrOrderNotFound = APIError{Status: 404, Code: "order.not.found", Message: "订单不存在"}
```

- 词条缺失 / 未配置该语言 → 回落 `Message`，不会返回空文案。
- 译文占位符数量与 `Extra` 对不上 → 不强行 `Sprintf`，避免输出 `%!s(MISSING)` 这类脏数据。
- `WithMessage()` 是显式覆盖，会绕过词条查表。
- 完全关闭：`i18n.enabled: false`。

```go
// 业务代码里取当前语言
locale := contextx.Locale(c)
text   := i18n.Render(locale, "order.not.found", "订单不存在")
```

---

## 配置

### 多环境（profile 分层）

```
configs/config.yaml          基线，全环境共享
configs/config-{env}.yaml    环境差异，只写变化的键
APP_* 环境变量                最高优先级，CI/CD 与容器注入
代码默认值                    兜底
```

> **深合并，不是整体替换。** 实现用 `viper.MergeConfigMap`：环境文件里只写 `server.mode: release`，基线的 `server.host` / `server.port` 不会被清空。换成 `MergeConfig` 会整体替换掉整个 `server` 段 —— 最容易踩的坑。

### 环境名的确定顺序

| 顺序 | 来源 | 示例 |
|---|---|---|
| 1 | 启动参数 | `-e prod` / `--env=prod` |
| 2 | 环境变量 | `APP_ENV=prod` |
| 3 | 基线配置 | `config.yaml` 里的 `app.env` |
| 4 | 兜底 | `dev` |

```bash
go run ./cmd/server -e prod                          # 指定环境
APP_ENV=prod go run ./cmd/server                     # 环境变量指定
go run ./cmd/server -c configs/config.yaml -e prod   # 基线路径与环境正交
```

`-c` 只决定**基线文件在哪**，环境文件路径由基线路径推导（`configs/config.yaml` + `prod` → `configs/config-prod.yaml`）。

**基线或环境文件不存在都不算错误**（可完全依赖默认值 + 环境变量），但**存在却解析失败**会直接报错，避免带着半份配置启动。

> 判断「文件是否真的加载」不能用 `viper.ConfigFileUsed()`：`SetConfigFile` 会立刻记下路径，即使随后 `ReadInConfig` 因文件不存在而失败，它也照样返回那个路径。实现里改为看 `ReadInConfig` 的实际结果。

### 环境变量

前缀 `APP_`，层级用 `_` 连接：`app.env` → `APP_ENV`、`app.node_id` → `APP_NODE_ID`、`server.port` → `APP_SERVER_PORT`、`database.dsn` → `APP_DATABASE_DSN`、`jwt.secret` → `APP_JWT_SECRET`。

`app.env` 与 `app.node_id` 做了显式 `BindEnv`，否则 `AutomaticEnv` 会映射成别扭的 `APP_APP_ENV` / `APP_APP_NODE_ID`（后者仍可用且优先级更高）。

> **纯靠环境变量注入的键必须先在代码里登记。** viper 的 `Unmarshal` 只遍历「已知键」（默认值 + 配置文件里出现过的键）。`jwt.secret` 在 `setDefaults` 里被显式登记为 `""` —— 它没有可用默认值，但不登记的话 `APP_JWT_SECRET` 永远读不到，启动会以「密钥过短」失败。空值由 `Validate` 拦下，所以既支持纯环境变量注入，也不会静默用空密钥启动。

### 配置项一览

| 配置项 | 默认值 | 说明 |
|---|---|---|
| `app.env` | `dev` | 生效环境，由加载逻辑回填 |
| `app.node_id` | `-1` | 雪花节点号 0-1023；`-1` 表示按主机名自动推导 |
| `server.host` | `0.0.0.0` | 监听地址 |
| `server.port` | `8080` | 监听端口 |
| `server.mode` | `debug` | gin 模式 `debug/release/test` |
| `server.shutdown_timeout` | `10s` | 优雅退出等待时长 |
| `database.driver` | `mysql` | `mysql/postgres/sqlite`；dev profile 覆盖为 `sqlite` |
| `database.dsn` | MySQL 本地 DSN | **mysql 必须带 `parseTime=True`**，postgres 本地需 `sslmode=disable` |
| `database.auto_migrate` | `true` | 启动自动建表（生产建议关闭） |
| `jwt.secret` | 无（空） | **必须通过 `APP_JWT_SECRET` 覆盖**（长度 ≥ 16） |
| `jwt.access_ttl` / `jwt.refresh_ttl` | `2h` / `168h` | 访问令牌 / 刷新令牌有效期 |
| `i18n.enabled` | `true` | 关闭后全部文案使用代码内置中文兜底模板 |
| `i18n.fallback` | `zh-CN` | 语言协商失败时的兜底语言 |
| `i18n.support` | `[zh-CN, en-US]` | 支持的语言；留空则自动使用 locales 目录下发现的全部语言 |
| `i18n.alt_header` / `i18n.header` / `i18n.query` | `X-Lang` / `Accept-Language` / `lang` | 语言协商的三个来源 |
| `log.log_header` / `log.log_body` | `false` / `false` | 访问日志是否记录请求头响应头 / 请求体响应体 |
| `log.body_limit` | `4096` | 请求 / 响应体日志截断长度（字节） |
| `log.sensitive_keys` | `[]` | 在内置脱敏名单外追加的字段名 / 头名 |
| `rate_limit.rps` | `50` | 单 IP 每秒请求数 |
| `cors.allow_origins` | `["*"]` | 允许来源；开启 `allow_credentials` 时自动回显 Origin |
| `graphql.enabled` | `true` | 关闭后 `/graphql` 一条路由都不挂 |
| `graphql.path` | `/graphql` | 端点路径，必须以 `/` 开头（启动校验） |
| `graphql.playground` | `false` | 浏览器调试页（GraphiQL），dev 环境打开 |
| `graphql.introspection` | `false` | 是否允许拉取完整 schema，dev 环境打开 |

> 鉴权头不可配置，固定为 `Authorization`，故无 `auth.*` 配置项。
> `graphql.playground` 与 `graphql.introspection` 默认**关闭** —— 需要时由 `config-dev.yaml` 打开，
> 而不是默认放开再由生产去关。

```bash
APP_SERVER_PORT=9090 APP_JWT_SECRET=your-long-random-secret go run ./cmd/server
```

---

## 日志

`log.level` 控制最低级别，所有级别写 `logs/app.log`，`error` 及以上额外写 `logs/error.log`。hook 按**阈值**过滤（`minLevel` 遍历 `logrus.AllLevels`），不是集合精确匹配 —— 所以 `level: warn` 时 `error`/`fatal`/`panic` 同样会落盘。文件按 `max_size_mb` 轮转，保留 `max_backups` 份、`max_age_days` 天，可选 `compress`。

访问日志字段：基础字段（`request_id` / `client_ip` / `method` / `path` / `query` / `status` / `latency_ms` / `user_agent`）、`account_id`（鉴权通过后）、`request_headers` / `response_headers`、`request_body` / `response_body`（按 `body_limit` 截断）。状态码映射 `2xx/3xx → info`、`4xx → warn`、`5xx → error`；日志写在 `defer` 中，业务以 panic 抛错时同样落盘。

三个实现上的坑：

1. **`c.String` 的响应体不会被 `Write` 捕获。** gin 的 `responseWriter` 实现了 `io.StringWriter`，`WriteString` 直接写到下层、**不经过 `Write`**，所以 `bodyCaptureWriter` 必须显式实现 `WriteString`。
2. **读请求体不能顺手截断。** 不能把 `io.LimitReader` 直接挂到 `c.Request.Body`，那会把大请求体截断后交给业务代码。实现是「最多缓冲 1MiB，超出部分用 `io.MultiReader` 拼回 Body」。
3. **脱敏在写日志前完成，不是事后过滤。** `password` / `token` / `secret` / `authorization` / `cookie` / `set-cookie` / `x-api-key` 等内置名单命中即替换为 `****`；JSON 体递归打码，表单体按键打码，非法或被截断的 JSON 原样返回（宁可漏打码也不丢日志内容）。名单外字段通过 `log.sensitive_keys` 追加，匹配大小写不敏感且忽略 `-` 与 `_` 差异。

> 生产建议：`log_header` 体积小可按需长期开启；`log_body` 会记录业务数据，含 PII 的接口建议按环境关闭。

---

## 文档生成

```bash
go install github.com/swaggo/swag/cmd/swag@latest   # 一次性
make docs                                            # Windows: make.bat docs
```

生成后访问 `http://127.0.0.1:8080/swagger/index.html`。Swagger UI 已开启 `PersistAuthorization`，填一次令牌即可调试全部受保护接口。接口注释写在 handler 上，`@Security Authorization` 对应 `cmd/server/main.go` 中的 `@securityDefinitions.apikey Authorization`。

---

## 开发命令

```bash
make run              # 本地启动（默认 dev 环境）
make run ENV=test     # 指定环境启动
make run ENV=prod     # 生产启动（需先注入 APP_JWT_SECRET / APP_DATABASE_DSN / APP_NODE_ID）
make build            # 编译到 bin/
make docs             # 生成 Swagger 文档
make gqlgen           # 按 schema.graphqls 重新生成 GraphQL 代码（改完 schema 必跑）
make fmt vet test     # 格式化 + 静态检查 + 测试
make check            # 上面三件套
make docker           # 构建镜像
```

Windows 使用 `make.bat <target>`，环境用第二个参数：`make.bat run test`。

> `make gqlgen` 依赖 `tools.go` 里对 `github.com/99designs/gqlgen` 的空白 import。
> 别删它：`go mod tidy` 会把「没有任何包 import 的模块」及其 go.sum 条目剪掉，
> 之后 `go run github.com/99designs/gqlgen generate` 会因为缺 go.sum 条目直接失败。

---

## License

MIT
