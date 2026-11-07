package handler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
	"novel-proxy/internal/store"
)

// countingRT 内存假上游：返回一页带图正文，并记录被请求次数。
type countingRT struct {
	hits int
}

func (f *countingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	f.hits++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body: io.NopCloser(strings.NewReader(
			`<html><head><title>第1章 - 今日头条</title></head><body>` +
				`<article class="syl-article-base"><p>第一段。</p>` +
				`<img src="data:image/gif;base64,AA" data-src="https://p3-sign.toutiaoimg.com/a.jpeg">` +
				`<p>第二段。</p></article></body></html>`)),
		Request: req,
	}, nil
}

// 缓存全链路：第一次抓上游并入库，第二次命中缓存不打上游且返回纯文字。
func TestContentCacheRoundTrip(t *testing.T) {
	st, err := store.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	rt := &countingRT{}
	client := resty.New().SetTransport(rt)
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/content", Content(client, st)) })
	target := "/api/content?book_id=7526452300462705689&chapter_id=7526766660229808665"

	code, e := doJSON(t, r, target)
	if code != http.StatusOK || e.Code != api.CodeOK {
		t.Fatalf("首次请求: HTTP %d code %d msg %s", code, e.Code, e.Msg)
	}
	if rt.hits != 1 {
		t.Fatalf("首次请求应打一次上游, got %d", rt.hits)
	}
	var first struct {
		Content string `json:"content"`
	}
	if err := decodeData(e.Data, &first); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.Content, "<img") {
		t.Fatalf("未命中响应应带图原文: %q", first.Content)
	}

	code, e = doJSON(t, r, target)
	if code != http.StatusOK || e.Code != api.CodeOK {
		t.Fatalf("二次请求: HTTP %d code %d msg %s", code, e.Code, e.Msg)
	}
	if rt.hits != 1 {
		t.Fatalf("命中缓存不应再打上游, 上游被请求 %d 次", rt.hits)
	}
	var second struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := decodeData(e.Data, &second); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(second.Content, "<img") {
		t.Fatalf("缓存命中应返回纯文字: %q", second.Content)
	}
	if !strings.Contains(second.Content, "<p>第一段。</p>") {
		t.Fatalf("缓存正文缺失: %q", second.Content)
	}
	if second.Title != "第1章" {
		t.Fatalf("缓存标题不符: %q", second.Title)
	}
}

// 库里存的内容必须无 img：直接查库断言。
func TestStoredContentHasNoImg(t *testing.T) {
	st, err := store.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	rt := &countingRT{}
	client := resty.New().SetTransport(rt)
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/content", Content(client, st)) })

	if _, e := doJSON(t, r, "/api/content?book_id=1&chapter_id=7526766660229808665"); e.Code != api.CodeOK {
		t.Fatalf("请求失败: %s", e.Msg)
	}
	art, err := st.Get(context.Background(), "1", "7526766660229808665")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(art.Content, "<img") || strings.Contains(art.Content, "data:") {
		t.Fatalf("入库内容含 img: %q", art.Content)
	}
	if !strings.Contains(art.Content, "<p>") {
		t.Fatalf("入库内容缺正文: %q", art.Content)
	}
}
