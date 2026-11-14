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
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/frontend"
	"novel-proxy/internal/handler"
	"novel-proxy/internal/logx"
	"novel-proxy/internal/middleware"
	"novel-proxy/internal/stats"
	"novel-proxy/internal/store"
	"novel-proxy/internal/upstream"
)

const (
	configPath   = "./config.yaml"
	instancePath = "./data/instance.yaml"
)

// serverDeps buildEngine 的共享依赖：跨 engine 存活的对象与路径。
// onConfirm 由 main 注入，init confirm 写盘并更新内存配置之后调用，
// 用 Confirm 阶段建好的存储连接（prepared）完成 engine 热挂载。
type serverDeps struct {
	lg           *logx.Log
	instancePath string
	staticDir    string
	frontends    *frontend.Manager
	startedAt    time.Time
	onConfirm    func(*handler.PreparedStorage) error
	// prepared 由 confirm 钩子写入、buildEngine 消费一次。
	prepared *handler.PreparedStorage
	// FrontendDir 由 buildEngine 写入，main 的启动日志读取。
	FrontendDir string
}

// buildEngine 按配置组装完整的 handler：全部中间件与全部路由。
// 每次调用产出一个独立的 engine，各自持有独立的限流器、登录失败
// 计数与 JWT key。返回的 cleanup 停掉本 engine 独有的后台协程，
// engine 被热挂载换掉后调用。出错返回 error：启动期直接退出，
// confirm 热挂载期向导保持可用。
// 中间件顺序固定：trace_id → stats → recover → ipfilter → initguard
// → auth → flags → ratelimit；init 路由注册在 auth/flags/ratelimit
// 之前，不经过这三层（自身的会话与 IP 锁就是全部防护）。
func buildEngine(cfg *config.Config, deps *serverDeps) (*gin.Engine, func(), error) {
	engine := gin.New()
	engine.Use(api.SlogMiddleware(deps.lg.Logger, cfg.Server.AdminPath))

	// 统计钩子：紧随 trace_id，包裹全部后续中间件（含 recover，panic 也记录）。
	// 与文章缓存同库；storage 未启用时为 nil，三组统计接口返回 50301。
	// confirm 热挂载时 storage 已在写盘前建好（deps.prepared），直接挂用。
	// 消费标记留到成功收尾：中途失败时 deps.prepared 仍指向连接组，
	// Confirm 侧按未消费释放，stats 写入协程不会变僵尸。
	var statStore *stats.Stats
	var counters *store.CountingStore
	var articleStore store.ArticleStore
	if deps.prepared != nil {
		statStore = deps.prepared.StatStore
		counters = deps.prepared.Counters
		articleStore = deps.prepared.ArticleStore
	} else if cfg.Storage.Enabled {
		ss, serr := stats.New(cfg.Storage.Driver, cfg.Storage.DSN, deps.lg.Logger)
		if serr != nil {
			return nil, nil, fmt.Errorf("初始化统计失败: %w", serr)
		}
		st, oerr := store.Open(cfg.Storage.Driver, cfg.Storage.DSN)
		if oerr != nil {
			return nil, nil, fmt.Errorf("初始化存储失败: %w", oerr)
		}
		if err := st.Init(context.Background()); err != nil {
			return nil, nil, fmt.Errorf("初始化存储失败: %w", err)
		}
		if err := st.Ping(context.Background()); err != nil {
			return nil, nil, fmt.Errorf("存储连接失败: %w", err)
		}
		statStore = ss
		counters = store.Count(st)
		articleStore = counters
	}
	engine.Use(statStore.Middleware())
	engine.Use(api.RecoveryMiddleware(deps.lg.Logger, cfg.App.Mode))

	// 固定链：ipfilter 在 recover 之后、auth 之前。
	ipFilter, err := middleware.NewIPFilter(cfg.IPFilter, cfg.Server.AdminPath)
	if err != nil {
		return nil, nil, fmt.Errorf("加载 IP 名单失败: %w", err)
	}
	engine.Use(ipFilter.Middleware())

	// 前端版本管理：目录由 manager 提供，版本切换时替换 swap 里的 handler。
	// 静态目录解析顺序：-static > 指针文件 > default/ 兜底 > 固定 404 页。
	mgr := deps.frontends
	swap := &frontend.SwapHandler{}
	adminSwap := &frontend.SwapHandler{}
	if deps.staticDir != "" {
		deps.FrontendDir = deps.staticDir
		swap.Set(frontend.FileServer(deps.staticDir, "/"))
	} else {
		// reader 用 BrowserRouter，挂载点带 SPA 兜底（无扩展名回落 index.html）。
		mgr.Mount("reader", swap, "/", true)
		mgr.Reload("reader")
		deps.FrontendDir, _ = mgr.Resolve("reader")
	}
	if cfg.Server.AdminPath != "" {
		// 管理端 SPA 挂在 /<secret>/ 下，页面路径免 token，
		// 版本上传激活后由 manager 直接换 handler。
		mgr.Mount("admin", adminSwap, "/"+cfg.Server.AdminPath, false)
		mgr.Reload("admin")
	}
	// init 页面是 frontends 的第三个 app：指针 > default > Go 固定说明页，
	// 重启生效；-static 不作用于它。
	initSwap := &frontend.SwapHandler{}
	mgr.Mount("init", initSwap, "/init", false)
	mgr.Reload("init")
	if dir, ok := mgr.Resolve("init"); ok {
		initSwap.Set(frontend.FileServer(dir, "/init"))
	} else {
		initSwap.Set(frontend.InitFallbackPage(filepath.Join("data", "frontends", "init", "default")))
	}

	// 未初始化门禁：固定链中 ipfilter 之后、auth 之前。
	// completed 是闭包，读 cfg 活值：confirm 改完内存配置立刻生效，
	// 旧 engine 也会把 /init/* 判成 404。
	initGuard := middleware.NewInitGuard(func() bool { return cfg.Setup.Completed })
	engine.Use(initGuard.Middleware())

	// init 向导路由：/init/* 是否可达由 guard 决定（完成后全部 404）。
	// 令牌门禁在未初始化时启动：生成或复用 data/init-token.txt 并打印。
	// 热挂载钩子：confirm 写盘后由 wizard 调用，完成 storage 连接与
	// engine 整体替换。第二个 engine 上 /init/* 已被 guard 判 404，
	// 钩子不会再被触发。
	wizard := handler.NewInitWizard(cfg, deps.instancePath, "data", deps.lg.Logger)
	if deps.onConfirm != nil {
		wizard.SetOnConfirm(deps.onConfirm)
	}
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

	startedAt := deps.startedAt
	client := upstream.NewClient(cfg.Upstream.TimeoutSeconds, cfg.Upstream.MaxConcurrency)

	// 固定链尾段：auth（管理端）→ flags → ratelimit。
	// 所有 Use 必须先于任何路由注册，gin 在注册时捕获链。
	flagGate := middleware.NewFlags(cfg.Flags)
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimit, cfg.Server.AdminPath)

	// 管理端认证：admin_path 为空（未初始化）时不挂载任何管理路由。
	// authMgr 只建一次，中间件校验与 handler 签发必须同一把 key。
	var authMgr *middleware.AuthManager
	if cfg.Server.AdminPath != "" {
		authMgr = middleware.NewAuthManager(cfg.Server.AdminPath)
		engine.Use(middleware.Auth(authMgr))
	}
	engine.Use(flagGate.Middleware())
	engine.Use(rateLimiter.Middleware())

	if authMgr != nil {
		admin := engine.Group("/" + cfg.Server.AdminPath)
		admin.POST("/api/auth/login", handler.Login(authMgr, cfg, statStore))
		admin.POST("/api/auth/logout", handler.Logout(authMgr, statStore))
		admin.GET("/api/auth/me", handler.Me())
		admin.POST("/api/auth/revoke", handler.Revoke(authMgr, statStore))
		admin.POST("/api/account/password", handler.ChangePassword(authMgr, cfg, deps.instancePath, statStore))
		admin.GET("/api/ip-filter", handler.GetIPFilter(ipFilter))
		admin.PUT("/api/ip-filter", handler.PutIPFilter(ipFilter, cfg, deps.instancePath, statStore))
		admin.GET("/api/frontends", handler.ListFrontends(mgr))
		admin.POST("/api/frontends/:app/upload", handler.UploadFrontend(mgr, statStore))
		admin.POST("/api/frontends/:app/activate", handler.ActivateFrontend(mgr, statStore))
		admin.POST("/api/frontends/:app/confirm", handler.ConfirmFrontend(mgr, statStore))
		admin.POST("/api/frontends/:app/rollback", handler.RollbackFrontend(mgr, statStore))
		admin.DELETE("/api/frontends/:app/:version", handler.DeleteFrontend(mgr, statStore))
		admin.GET("/api/flags", handler.GetFlags(flagGate))
		admin.PUT("/api/flags/:name", handler.PutFlag(flagGate, cfg, deps.instancePath, statStore))
		admin.GET("/api/rate-limits", handler.GetRateLimits(rateLimiter))
		admin.PUT("/api/rate-limits", handler.PutRateLimits(rateLimiter, cfg, deps.instancePath, statStore))
		admin.GET("/api/stats/overview", handler.RequireStats(statStore), handler.StatsOverview(statStore, counters, startedAt))
		admin.GET("/api/stats/query", handler.RequireStats(statStore), handler.StatsQuery(statStore))
		admin.GET("/api/audit", handler.RequireStats(statStore), handler.AuditList(statStore))
		admin.GET("/api/cache", handler.RequireStats(statStore), handler.CacheList(statStore, counters))
		admin.DELETE("/api/cache/:item_id", handler.RequireStats(statStore), handler.CacheDeleteItem(statStore, articleStore))
		admin.DELETE("/api/cache", handler.RequireStats(statStore), handler.CacheDeleteByBook(statStore, articleStore))
		admin.DELETE("/api/cache/all", handler.RequireStats(statStore), handler.CacheDeleteAll(statStore, articleStore))
		admin.GET("/api/content-rules", handler.GetContentRules(cfg))
		admin.PUT("/api/content-rules", handler.PutContentRules(cfg, deps.instancePath, statStore))
		admin.POST("/api/content-rules/preview", handler.ContentRulesPreview(cfg, client, articleStore))
		admin.GET("/api/config/log", handler.GetLogConfig(deps.lg, cfg))
		admin.PUT("/api/config/log", handler.PutLogConfig(deps.lg, cfg, deps.instancePath, statStore))
		admin.GET("/api/config/upstream", handler.GetUpstreamConfig(cfg))
		admin.PUT("/api/config/upstream", handler.PutUpstreamConfig(cfg, client, deps.instancePath, statStore))
	}

	// 静态目录解析：-static > 指针文件 > default/ 兜底 > 固定 404 页。

	engine.NoRoute(api.SplitNoRoute(cfg.Server.AdminPath, adminSwap, swap, initSwap))

	engine.GET("/api/search", handler.Search(client, cfg))
	engine.GET("/api/detail", handler.Detail(client, cfg))
	engine.GET("/api/catalog", handler.Catalog(client, cfg))
	engine.GET("/api/content", handler.Content(client, articleStore, cfg))

	// cleanup：停掉本 engine 独有的后台协程。Stop 只停限流扫描协程，
	// 在途请求持有的令牌桶不受影响，可以安全地在换 engine 后调用。
	// 成功收尾：连接组归本 engine，标记已消费并清空钩子侧引用。
	if deps.prepared != nil {
		deps.prepared.MarkConsumed()
		deps.prepared = nil
	}
	return engine, rateLimiter.Stop, nil
}

func main() {
	staticDir := flag.String("static", "", "前端静态目录，优先于指针文件与 default 兜底")
	flag.Parse()

	cfg, err := config.Load(configPath, instancePath)
	if err != nil {
		// 日志还没起来，直接写 stderr 退出。
		fmt.Fprintln(os.Stderr, "加载配置失败:", err)
		os.Exit(1)
	}

	logHandle := logx.New(cfg.Log.Level, cfg.Log.File, cfg.Log.MaxSizeMB, cfg.Log.MaxBackups, cfg.Log.MaxAgeDays, cfg.Log.Compress)
	defer logHandle.Close()
	lg := logHandle.Logger
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

	// 顶层可换容器：init confirm 后整体替换 engine，进程不重启。
	swappable := &api.Swappable{}

	deps := &serverDeps{
		lg:           logHandle,
		instancePath: instancePath,
		staticDir:    *staticDir,
		frontends:    frontend.New("./data/frontends"),
		startedAt:    time.Now(),
	}
	deps.frontends.SetLogger(lg)

	// 清理链：换 engine 后停掉旧 engine 的后台协程。只在启动与
	// confirm 两处变更；confirm 由 wizard 的互斥锁串行化。
	var stopMu sync.Mutex
	var curStop func()
	setStop := func(f func()) { stopMu.Lock(); curStop = f; stopMu.Unlock() }
	getStop := func() func() {
		stopMu.Lock()
		f := curStop
		stopMu.Unlock()
		return f
	}

	deps.onConfirm = func(p *handler.PreparedStorage) error {
		deps.prepared = p
		h, stop, err := buildEngine(&cfg, deps)
		if err != nil {
			return err
		}
		// 先换 handler 再停旧协程：在途请求在旧 engine 上自然跑完，
		// 新请求立刻走新 engine。
		swappable.Store(h)
		if old := getStop(); old != nil {
			old()
		}
		setStop(stop)
		return nil
	}

	engine, stopEngine, err := buildEngine(&cfg, deps)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	swappable.Store(engine)
	setStop(stopEngine)
	defer func() {
		if s := getStop(); s != nil {
			s()
		}
	}()

	srv := &http.Server{Addr: cfg.Server.Addr, Handler: swappable}
	go func() {
		slog.Info("novel-proxy 启动",
			"mode", cfg.App.Mode,
			"addr", cfg.Server.Addr,
			"frontend_dir", deps.FrontendDir,
		)
		if deps.FrontendDir == "" {
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
