package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newGuardEnv(completed bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	g := NewInitGuard(func() bool { return completed })
	r := gin.New()
	r.Use(g.Middleware())
	r.GET("/api/search", func(c *gin.Context) { c.Status(200) })
	r.GET("/init/", func(c *gin.Context) { c.Status(200) })
	r.GET("/init/api/state", func(c *gin.Context) { c.Status(200) })
	r.GET("/", func(c *gin.Context) { c.Status(200) })
	return r
}

func TestInitGuardNotCompleted(t *testing.T) {
	r := newGuardEnv(false)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/init/" {
		t.Fatalf("未初始化访问 / 应 302 到 /init/: %d %q", w.Code, w.Header().Get("Location"))
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/search", nil))
	if w.Code != http.StatusServiceUnavailable || !containsSub(w.Body.String(), "50302") {
		t.Fatalf("未初始化 API 应 503 + 50302: %d %s", w.Code, w.Body.String())
	}

	// /init/* 放行。
	for _, p := range []string{"/init/", "/init/api/state"} {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s 应放行: %d", p, w.Code)
		}
	}
}

func TestInitGuardCompleted(t *testing.T) {
	r := newGuardEnv(true)

	// /init/* 全部 404。
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/init/", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("完成后 /init/ 应 404: %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/init/api/state", nil))
	if w.Code != http.StatusNotFound || !containsSub(w.Body.String(), "40400") {
		t.Fatalf("完成后 /init/api/* 应 JSON 404: %d %s", w.Code, w.Body.String())
	}

	// 其余路径正常。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("完成后 / 不应拦截: %d", w.Code)
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
