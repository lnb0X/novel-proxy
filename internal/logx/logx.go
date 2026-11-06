// Package logx 初始化全局日志：JSON 行，同时写 stdout 和滚动文件。
package logx

import (
	"io"
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

// New 按 config.Log 的参数构建 slog.Logger，JSON handler，
// 输出同时进 stdout 和 lumberjack 滚动文件。
// 返回的 io.Closer 关闭滚动文件，Windows 上退出前必须调用。
func New(level, file string, maxSizeMB, maxBackups, maxAgeDays int, compress bool) (*slog.Logger, io.Closer) {
	lj := &lumberjack.Logger{
		Filename:   file,
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
		Compress:   compress,
	}
	w := io.MultiWriter(os.Stdout, lj)
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: parseLevel(level)})), lj
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
