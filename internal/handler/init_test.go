package handler

import (
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
)

func newInitEnv(t *testing.T, precompleted bool) (*gin.Engine, *InitWizard, *config.Config, string, string) {
	t.Helper()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	instancePath := filepath.Join(dataDir, "instance.yaml")
	cfg := config.Config{}
	cfg.Setup.Completed = precompleted
	lg := slog.New(slog.NewJSONHandler(io.Discard, nil))
	w := NewInitWizard(&cfg, instancePath, dataDir, lg)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(api.SlogMiddleware(lg, ""))
	r.GET("/init/api/state", w.State())
	r.POST("/init/api/token", w.Token())
	sec := r.Group("/init/api", w.RequireSession())
	sec.POST("/account", w.Account())
	sec.POST("/storage", w.Storage())
	sec.POST("/database/test", w.DatabaseTest())
	sec.POST("/database", w.Database())
	sec.POST("/confirm", w.Confirm())
	sec.POST("/cancel", w.Cancel())
	return r, w, &cfg, dataDir, instancePath
}

func postInit(r *gin.Engine, path, body, session string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if session != "" {
		req.AddCookie(&http.Cookie{Name: "init_session", Value: session})
	}
	r.ServeHTTP(w, req)
	return w
}

// getTokenCookie 提交正确令牌并返回会话 cookie 值；同时返回印在日志里的令牌。
func getToken(t *testing.T, r *gin.Engine, w *InitWizard) (session, token string) {
	t.Helper()
	w.mu.Lock()
	token = w.token
	w.mu.Unlock()
	resp := postInit(r, "/init/api/token", `{"token":"`+token+`"}`, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("令牌提交失败: %d %s", resp.Code, resp.Body.String())
	}
	for _, c := range resp.Result().Cookies() {
		if c.Name == "init_session" {
			return c.Value, token
		}
	}
	t.Fatal("无 init_session cookie")
	return "", ""
}

func TestInitTokenReuseAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	instancePath := filepath.Join(dataDir, "instance.yaml")
	cfg := config.Config{}
	lg := slog.New(slog.NewJSONHandler(io.Discard, nil))

	w1 := NewInitWizard(&cfg, instancePath, dataDir, lg)
	w1.mu.Lock()
	t1 := w1.token
	w1.mu.Unlock()
	if t1 == "" {
		t.Fatal("未生成令牌")
	}
	// 模拟重启：同一 data 目录再建一个 wizard，令牌必须不变。
	w2 := NewInitWizard(&cfg, instancePath, dataDir, lg)
	w2.mu.Lock()
	t2 := w2.token
	w2.mu.Unlock()
	if t1 != t2 {
		t.Fatalf("重启后令牌变了: %s vs %s", t1, t2)
	}
}

func TestInitTokenWrongFiveTimesLocks(t *testing.T) {
	r, w, _, _, _ := newInitEnv(t, false)
	w.mu.Lock()
	w.token = "correct-token-abcdef"
	w.mu.Unlock()

	for i := 0; i < 5; i++ {
		resp := postInit(r, "/init/api/token", `{"token":"wrong"}`, "")
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("第 %d 次错误令牌应 401: %d", i+1, resp.Code)
		}
	}
	resp := postInit(r, "/init/api/token", `{"token":"correct-token-abcdef"}`, "")
	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("5 次失败后正确令牌也应 429: %d", resp.Code)
	}
	if resp.Header().Get("Retry-After") == "" {
		t.Fatal("429 应带 Retry-After")
	}
}

func TestInitWizardFullFlow(t *testing.T) {
	r, w, cfg, dataDir, instancePath := newInitEnv(t, false)
	session, _ := getToken(t, r, w)

	// 无会话访问向导步骤应 401。
	resp := postInit(r, "/init/api/account", `{"username":"boss","password":"Abcdef1!Abcde","admin_path":"adm"}`, "")
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("无会话应 401: %d", resp.Code)
	}

	// 弱密码拒绝。
	resp = postInit(r, "/init/api/account", `{"username":"boss","password":"short1A!","admin_path":"adm"}`, session)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("弱密码应 40001: %d %s", resp.Code, resp.Body.String())
	}
	// 非法 admin_path 拒绝。
	resp = postInit(r, "/init/api/account", `{"username":"boss","password":"Abcdef1!Abcde","admin_path":"a b"}`, session)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("非法 admin_path 应 40001: %d", resp.Code)
	}

	// confirm 前不应有任何 instance.yaml。
	if _, err := os.Stat(instancePath); !os.IsNotExist(err) {
		t.Fatalf("confirm 之前不应写 instance.yaml: %v", err)
	}

	// 四步：账号、存储开关、数据库、confirm。
	for _, step := range []struct {
		path string
		body string
	}{
		{"/init/api/account", `{"username":"boss","password":"Abcdef1!Abcde","admin_path":"adm"}`},
		{"/init/api/storage", `{"enabled":true}`},
		{"/init/api/database", `{"driver":"sqlite","dsn":"` + filepath.ToSlash(filepath.Join(dataDir, "cache.db")) + `"}`},
	} {
		resp = postInit(r, step.path, step.body, session)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.path, resp.Code, resp.Body.String())
		}
	}
	resp = postInit(r, "/init/api/confirm", "", session)
	if resp.Code != http.StatusOK {
		t.Fatalf("confirm = %d %s", resp.Code, resp.Body.String())
	}

	// instance.yaml 出现且 setup.completed 为 true，内存配置同步。
	b, err := os.ReadFile(instancePath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"completed: true", "boss", "adm", "sqlite"} {
		if !strings.Contains(s, want) {
			t.Fatalf("instance.yaml 缺 %q:\n%s", want, s)
		}
	}
	if !cfg.Setup.Completed || cfg.Server.AdminPath != "adm" {
		t.Fatalf("内存配置未生效: %+v", cfg)
	}

	// 令牌文件删除。
	if _, err := os.Stat(filepath.Join(dataDir, "init-token.txt")); !os.IsNotExist(err) {
		t.Fatal("confirm 后令牌文件应删除")
	}
}
