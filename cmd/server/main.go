package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/handler"
	"novel-proxy/internal/logx"
	"novel-proxy/internal/upstream"
)

const (
	configPath   = "./config.yaml"
	instancePath = "./data/instance.yaml"
)

func main() {
	cfg, err := config.Load(configPath, instancePath)
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
		"setup_completed", cfg.Setup.Completed,
		"storage_enabled", cfg.Storage.Enabled,
		"storage_driver", cfg.Storage.Driver,
		"upstream_timeout_seconds", cfg.Upstream.TimeoutSeconds,
		"upstream_max_concurrency", cfg.Upstream.MaxConcurrency,
	)

	// app.mode 映射到 gin 模式：debug 打开启动提示，release 静默。
	gin.SetMode(cfg.App.Mode)

	engine := gin.New()
	engine.Use(api.SlogMiddleware(lg), api.RecoveryMiddleware(lg, cfg.App.Mode))
	engine.NoRoute(api.NoRoute)

	client := upstream.NewClient(cfg.Upstream.TimeoutSeconds)
	engine.GET("/api/search", handler.Search(client))
	engine.GET("/api/detail", handler.Detail(client))
	engine.GET("/api/catalog", handler.Catalog(client))
	engine.GET("/api/content", handler.Content(client))

	srv := &http.Server{Addr: cfg.Server.Addr, Handler: engine}
	go func() {
		// frontend_dir 由 internal/frontend 的 manager 提供（W4），当前未配置。
		slog.Info("novel-proxy 启动",
			"mode", cfg.App.Mode,
			"addr", cfg.Server.Addr,
			"frontend_dir", "",
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("error", "err", err.Error())
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("novel-proxy 停止中")
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Server.ShutdownTimeoutSeconds)*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("error", "err", err.Error())
	}
}
