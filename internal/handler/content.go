package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
	"novel-proxy/internal/upstream"
)

// Content GET /api/content?chapter_id=
func Content(client *resty.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		chapterID := c.Query("chapter_id")
		if chapterID == "" {
			api.Fail(c, api.CodeBadRequest, "参数 chapter_id 必填")
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
		result.ChapterID = chapterID
		api.OK(c, result)
	}
}
