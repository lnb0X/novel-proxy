package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
	"novel-proxy/internal/upstream"
)

// Catalog GET /api/catalog?book_id=
func Catalog(client *resty.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookID, err := parseID("book_id", c.Query("book_id"))
		if err != nil {
			api.Fail(c, api.CodeBadRequest, err.Error())
			return
		}

		lg := api.Logger(c)
		fetchStart := time.Now()
		raw, err := upstream.FetchCatalog(c.Request.Context(), client, lg, bookID)
		api.SetUpstreamMS(c, time.Since(fetchStart).Milliseconds())
		if err != nil {
			failUpstream(c, err)
			return
		}
		result, err := upstream.ParseCatalog(raw)
		if err != nil {
			lg.Debug("parse.result", "ok", false, "items_count", 0)
			failUpstreamBody(c, err)
			return
		}
		lg.Debug("parse.result", "ok", true, "items_count", len(result.Chapters))
		result.BookID = bookID
		for _, id := range result.Missing {
			api.Logger(c).Debug("catalog 章节缺失", "item_id", id)
		}
		api.OK(c, result)
	}
}
