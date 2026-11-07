package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/middleware"
)

const testAdminPath = "x7k2p9"

// newAdminEnv 搭一个完整的管理端测试环境：config + instance 临时目录 + 认证链。
func newAdminEnv(t *testing.T) (*gin.Engine, *middleware.AuthManager, *config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	instancePath := filepath.Join(dir, "instance.yaml")
	hash, err := middleware.HashPassword("Abcdef1!Abcde")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{}
	cfg.Server.AdminPath = testAdminPath
	cfg.Flags = config.FlagsConfig{Search: true, Detail: true, Catalog: true, Content: true}
	cfg.Admin.Username = "boss"
	cfg.Admin.PasswordHash = hash

	ipFilter, err := middleware.NewIPFilter(config.IPFilterConfig{Mode: "off", Scope: "all"}, testAdminPath)
	if err != nil {
		t.Fatal(err)
	}
	flagGate := middleware.NewFlags(cfg.Flags)
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimit, testAdminPath)
	t.Cleanup(rateLimiter.Stop)

	gin.SetMode(gin.TestMode)
	auth := middleware.NewAuthManager(testAdminPath)
	lg := slog.New(slog.NewJSONHandler(io.Discard, nil))
	r := gin.New()
	r.Use(api.SlogMiddleware(lg, testAdminPath), middleware.Auth(auth))
	r.POST("/"+testAdminPath+"/api/auth/login", Login(auth, &cfg))
	r.POST("/"+testAdminPath+"/api/auth/logout", Logout(auth))
	r.GET("/"+testAdminPath+"/api/auth/me", Me())
	r.POST("/"+testAdminPath+"/api/auth/revoke", Revoke(auth))
	r.POST("/"+testAdminPath+"/api/account/password", ChangePassword(auth, &cfg, instancePath))
	r.GET("/"+testAdminPath+"/api/ip-filter", GetIPFilter(ipFilter))
	r.PUT("/"+testAdminPath+"/api/ip-filter", PutIPFilter(ipFilter, &cfg, instancePath))
	r.GET("/"+testAdminPath+"/api/flags", GetFlags(flagGate))
	r.PUT("/"+testAdminPath+"/api/flags/:name", PutFlag(flagGate, &cfg, instancePath))
	r.GET("/"+testAdminPath+"/api/rate-limits", GetRateLimits(rateLimiter))
	r.PUT("/"+testAdminPath+"/api/rate-limits", PutRateLimits(rateLimiter, &cfg, instancePath))
	return r, auth, &cfg, instancePath
}

// postAdmin 发一个管理端写请求，自带 X-Admin-Request 头与可选 cookie。
func postAdmin(r *gin.Engine, path, body, tokenCookie string, withHeader bool) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if withHeader {
		req.Header.Set("X-Admin-Request", "1")
	}
	req.Header.Set("Content-Type", "application/json")
	if tokenCookie != "" {
		req.AddCookie(&http.Cookie{Name: "token", Value: tokenCookie})
	}
	r.ServeHTTP(w, req)
	return w
}

// loginAndGetToken 用正确密码登录，返回 Set-Cookie 里的 token。
func loginAndGetToken(t *testing.T, r *gin.Engine) string {
	t.Helper()
	w := postAdmin(r, "/"+testAdminPath+"/api/auth/login", `{"username":"boss","password":"Abcdef1!Abcde"}`, "", true)
	if w.Code != http.StatusOK {
		t.Fatalf("登录失败: %d %s", w.Code, w.Body.String())
	}
	cks := w.Result().Cookies()
	for _, ck := range cks {
		if ck.Name == "token" && ck.Value != "" {
			return ck.Value
		}
	}
	t.Fatal("登录响应没有 token cookie")
	return ""
}

func TestLoginSetsSecureCookie(t *testing.T) {
	r, _, _, _ := newAdminEnv(t)
	w := postAdmin(r, "/"+testAdminPath+"/api/auth/login", `{"username":"boss","password":"Abcdef1!Abcde"}`, "", true)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var ck *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "token" {
			ck = c
		}
	}
	if ck == nil {
		t.Fatal("缺 token cookie")
	}
	if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie 属性不符: %+v", ck)
	}
	if ck.MaxAge != 1800 {
		t.Fatalf("Max-Age = %d", ck.MaxAge)
	}
	// data: { username, expires_at, sat }
	var e struct {
		Code int `json:"code"`
		Data struct {
			Username  string `json:"username"`
			ExpiresAt int64  `json:"expires_at"`
			Sat       int64  `json:"sat"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Code != 0 || e.Data.Username != "boss" || e.Data.ExpiresAt == 0 || e.Data.Sat == 0 {
		t.Fatalf("登录 data 不符: %s", w.Body.String())
	}
}

func TestLoginWrongPasswordAndLockout(t *testing.T) {
	r, _, _, _ := newAdminEnv(t)
	for i := 0; i < 5; i++ {
		w := postAdmin(r, "/"+testAdminPath+"/api/auth/login", `{"username":"boss","password":"Wrong123!pass"}`, "", true)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次: HTTP %d", i+1, w.Code)
		}
		if !strings.Contains(w.Body.String(), "账号或密码错误") {
			t.Fatalf("msg 不符: %s", w.Body.String())
		}
	}
	w := postAdmin(r, "/"+testAdminPath+"/api/auth/login", `{"username":"boss","password":"Abcdef1!Abcde"}`, "", true)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("锁定后应 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("429 应带 Retry-After")
	}
}

func TestLoginRequiresXAdminRequest(t *testing.T) {
	r, _, _, _ := newAdminEnv(t)
	w := postAdmin(r, "/"+testAdminPath+"/api/auth/login", `{"username":"boss","password":"Abcdef1!Abcde"}`, "", false)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "40101") {
		t.Fatalf("缺 X-Admin-Request 应 401 + 40101: %d %s", w.Code, w.Body.String())
	}
}

func TestMeWithoutToken(t *testing.T) {
	r, _, _, _ := newAdminEnv(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/"+testAdminPath+"/api/auth/me", nil))
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "40101") {
		t.Fatalf("未登录 me 应 401: %d %s", w.Code, w.Body.String())
	}
	// 拒绝后 me handler 不得执行：响应里不应出现认证数据。
	if strings.Contains(w.Body.String(), "perm") {
		t.Fatalf("拒绝后不应有第二个响应体: %s", w.Body.String())
	}
}

func TestMeWithToken(t *testing.T) {
	r, _, _, _ := newAdminEnv(t)
	token := loginAndGetToken(t, r)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+testAdminPath+"/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("me = %d %s", w.Code, w.Body.String())
	}
	var e struct {
		Data struct {
			Username string `json:"username"`
			Perm     string `json:"perm"`
			Exp      int64  `json:"exp"`
			Sat      int64  `json:"sat"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Data.Username != "boss" || e.Data.Perm != "admin" || e.Data.Exp == 0 || e.Data.Sat == 0 {
		t.Fatalf("me data 不符: %s", w.Body.String())
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	r, _, _, _ := newAdminEnv(t)
	token := loginAndGetToken(t, r)
	w := postAdmin(r, "/"+testAdminPath+"/api/auth/logout", "", token, true)
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", w.Code)
	}
	var clear *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "token" {
			clear = c
		}
	}
	if clear == nil || clear.MaxAge >= 0 {
		t.Fatalf("logout 应下发清除 cookie: %v", clear)
	}
}

func TestRevokeInvalidatesOldToken(t *testing.T) {
	r, _, _, _ := newAdminEnv(t)
	token := loginAndGetToken(t, r)
	w := postAdmin(r, "/"+testAdminPath+"/api/auth/revoke", "", token, true)
	if w.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d", w.Code)
	}
	// 旧 token 立即失效。
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+testAdminPath+"/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("revoke 后旧 token 应 401, got %d", w2.Code)
	}
}

func TestChangePassword(t *testing.T) {
	r, _, cfg, instancePath := newAdminEnv(t)
	token := loginAndGetToken(t, r)

	// 旧密码错误。
	w := postAdmin(r, "/"+testAdminPath+"/api/account/password",
		`{"old_password":"nope123456!A","new_password":"Zx9$mVq2Zw81"}`, token, true)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "旧密码错误") {
		t.Fatalf("旧密码错误分支: %d %s", w.Code, w.Body.String())
	}

	// 新密码不过策略。
	w = postAdmin(r, "/"+testAdminPath+"/api/account/password",
		`{"old_password":"Abcdef1!Abcde","new_password":"short1A!"}`, token, true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("策略拒绝: %d %s", w.Code, w.Body.String())
	}

	// 成功：写 instance.yaml + 轮换 key + 清 cookie。
	w = postAdmin(r, "/"+testAdminPath+"/api/account/password",
		`{"old_password":"Abcdef1!Abcde","new_password":"Zx9$mVq2Zw81"}`, token, true)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "密码已修改") {
		t.Fatalf("改密成功分支: %d %s", w.Code, w.Body.String())
	}
	b, err := os.ReadFile(instancePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "password_hash") {
		t.Fatalf("instance.yaml 未写入 hash: %s", b)
	}
	if !middleware.CheckPassword(cfg.Admin.PasswordHash, "Zx9$mVq2Zw81") {
		t.Fatal("内存配置应更新为新 hash")
	}
	// 旧 token 已因轮换失效。
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+testAdminPath+"/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("改密后旧 token 应 401: %d", w2.Code)
	}
	// 新密码可登录（注意：测试共用一个假 IP，登录失败锁定计数已被成功登录清零）。
	w3 := postAdmin(r, "/"+testAdminPath+"/api/auth/login", `{"username":"boss","password":"Zx9$mVq2Zw81"}`, "", true)
	if w3.Code != http.StatusOK {
		t.Fatalf("新密码登录应成功: %d %s", w3.Code, w3.Body.String())
	}
}

func TestPutIPFilterLockdown(t *testing.T) {
	r, _, _, instancePath := newAdminEnv(t)
	token := loginAndGetToken(t, r)

	// httptest 默认来源是 192.0.2.1，名单不含它时 allow 模式拒绝保存。
	w := putAdmin(r, "/"+testAdminPath+"/api/ip-filter",
		`{"mode":"allow","scope":"all","rules":["10.0.0.0/8"]}`, token)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "拒绝保存") {
		t.Fatalf("锁死检查应拒绝: %d %s", w.Code, w.Body.String())
	}

	// 名单包含当前 IP：保存成功，热更生效且写入 instance.yaml。
	w = putAdmin(r, "/"+testAdminPath+"/api/ip-filter",
		`{"mode":"allow","scope":"api","rules":["192.0.2.1"]}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("含当前 IP 应保存成功: %d %s", w.Code, w.Body.String())
	}
	w = getAdmin(r, "/"+testAdminPath+"/api/ip-filter", token)
	if !strings.Contains(w.Body.String(), `"mode":"allow"`) || !strings.Contains(w.Body.String(), "192.0.2.1") {
		t.Fatalf("GET 应读到新配置: %s", w.Body.String())
	}
	b, err := os.ReadFile(instancePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "ip_filter") || !strings.Contains(string(b), "192.0.2.1") {
		t.Fatalf("instance.yaml 未写入: %s", b)
	}

	// 非法 mode。
	w = putAdmin(r, "/"+testAdminPath+"/api/ip-filter",
		`{"mode":"deny","scope":"all","rules":[]}`, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 mode 应 40001: %d", w.Code)
	}
	// 非法 CIDR。
	w = putAdmin(r, "/"+testAdminPath+"/api/ip-filter",
		`{"mode":"block","scope":"all","rules":["abc"]}`, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法规则应 40001: %d", w.Code)
	}
}

// putAdmin 发管理端 PUT 请求。
func putAdmin(r *gin.Engine, path, body, tokenCookie string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("X-Admin-Request", "1")
	req.Header.Set("Content-Type", "application/json")
	if tokenCookie != "" {
		req.AddCookie(&http.Cookie{Name: "token", Value: tokenCookie})
	}
	r.ServeHTTP(w, req)
	return w
}

// getAdmin 发管理端 GET 请求。
func getAdmin(r *gin.Engine, path, tokenCookie string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if tokenCookie != "" {
		req.AddCookie(&http.Cookie{Name: "token", Value: tokenCookie})
	}
	r.ServeHTTP(w, req)
	return w
}

func TestFlagsEndpoints(t *testing.T) {
	r, _, cfg, instancePath := newAdminEnv(t)
	token := loginAndGetToken(t, r)

	w := getAdmin(r, "/"+testAdminPath+"/api/flags", token)
	if !strings.Contains(w.Body.String(), `"search":true`) {
		t.Fatalf("初始 flags 应全开: %s", w.Body.String())
	}
	w = putAdmin(r, "/"+testAdminPath+"/api/flags/search", `{"enabled":false}`, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT flags = %d %s", w.Code, w.Body.String())
	}
	if cfg.Flags.Search {
		t.Fatal("内存配置应同步关闭")
	}
	w = putAdmin(r, "/"+testAdminPath+"/api/flags/nope", `{"enabled":false}`, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("未知开关名应 40001: %d", w.Code)
	}
	b, err := os.ReadFile(instancePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "search: false") {
		t.Fatalf("instance.yaml 未写入: %s", b)
	}
	// 重启持久化：从写盘的 instance 重新 Load，search 应为 false。
	dir := filepath.Dir(instancePath)
	cp := filepath.Join(dir, "..", "config.yaml")
	os.WriteFile(cp, []byte(""), 0o644)
	loaded, err := config.Load(cp, instancePath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Flags.Search {
		t.Fatal("重启后关闭状态应保持")
	}
}

func TestRateLimitsEndpoints(t *testing.T) {
	r, _, cfg, instancePath := newAdminEnv(t)
	token := loginAndGetToken(t, r)

	body := `{"search":{"mode":"global","per_minute":2,"burst":2},` +
		`"detail":{"mode":"off","per_minute":0,"burst":0},` +
		`"catalog":{"mode":"off","per_minute":0,"burst":0},` +
		`"content":{"mode":"off","per_minute":0,"burst":0}}`
	w := putAdmin(r, "/"+testAdminPath+"/api/rate-limits", body, token)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT rate-limits = %d %s", w.Code, w.Body.String())
	}
	if cfg.RateLimit.Search.PerMinute != 2 {
		t.Fatal("内存配置应同步")
	}
	b, err := os.ReadFile(instancePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "per_minute: 2") {
		t.Fatalf("instance.yaml 未写入: %s", b)
	}
	// 非法 mode。
	w = putAdmin(r, "/"+testAdminPath+"/api/rate-limits",
		`{"search":{"mode":"nope","per_minute":0,"burst":0},"detail":{"mode":"off","per_minute":0,"burst":0},"catalog":{"mode":"off","per_minute":0,"burst":0},"content":{"mode":"off","per_minute":0,"burst":0}}`, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 mode 应 40001: %d", w.Code)
	}
}
