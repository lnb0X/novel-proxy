package stats

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Middleware 请求日志钩子。位置紧随 trace_id 之后（包裹全部后续
// 中间件，panic 也记录），读取 api 包写入的 code 与 upstream_ms。
// nil 接收者直接放行（storage 未启用）。
func (s *Stats) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		target := c.Request.URL.Path
		if c.Request.URL.RawQuery != "" {
			target += "?" + c.Request.URL.RawQuery
		}
		s.Record(RequestRow{
			Time:       start,
			Path:       target,
			Status:     c.Writer.Status(),
			Code:       c.GetInt("code"),
			DurationMS: time.Since(start).Milliseconds(),
			UpstreamMS: c.GetInt64("upstream_ms"),
			Bytes:      int64(c.Writer.Size()),
			TraceID:    c.GetString("trace_id"),
			Query:      c.GetString("stats_query"),
		})
	}
}

// ParseTime 解析 from/to 参数：RFC3339 或 Unix 秒。
func ParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(n, 0), nil
	}
	return time.Parse(time.RFC3339, s)
}
