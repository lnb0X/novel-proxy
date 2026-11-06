package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
	"novel-proxy/internal/upstream"
)

// Detail GET /api/detail?book_id=
func Detail(client *resty.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookID := c.Query("book_id")
		if bookID == "" {
			api.Fail(c, api.CodeBadRequest, "参数 book_id 必填")
			return
		}

		raw, err := upstream.FetchDetail(c.Request.Context(), client, bookID)
		if err != nil {
			failUpstream(c, err)
			return
		}
		result, err := upstream.ParseDetail(raw)
		if err != nil {
			api.Fail(c, api.CodeUpstreamBizError, err.Error())
			return
		}
		api.OK(c, result)
	}
}
