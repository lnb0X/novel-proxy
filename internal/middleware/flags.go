package middleware

import (
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
)

// Flags 四个公开接口的开关，热更即时生效。
type Flags struct {
	mu sync.RWMutex
	v  map[string]bool
}

func NewFlags(cfg config.FlagsConfig) *Flags {
	return &Flags{v: map[string]bool{
		"search":  cfg.Search,
		"detail":  cfg.Detail,
		"catalog": cfg.Catalog,
		"content": cfg.Content,
	}}
}

// Set 更新单个开关，name 非法返回 false。
func (f *Flags) Set(name string, enabled bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.v[name]; !ok {
		return false
	}
	f.v[name] = enabled
	return true
}

func (f *Flags) Snapshot() map[string]bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make(map[string]bool, len(f.v))
	for k, v := range f.v {
		out[k] = v
	}
	return out
}

// Middleware 关闭的接口返回 503 + 50301，只作用于 /api/<name> 本段。
func (f *Flags) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		name := strings.TrimPrefix(c.Request.URL.Path, "/api/")
		f.mu.RLock()
		enabled, known := f.v[name]
		f.mu.RUnlock()
		if !known || enabled {
			c.Next()
			return
		}
		api.Fail(c, api.CodeDisabled, "接口已关闭")
	}
}
