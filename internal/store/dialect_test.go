package store

import (
	"strings"
	"testing"
)

// 纯字符串断言，不连库（W10：单测只测方言 DDL 字符串输出）。
func TestSQLiteDialect(t *testing.T) {
	d := sqliteDialect{}
	sqls := d.CreateSQL()
	if len(sqls) != 2 {
		t.Fatalf("sqlite 应有建表与建索引两条: %d", len(sqls))
	}
	if !strings.Contains(sqls[0], "CREATE TABLE IF NOT EXISTS articles") ||
		!strings.Contains(sqls[0], "text") {
		t.Fatalf("sqlite 建表不符: %s", sqls[0])
	}
	if !strings.Contains(sqls[1], "CREATE INDEX IF NOT EXISTS") {
		t.Fatalf("sqlite 建索引不符: %s", sqls[1])
	}
	up := d.UpsertArticleSQL()
	if !strings.Contains(up, "ON CONFLICT(item_id) DO UPDATE") || strings.Count(up, "?") != 7 {
		t.Fatalf("sqlite upsert 不符: %s", up)
	}
	if d.Placeholder(1) != "?" || d.Placeholder(7) != "?" {
		t.Fatal("sqlite 占位符应为 ?")
	}
}

func TestMySQLDialect(t *testing.T) {
	d := mysqlDialect{}
	sqls := d.CreateSQL()
	if len(sqls) != 2 {
		t.Fatalf("mysql 应有建表与建索引两条: %d", len(sqls))
	}
	if !strings.Contains(sqls[0], "longtext") || !strings.Contains(sqls[0], "datetime") {
		t.Fatalf("mysql 建表应用 longtext/datetime: %s", sqls[0])
	}
	up := d.UpsertArticleSQL()
	if !strings.Contains(up, "ON DUPLICATE KEY UPDATE") || !strings.Contains(up, "VALUES(") {
		t.Fatalf("mysql upsert 不符: %s", up)
	}
	if d.Placeholder(3) != "?" {
		t.Fatal("mysql 占位符应为 ?")
	}
}

func TestPostgresDialect(t *testing.T) {
	d := postgresDialect{}
	sqls := d.CreateSQL()
	if !strings.Contains(sqls[0], "timestamptz") {
		t.Fatalf("postgres 建表应用 timestamptz: %s", sqls[0])
	}
	if !strings.Contains(sqls[1], "IF NOT EXISTS") {
		t.Fatalf("postgres 建索引应幂等: %s", sqls[1])
	}
	up := d.UpsertArticleSQL()
	if !strings.Contains(up, "$1") || !strings.Contains(up, "$7") {
		t.Fatalf("postgres upsert 占位符不符: %s", up)
	}
	if d.Placeholder(1) != "$1" || d.Placeholder(12) != "$12" {
		t.Fatalf("postgres 占位符不符: %s %s", d.Placeholder(1), d.Placeholder(12))
	}
}
