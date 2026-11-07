// Package frontend 管理前端版本目录：解析当前版本、读写指针文件、
// 提供可热替换的静态 handler。app 取 reader 或 admin。
package frontend

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// notFoundHTML 固定 404 页，零动态内容。未配置任何前端目录时，
// 非 /api 路径一律返回它。
const notFoundHTML = `<!DOCTYPE html>
<html lang="zh">
<head><meta charset="utf-8"><title>404</title></head>
<body><p>404 页面不存在</p></body>
</html>
`

// SwapHandler 原子持有当前静态 handler；未设置时返回固定 404 页。
type SwapHandler struct {
	h atomic.Pointer[http.Handler]
}

// Set 替换当前 handler。
func (s *SwapHandler) Set(h http.Handler) {
	s.h.Store(&h)
}

func (s *SwapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h := s.h.Load(); h != nil {
		(*h).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(notFoundHTML))
}

// Manager 解析 data/frontends 下的版本目录。
type Manager struct {
	base string
}

// New 构建 manager，base 是版本根目录（如 ./data/frontends）。
func New(base string) *Manager {
	return &Manager{base: base}
}

// Resolve 返回 app 当前应伺服的目录：
// 指针文件指向的版本目录（必须存在）> default/ 兜底 > 无（false）。
func (m *Manager) Resolve(app string) (string, bool) {
	if v, ok := m.ReadPointer(app); ok {
		dir := filepath.Join(m.base, app, v)
		if isDir(dir) {
			return dir, true
		}
	}
	dir := filepath.Join(m.base, app, "default")
	if isDir(dir) {
		return dir, true
	}
	return "", false
}

// ReadPointer 读 data/frontends/<app>-current.json，内容 {"version": "..."}。
func (m *Manager) ReadPointer(app string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(m.base, app+"-current.json"))
	if err != nil {
		return "", false
	}
	var p struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &p) != nil || p.Version == "" {
		return "", false
	}
	return p.Version, true
}

// WritePointer 先写 .tmp 再 os.Rename，保证指针文件不出现半截内容。
func (m *Manager) WritePointer(app, version string) error {
	b, err := json.Marshal(struct {
		Version string `json:"version"`
	}{Version: version})
	if err != nil {
		return err
	}
	final := filepath.Join(m.base, app+"-current.json")
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// FileServer 返回目录静态服务：找不到或命中目录一律空 body 404，
// 不开目录列表。URL 路径用 path.Clean（正斜杠语义，Windows 下也正确），
// 清洗后 Join 进 dir，不逃出目录。用 ServeContent 伺服，
// 避免 ServeFile 对 URL 路径的规范化重定向。
func FileServer(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if rel == "" || rel == "." {
			rel = "index.html"
		}
		full := filepath.Join(dir, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			empty404(w)
			return
		}
		f, err := os.Open(full)
		if err != nil {
			empty404(w)
			return
		}
		defer f.Close()
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	})
}

// empty404 空 body 404。只 WriteHeader 不写 body 时，gin 会在 NoRoute
// 链末尾补它的默认 404 文本，Flush 一次把空 body 定下来。
func empty404(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
