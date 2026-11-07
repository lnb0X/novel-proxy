package handler

import (
	"context"
	"errors"
	"net"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
	"novel-proxy/internal/upstream"
)

// failUpstream 把 Fetch 阶段的错误映射为 50201（超时）或 50202（其余）。
// 对外统一文案，真实错误只进日志（CONTRACTS 第 0 节 / DECISIONS 安全基线）。
func failUpstream(c *gin.Context, err error) {
	api.Logger(c).Error("error", "err", err.Error())
	var netErr net.Error
	if (errors.As(err, &netErr) && netErr.Timeout()) || errors.Is(err, context.DeadlineExceeded) {
		api.Fail(c, api.CodeUpstreamTimeout, "上游超时")
		return
	}
	api.Fail(c, api.CodeUpstreamBadStatus, "上游服务异常")
}

// failUpstreamBody 把 Parse 阶段的错误映射为 502 + 50203。
// 上游业务错误按契约把上游 code 写进 msg；其余统一文案。
func failUpstreamBody(c *gin.Context, err error) {
	var biz *upstream.BizError
	if errors.As(err, &biz) {
		api.Fail(c, api.CodeUpstreamBizError, err.Error())
		return
	}
	api.Logger(c).Error("error", "err", err.Error())
	api.Fail(c, api.CodeUpstreamBizError, "上游服务异常")
}

// failInternal 50001 统一文案，真实错误只进日志。
func failInternal(c *gin.Context, err error) {
	api.Logger(c).Error("error", "err", err.Error())
	api.Fail(c, api.CodeInternal, "服务内部错误")
}
