// Package store 提供文章缓存的 ArticleStore 接口与各驱动实现。
// 数据库只在 storage.enabled 时连接，未启用时 handler 拿到 nil store 直接旁路。
package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound 记录不存在（Get 未命中、Delete 零行）。
var ErrNotFound = errors.New("记录不存在")

// Article articles 表的一行。Put 时 SizeBytes / CreatedAt / UpdatedAt
// 由存储层负责填写，调用方只提供业务字段。
type Article struct {
	ItemID    string
	BookID    string
	Title     string
	Content   string
	SizeBytes int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ArticleStore 文章缓存接口，全部方法幂等或可重入。
type ArticleStore interface {
	Init(ctx context.Context) error
	Ping(ctx context.Context) error
	Get(ctx context.Context, bookID, itemID string) (*Article, error)
	Put(ctx context.Context, a *Article) error
	Delete(ctx context.Context, bookID, itemID string) error
	DeleteByBook(ctx context.Context, bookID string) (int64, error)
	DeleteAll(ctx context.Context) (int64, error)
	Count(ctx context.Context) (int64, error)
	SumSize(ctx context.Context) (int64, error)
}
