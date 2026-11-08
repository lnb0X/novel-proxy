package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// 热挂载语义：Store 之后的请求走新 handler，之前的在途请求不受影响。
// 并发 Store 与 ServeHTTP 在 -race 下不得报竞争。
func TestSwappableSwitch(t *testing.T) {
	sw := &Swappable{}
	sw.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("old"))
	}))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sw.ServeHTTP(w, req)
	if w.Body.String() != "old" {
		t.Fatalf("初始 handler 应生效, got %q", w.Body.String())
	}

	sw.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("new"))
	}))
	w = httptest.NewRecorder()
	sw.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Body.String() != "new" {
		t.Fatalf("Store 后应走新 handler, got %q", w.Body.String())
	}
}

func TestSwappableConcurrent(t *testing.T) {
	sw := &Swappable{}
	sw.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				w := httptest.NewRecorder()
				sw.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
			}
		}(i)
	}
	for j := 0; j < 100; j++ {
		sw.Store(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	}
	wg.Wait()
}
