package handler

import (
	"net/http"
	"net/netip"
	"strconv"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/middleware"
	"novel-proxy/internal/stats"
)

// Login POST /<secret>/api/auth/login
func Login(auth *middleware.AuthManager, cfg *config.Config, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		ip := middleware.ClientIP(c.Request)
		if wait, ok := auth.LoginAllowed(ip, body.Username); !ok {
			au.Audit("login_fail", body.Username, ip, c.Request.UserAgent(), "locked", "rate_limited")
			seconds := int(wait.Seconds())
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			api.Fail(c, api.CodeTooManyRequests, "登录尝试过于频繁")
			return
		}
		if body.Username != cfg.Admin.Username || !middleware.CheckPassword(cfg.Admin.PasswordHash, body.Password) {
			auth.LoginFailure(ip, body.Username)
			au.Audit("login_fail", body.Username, ip, c.Request.UserAgent(), "password_mismatch", "fail")
			api.Fail(c, api.CodeUnauthorized, "账号或密码错误")
			return
		}
		auth.LoginSuccess(ip, body.Username)
		au.Audit("login_success", body.Username, ip, c.Request.UserAgent(), "", "ok")
		token, exp, sat := auth.Issue(body.Username)
		http.SetCookie(c.Writer, auth.AuthCookie(c.Request, token))
		api.OK(c, gin.H{
			"username":   body.Username,
			"expires_at": exp.Unix(),
			"sat":        sat.Unix(),
		})
	}
}

// Logout POST /<secret>/api/auth/logout：免 token，幂等清 cookie。
func Logout(auth *middleware.AuthManager, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		http.SetCookie(c.Writer, auth.ClearCookie(c.Request))
		au.Audit("logout", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(), "", "ok")
		c.Status(http.StatusNoContent)
	}
}

// Me GET /<secret>/api/auth/me
func Me() gin.HandlerFunc {
	return func(c *gin.Context) {
		api.OK(c, gin.H{
			"username": c.GetString("auth_user"),
			"perm":     "admin",
			"exp":      c.GetInt64("auth_exp"),
			"sat":      c.GetInt64("auth_sat"),
		})
	}
}

// Revoke POST /<secret>/api/auth/revoke：轮换 key 即强制登出。
func Revoke(auth *middleware.AuthManager, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth.Rotate()
		http.SetCookie(c.Writer, auth.ClearCookie(c.Request))
		api.Logger(c).Info("auth.revoke", "actor", c.GetString("auth_user"), "ip", middleware.ClientIP(c.Request))
		au.Audit("force_logout", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(), "", "ok")
		c.Status(http.StatusNoContent)
	}
}

// ChangePassword POST /<secret>/api/account/password：
// 成功后写回 instance.yaml 并轮换签名 key，等同一次强制登出。
func ChangePassword(auth *middleware.AuthManager, cfg *config.Config, instancePath string, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		if !middleware.CheckPassword(cfg.Admin.PasswordHash, body.OldPassword) {
			api.Fail(c, api.CodeBadRequest, "旧密码错误")
			return
		}
		if err := middleware.ValidatePassword(cfg.Admin.Username, body.NewPassword); err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		hash, err := middleware.HashPassword(body.NewPassword)
		if err != nil {
			failInternal(c, err)
			return
		}
		err = config.UpdateInstance(instancePath, func(doc *yaml.Node) error {
			if err := config.SetMappingKey(doc, []string{"admin", "username"}, cfg.Admin.Username); err != nil {
				return err
			}
			return config.SetMappingKey(doc, []string{"admin", "password_hash"}, hash)
		})
		if err != nil {
			failInternal(c, err)
			return
		}
		cfg.Admin.PasswordHash = hash
		auth.Rotate()
		http.SetCookie(c.Writer, auth.ClearCookie(c.Request))
		au.Audit("password_change", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(), "", "ok")
		api.OKMsg(c, "密码已修改，请重新登录", nil)
	}
}

// GetIPFilter GET /<secret>/api/ip-filter
func GetIPFilter(f *middleware.IPFilter) gin.HandlerFunc {
	return func(c *gin.Context) {
		mode, scope, rules := f.Snapshot()
		out := make([]string, len(rules))
		for i, p := range rules {
			out[i] = p.String()
		}
		api.OK(c, gin.H{"mode": mode, "scope": scope, "rules": out})
	}
}

// PutIPFilter PUT /<secret>/api/ip-filter：整体替换并热更。
// allow 模式下当前 IP 不在新名单则拒绝保存（锁死检查）。
func PutIPFilter(f *middleware.IPFilter, cfg *config.Config, instancePath string, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Mode  string   `json:"mode"`
			Scope string   `json:"scope"`
			Rules []string `json:"rules"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		switch body.Mode {
		case "off", "allow", "block":
		default:
			api.Fail(c, api.CodeBadRequest, "mode 非法")
			return
		}
		switch body.Scope {
		case "all", "api", "admin":
		default:
			api.Fail(c, api.CodeBadRequest, "scope 非法")
			return
		}
		rules, err := middleware.ParseIPRules(body.Rules)
		if err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		if body.Mode == "allow" {
			addr, aerr := netip.ParseAddr(middleware.ClientIP(c.Request))
			locked := aerr != nil
			if !locked {
				locked = true
				for _, p := range rules {
					if p.Contains(addr) {
						locked = false
						break
					}
				}
			}
			if locked {
				api.Fail(c, api.CodeBadRequest, "当前 IP 不在新名单内，拒绝保存")
				return
			}
		}

		f.Update(body.Mode, body.Scope, rules)
		err = config.UpdateInstance(instancePath, func(doc *yaml.Node) error {
			if err := config.SetMappingKey(doc, []string{"ip_filter", "mode"}, body.Mode); err != nil {
				return err
			}
			if err := config.SetMappingKey(doc, []string{"ip_filter", "scope"}, body.Scope); err != nil {
				return err
			}
			return config.SetMappingKey(doc, []string{"ip_filter", "rules"}, body.Rules)
		})
		if err != nil {
			failInternal(c, err)
			return
		}
		cfg.IPFilter.Mode = body.Mode
		cfg.IPFilter.Scope = body.Scope
		cfg.IPFilter.Rules = body.Rules
		au.Audit("ip_filter_change", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(),
			"mode="+body.Mode+" scope="+body.Scope+" rules="+strconv.Itoa(len(body.Rules)), "ok")
		api.OK(c, gin.H{"mode": body.Mode, "scope": body.Scope, "rules": body.Rules})
	}
}
