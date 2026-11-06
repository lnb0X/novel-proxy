package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaults(t *testing.T) {
	// 全部键省略（空文件）时应得到全部默认值。
	c, err := Load(writeFile(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	want := defaults()
	if c != want {
		t.Fatalf("默认值不符:\n got %+v\nwant %+v", c, want)
	}
}

func TestYamlOverride(t *testing.T) {
	yaml := `
app:
  mode: debug
server:
  addr: ":9090"
  shutdown_timeout_seconds: 3
log:
  level: error
  file: ./tmp/x.log
  max_size_mb: 1
  max_backups: 2
  max_age_days: 3
  compress: false
upstream:
  timeout_seconds: 7
  max_concurrency: 4
`
	c, err := Load(writeFile(t, yaml))
	if err != nil {
		t.Fatal(err)
	}
	if c.App.Mode != "debug" {
		t.Errorf("app.mode = %q", c.App.Mode)
	}
	if c.Server.Addr != ":9090" {
		t.Errorf("server.addr = %q", c.Server.Addr)
	}
	if c.Server.ShutdownTimeoutSeconds != 3 {
		t.Errorf("shutdown_timeout_seconds = %d", c.Server.ShutdownTimeoutSeconds)
	}
	if c.Log.Level != "error" {
		t.Errorf("log.level = %q", c.Log.Level)
	}
	if c.Log.File != "./tmp/x.log" {
		t.Errorf("log.file = %q", c.Log.File)
	}
	if c.Log.MaxSizeMB != 1 || c.Log.MaxBackups != 2 || c.Log.MaxAgeDays != 3 || c.Log.Compress {
		t.Errorf("log 滚动参数不符: %+v", c.Log)
	}
	if c.Upstream.TimeoutSeconds != 7 || c.Upstream.MaxConcurrency != 4 {
		t.Errorf("upstream 参数不符: %+v", c.Upstream)
	}
}

func TestFlagPathWinsOverEnv(t *testing.T) {
	envFile := writeFile(t, "server:\n  addr: \":1111\"\n")
	flagFile := writeFile(t, "server:\n  addr: \":2222\"\n")
	t.Setenv("APP_CONFIG", envFile)
	c, err := Load(flagFile)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Addr != ":2222" {
		t.Fatalf("-config 应优先于 APP_CONFIG，got addr %q", c.Server.Addr)
	}
}

func TestEnvConfigPathUsedWhenNoFlag(t *testing.T) {
	envFile := writeFile(t, "server:\n  addr: \":3333\"\n")
	t.Setenv("APP_CONFIG", envFile)
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Addr != ":3333" {
		t.Fatalf("无 -config 时应使用 APP_CONFIG，got addr %q", c.Server.Addr)
	}
}

func TestEnvOverride(t *testing.T) {
	// yaml 给出与默认不同的值，env 必须盖过 yaml。
	p := writeFile(t, `
app:
  mode: release
server:
  addr: ":8080"
log:
  level: info
`)
	t.Setenv("APP_MODE", "debug")
	t.Setenv("APP_ADDR", ":9999")
	t.Setenv("APP_LOG_LEVEL", "error")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.App.Mode != "debug" || c.Server.Addr != ":9999" || c.Log.Level != "error" {
		t.Fatalf("env 覆盖未生效: %+v", c)
	}
}

func TestEnvOnlyForThreeKeys(t *testing.T) {
	// 契约只允许 APP_MODE / APP_ADDR / APP_LOG_LEVEL 三个环境变量，其余只读 yaml。
	p := writeFile(t, "")
	t.Setenv("APP_LOG_FILE", "./other.log")
	t.Setenv("APP_SHUTDOWN_TIMEOUT_SECONDS", "99")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Log.File != "./logs/app.log" {
		t.Fatalf("APP_LOG_FILE 不应生效, got %q", c.Log.File)
	}
	if c.Server.ShutdownTimeoutSeconds != 10 {
		t.Fatalf("其余键不读环境变量, got %d", c.Server.ShutdownTimeoutSeconds)
	}
}

func TestInvalidLevel(t *testing.T) {
	if _, err := Load(writeFile(t, "log:\n  level: warn\n")); err == nil {
		t.Fatal("yaml 里非法 log.level 应报错")
	}
	p := writeFile(t, "")
	t.Setenv("APP_LOG_LEVEL", "trace")
	if _, err := Load(p); err == nil {
		t.Fatal("env 里非法 log.level 应报错")
	}
}

func TestInvalidMode(t *testing.T) {
	if _, err := Load(writeFile(t, "app:\n  mode: prod\n")); err == nil {
		t.Fatal("yaml 里非法 app.mode 应报错")
	}
	p := writeFile(t, "")
	t.Setenv("APP_MODE", "test")
	if _, err := Load(p); err == nil {
		t.Fatal("env 里非法 app.mode 应报错")
	}
}

func TestUnknownKey(t *testing.T) {
	_, err := Load(writeFile(t, "foo: bar\n"))
	if err == nil {
		t.Fatal("未知键应报错")
	}
	if !strings.Contains(err.Error(), "foo") {
		t.Fatalf("错误信息应指出未知键 foo, got %v", err)
	}
	// 嵌套层级的未知键同样报错。
	_, err = Load(writeFile(t, "log:\n  level: info\n  unknow: 1\n"))
	if err == nil {
		t.Fatal("嵌套未知键应报错")
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("配置文件不存在应报错，不能静默用默认值")
	}
}
