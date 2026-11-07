package stats

import (
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	// 纯 Go SQLite 驱动（与 store 一致）。
	_ "modernc.org/sqlite"
)

// sqlDB 包内对 *sql.DB 的别名，方便测试替换。
type sqlDB = sql.DB

// New 打开统计库（与文章缓存同一 driver/dsn），建表并启动写入协程。
func New(driver, dsn string, lg *slog.Logger) (*Stats, error) {
	if driver != "sqlite" {
		return nil, fmt.Errorf("统计库暂只支持 sqlite，其余驱动待接入")
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开统计库: %w", err)
	}
	// 单文件库：单连接避免并发写锁。
	db.SetMaxOpenConns(1)
	s := &Stats{
		db:      db,
		reqCh:   make(chan RequestRow, 1024),
		auditCh: make(chan AuditRow, 1024),
		done:    make(chan struct{}),
	}
	if err := s.ensureTables(); err != nil {
		db.Close()
		return nil, err
	}
	go s.writer(lg)
	return s, nil
}

// Close 排空通道并关闭数据库。
func (s *Stats) Close() {
	if s == nil {
		return
	}
	if !s.stopped.CompareAndSwap(false, true) {
		return
	}
	close(s.reqCh)
	close(s.auditCh)
	<-s.done
	_ = s.db.Close()
}

func (s *Stats) ensureTables() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS request_log (
			time        timestamp NOT NULL,
			path        varchar(512) NOT NULL,
			status      int NOT NULL,
			code        int NOT NULL,
			duration_ms bigint NOT NULL,
			upstream_ms bigint NOT NULL,
			bytes       bigint NOT NULL,
			trace_id    varchar(32) NOT NULL,
			query       varchar(256) NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_request_log_time ON request_log(time)`,
		`CREATE TABLE IF NOT EXISTS audit_log (
			time   timestamp NOT NULL,
			event  varchar(64) NOT NULL,
			actor  varchar(64) NOT NULL,
			ip     varchar(64) NOT NULL,
			ua     varchar(256) NOT NULL,
			detail varchar(1024) NOT NULL,
			result varchar(256) NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_log(time)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("建统计表: %w", err)
		}
	}
	return nil
}

func (s *Stats) insertRequest(r RequestRow) {
	_, err := s.db.Exec(
		`INSERT INTO request_log (time, path, status, code, duration_ms, upstream_ms, bytes, trace_id, query)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Time.UTC(), r.Path, r.Status, r.Code, r.DurationMS, r.UpstreamMS, r.Bytes, r.TraceID, r.Query)
	if err != nil {
		s.dropped.Add(1)
	}
}

func (s *Stats) insertAudit(a AuditRow) {
	_, err := s.db.Exec(
		`INSERT INTO audit_log (time, event, actor, ip, ua, detail, result)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.Time.UTC(), a.Event, a.Actor, a.IP, a.UA, a.Detail, a.Result)
	if err != nil {
		s.dropped.Add(1)
	}
}

func (s *Stats) deleteOlderThan(cutoff time.Time) int64 {
	var n int64
	_ = s.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM request_log WHERE time < ?) +
		(SELECT COUNT(*) FROM audit_log WHERE time < ?)`, cutoff, cutoff).Scan(&n)
	_, _ = s.db.Exec(`DELETE FROM request_log WHERE time < ?`, cutoff)
	_, _ = s.db.Exec(`DELETE FROM audit_log WHERE time < ?`, cutoff)
	return n
}

// ArticlePage /cache 管理接口的列表项。ttl_left_seconds 恒为 0：
// 当前没有缓存 TTL 概念，条目不过期。
type ArticlePage struct {
	Total     int64       `json:"total"`
	TotalSize int64       `json:"total_bytes"`
	HitRate   float64     `json:"hit_rate"`
	Items     []CacheItem `json:"items"`
}

type CacheItem struct {
	ItemID    string `json:"item_id"`
	BookID    string `json:"book_id"`
	Title     string `json:"title"`
	SizeBytes int64  `json:"size_bytes"`
	CreatedAt int64  `json:"created_at"`
	TTLLeft   int64  `json:"ttl_left_seconds"`
}

// ListArticles 分页查文章缓存（供 /cache 管理接口）。
func (s *Stats) ListArticles(page, size int, bookID, chapterID string) (*ArticlePage, error) {
	if s == nil {
		return nil, fmt.Errorf("统计未启用")
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 50
	}
	where := " WHERE 1=1"
	args := []any{}
	if bookID != "" {
		where += " AND book_id = ?"
		args = append(args, bookID)
	}
	if chapterID != "" {
		where += " AND item_id = ?"
		args = append(args, chapterID)
	}
	out := &ArticlePage{Items: []CacheItem{}}
	if err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM articles`+where,
		args...).Scan(&out.Total, &out.TotalSize); err != nil {
		return nil, fmt.Errorf("统计缓存: %w", err)
	}
	rows, err := s.db.Query(`SELECT item_id, book_id, title, size_bytes, created_at FROM articles`+
		where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, fmt.Errorf("查询缓存: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var it CacheItem
		var ts time.Time
		if err := rows.Scan(&it.ItemID, &it.BookID, &it.Title, &it.SizeBytes, &ts); err != nil {
			return nil, err
		}
		it.CreatedAt = ts.Unix()
		out.Items = append(out.Items, it)
	}
	return out, rows.Err()
}

// LookupArticleBookID 按 item_id 查 book_id（DELETE /cache/{item_id} 用）。
func (s *Stats) LookupArticleBookID(itemID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("统计未启用")
	}
	var bookID string
	err := s.db.QueryRow(`SELECT book_id FROM articles WHERE item_id = ?`, itemID).Scan(&bookID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("条目不存在")
	}
	return bookID, err
}

// Overview 概览聚合（Q3 口径）。
type Overview struct {
	TodayRequests  int64            `json:"today_requests"`
	TodayErrors    int64            `json:"today_errors"`
	P95MS          int64            `json:"p95_ms"`
	CacheHitRate   float64          `json:"cache_hit_rate"`
	PerAPI         map[string]any   `json:"per_api"`
	UpstreamHealth string           `json:"upstream_health"`
	RecentErrors   []RequestSummary `json:"recent_errors"`
	Version        string           `json:"version"`
	UptimeSeconds  int64            `json:"uptime_seconds"`
}

type RequestSummary struct {
	Time    int64  `json:"time"`
	Path    string `json:"path"`
	Status  int    `json:"status"`
	Code    int    `json:"code"`
	TraceID string `json:"trace_id"`
}

// ComputeOverview 计算今日概览；hitRate 为缓存命中率（store 计数器）。
func (s *Stats) ComputeOverview(hitRate float64) (*Overview, error) {
	if s == nil {
		return nil, fmt.Errorf("统计未启用")
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	out := &Overview{PerAPI: map[string]any{}, UpstreamHealth: "ok", RecentErrors: []RequestSummary{}}

	if err := s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN code != 0 THEN 1 ELSE 0 END), 0)
		 FROM request_log WHERE time >= ?`, today).Scan(&out.TodayRequests, &out.TodayErrors); err != nil {
		return nil, err
	}

	// p95：取今日时长升序的第 95 百分位。
	durs, err := s.durationsSince(today)
	if err != nil {
		return nil, err
	}
	sortSlice(durs)
	out.P95MS = percentile(durs, 95)

	// upstream_health：最近 100 条中 5xx 或超时的占比。
	rows, err := s.db.Query(
		`SELECT status, code FROM request_log ORDER BY time DESC, rowid DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	total, bad := 0, 0
	for rows.Next() {
		var status, code int
		if err := rows.Scan(&status, &code); err != nil {
			rows.Close()
			return nil, err
		}
		total++
		if status >= 500 || code == 50201 {
			bad++
		}
	}
	rows.Close()
	if total > 0 {
		ratio := float64(bad) / float64(total)
		switch {
		case ratio > 0.20:
			out.UpstreamHealth = "down"
		case ratio > 0.05:
			out.UpstreamHealth = "degraded"
		}
	}

	// per_api：按路径前缀聚合今日请求数与错误数。
	apiRows, err := s.db.Query(
		`SELECT path, COUNT(*), COALESCE(SUM(CASE WHEN code != 0 THEN 1 ELSE 0 END), 0)
		 FROM request_log WHERE time >= ? GROUP BY path`, today)
	if err != nil {
		return nil, err
	}
	defer apiRows.Close()
	perPath := map[string][2]int64{}
	for apiRows.Next() {
		var p string
		var cnt, errs int64
		if err := apiRows.Scan(&p, &cnt, &errs); err != nil {
			return nil, err
		}
		name := apiName(p)
		agg := perPath[name]
		agg[0] += cnt
		agg[1] += errs
		perPath[name] = agg
	}
	apiRows.Close()
	for name, agg := range perPath {
		out.PerAPI[name] = map[string]int64{"requests": agg[0], "errors": agg[1]}
	}
	out.CacheHitRate = hitRate

	// recent_errors：最近 20 条非 2xx 或非 0 code。
	errRows, err := s.db.Query(
		`SELECT time, path, status, code, trace_id FROM request_log
		 WHERE code != 0 OR status >= 400 ORDER BY time DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer errRows.Close()
	for errRows.Next() {
		var r RequestSummary
		var ts time.Time
		if err := errRows.Scan(&ts, &r.Path, &r.Status, &r.Code, &r.TraceID); err != nil {
			return nil, err
		}
		r.Time = ts.Unix()
		out.RecentErrors = append(out.RecentErrors, r)
	}
	return out, errRows.Err()
}

// QueryResult /stats/query 的聚合结果。
type QueryResult struct {
	Buckets    []Bucket         `json:"buckets"`
	P50        int64            `json:"p50"`
	P95        int64            `json:"p95"`
	P99        int64            `json:"p99"`
	ErrorCodes map[string]int64 `json:"error_codes"`
	TopQueries []NameCount      `json:"top_queries"`
	TopBooks   []NameCount      `json:"top_books"`
}

type Bucket struct {
	Time     int64 `json:"time"`
	Requests int64 `json:"requests"`
	Errors   int64 `json:"errors"`
}

type NameCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// ComputeQuery 时间范围（空即不限）+ 可选 api 前缀过滤的聚合。
func (s *Stats) ComputeQuery(from, to time.Time, apiPrefix string) (*QueryResult, error) {
	if s == nil {
		return nil, fmt.Errorf("统计未启用")
	}
	where := " WHERE 1=1"
	args := []any{}
	if !from.IsZero() {
		where += " AND time >= ?"
		args = append(args, from.UTC())
	}
	if !to.IsZero() {
		where += " AND time < ?"
		args = append(args, to.UTC())
	}
	if apiPrefix != "" {
		where += " AND path LIKE ?"
		args = append(args, apiPrefix+"%")
	}

	out := &QueryResult{Buckets: []Bucket{}, ErrorCodes: map[string]int64{}, TopQueries: []NameCount{}, TopBooks: []NameCount{}}

	type row struct {
		ts    time.Time
		code  int
		dur   int64
		path  string
		query string
	}
	var all []row
	q := `SELECT time, code, duration_ms, path, query FROM request_log` + where
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ts, &r.code, &r.dur, &r.path, &r.query); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, r)
	}
	rows.Close()

	// 小时桶。
	bucketAgg := map[int64]*Bucket{}
	durs := make([]int64, 0, len(all))
	for _, r := range all {
		h := r.ts.UTC().Truncate(time.Hour)
		b, ok := bucketAgg[h.Unix()]
		if !ok {
			b = &Bucket{Time: h.Unix()}
			bucketAgg[h.Unix()] = b
		}
		b.Requests++
		if r.code != 0 {
			b.Errors++
			out.ErrorCodes[itoa(r.code)]++
		}
		durs = append(durs, r.dur)
	}
	for _, b := range bucketAgg {
		out.Buckets = append(out.Buckets, *b)
	}
	sortBuckets(out.Buckets)
	sortSlice(durs)
	out.P50 = percentile(durs, 50)
	out.P95 = percentile(durs, 95)
	out.P99 = percentile(durs, 99)

	// top_queries：query 列非空聚合。
	qc := map[string]int64{}
	bc := map[string]int64{}
	for _, r := range all {
		if r.query != "" {
			qc[r.query]++
		}
		if strings.HasPrefix(r.path, "/api/content") || strings.HasPrefix(r.path, "/api/detail") {
			if id := extractParam(r.path, "book_id"); id != "" {
				bc[id]++
			}
		}
	}
	out.TopQueries = topN(qc, 10)
	out.TopBooks = topN(bc, 10)
	return out, nil
}

// AuditPage 审计分页结果。
type AuditPage struct {
	Total int64      `json:"total"`
	Items []AuditRow `json:"items"`
}

// QueryAudit 审计查询：event 精确匹配，from/to 时间过滤，分页。
func (s *Stats) QueryAudit(event string, from, to time.Time, page, size int) (*AuditPage, error) {
	if s == nil {
		return nil, fmt.Errorf("统计未启用")
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 50
	}
	where := " WHERE 1=1"
	args := []any{}
	if event != "" {
		where += " AND event = ?"
		args = append(args, event)
	}
	if !from.IsZero() {
		where += " AND time >= ?"
		args = append(args, from.UTC())
	}
	if !to.IsZero() {
		where += " AND time < ?"
		args = append(args, to.UTC())
	}
	out := &AuditPage{Items: []AuditRow{}}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_log`+where, args...).Scan(&out.Total); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT time, event, actor, ip, ua, detail, result FROM audit_log`+
		where+` ORDER BY time DESC LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a AuditRow
		var ts time.Time
		if err := rows.Scan(&ts, &a.Event, &a.Actor, &a.IP, &a.UA, &a.Detail, &a.Result); err != nil {
			return nil, err
		}
		a.Time = ts
		out.Items = append(out.Items, a)
	}
	return out, rows.Err()
}

func (s *Stats) durationsSince(since time.Time) ([]int64, error) {
	rows, err := s.db.Query(`SELECT duration_ms FROM request_log WHERE time >= ?`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var d int64
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func percentile(sorted []int64, p int) int64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p) / 100 * float64(n)))
	if idx < 1 {
		idx = 1
	}
	if idx > n {
		idx = n
	}
	return sorted[idx-1]
}

func sortBuckets(bs []Bucket) {
	for i := 1; i < len(bs); i++ {
		for j := i; j > 0 && bs[j].Time < bs[j-1].Time; j-- {
			bs[j], bs[j-1] = bs[j-1], bs[j]
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func sortSlice(v []int64) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

func topN(m map[string]int64, n int) []NameCount {
	out := make([]NameCount, 0, len(m))
	for k, v := range m {
		out = append(out, NameCount{Name: k, Count: v})
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Count > out[j-1].Count; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// extractParam 从完整请求目标里取查询参数值。
func extractParam(target, key string) string {
	i := strings.Index(target, "?")
	if i < 0 {
		return ""
	}
	for _, kv := range strings.Split(target[i+1:], "&") {
		if strings.HasPrefix(kv, key+"=") {
			v := strings.TrimPrefix(kv, key+"=")
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// apiName 由请求目标推导接口名（管理端路径归入 admin）。
func apiName(target string) string {
	switch {
	case strings.HasPrefix(target, "/api/search"):
		return "search"
	case strings.HasPrefix(target, "/api/detail"):
		return "detail"
	case strings.HasPrefix(target, "/api/catalog"):
		return "catalog"
	case strings.HasPrefix(target, "/api/content"):
		return "content"
	default:
		return "other"
	}
}
