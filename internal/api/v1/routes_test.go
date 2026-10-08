package v1_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "gin-quick-start/internal/api/v1"
	"gin-quick-start/internal/config"

	"github.com/gin-gonic/gin"
)

// TestNoAdminAccountRoutes 管理端账号 CRUD 不应存在。
//
// 列表 / 创建 / 更新 / 删除他人账号需要独立的权限模型（谁能管谁、管哪个组织），
// 本模板只提供「认证 + 账号自服务」两类接口，且所有受保护接口都只操作
// 当前登录账号自己、不接收目标账号 ID。
//
// 这个测试是防回归用的：如果有人把 admin CRUD 加回来，它会失败。
func TestNoAdminAccountRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	// 只关心路由表，不实际调用处理器，因此依赖留空
	v1.Register(engine, v1.Dependencies{Config: &config.Config{}})

	registered := make(map[string]bool, len(engine.Routes()))
	for _, r := range engine.Routes() {
		registered[r.Method+" "+r.Path] = true
		if strings.HasPrefix(r.Path, "/api/v1/accounts") {
			t.Errorf("不应存在管理端账号接口: %s %s", r.Method, r.Path)
		}
	}

	// 自服务接口必须在
	want := []string{
		"GET /healthz",
		"GET /api/v1/system/health-check",
		"POST /api/v1/auth/register",
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/refresh",
		"POST /api/v1/auth/logout",
		"GET /api/v1/auth/profile",
		"PUT /api/v1/auth/password",
	}
	for _, w := range want {
		if !registered[w] {
			t.Errorf("缺少路由: %s", w)
		}
	}
	if got := len(engine.Routes()); got != len(want) {
		t.Errorf("路由数量应为 %d，实际 %d（新增了未预期的接口？）", len(want), got)
	}
}

// TestRemovedAccountRoutesAre404 确认旧的管理端路径是真的不存在，
// 而不是「存在但被鉴权中间件拦下」—— 后者会让人误以为接口还在。
func TestRemovedAccountRoutesAre404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	v1.Register(engine, v1.Dependencies{Config: &config.Config{}})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/accounts"},
		{http.MethodPost, "/api/v1/accounts"},
		{http.MethodGet, "/api/v1/accounts/1"},
		{http.MethodPut, "/api/v1/accounts/1"},
		{http.MethodDelete, "/api/v1/accounts/1"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s 应返回 404，实际 %d", tc.method, tc.path, rec.Code)
		}
	}
}
