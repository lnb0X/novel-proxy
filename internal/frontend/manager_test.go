package frontend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkdir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveNone(t *testing.T) {
	m := New(t.TempDir())
	if _, ok := m.Resolve("reader"); ok {
		t.Fatal("空目录不应解析出前端目录")
	}
}

func TestResolveDefaultFallback(t *testing.T) {
	base := t.TempDir()
	def := mkdir(t, filepath.Join(base, "reader", "default"))
	writeFile(t, filepath.Join(def, "index.html"), "<p>default</p>")

	m := New(base)
	dir, ok := m.Resolve("reader")
	if !ok || dir != def {
		t.Fatalf("应回落 default, got %q ok=%v", dir, ok)
	}
}

func TestResolvePointerWins(t *testing.T) {
	base := t.TempDir()
	v1 := mkdir(t, filepath.Join(base, "reader", "v1"))
	mkdir(t, filepath.Join(base, "reader", "default"))

	m := New(base)
	if err := m.WritePointer("reader", "v1"); err != nil {
		t.Fatal(err)
	}
	dir, ok := m.Resolve("reader")
	if !ok || dir != v1 {
		t.Fatalf("指针应生效, got %q ok=%v", dir, ok)
	}
}

func TestResolveDanglingPointerFallsBack(t *testing.T) {
	base := t.TempDir()
	def := mkdir(t, filepath.Join(base, "reader", "default"))

	m := New(base)
	if err := m.WritePointer("reader", "v9"); err != nil {
		t.Fatal(err)
	}
	dir, ok := m.Resolve("reader")
	if !ok || dir != def {
		t.Fatalf("悬空指针应回落 default, got %q ok=%v", dir, ok)
	}
}

func TestPointerRoundTrip(t *testing.T) {
	base := t.TempDir()
	m := New(base)
	if _, ok := m.ReadPointer("admin"); ok {
		t.Fatal("无指针文件时不应读到版本")
	}
	if err := m.WritePointer("admin", "20260101"); err != nil {
		t.Fatal(err)
	}
	v, ok := m.ReadPointer("admin")
	if !ok || v != "20260101" {
		t.Fatalf("roundtrip = %q %v", v, ok)
	}
	// 重写覆盖旧值。
	if err := m.WritePointer("admin", "20260102"); err != nil {
		t.Fatal(err)
	}
	if v, _ := m.ReadPointer("admin"); v != "20260102" {
		t.Fatalf("重写后 = %q", v)
	}
	// 不留 .tmp 残留。
	entries, _ := os.ReadDir(base)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("残留临时文件: %s", e.Name())
		}
	}
}

func TestSwapHandlerUnsetServesFixed404(t *testing.T) {
	s := &SwapHandler{}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("固定 404 页应带 no-store")
	}
	if !strings.Contains(w.Body.String(), "404") || strings.Contains(w.Body.String(), "01J") {
		t.Fatalf("固定页不应含动态内容: %s", w.Body.String())
	}
}

func TestSwapHandlerSwitch(t *testing.T) {
	base := t.TempDir()
	dirA := mkdir(t, filepath.Join(base, "a"))
	writeFile(t, filepath.Join(dirA, "index.html"), "<p>A</p>")
	dirB := mkdir(t, filepath.Join(base, "b"))
	writeFile(t, filepath.Join(dirB, "index.html"), "<p>B</p>")

	s := &SwapHandler{}
	s.Set(FileServer(dirA, "/"))
	get := func() string {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		return w.Body.String()
	}
	if !strings.Contains(get(), "A") {
		t.Fatal("应伺服 A")
	}
	// 模拟重新加载：重新 Set 新目录后立即生效。
	s.Set(FileServer(dirB, "/"))
	if !strings.Contains(get(), "B") {
		t.Fatal("切换后应伺服 B")
	}
}

func TestFileServer(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), "<p>home</p>")
	writeFile(t, filepath.Join(dir, "assets", "app.js"), "console.log(1)")
	mkdir(t, filepath.Join(dir, "nolisting"))

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		FileServer(dir, "/").ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	if w := get("/"); w.Code != 200 || !strings.Contains(w.Body.String(), "home") {
		t.Fatalf("/ = %d %s", w.Code, w.Body.String())
	}
	if w := get("/assets/app.js"); w.Code != 200 || w.Body.String() != "console.log(1)" {
		t.Fatalf("/assets/app.js = %d %s", w.Code, w.Body.String())
	}
	// 找不到：空 body 404。
	if w := get("/nope.js"); w.Code != 404 || w.Body.Len() != 0 {
		t.Fatalf("未命中应空 body 404: %d %q", w.Code, w.Body.String())
	}
	// 目录列表关闭。
	if w := get("/nolisting"); w.Code != 404 || w.Body.Len() != 0 {
		t.Fatalf("目录列表应 404: %d %q", w.Code, w.Body.String())
	}
	// 穿越段被 Clean 规整后仍落在目录内的真实文件上。
	if w := get("/sub/../index.html"); w.Code != 200 || !strings.Contains(w.Body.String(), "home") {
		t.Fatalf("穿越路径应被清理后正常命中: %d %s", w.Code, w.Body.String())
	}
	if w := get("/../outside.txt"); w.Code != 404 {
		t.Fatalf("目录外文件不可达: %d", w.Code)
	}
}
