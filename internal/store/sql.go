package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	// 纯 Go 的 SQLite 驱动，CGO_ENABLED=0 可编译，注册名为 sqlite。
	_ "modernc.org/sqlite"
)

type sqlStore struct {
	db *sql.DB
	d  dialect
}

// NewSQLStore 用现成的 *sql.DB 和方言构建存储。
func NewSQLStore(db *sql.DB, d dialect) *sqlStore {
	return &sqlStore{db: db, d: d}
}

// Open 按 driver 建立连接。sqlite 已实现，postgres / mysql / mongo 后续任务接入。
func Open(driver, dsn string) (ArticleStore, error) {
	switch driver {
	case "":
		return nil, fmt.Errorf("storage.driver 未配置")
	case "sqlite":
		// busy_timeout 兜住与统计连接之间的并发写锁竞争。
		if dsn != ":memory:" && !strings.Contains(dsn, "_pragma") {
			sep := "?"
			if strings.Contains(dsn, "?") {
				sep = "&"
			}
			dsn += sep + "_pragma=busy_timeout(5000)"
		}
		if dsn != ":memory:" {
			// SQLite 不会创建父目录，落盘前先建。
			if dir := filepath.Dir(dsn); dir != "" && dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return nil, fmt.Errorf("创建数据目录: %w", err)
				}
			}
		}
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, fmt.Errorf("打开 sqlite: %w", err)
		}
		if dsn == ":memory:" {
			// 内存库每个连接是独立的库，必须单连接才能共享数据。
			db.SetMaxOpenConns(1)
		}
		return NewSQLStore(db, sqliteDialect{}), nil
	default:
		return nil, fmt.Errorf("存储驱动 %s 尚未实现", driver)
	}
}

func (s *sqlStore) Init(ctx context.Context) error {
	for _, stmt := range s.d.CreateSQL() {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("建表: %w", err)
		}
	}
	return nil
}

func (s *sqlStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *sqlStore) Get(ctx context.Context, bookID, itemID string) (*Article, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT item_id, book_id, title, content, size_bytes, created_at, updated_at
		 FROM articles WHERE item_id = ? AND book_id = ?`, itemID, bookID)
	var a Article
	err := row.Scan(&a.ItemID, &a.BookID, &a.Title, &a.Content, &a.SizeBytes, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询文章: %w", err)
	}
	return &a, nil
}

func (s *sqlStore) Put(ctx context.Context, a *Article) error {
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	// size_bytes 由存储层统一按内容字节数计算。
	a.SizeBytes = int64(len(a.Content))
	_, err := s.db.ExecContext(ctx, s.d.UpsertArticleSQL(),
		a.ItemID, a.BookID, a.Title, a.Content, a.SizeBytes, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("写入文章: %w", err)
	}
	return nil
}

func (s *sqlStore) Delete(ctx context.Context, bookID, itemID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM articles WHERE item_id = ? AND book_id = ?`, itemID, bookID)
	if err != nil {
		return fmt.Errorf("删除文章: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *sqlStore) DeleteByBook(ctx context.Context, bookID string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM articles WHERE book_id = ?`, bookID)
	if err != nil {
		return 0, fmt.Errorf("按书删除: %w", err)
	}
	return res.RowsAffected()
}

func (s *sqlStore) DeleteAll(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM articles`)
	if err != nil {
		return 0, fmt.Errorf("清空缓存: %w", err)
	}
	return res.RowsAffected()
}

func (s *sqlStore) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM articles`).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计条数: %w", err)
	}
	return n, nil
}

func (s *sqlStore) SumSize(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes), 0) FROM articles`).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计体积: %w", err)
	}
	return n, nil
}
