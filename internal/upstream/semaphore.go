package upstream

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"
)

// fetchSem 包住四个 FetchXxx 的 HTTP 调用，上限来自
// upstream.max_concurrency。x/sync 当前可用版本没有 SetLimit
// （QUESTIONS Q7），运行期调整改为在互斥锁下重建 Weighted：
// 旧名额持有者继续用旧信号量释放，新请求立即按新上限排队。
type fetchSemaphore struct {
	mu    sync.Mutex
	sem   *semaphore.Weighted
	limit int64
}

func (f *fetchSemaphore) setLimit(n int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n == f.limit || n < 1 {
		return
	}
	f.sem = semaphore.NewWeighted(n)
	f.limit = n
}

func (f *fetchSemaphore) acquire(ctx context.Context, weight int64) error {
	f.mu.Lock()
	sem := f.sem
	f.mu.Unlock()
	return sem.Acquire(ctx, weight)
}

func (f *fetchSemaphore) release(weight int64) {
	f.mu.Lock()
	sem := f.sem
	f.mu.Unlock()
	sem.Release(weight)
}

var (
	fetchSema = &fetchSemaphore{sem: semaphore.NewWeighted(8), limit: 8}
	semLimit  atomic.Int64
)

// fetchTimeout 并发名额获取与上游请求共用的超时时长。
func fetchTimeout(timeoutSeconds int) time.Duration {
	if timeoutSeconds < 1 {
		timeoutSeconds = 10
	}
	return time.Duration(timeoutSeconds) * time.Second
}

// SetConcurrency 运行期调整并发上限。小于 1 的值按 1 处理。
func SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	fetchSema.setLimit(int64(n))
	semLimit.Store(int64(n))
}

// Concurrency 返回当前上限（测试与管理接口读取用）。
func Concurrency() int64 { return semLimit.Load() }

// acquireFetch 按请求上下文获取一个并发名额，获取本身的超时与
// upstream.timeout_seconds 一致。返回释放函数。
func acquireFetch(ctx context.Context, timeout time.Duration) (func(), error) {
	actx, cancel := context.WithTimeout(ctx, timeout)
	if err := fetchSema.acquire(actx, 1); err != nil {
		cancel()
		return nil, err
	}
	release := func() {
		fetchSema.release(1)
		cancel()
	}
	return release, nil
}
