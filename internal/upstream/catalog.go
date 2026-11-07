package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

const catalogURL = "https://fanqienovel.com/api/reader/directory/detail"

// FetchCatalog 请求上游目录接口，GET，参数驼峰 bookId。
func FetchCatalog(ctx context.Context, client *resty.Client, lg *slog.Logger, timeoutSeconds int, bookID string) ([]byte, error) {
	start := time.Now()
	release, aerr := acquireFetch(ctx, fetchTimeout(timeoutSeconds))
	if aerr != nil {
		return nil, aerr
	}
	defer release()
	resp, err := client.R().
		SetContext(ctx).
		SetQueryParam("bookId", bookID).
		Get(catalogURL)
	logResponse(lg, resp, start)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("上游非 2xx: %d", resp.StatusCode())
	}
	return resp.Body(), nil
}

// CatalogChapter 对外输出的目录单章。
type CatalogChapter struct {
	ChapterID string `json:"chapter_id"`
	Title     string `json:"title"`
	Volume    string `json:"volume"`
	IsVip     bool   `json:"is_vip"`
	IsPay     bool   `json:"is_pay"`
	UpdatedAt int64  `json:"updated_at"`
}

// Catalog GET /api/catalog 的 data。
type Catalog struct {
	BookID   string           `json:"book_id"`
	Total    int              `json:"total"`
	Chapters []CatalogChapter `json:"chapters"`

	// Missing 是 allItemIds 里在 chapterListWithVolume 中找不到的章节，
	// 由 handler 记 debug 日志用，不下发。
	Missing []string `json:"-"`
}

type catalogChapterRaw struct {
	ItemID            string `json:"itemId"`
	Title             string `json:"title"`
	NeedPay           int64  `json:"needPay"`
	IsChapterLock     bool   `json:"isChapterLock"`
	IsPaidPublication bool   `json:"isPaidPublication"`
	IsPaidStory       bool   `json:"isPaidStory"`
	FirstPassTime     string `json:"firstPassTime"`
}

type catalogEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    *struct {
		AllItemIds            []string              `json:"allItemIds"`
		VolumeNameList        []string              `json:"volumeNameList"`
		ChapterListWithVolume [][]catalogChapterRaw `json:"chapterListWithVolume"`
	} `json:"data"`
}

// ParseCatalog 解析上游目录响应，纯函数。
// 先把 chapterListWithVolume 展平成 map[itemId]（同时记下卷名），
// 再按 allItemIds 顺序取值输出，两个数组的顺序互不依赖。
func ParseCatalog(raw []byte) (Catalog, error) {
	var env catalogEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Catalog{}, fmt.Errorf("解析失败: %w", err)
	}
	if env.Code != 0 {
		return Catalog{}, &BizError{Code: env.Code, Message: env.Message}
	}
	if env.Data == nil {
		return Catalog{}, fmt.Errorf("上游 data 为 null")
	}

	type flatChapter struct {
		raw    catalogChapterRaw
		volume string
	}
	byID := make(map[string]flatChapter, len(env.Data.AllItemIds))
	for i, vol := range env.Data.ChapterListWithVolume {
		volumeName := ""
		if i < len(env.Data.VolumeNameList) {
			volumeName = env.Data.VolumeNameList[i]
		}
		for _, ch := range vol {
			byID[ch.ItemID] = flatChapter{raw: ch, volume: volumeName}
		}
	}

	chapters := make([]CatalogChapter, 0, len(env.Data.AllItemIds))
	var missing []string
	for _, id := range env.Data.AllItemIds {
		f, ok := byID[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		updatedAt, _ := strconv.ParseInt(f.raw.FirstPassTime, 10, 64)
		chapters = append(chapters, CatalogChapter{
			ChapterID: f.raw.ItemID,
			Title:     f.raw.Title,
			Volume:    f.volume,
			IsVip:     f.raw.IsChapterLock || f.raw.IsPaidPublication || f.raw.IsPaidStory,
			IsPay:     f.raw.NeedPay != 0,
			UpdatedAt: updatedAt,
		})
	}
	return Catalog{
		Total:    len(chapters),
		Chapters: chapters,
		Missing:  missing,
	}, nil
}
