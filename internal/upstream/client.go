// Package upstream 每个上游接口一个适配文件：
// FetchXxx 只发请求（统一 resty client），ParseXxx 是纯函数只做解析和字段映射。
package upstream

import (
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

// NewClient 构建全局复用的 resty client，超时与并发上限来自
// upstream.timeout_seconds / upstream.max_concurrency。
func NewClient(timeoutSeconds, maxConcurrency int) *resty.Client {
	clientTimeoutMs.Store(int64(timeoutSeconds) * 1000)
	SetConcurrency(maxConcurrency)
	return resty.New().SetTimeout(time.Duration(timeoutSeconds) * time.Second)
}

// BizError 上游 HTTP 200 但 body.code != 0 的业务错误。
// handler 映射为 502 + 50203，并把上游 code 写进响应 msg。
type BizError struct {
	Code    int
	Message string
}

func (e *BizError) Error() string {
	return fmt.Sprintf("上游业务错误 code=%d message=%s", e.Code, e.Message)
}
