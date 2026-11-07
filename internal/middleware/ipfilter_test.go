package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/config"
)

func newFilterEnv(t *testing.T, cfg config.IPFilterConfig, adminPath string) (*gin.Engine, *IPFilter) {
	t.Helper()
	f, err := NewIPFilter(cfg, adminPath)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(f.Middleware())
	r.GET("/api/search", func(c *gin.Context) { c.Status(200) })
	r.GET("/init/", func(c *gin.Context) { c.Status(200) })
	if adminPath != "" {
		r.GET("/"+adminPath+"/flags", func(c *gin.Context) { c.Status(200) })
	}
	return r, f
}

func req(r *gin.Engine, ip, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = ip + ":12345"
	r.ServeHTTP(w, req)
	return w
}

func TestIPFilterOff(t *testing.T) {
	r, _ := newFilterEnv(t, config.IPFilterConfig{Mode: "off", Scope: "all"}, "x7k2p9")
	if w := req(r, "8.8.8.8", "/api/search"); w.Code != 200 {
		t.Fatalf("off 模式不应拦截: %d", w.Code)
	}
}

func TestIPFilterAllowAPI(t *testing.T) {
	r, _ := newFilterEnv(t, config.IPFilterConfig{
		Mode: "allow", Scope: "api", Rules: []string{"10.0.0.0/8", "192.0.2.1"},
	}, "x7k2p9")

	if w := req(r, "10.1.2.3", "/api/search"); w.Code != 200 {
		t.Fatalf("名单内 CIDR 应放行: %d %s", w.Code, w.Body.String())
	}
	if w := req(r, "192.0.2.1", "/api/search"); w.Code != 200 {
		t.Fatalf("名单内裸 IP 应放行: %d", w.Code)
	}
	w := req(r, "8.8.8.8", "/api/search")
	if w.Code != 403 || !strings.Contains(w.Body.String(), "40301") {
		t.Fatalf("名单外访问 API 应 403 + 40301: %d %s", w.Code, w.Body.String())
	}
	// scope=api 时管理端不受 allow 名单约束（本测试未在名单内也一样放行）。
	if w := req(r, "8.8.8.8", "/x7k2p9/flags"); w.Code != 200 {
		t.Fatalf("scope=api 不应拦管理端: %d", w.Code)
	}
	// /init/ 永不受影响。
	if w := req(r, "8.8.8.8", "/init/"); w.Code != 200 {
		t.Fatalf("/init/ 应豁免: %d", w.Code)
	}
}

func TestIPFilterAllowAdminScope(t *testing.T) {
	r, _ := newFilterEnv(t, config.IPFilterConfig{
		Mode: "allow", Scope: "admin", Rules: []string{"10.0.0.0/8"},
	}, "x7k2p9")

	// 名单外访问管理端：伪装 404。
	if w := req(r, "8.8.8.8", "/x7k2p9/flags"); w.Code != 404 {
		t.Fatalf("管理端名单外应 404: %d", w.Code)
	}
	// 名单内正常。
	if w := req(r, "10.0.0.5", "/x7k2p9/flags"); w.Code != 200 {
		t.Fatalf("管理端名单内应放行: %d", w.Code)
	}
	// API 不受 admin scope 影响。
	if w := req(r, "8.8.8.8", "/api/search"); w.Code != 200 {
		t.Fatalf("scope=admin 不应拦 API: %d", w.Code)
	}
}

func TestIPFilterBlock(t *testing.T) {
	r, _ := newFilterEnv(t, config.IPFilterConfig{
		Mode: "block", Scope: "all", Rules: []string{"10.0.0.0/8"},
	}, "x7k2p9")

	if w := req(r, "10.9.9.9", "/api/search"); w.Code != 403 {
		t.Fatalf("block 命中应 403: %d", w.Code)
	}
	if w := req(r, "10.9.9.9", "/init/"); w.Code != 200 {
		t.Fatalf("block 模式 /init/ 仍豁免: %d", w.Code)
	}
	if w := req(r, "8.8.8.8", "/api/search"); w.Code != 200 {
		t.Fatalf("block 未命中应放行: %d", w.Code)
	}
}

func TestIPFilterHotUpdate(t *testing.T) {
	r, f := newFilterEnv(t, config.IPFilterConfig{Mode: "off", Scope: "all"}, "x7k2p9")
	if w := req(r, "8.8.8.8", "/api/search"); w.Code != 200 {
		t.Fatal("初始 off 应放行")
	}
	// 热更新为 allow，且 8.8.8.8 不在名单。
	rules, err := ParseIPRules([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	f.Update("allow", "api", rules)
	if w := req(r, "8.8.8.8", "/api/search"); w.Code != 403 {
		t.Fatalf("热更后应拦截: %d", w.Code)
	}
	mode, scope, got := f.Snapshot()
	if mode != "allow" || scope != "api" || len(got) != 1 {
		t.Fatalf("Snapshot 不符: %s %s %v", mode, scope, got)
	}
}

func TestParseIPRulesErrors(t *testing.T) {
	if _, err := ParseIPRules([]string{"not-an-ip"}); err == nil {
		t.Fatal("非法规则应报错")
	}
	many := make([]string, 201)
	for i := range many {
		many[i] = "10.0.0.1"
	}
	if _, err := ParseIPRules(many); err == nil {
		t.Fatal("超过 200 条应报错")
	}
}
