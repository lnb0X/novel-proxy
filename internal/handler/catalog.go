package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
	"novel-proxy/internal/upstream"
)

// Catalog GET /api/catalog?book_id=
func Catalog(client *resty.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookID := c.Query("book_id")
		if bookID == "" {
			api.Fail(c, api.CodeBadRequest, "参数 book_id 必填")
			return
		}

		raw, err := upstream.FetchCatalog(c.Request.Context(), client, bookID)
		if err != nil {
			failUpstream(c, err)
			return
		}
		result, err := upstream.ParseCatalog(raw)
		if err != nil {
			api.Fail(c, api.CodeUpstreamBizError, err.Error())
			return
		}
		result.BookID = bookID
		for _, id := range result.Missing {
			api.Logger(c).Debug("catalog 章节缺失", "item_id", id)
		}
		api.OK(c, result)
	}
}
