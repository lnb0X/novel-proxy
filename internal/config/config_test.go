package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// loadFiles 在临时目录写两个配置文件后 Load。
func loadFiles(t *testing.T, configYaml, instanceYaml string) (Config, error) {
	t.Helper()
	dir := t.TempDir()
	cp := writeFile(t, dir, "config.yaml", configYaml)
	ip := filepath.Join(dir, "data", "instance.yaml")
	if instanceYaml != "" {
		if err := os.MkdirAll(filepath.Dir(ip), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, "data"), "instance.yaml", instanceYaml)
	}
	return Load(cp, ip)
}

func TestDefaults(t *testing.T) {
	// config.yaml 全键省略，且 instance.yaml 不存在。
	c, err := loadFiles(t, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, defaults()) {
		t.Fatalf("默认值不符:\n got %+v\nwant %+v", c, defaults())
	}
}

func TestInstanceOverridesConfig(t *testing.T) {
	configYaml := `
server:
  addr: ":1000"
log:
  level: debug
storage:
  enabled: false
rate_limit:
  search:
    mode: per_ip
    per_minute: 60
    burst: 10
content_rules:
  default:
    replace:
      - pattern: "a"
        replacement: "b"
        regex: false
`
	instanceYaml := `
server:
  addr: ":2000"
storage:
  enabled: true
  driver: sqlite
  dsn: "./data/cache.db"
content_rules:
  default:
    prepend: "片头"
  books:
    "7526452300462705689":
      override: "整章接管"
`
	c, err := loadFiles(t, configYaml, instanceYaml)
	if err != nil {
		t.Fatal(err)
	}
	// instance 覆盖 config。
	if c.Server.Addr != ":2000" {
		t.Errorf("addr = %q", c.Server.Addr)
	}
	if c.Storage.Enabled != true || c.Storage.Driver != "sqlite" || c.Storage.DSN != "./data/cache.db" {
		t.Errorf("storage 覆盖不符: %+v", c.Storage)
	}
	// config 独有的键保留。
	if c.Log.Level != "debug" {
		t.Errorf("log.level 应保留 config 值: %q", c.Log.Level)
	}
	if c.RateLimit.Search.Mode != "per_ip" || c.RateLimit.Search.PerMinute != 60 {
		t.Errorf("rate_limit.search 应保留 config 值: %+v", c.RateLimit.Search)
	}
	// content_rules 深层字段。
	if c.ContentRules.Default.Prepend == nil || *c.ContentRules.Default.Prepend != "片头" {
		t.Errorf("prepend 覆盖不符: %v", c.ContentRules.Default.Prepend)
	}
	if len(c.ContentRules.Default.Replace) != 1 || c.ContentRules.Default.Replace[0].Pattern != "a" {
		t.Errorf("replace 应保留 config 值: %+v", c.ContentRules.Default.Replace)
	}
	book, ok := c.ContentRules.Books["7526452300462705689"]
	if !ok || book.Override == nil || *book.Override != "整章接管" {
		t.Errorf("books 解析不符: %+v", c.ContentRules.Books)
	}
}

func TestEnvOverridesBoth(t *testing.T) {
	configYaml := `
server:
  addr: ":1000"
app:
  mode: release
log:
  level: info
storage:
  dsn: "./from-file.db"
`
	t.Setenv("APP_MODE", "debug")
	t.Setenv("APP_ADDR", ":9999")
	t.Setenv("APP_LOG_LEVEL", "error")
	t.Setenv("APP_STORAGE_DSN", "./from-env.db")
	c, err := loadFiles(t, configYaml, "")
	if err != nil {
		t.Fatal(err)
	}
	if c.App.Mode != "debug" || c.Server.Addr != ":9999" || c.Log.Level != "error" {
		t.Fatalf("env 覆盖未生效: %+v", c)
	}
	if c.Storage.DSN != "./from-env.db" {
		t.Fatalf("APP_STORAGE_DSN 覆盖未生效: %q", c.Storage.DSN)
	}
}

func TestEnvOnlyForFourKeys(t *testing.T) {
	// 只有四个 env 变量生效，其余键只读文件。
	t.Setenv("APP_LOG_FILE", "./other.log")
	t.Setenv("APP_STORAGE_DRIVER", "mongo")
	c, err := loadFiles(t, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Log.File != "./logs/app.log" {
		t.Fatalf("APP_LOG_FILE 不应生效: %q", c.Log.File)
	}
	if c.Storage.Driver != "" {
		t.Fatalf("APP_STORAGE_DRIVER 不应生效: %q", c.Storage.Driver)
	}
}

func TestUnknownKey(t *testing.T) {
	_, err := loadFiles(t, "foo: bar\n", "")
	if err == nil || !strings.Contains(err.Error(), "foo") {
		t.Fatalf("config.yaml 未知键应报错: %v", err)
	}
	_, err = loadFiles(t, "", "logg:\n  level: info\n")
	if err == nil || !strings.Contains(err.Error(), "logg") {
		t.Fatalf("instance.yaml 未知键应报错: %v", err)
	}
	// 嵌套层级的未知键同样报错。
	_, err = loadFiles(t, "rate_limit:\n  search:\n  - unknow: 1\n", "")
	if err == nil {
		t.Fatalf("嵌套未知键应报错: %v", err)
	}
}

func TestEnumErrors(t *testing.T) {
	cases := []string{
		"app:\n  mode: prod\n",
		"log:\n  level: trace\n",
		"storage:\n  driver: redis\n",
		"rate_limit:\n  search:\n    mode: foo\n",
		"rate_limit:\n  content:\n    mode: GLOBAL\n",
		"ip_filter:\n  mode: deny\n",
		"ip_filter:\n  scope: none\n",
	}
	for _, y := range cases {
		if _, err := loadFiles(t, y, ""); err == nil {
			t.Errorf("非法枚举应报错: %q", y)
		}
	}
	// env 侧的枚举同样报错。
	t.Setenv("APP_LOG_LEVEL", "verbose")
	if _, err := loadFiles(t, "", ""); err == nil {
		t.Fatal("env 非法 log.level 应报错")
	}
}

func TestMissingConfigFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(filepath.Join(dir, "nope.yaml"), filepath.Join(dir, "data", "instance.yaml")); err == nil {
		t.Fatal("config.yaml 缺失应报错")
	}
}

func TestMissingInstanceOK(t *testing.T) {
	// instance.yaml 不存在是常态（init 之前），不得报错。
	dir := t.TempDir()
	cp := writeFile(t, dir, "config.yaml", "")
	if _, err := Load(cp, filepath.Join(dir, "data", "instance.yaml")); err != nil {
		t.Fatalf("instance.yaml 缺失不应报错: %v", err)
	}
}
