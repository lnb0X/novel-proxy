package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/middleware"
)

// Login POST /<secret>/api/auth/login
func Login(auth *middleware.AuthManager, cfg *config.Config) gin.HandlerFunc {
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
			api.Fail(c, api.CodeUnauthorized, "账号或密码错误")
			return
		}
		auth.LoginSuccess(ip, body.Username)
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
func Logout(auth *middleware.AuthManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		http.SetCookie(c.Writer, auth.ClearCookie(c.Request))
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
func Revoke(auth *middleware.AuthManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth.Rotate()
		http.SetCookie(c.Writer, auth.ClearCookie(c.Request))
		api.Logger(c).Info("auth.revoke", "actor", c.GetString("auth_user"), "ip", middleware.ClientIP(c.Request))
		c.Status(http.StatusNoContent)
	}
}

// ChangePassword POST /<secret>/api/account/password：
// 成功后写回 instance.yaml 并轮换签名 key，等同一次强制登出。
func ChangePassword(auth *middleware.AuthManager, cfg *config.Config, instancePath string) gin.HandlerFunc {
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
			api.Fail(c, api.CodeInternal, err.Error())
			return
		}
		err = config.UpdateInstance(instancePath, func(doc *yaml.Node) error {
			if err := config.SetMappingKey(doc, []string{"admin", "username"}, cfg.Admin.Username); err != nil {
				return err
			}
			return config.SetMappingKey(doc, []string{"admin", "password_hash"}, hash)
		})
		if err != nil {
			api.Fail(c, api.CodeInternal, err.Error())
			return
		}
		cfg.Admin.PasswordHash = hash
		auth.Rotate()
		http.SetCookie(c.Writer, auth.ClearCookie(c.Request))
		api.OKMsg(c, "密码已修改，请重新登录", nil)
	}
}
