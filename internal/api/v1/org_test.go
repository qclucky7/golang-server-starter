package v1_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang-server-starter/internal/api"
	"golang-server-starter/internal/config"
	"golang-server-starter/internal/model"
	"golang-server-starter/internal/pkg/request"
	"golang-server-starter/internal/pkg/snowflake"
	"golang-server-starter/internal/pkg/token"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// registerValidationOnce 自定义校验规则（username / password）注册在 gin 的全局
// validator 上，重复注册会报错，所以整个测试进程只注册一次 ——
// 真实启动路径里由 bootstrap.Container.New 负责这一次调用。
var registerValidationOnce sync.Once

// newTestEngine 装配一个接内存 sqlite 的完整引擎（含中间件与全部路由）。
//
// 用 api.NewRouter 而不是手工拼 engine，是为了让测试覆盖真实装配路径：
// 中间件顺序、路由前缀、鉴权分组一旦被改坏，这里会直接失败。
func newTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	registerValidationOnce.Do(func() {
		if err := request.RegisterValidation(); err != nil {
			t.Fatalf("注册自定义校验规则失败: %v", err)
		}
	})

	if err := snowflake.Setup(snowflake.AutoNodeID); err != nil {
		t.Fatalf("初始化雪花 ID 失败: %v", err)
	}

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	cfg := &config.Config{}
	cfg.App.Name = "test"
	cfg.App.Env = "test"
	cfg.Server.Mode = gin.TestMode
	cfg.JWT.Secret = "test-secret-key-at-least-16-chars"

	return api.NewRouter(api.Dependencies{
		Config: cfg,
		Logger: logger,
		DB:     db,
		Token:  token.NewManager(cfg.JWT.Secret, "test", time.Hour, 24*time.Hour),
	})
}

// doJSON 发一个请求，返回响应记录。token 非空时按标准 Authorization 头携带。
func doJSON(t *testing.T, engine *gin.Engine, method, path, accessToken, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader = http.NoBody
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

// loginAsDemo 注册并登录 demo 账号，返回 access_token。
func loginAsDemo(t *testing.T, engine *gin.Engine) string {
	t.Helper()

	if rec := doJSON(t, engine, http.MethodPost, "/api/v1/auth/register", "",
		`{"username":"demo","email":"demo@example.com","password":"demo1234"}`); rec.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", rec.Code, rec.Body.String())
	}

	rec := doJSON(t, engine, http.MethodPost, "/api/v1/auth/login", "",
		`{"account":"demo","password":"demo1234"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("登录失败: %d %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data struct {
			Token struct {
				AccessToken string `json:"access_token"`
			} `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}
	if resp.Data.Token.AccessToken == "" {
		t.Fatalf("未取到 access_token: %s", rec.Body.String())
	}
	return resp.Data.Token.AccessToken
}

// TestOrgListEndpoint 分页查询接口的端到端形状：
// 鉴权生效、响应是 grid.result 契约、datas 里是当前账号自己的组织。
func TestOrgListEndpoint(t *testing.T) {
	engine := newTestEngine(t)
	accessToken := loginAsDemo(t, engine)

	rec := doJSON(t, engine, http.MethodGet, "/api/v1/orgs?current=1&size=10", accessToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("查询组织列表失败: %d %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Model string `json:"model"`
		Page  struct {
			Current   int `json:"current"`
			Size      int `json:"size"`
			Total     int `json:"total"`
			TotalPage int `json:"total_page"`
		} `json:"page"`
		Result struct {
			Model string `json:"model"`
			Total int    `json:"total"`
			Datas []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				IsDefault bool   `json:"is_default"`
			} `json:"datas"`
		} `json:"result"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v (%s)", err, rec.Body.String())
	}

	if body.Model != "grid.result" || body.Result.Model != "data.set" {
		t.Fatalf("响应契约不符: %s", rec.Body.String())
	}
	if body.Page.Total != 1 || body.Result.Total != 1 {
		t.Fatalf("注册后应只有 1 个组织，实际 page.total=%d result.total=%d", body.Page.Total, body.Result.Total)
	}
	if len(body.Result.Datas) != 1 {
		t.Fatalf("datas 应含 1 条，实际 %d", len(body.Result.Datas))
	}
	if !body.Result.Datas[0].IsDefault || body.Result.Datas[0].Name != "demo的组织" {
		t.Fatalf("组织内容不符: %+v", body.Result.Datas[0])
	}
	if body.RequestID == "" {
		t.Fatal("响应应带 request_id")
	}
}

// TestOrgListRequiresAuth 未带令牌必须被拦下，且返回统一错误信封。
func TestOrgListRequiresAuth(t *testing.T) {
	engine := newTestEngine(t)

	rec := doJSON(t, engine, http.MethodGet, "/api/v1/orgs", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未鉴权应返回 401，实际 %d %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Model  string `json:"model"`
		Errors struct {
			Datas []struct {
				Code string `json:"code"`
			} `json:"datas"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析错误响应失败: %v", err)
	}
	if body.Model != "errors" || len(body.Errors.Datas) == 0 || body.Errors.Datas[0].Code != "auth.token.missing" {
		t.Fatalf("错误信封不符: %s", rec.Body.String())
	}
}
