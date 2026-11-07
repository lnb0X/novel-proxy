// Package middleware 承载请求链上的各中间件与认证设施。
// 固定顺序见 DECISIONS：trace_id → recover → ipfilter → initguard →
// secret → auth → flags → ratelimit；ipfilter/initguard/flags/ratelimit
// 随对应任务接入。
package middleware

import (
	"crypto/rand"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"novel-proxy/internal/api"
)

const (
	jwtTTL        = 30 * time.Minute
	sessionCap    = 12 * time.Hour
	renewWindow   = 10 * time.Minute
	cookieMaxAge  = 1800
	cookieName    = "token"
	bcryptCost    = 12
	ipFailsMax    = 5
	userFailsMax  = 10
	lockDuration  = 15 * time.Minute
	permAdmin     = "admin"
	adminHeader   = "X-Admin-Request"
	adminHeaderV  = "1"
)

// AuthManager 持有 JWT 签名 key 与登录失败锁定状态。
// key 只在内存，atomic.Pointer 支持并发轮换；重启即失效。
type AuthManager struct {
	adminPath string
	key       atomic.Pointer[[]byte]

	mu        sync.Mutex
	ipFails   map[string]int
	userFails map[string]int
	ipLocks   map[string]time.Time
	userLocks map[string]time.Time
}

// NewAuthManager 生成 32 字节随机签名 key。
func NewAuthManager(adminPath string) *AuthManager {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("crypto/rand 不可用: %v", err))
	}
	m := &AuthManager{
		adminPath: adminPath,
		ipFails:   map[string]int{},
		userFails: map[string]int{},
		ipLocks:   map[string]time.Time{},
		userLocks: map[string]time.Time{},
	}
	m.key.Store(&key)
	return m
}

// Rotate 轮换签名 key，所有已签发 token 立即失效。
func (m *AuthManager) Rotate() {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(fmt.Sprintf("crypto/rand 不可用: %v", err))
	}
	m.key.Store(&key)
}

type authClaims struct {
	Perm string `json:"perm"`
	Sat  int64  `json:"sat"`
	jwt.RegisteredClaims
}

// Issue 签发新会话 token：exp = iat + 30 分钟，sat = iat + 12 小时。
func (m *AuthManager) Issue(username string) (string, time.Time, time.Time) {
	now := time.Now()
	return m.sign(username, now.Add(sessionCap), jwtTTL), now.Add(jwtTTL), now.Add(sessionCap)
}

// Renew 续签：exp 不得越过 sat。
func (m *AuthManager) Renew(username string, sat time.Time) (string, time.Time) {
	ttl := jwtTTL
	if sat.Before(time.Now().Add(jwtTTL)) {
		ttl = time.Until(sat)
	}
	return m.sign(username, sat, ttl), time.Now().Add(ttl)
}

func (m *AuthManager) sign(username string, sat time.Time, ttl time.Duration) string {
	now := time.Now()
	cl := authClaims{
		Perm: permAdmin,
		Sat:  sat.Unix(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, cl)
	signed, err := t.SignedString(*m.key.Load())
	if err != nil {
		panic(fmt.Sprintf("签名失败: %v", err))
	}
	return signed
}

// Parse 校验 token：仅 HS256，且当前签名 key 必须能验签，
// sat 已过即失效（绝对上限）。
func (m *AuthManager) Parse(token string) (*authClaims, error) {
	cl := &authClaims{}
	_, err := jwt.ParseWithClaims(token, cl, func(t *jwt.Token) (any, error) {
		return *m.key.Load(), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, fmt.Errorf("token 无效: %w", err)
	}
	if cl.Perm != permAdmin {
		return nil, fmt.Errorf("token 权限不符")
	}
	if time.Now().After(time.Unix(cl.Sat, 0)) {
		return nil, fmt.Errorf("会话已超过绝对上限")
	}
	return cl, nil
}

// AuthCookie 管理端会话 cookie。
func (m *AuthManager) AuthCookie(r *http.Request, token string) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/" + m.adminPath,
		MaxAge:   cookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	}
}

// ClearCookie 清除会话 cookie。
func (m *AuthManager) ClearCookie(r *http.Request) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/" + m.adminPath,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	}
}

// LoginAllowed 返回是否允许尝试；锁定中返回剩余锁定时长。
func (m *AuthManager) LoginAllowed(ip, username string) (time.Duration, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if until, ok := m.ipLocks[ip]; ok {
		if now.Before(until) {
			return until.Sub(now), false
		}
		delete(m.ipLocks, ip)
		delete(m.ipFails, ip)
	}
	if until, ok := m.userLocks[username]; ok {
		if now.Before(until) {
			return until.Sub(now), false
		}
		delete(m.userLocks, username)
		delete(m.userFails, username)
	}
	return 0, true
}

// LoginFailure 记一次失败：IP 满 5 次或账号满 10 次锁 15 分钟。
// 表上限 10000 是故意设的：用户名攻击者可控，发不同用户名即可无限
// 增长且永不回收，超过上限后放弃新增计数（已锁定的照常生效）。
const failMapMax = 10000

func (m *AuthManager) LoginFailure(ip, username string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ipFails[ip]++
	if m.ipFails[ip] >= ipFailsMax {
		m.ipLocks[ip] = time.Now().Add(lockDuration)
		delete(m.ipFails, ip)
	}
	if len(m.userFails) >= failMapMax {
		if _, exists := m.userFails[username]; !exists {
			return
		}
	}
	m.userFails[username]++
	if m.userFails[username] >= userFailsMax {
		m.userLocks[username] = time.Now().Add(lockDuration)
		delete(m.userFails, username)
	}
}

// LoginSuccess 清空该 IP 与账号的失败计数。
func (m *AuthManager) LoginSuccess(ip, username string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.ipFails, ip)
	delete(m.userFails, username)
}

// Auth 管理端路径的认证中间件：
// 非 GET 要求 X-Admin-Request: 1；login / logout 免 token；
// 其余校验 JWT，剩余有效期不足 10 分钟时随响应续签。
func Auth(m *AuthManager) gin.HandlerFunc {
	adminPrefix := "/" + m.adminPath
	exempt := map[string]bool{
		"/api/auth/login":  true,
		"/api/auth/logout": true,
	}
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if p != adminPrefix && !strings.HasPrefix(p, adminPrefix+"/") {
			c.Next()
			return
		}
		rel := strings.TrimPrefix(p, adminPrefix)
		// 管理端页面路径（非 /api/*）不设防：页面要能加载，
		// 未登录跳转由前端根据 /api/auth/me 的 401 完成。
		if !strings.HasPrefix(rel, "/api/") {
			c.Next()
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Header.Get(adminHeader) != adminHeaderV {
			api.Fail(c, api.CodeUnauthorized, "缺少 "+adminHeader+" 请求头")
			return
		}
		if !exempt[rel] {
			token, err := c.Cookie(cookieName)
			if err != nil {
				api.Fail(c, api.CodeUnauthorized, "未登录或登录已过期")
				return
			}
			cl, err := m.Parse(token)
			if err != nil {
				api.Fail(c, api.CodeUnauthorized, "未登录或登录已过期")
				return
			}
			c.Set("auth_user", cl.Subject)
			c.Set("auth_exp", cl.ExpiresAt.Unix())
			c.Set("auth_sat", cl.Sat)
			if time.Until(cl.ExpiresAt.Time) < renewWindow {
				token, _ := m.Renew(cl.Subject, time.Unix(cl.Sat, 0))
				http.SetCookie(c.Writer, m.AuthCookie(c.Request, token))
			}
		}
		c.Next()
	}
}

// ClientIP 取 RemoteAddr 的 host 部分，不解析 X-Forwarded-For。
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ValidatePassword 密码策略：长度至少 12；四类字符至少三类；
// 不得与用户名相同或包含用户名（忽略大小写）；拒绝常见弱口令。
func ValidatePassword(username, password string) error {
	if len(password) < 12 {
		return fmt.Errorf("密码长度至少 12 位")
	}
	var hasLower, hasUpper, hasDigit, hasSymbol bool
	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	classes := 0
	for _, b := range []bool{hasLower, hasUpper, hasDigit, hasSymbol} {
		if b {
			classes++
		}
	}
	if classes < 3 {
		return fmt.Errorf("密码需包含小写、大写、数字、符号中的至少三类")
	}
	if username != "" {
		lu := strings.ToLower(username)
		lp := strings.ToLower(password)
		if lp == lu || strings.Contains(lp, lu) {
			return fmt.Errorf("密码不得与用户名相同或包含用户名")
		}
	}
	lp := strings.ToLower(password)
	for _, w := range []string{"password", "qwerty", "123456", "admin", "letmein"} {
		if strings.Contains(lp, w) {
			return fmt.Errorf("密码包含常见弱口令，不允许使用")
		}
	}
	return nil
}

// HashPassword bcrypt 哈希，cost 12。
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(b), err
}

// CheckPassword 校验明文与哈希。
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
