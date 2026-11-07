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
		bookID, err := parseID("book_id", c.Query("book_id"))
		if err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}

		raw, err := upstream.FetchDetail(c.Request.Context(), client, bookID)
		if err != nil {
			failUpstream(c, err)
			return
		}
		result, err := upstream.ParseDetail(raw)
		if err != nil {
			failUpstreamBody(c, err)
			return
		}
		api.OK(c, result)
	}
}
