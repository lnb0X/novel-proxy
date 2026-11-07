package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const detailURL = "https://api5-sinfonlinec.novelfm.com/novelfm/bookapi/detail/v1/"

type detailRequest struct {
	BookID string `json:"book_id"`
}

// FetchDetail 请求上游详情接口，POST JSON，只发请求。
func FetchDetail(ctx context.Context, client *resty.Client, lg *slog.Logger, timeoutSeconds int, bookID string) ([]byte, error) {
	start := time.Now()
	release, aerr := acquireFetch(ctx, fetchTimeout(timeoutSeconds))
	if aerr != nil {
		return nil, aerr
	}
	defer release()
	resp, err := client.R().
		SetContext(ctx).
		SetQueryParam("aid", "3040").
		SetHeader("Content-Type", "application/json").
		SetBody(detailRequest{BookID: bookID}).
		Post(detailURL)
	logResponse(lg, resp, start)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("上游非 2xx: %d", resp.StatusCode())
	}
	return resp.Body(), nil
}

// Detail GET /api/detail 的 data。
type Detail struct {
	BookID      string `json:"book_id"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	WordNumber  int    `json:"word_number"`
	LastChapter string `json:"last_chapter"`
	Intro       string `json:"intro"`
	ThumbURL    string `json:"thumb_url"`
}

type detailEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    *struct {
		BookID            string `json:"book_id"`
		BookName          string `json:"book_name"`
		Anchor            string `json:"anchor"`
		WordNumber        string `json:"word_number"`
		LastChapterItemID string `json:"last_chapter_item_id"`
		Abstract          string `json:"abstract"`
		ThumbURL          string `json:"thumb_url"`
	} `json:"data"`
}

// ParseDetail 解析上游详情响应，纯函数。
// title 取 book_name，author 取 anchor，last_chapter 取 last_chapter_item_id，intro 取 abstract。
func ParseDetail(raw []byte) (Detail, error) {
	var env detailEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Detail{}, fmt.Errorf("解析失败: %w", err)
	}
	if env.Code != 0 {
		return Detail{}, &BizError{Code: env.Code, Message: env.Message}
	}
	if env.Data == nil {
		return Detail{}, fmt.Errorf("上游 data 为 null")
	}
	// word_number 上游是字符串，转 int，解析失败给 0。
	wordNumber, _ := strconv.Atoi(env.Data.WordNumber)
	return Detail{
		BookID:      env.Data.BookID,
		Title:       env.Data.BookName,
		Author:      env.Data.Anchor,
		WordNumber:  wordNumber,
		LastChapter: env.Data.LastChapterItemID,
		Intro:       strings.TrimSpace(env.Data.Abstract),
		ThumbURL:    env.Data.ThumbURL,
	}, nil
}
