package logger

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/natefinch/lumberjack.v2"
)

var l *slog.Logger
var accessLogger *lumberjack.Logger
var pluginLogger *slog.Logger

// getLogDir returns the appropriate log directory based on the operating system
func getLogDir() string {
	if runtime.GOOS == "darwin" {
		return "/tmp/hub_logs"
	}
	return "/var/log/hub_logs"
}

// ensureLogDir creates the log directory if it doesn't exist
func ensureLogDir() error {
	logDir := getLogDir()
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// detectLocalIP returns first non-loopback IPv4 address or "unknown"
func detectLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "unknown"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not ipv4
			}
			return ip.String()
		}
	}
	return "unknown"
}

func InitLogger() *slog.Logger {
	// Ensure log directory exists
	if err := ensureLogDir(); err != nil {
		// Fallback to current directory if unable to create system log directory
		logFile := &lumberjack.Logger{
			Filename:   "./logs/hub.log",
			MaxSize:    100,
			MaxBackups: 30,
			MaxAge:     15,
			Compress:   false,
		}

		// Create local logs directory if it doesn't exist
		if _, err := os.Stat("./logs"); os.IsNotExist(err) {
			if err := os.MkdirAll("./logs", 0755); err != nil {
				// If we can't create any log directory, write to stderr
				handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
					Level: slog.LevelInfo,
				})
				logger := slog.New(handler)
				slog.SetDefault(logger)
				logger.Warn("Failed to create any log directory, logging to stderr", "local_dir_error", err.Error())
				return logger
			}
		}

		handler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: true,
		})

		base := slog.New(handler)

		logger := base.With("node_ip", detectLocalIP())

		slog.SetDefault(logger)
		return logger
	}

	logFile := &lumberjack.Logger{
		Filename:   filepath.Join(getLogDir(), "hub.log"),
		MaxSize:    100,
		MaxBackups: 30,
		MaxAge:     15,
		Compress:   false, // Disable compression to allow error log reading
	}

	handler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	})

	base := slog.New(handler)

	logger := base.With("node_ip", detectLocalIP())

	slog.SetDefault(logger)

	return logger
}

// InitPluginLogger initializes the plugin-specific logger for plugin failures
func InitPluginLogger() *slog.Logger {
	// Get current working directory for debugging
	pwd, _ := os.Getwd()
	logDir := getLogDir()
	pluginLogPath := filepath.Join(logDir, "plugin.log")

	if l != nil {
		l.Info("initializing plugin logger", "working_directory", pwd, "target_path", pluginLogPath)
	}

	// Ensure log directory exists
	if err := ensureLogDir(); err != nil {
		if l != nil {
			l.Error("failed to create log directory for plugin logger", "error", err, "working_directory", pwd, "log_dir", logDir)
		}
		// Fallback to local directory
		if _, err := os.Stat("./logs"); os.IsNotExist(err) {
			if err := os.MkdirAll("./logs", 0755); err != nil {
				// Return a logger that writes to stderr as fallback
				handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
					Level: slog.LevelInfo,
				})
				return slog.New(handler)
			}
		}
		pluginLogPath = "./logs/plugin.log"
	}

	pluginLogFile := &lumberjack.Logger{
		Filename:   pluginLogPath,
		MaxSize:    100,   // Same as hub.log
		MaxBackups: 30,    // Same as hub.log
		MaxAge:     15,    // Same as hub.log
		Compress:   false, // Disable compression to allow error log reading
	}

	handler := slog.NewJSONHandler(pluginLogFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := slog.New(handler)

	// Log successful initialization
	if l != nil {
		l.Info("plugin logger initialized", "filename", pluginLogPath, "working_directory", pwd)
	}

	return logger
}

// GetPluginLogger returns the plugin logger instance
func GetPluginLogger() *slog.Logger {
	if pluginLogger == nil {
		pluginLogger = InitPluginLogger()
	}
	return pluginLogger
}

// InitAccessLogger initializes the access logger for API requests
func InitAccessLogger() io.Writer {
	// Get current working directory for debugging
	pwd, _ := os.Getwd()
	logDir := getLogDir()
	accessLogPath := filepath.Join(logDir, "access.log")

	if l != nil {
		l.Info("initializing access logger", "working_directory", pwd, "target_path", accessLogPath)
	}

	// Ensure log directory exists
	if err := ensureLogDir(); err != nil {
		if l != nil {
			l.Error("failed to create log directory", "error", err, "working_directory", pwd, "log_dir", logDir)
		}
		// Fallback to local directory
		if _, err := os.Stat("./logs"); os.IsNotExist(err) {
			if err := os.MkdirAll("./logs", 0755); err != nil {
				if l != nil {
					l.Error("failed to create local logs directory", "error", err, "working_directory", pwd)
				}
				return os.Stderr
			}
		}
		accessLogPath = "./logs/access.log"
	}

	accessLogger = &lumberjack.Logger{
		Filename:   accessLogPath,
		MaxSize:    50, // 50MB per file
		MaxBackups: 30, // Keep 30 backup files
		MaxAge:     15, // Keep files for 15 days
		Compress:   true,
	}

	// Log successful initialization
	if l != nil {
		l.Info("access logger initialized", "filename", accessLogPath, "working_directory", pwd)
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

// Plugin-specific logging functions
func PluginError(msg string, args ...any) {
	pluginLog := GetPluginLogger()
	pluginLog.Error(msg, args...)
}

func PluginWarn(msg string, args ...any) {
	pluginLog := GetPluginLogger()
	pluginLog.Warn(msg, args...)
}

func PluginInfo(msg string, args ...any) {
	pluginLog := GetPluginLogger()
	pluginLog.Info(msg, args...)
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
