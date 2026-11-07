package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"

	"novel-proxy/internal/api"
	"novel-proxy/internal/upstream"
)

// Search GET /api/search?q=&page=
func Search(client *resty.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := strings.TrimSpace(c.Query("q"))
		if q == "" {
			api.Fail(c, api.CodeBadRequest, "参数 q 必填")
			return
		}

		page := 1
		if s := c.Query("page"); s != "" {
			p, err := strconv.Atoi(s)
			if err != nil {
				api.Fail(c, api.CodeBadRequest, "参数 page 必须是整数")
				return
			}
			if p < 1 {
				p = 1
			}
			page = p
		}

		lg := api.Logger(c)
		raw, err := upstream.FetchSearch(c.Request.Context(), client, lg, q, page)
		if err != nil {
			failUpstream(c, err)
			return
		}
		result, err := upstream.ParseSearch(raw)
		if err != nil {
			lg.Debug("parse.result", "ok", false, "items_count", 0)
			failUpstreamBody(c, err)
			return
		}
		lg.Debug("parse.result", "ok", true, "items_count", len(result.Items))
		result.Page = page
		api.OK(c, result)
	}
}
