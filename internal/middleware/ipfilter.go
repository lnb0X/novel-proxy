package middleware

import (
	"fmt"
	"net/netip"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/frontend"
)

const ipRulesMax = 200

// IPFilter 管理 IP 名单：mode off / allow / block，scope all / api / admin，
// 规则支持 CIDR 与裸 IP。/init/* 永不受影响。
type IPFilter struct {
	mu        sync.RWMutex
	mode      string
	scope     string
	rules     []netip.Prefix
	adminPath string
}

// NewIPFilter 构建并解析规则；非法 CIDR 报错（启动时直接退出）。
func NewIPFilter(cfg config.IPFilterConfig, adminPath string) (*IPFilter, error) {
	rules, err := ParseIPRules(cfg.Rules)
	if err != nil {
		return nil, err
	}
	return &IPFilter{
		mode:      cfg.Mode,
		scope:     cfg.Scope,
		rules:     rules,
		adminPath: adminPath,
	}, nil
}

// ParseIPRules 解析 CIDR 或裸 IP 列表，最多 200 条。
func ParseIPRules(rules []string) ([]netip.Prefix, error) {
	if len(rules) > ipRulesMax {
		return nil, fmt.Errorf("IP 名单最多 %d 条", ipRulesMax)
	}
	out := make([]netip.Prefix, 0, len(rules))
	for _, r := range rules {
		p, err := netip.ParsePrefix(r)
		if err != nil {
			addr, aerr := netip.ParseAddr(r)
			if aerr != nil {
				return nil, fmt.Errorf("非法 IP 规则 %q: %w", r, err)
			}
			p = netip.PrefixFrom(addr, addr.BitLen())
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// Update 热更新规则（已由调用方校验）。
func (f *IPFilter) Update(mode, scope string, rules []netip.Prefix) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
	f.scope = scope
	f.rules = rules
}

// Snapshot 返回当前配置，供管理接口读取。
func (f *IPFilter) Snapshot() (mode, scope string, rules []netip.Prefix) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]netip.Prefix, len(f.rules))
	copy(out, f.rules)
	return f.mode, f.scope, out
}

// Middleware 固定链中位于 recover 之后、auth 之前。
// 公开 API 被拒返回 403 + 40301；管理端路径被拒返回 404（固定 HTML 页）。
func (f *IPFilter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		f.mu.RLock()
		mode, scope, rules := f.mode, f.scope, f.rules
		f.mu.RUnlock()

		if mode == "off" {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		// /init/* 永不受影响。
		if path == "/init" || strings.HasPrefix(path, "/init/") {
			c.Next()
			return
		}

		isAPI := strings.HasPrefix(path, "/api/")
		adminPrefix := "/" + f.adminPath
		isAdmin := f.adminPath != "" &&
			(path == adminPrefix || strings.HasPrefix(path, adminPrefix+"/"))
		switch scope {
		case "api":
			if !isAPI {
				c.Next()
				return
			}
		case "admin":
			if !isAdmin {
				c.Next()
				return
			}
		}

		addr, err := netip.ParseAddr(ClientIP(c.Request))
		matched := false
		if err == nil {
			for _, p := range rules {
				if p.Contains(addr) {
					matched = true
					break
				}
			}
		}

		reject := (mode == "allow" && !matched) || (mode == "block" && matched)
		if !reject {
			c.Next()
			return
		}
		if isAdmin {
			// 管理端被拒伪装成 404，不暴露路径存在性。
			frontend.NotFound(c.Writer)
			return
		}
		api.Fail(c, api.CodeIPRejected, "IP 被拒绝访问")
	}
}
