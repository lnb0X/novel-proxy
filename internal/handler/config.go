package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	"gopkg.in/yaml.v3"
	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/logx"
	"novel-proxy/internal/middleware"
	"novel-proxy/internal/stats"
	"novel-proxy/internal/upstream"
)

// GetLogConfig GET /<secret>/api/config/log
func GetLogConfig(lh *logx.Log, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		api.OK(c, gin.H{
			"level":        lh.Level(),
			"max_size_mb":  cfg.Log.MaxSizeMB,
			"max_backups":  cfg.Log.MaxBackups,
			"max_age_days": cfg.Log.MaxAgeDays,
			"compress":     cfg.Log.Compress,
		})
	}
}

// PutLogConfig PUT /<secret>/api/config/log
// level 立即生效；滚动四项写回 instance.yaml，重启后生效
// （lumberjack 字段在自己锁内，运行期不可变，见 W14）。
func PutLogConfig(lh *logx.Log, cfg *config.Config, instancePath string, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Level      string `json:"level"`
			MaxSizeMB  int    `json:"max_size_mb"`
			MaxBackups int    `json:"max_backups"`
			MaxAgeDays int    `json:"max_age_days"`
			Compress   bool   `json:"compress"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		if err := lh.SetLevel(body.Level); err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		err := config.UpdateInstance(instancePath, func(doc *yaml.Node) error {
			steps := []struct {
				path []string
				val  any
			}{
				{[]string{"log", "level"}, body.Level},
				{[]string{"log", "max_size_mb"}, body.MaxSizeMB},
				{[]string{"log", "max_backups"}, body.MaxBackups},
				{[]string{"log", "max_age_days"}, body.MaxAgeDays},
				{[]string{"log", "compress"}, body.Compress},
			}
			for _, s := range steps {
				if err := config.SetMappingKey(doc, s.path, s.val); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			failInternal(c, err)
			return
		}
		cfg.Log.Level = body.Level
		cfg.Log.MaxSizeMB = body.MaxSizeMB
		cfg.Log.MaxBackups = body.MaxBackups
		cfg.Log.MaxAgeDays = body.MaxAgeDays
		cfg.Log.Compress = body.Compress
		au.Audit("config_change", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(),
			"config/log level="+body.Level, "ok")
		api.OKMsg(c, "level 已立即生效；滚动四项已写盘，重启后生效", gin.H{
			"level":        lh.Level(),
			"max_size_mb":  body.MaxSizeMB,
			"max_backups":  body.MaxBackups,
			"max_age_days": body.MaxAgeDays,
			"compress":     body.Compress,
		})
	}
}

// GetUpstreamConfig GET /<secret>/api/config/upstream
func GetUpstreamConfig(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		api.OK(c, gin.H{
			"timeout_seconds": cfg.Upstream.TimeoutSeconds,
			"max_concurrency": upstream.Concurrency(),
		})
	}
}

// PutUpstreamConfig PUT /<secret>/api/config/upstream
// 两项都立即生效：resty client 热改超时，信号量热调上限。
func PutUpstreamConfig(cfg *config.Config, client *resty.Client, instancePath string, au *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			TimeoutSeconds int `json:"timeout_seconds"`
			MaxConcurrency int `json:"max_concurrency"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		if body.TimeoutSeconds < 1 || body.TimeoutSeconds > 300 {
			api.Fail(c, api.CodeBadRequest, "timeout_seconds 需在 1 到 300 之间")
			return
		}
		if body.MaxConcurrency < 1 || body.MaxConcurrency > 1000 {
			api.Fail(c, api.CodeBadRequest, "max_concurrency 需在 1 到 1000 之间")
			return
		}
		client.SetTimeout(time.Duration(body.TimeoutSeconds) * time.Second)
		upstream.SetConcurrency(body.MaxConcurrency)
		err := config.UpdateInstance(instancePath, func(doc *yaml.Node) error {
			if err := config.SetMappingKey(doc, []string{"upstream", "timeout_seconds"}, body.TimeoutSeconds); err != nil {
				return err
			}
			return config.SetMappingKey(doc, []string{"upstream", "max_concurrency"}, body.MaxConcurrency)
		})
		if err != nil {
			failInternal(c, err)
			return
		}
		cfg.Upstream.TimeoutSeconds = body.TimeoutSeconds
		cfg.Upstream.MaxConcurrency = body.MaxConcurrency
		au.Audit("config_change", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(),
			"config/upstream timeout="+strconv.Itoa(body.TimeoutSeconds)+" concurrency="+strconv.Itoa(body.MaxConcurrency), "ok")
		api.OK(c, gin.H{
			"timeout_seconds": body.TimeoutSeconds,
			"max_concurrency": upstream.Concurrency(),
		})
	}
}
