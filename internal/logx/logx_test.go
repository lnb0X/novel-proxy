package logx

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout 替换 os.Stdout 为管道，返回读写两端；
// 调用方写完后须 closeWriter 关写端（触发读端 EOF），结束时 restore 还原。
func captureStdout(t *testing.T) (r, w *os.File, restore func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	return r, w, func() {
		os.Stdout = old
	}
}

func readAll(t *testing.T, r *os.File) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBothSinksGetSameLine(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	r, w, restore := captureStdout(t)
	l := New("info", logFile, 50, 7, 14, false)
	defer l.Close()
	l.Logger.Info("novel-proxy 启动", "mode", "release", "addr", ":8080")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	restore()
	stdout := readAll(t, r)

	fileB, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(fileB) != stdout {
		t.Fatalf("stdout 与文件内容不一致:\nstdout=%q\nfile=%q", stdout, string(fileB))
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Fatalf("日志行应以换行结尾: %q", stdout)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &rec); err != nil {
		t.Fatalf("日志行不是合法 JSON: %v", err)
	}
	if rec["msg"] != "novel-proxy 启动" {
		t.Fatalf("msg 不符: %v", rec["msg"])
	}
	if rec["level"] != "INFO" {
		t.Fatalf("level 不符: %v", rec["level"])
	}
	if _, ok := rec["time"]; !ok {
		t.Fatal("缺少 time 字段")
	}
}

func TestLevelFilter(t *testing.T) {
	dir := t.TempDir()

	logFile := filepath.Join(dir, "info.log")
	l := New("info", logFile, 50, 7, 14, false)
	l.Logger.Debug("debug 行", "k", "v")
	l.Logger.Info("info 行")
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "debug 行") {
		t.Fatal("info 级别不应输出 debug 日志")
	}
	if !strings.Contains(s, "info 行") {
		t.Fatal("info 日志缺失")
	}

	dbgFile := filepath.Join(dir, "debug.log")
	l2 := New("debug", dbgFile, 50, 7, 14, false)
	l2.Logger.Debug("debug 行", "k", "v")
	if err := l2.Close(); err != nil {
		t.Fatal(err)
	}
	b2, err := os.ReadFile(dbgFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b2), "debug 行") {
		t.Fatal("debug 级别下 debug 日志应出现")
	}

	errFile := filepath.Join(dir, "error.log")
	l3 := New("error", errFile, 50, 7, 14, false)
	l3.Logger.Info("info 行")
	l3.Logger.Error("error 行")
	if err := l3.Close(); err != nil {
		t.Fatal(err)
	}
	b3, err := os.ReadFile(errFile)
	if err != nil {
		t.Fatal(err)
	}
	s3 := string(b3)
	if strings.Contains(s3, "info 行") || !strings.Contains(s3, "error 行") {
		t.Fatalf("error 级别过滤不符: %q", s3)
	}
}

func TestRotation(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	// max_size_mb=1，写入约 1.1MB 应触发滚动。
	l := New("info", logFile, 1, 2, 7, false)
	pad := strings.Repeat("x", 1024)
	for i := 0; i < 1100; i++ {
		l.Logger.Info("fill", "pad", pad)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var backups int
	var mainSize int64
	for _, e := range entries {
		if e.Name() == "app.log" {
			if info, err := e.Info(); err == nil {
				mainSize = info.Size()
			}
			continue
		}
		backups++
	}
	if backups == 0 {
		t.Fatal("超过 max_size_mb 后应产生轮转备份文件")
	}
	if mainSize > 1024*1024 {
		t.Fatalf("主日志文件 %d 字节，超过 1MB 未滚动", mainSize)
	}
}

func TestSetLevelHot(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "hot.log")
	l := New("info", logFile, 50, 7, 14, false)
	defer l.Close()

	l.Logger.Debug("不应出现")
	if err := l.SetLevel("warn"); err == nil {
		t.Fatal("非法级别应报错")
	}
	if err := l.SetLevel("debug"); err != nil {
		t.Fatal(err)
	}
	if l.Level() != "debug" {
		t.Fatalf("Level = %q", l.Level())
	}
	l.Logger.Debug("热更后应出现")
	b, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "不应出现") || !strings.Contains(string(b), "热更后应出现") {
		t.Fatalf("级别热更不符: %s", b)
	}
}
