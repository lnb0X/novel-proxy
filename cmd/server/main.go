package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"novel-proxy/internal/config"
	"novel-proxy/internal/logx"
)

func main() {
	configPath := flag.String("config", "", "配置文件路径，缺省依次取 APP_CONFIG 和 ./config.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		// 日志还没起来，直接写 stderr 退出。
		fmt.Fprintln(os.Stderr, "加载配置失败:", err)
		os.Exit(1)
	}

	lg, closer := logx.New(cfg.Log.Level, cfg.Log.File, cfg.Log.MaxSizeMB, cfg.Log.MaxBackups, cfg.Log.MaxAgeDays, cfg.Log.Compress)
	defer closer.Close()
	slog.SetDefault(lg)

	slog.Debug("配置已加载",
		"app_mode", cfg.App.Mode,
		"server_addr", cfg.Server.Addr,
		"shutdown_timeout_seconds", cfg.Server.ShutdownTimeoutSeconds,
		"log_level", cfg.Log.Level,
		"log_file", cfg.Log.File,
		"upstream_timeout_seconds", cfg.Upstream.TimeoutSeconds,
		"upstream_max_concurrency", cfg.Upstream.MaxConcurrency,
	)
	slog.Info("novel-proxy 启动", "mode", cfg.App.Mode, "addr", cfg.Server.Addr)
}
