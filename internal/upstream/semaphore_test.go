package upstream

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSemaphoreHotLimit 并发名额随 SetConcurrency 热调：
// 限 1 时最多 1 个持有者，调大后并发数随之上升。
func TestSemaphoreHotLimit(t *testing.T) {
	SetConcurrency(1)
	if Concurrency() != 1 {
		t.Fatalf("limit = %d", Concurrency())
	}
	var cur, max atomic.Int64
	var wg sync.WaitGroup
	hold := func() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, err := acquireFetch(t.Context(), time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			n := cur.Add(1)
			for {
				old := max.Load()
				if n <= old || max.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			cur.Add(-1)
			rel()
		}()
	}
	hold()
	hold()
	hold()
	wg.Wait()
	if max.Load() != 1 {
		t.Fatalf("限 1 时最大并发 = %d", max.Load())
	}

	SetConcurrency(3)
	hold()
	hold()
	hold()
	wg.Wait()
	if max.Load() < 2 {
		t.Fatalf("调大到 3 后并发应上升, got %d", max.Load())
	}
}

// TestSemaphoreResizeDuringTraffic 20 个 goroutine 循环 acquire/release，
// 循环中间把上限 8 → 3 → 8：释放必须打到各自获取时的信号量上，
// 不触发 released-more-than-held，结束后可用名额回到上限。
func TestSemaphoreResizeDuringTraffic(t *testing.T) {
	SetConcurrency(8)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				rel, err := acquireFetch(t.Context(), time.Second)
				if err != nil {
					t.Error(err)
					return
				}
				rel()
			}
		}()
	}
	time.Sleep(30 * time.Millisecond)
	SetConcurrency(3)
	time.Sleep(30 * time.Millisecond)
	SetConcurrency(8)
	close(stop)
	wg.Wait()
	// 名额已全部归还：再次获取不该超时。
	rel, err := acquireFetch(t.Context(), time.Second)
	if err != nil {
		t.Fatalf("结束后名额未回到上限: %v", err)
	}
	rel()
}
