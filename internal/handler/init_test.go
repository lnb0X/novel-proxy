package handler

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

// 热挂载钩子：confirm 在写盘与内存配置更新后调用钩子；钩子失败时
// 内存配置整体回滚、返回 50001，向导保持可用并可重新 confirm。
func TestInitConfirmHotSwapHook(t *testing.T) {
	r, w, cfg, dataDir, instancePath := newInitEnv(t, false)
	session, _ := getToken(t, r, w)

	for _, step := range []struct {
		path string
		body string
	}{
		{"/init/api/account", `{"username":"boss","password":"Abcdef1!Abcde","admin_path":"adm"}`},
		{"/init/api/storage", `{"enabled":false}`},
	} {
		resp := postInit(r, step.path, step.body, session)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.path, resp.Code, resp.Body.String())
		}
	}

	// 钩子失败：配置回滚、50001、令牌文件保留、可重新 confirm。
	var calls int
	w.SetOnConfirm(func(*PreparedStorage) error {
		calls++
		return errors.New("模拟热挂载失败")
	})
	resp := postInit(r, "/init/api/confirm", "", session)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("钩子失败应 50001: %d %s", resp.Code, resp.Body.String())
	}
	if calls != 1 {
		t.Fatalf("钩子应被调用一次, got %d", calls)
	}
	if cfg.Setup.Completed || cfg.Server.AdminPath != "" {
		t.Fatalf("钩子失败后内存配置应回滚: %+v", cfg)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "init-token.txt")); err != nil {
		t.Fatalf("钩子失败后令牌文件应保留: %v", err)
	}
	// 向导仍可用：换成功钩子重新 confirm。
	w.SetOnConfirm(func(*PreparedStorage) error { return nil })
	resp = postInit(r, "/init/api/confirm", "", session)
	if resp.Code != http.StatusOK {
		t.Fatalf("重试 confirm 应成功: %d %s", resp.Code, resp.Body.String())
	}
	if !cfg.Setup.Completed {
		t.Fatalf("成功后内存配置应生效: %+v", cfg)
	}
	// instance.yaml 已写盘。
	if _, err := os.Stat(instancePath); err != nil {
		t.Fatalf("instance.yaml 应存在: %v", err)
	}
}

// confirm 阶段存储不可用：先建库失败，返回 40001 + 原因，
// 不产生任何磁盘写入（instance.yaml 不存在）。
// 127.0.0.1:1 端口连接被拒，断网与沙箱环境下行为一致。
func TestInitConfirmStorageFailNoWrite(t *testing.T) {
	r, w, _, _, instancePath := newInitEnv(t, false)
	session, _ := getToken(t, r, w)

	for _, step := range []struct {
		path string
		body string
	}{
		{"/init/api/account", `{"username":"boss","password":"Abcdef1!Abcde","admin_path":"adm"}`},
		{"/init/api/storage", `{"enabled":true}`},
		{"/init/api/database", `{"driver":"postgres","dsn":"postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"}`},
	} {
		resp := postInit(r, step.path, step.body, session)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.path, resp.Code, resp.Body.String())
		}
	}
	resp := postInit(r, "/init/api/confirm", "", session)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("存储失败应 40001: %d %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "连接失败") {
		t.Fatalf("应写明连接失败原因: %s", resp.Body.String())
	}
	if _, err := os.Stat(instancePath); !os.IsNotExist(err) {
		t.Fatalf("confirm 失败不应写 instance.yaml: %v", err)
	}
}

// database/test 只做连通性检查：连不上的库返回 ok=false，
// 连得上的库（sqlite 内存）返回 ok=true，全程不建表。
func TestInitDatabaseTestPingOnly(t *testing.T) {
	r, w, _, _, _ := newInitEnv(t, false)
	session, _ := getToken(t, r, w)
	resp := postInit(r, "/init/api/database/test", `{"driver":"postgres","dsn":"postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"}`, session)
	if resp.Code != http.StatusOK {
		t.Fatalf("test 应 200 封装: %d %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"ok":false`) {
		t.Fatalf("不可达库应 ok=false: %s", resp.Body.String())
	}
	resp = postInit(r, "/init/api/database/test", `{"driver":"sqlite","dsn":":memory:"}`, session)
	if !strings.Contains(resp.Body.String(), `"ok":true`) {
		t.Fatalf("sqlite 内存库应 ok=true: %s", resp.Body.String())
	}
}

// confirm 热挂载失败时预建连接组必须释放：stats 写入协程与连接池不能
// 随重试累积成僵尸。连续三次失败，goroutine 数回到基线（±2 抖动）。
func TestInitConfirmFailureReleasesGoroutines(t *testing.T) {
	r, w, _, _, _ := newInitEnv(t, false)
	session, _ := getToken(t, r, w)

	for _, step := range []struct {
		path string
		body string
	}{
		{"/init/api/account", `{"username":"boss","password":"Abcdef1!Abcde","admin_path":"adm"}`},
		{"/init/api/storage", `{"enabled":true}`},
		{"/init/api/database", `{"driver":"sqlite","dsn":":memory:"}`},
	} {
		resp := postInit(r, step.path, step.body, session)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.path, resp.Code, resp.Body.String())
		}
	}

	// 钩子始终失败：prepareStorage 已成功、后续热挂载失败的情形。
	w.SetOnConfirm(func(*PreparedStorage) error { return errors.New("热挂载失败") })

	runtime.GC()
	base := runtime.NumGoroutine()
	for i := 0; i < 3; i++ {
		resp := postInit(r, "/init/api/confirm", "", session)
		if resp.Code != http.StatusInternalServerError {
			t.Fatalf("第 %d 次 confirm 应 50001: %d %s", i+1, resp.Code, resp.Body.String())
		}
		// Close 同步 drain，容忍调度抖动，短暂等待回落。
		deadline := time.Now().Add(time.Second)
		for runtime.NumGoroutine() > base+2 && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if now := runtime.NumGoroutine(); now > base+2 || now < base-2 {
			t.Fatalf("第 %d 次失败后 goroutine 未回基线: base=%d now=%d", i+1, base, now)
		}
	}
}
