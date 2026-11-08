package api

import (
	"net/http"
	"sync/atomic"
)

// Swappable 顶层可换容器：http.Server.Handler 指向它，init confirm
// 之后整体替换内部 engine，不重启进程。关键约束：gin 的路由树不支持
// 并发写，热挂载绝不可以在运行中的 engine 上 AddRoute，必须换掉整个
// handler。与 frontend.SwapHandler 同思路，但不带 404 兜底——启动即
// Store，不存在空窗口。
type Swappable struct {
	h atomic.Pointer[http.Handler]
}

// Store 替换当前 handler。旧 handler 上的在途请求继续跑完，
// 新请求立刻走新 handler。
func (s *Swappable) Store(h http.Handler) {
	s.h.Store(&h)
}

func (s *Swappable) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if hp := s.h.Load(); hp != nil {
		(*hp).ServeHTTP(w, r)
		return
	}
	// 不可达：启动流程先 Store 再 ListenAndServe。
	w.WriteHeader(http.StatusServiceUnavailable)
}
