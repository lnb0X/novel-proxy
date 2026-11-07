package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/frontend"
)

// InitGuard 未初始化门禁，固定链中位于 ipfilter 之后、auth 之前。
// 未完成：非 /init 路径 302 到 /init/，/api/* 返回 503 + 50302；
// 已完成：/init/* 全部 404（api 走 JSON，页面走固定 HTML）。
type InitGuard struct {
	completed func() bool
}

func NewInitGuard(completed func() bool) *InitGuard {
	return &InitGuard{completed: completed}
}

func (g *InitGuard) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		isInit := p == "/init" || strings.HasPrefix(p, "/init/")
		if g.completed() {
			if !isInit {
				c.Next()
				return
			}
			if strings.HasPrefix(p, "/init/api/") {
				api.Fail(c, api.CodeNotFound, "路由不存在")
				return
			}
			frontend.NotFound(c.Writer)
			c.Abort()
			return
		}
		if isInit {
			c.Next()
			return
		}
		if strings.HasPrefix(p, "/api/") {
			api.Fail(c, api.CodeNotInitialized, "服务尚未初始化，请先访问 /init/")
			return
		}
		// 302 相对路径，兼容任意挂载前缀；Redirect 不中断链路，必须 Abort。
		c.Redirect(http.StatusFound, "/init/")
		c.Abort()
	}
}
