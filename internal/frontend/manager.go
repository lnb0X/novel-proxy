// Package frontend 管理前端版本目录：解析当前版本、读写指针文件、
// 提供可热替换的静态 handler。app 取 reader 或 admin。
package frontend

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	NotFound(w)
}

// NotFound 输出固定 HTML 404 页：零动态内容，no-store。
func NotFound(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(notFoundHTML))
}

// Manager 解析 data/frontends 下的版本目录。
type Manager struct {
	base string

	mu          sync.Mutex
	swaps       map[string]*SwapHandler
	prefixes    map[string]string
	spaFallback map[string]bool
	pending     map[string]*pendingSwitch
	lg          *slog.Logger
}

// New 构建 manager，base 是版本根目录（如 ./data/frontends）。
func New(base string) *Manager {
	return &Manager{
		base:        base,
		swaps:       map[string]*SwapHandler{},
		prefixes:    map[string]string{},
		spaFallback: map[string]bool{},
		pending:     map[string]*pendingSwitch{},
	}
}

// SetLogger 注入生命周期日志（自动回滚等脱离请求上下文的场景用）。
func (m *Manager) SetLogger(lg *slog.Logger) {
	m.lg = lg
}

// Mount 登记某 app 的可换 handler 与它的 URL 前缀（reader 为 "/"，
// admin 为 "/<secret>"），Reload 时按前缀解析路径。
// spaFallback：reader 用 BrowserRouter，未命中无扩展名路径回落 index.html。
func (m *Manager) Mount(app string, swap *SwapHandler, urlPrefix string, spaFallback bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.swaps[app] = swap
	m.prefixes[app] = urlPrefix
	m.spaFallback[app] = spaFallback
}

// Reload 重新解析 app 目录并替换内存中的 handler；解析失败回到固定 404 页。
func (m *Manager) Reload(app string) {
	m.mu.Lock()
	swap := m.swaps[app]
	prefix := m.prefixes[app]
	m.mu.Unlock()
	if swap == nil {
		return
	}
	if dir, ok := m.Resolve(app); ok {
		if m.spaFallback[app] {
			swap.Set(ReaderFileServer(dir, prefix))
		} else {
			swap.Set(FileServer(dir, prefix))
		}
		return
	}
	swap.Set(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { NotFound(w) }))
}

// VersionInfo GET /frontends 里 versions[] 的一项。
type VersionInfo struct {
	Version   string `json:"version"`
	Size      int64  `json:"size"`
	Files     int    `json:"files"`
	CreatedAt int64  `json:"created_at"`
}

// ListVersions 列出 app 下已上传的版本目录（不含 default 与临时目录）。
func (m *Manager) ListVersions(app string) []VersionInfo {
	root := filepath.Join(m.base, app)
	entries, err := os.ReadDir(root)
	if err != nil {
		return []VersionInfo{}
	}
	out := []VersionInfo{}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "default" || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		var size int64
		var files int
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if info, ierr := d.Info(); ierr == nil {
				size += info.Size()
				files++
			}
			return nil
		})
		info, ierr := e.Info()
		var created int64
		if ierr == nil {
			created = info.ModTime().Unix()
		}
		out = append(out, VersionInfo{Version: e.Name(), Size: size, Files: files, CreatedAt: created})
	}
	return out
}

// CurrentBasis 返回当前生效版本名（指针版本 / default / 空串）。
func (m *Manager) CurrentBasis(app string) string {
	return m.currentBasis(app)
}

// DeleteVersion 删除版本目录；若是待确认版本则一并取消。
func (m *Manager) DeleteVersion(app, version string) error {
	// version 来自 URL 参数，先挡掉路径穿越。
	if err := validateVersion(version); err != nil {
		return err
	}
	m.mu.Lock()
	if p := m.pending[app]; p != nil && p.version == version {
		p.timer.Stop()
		delete(m.pending, app)
	}
	m.mu.Unlock()
	return os.RemoveAll(filepath.Join(m.base, app, version))
}

// ReadPrevious 读 data/frontends/<app>-previous.json。
func (m *Manager) ReadPrevious(app string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(m.base, app+"-previous.json"))
	if err != nil {
		return "", false
	}
	var p struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &p) != nil {
		return "", false
	}
	// 空串是合法回滚点：表示回到「无前端」状态（清指针）。
	return p.Version, true
}

// WritePrevious 写回滚点，.tmp 加 rename 原子替换。
func (m *Manager) WritePrevious(app, version string) error {
	return m.writeMarker(app+"-previous.json", version)
}

// ClearPointer 删除 current 指针（回落 default 或无）。
func (m *Manager) ClearPointer(app string) error {
	err := os.Remove(filepath.Join(m.base, app+"-current.json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (m *Manager) writeMarker(name, version string) error {
	if err := os.MkdirAll(m.base, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(struct {
		Version string `json:"version"`
	}{Version: version})
	if err != nil {
		return err
	}
	final := filepath.Join(m.base, name)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// currentBasis 返回当前生效的版本名（指针版本或 default）。
func (m *Manager) currentBasis(app string) string {
	if v, ok := m.ReadPointer(app); ok {
		return v
	}
	if isDir(filepath.Join(m.base, app, "default")) {
		return "default"
	}
	return ""
}

type pendingSwitch struct {
	version string
	timer   *time.Timer
}

// validateVersion 版本名只允许单个路径段：挡住 "."、".."、分隔符与
// 绝对路径，Activate 与 DeleteVersion 共用。
func validateVersion(version string) error {
	if version == "" || version == "." || version == ".." ||
		strings.ContainsAny(version, "/\\") || filepath.IsAbs(version) {
		return fmt.Errorf("非法版本名 %q", version)
	}
	return nil
}

// Activate 把版本切为当前：记录回滚点、重写指针、替换内存 handler，
// within 内未 Confirm 则自动回滚。
func (m *Manager) Activate(app, version string, within time.Duration) (previous string, err error) {
	if err := validateVersion(version); err != nil {
		return "", err
	}
	if !isDir(filepath.Join(m.base, app, version)) {
		return "", fmt.Errorf("版本 %s 不存在", version)
	}
	prev := m.currentBasis(app)
	if err := m.WritePrevious(app, prev); err != nil {
		return "", err
	}
	if err := m.WritePointer(app, version); err != nil {
		return "", err
	}
	m.Reload(app)

	m.mu.Lock()
	if old := m.pending[app]; old != nil {
		old.timer.Stop()
	}
	p := &pendingSwitch{version: version}
	p.timer = time.AfterFunc(within, func() {
		m.mu.Lock()
		cur := m.pending[app]
		if cur == nil || cur.version != version {
			m.mu.Unlock()
			return
		}
		delete(m.pending, app)
		m.mu.Unlock()
		if rerr := m.Rollback(app); rerr == nil && m.lg != nil {
			m.lg.Info("frontend.auto-rollback", "app", app, "from", version, "to", prev)
		}
	})
	m.pending[app] = p
	m.mu.Unlock()
	return prev, nil
}

// Confirm 确认本次切换，取消自动回滚。
func (m *Manager) Confirm(app, version string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pending[app]
	if p == nil || p.version != version {
		return fmt.Errorf("没有待确认的切换或版本不符")
	}
	p.timer.Stop()
	delete(m.pending, app)
	return nil
}

// Rollback 回到上一个版本；回滚点与被回滚版本互换，可反复回滚。
func (m *Manager) Rollback(app string) error {
	m.mu.Lock()
	if p := m.pending[app]; p != nil {
		p.timer.Stop()
		delete(m.pending, app)
	}
	m.mu.Unlock()

	prev, ok := m.ReadPrevious(app)
	if !ok {
		return fmt.Errorf("没有可用的回滚点")
	}
	from := m.currentBasis(app)
	if prev == "default" || prev == "" {
		// 回到 default 或「无前端」状态都通过清除指针实现。
		if err := m.ClearPointer(app); err != nil {
			return err
		}
	} else {
		if err := m.WritePointer(app, prev); err != nil {
			return err
		}
	}
	if err := m.WritePrevious(app, from); err != nil {
		return err
	}
	m.Reload(app)
	return nil
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

// FileServer 返回目录静态服务：先剥掉挂载前缀（urlPrefix，"/" 表示根），
// 找不到或命中目录一律空 body 404，不开目录列表。URL 路径用 path.Clean
// （正斜杠语义，Windows 下也正确），清洗后 Join 进 dir，不逃出目录。
// 用 ServeContent 伺服，避免 ServeFile 对 URL 路径的规范化重定向。
func FileServer(dir, urlPrefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		if urlPrefix != "" && urlPrefix != "/" {
			p = strings.TrimPrefix(p, urlPrefix)
			if p == "." {
				p = "/"
			}
		}
		rel := strings.TrimPrefix(p, "/")
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

// ReaderFileServer 带SPA兜底的静态服务：无扩展名的路径未命中时
// 回落 index.html（DECISIONS「阅读端用 BrowserRouter（有 SPA 兜底）」），
// 带扩展名的资源（js/css/图片）未命中仍为空 body 404。
func ReaderFileServer(dir, urlPrefix string) http.Handler {
	inner := FileServer(dir, urlPrefix)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if rel == "" || rel == "." {
			rel = "index.html"
		}
		if !strings.Contains(rel, ".") {
			// 无扩展名 → 路由路径，回落 index.html。
			index := filepath.Join(dir, "index.html")
			if info, err := os.Stat(index); err == nil && !info.IsDir() {
				f, err := os.Open(index)
				if err == nil {
					defer f.Close()
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.Header().Set("Cache-Control", "no-store")
					http.ServeContent(w, r, "index.html", info.ModTime(), f)
					return
				}
			}
		}
		inner.ServeHTTP(w, r)
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
