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
	cfg.Admin.Username = "boss"
	cfg.Admin.PasswordHash = hash

	gin.SetMode(gin.TestMode)
	auth := middleware.NewAuthManager(testAdminPath)
	lg := slog.New(slog.NewJSONHandler(io.Discard, nil))
	r := gin.New()
	r.Use(api.SlogMiddleware(lg, testAdminPath), middleware.Auth(auth))
	r.POST("/" + testAdminPath + "/api/auth/login", Login(auth, &cfg))
	r.POST("/" + testAdminPath + "/api/auth/logout", Logout(auth))
	r.GET("/" + testAdminPath + "/api/auth/me", Me())
	r.POST("/" + testAdminPath + "/api/auth/revoke", Revoke(auth))
	r.POST("/"+testAdminPath+"/api/account/password", ChangePassword(auth, &cfg, instancePath))
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
