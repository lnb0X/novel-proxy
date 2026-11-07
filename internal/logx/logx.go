// Package logx 初始化全局日志：JSON 行，同时写 stdout 和滚动文件。
// level 走 slog.LevelVar 支持运行期热更；滚动四项由 lumberjack 持有，
// 运行期不可变（DECISIONS：日志滚动参数需重启）。
package logx

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Log 持有 logger 与可热更的级别。
type Log struct {
	Logger *slog.Logger
	level  *slog.LevelVar
	writer *lumberjack.Logger
}

// New 按 config.Log 的参数构建日志：JSON handler，输出同时进 stdout
// 和 lumberjack 滚动文件。
func New(level, file string, maxSizeMB, maxBackups, maxAgeDays int, compress bool) *Log {
	lv := &slog.LevelVar{}
	lv.Set(parseLevel(level))
	writer := &lumberjack.Logger{
		Filename:   file,
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
		Compress:   compress,
	}
	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, writer), &slog.HandlerOptions{Level: lv}))
	return &Log{Logger: logger, level: lv, writer: writer}
}

// SetLevel 热更日志级别，立即生效。
func (l *Log) SetLevel(level string) error {
	switch level {
	case "debug", "info", "error":
	default:
		return fmt.Errorf("log.level 非法值 %q", level)
	}
	l.level.Set(parseLevel(level))
	return nil
}

// Level 返回当前级别（管理接口读取用）。
func (l *Log) Level() string {
	switch {
	case l.level == nil:
		return ""
	case l.level.Level() <= slog.LevelDebug:
		return "debug"
	case l.level.Level() <= slog.LevelInfo:
		return "info"
	default:
		return "error"
	}
}

// Close 关闭滚动文件，Windows 上退出前必须调用。
func (l *Log) Close() error {
	if l.writer == nil {
		return nil
	}
	return l.writer.Close()
}

func parseLevel(s string) slog.Level {
	switch strings.TrimSpace(s) {
	case "debug":
		return slog.LevelDebug
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
