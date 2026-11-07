package handler

import (
	"time"

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

		lg := api.Logger(c)
		fetchStart := time.Now()
		raw, err := upstream.FetchDetail(c.Request.Context(), client, lg, bookID)
		api.SetUpstreamMS(c, time.Since(fetchStart).Milliseconds())
		if err != nil {
			failUpstream(c, err)
			return
		}
		result, err := upstream.ParseDetail(raw)
		if err != nil {
			lg.Debug("parse.result", "ok", false, "items_count", 0)
			failUpstreamBody(c, err)
			return
		}
		lg.Debug("parse.result", "ok", true, "items_count", 1)
		api.OK(c, result)
	}
}
