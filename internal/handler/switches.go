package handler

import (

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/middleware"
)

// GetFlags GET /<secret>/api/flags
func GetFlags(f *middleware.Flags) gin.HandlerFunc {
	return func(c *gin.Context) {
		api.OK(c, f.Snapshot())
	}
}

// PutFlag PUT /<secret>/api/flags/:name body { enabled }：热更并写盘。
func PutFlag(f *middleware.Flags, cfg *config.Config, instancePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		if !f.Set(name, body.Enabled) {
			api.Fail(c, api.CodeBadRequest, "未知的接口名")
			return
		}
		err := config.UpdateInstance(instancePath, func(doc *yaml.Node) error {
			return config.SetMappingKey(doc, []string{"flags", name}, body.Enabled)
		})
		if err != nil {
			failInternal(c, err)
			return
		}
		switch name {
		case "search":
			cfg.Flags.Search = body.Enabled
		case "detail":
			cfg.Flags.Detail = body.Enabled
		case "catalog":
			cfg.Flags.Catalog = body.Enabled
		case "content":
			cfg.Flags.Content = body.Enabled
		}
		api.OK(c, gin.H{"name": name, "enabled": body.Enabled})
	}
}

// GetRateLimits GET /<secret>/api/rate-limits
func GetRateLimits(rl *middleware.RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		api.OK(c, rl.Snapshot())
	}
}

// PutRateLimits PUT /<secret>/api/rate-limits body 整体替换，热更并写盘。
func PutRateLimits(rl *middleware.RateLimiter, cfg *config.Config, instancePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body config.RateLimitConfig
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		for name, rule := range map[string]config.RateLimitRule{
			"search": body.Search, "detail": body.Detail,
			"catalog": body.Catalog, "content": body.Content,
		} {
			switch rule.Mode {
			case "off", "global", "per_ip":
			default:
				api.Fail(c, api.CodeBadRequest, "rate_limit."+name+".mode 非法")
				return
			}
		}
		rl.Update(body)
		err := config.UpdateInstance(instancePath, func(doc *yaml.Node) error {
			sections := map[string]config.RateLimitRule{
				"search": body.Search, "detail": body.Detail,
				"catalog": body.Catalog, "content": body.Content,
			}
			for name, rule := range sections {
				if err := config.SetMappingKey(doc, []string{"rate_limit", name, "mode"}, rule.Mode); err != nil {
					return err
				}
				if err := config.SetMappingKey(doc, []string{"rate_limit", name, "per_minute"}, rule.PerMinute); err != nil {
					return err
				}
				if err := config.SetMappingKey(doc, []string{"rate_limit", name, "burst"}, rule.Burst); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			failInternal(c, err)
			return
		}
		cfg.RateLimit = body
		api.OK(c, rl.Snapshot())
	}
}
