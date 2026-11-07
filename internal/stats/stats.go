// Package stats 维护 request_log 与 audit_log 两张表、异步写入、
// 90 天保留清理，以及统计聚合查询。数据库与文章缓存共用同一实例，
// storage 关闭时 Stats 为 nil，三组接口返回 50301（Q3-1）。
package stats

import (
	"log/slog"
	"sync/atomic"
	"time"
)

// 保留期与清理周期（包级变量便于测试下调）。
var (
	retention    = 90 * 24 * time.Hour
	cleanupEvery = time.Hour
)

// RequestRow request_log 一行。Path 为请求目标（含查询串，
// top_books 从中解析 book_id）；Query 单独存 search 的 q。
type RequestRow struct {
	Time       time.Time
	Path       string
	Status     int
	Code       int
	DurationMS int64
	UpstreamMS int64
	Bytes      int64
	TraceID    string
	Query      string
}

// AuditRow audit_log 一行。
type AuditRow struct {
	Time   time.Time `json:"ts"`
	Event  string    `json:"event"`
	Actor  string    `json:"actor"`
	IP     string    `json:"ip"`
	UA     string    `json:"ua"`
	Detail string    `json:"detail"`
	Result string    `json:"result"`
}

// Stats 异步写入器。写入通道满时丢弃并计一次 dropped（统计自身不许
// 拖垮请求路径）。关闭走 quit channel：通道本身永不 close，
// 并发的 Record / Audit 不会因向已关闭通道发送而 panic。
type Stats struct {
	db      *sqlDB
	quit    chan struct{}
	reqCh   chan RequestRow
	auditCh chan AuditRow
	dropped atomic.Int64
	stopped atomic.Bool
	done    chan struct{}
}

// Audit 记审计事件；nil 接收者静默（storage 未启用时 handler 直接透传）。
func (s *Stats) Audit(event, actor, ip, ua, detail, result string) {
	if s == nil {
		return
	}
	s.sendAudit(AuditRow{Time: time.Now(), Event: event, Actor: actor, IP: ip, UA: ua, Detail: detail, Result: result})
}

// AuditError 便捷封装：result 取 err 文本，err 为 nil 时记 ok。
func (s *Stats) AuditError(event, actor, ip, ua, detail string, err error) {
	result := "ok"
	if err != nil {
		result = err.Error()
	}
	s.Audit(event, actor, ip, ua, detail, result)
}

// Record 记一行请求日志；nil 接收者静默。
func (s *Stats) Record(r RequestRow) {
	if s == nil {
		return
	}
	s.sendReq(r)
}

// Dropped 返回因通道满被丢弃的行数（诊断用）。
func (s *Stats) Dropped() int64 { return s.dropped.Load() }

func (s *Stats) sendReq(r RequestRow) {
	select {
	case <-s.quit:
		s.dropped.Add(1)
		return
	default:
	}
	select {
	case s.reqCh <- r:
	default:
		s.dropped.Add(1)
	}
}

func (s *Stats) sendAudit(a AuditRow) {
	select {
	case <-s.quit:
		s.dropped.Add(1)
		return
	default:
	}
	select {
	case s.auditCh <- a:
	default:
		s.dropped.Add(1)
	}
}

// writer 协程：批量落表 + 定期清理超期行。收到 quit 后排空缓冲再退出。
func (s *Stats) writer(lg *slog.Logger) {
	defer close(s.done)
	cleanup := time.NewTicker(cleanupEvery)
	defer cleanup.Stop()
	for {
		select {
		case <-s.quit:
			s.drain()
			return
		case r := <-s.reqCh:
			s.insertRequest(r)
		case a := <-s.auditCh:
			s.insertAudit(a)
		case <-cleanup.C:
			cutoff := time.Now().Add(-retention)
			if n := s.deleteOlderThan(cutoff); n > 0 && lg != nil {
				lg.Info("stats 过期数据清理", "rows", n)
			}
		}
	}
}

// drain 非阻塞排空两个缓冲中已排队的行。
func (s *Stats) drain() {
	for {
		select {
		case r := <-s.reqCh:
			s.insertRequest(r)
		case a := <-s.auditCh:
			s.insertAudit(a)
		default:
			return
		}
	}
}
