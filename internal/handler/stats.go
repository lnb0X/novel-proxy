package handler

import (
	"runtime/debug"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/middleware"
	"novel-proxy/internal/stats"
	"novel-proxy/internal/store"
)

// requireStats 三组统计接口的前置：storage 未启用时 Stats 为 nil，
// 一律 503 + 50301 固定文案（Q3-1）。
// RequireStats 三组统计接口与缓存端点的前置：storage 未启用时 Stats 为 nil，
// 一律 503 + 50301 固定文案（Q3-1）。
func RequireStats(s *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			api.Fail(c, api.CodeDisabled, "统计与审计需要开启文章缓存")
			return
		}
		c.Next()
	}
}

// StatsOverview GET /<secret>/api/stats/overview
func StatsOverview(s *stats.Stats, counters *store.CountingStore, startedAt time.Time) gin.HandlerFunc {
	return func(c *gin.Context) {
		hits, misses := counters.Hits(), counters.Misses()
		rate := 0.0
		if hits+misses > 0 {
			rate = float64(hits) / float64(hits+misses)
		}
		ov, err := s.ComputeOverview(rate)
		if err != nil {
			failInternal(c, err)
			return
		}
		version := "dev"
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			version = info.Main.Version
		}
		ov.Version = version
		ov.UptimeSeconds = int64(time.Since(startedAt).Seconds())
		api.OK(c, ov)
	}
}

// StatsQuery GET /<secret>/api/stats/query?from=&to=&api=
func StatsQuery(s *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		from, err := stats.ParseTime(c.Query("from"))
		if err != nil {
			api.Fail(c, api.CodeBadRequest, "from 需为 RFC3339 或 Unix 秒")
			return
		}
		to, err := stats.ParseTime(c.Query("to"))
		if err != nil {
			api.Fail(c, api.CodeBadRequest, "to 需为 RFC3339 或 Unix 秒")
			return
		}
		result, err := s.ComputeQuery(from, to, c.Query("api"))
		if err != nil {
			failInternal(c, err)
			return
		}
		api.OK(c, result)
	}
}

// AuditList GET /<secret>/api/audit?event=&from=&to=&page=&size=
func AuditList(s *stats.Stats) gin.HandlerFunc {
	return func(c *gin.Context) {
		from, err := stats.ParseTime(c.Query("from"))
		if err != nil {
			api.Fail(c, api.CodeBadRequest, "from 需为 RFC3339 或 Unix 秒")
			return
		}
		to, err := stats.ParseTime(c.Query("to"))
		if err != nil {
			api.Fail(c, api.CodeBadRequest, "to 需为 RFC3339 或 Unix 秒")
			return
		}
		page := atoiDefault(c.Query("page"), 1)
		size := atoiDefault(c.Query("size"), 50)
		result, err := s.QueryAudit(c.Query("event"), from, to, page, size)
		if err != nil {
			failInternal(c, err)
			return
		}
		api.OK(c, result)
	}
}

// CacheList GET /<secret>/api/cache?book_id=&chapter_id=&page=&size=
func CacheList(s *stats.Stats, counters *store.CountingStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := atoiDefault(c.Query("page"), 1)
		size := atoiDefault(c.Query("size"), 50)
		out, err := s.ListArticles(page, size, c.Query("book_id"), c.Query("chapter_id"))
		if err != nil {
			failInternal(c, err)
			return
		}
		hits, misses := counters.Hits(), counters.Misses()
		if hits+misses > 0 {
			out.HitRate = float64(hits) / float64(hits+misses)
		}
		api.OK(c, out)
	}
}

// CacheDeleteItem DELETE /<secret>/api/cache/{item_id} → 204
func CacheDeleteItem(s *stats.Stats, st store.ArticleStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		itemID := c.Param("item_id")
		bookID, err := s.LookupArticleBookID(itemID)
		if err != nil {
			api.Fail(c, api.CodeNotFound, err.Error())
			return
		}
		if err := st.Delete(c.Request.Context(), bookID, itemID); err != nil {
			failInternal(c, err)
			return
		}
		s.Audit("cache_clear", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(),
			"item_id="+itemID, "ok")
		c.Status(204)
	}
}

// CacheDeleteByBook DELETE /<secret>/api/cache?book_id= → { deleted: n }
func CacheDeleteByBook(s *stats.Stats, st store.ArticleStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		bookID := c.Query("book_id")
		if bookID == "" {
			api.Fail(c, api.CodeBadRequest, "book_id 必填（全部删除用 /cache/all）")
			return
		}
		n, err := st.DeleteByBook(c.Request.Context(), bookID)
		if err != nil {
			failInternal(c, err)
			return
		}
		s.Audit("cache_clear", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(),
			"book_id="+bookID, "ok")
		api.OK(c, gin.H{"deleted": n})
	}
}

// CacheDeleteAll DELETE /<secret>/api/cache/all body { confirm: "DELETE" }
func CacheDeleteAll(s *stats.Stats, st store.ArticleStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Confirm string `json:"confirm"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Confirm != "DELETE" {
			api.Fail(c, api.CodeBadRequest, "需要 body {\"confirm\":\"DELETE\"}")
			return
		}
		n, err := st.DeleteAll(c.Request.Context())
		if err != nil {
			failInternal(c, err)
			return
		}
		s.Audit("cache_clear", actor(c), middleware.ClientIP(c.Request), c.Request.UserAgent(),
			"all", "ok")
		api.OK(c, gin.H{"deleted": n})
	}
}

func actor(c *gin.Context) string {
	if u := c.GetString("auth_user"); u != "" {
		return u
	}
	return "-"
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
