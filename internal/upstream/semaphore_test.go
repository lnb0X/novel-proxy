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
