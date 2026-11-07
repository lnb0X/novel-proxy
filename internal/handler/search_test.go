package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
)

// TestSearchRejectsEmptyQ 校验分支传 nil client：
// 校验若放行就会调 FetchSearch 并在 nil client 上 panic，测试直接失败，
// 以此证明校验发生在发请求之前。
func TestSearchRejectsEmptyQ(t *testing.T) {
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/search", Search(nil, &config.Config{})) })

	for _, q := range []string{"", " ", "  \t "} {
		vals := url.Values{}
		if q != "" {
			vals.Set("q", q)
		}
		target := "/api/search"
		if len(vals) > 0 {
			target += "?" + vals.Encode()
		}
		code, e := doJSON(t, r, target)
		wantBadRequest(t, target, code, e)
	}
}

func TestSearchRejectsNonIntegerPage(t *testing.T) {
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/search", Search(nil, &config.Config{})) })
	target := "/api/search?q=x&page=abc"
	code, e := doJSON(t, r, target)
	wantBadRequest(t, target, code, e)
}

// roundTripFunc 内存假上游，不产生任何网络连接。
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSearchPageClampedToOne(t *testing.T) {
	// page 缺省、0、负数都按 1 处理：offset 必须是 0，返回的 page 必须是 1，不报 40001。
	var gotQuery url.Values
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotQuery = req.URL.Query()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"code":0,"data":{"has_more":false,"offset":10,"ret_data":[]}}`)),
			Request:    req,
		}, nil
	})
	client := resty.New().SetTransport(rt)
	r := newValidationEngine(func(e *gin.Engine) { e.GET("/api/search", Search(client, &config.Config{})) })

	for _, page := range []string{"", "0", "-3"} {
		vals := url.Values{"q": []string{"斗破苍穹"}}
		if page != "" {
			vals.Set("page", page)
		}
		target := "/api/search?" + vals.Encode()
		code, e := doJSON(t, r, target)
		if code != http.StatusOK {
			t.Fatalf("page=%q: HTTP status = %d, want 200\n%s", page, code, e.Msg)
		}
		if e.Code != api.CodeOK {
			t.Fatalf("page=%q: code = %d, want 0", page, e.Code)
		}
		if gotQuery.Get("offset") != "0" {
			t.Fatalf("page=%q: 上游 offset = %q, want 0", page, gotQuery.Get("offset"))
		}
		var data struct {
			Page  int   `json:"page"`
			Items []any `json:"items"`
		}
		if err := json.Unmarshal(e.Data, &data); err != nil {
			t.Fatalf("page=%q: data 不是 JSON: %v", page, err)
		}
		if data.Page != 1 {
			t.Fatalf("page=%q: 返回 page = %d, want 1", page, data.Page)
		}
		if data.Items == nil || len(data.Items) != 0 {
			t.Fatalf("page=%q: items 应为空数组", page)
		}
	}
}
