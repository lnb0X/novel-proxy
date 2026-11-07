package handler

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
	"novel-proxy/internal/config"
	"novel-proxy/internal/rules"
	"novel-proxy/internal/store"
	"novel-proxy/internal/upstream"
)

// Content GET /api/content?book_id=&chapter_id=
// book_id 只做格式校验后透传回填，上游请求只用 chapter_id 构造。
// 执行流程（DECISIONS）：接管 → 缓存 → 上游 → 剥图入库 →
// replace → prepend/append → 返回；拼接只在响应时进行。
func Content(client *resty.Client, st store.ArticleStore, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookID, err := parseID("book_id", c.Query("book_id"))
		if err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}
		chapterID, err := parseID("chapter_id", c.Query("chapter_id"))
		if err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}

		// 规则解析：override 非空即接管（Q4），不抓上游、不查缓存。
		rs := rules.Resolve(cfg.ContentRules, bookID)
		bookCfg, hasBook := cfg.ContentRules.Books[bookID]
		bookOverridden := hasBook && bookCfg.Override != nil
		if source, content, ok := rules.TakeOver(rs, bookOverridden); ok {
			api.Logger(c).Info("content.overridden",
				"book_id", bookID, "chapter_id", chapterID, "rule_source", source)
			api.OK(c, upstream.Content{
				BookID:    bookID,
				ChapterID: chapterID,
				Content:   content,
			})
			return
		}

		if st != nil {
			art, err := st.Get(c.Request.Context(), bookID, chapterID)
			if err == nil {
				content, _ := rules.Assemble(art.Content, rs, "g")
				api.OK(c, upstream.Content{
					BookID:    bookID,
					ChapterID: chapterID,
					Title:     art.Title,
					Content:   content,
				})
				return
			}
			if !errors.Is(err, store.ErrNotFound) {
				failInternal(c, err)
				return
			}
		}

		lg := api.Logger(c)
		fetchStart := time.Now()
		raw, err := upstream.FetchContent(c.Request.Context(), client, lg, cfg.Upstream.TimeoutSeconds, chapterID)
		if err == nil {
			api.SetUpstreamMS(c, time.Since(fetchStart).Milliseconds())
		}
		if err != nil {
			if errors.Is(err, upstream.ErrNoText) {
				api.Fail(c, api.CodeNotFound, "该章节无文本内容")
				return
			}
			failUpstream(c, err)
			return
		}
		result, err := upstream.ParseContent(raw)
		if err != nil {
			failUpstreamBody(c, err)
			return
		}

		if st != nil {
			// 入库前剥掉所有 img，只留 p / br / 文字。
			// 写失败不拦请求：正文已经抓到，缓存缺失只记日志，
			// 下次请求重新走未命中路径。
			stripped := upstream.StripImages(result.Content)
			err := st.Put(c.Request.Context(), &store.Article{
				ItemID:  chapterID,
				BookID:  bookID,
				Title:   result.Title,
				Content: stripped,
			})
			if err != nil {
				api.Logger(c).Error("error", "err", err.Error())
			}
		}

		content, _ := rules.Assemble(result.Content, rs, "g")
		result.Content = content
		result.BookID = bookID
		result.ChapterID = chapterID
		api.OK(c, result)
	}
}
