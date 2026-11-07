package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/frontend"
	"novel-proxy/internal/handler"
	"novel-proxy/internal/logx"
	"novel-proxy/internal/middleware"
	"novel-proxy/internal/store"
	"novel-proxy/internal/upstream"
)

const (
	configPath   = "./config.yaml"
	instancePath = "./data/instance.yaml"
)

func main() {
	staticDir := flag.String("static", "", "前端静态目录，优先于指针文件与 default 兜底")
	flag.Parse()

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
	engine.Use(api.SlogMiddleware(lg, cfg.Server.AdminPath), api.RecoveryMiddleware(lg, cfg.App.Mode))

	// 固定链：ipfilter 在 recover 之后、auth 之前。
	ipFilter, err := middleware.NewIPFilter(cfg.IPFilter, cfg.Server.AdminPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "加载 IP 名单失败:", err)
		os.Exit(1)
	}
	engine.Use(ipFilter.Middleware())

	// 前端版本管理：目录由 manager 提供，版本切换时替换 swap 里的 handler。
	// 静态目录解析顺序：-static > 指针文件 > default/ 兜底 > 固定 404 页。
	mgr := frontend.New("./data/frontends")
	mgr.SetLogger(lg)
	swap := &frontend.SwapHandler{}
	adminSwap := &frontend.SwapHandler{}
	frontendDir := ""
	if *staticDir != "" {
		frontendDir = *staticDir
		swap.Set(frontend.FileServer(frontendDir, "/"))
	} else {
		mgr.Mount("reader", swap, "/")
		mgr.Reload("reader")
		frontendDir, _ = mgr.Resolve("reader")
	}
	if cfg.Server.AdminPath != "" {
		// 管理端 SPA 挂在 /<secret>/ 下，页面路径免 token，
		// 版本上传激活后由 manager 直接换 handler。
		mgr.Mount("admin", adminSwap, "/"+cfg.Server.AdminPath)
		mgr.Reload("admin")
	}
	// init 页面是 frontends 的第三个 app：指针 > default > Go 固定说明页，
	// 重启生效；-static 不作用于它。
	initSwap := &frontend.SwapHandler{}
	mgr.Mount("init", initSwap, "/init")
	mgr.Reload("init")
	if dir, ok := mgr.Resolve("init"); ok {
		initSwap.Set(frontend.FileServer(dir, "/init"))
	} else {
		initSwap.Set(frontend.InitFallbackPage(filepath.Join("data", "frontends", "init", "default")))
	}

	// 未初始化门禁：固定链中 ipfilter 之后、auth 之前。
	initGuard := middleware.NewInitGuard(func() bool { return cfg.Setup.Completed })
	engine.Use(initGuard.Middleware())

	// init 向导路由：/init/* 是否可达由 guard 决定（完成后全部 404）。
	// 令牌门禁在未初始化时启动：生成或复用 data/init-token.txt 并打印。
	wizard := handler.NewInitWizard(&cfg, instancePath, "data", lg)
	initGroup := engine.Group("/init")
	initGroup.GET("/api/state", wizard.State())
	initGroup.POST("/api/token", wizard.Token())
	secured := initGroup.Group("", wizard.RequireSession())
	secured.POST("/api/account", wizard.Account())
	secured.POST("/api/storage", wizard.Storage())
	secured.POST("/api/database/test", wizard.DatabaseTest())
	secured.POST("/api/database", wizard.Database())
	secured.POST("/api/confirm", wizard.Confirm())
	secured.POST("/api/cancel", wizard.Cancel())

	// 管理端认证：admin_path 为空（未初始化）时不挂载任何管理路由。
	if cfg.Server.AdminPath != "" {
		authMgr := middleware.NewAuthManager(cfg.Server.AdminPath)
		engine.Use(middleware.Auth(authMgr))

		admin := engine.Group("/" + cfg.Server.AdminPath)
		admin.POST("/api/auth/login", handler.Login(authMgr, &cfg))
		admin.POST("/api/auth/logout", handler.Logout(authMgr))
		admin.GET("/api/auth/me", handler.Me())
		admin.POST("/api/auth/revoke", handler.Revoke(authMgr))
		admin.POST("/api/account/password", handler.ChangePassword(authMgr, &cfg, instancePath))
		admin.GET("/api/ip-filter", handler.GetIPFilter(ipFilter))
		admin.PUT("/api/ip-filter", handler.PutIPFilter(ipFilter, &cfg, instancePath))
		admin.GET("/api/frontends", handler.ListFrontends(mgr))
		admin.POST("/api/frontends/:app/upload", handler.UploadFrontend(mgr))
		admin.POST("/api/frontends/:app/activate", handler.ActivateFrontend(mgr))
		admin.POST("/api/frontends/:app/confirm", handler.ConfirmFrontend(mgr))
		admin.POST("/api/frontends/:app/rollback", handler.RollbackFrontend(mgr))
		admin.DELETE("/api/frontends/:app/:version", handler.DeleteFrontend(mgr))
	}

	// 静态目录解析：-static > 指针文件 > default/ 兜底 > 固定 404 页。

	engine.NoRoute(api.SplitNoRoute(cfg.Server.AdminPath, adminSwap, swap, initSwap))

	// 数据库只在 storage.enabled 时连接；未启用时 content 直接旁路缓存。
	var articleStore store.ArticleStore
	if cfg.Storage.Enabled {
		st, err := store.Open(cfg.Storage.Driver, cfg.Storage.DSN)
		if err != nil {
			fmt.Fprintln(os.Stderr, "初始化存储失败:", err)
			os.Exit(1)
		}
		if err := st.Init(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, "初始化存储失败:", err)
			os.Exit(1)
		}
		if err := st.Ping(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, "存储连接失败:", err)
			os.Exit(1)
		}
		articleStore = st
	}

	client := upstream.NewClient(cfg.Upstream.TimeoutSeconds)
	engine.GET("/api/search", handler.Search(client))
	engine.GET("/api/detail", handler.Detail(client))
	engine.GET("/api/catalog", handler.Catalog(client))
	engine.GET("/api/content", handler.Content(client, articleStore))

	srv := &http.Server{Addr: cfg.Server.Addr, Handler: engine}
	go func() {
		slog.Info("novel-proxy 启动",
			"mode", cfg.App.Mode,
			"addr", cfg.Server.Addr,
			"frontend_dir", frontendDir,
		)
		if frontendDir == "" {
			// 未配置静态目录的原因：无 -static 参数、无指针文件、无 default 兜底。
			slog.Info("未配置前端目录，非 API 路径将返回固定 404 页")
		}
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
