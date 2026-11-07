package store

// dialect 封装 SQL 方言差异：建表语句与 upsert 语句。
// 普通查询用的是各库都兼容的 ANSI 写法，不需要进 dialect。
type dialect interface {
	// CreateSQL 返回按顺序执行的建表 / 建索引语句，要求幂等。
	CreateSQL() []string
	// UpsertArticleSQL 占位符顺序：item_id, book_id, title, content,
	// size_bytes, created_at, updated_at。冲突时保留原 created_at。
	UpsertArticleSQL() string
}

type sqliteDialect struct{}

func (sqliteDialect) CreateSQL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS articles (
			item_id     varchar(32) PRIMARY KEY,
			book_id     varchar(32) NOT NULL,
			title       varchar(512) NOT NULL DEFAULT '',
			content     text NOT NULL DEFAULT '',
			size_bytes  bigint NOT NULL DEFAULT 0,
			created_at  timestamp NOT NULL,
			updated_at  timestamp NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_articles_book_id ON articles(book_id)`,
	}
}

func (sqliteDialect) UpsertArticleSQL() string {
	return `INSERT INTO articles
		(item_id, book_id, title, content, size_bytes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(item_id) DO UPDATE SET
			book_id = excluded.book_id,
			title = excluded.title,
			content = excluded.content,
			size_bytes = excluded.size_bytes,
			updated_at = excluded.updated_at`
}
