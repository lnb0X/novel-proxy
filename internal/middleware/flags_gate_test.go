package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/config"
)

func TestFlagsGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := NewFlags(config.FlagsConfig{Search: false, Detail: true, Catalog: true, Content: true})
	r := gin.New()
	r.Use(f.Middleware())
	r.GET("/api/search", func(c *gin.Context) { c.Status(200) })
	r.GET("/api/detail", func(c *gin.Context) { c.Status(200) })
	r.GET("/api/other", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/search", nil))
	if w.Code != http.StatusServiceUnavailable || !containsSub(w.Body.String(), "50301") {
		t.Fatalf("关闭的接口应 503 + 50301: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/detail", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("开启的接口应放行: %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/other", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("未知接口不拦: %d", w.Code)
	}
	// 热更。
	if !f.Set("search", true) {
		t.Fatal("合法开关名应设置成功")
	}
	if f.Set("nope", true) {
		t.Fatal("非法开关名应失败")
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/search", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("热更后应放行: %d", w.Code)
	}
}
