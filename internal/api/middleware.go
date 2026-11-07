package api

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	keyTraceID = "trace_id"
	keyLogger  = "logger"
	keyCode    = "code"
)

const ulidEncoding = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewTraceID 生成 26 位 ULID：48 位毫秒时间戳 + 80 位 crypto/rand 随机数。
func NewTraceID() string {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	if _, err := rand.Read(b[6:]); err != nil {
		panic(fmt.Sprintf("crypto/rand 不可用: %v", err))
	}
	return encodeULID(b)
}

// encodeULID 按 ULID 规范编码：130 位布局，最高 2 位固定为 0，
// 即首字符取 3 位，其余 25 个字符各取 5 位，Crockford base32。
func encodeULID(b [16]byte) string {
	var out [26]byte
	bits, n, i := uint64(0), uint(2), 0
	for _, by := range b {
		bits = bits<<8 | uint64(by)
		n += 8
		for n >= 5 {
			n -= 5
			out[i] = ulidEncoding[(bits>>n)&0x1F]
			i++
		}
	}
	return string(out[:])
}

// TraceID 返回当前请求的 trace_id。
func TraceID(c *gin.Context) string {
	return c.GetString(keyTraceID)
}

// Logger 返回带 trace_id 的请求级 logger，由 SlogMiddleware 注入。
func Logger(c *gin.Context) *slog.Logger {
	return c.MustGet(keyLogger).(*slog.Logger)
}

// SlogMiddleware 注入 trace_id 与请求级 logger，
// 记 request.start（debug）与 response.end（info）。
func SlogMiddleware(lg *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		tid := NewTraceID()
		c.Set(keyTraceID, tid)
		rl := lg.With("trace_id", tid)
		c.Set(keyLogger, rl)

		rl.Debug("request.start",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"query", c.Request.URL.RawQuery,
		)

		c.Next()

		rl.Info("response.end",
			"status", c.Writer.Status(),
			"code", c.GetInt(keyCode),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}

// RecoveryMiddleware 兜住 panic：记 error 日志（app.mode=debug 时带 stack），
// 响应统一 500 + 50001，响应体永远不带堆栈。
func RecoveryMiddleware(lg *slog.Logger, mode string) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			attrs := []any{"err", fmt.Sprintf("%v", p)}
			if mode == "debug" {
				attrs = append(attrs, "stack", string(debug.Stack()))
			}
			// recovery 自己不能再 panic，这里用安全取值而非 MustGet。
			rl := lg
			if v, ok := c.Get(keyLogger); ok {
				if reqLg, ok := v.(*slog.Logger); ok {
					rl = reqLg
				}
			}
			rl.Error("error", attrs...)
			Fail(c, CodeInternal, "内部错误")
		}()
		c.Next()
	}
}

// NoRoute 统一 JSON 404，格式与 /api/* 一致。
func NoRoute(c *gin.Context) {
	Fail(c, CodeNotFound, "路由不存在")
}

// SplitNoRoute 未匹配路由分流：/api/* 返回 JSON 404，
// 其余路径交给静态 handler（未配置静态目录时是固定 HTML 404 页）。
func SplitNoRoute(static http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			NoRoute(c)
			return
		}
		static.ServeHTTP(c.Writer, c.Request)
	}
}
