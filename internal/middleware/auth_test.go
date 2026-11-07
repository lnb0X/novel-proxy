package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func TestIssueAndParse(t *testing.T) {
	m := NewAuthManager("x7k2p9")
	token, exp, sat := m.Issue("boss")

	cl, err := m.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if cl.Subject != "boss" || cl.Perm != "admin" {
		t.Fatalf("claims 不符: %+v", cl)
	}
	// NumericDate 按秒截断，比较允许一秒内误差。
	if abs(cl.ExpiresAt.Time.Sub(exp)) > time.Second || cl.Sat != sat.Unix() {
		t.Fatalf("exp/sat 不符: %v %v", cl.ExpiresAt.Time, sat)
	}
	if d := time.Until(exp); d < 29*time.Minute || d > 30*time.Minute {
		t.Fatalf("exp 应为 30 分钟: %v", d)
	}
	if d := time.Until(sat); d < 11*time.Hour || d > 12*time.Hour {
		t.Fatalf("sat 应为 12 小时: %v", d)
	}
}

func TestParseRejectsAfterRotate(t *testing.T) {
	m := NewAuthManager("x7k2p9")
	token, _, _ := m.Issue("boss")
	if _, err := m.Parse(token); err != nil {
		t.Fatal(err)
	}
	m.Rotate()
	if _, err := m.Parse(token); err == nil {
		t.Fatal("轮换 key 后旧 token 应失效")
	}
}

func TestParseRejectsForeignManager(t *testing.T) {
	// 重启 = 新 manager = 新 key，旧 token 全部失效。
	m1 := NewAuthManager("x7k2p9")
	token, _, _ := m1.Issue("boss")
	m2 := NewAuthManager("x7k2p9")
	if _, err := m2.Parse(token); err == nil {
		t.Fatal("重启后旧 token 应失效")
	}
}

func TestParseRejectsExpiredSat(t *testing.T) {
	m := NewAuthManager("x7k2p9")
	// exp 仍有效但 sat 已过：绝对上限一到必须拒绝。
	token := m.sign("boss", time.Now().Add(-time.Hour), jwtTTL)
	if _, err := m.Parse(token); err == nil {
		t.Fatal("sat 过期应拒绝")
	}
}

func TestParseRejectsOtherAlgorithm(t *testing.T) {
	m := NewAuthManager("x7k2p9")
	cl := authClaims{
		Perm: "admin",
		Sat:  time.Now().Add(time.Hour).Unix(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "boss",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS512, cl).SignedString(*m.key.Load())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Parse(signed); err == nil {
		t.Fatal("非 HS256 算法应拒绝")
	}
}

func TestRenewalCapsAtSat(t *testing.T) {
	m := NewAuthManager("x7k2p9")
	sat := time.Now().Add(5 * time.Minute)
	token, exp := m.Renew("boss", sat)
	if !exp.Equal(sat) {
		t.Fatalf("续签不得越过 sat: exp=%v sat=%v", exp, sat)
	}
	if _, err := m.Parse(token); err != nil {
		t.Fatalf("续签 token 应可解析: %v", err)
	}
}

func TestCookieFlags(t *testing.T) {
	m := NewAuthManager("x7k2p9")
	r := httptest.NewRequest(http.MethodPost, "/x7k2p9/api/auth/login", nil)
	ck := m.AuthCookie(r, "tok")
	if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie 安全属性不符: %+v", ck)
	}
	if ck.Path != "/x7k2p9" || ck.MaxAge != 1800 {
		t.Fatalf("Path/Max-Age 不符: %+v", ck)
	}
	if ck.Secure {
		t.Fatal("非 TLS 请求不应带 Secure")
	}
	rTLS := httptest.NewRequest(http.MethodPost, "/x7k2p9/api/auth/login", nil)
	rTLS.TLS = &tls.ConnectionState{}
	if !m.AuthCookie(rTLS, "tok").Secure {
		t.Fatal("HTTPS 下应带 Secure")
	}
	if clear := m.ClearCookie(r); clear.MaxAge >= 0 {
		t.Fatalf("清除 cookie 的 Max-Age 应为负: %d", clear.MaxAge)
	}
}

// newAuthEnv 构建只挂 Auth 中间件的 gin 引擎。
func newAuthEnv(m *AuthManager) (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Auth(m))
	r.GET("/x7k2p9/api/auth/me", func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	return r, w
}

func TestRenewalWindowInMiddleware(t *testing.T) {
	m := NewAuthManager("x7k2p9")

	do := func(ttl time.Duration, sat time.Time) *httptest.ResponseRecorder {
		engine, w := newAuthEnv(m)
		req := httptest.NewRequest(http.MethodGet, "/x7k2p9/api/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "token", Value: m.sign("boss", sat, ttl)})
		engine.ServeHTTP(w, req)
		return w
	}

	// 剩余 20 分钟：不续签。
	if len(do(20*time.Minute, time.Now().Add(sessionCap)).Result().Cookies()) != 0 {
		t.Fatal("剩余充足时不应续签")
	}
	// 剩余 5 分钟：随响应续签。
	w := do(5*time.Minute, time.Now().Add(sessionCap))
	cks := w.Result().Cookies()
	if len(cks) == 0 || cks[0].Name != "token" {
		t.Fatalf("剩余不足 10 分钟应带新 cookie: %v", cks)
	}
	if _, err := m.Parse(cks[0].Value); err != nil {
		t.Fatalf("续签 cookie 应是有效 token: %v", err)
	}
	// 剩余 5 分钟且 sat 只剩 8 分钟：续签 exp 不得越过 sat。
	w2 := do(5*time.Minute, time.Now().Add(8*time.Minute))
	ck2 := w2.Result().Cookies()[0]
	cl, err := m.Parse(ck2.Value)
	if err != nil {
		t.Fatal(err)
	}
	if cl.ExpiresAt.Time.After(time.Now().Add(8*time.Minute)) {
		t.Fatalf("续签越过 sat: %v", cl.ExpiresAt.Time)
	}
}

func TestAuthMiddlewareNonGetHeader(t *testing.T) {
	m := NewAuthManager("x7k2p9")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Auth(m))
	ran := false
	mark := func(c *gin.Context) { ran = true; c.Status(200) }
	r.POST("/x7k2p9/api/other", mark)
	r.POST("/x7k2p9/api/auth/login", mark)

	// 非 GET 且无 X-Admin-Request：40101，且下游 handler 不得执行。
	ran = false
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/x7k2p9/api/other", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("缺 X-Admin-Request 应 401: %d", w.Code)
	}
	if ran {
		t.Fatal("被拒绝后下游 handler 不应执行")
	}
	// 带 header 放行（此路径仍需 token，会被拒）。
	ran = false
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/x7k2p9/api/other", nil)
	req2.Header.Set("X-Admin-Request", "1")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("带 header 缺 token 仍应 401: %d", w2.Code)
	}
	if ran {
		t.Fatal("token 无效时下游 handler 不应执行")
	}
	// login 免 token 但 header 必须有。
	ran = false
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest(http.MethodPost, "/x7k2p9/api/auth/login", nil))
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("login 缺 X-Admin-Request 应 401: %d", w3.Code)
	}
	if ran {
		t.Fatal("login 被拒后 handler 不应执行")
	}
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodPost, "/x7k2p9/api/auth/login", nil)
	req4.Header.Set("X-Admin-Request", "1")
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK || !ran {
		t.Fatalf("login 带 header 应放行: %d ran=%v", w4.Code, ran)
	}
	// 非 admin 路径不受影响。
	gin2 := gin.New()
	gin2.Use(Auth(m))
	gin2.GET("/api/search", func(c *gin.Context) { c.Status(200) })
	w5 := httptest.NewRecorder()
	gin2.ServeHTTP(w5, httptest.NewRequest(http.MethodGet, "/api/search", nil))
	if w5.Code != http.StatusOK {
		t.Fatalf("非 admin 路径不应拦截: %d", w5.Code)
	}
	// 管理端页面路径（非 /api/*）免 token：未登录也能加载 SPA。
	gin3 := gin.New()
	gin3.Use(Auth(m))
	gin3.GET("/x7k2p9/", func(c *gin.Context) { c.String(200, "spa") })
	w6 := httptest.NewRecorder()
	gin3.ServeHTTP(w6, httptest.NewRequest(http.MethodGet, "/x7k2p9/", nil))
	if w6.Code != http.StatusOK {
		t.Fatalf("管理端页面不应拦 token: %d", w6.Code)
	}
}

func TestLoginLockout(t *testing.T) {
	m := NewAuthManager("x7k2p9")

	for i := 0; i < ipFailsMax; i++ {
		if _, ok := m.LoginAllowed("1.1.1.1", "u"); !ok {
			t.Fatalf("第 %d 次不应锁定", i+1)
		}
		m.LoginFailure("1.1.1.1", "u")
	}
	if wait, ok := m.LoginAllowed("1.1.1.1", "u"); ok || wait <= 0 {
		t.Fatal("IP 满 5 次失败应锁定")
	}
	if _, ok := m.LoginAllowed("2.2.2.2", "u"); !ok {
		t.Fatal("其他 IP 不应被连带锁定")
	}

	for i := 0; i < userFailsMax; i++ {
		m.LoginFailure("3.3.3.3", "boss")
	}
	if _, ok := m.LoginAllowed("9.9.9.9", "boss"); ok {
		t.Fatal("账号满 10 次失败应锁定")
	}

	m2 := NewAuthManager("x7k2p9")
	for i := 0; i < ipFailsMax-1; i++ {
		m2.LoginFailure("4.4.4.4", "u")
	}
	m2.LoginSuccess("4.4.4.4", "u")
	if _, ok := m2.LoginAllowed("4.4.4.4", "u"); !ok {
		t.Fatal("成功登录后计数应清零")
	}
}

func TestValidatePassword(t *testing.T) {
	good := []string{"Abcdef1!Abcde", "xK9$mVq2Zw81", "L0ng-Passw0rd-XX"}
	for _, p := range good {
		if err := ValidatePassword("boss", p); err != nil {
			t.Errorf("%q 应通过: %v", p, err)
		}
	}
	bad := []struct {
		p    string
		want string
	}{
		{"short1A!", "长度"},
		{"alllowercase12", "三类"},
		{"NOLOWERCLASS11", "三类"},
		{"abcdefgh1234", "三类"},
		{"bossPassword1!", "用户名"},
		{"bOsSrul3s!Abc", "用户名"},
		{"xK9$mVq2Zw81password", "弱口令"},
		{"Qwerty123456!x", "弱口令"},
		{"Admin123456!xk", "弱口令"},
	}
	for _, c := range bad {
		err := ValidatePassword("boss", c.p)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q 应因 %s 拒绝, got %v", c.p, c.want, err)
		}
	}
}
