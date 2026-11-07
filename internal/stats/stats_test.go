package stats

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

func newTestStats(t *testing.T) *Stats {
	t.Helper()
	s, err := New("sqlite", filepath.Join(t.TempDir(), "stats.db"), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// waitRows 轮询等待 request_log 达到目标行数（异步写入是尽力而为的）。
func waitRows(t *testing.T, s *Stats, want int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var n int64
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM request_log`).Scan(&n); err == nil && n >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待写入超时: got %d want %d", n, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitAuditRows 轮询等待 audit_log 达到目标行数。
func waitAuditRows(t *testing.T, s *Stats, want int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var n int64
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n); err == nil && n >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待审计写入超时: got %d want %d", n, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRequestLogRoundTripAndAggregation(t *testing.T) {
	s := newTestStats(t)
	now := time.Now()
	rows := []RequestRow{
		{Time: now, Path: "/api/search?q=a", Status: 200, Code: 0, DurationMS: 10, TraceID: "t1", Query: "a"},
		{Time: now, Path: "/api/search?q=b", Status: 200, Code: 0, DurationMS: 20, TraceID: "t2", Query: "b"},
		{Time: now, Path: "/api/detail?book_id=7526452300462705689", Status: 502, Code: 50203, DurationMS: 30, TraceID: "t3"},
		{Time: now.Add(-time.Hour), Path: "/api/content", Status: 200, Code: 0, DurationMS: 5, TraceID: "t4"},
	}
	for _, r := range rows {
		s.Record(r)
	}
	waitRows(t, s, 4)

	ov, err := s.ComputeOverview(0.5)
	if err != nil {
		t.Fatal(err)
	}
	// 今日 3 行（1 小时前那行不算今天？UTC 截断可能包含——按 >= today 统计，
	// 测试环境 now-1h 仍可能在同一 UTC 日内，这里只断言下限）。
	if ov.TodayRequests < 3 {
		t.Fatalf("today_requests = %d", ov.TodayRequests)
	}
	if ov.TodayErrors < 1 {
		t.Fatalf("today_errors = %d", ov.TodayErrors)
	}
	if ov.UpstreamHealth != "down" {
		t.Fatalf("5xx 占比应判定 down: %s", ov.UpstreamHealth)
	}

	q, err := s.ComputeQuery(time.Time{}, time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(q.TopQueries) != 2 {
		t.Fatalf("top_queries = %+v", q.TopQueries)
	}
	if len(q.TopBooks) != 1 || q.TopBooks[0].Name != "7526452300462705689" {
		t.Fatalf("top_books = %+v", q.TopBooks)
	}
	if q.ErrorCodes["50203"] < 1 {
		t.Fatalf("error_codes 缺 50203: %+v", q.ErrorCodes)
	}
	if q.P50 == 0 || q.P99 < q.P50 {
		t.Fatalf("分位数异常: %+v", q)
	}
}

func TestUpstreamHealthThresholds(t *testing.T) {
	s := newTestStats(t)
	now := time.Now()
	// 201 条时间递增：最后 11 条为 5xx（11/201 约 5.5% → degraded）。
	for i := 0; i < 190; i++ {
		s.Record(RequestRow{Time: now.Add(time.Duration(i) * time.Millisecond), Path: "/api/search", Status: 200, Code: 0, DurationMS: 1, TraceID: "ok"})
	}
	for i := 0; i < 11; i++ {
		s.Record(RequestRow{Time: now.Add(time.Duration(190+i) * time.Millisecond), Path: "/api/search", Status: 500, Code: 50001, DurationMS: 1, TraceID: "e"})
	}
	waitRows(t, s, 201)
	ov, err := s.ComputeOverview(0)
	if err != nil {
		t.Fatal(err)
	}
	if ov.UpstreamHealth != "degraded" {
		t.Fatalf("5%% 上下应为 degraded: %s", ov.UpstreamHealth)
	}
}

func TestAuditQueryPagination(t *testing.T) {
	s := newTestStats(t)
	for i := 0; i < 5; i++ {
		s.Audit("login_success", "boss", "1.1.1.1", "ua", "", "ok")
	}
	s.Audit("flag_change", "boss", "1.1.1.1", "ua", "name=search", "ok")
	waitAuditRows(t, s, 6)

	page, err := s.QueryAudit("", time.Time{}, time.Time{}, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 6 || len(page.Items) != 3 {
		t.Fatalf("total=%d items=%d", page.Total, len(page.Items))
	}
	byEvent, err := s.QueryAudit("flag_change", time.Time{}, time.Time{}, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if byEvent.Total != 1 {
		t.Fatalf("按 event 过滤 = %d", byEvent.Total)
	}
	// 未来时间过滤为空。
	future, err := s.QueryAudit("", time.Now().Add(time.Hour), time.Time{}, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if future.Total != 0 {
		t.Fatalf("未来时间应为空: %d", future.Total)
	}
}

func TestNilReceiverSilent(t *testing.T) {
	var s *Stats
	s.Audit("x", "y", "z", "u", "d", "r") // 不应 panic
	s.Record(RequestRow{})                // 不应 panic
	if _, err := s.ListArticles(1, 50, "", ""); err == nil {
		t.Fatal("nil stats 的查询应报错")
	}
}

func TestCacheListAndLookup(t *testing.T) {
	s := newTestStats(t)
	// 建文章表并插入两行（与 store 同构）。
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS articles (
		item_id varchar(32) PRIMARY KEY, book_id varchar(32) NOT NULL,
		title varchar(512) NOT NULL DEFAULT '', content text NOT NULL DEFAULT '',
		size_bytes bigint NOT NULL DEFAULT 0,
		created_at timestamp NOT NULL, updated_at timestamp NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, a := range [][2]string{{"1", "100"}, {"2", "100"}} {
		if _, err := s.db.Exec(`INSERT INTO articles (item_id, book_id, title, content, size_bytes, created_at, updated_at)
			VALUES (?, ?, 't', 'c', 3, ?, ?)`, a[0], a[1], now, now); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.ListArticles(1, 50, "100", "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.TotalSize != 6 || len(page.Items) != 2 {
		t.Fatalf("page = %+v", page)
	}
	if page.Items[0].TTLLeft != 0 {
		t.Fatal("无 TTL 概念时 ttl_left_seconds 应为 0")
	}
	bookID, err := s.LookupArticleBookID("1")
	if err != nil || bookID != "100" {
		t.Fatalf("lookup = %q %v", bookID, err)
	}
	if _, err := s.LookupArticleBookID("nope"); err == nil {
		t.Fatal("查不到应报错")
	}
}
