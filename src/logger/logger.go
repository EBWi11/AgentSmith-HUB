package logger

import (
	"context"
	"gopkg.in/natefinch/lumberjack.v2"
	"log/slog"
)

var l *slog.Logger

func InitLogger() *slog.Logger {
	logFile := &lumberjack.Logger{
		Filename:   "./logs/hub.log",
		MaxSize:    100,
		MaxBackups: 128,
		MaxAge:     30,
		Compress:   true,
	}

	handler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

func Debug(msg string, args ...any) {
	l.Debug(msg, args...)
}

func DebugContext(ctx context.Context, msg string, args ...any) {
	l.DebugContext(ctx, msg, args...)
}

func Info(msg string, args ...any) {
	l.Info(msg, args...)
}

func InfoContext(ctx context.Context, msg string, args ...any) {
	l.InfoContext(ctx, msg, args...)
}

func Warn(msg string, args ...any) {
	l.Warn(msg, args...)
}

func WarnContext(ctx context.Context, msg string, args ...any) {
	l.WarnContext(ctx, msg, args...)
}

func Error(msg string, args ...any) {
	l.Error(msg, args...)
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	l.ErrorContext(ctx, msg, args...)
}

func init() {
	if l == nil {
		l = InitLogger()
	}
}
