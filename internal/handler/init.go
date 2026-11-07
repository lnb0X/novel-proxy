package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
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
		defer func() { _ = st.Ping(c.Request.Context()) }()
		if err := st.Init(c.Request.Context()); err != nil {
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

// Confirm POST /init/api/confirm：账号必填；一次性写 instance.yaml，
// 内存配置同步生效，删除令牌文件。
func (w *InitWizard) Confirm() gin.HandlerFunc {
	return func(c *gin.Context) {
		w.mu.Lock()
		d := w.draft
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

		// 内存配置同步生效；admin 路由与存储连接按约定重启后挂载。
		w.cfg.Setup.Completed = true
		w.cfg.Admin.Username = d.username
		w.cfg.Admin.PasswordHash = hash
		w.cfg.Server.AdminPath = d.adminPath
		w.cfg.Storage.Enabled = d.storageOn
		w.cfg.Storage.Driver = d.driver
		w.cfg.Storage.DSN = d.dsn

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
