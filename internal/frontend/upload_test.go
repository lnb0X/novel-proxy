package frontend

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeZip 生成内存 zip，files 为 name -> content。
func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
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
	return buf.Bytes()
}

func TestUploadValidZip(t *testing.T) {
	base := t.TempDir()
	m := New(base)
	info, err := m.Upload("reader", bytes.NewReader(makeZip(t, map[string]string{
		"index.html":      "<p>v1</p>",
		"assets/app.js":   "console.log(1)",
		"assets/sub/x.css": "body{}",
	})))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "reader", info.Version)
	if !isDir(dir) {
		t.Fatal("版本目录未出现")
	}
	if info.Files != 3 || info.Size != int64(len("<p>v1</p>")+len("console.log(1)")+len("body{}")) {
		t.Fatalf("统计不符: %+v", info)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if string(b) != "<p>v1</p>" {
		t.Fatalf("内容不符: %q", b)
	}
	// 列表可见。
	vs := m.ListVersions("reader")
	if len(vs) != 1 || vs[0].Version != info.Version {
		t.Fatalf("ListVersions = %+v", vs)
	}
}

func TestUploadMissingIndex(t *testing.T) {
	base := t.TempDir()
	m := New(base)
	_, err := m.Upload("reader", bytes.NewReader(makeZip(t, map[string]string{
		"page.html": "<p>x</p>",
	})))
	if err == nil || !containsStr(err.Error(), "index.html") {
		t.Fatalf("缺 index.html 应报错: %v", err)
	}
	// 不留任何文件。
	if _, err := os.Stat(filepath.Join(base, "reader")); !os.IsNotExist(err) {
		t.Fatal("失败上传不应留下目录")
	}
}

func TestUploadZipSlip(t *testing.T) {
	base := t.TempDir()
	m := New(base)
	for _, name := range []string{"../evil.txt", "a/../../evil.txt", "/abs/evil.txt"} {
		_, err := m.Upload("reader", bytes.NewReader(makeZip(t, map[string]string{
			"index.html": "<p>x</p>",
			name:         "bad",
		})))
		if err == nil {
			t.Fatalf("%q 应被拒绝", name)
		}
	}
	// 目标目录之外不能出现 evil.txt。
	if _, err := os.Stat(filepath.Join(filepath.Dir(base), "evil.txt")); !os.IsNotExist(err) {
		t.Fatal("穿越文件不应存在")
	}
	// base 内除 reader 目录外无残留 .tmp。
	entries2, _ := os.ReadDir(base)
	for _, e := range entries2 {
		if !e.IsDir() && hasSuffix(e.Name(), ".tmp") {
			t.Fatalf("残留临时文件: %s", e.Name())
		}
	}
}

func hasSuffix(s, suf string) bool { return len(s) >= len(suf) && s[len(s)-len(suf):] == suf }

func containsStr(s, sub string) bool { return index(s, sub) >= 0 }

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestUploadSizeLimits(t *testing.T) {
	old1, old2 := uploadMaxZipBytes, uploadMaxTotalBytes
	uploadMaxZipBytes = 100
	uploadMaxTotalBytes = 200
	defer func() { uploadMaxZipBytes, uploadMaxTotalBytes = old1, old2 }()

	base := t.TempDir()
	m := New(base)
	_, err := m.Upload("reader", bytes.NewReader(makeZip(t, map[string]string{
		"index.html": "<p>" + string(make([]byte, 300)) + "</p>",
	})))
	if err == nil || !containsStr(err.Error(), "50MB") {
		t.Fatalf("压缩包超限应报错: %v", err)
	}

	// 解压后超限：小 zip 谎报大小没关系，按实际写入计。
	uploadMaxZipBytes = 1 << 20
	_, err = m.Upload("reader", bytes.NewReader(makeZip(t, map[string]string{
		"index.html": "<p>x</p>",
		"big.bin":    string(make([]byte, 300)),
	})))
	if err == nil || !containsStr(err.Error(), "200MB") {
		t.Fatalf("解压后超限应报错: %v", err)
	}
}

func TestActivateAutoRollback(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "reader", "default", "index.html"), "<p>default</p>")
	writeFile(t, filepath.Join(base, "reader", "v1", "index.html"), "<p>v1</p>")
	writeFile(t, filepath.Join(base, "reader", "v2", "index.html"), "<p>v2</p>")

	m := New(base)
	swap := &SwapHandler{}
	m.Mount("reader", swap, "/")
	m.Reload("reader")
	body := func() string {
		w := httptest.NewRecorder()
		swap.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		return w.Body.String()
	}
	if !containsStr(body(), "default") {
		t.Fatalf("初始应伺服 default: %s", body())
	}

	// activate v1，1 秒内不确认 → 自动回滚。
	if _, err := m.Activate("reader", "v1", time.Second); err != nil {
		t.Fatal(err)
	}
	if !containsStr(body(), "v1") {
		t.Fatalf("activate 后应伺服 v1: %s", body())
	}
	if prev, ok := m.ReadPrevious("reader"); !ok || prev != "default" {
		t.Fatalf("回滚点应记 default: %q %v", prev, ok)
	}
	time.Sleep(1500 * time.Millisecond)
	if !containsStr(body(), "default") {
		t.Fatalf("超时未确认应自动回滚: %s", body())
	}
	if v, _ := m.ReadPointer("reader"); v != "" {
		t.Fatalf("回滚后指针应清除: %q", v)
	}

	// activate v2 + confirm → 保持 v2。
	if _, err := m.Activate("reader", "v2", time.Second); err != nil {
		t.Fatal(err)
	}
	if err := m.Confirm("reader", "v2"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	if !containsStr(body(), "v2") {
		t.Fatalf("confirm 后不应回滚: %s", body())
	}
	// confirm 版本不符。
	if err := m.Confirm("reader", "v9"); err == nil {
		t.Fatal("版本不符的 confirm 应报错")
	}
}

func TestRollbackToggles(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "reader", "default", "index.html"), "<p>default</p>")
	writeFile(t, filepath.Join(base, "reader", "v1", "index.html"), "<p>v1</p>")

	m := New(base)
	if err := m.WritePointer("reader", "v1"); err != nil {
		t.Fatal(err)
	}
	swap := &SwapHandler{}
	m.Mount("reader", swap, "/")
	m.Reload("reader")

	// 预置回滚点：从 v1 回到 default。
	if err := m.WritePrevious("reader", "default"); err != nil {
		t.Fatal(err)
	}
	if err := m.Rollback("reader"); err != nil {
		t.Fatal(err)
	}
	if v, _ := m.ReadPointer("reader"); v != "" {
		t.Fatalf("回滚到 default 应清指针: %q", v)
	}
	// 回滚点互换为被回滚的 v1，可再次回滚回 v1。
	if prev, _ := m.ReadPrevious("reader"); prev != "v1" {
		t.Fatalf("回滚点应互换: %q", prev)
	}
	if err := m.Rollback("reader"); err != nil {
		t.Fatal(err)
	}
	if v, _ := m.ReadPointer("reader"); v != "v1" {
		t.Fatalf("第二次回滚应回 v1: %q", v)
	}
	// 清掉回滚点后报错。
	os.Remove(filepath.Join(base, "reader-previous.json"))
	if err := m.Rollback("reader"); err == nil {
		t.Fatal("无回滚点应报错")
	}
}

func TestDeleteVersion(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "reader", "v1", "index.html"), "<p>v1</p>")
	m := New(base)

	if err := m.DeleteVersion("reader", "../escape"); err == nil {
		t.Fatal("穿越版本名应拒绝")
	}
	if err := m.DeleteVersion("reader", "v1"); err != nil {
		t.Fatal(err)
	}
	if isDir(filepath.Join(base, "reader", "v1")) {
		t.Fatal("删除后目录应消失")
	}
}
