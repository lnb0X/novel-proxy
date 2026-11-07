package upstream

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	htmlparser "golang.org/x/net/html"
)

const contentURLPrefix = "https://www.toutiao.com/article/"

// chromeUA 契约要求的桌面 Chrome UA。
const chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// ErrNoText 上游 404，该章节无文本内容（有声书）。
var ErrNoText = errors.New("该章节无文本内容")

// acNonce 生成 __ac_nonce cookie：11 字节随机数的 hex 前 21 位。
func acNonce() string {
	var b [11]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand 不可用: %v", err))
	}
	return hex.EncodeToString(b[:])[:21]
}

// FetchContent 请求上游正文页，wid 用当前毫秒时间戳，带桌面 Chrome UA、
// Referer 和随机 __ac_nonce cookie。
func FetchContent(ctx context.Context, client *resty.Client, lg *slog.Logger, timeoutSeconds int, chapterID string) ([]byte, error) {
	start := time.Now()
	release, aerr := acquireFetch(ctx, fetchTimeout(timeoutSeconds))
	if aerr != nil {
		return nil, aerr
	}
	defer release()
	resp, err := client.R().
		SetContext(ctx).
		SetQueryParam("wid", strconv.FormatInt(time.Now().UnixMilli(), 10)).
		SetHeader("User-Agent", chromeUA).
		SetHeader("Referer", "https://www.toutiao.com/").
		SetHeader("Cookie", "__ac_nonce="+acNonce()).
		Get(contentURLPrefix + chapterID + "/")
	logResponse(lg, resp, start)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == 404 {
		return nil, ErrNoText
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("上游非 2xx: %d", resp.StatusCode())
	}
	return resp.Body(), nil
}

// Content GET /api/content 的 data，content 是过滤后的 HTML 字符串。
// BookID 不参与解析，由 handler 校验透传后回填。
type Content struct {
	BookID    string `json:"book_id"`
	ChapterID string `json:"chapter_id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
}

// ParseContent 用 Tokenizer 流式过滤正文，纯函数。
// 白名单只保留 p / img / br：img 取 data-src（没有才取 src），
// 以 data: 开头或非 http(s) 的丢弃，只输出 <img src="...">；
// 其他标签剥标签保留文字。找不到 article 或过滤后为空都报错。
func ParseContent(raw []byte) (Content, error) {
	var (
		out       strings.Builder
		title     strings.Builder
		inArticle bool
		inTitle   bool
	)

	z := htmlparser.NewTokenizer(strings.NewReader(string(raw)))
	for {
		tt := z.Next()
		if tt == htmlparser.ErrorToken {
			if z.Err() != io.EOF {
				return Content{}, fmt.Errorf("解析失败: %w", z.Err())
			}
			break
		}
		switch tt {
		case htmlparser.StartTagToken, htmlparser.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := strings.ToLower(string(name))
			if tag == "title" {
				// 页面里有多个 title（如 aria 用的 Loading 占位），
				// 只取文档中第一个有内容的。
				inTitle = title.Len() == 0
				continue
			}
			if tag == "article" {
				inArticle = hasArticleClass(z, hasAttr)
				continue
			}
			if !inArticle {
				continue
			}
			switch tag {
			case "p":
				out.WriteString("<p>")
			case "br":
				out.WriteString("<br>")
			case "img":
				if src, ok := imgSrc(z, hasAttr); ok {
					out.WriteString(`<img src="`)
					out.WriteString(html.EscapeString(src))
					out.WriteString(`">`)
				}
			}
		case htmlparser.EndTagToken:
			name, _ := z.TagName()
			tag := strings.ToLower(string(name))
			switch tag {
			case "title":
				inTitle = false
			case "article":
				inArticle = false
			case "p":
				if inArticle {
					out.WriteString("</p>")
				}
			}
		case htmlparser.TextToken:
			if inTitle {
				title.Write(z.Text())
				continue
			}
			if inArticle {
				// Tokenizer 已把实体解码成原始文字，输出前重新转义，
				// 保证 content 是合法 HTML。
				out.WriteString(html.EscapeString(string(z.Text())))
			}
		}
	}

	content := out.String()
	if content == "" {
		return Content{}, fmt.Errorf("页面里找不到 article 或正文为空")
	}
	cleanTitle := strings.TrimSpace(title.String())
	cleanTitle = strings.TrimSuffix(cleanTitle, " - 今日头条")
	return Content{Title: cleanTitle, Content: content}, nil
}

// hasArticleClass 判断当前 article 开始标签的 class 是否含 syl-article-base。
func hasArticleClass(z *htmlparser.Tokenizer, hasAttr bool) bool {
	for hasAttr {
		var key, val []byte
		key, val, hasAttr = z.TagAttr()
		if strings.ToLower(string(key)) != "class" {
			continue
		}
		for _, cls := range strings.Fields(string(val)) {
			if cls == "syl-article-base" {
				return true
			}
		}
		return false
	}
	return false
}

// imgSrc 从 img 标签属性取图片地址：先 data-src 后 src，
// data: 占位符与非 http(s) 地址丢弃。
func imgSrc(z *htmlparser.Tokenizer, hasAttr bool) (string, bool) {
	dataSrc := ""
	src := ""
	for hasAttr {
		var key, val []byte
		key, val, hasAttr = z.TagAttr()
		switch strings.ToLower(string(key)) {
		case "data-src":
			dataSrc = string(val)
		case "src":
			src = string(val)
		}
	}
	for _, v := range []string{dataSrc, src} {
		if v == "" || strings.HasPrefix(v, "data:") {
			continue
		}
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			return v, true
		}
	}
	return "", false
}

// StripImages 剥掉 HTML 里所有 img 标签，只留 p / br / 文字，入库前用。
// 与 ParseContent 相同的 Tokenizer 处理：文本实体先解码再转义，
// 对 ParseContent 的输出做一遍是恒等变换（除 img 外）。
func StripImages(raw string) string {
	var out strings.Builder
	z := htmlparser.NewTokenizer(strings.NewReader(raw))
	for {
		tt := z.Next()
		if tt == htmlparser.ErrorToken {
			break
		}
		switch tt {
		case htmlparser.StartTagToken, htmlparser.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := strings.ToLower(string(name))
			if tag == "img" {
				continue
			}
			switch tag {
			case "p":
				out.WriteString("<p>")
			case "br":
				out.WriteString("<br>")
			}
		case htmlparser.EndTagToken:
			name, _ := z.TagName()
			if strings.ToLower(string(name)) == "p" {
				out.WriteString("</p>")
			}
		case htmlparser.TextToken:
			out.WriteString(html.EscapeString(string(z.Text())))
		}
	}
	return out.String()
}
