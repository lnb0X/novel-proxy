// Package api 提供统一响应封装、请求日志与 recovery 中间件。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 业务错误码与 HTTP 状态码的对应关系见 CONTRACTS.md 第 0 节。
const (
	CodeOK                = 0
	CodeUnauthorized      = 40101
	CodeIPRejected        = 40301
	CodeNotFound          = 40400
	CodeAlreadyInit       = 40901
	CodeTooManyRequests   = 42901
	CodeInternal          = 50001
	CodeUpstreamTimeout   = 50201
	CodeUpstreamBadStatus = 50202
	CodeUpstreamBizError  = 50203
	CodeDisabled          = 50301
	CodeNotInitialized    = 50302
	CodeBadRequest        = 40001
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

// OKMsg 返回带提示文案的成功响应（如改密码后要求重新登录）。
func OKMsg(c *gin.Context, msg string, data any) {
	write(c, CodeOK, msg, data)
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
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeIPRejected:
		return http.StatusForbidden
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeNotFound:
		return http.StatusNotFound
	case CodeAlreadyInit:
		return http.StatusConflict
	case CodeTooManyRequests:
		return http.StatusTooManyRequests
	case CodeUpstreamTimeout, CodeUpstreamBadStatus, CodeUpstreamBizError:
		return http.StatusBadGateway
	case CodeDisabled, CodeNotInitialized:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
