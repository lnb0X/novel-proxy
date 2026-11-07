package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
	"novel-proxy/internal/upstream"
)

// Content GET /api/content?book_id=&chapter_id=
// book_id 只做格式校验后透传回填，上游请求只用 chapter_id 构造。
func Content(client *resty.Client) gin.HandlerFunc {
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
			api.Fail(c, api.CodeUpstreamBizError, err.Error())
			return
		}
		result.BookID = bookID
		result.ChapterID = chapterID
		api.OK(c, result)
	}
}
