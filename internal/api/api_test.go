package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	lg := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return lg, buf
}

func newTestEngine(lg *slog.Logger, mode string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SlogMiddleware(lg), RecoveryMiddleware(lg, mode))
	r.NoRoute(NoRoute)
	return r
}

func do(r *gin.Engine, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) Response {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应体不是合法 JSON: %v\n%s", err, w.Body.String())
	}
	return resp
}

func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if ln == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("日志行不是合法 JSON: %v\n%s", err, ln)
		}
		lines = append(lines, m)
	}
	return lines
}

func TestEncodeULIDVectors(t *testing.T) {
	cases := []struct {
		name string
		in   [16]byte
		want string
	}{
		{"全零", [16]byte{}, strings.Repeat("0", 26)},
		// 首字符只承载 3 位（最高 2 位是固定 0 的填充），0xFF 的首字符是 7。
		{"全一", [16]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, "7" + strings.Repeat("Z", 25)},
		{"首字节为1", [16]byte{1}, "01" + strings.Repeat("0", 24)},
		{"首字节为0x1F", [16]byte{0x1F}, "0Z" + strings.Repeat("0", 24)},
	}
	for _, c := range cases {
		if got := encodeULID(c.in); got != c.want {
			t.Errorf("%s: encodeULID = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTraceIDFormatAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := NewTraceID()
		if len(id) != 26 {
			t.Fatalf("trace_id 长度应为 26: %q", id)
		}
		if !strings.HasPrefix(id, "01") {
			t.Fatalf("当前毫秒时间戳下 trace_id 应以 01 开头: %q", id)
		}
		for _, ch := range id {
			if !strings.ContainsRune(ulidEncoding, ch) {
				t.Fatalf("trace_id 含非法字符 %q: %q", ch, id)
			}
		}
		if seen[id] {
			t.Fatalf("trace_id 重复: %q", id)
		}
		seen[id] = true
	}
}

func TestOKEnvelope(t *testing.T) {
	lg, _ := newTestLogger(t)
	r := newTestEngine(lg, "release")
	r.GET("/api/x", func(c *gin.Context) { OK(c, gin.H{"k": "v"}) })

	w := do(r, http.MethodGet, "/api/x")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	resp := decodeBody(t, w)
	if resp.Code != 0 || resp.Msg != "" || resp.Data == nil {
		t.Fatalf("封装字段不符: %+v", resp)
	}
	if len(resp.TraceID) != 26 {
		t.Fatalf("trace_id 长度不符: %q", resp.TraceID)
	}
}

func TestFailStatusMapping(t *testing.T) {
	lg, _ := newTestLogger(t)
	r := newTestEngine(lg, "release")
	r.GET("/api/fail/:code", func(c *gin.Context) {
		var code int
		switch c.Param("code") {
		case "timeout":
			code = CodeUpstreamTimeout
		case "badstatus":
			code = CodeUpstreamBadStatus
		case "biz":
			code = CodeUpstreamBizError
		case "badrequest":
			code = CodeBadRequest
		case "notfound":
			code = CodeNotFound
		default:
			code = CodeInternal
		}
		Fail(c, code, "some msg")
	})

	cases := []struct {
		param  string
		code   int
		status int
	}{
		{"timeout", CodeUpstreamTimeout, http.StatusBadGateway},
		{"badstatus", CodeUpstreamBadStatus, http.StatusBadGateway},
		{"biz", CodeUpstreamBizError, http.StatusBadGateway},
		{"badrequest", CodeBadRequest, http.StatusBadRequest},
		{"notfound", CodeNotFound, http.StatusNotFound},
		{"other", CodeInternal, http.StatusInternalServerError},
	}
	for _, c := range cases {
		w := do(r, http.MethodGet, "/api/fail/"+c.param)
		if w.Code != c.status {
			t.Errorf("code %d: HTTP status = %d, want %d", c.code, w.Code, c.status)
		}
		resp := decodeBody(t, w)
		if resp.Code != c.code || resp.Msg != "some msg" || resp.Data != nil {
			t.Errorf("code %d: 封装不符: %+v", c.code, resp)
		}
	}
}

func TestNoRoute(t *testing.T) {
	lg, _ := newTestLogger(t)
	r := newTestEngine(lg, "release")

	for _, target := range []string{"/api/nope", "/"} {
		w := do(r, http.MethodGet, target)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d", target, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("%s: Content-Type = %q, 应为 JSON", target, ct)
		}
		resp := decodeBody(t, w)
		if resp.Code != CodeNotFound || resp.Data != nil {
			t.Fatalf("%s: 封装不符: %+v", target, resp)
		}
		if len(resp.TraceID) != 26 {
			t.Fatalf("%s: trace_id 缺失: %q", target, resp.TraceID)
		}
	}
}

func TestRequestLogLines(t *testing.T) {
	lg, buf := newTestLogger(t)
	r := newTestEngine(lg, "release")

	do(r, http.MethodGet, "/api/nope?a=1&b=2")

	lines := logLines(t, buf)
	var start, end map[string]any
	for _, m := range lines {
		switch m["msg"] {
		case "request.start":
			start = m
		case "response.end":
			end = m
		}
	}
	if start == nil || end == nil {
		t.Fatalf("缺少 request.start 或 response.end: %v", lines)
	}
	if start["level"] != "DEBUG" || start["method"] != "GET" ||
		start["path"] != "/api/nope" || start["query"] != "a=1&b=2" {
		t.Fatalf("request.start 字段不符: %v", start)
	}
	if start["trace_id"] != end["trace_id"] {
		t.Fatalf("trace_id 不一致: %v vs %v", start["trace_id"], end["trace_id"])
	}
	if end["level"] != "INFO" || end["status"] != float64(404) ||
		end["code"] != float64(CodeNotFound) {
		t.Fatalf("response.end 字段不符: %v", end)
	}
	if _, ok := end["duration_ms"]; !ok {
		t.Fatal("response.end 缺 duration_ms")
	}
}

func TestInfoLevelOnlyOneLinePerRequest(t *testing.T) {
	// CONTRACTS 第 0 节：level=info 时每请求只有 response.end 一行。
	buf := &bytes.Buffer{}
	lg := slog.New(slog.NewJSONHandler(buf, nil))
	r := newTestEngine(lg, "release")

	do(r, http.MethodGet, "/api/nope")

	lines := logLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("info 级别每请求应只有 1 行, got %d: %v", len(lines), lines)
	}
	if lines[0]["msg"] != "response.end" {
		t.Fatalf("应为 response.end: %v", lines[0])
	}
}

func TestRecovery(t *testing.T) {
	// 两种模式下：HTTP 500、除 trace_id 外响应体一致、响应体不带堆栈。
	var bodies [2]Response
	var stackFound [2]bool

	for i, mode := range []string{"release", "debug"} {
		lg, buf := newTestLogger(t)
		r := newTestEngine(lg, mode)
		r.GET("/api/boom", func(c *gin.Context) { panic("boom") })

		w := do(r, http.MethodGet, "/api/boom")
		bodies[i] = decodeBody(t, w)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("panic 后 status = %d", w.Code)
		}
		if bodies[i].Code != CodeInternal || bodies[i].Data != nil || strings.Contains(w.Body.String(), "boom") {
			t.Fatalf("panic 响应体不符: %s", w.Body.String())
		}

		var errLine map[string]any
		for _, m := range logLines(t, buf) {
			if m["msg"] == "error" {
				errLine = m
			}
		}
		if errLine == nil {
			t.Fatalf("mode=%s: 缺少 error 日志", mode)
		}
		if errLine["err"] != "boom" {
			t.Fatalf("mode=%s: err 字段不符: %v", mode, errLine["err"])
		}
		_, stackFound[i] = errLine["stack"]
	}
	if bodies[0].Code != bodies[1].Code || bodies[0].Msg != bodies[1].Msg {
		t.Fatalf("两种模式响应体应一致: release=%+v debug=%+v", bodies[0], bodies[1])
	}
	if stackFound[0] {
		t.Fatal("release 模式 error 日志不应带 stack")
	}
	if !stackFound[1] {
		t.Fatal("debug 模式 error 日志应带 stack")
	}
}

func TestPanicTraceIDConsistent(t *testing.T) {
	// panic 路径下 error 日志与响应体使用同一 trace_id。
	lg, buf := newTestLogger(t)
	r := newTestEngine(lg, "debug")
	r.GET("/api/boom", func(c *gin.Context) { panic("boom") })

	w := do(r, http.MethodGet, "/api/boom")
	resp := decodeBody(t, w)

	var errLine map[string]any
	for _, m := range logLines(t, buf) {
		if m["msg"] == "error" {
			errLine = m
		}
	}
	if errLine["trace_id"] != resp.TraceID {
		t.Fatalf("trace_id 不一致: 日志 %v vs 响应 %q", errLine["trace_id"], resp.TraceID)
	}
}

func TestSplitNoRoute(t *testing.T) {
	// /api/* 走 JSON 封装；其余交给静态 handler。
	// 静态未命中是空 body 404，gin 不允许在 NoRoute 链后补默认文本。
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.NoRoute(SplitNoRoute(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})))

	w := do(r, http.MethodGet, "/api/nope")
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"code":40400`) {
		t.Fatalf("/api/nope = %d %s", w.Code, w.Body.String())
	}

	w = do(r, http.MethodGet, "/some/page")
	if w.Code != http.StatusNotFound {
		t.Fatalf("static path = %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("静态未命中应空 body, got %q", w.Body.String())
	}
}
