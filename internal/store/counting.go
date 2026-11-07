package store

import (
	"context"
	"sync/atomic"
)

// CountingStore 包装任意 ArticleStore，维护缓存命中 / 未命中两个
// atomic 计数器（Q3-4：cache_hit_rate 的数据来源），接口保持不变。
type CountingStore struct {
	inner  ArticleStore
	hits   atomic.Int64
	misses atomic.Int64
}

// Count 包装 store 并开始计数。
func Count(inner ArticleStore) *CountingStore {
	return &CountingStore{inner: inner}
}

func (c *CountingStore) Hits() int64   { return c.hits.Load() }
func (c *CountingStore) Misses() int64 { return c.misses.Load() }

func (c *CountingStore) Get(ctx context.Context, bookID, itemID string) (*Article, error) {
	a, err := c.inner.Get(ctx, bookID, itemID)
	if err == nil {
		c.hits.Add(1)
		return a, nil
	}
	c.misses.Add(1)
	return a, err
}

func (c *CountingStore) Init(ctx context.Context) error { return c.inner.Init(ctx) }
func (c *CountingStore) Ping(ctx context.Context) error { return c.inner.Ping(ctx) }
func (c *CountingStore) Put(ctx context.Context, a *Article) error {
	return c.inner.Put(ctx, a)
}
func (c *CountingStore) Delete(ctx context.Context, bookID, itemID string) error {
	return c.inner.Delete(ctx, bookID, itemID)
}
func (c *CountingStore) DeleteByBook(ctx context.Context, bookID string) (int64, error) {
	return c.inner.DeleteByBook(ctx, bookID)
}
func (c *CountingStore) DeleteAll(ctx context.Context) (int64, error) {
	return c.inner.DeleteAll(ctx)
}
func (c *CountingStore) Count(ctx context.Context) (int64, error) {
	return c.inner.Count(ctx)
}
func (c *CountingStore) SumSize(ctx context.Context) (int64, error) {
	return c.inner.SumSize(ctx)
}
