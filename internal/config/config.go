// Package config 负责加载并校验配置。
// 优先级：env > data/instance.yaml > config.yaml。
// config.yaml 进 git，程序不写；data/instance.yaml 由 init 与运行期写入，
// 不存在时跳过。config.yaml 必须存在。
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	Mode string `yaml:"mode"`
}

type ServerConfig struct {
	Addr                   string `yaml:"addr"`
	ShutdownTimeoutSeconds int    `yaml:"shutdown_timeout_seconds"`
	AdminPath              string `yaml:"admin_path"`
}

type LogConfig struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAgeDays int    `yaml:"max_age_days"`
	Compress   bool   `yaml:"compress"`
}

type SetupConfig struct {
	Completed bool `yaml:"completed"`
}

type FlagsConfig struct {
	Search  bool `yaml:"search" json:"search"`
	Detail  bool `yaml:"detail" json:"detail"`
	Catalog bool `yaml:"catalog" json:"catalog"`
	Content bool `yaml:"content" json:"content"`
}

type AdminConfig struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

type StorageConfig struct {
	Enabled bool   `yaml:"enabled"`
	Driver  string `yaml:"driver"`
	DSN     string `yaml:"dsn"`
}

type UpstreamConfig struct {
	TimeoutSeconds int `yaml:"timeout_seconds"`
	MaxConcurrency int `yaml:"max_concurrency"`
}

type RateLimitRule struct {
	Mode      string `yaml:"mode" json:"mode"`
	PerMinute int    `yaml:"per_minute" json:"per_minute"`
	Burst     int    `yaml:"burst" json:"burst"`
}

type RateLimitConfig struct {
	Search  RateLimitRule `yaml:"search" json:"search"`
	Detail  RateLimitRule `yaml:"detail" json:"detail"`
	Catalog RateLimitRule `yaml:"catalog" json:"catalog"`
	Content RateLimitRule `yaml:"content" json:"content"`
}

type IPFilterConfig struct {
	Mode  string   `yaml:"mode"`
	Scope string   `yaml:"scope"`
	Rules []string `yaml:"rules"`
}

// ReplaceRule 内容替换规则，级联语义见 DECISIONS 内容规则节。
type ReplaceRule struct {
	Pattern     string `yaml:"pattern" json:"pattern"`
	Replacement string `yaml:"replacement" json:"replacement"`
	Regex       bool   `yaml:"regex" json:"regex"`
}

type ContentRuleSet struct {
	Prepend  *string       `yaml:"prepend" json:"prepend"`
	Append   *string       `yaml:"append" json:"append"`
	Replace  []ReplaceRule `yaml:"replace" json:"replace"`
	Override *string       `yaml:"override" json:"override"`
}

type ContentRulesConfig struct {
	Default ContentRuleSet            `yaml:"default" json:"default"`
	Books   map[string]ContentRuleSet `yaml:"books" json:"books"`
}

type Config struct {
	App          AppConfig          `yaml:"app"`
	Server       ServerConfig       `yaml:"server"`
	Log          LogConfig          `yaml:"log"`
	Setup        SetupConfig        `yaml:"setup"`
	Flags        FlagsConfig        `yaml:"flags"`
	Admin        AdminConfig        `yaml:"admin"`
	Storage      StorageConfig      `yaml:"storage"`
	Upstream     UpstreamConfig     `yaml:"upstream"`
	RateLimit    RateLimitConfig    `yaml:"rate_limit"`
	IPFilter     IPFilterConfig     `yaml:"ip_filter"`
	ContentRules ContentRulesConfig `yaml:"content_rules"`
}

// defaults 返回 DECISIONS.md 配置 schema 规定的全部默认值。
func defaults() Config {
	var c Config
	c.App.Mode = "release"
	c.Server.Addr = "127.0.0.1:8080"
	c.Server.ShutdownTimeoutSeconds = 10
	c.Log.Level = "info"
	c.Log.File = "./logs/app.log"
	c.Log.MaxSizeMB = 50
	c.Log.MaxBackups = 14
	c.Log.MaxAgeDays = 14
	c.Log.Compress = true
	c.Flags.Search = true
	c.Flags.Detail = true
	c.Flags.Catalog = true
	c.Flags.Content = true
	c.Upstream.TimeoutSeconds = 10
	c.Upstream.MaxConcurrency = 8
	c.RateLimit.Search.Mode = "off"
	c.RateLimit.Detail.Mode = "off"
	c.RateLimit.Catalog.Mode = "off"
	c.RateLimit.Content.Mode = "off"
	c.IPFilter.Mode = "off"
	c.IPFilter.Scope = "all"
	return c
}

// Load 按 env > instancePath > configPath 的顺序叠加配置，
// 再校验枚举。config.yaml 必须存在；instance.yaml 不存在时跳过。
func Load(configPath, instancePath string) (Config, error) {
	c := defaults()
	if err := decodeInto(configPath, &c); err != nil {
		return Config{}, err
	}
	if _, err := os.Stat(instancePath); err == nil {
		if err := decodeInto(instancePath, &c); err != nil {
			return Config{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("读取配置 %s: %w", instancePath, err)
	}

	if v := os.Getenv("APP_MODE"); v != "" {
		c.App.Mode = v
	}
	if v := os.Getenv("APP_ADDR"); v != "" {
		c.Server.Addr = v
	}
	if v := os.Getenv("APP_LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	if v := os.Getenv("APP_STORAGE_DSN"); v != "" {
		c.Storage.DSN = v
	}

	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// decodeInto 把单个 yaml 文件解进 c。两个文件都开 KnownFields，
// 未知键（含嵌套层级）报错，防止拼写错误被静默忽略。
func decodeInto(path string, c *Config) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取配置 %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("解析配置 %s: %w", path, err)
	}
	return nil
}

func (c Config) validate() error {
	switch c.App.Mode {
	case "debug", "release":
	default:
		return fmt.Errorf("app.mode 非法值 %q", c.App.Mode)
	}
	switch c.Log.Level {
	case "debug", "info", "error":
	default:
		return fmt.Errorf("log.level 非法值 %q", c.Log.Level)
	}
	switch c.Storage.Driver {
	case "", "sqlite", "postgres", "mysql", "mongo":
	default:
		return fmt.Errorf("storage.driver 非法值 %q", c.Storage.Driver)
	}
	named := []struct {
		name string
		rule RateLimitRule
	}{
		{"search", c.RateLimit.Search},
		{"detail", c.RateLimit.Detail},
		{"catalog", c.RateLimit.Catalog},
		{"content", c.RateLimit.Content},
	}
	for _, nr := range named {
		switch nr.rule.Mode {
		case "off", "global", "per_ip":
		default:
			return fmt.Errorf("rate_limit.%s.mode 非法值 %q", nr.name, nr.rule.Mode)
		}
	}
	switch c.IPFilter.Mode {
	case "off", "allow", "block":
	default:
		return fmt.Errorf("ip_filter.mode 非法值 %q", c.IPFilter.Mode)
	}
	switch c.IPFilter.Scope {
	case "all", "api", "admin":
	default:
		return fmt.Errorf("ip_filter.scope 非法值 %q", c.IPFilter.Scope)
	}
	return nil
}
