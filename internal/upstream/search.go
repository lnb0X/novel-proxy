package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-resty/resty/v2"
)

const searchURL = "https://novel.snssdk.com/api/novel/channel/homepage/search/search/v1/"

// FetchSearch 请求上游搜索接口，只发请求。
// 上游参数风格：q / offset，offset = (page-1)*10。
func FetchSearch(ctx context.Context, client *resty.Client, q string, page int) ([]byte, error) {
	resp, err := client.R().
		SetContext(ctx).
		SetQueryParam("device_platform", "android").
		SetQueryParam("aid", "1967").
		SetQueryParam("offset", strconv.Itoa((page-1)*10)).
		SetQueryParam("q", q).
		Get(searchURL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("上游非 2xx: %d", resp.StatusCode())
	}
	return resp.Body(), nil
}

// SearchItem 对外输出的搜索单条结果。
type SearchItem struct {
	BookID   string `json:"book_id"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	Category string `json:"category"`
	Abstract string `json:"abstract"`
	ThumbURL string `json:"thumb_url"`
}

// SearchResult GET /api/search 的 data。
type SearchResult struct {
	Items      []SearchItem `json:"items"`
	Page       int          `json:"page"`
	HasMore    bool         `json:"has_more"`
	NextOffset int          `json:"next_offset"`
}

type searchEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    *struct {
		HasMore bool `json:"has_more"`
		Offset  int  `json:"offset"`
		// ret_data 用指针接：nil 是上游异常，非 nil 空数组是真没搜到。
		RetData *[]struct {
			BookID   string `json:"book_id"`
			Title    string `json:"title"`
			Author   string `json:"author"`
			Category string `json:"category"`
			Abstract string `json:"abstract"`
			ThumbURL string `json:"thumb_url"`
		} `json:"ret_data"`
	} `json:"data"`
}

// ParseSearch 解析上游搜索响应，纯函数。
// 上游 search_id / log_id 不下发，直接丢弃。
func ParseSearch(raw []byte) (SearchResult, error) {
	var env searchEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return SearchResult{}, fmt.Errorf("解析失败: %w", err)
	}
	if env.Code != 0 {
		return SearchResult{}, &BizError{Code: env.Code, Message: env.Message}
	}
	if env.Data == nil {
		return SearchResult{}, fmt.Errorf("上游 data 为 null")
	}
	if env.Data.RetData == nil {
		return SearchResult{}, fmt.Errorf("上游 ret_data 为 null")
	}

	src := *env.Data.RetData
	items := make([]SearchItem, 0, len(src))
	for _, it := range src {
		items = append(items, SearchItem{
			BookID:   it.BookID,
			Title:    it.Title,
			Author:   it.Author,
			Category: it.Category,
			Abstract: strings.TrimSpace(it.Abstract),
			ThumbURL: it.ThumbURL,
		})
	}
	return SearchResult{
		Items:      items,
		HasMore:    env.Data.HasMore,
		NextOffset: env.Data.Offset,
	}, nil
}
