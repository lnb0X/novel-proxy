package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/frontend"
)

func newFrontendsEnv(t *testing.T) (*gin.Engine, *frontend.Manager, string) {
	t.Helper()
	base := t.TempDir()
	m := frontend.New(base)
	gin.SetMode(gin.TestMode)
	lg := slog.New(slog.NewJSONHandler(io.Discard, nil))
	r := gin.New()
	r.Use(api.SlogMiddleware(lg, ""))
	r.POST("/api/frontends/:app/upload", UploadFrontend(m, nil))
	r.POST("/api/frontends/:app/activate", ActivateFrontend(m, nil))
	r.POST("/api/frontends/:app/confirm", ConfirmFrontend(m, nil))
	r.POST("/api/frontends/:app/rollback", RollbackFrontend(m, nil))
	r.DELETE("/api/frontends/:app/:version", DeleteFrontend(m, nil))
	r.GET("/api/frontends", ListFrontends(m))
	return r, m, base
}

func uploadZip(t *testing.T, r *gin.Engine, app string, files map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "site.zip")
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(fw)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/frontends/"+app+"/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestFrontendsEndpoints(t *testing.T) {
	r, m, base := newFrontendsEnv(t)

	// 上传合法 zip。
	w := uploadZip(t, r, "reader", map[string]string{
		"index.html": "<p>v1</p>",
		"app.js":     "1",
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"version":"`) {
		t.Fatalf("上传失败: %d %s", w.Code, w.Body.String())
	}
	var up struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &up); err != nil {
		t.Fatal(err)
	}
	version := up.Data.Version
	if !isDirPath(filepath.Join(base, "reader", version)) {
		t.Fatal("版本目录未出现")
	}

	// 非法 app。
	w = uploadZip(t, r, "nope", map[string]string{"index.html": "x"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 app 应 40001: %d", w.Code)
	}

	// activate v1 → current 变 v1。
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/frontends/reader/activate", strings.NewReader(`{"version":"`+version+`","confirm_within_seconds":60}`))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("activate = %d %s", w.Code, w.Body.String())
	}

	// GET 列表里 current 为该版本。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/frontends", nil))
	if !strings.Contains(w.Body.String(), `"current":"`+version+`"`) {
		t.Fatalf("current 应为新版本: %s", w.Body.String())
	}

	// confirm。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/frontends/reader/confirm", strings.NewReader(`{"version":"`+version+`"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("confirm = %d %s", w.Code, w.Body.String())
	}

	// rollback → 指针清空（回 default，目录无 default 时 current 为空串）。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/frontends/reader/rollback", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("rollback = %d %s", w.Code, w.Body.String())
	}
	if _, ok := m.ReadPointer("reader"); ok {
		t.Fatal("回滚后指针应清空")
	}

	// delete 版本目录消失。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/frontends/reader/"+version, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", w.Code)
	}
	if isDirPath(filepath.Join(base, "reader", version)) {
		t.Fatal("删除后目录应消失")
	}
	// 删除后列表为空。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/frontends", nil))
	if strings.Contains(w.Body.String(), `"version":"`+version+`"`) {
		t.Fatalf("删除后不应再列出: %s", w.Body.String())
	}
}

func TestUploadRejectsBadZips(t *testing.T) {
	r, _, base := newFrontendsEnv(t)

	// 缺 index.html。
	w := uploadZip(t, r, "admin", map[string]string{"other.html": "x"})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "index.html") {
		t.Fatalf("缺 index.html 应 40001: %d %s", w.Code, w.Body.String())
	}
	// zip slip。
	w = uploadZip(t, r, "admin", map[string]string{"index.html": "x", "../evil.txt": "y"})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "穿越") {
		t.Fatalf("穿越应 40001: %d %s", w.Code, w.Body.String())
	}
	// 不留文件。
	entries, _ := os.ReadDir(filepath.Join(base, "admin"))
	if len(entries) != 0 {
		t.Fatalf("失败上传不应留文件: %v", entries)
	}
	// 不是 zip。
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/frontends/admin/upload", strings.NewReader("hello"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("垃圾 body 应 40001: %d", w.Code)
	}
}

func isDirPath(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
