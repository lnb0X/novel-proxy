package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/config"
)

func newRateEnv(t *testing.T, cfg config.RateLimitConfig) (*gin.Engine, *RateLimiter) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rl := NewRateLimiter(cfg, "")
	t.Cleanup(rl.Stop)
	r := gin.New()
	r.Use(rl.Middleware())
	r.GET("/api/search", func(c *gin.Context) { c.Status(200) })
	r.GET("/api/detail", func(c *gin.Context) { c.Status(200) })
	return r, rl
}

func getAs(r *gin.Engine, ip, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = ip + ":1234"
	r.ServeHTTP(w, req)
	return w
}

func TestRateLimitOff(t *testing.T) {
	r, _ := newRateEnv(t, config.RateLimitConfig{})
	for i := 0; i < 10; i++ {
		if w := getAs(r, "1.1.1.1", "/api/search"); w.Code != 200 {
			t.Fatalf("off 模式不应限速: %d", w.Code)
		}
	}
}

func TestRateLimitGlobal(t *testing.T) {
	r, _ := newRateEnv(t, config.RateLimitConfig{
		Search: config.RateLimitRule{Mode: "global", PerMinute: 2, Burst: 2},
	})
	// 前两次放行，第三次 429 + Retry-After。
	if w := getAs(r, "1.1.1.1", "/api/search"); w.Code != 200 {
		t.Fatalf("第 1 次: %d", w.Code)
	}
	if w := getAs(r, "1.1.1.1", "/api/search"); w.Code != 200 {
		t.Fatalf("第 2 次: %d", w.Code)
	}
	w := getAs(r, "1.1.1.1", "/api/search")
	if w.Code != http.StatusTooManyRequests || !containsSub(w.Body.String(), "42901") {
		t.Fatalf("第 3 次应 429 + 42901: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("429 应带 Retry-After")
	}
	// 其他接口不受影响。
	if w := getAs(r, "1.1.1.1", "/api/detail"); w.Code != 200 {
		t.Fatalf("detail 不应被连带限速: %d", w.Code)
	}
}

func TestRateLimitPerIP(t *testing.T) {
	r, _ := newRateEnv(t, config.RateLimitConfig{
		Search: config.RateLimitRule{Mode: "per_ip", PerMinute: 2, Burst: 2},
	})
	for i := 0; i < 2; i++ {
		if w := getAs(r, "1.1.1.1", "/api/search"); w.Code != 200 {
			t.Fatalf("IP1 第 %d 次: %d", i+1, w.Code)
		}
	}
	if w := getAs(r, "1.1.1.1", "/api/search"); w.Code != 429 {
		t.Fatal("IP1 第 3 次应 429")
	}
	// 另一个来源不受影响。
	if w := getAs(r, "2.2.2.2", "/api/search"); w.Code != 200 {
		t.Fatalf("IP2 应不受 IP1 影响: %d", w.Code)
	}
}

func TestRateLimitUpdateHot(t *testing.T) {
	r, rl := newRateEnv(t, config.RateLimitConfig{})
	if w := getAs(r, "1.1.1.1", "/api/search"); w.Code != 200 {
		t.Fatal("初始不限速")
	}
	rl.Update(config.RateLimitConfig{
		Search: config.RateLimitRule{Mode: "global", PerMinute: 1, Burst: 1},
	})
	if w := getAs(r, "1.1.1.1", "/api/search"); w.Code != 200 {
		t.Fatal("更新后第 1 次放行")
	}
	if w := getAs(r, "1.1.1.1", "/api/search"); w.Code != 429 {
		t.Fatal("更新后第 2 次应 429")
	}
	if s := rl.Snapshot(); s.Search.PerMinute != 1 {
		t.Fatalf("Snapshot 未更新: %+v", s)
	}
}

func TestRateLimitIPMapCap(t *testing.T) {
	r, _ := newRateEnv(t, config.RateLimitConfig{
		Search: config.RateLimitRule{Mode: "per_ip", PerMinute: 1, Burst: 1},
	})
	// 灌满表。
	for i := 0; i < ipMapMax; i++ {
		getAs(r, "10.0."+string(rune('0'+i%26))+"."+itoa2(i%250), "/api/search")
	}
	// 新 IP 走兜底桶：第 2 次同样被限。
	if w := getAs(r, "9.9.9.9", "/api/search"); w.Code != 200 {
		t.Fatalf("溢出 IP 第 1 次应放行: %d", w.Code)
	}
	if w := getAs(r, "9.9.9.9", "/api/search"); w.Code != 429 {
		t.Fatalf("溢出 IP 第 2 次应按兜底限速: %d", w.Code)
	}
}

func TestRateLimitJanitorRemovesIdle(t *testing.T) {
	oldScan, oldTTL := ipScanInterval, ipIdleTTL
	ipScanInterval = 20 * time.Millisecond
	ipIdleTTL = 30 * time.Millisecond
	defer func() { ipScanInterval, ipIdleTTL = oldScan, oldTTL }()

	r, rl := newRateEnv(t, config.RateLimitConfig{
		Search: config.RateLimitRule{Mode: "per_ip", PerMinute: 100, Burst: 100},
	})
	getAs(r, "1.1.1.1", "/api/search")
	rl.mu.Lock()
	n := len(rl.buckets["search"].ips)
	rl.mu.Unlock()
	if n != 1 {
		t.Fatalf("应有 1 条: %d", n)
	}
	time.Sleep(80 * time.Millisecond)
	rl.mu.Lock()
	n = len(rl.buckets["search"].ips)
	rl.mu.Unlock()
	if n != 0 {
		t.Fatalf("空闲条目应被回收: %d", n)
	}
}

func TestAdminFixedLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rl := NewRateLimiter(config.RateLimitConfig{}, "adm")
	defer rl.Stop()
	r := gin.New()
	r.Use(rl.Middleware())
	r.POST("/adm/api/x", func(c *gin.Context) { c.Status(200) })
	// 固定限额 300/min：连打 60（burst）次之内不应 429。
	for i := 0; i < 60; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/adm/api/x", nil))
		if w.Code != 200 {
			t.Fatalf("管理端第 %d 次不应 429: %d", i+1, w.Code)
		}
	}
}

func itoa2(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for ; i > 0; i /= 10 {
		s = string(rune('0'+i%10)) + s
	}
	return s
}
