// Package config 负责加载并校验配置。
// 文件路径优先级：-config 参数 > APP_CONFIG 环境变量 > ./config.yaml。
// 键值覆盖：只有 app.mode、server.addr、log.level 支持环境变量
// （APP_MODE / APP_ADDR / APP_LOG_LEVEL）。
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
}

type LogConfig struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAgeDays int    `yaml:"max_age_days"`
	Compress   bool   `yaml:"compress"`
}

type UpstreamConfig struct {
	TimeoutSeconds int `yaml:"timeout_seconds"`
	MaxConcurrency int `yaml:"max_concurrency"`
}

type Config struct {
	App      AppConfig      `yaml:"app"`
	Server   ServerConfig   `yaml:"server"`
	Log      LogConfig      `yaml:"log"`
	Upstream UpstreamConfig `yaml:"upstream"`
}

// defaults 返回文档规定的全部默认值，先填默认值再让 yaml 覆盖。
func defaults() Config {
	var c Config
	c.App.Mode = "release"
	c.Server.Addr = ":8080"
	c.Server.ShutdownTimeoutSeconds = 10
	c.Log.Level = "info"
	c.Log.File = "./logs/app.log"
	c.Log.MaxSizeMB = 50
	c.Log.MaxBackups = 7
	c.Log.MaxAgeDays = 14
	c.Log.Compress = true
	c.Upstream.TimeoutSeconds = 10
	c.Upstream.MaxConcurrency = 8
	return c
}

// Load 加载配置。flagPath 是 -config 参数的值，可以为空。
// 解析不到文件、文件里有未知键、枚举字段取到非法值时返回错误。
func Load(flagPath string) (Config, error) {
	path := flagPath
	if path == "" {
		path = os.Getenv("APP_CONFIG")
	}
	if path == "" {
		path = "./config.yaml"
	}

	c := defaults()
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("读取配置 %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	// 未知键必须报错，防止拼写错误被静默忽略。
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("解析配置 %s: %w", path, err)
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

	switch c.App.Mode {
	case "debug", "release":
	default:
		return Config{}, fmt.Errorf("app.mode 非法值 %q", c.App.Mode)
	}
	switch c.Log.Level {
	case "debug", "info", "error":
	default:
		return Config{}, fmt.Errorf("log.level 非法值 %q", c.Log.Level)
	}
	return c, nil
}
