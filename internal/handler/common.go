package handler

import (
	"context"
	"errors"
	"net"

	"github.com/gin-gonic/gin"

	"novel-proxy/internal/api"
)

// failUpstream 把 Fetch 阶段的错误映射为 50201（超时）或 50202（其余）。
func failUpstream(c *gin.Context, err error) {
	var netErr net.Error
	if (errors.As(err, &netErr) && netErr.Timeout()) || errors.Is(err, context.DeadlineExceeded) {
		api.Fail(c, api.CodeUpstreamTimeout, "上游超时")
		return
	}
	api.Fail(c, api.CodeUpstreamBadStatus, err.Error())
}
