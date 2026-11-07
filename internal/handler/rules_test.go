package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/rules"
)

func strPtr(s string) *string { return &s }

// htmlRT 返回固定 HTML 页面的假上游。
type htmlRT struct {
	body string
	hits int
}

func (f *htmlRT) RoundTrip(req *http.Request) (*http.Response, error) {
	f.hits++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Request:    req,
	}, nil
}

const previewHTML = `<html><head><title>t - 今日头条</title></head>` +
	`<body><article class="syl-article-base"><p>蛇群蠕动。</p></article></body></html>`

func newRulesEnv(t *testing.T) (*gin.Engine, *config.Config, *htmlRT, *strings.Builder) {
	t.Helper()
	cfg := &config.Config{}
	rt := &htmlRT{body: previewHTML}
	buf := &strings.Builder{}
	lg := slog.New(slog.NewJSONHandler(buf, nil))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(api.SlogMiddleware(lg, ""))
	client := resty.New().SetTransport(rt)
	r.GET("/api/content", Content(client, nil, cfg))
	r.POST("/api/content-rules/preview", ContentRulesPreview(cfg, client, nil))
	r.PUT("/api/content-rules", PutContentRules(cfg, t.TempDir()+"/instance.yaml", nil))
	r.GET("/api/content-rules", GetContentRules(cfg))
	return r, cfg, rt, buf
}

func contentContent(t *testing.T, e envelope) string {
	t.Helper()
	var data struct {
		Content string `json:"content"`
	}
	if err := decodeData(e.Data, &data); err != nil {
		t.Fatalf("data 解析失败: %v\n%s", err, e.Data)
	}
	return data.Content
}

func TestContentGlobalPrepend(t *testing.T) {
	r, cfg, _, _ := newRulesEnv(t)
	cfg.ContentRules.Default.Prepend = strPtr("<p>片头</p>")
	target := "/api/content?book_id=7526452300462705689&chapter_id=7526766660229808665"

	_, e := doJSON(t, r, target)
	if !strings.Contains(contentContent(t, e), "<p>片头</p>") {
		t.Fatalf("全局 prepend 未生效: %s", e.Data)
	}

	// 某书空字符串抑制全局。
	cfg.ContentRules.Books = map[string]config.ContentRuleSet{
		"7526452300462705689": {Prepend: strPtr("")},
	}
	_, e = doJSON(t, r, target)
	if strings.Contains(contentContent(t, e), "<p>片头</p>") {
		t.Fatalf("空字符串应抑制: %s", e.Data)
	}
	// 其他书仍生效。
	_, e = doJSON(t, r, "/api/content?book_id=1&chapter_id=7526766660229808665")
	if !strings.Contains(contentContent(t, e), "<p>片头</p>") {
		t.Fatalf("其他书应仍生效: %s", e.Data)
	}
}

func TestContentOverride(t *testing.T) {
	r, cfg, rt, logs := newRulesEnv(t)
	cfg.ContentRules.Default.Override = strPtr("<p>接管内容</p>")
	target := "/api/content?book_id=7526452300462705689&chapter_id=7526766660229808665"

	_, e := doJSON(t, r, target)
	if contentContent(t, e) != "<p>接管内容</p>" {
		t.Fatalf("override 未生效: %s", e.Data)
	}
	if rt.hits != 0 {
		t.Fatalf("接管时不应请求上游, hits=%d", rt.hits)
	}
	if !strings.Contains(logs.String(), "content.overridden") ||
		!strings.Contains(logs.String(), "global.override") {
		t.Fatalf("缺 content.overridden 日志: %s", logs.String())
	}

	// 书级优先。
	cfg.ContentRules.Books = map[string]config.ContentRuleSet{
		"7526452300462705689": {Override: strPtr("<p>本书接管</p>")},
	}
	_, e = doJSON(t, r, target)
	if contentContent(t, e) != "<p>本书接管</p>" {
		t.Fatalf("书级接管未生效: %s", e.Data)
	}
	if !strings.Contains(logs.String(), "book.override") {
		t.Fatal("rule_source 应为 book.override")
	}
}

func TestContentRulesEndpoints(t *testing.T) {
	r, cfg, _, _ := newRulesEnv(t)
	body := `{
		"default": {"prepend": "全局头", "replace": [{"pattern":"a","replacement":"b","regex":false}]},
		"books": {"100": {"override": "本书接管"}}
	}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/content-rules", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", w.Code, w.Body.String())
	}
	if cfg.ContentRules.Default.Prepend == nil || *cfg.ContentRules.Default.Prepend != "全局头" {
		t.Fatalf("内存配置未生效: %+v", cfg.ContentRules)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/content-rules", nil))
	if !strings.Contains(w.Body.String(), `"prepend":"全局头"`) {
		t.Fatalf("GET 回读不符: %s", w.Body.String())
	}
	// 非法正则。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/content-rules",
		strings.NewReader(`{"default":{"replace":[{"pattern":"([","replacement":"x","regex":true}]}}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法正则应 40001: %d", w.Code)
	}
	// 超过 100 本。
	var sb strings.Builder
	sb.WriteString(`{"default":{},"books":{`)
	for i := 0; i < 101; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"b` + itoaTest(i) + `":{}`)
	}
	sb.WriteString(`}}`)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/content-rules", strings.NewReader(sb.String())))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("超过 100 本应 40001: %d", w.Code)
	}
}

func TestPreviewApplied(t *testing.T) {
	r, cfg, _, _ := newRulesEnv(t)
	cfg.ContentRules.Default.Replace = []config.ReplaceRule{
		{Pattern: "蛇", Replacement: "龙"},
	}
	body := `{"book_id":"7526452300462705689","chapter_id":"7526766660229808665"}`

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/content-rules/preview", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Original string          `json:"original"`
			Result   string          `json:"result"`
			Applied  []rules.Applied `json:"applied"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Original != "<p>蛇群蠕动。</p>" || resp.Data.Result != "<p>龙群蠕动。</p>" {
		t.Fatalf("预览不符: %+v", resp.Data)
	}
	if len(resp.Data.Applied) != 1 || resp.Data.Applied[0].Hits != 1 {
		t.Fatalf("applied 应带命中次数: %+v", resp.Data.Applied)
	}

	// override 命中：original null，applied 记 book.override hits 1。
	cfg.ContentRules.Books = map[string]config.ContentRuleSet{
		"7526452300462705689": {Override: strPtr("接管")},
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/content-rules/preview", strings.NewReader(body)))
	var resp2 struct {
		Data struct {
			Original *string         `json:"original"`
			Result   string          `json:"result"`
			Applied  []rules.Applied `json:"applied"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp2); err != nil {
		t.Fatal(err)
	}
	if resp2.Data.Original != nil || resp2.Data.Result != "接管" {
		t.Fatalf("接管预览不符: %+v", resp2.Data)
	}
	if len(resp2.Data.Applied) != 1 || resp2.Data.Applied[0].Rule != "book.override" || resp2.Data.Applied[0].Hits != 1 {
		t.Fatalf("接管 applied 不符: %+v", resp2.Data.Applied)
	}
}

func itoaTest(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for ; i > 0; i /= 10 {
		s = string(rune('0'+i%10)) + s
	}
	return s
}
