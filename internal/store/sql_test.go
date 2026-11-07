package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 内存库走真实 SQL 路径，禁止连外部数据库。
func newTestStore(t *testing.T) ArticleStore {
	t.Helper()
	s, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Ping(context.Background()) })
	return s
}

func TestPutGetRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := &Article{ItemID: "7526766660229808665", BookID: "7526452300462705689", Title: "第1章 重生", Content: "<p>正文</p>"}
	if err := s.Put(ctx, in); err != nil {
		t.Fatal(err)
	}
	out, err := s.Get(ctx, "7526452300462705689", "7526766660229808665")
	if err != nil {
		t.Fatal(err)
	}
	if out.ItemID != in.ItemID || out.BookID != in.BookID || out.Title != in.Title || out.Content != in.Content {
		t.Fatalf("roundtrip 不符: %+v", out)
	}
	if out.SizeBytes != int64(len(in.Content)) {
		t.Fatalf("size_bytes = %d", out.SizeBytes)
	}
	if out.CreatedAt.IsZero() || out.UpdatedAt.IsZero() {
		t.Fatal("时间戳未填")
	}
}

func TestGetMissIsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Get(context.Background(), "1", "2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	// item_id 命中但 book_id 不符同样算未命中。
	ctx := context.Background()
	if err := s.Put(ctx, &Article{ItemID: "2", BookID: "1", Title: "t", Content: "c"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "9", "2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPutUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.Put(ctx, &Article{ItemID: "2", BookID: "1", Title: "旧标题", Content: "旧内容"}); err != nil {
		t.Fatal(err)
	}
	first, _ := s.Get(ctx, "1", "2")

	time.Sleep(10 * time.Millisecond) // 保证 updated_at 可观测地前移
	if err := s.Put(ctx, &Article{ItemID: "2", BookID: "1", Title: "新标题", Content: "新内容"}); err != nil {
		t.Fatal(err)
	}
	out, err := s.Get(ctx, "1", "2")
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "新标题" || out.Content != "新内容" {
		t.Fatalf("upsert 未生效: %+v", out)
	}
	if !out.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("created_at 应保留首次写入值: %v vs %v", out.CreatedAt, first.CreatedAt)
	}
	if !out.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("updated_at 应前移: %v vs %v", out.UpdatedAt, first.UpdatedAt)
	}
}

func TestDeleteAndStats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, a := range []Article{
		{ItemID: "1", BookID: "100", Title: "a", Content: "12345"},
		{ItemID: "2", BookID: "100", Title: "b", Content: "123"},
		{ItemID: "3", BookID: "200", Title: "c", Content: "1234567"},
	} {
		if err := s.Put(ctx, &a); err != nil {
			t.Fatal(err)
		}
	}

	if n, err := s.Count(ctx); err != nil || n != 3 {
		t.Fatalf("count = %d, %v", n, err)
	}
	if n, err := s.SumSize(ctx); err != nil || n != 15 {
		t.Fatalf("sum = %d, %v", n, err)
	}

	if n, err := s.DeleteByBook(ctx, "100"); err != nil || n != 2 {
		t.Fatalf("delete by book = %d, %v", n, err)
	}
	if err := s.Delete(ctx, "100", "1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除不存在的行应 ErrNotFound: %v", err)
	}
	if err := s.Delete(ctx, "200", "3"); err != nil {
		t.Fatal(err)
	}
	if n, err := s.DeleteAll(ctx); err != nil || n != 0 {
		t.Fatalf("delete all = %d, %v", n, err)
	}
	if n, _ := s.Count(ctx); n != 0 {
		t.Fatalf("清空后 count = %d", n)
	}
}

func TestOpenRejectsUnknownDriver(t *testing.T) {
	if _, err := Open("", "./x.db"); err == nil {
		t.Fatal("空 driver 应报错")
	}
	if _, err := Open("redis", "./x.db"); err == nil {
		t.Fatal("未知 driver 应报错")
	}
}
