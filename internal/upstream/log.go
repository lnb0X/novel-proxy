package upstream

import (
	"sync/atomic"
	"time"

	"github.com/go-resty/resty/v2"
	"log/slog"
)

// 全局 client 的超时毫秒数，由 NewClient 登记，供 upstream.request 日志用。
var clientTimeoutMs atomic.Int64

// logResponse 按 CONTRACTS 第 0 节记 upstream.request 与 upstream.response
// 两条 debug 日志；lg 为 nil 时静默。请求失败时只由调用方的 error 事件兜底。
func logResponse(lg *slog.Logger, resp *resty.Response, start time.Time) {
	if lg == nil || resp == nil {
		return
	}
	lg.Debug("upstream.request",
		"url", resp.Request.URL,
		"timeout_ms", clientTimeoutMs.Load(),
	)
	lg.Debug("upstream.response",
		"status", resp.StatusCode(),
		"duration_ms", time.Since(start).Milliseconds(),
		"bytes", len(resp.Body()),
	)
}
