package middleware

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
)

// per_ip 条目的清理参数（包级变量便于测试下调）。
var (
	ipScanInterval = 5 * time.Minute
	ipIdleTTL      = 10 * time.Minute
	// per_ip map 上限：超过后新 IP 按全局限流处理（DECISIONS 限速节）。
	ipMapMax = 10000
	// 管理端 API 的固定内部限额，不进配置界面。
	adminPerMinute = 300
	adminBurst     = 60
)

type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type apiBucket struct {
	rule   config.RateLimitRule
	global *rate.Limiter
	fallbk *rate.Limiter // per_ip 表满时的兜底，与全局同参数
	ips    map[string]*ipEntry
}

// RateLimiter 四个公开接口的令牌桶限速 + 管理端固定内部限额。
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*apiBucket
	admin    *rate.Limiter
	adminPfx string
	stop     chan struct{}
}

func newBucket(rule config.RateLimitRule) *apiBucket {
	b := &apiBucket{rule: rule, ips: map[string]*ipEntry{}}
	b.rebuildLocked()
	return b
}

// rebuildLocked 按 rule 重建桶。per_minute<=0 视为不限速（恒放行）；
// burst 未配置（<=0）时取 per_minute 本身。
func (b *apiBucket) rebuildLocked() {
	perMinute := float64(b.rule.PerMinute)
	if b.rule.PerMinute <= 0 {
		b.global = nil
		b.fallbk = nil
		return
	}
	burst := b.rule.Burst
	if burst <= 0 {
		burst = b.rule.PerMinute
	}
	b.global = rate.NewLimiter(rate.Limit(perMinute/60.0), burst)
	b.fallbk = rate.NewLimiter(rate.Limit(perMinute/60.0), burst)
}

// NewRateLimiter 构建限速器并启动 per_ip 条目清理（每 5 分钟扫一次，
// 10 分钟无访问删除）。
func NewRateLimiter(cfg config.RateLimitConfig, adminPrefix string) *RateLimiter {
	rl := &RateLimiter{
		buckets: map[string]*apiBucket{
			"search":  newBucket(cfg.Search),
			"detail":  newBucket(cfg.Detail),
			"catalog": newBucket(cfg.Catalog),
			"content": newBucket(cfg.Content),
		},
		admin:    rate.NewLimiter(rate.Limit(float64(adminPerMinute)/60.0), adminBurst),
		adminPfx: adminPrefix,
		stop:     make(chan struct{}),
	}
	go rl.janitor()
	return rl
}

func (rl *RateLimiter) janitor() {
	t := time.NewTicker(ipScanInterval)
	defer t.Stop()
	for {
		select {
		case <-rl.stop:
			return
		case now := <-t.C:
			rl.mu.Lock()
			for _, b := range rl.buckets {
				for ip, e := range b.ips {
					if now.Sub(e.lastSeen) > ipIdleTTL {
						delete(b.ips, ip)
					}
				}
			}
			rl.mu.Unlock()
		}
	}
}

// Stop 停止清理协程（测试用）。
func (rl *RateLimiter) Stop() {
	close(rl.stop)
}

// Update 整体替换四个接口的限速配置并重建桶（已存 IP 条目清空）。
func (rl *RateLimiter) Update(cfg config.RateLimitConfig) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for name, rule := range map[string]config.RateLimitRule{
		"search": cfg.Search, "detail": cfg.Detail,
		"catalog": cfg.Catalog, "content": cfg.Content,
	} {
		b := rl.buckets[name]
		b.rule = rule
		b.ips = map[string]*ipEntry{}
		b.rebuildLocked()
	}
}

// Snapshot 供管理接口读取。
func (rl *RateLimiter) Snapshot() config.RateLimitConfig {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	var out config.RateLimitConfig
	out.Search = rl.buckets["search"].rule
	out.Detail = rl.buckets["detail"].rule
	out.Catalog = rl.buckets["catalog"].rule
	out.Content = rl.buckets["content"].rule
	return out
}

// Middleware 固定链最后一环。超限返回 429 + 42901 + Retry-After。
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if rl.adminPfx != "" && strings.HasPrefix(p, rl.adminPfx+"/api/") {
			if !rl.admin.Allow() {
				retryAfter(c, rl.admin)
				return
			}
			c.Next()
			return
		}
		name := strings.TrimPrefix(p, "/api/")
		rl.mu.Lock()
		b := rl.buckets[name]
		rl.mu.Unlock()
		if b == nil {
			c.Next()
			return
		}
		rl.mu.Lock()
		rule := b.rule
		var lim *rate.Limiter
		switch {
		case rule.Mode == "off" || rule.PerMinute <= 0:
		case rule.Mode == "global":
			lim = b.global
		case rule.Mode == "per_ip":
			ip := ClientIP(c.Request)
			if e, ok := b.ips[ip]; ok {
				e.lastSeen = time.Now()
				lim = e.limiter
			} else if len(b.ips) >= ipMapMax {
				lim = b.fallbk
			} else {
				perMinute := float64(rule.PerMinute)
				burst := rule.Burst
				if burst <= 0 {
					burst = rule.PerMinute
				}
				e := &ipEntry{
					limiter:  rate.NewLimiter(rate.Limit(perMinute/60.0), burst),
					lastSeen: time.Now(),
				}
				b.ips[ip] = e
				lim = e.limiter
			}
		}
		rl.mu.Unlock()

		if lim == nil || lim.Allow() {
			c.Next()
			return
		}
		retryAfter(c, lim)
	}
}

func retryAfter(c *gin.Context, lim *rate.Limiter) {
	res := lim.Reserve()
	delay := res.Delay()
	res.Cancel()
	seconds := int(math.Ceil(delay.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", strconv.Itoa(seconds))
	api.Fail(c, api.CodeTooManyRequests, "请求过于频繁")
}
