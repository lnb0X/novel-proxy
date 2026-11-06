// Package api 提供统一响应封装、请求日志与 recovery 中间件。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 业务错误码与 HTTP 状态码的对应关系见 CONTRACTS.md 第 0 节。
const (
	CodeOK                = 0
	CodeUpstreamTimeout   = 50201
	CodeUpstreamBadStatus = 50202
	CodeUpstreamBizError  = 50203
	CodeBadRequest        = 40001
	CodeNotFound          = 40400
	CodeInternal          = 50001
)

// Response 统一响应封装，所有 /api/* 与未匹配路由共用。
type Response struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	TraceID string `json:"trace_id"`
	Data    any    `json:"data"`
}

// OK 返回 200 成功响应。
func OK(c *gin.Context, data any) {
	write(c, CodeOK, "", data)
}

// Fail 返回业务错误响应，data 恒为 null，HTTP 状态码随 code 映射。
func Fail(c *gin.Context, code int, msg string) {
	write(c, code, msg, nil)
}

func write(c *gin.Context, code int, msg string, data any) {
	c.Set(keyCode, code)
	c.JSON(httpStatus(code), Response{
		Code:    code,
		Msg:     msg,
		TraceID: c.GetString(keyTraceID),
		Data:    data,
	})
}

func httpStatus(code int) int {
	switch code {
	case CodeOK:
		return http.StatusOK
	case CodeUpstreamTimeout, CodeUpstreamBadStatus, CodeUpstreamBizError:
		return http.StatusBadGateway
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeNotFound:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
