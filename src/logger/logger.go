package logger

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

var l *slog.Logger
var accessLogger *lumberjack.Logger

func InitLogger() *slog.Logger {
	logFile := &lumberjack.Logger{
		Filename:   "./logs/hub.log",
		MaxSize:    100,
		MaxBackups: 30,
		MaxAge:     15,
		Compress:   true,
	}

	handler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

// InitAccessLogger initializes the access logger for API requests
func InitAccessLogger() io.Writer {
	// Get current working directory for debugging
	pwd, _ := os.Getwd()
	if l != nil {
		l.Info("initializing access logger", "working_directory", pwd, "target_path", "./logs/access.log")
	}

	// Create logs directory if it doesn't exist
	if _, err := os.Stat("./logs"); os.IsNotExist(err) {
		if err := os.Mkdir("./logs", 0755); err != nil {
			// If failed to create logs directory, log the error and use stderr as fallback
			if l != nil {
				l.Error("failed to create logs directory", "error", err, "working_directory", pwd)
			}
			return os.Stderr
		}
		if l != nil {
			l.Info("created logs directory", "path", "./logs")
		}
	}

	accessLogger = &lumberjack.Logger{
		Filename:   "./logs/access.log",
		MaxSize:    50, // 50MB per file
		MaxBackups: 30, // Keep 30 backup files
		MaxAge:     15, // Keep files for 15 days
		Compress:   true,
	}

	// Log successful initialization
	if l != nil {
		l.Info("access logger initialized", "filename", "./logs/access.log", "working_directory", pwd)
	}

	return accessLogger
}

// GetAccessLogger returns the access logger instance
func GetAccessLogger() io.Writer {
	if accessLogger == nil {
		return InitAccessLogger()
	}
	return accessLogger
}

// TestAccessLogger writes a test message to verify access logger works
func TestAccessLogger() error {
	accessWriter := GetAccessLogger()
	if accessWriter == nil {
		return errors.New("access logger is nil")
	}

	testMsg := `{"time":"2025-01-21T09:00:00Z","message":"access_logger_test","status":"ok"}` + "\n"
	_, err := accessWriter.Write([]byte(testMsg))
	if err != nil {
		if l != nil {
			l.Error("failed to write test message to access log", "error", err)
		}
		return err
	}

	if l != nil {
		l.Info("access logger test successful")
	}
	return nil
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
