package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/middleware"
	"novel-proxy/internal/stats"
	"novel-proxy/internal/store"
)

func TestStatsDisabledReturns50301(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/adm/api/stats/overview", RequireStats(nil), StatsOverview(nil, nil, time.Now()))
	r.GET("/adm/api/stats/query", RequireStats(nil), StatsQuery(nil))
	r.GET("/adm/api/audit", RequireStats(nil), AuditList(nil))
	r.GET("/adm/api/cache", RequireStats(nil), CacheList(nil, nil))

	for _, p := range []string{
		"/adm/api/stats/overview", "/adm/api/stats/query", "/adm/api/audit", "/adm/api/cache",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "统计与审计需要开启文章缓存") {
			t.Fatalf("%s = %d %s", p, w.Code, w.Body.String())
		}
	}
}

func TestStatsOverviewCountsRequests(t *testing.T) {
	dir := t.TempDir()
	ss, err := stats.New("sqlite", filepath.Join(dir, "stats.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ss.Close)
	counters := store.Count(nil)

	gin.SetMode(gin.TestMode)
	lg := slog.New(slog.NewJSONHandler(io.Discard, nil))
	r := gin.New()
	r.Use(api.SlogMiddleware(lg, ""), ss.Middleware())
	r.GET("/adm/api/ping", func(c *gin.Context) { c.Status(200) })
	r.GET("/adm/api/stats/overview", RequireStats(ss), StatsOverview(ss, counters, time.Now()))

	// 发 10 次请求（每次都会被统计钩子记录）。
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/adm/api/ping", nil))
		if w.Code != 200 {
			t.Fatalf("ping = %d", w.Code)
		}
	}
	// 异步写入有延迟，轮询直到概览计数达到 10。
	deadline := time.Now().Add(3 * time.Second)
	for {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/adm/api/stats/overview", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("overview = %d %s", w.Code, w.Body.String())
		}
		var resp struct {
			Data struct {
				TodayRequests  int64  `json:"today_requests"`
				UpstreamHealth string `json:"upstream_health"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Data.TodayRequests >= 10 {
			if resp.Data.UpstreamHealth != "ok" {
				t.Fatalf("upstream_health = %s", resp.Data.UpstreamHealth)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("today_requests 停在 %d, want 10", resp.Data.TodayRequests)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAuditViaEndpoints(t *testing.T) {
	dir := t.TempDir()
	hash, _ := middleware.HashPassword("Abcdef1!Abcde")
	cfg := config.Config{}
	cfg.Server.AdminPath = testAdminPath
	cfg.Admin.Username = "boss"
	cfg.Admin.PasswordHash = hash
	cfg.Flags = config.FlagsConfig{Search: true, Detail: true, Catalog: true, Content: true}

	ss, err := stats.New("sqlite", filepath.Join(dir, "stats.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ss.Close)
	instancePath := filepath.Join(dir, "instance.yaml")

	gin.SetMode(gin.TestMode)
	auth := middleware.NewAuthManager(testAdminPath)
	lg := slog.New(slog.NewJSONHandler(io.Discard, nil))
	r := gin.New()
	r.Use(api.SlogMiddleware(lg, testAdminPath))
	r.POST("/"+testAdminPath+"/api/auth/login", Login(auth, &cfg, ss))
	r.PUT("/"+testAdminPath+"/api/flags/:name", PutFlag(middleware.NewFlags(cfg.Flags), &cfg, instancePath, ss))
	r.GET("/"+testAdminPath+"/api/audit", RequireStats(ss), AuditList(ss))

	// 一次登录 + 一次开关变更 → 审计两条。
	token := loginAndGetToken(t, r)
	w := putAdmin(r, "/"+testAdminPath+"/api/flags/search", `{"enabled":false}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT flags = %d %s", w.Code, w.Body.String())
	}
	time.Sleep(200 * time.Millisecond) // 异步写入

	w = getAdmin(r, "/"+testAdminPath+"/api/audit?page=1&size=50", token)
	if w.Code != http.StatusOK {
		t.Fatalf("audit = %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Total int64 `json:"total"`
			Items []struct {
				Event string `json:"event"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Total != 2 {
		t.Fatalf("审计应为两条, got %d: %s", resp.Data.Total, w.Body.String())
	}
	events := map[string]bool{}
	for _, it := range resp.Data.Items {
		events[it.Event] = true
	}
	if !events["login_success"] || !events["flag_change"] {
		t.Fatalf("缺少预期事件: %v", events)
	}
}
