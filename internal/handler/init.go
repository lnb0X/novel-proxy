package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/middleware"
	"novel-proxy/internal/stats"
	"novel-proxy/internal/store"
)

const (
	initTokenFile  = "init-token.txt"
	initCookie     = "init_session"
	initSessionTTL = 30 * time.Minute
	initIPFailsMax = 5
	initLock       = 15 * time.Minute
)

var adminPathRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

type initDraft struct {
	accountSet bool
	username   string
	password   string // 明文只存内存草稿，confirm 时才哈希写盘
	adminPath  string
	storageSet bool
	storageOn  bool
	driver     string
	dsn        string
}

type initSession struct {
	expiry time.Time
}

// InitWizard 初始化向导：令牌门禁、内存草稿、confirm 一次性写盘。
// confirm 之前唯一的磁盘写入是启动期的令牌文件（门禁机制的一部分，
// 重启后令牌不变依赖它），向导本身的任何步骤都不落盘。
type InitWizard struct {
	mu           sync.Mutex
	token        string
	draft        initDraft
	sessions     map[string]initSession
	ipFails      map[string]int
	ipLocks      map[string]time.Time
	cfg          *config.Config
	instancePath string
	onConfirm    func(*PreparedStorage) error
	lg           *slog.Logger
}

// NewInitWizard 启动时调用。未初始化状态下确保令牌存在（复用或新生成
// 并写入 data 目录），已初始化则不做任何事。
func NewInitWizard(cfg *config.Config, instancePath, dataDir string, lg *slog.Logger) *InitWizard {
	w := &InitWizard{
		sessions:     map[string]initSession{},
		ipFails:      map[string]int{},
		ipLocks:      map[string]time.Time{},
		cfg:          cfg,
		instancePath: instancePath,
		lg:           lg,
	}
	if cfg.Setup.Completed {
		return w
	}
	tokenFile := filepath.Join(dataDir, initTokenFile)
	if b, err := os.ReadFile(tokenFile); err == nil && len(strings.TrimSpace(string(b))) >= 32 {
		w.token = strings.TrimSpace(string(b))
		return w
	}
	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		panic(fmt.Sprintf("crypto/rand 不可用: %v", err))
	}
	w.token = hex.EncodeToString(token)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		panic(fmt.Sprintf("创建数据目录失败: %v", err))
	}
	if err := os.WriteFile(tokenFile, []byte(w.token), 0o600); err != nil {
		panic(fmt.Sprintf("写初始化令牌失败: %v", err))
	}
	lg.Info("初始化令牌已生成（重启不变，完成初始化后失效）", "token", w.token)
	return w
}

// SetOnConfirm 注入 confirm 热挂载钩子：instance.yaml 写盘、内存配置
// 更新之后调用（用 Confirm 阶段建好的存储连接重建 engine 并整体替换）。
// 钩子失败时 Confirm 回滚内存配置并向导返回 50001，此时向导仍可用，
// 可修正后重新 confirm。
func (w *InitWizard) SetOnConfirm(fn func(*PreparedStorage) error) {
	w.mu.Lock()
	w.onConfirm = fn
	w.mu.Unlock()
}

// session 校验 init_session cookie，返回是否有效（顺带清理过期会话）。
func (w *InitWizard) sessionOK(c *gin.Context) bool {
	tok, err := c.Cookie(initCookie)
	if err != nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	s, ok := w.sessions[tok]
	if !ok {
		return false
	}
	if time.Now().After(s.expiry) {
		delete(w.sessions, tok)
		return false
	}
	return true
}

func (w *InitWizard) newSession(c *gin.Context) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand 不可用: %v", err))
	}
	tok := hex.EncodeToString(b)
	w.mu.Lock()
	w.sessions[tok] = initSession{expiry: time.Now().Add(initSessionTTL)}
	w.mu.Unlock()
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     initCookie,
		Value:    tok,
		Path:     "/init",
		MaxAge:   int(initSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   c.Request.TLS != nil,
	})
}

// ipAllowed 令牌失败限流：同一 IP 5 次锁 15 分钟。
func (w *InitWizard) ipAllowed(ip string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if until, ok := w.ipLocks[ip]; ok {
		if time.Now().Before(until) {
			return false
		}
		delete(w.ipLocks, ip)
		delete(w.ipFails, ip)
	}
	return true
}

func (w *InitWizard) ipFail(ip string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ipFails[ip]++
	if w.ipFails[ip] >= initIPFailsMax {
		w.ipLocks[ip] = time.Now().Add(initLock)
		delete(w.ipFails, ip)
	}
}

// State GET /init/api/state → { completed, has_token }
func (w *InitWizard) State() gin.HandlerFunc {
	return func(c *gin.Context) {
		w.mu.Lock()
		hasToken := w.token != ""
		completed := w.cfg.Setup.Completed
		w.mu.Unlock()
		api.OK(c, gin.H{"completed": completed, "has_token": hasToken})
	}
}

// Token POST /init/api/token body { token } → 会话 cookie；错误 5 次锁 IP。
func (w *InitWizard) Token() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := middleware.ClientIP(c.Request)
		if !w.ipAllowed(ip) {
			c.Header("Retry-After", "900")
			api.Fail(c, api.CodeTooManyRequests, "尝试过于频繁")
			return
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		w.mu.Lock()
		want := w.token
		w.mu.Unlock()
		// 常量时间比较，防时序侧信道。
		if subtle.ConstantTimeCompare([]byte(body.Token), []byte(want)) != 1 {
			w.ipFail(ip)
			api.Fail(c, api.CodeUnauthorized, "令牌错误")
			return
		}
		w.mu.Lock()
		w.ipFails[ip] = 0
		w.mu.Unlock()
		w.newSession(c)
		api.OK(c, gin.H{"ok": true})
	}
}

// requireSession 包裹四步流程与 confirm：无有效会话一律 401。
func (w *InitWizard) RequireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !w.sessionOK(c) {
			api.Fail(c, api.CodeUnauthorized, "初始化会话无效，请先提交令牌")
			return
		}
		c.Next()
	}
}

// Account POST /init/api/account body { username, password, admin_path }
func (w *InitWizard) Account() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Username  string `json:"username"`
			Password  string `json:"password"`
			AdminPath string `json:"admin_path"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		if len(body.Username) == 0 || len(body.Username) > 32 {
			api.Fail(c, api.CodeBadRequest, "用户名需 1 到 32 个字符")
			return
		}
		if err := middleware.ValidatePassword(body.Username, body.Password); err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		if !adminPathRe.MatchString(body.AdminPath) {
			api.Fail(c, api.CodeBadRequest, "admin_path 需 1 到 32 位字母、数字、下划线或连字符")
			return
		}
		w.mu.Lock()
		w.draft.accountSet = true
		w.draft.username = body.Username
		w.draft.password = body.Password
		w.draft.adminPath = body.AdminPath
		w.mu.Unlock()
		api.OK(c, gin.H{"ok": true})
	}
}

// Storage POST /init/api/storage body { enabled }
func (w *InitWizard) Storage() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		w.mu.Lock()
		w.draft.storageSet = true
		w.draft.storageOn = body.Enabled
		if !body.Enabled {
			w.draft.driver = ""
			w.draft.dsn = ""
		}
		w.mu.Unlock()
		api.OK(c, gin.H{"ok": true})
	}
}

// DatabaseTest POST /init/api/database/test body { driver, dsn } → { ok, message }
// 只做连通性检查（Ping），不建表；建表发生在 confirm。
func (w *InitWizard) DatabaseTest() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Driver string `json:"driver"`
			DSN    string `json:"dsn"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		st, err := store.Open(body.Driver, body.DSN)
		if err != nil {
			api.OK(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		if err := st.Ping(c.Request.Context()); err != nil {
			api.OK(c, gin.H{"ok": false, "message": err.Error()})
			return
		}
		api.OK(c, gin.H{"ok": true, "message": "连接成功"})
	}
}

// Database POST /init/api/database body { driver, dsn }
func (w *InitWizard) Database() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Driver string `json:"driver"`
			DSN    string `json:"dsn"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			api.Fail(c, api.CodeBadRequest, "请求体必须是 JSON")
			return
		}
		switch body.Driver {
		case "", "sqlite", "postgres", "mysql", "mongo":
		default:
			api.Fail(c, api.CodeBadRequest, "driver 非法")
			return
		}
		w.mu.Lock()
		w.draft.storageSet = true
		w.draft.storageOn = true
		w.draft.driver = body.Driver
		w.draft.dsn = body.DSN
		w.mu.Unlock()
		api.OK(c, gin.H{"ok": true})
	}
}

// PreparedStorage confirm 阶段提前建好的存储连接组：Init 成功后交给
// onConfirm 挂进新 engine，避免盘写完才发现库不可用。
type PreparedStorage struct {
	StatStore    *stats.Stats
	Counters     *store.CountingStore
	ArticleStore store.ArticleStore
}

// releaseStorage 释放连接组。ArticleStore 接口没有 Close（接口已冻结），
// 对实现了 Close 的具体类型做可选断言。
func releaseStorage(p *PreparedStorage) {
	if p == nil {
		return
	}
	if c, ok := p.ArticleStore.(interface{ Close() error }); ok {
		_ = c.Close()
	}
	if p.StatStore != nil {
		p.StatStore.Close()
	}
}

// prepareStorage 连接存储并建表。连接层失败（Open / Ping / 统计连接）
// 报「连接失败」，建表失败（Init）报「数据库初始化失败」。
// Ping 必须先于 Init：pgx 等驱动是惰性连接，死主机要等第一条语句才
// 报错，先 Ping 才能把「连不上」和「没权限建表」分开。
// 失败路径上已打开的连接全部释放（具体类型实现了可选 Close）。
func (w *InitWizard) prepareStorage(ctx context.Context, driver, dsn string) (*PreparedStorage, error) {
	st, err := store.Open(driver, dsn)
	if err != nil {
		return nil, errors.New("连接失败：" + err.Error())
	}
	fail := func(msg string) (*PreparedStorage, error) {
		if c, ok := st.(interface{ Close() error }); ok {
			_ = c.Close()
		}
		return nil, errors.New(msg)
	}
	if err := st.Ping(ctx); err != nil {
		return fail("连接失败：" + err.Error())
	}
	if err := st.Init(ctx); err != nil {
		return fail("数据库初始化失败：" + err.Error())
	}
	ss, serr := stats.New(driver, dsn, w.lg)
	if serr != nil {
		return fail("连接失败：" + serr.Error())
	}
	return &PreparedStorage{StatStore: ss, Counters: store.Count(st), ArticleStore: st}, nil
}

// Confirm POST /init/api/confirm：账号必填；先连接存储并建表，成功才写
// instance.yaml，随后内存配置同步生效并触发热挂载（整体替换 engine，
// 不重启进程）。存储失败返回 40001 不写盘，前端停在确认页可回去改；
// 热挂载失败回滚内存配置返回 50001，向导保持可用。
func (w *InitWizard) Confirm() gin.HandlerFunc {
	return func(c *gin.Context) {
		w.mu.Lock()
		d := w.draft
		onConfirm := w.onConfirm
		w.mu.Unlock()
		if !d.accountSet {
			api.Fail(c, api.CodeBadRequest, "请先完成账号步骤")
			return
		}
		hash, err := middleware.HashPassword(d.password)
		if err != nil {
			failInternal(c, err)
			return
		}

		// 先建库建表，成功才写盘。失败保持 40001，不产生任何磁盘写入。
		var prepared *PreparedStorage
		if d.storageOn {
			prepared, err = w.prepareStorage(c.Request.Context(), d.driver, d.dsn)
			if err != nil {
				api.Fail(c, api.CodeBadRequest, err.Error())
				return
			}
		}

		err = config.UpdateInstance(w.instancePath, func(doc *yaml.Node) error {
			steps := []struct {
				path []string
				val  any
			}{
				{[]string{"setup", "completed"}, true},
				{[]string{"admin", "username"}, d.username},
				{[]string{"admin", "password_hash"}, hash},
				{[]string{"server", "admin_path"}, d.adminPath},
				{[]string{"storage", "enabled"}, d.storageOn},
				{[]string{"storage", "driver"}, d.driver},
				{[]string{"storage", "dsn"}, d.dsn},
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

		// 内存配置同步生效。先改再挂载：buildEngine 要读到新值
		// （挂载管理路由、guard 判定 /init/* 为 404）。失败整体回滚。
		oldSetup, oldUser, oldHash := w.cfg.Setup.Completed, w.cfg.Admin.Username, w.cfg.Admin.PasswordHash
		oldPath, oldEnabled, oldDriver, oldDSN := w.cfg.Server.AdminPath, w.cfg.Storage.Enabled, w.cfg.Storage.Driver, w.cfg.Storage.DSN
		w.cfg.Setup.Completed = true
		w.cfg.Admin.Username = d.username
		w.cfg.Admin.PasswordHash = hash
		w.cfg.Server.AdminPath = d.adminPath
		w.cfg.Storage.Enabled = d.storageOn
		w.cfg.Storage.Driver = d.driver
		w.cfg.Storage.DSN = d.dsn
		if onConfirm != nil {
			if err := onConfirm(prepared); err != nil {
				w.cfg.Setup.Completed = oldSetup
				w.cfg.Admin.Username = oldUser
				w.cfg.Admin.PasswordHash = oldHash
				w.cfg.Server.AdminPath = oldPath
				w.cfg.Storage.Enabled = oldEnabled
				w.cfg.Storage.Driver = oldDriver
				w.cfg.Storage.DSN = oldDSN
				// 钩子失败，没有 engine 接管这份连接，释放。
				releaseStorage(prepared)
				failInternal(c, err)
				return
			}
		} else {
			// 无钩子（测试场景）：没有任何 engine 会接管，直接释放。
			releaseStorage(prepared)
		}

		_ = os.Remove(filepath.Join(filepath.Dir(w.instancePath), initTokenFile))
		api.OK(c, gin.H{"admin_path": d.adminPath})
	}
}

// Cancel POST /init/api/cancel：清空内存草稿。
func (w *InitWizard) Cancel() gin.HandlerFunc {
	return func(c *gin.Context) {
		w.mu.Lock()
		w.draft = initDraft{}
		w.mu.Unlock()
		api.OK(c, gin.H{"ok": true})
	}
}
