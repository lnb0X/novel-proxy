package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
	"novel-proxy/internal/store"
	"novel-proxy/internal/upstream"
)

// Content GET /api/content?book_id=&chapter_id=
// book_id 只做格式校验后透传回填，上游请求只用 chapter_id 构造。
// store 非 nil 时走缓存：命中返回纯文字，未命中抓上游、剥图入库后返回带图原文。
func Content(client *resty.Client, st store.ArticleStore) gin.HandlerFunc {
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

		if st != nil {
			art, err := st.Get(c.Request.Context(), bookID, chapterID)
			if err == nil {
				api.OK(c, upstream.Content{
					BookID:    bookID,
					ChapterID: chapterID,
					Title:     art.Title,
					Content:   art.Content,
				})
				return
			}
			if !errors.Is(err, store.ErrNotFound) {
				failInternal(c, err)
				return
			}
		}

		raw, err := upstream.FetchContent(c.Request.Context(), client, chapterID)
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
			stripped := upstream.StripImages(result.Content)
			err := st.Put(c.Request.Context(), &store.Article{
				ItemID:  chapterID,
				BookID:  bookID,
				Title:   result.Title,
				Content: stripped,
			})
			if err != nil {
				api.Fail(c, api.CodeInternal, err.Error())
				return
			}
		}

		result.BookID = bookID
		result.ChapterID = chapterID
		api.OK(c, result)
	}
}
