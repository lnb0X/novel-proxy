package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
)

func TestDetailRejectsEmptyBookID(t *testing.T) {
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/detail", Detail(nil)) })
	target := "/api/detail"
	code, e := doJSON(t, r, target)
	wantBadRequest(t, target, code, e)
}

func TestCatalogRejectsEmptyBookID(t *testing.T) {
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/catalog", Catalog(nil)) })
	target := "/api/catalog"
	code, e := doJSON(t, r, target)
	wantBadRequest(t, target, code, e)
}

func TestContentRejectsEmptyChapterID(t *testing.T) {
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/content", Content(nil, nil)) })
	target := "/api/content?book_id=1"
	code, e := doJSON(t, r, target)
	wantBadRequest(t, target, code, e)
}

func TestContentRequiresBookID(t *testing.T) {
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/content", Content(nil, nil)) })
	for _, target := range []string{
		"/api/content?chapter_id=1",
		"/api/content?book_id=abc&chapter_id=1",
		"/api/content?book_id=1&chapter_id=abc",
	} {
		code, e := doJSON(t, r, target)
		wantBadRequest(t, target, code, e)
	}
}

func TestContentEchoesBookID(t *testing.T) {
	// book_id 合法时打到内存假上游（HTML），200 且 data 原样带回 book_id。
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body: io.NopCloser(strings.NewReader(
				`<html><head><title>第1章 - 今日头条</title></head>` +
					`<body><article class="syl-article-base"><p>x</p></article></body></html>`)),
			Request: req,
		}, nil
	})
	client := resty.New().SetTransport(rt)
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/content", Content(client, nil)) })

	target := "/api/content?book_id=7526452300462705689&chapter_id=7526766660229808665"
	code, e := doJSON(t, r, target)
	if code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200\n%s", code, e.Msg)
	}
	if e.Code != api.CodeOK {
		t.Fatalf("code = %d, want 0", e.Code)
	}
	var data struct {
		BookID    string `json:"book_id"`
		ChapterID string `json:"chapter_id"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(e.Data, &data); err != nil {
		t.Fatalf("data 不是 JSON: %v", err)
	}
	if data.BookID != "7526452300462705689" {
		t.Fatalf("data.book_id = %q", data.BookID)
	}
	if data.ChapterID != "7526766660229808665" {
		t.Fatalf("data.chapter_id = %q", data.ChapterID)
	}
	if !strings.Contains(data.Content, "<p>") {
		t.Fatalf("content 缺正文: %q", data.Content)
	}
}

func TestDetailRejectsBadBookID(t *testing.T) {
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/detail", Detail(nil)) })
	target := "/api/detail?book_id=abc"
	code, e := doJSON(t, r, target)
	wantBadRequest(t, target, code, e)
}

func TestCatalogRejectsBadBookID(t *testing.T) {
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/catalog", Catalog(nil)) })
	target := "/api/catalog?book_id=abc"
	code, e := doJSON(t, r, target)
	wantBadRequest(t, target, code, e)
}
