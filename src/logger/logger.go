package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

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
	return initLoggerWithRedis(nil)
}

// InitLoggerWithRedisAndNodeID initializes logger with Redis error log writing capability and specific NodeID
func InitLoggerWithRedisAndNodeID(nodeID string, redisWriter func(entry RedisErrorLogEntry) error) *slog.Logger {
	return initLoggerWithRedisAndNodeID(nodeID, redisWriter)
}

func initLoggerWithRedis(redisWriter func(entry RedisErrorLogEntry) error) *slog.Logger {
	nodeID := detectLocalIP()
	return initLoggerWithRedisAndNodeID(nodeID, redisWriter)
}

func initLoggerWithRedisAndNodeID(nodeID string, redisWriter func(entry RedisErrorLogEntry) error) *slog.Logger {

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
				l = logger // Update global logger variable
				logger.Warn("Failed to create any log directory, logging to stderr", "local_dir_error", err.Error())
				return logger
			}
		}

		fileHandler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})

		var handler slog.Handler = fileHandler
		if redisWriter != nil {
			handler = NewRedisErrorLogHandler(fileHandler, "hub", nodeID, redisWriter)
		}

		base := slog.New(handler)
		logger := base.With("node_ip", nodeID)

		slog.SetDefault(logger)
		l = logger // Update global logger variable
		return logger
	}

	logFile := &lumberjack.Logger{
		Filename:   filepath.Join(getLogDir(), "hub.log"),
		MaxSize:    100,
		MaxBackups: 30,
		MaxAge:     15,
		Compress:   false, // Disable compression to allow error log reading
	}

	fileHandler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	var handler slog.Handler = fileHandler
	if redisWriter != nil {
		handler = NewRedisErrorLogHandler(fileHandler, "hub", nodeID, redisWriter)
	}

	base := slog.New(handler)
	logger := base.With("node_ip", nodeID)

	slog.SetDefault(logger)
	l = logger // Update global logger variable

	return logger
}

// InitPluginLogger initializes the plugin-specific logger for plugin failures
func InitPluginLogger() *slog.Logger {
	return initPluginLoggerWithRedis(nil)
}

// InitPluginLoggerWithRedisAndNodeID initializes plugin logger with Redis error log writing capability and specific NodeID
func InitPluginLoggerWithRedisAndNodeID(nodeID string, redisWriter func(entry RedisErrorLogEntry) error) *slog.Logger {
	return initPluginLoggerWithRedisAndNodeID(nodeID, redisWriter)
}

func initPluginLoggerWithRedis(redisWriter func(entry RedisErrorLogEntry) error) *slog.Logger {
	nodeID := detectLocalIP()
	return initPluginLoggerWithRedisAndNodeID(nodeID, redisWriter)
}

func initPluginLoggerWithRedisAndNodeID(nodeID string, redisWriter func(entry RedisErrorLogEntry) error) *slog.Logger {

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
				fileHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
					Level: slog.LevelInfo,
				})

				var handler slog.Handler = fileHandler
				if redisWriter != nil {
					handler = NewRedisErrorLogHandler(fileHandler, "plugin", nodeID, redisWriter)
				}

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

	fileHandler := slog.NewJSONHandler(pluginLogFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	var handler slog.Handler = fileHandler
	if redisWriter != nil {
		handler = NewRedisErrorLogHandler(fileHandler, "plugin", nodeID, redisWriter)
	}

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

func Debug(msg string, args ...any) {
	logWithCaller(l.Debug, msg, args...)
}

func Info(msg string, args ...any) {
	logWithCaller(l.Info, msg, args...)
}

func Warn(msg string, args ...any) {
	logWithCaller(l.Warn, msg, args...)
}

func Error(msg string, args ...any) {
	logWithCaller(l.Error, msg, args...)
}

// logWithCaller adds caller information to log entries
func logWithCaller(logFunc func(string, ...any), msg string, args ...any) {
	// Get caller information (skip 2 frames: logWithCaller + the wrapper function)
	if pc, file, line, ok := runtime.Caller(2); ok {
		// Get function name
		funcName := "unknown"
		if fn := runtime.FuncForPC(pc); fn != nil {
			funcName = fn.Name()
		}

		// Add source information in slog's expected format
		args = append(args, slog.Group("source",
			"function", funcName,
			"file", file,
			"line", line,
		))
	}
	logFunc(msg, args...)
}

type RedisErrorLogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Source    string                 `json:"source"` // "hub" or "plugin"
	NodeID    string                 `json:"node_id"`
	Function  string                 `json:"function,omitempty"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// RedisErrorLogHandler is a custom slog handler that writes to both file and Redis
type RedisErrorLogHandler struct {
	fileHandler slog.Handler
	source      string // "hub" or "plugin"
	nodeID      string
	redisWriter func(entry RedisErrorLogEntry) error
}

// NewRedisErrorLogHandler creates a new Redis error log handler
func NewRedisErrorLogHandler(fileHandler slog.Handler, source string, nodeID string, redisWriter func(entry RedisErrorLogEntry) error) *RedisErrorLogHandler {
	return &RedisErrorLogHandler{
		fileHandler: fileHandler,
		source:      source,
		nodeID:      nodeID,
		redisWriter: redisWriter,
	}
}

// Enabled implements slog.Handler
func (h *RedisErrorLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.fileHandler.Enabled(ctx, level)
}

// Handle implements slog.Handler
func (h *RedisErrorLogHandler) Handle(ctx context.Context, record slog.Record) error {
	// Always write to file first
	if err := h.fileHandler.Handle(ctx, record); err != nil {
		return err
	}

	// Only write ERROR and FATAL levels to Redis
	if record.Level >= slog.LevelError && h.redisWriter != nil {
		entry := RedisErrorLogEntry{
			Timestamp: record.Time,
			Level:     record.Level.String(),
			Message:   record.Message,
			Source:    h.source,
			NodeID:    h.nodeID,
			Details:   make(map[string]interface{}),
		}

		// Extract attributes
		record.Attrs(func(attr slog.Attr) bool {
			switch attr.Key {
			case "source":
				// Extract source information if available
				if attr.Value.Kind() == slog.KindGroup {
					attrs := attr.Value.Group()
					for _, groupAttr := range attrs {
						switch groupAttr.Key {
						case "function":
							entry.Function = groupAttr.Value.String()
						case "file":
							entry.File = groupAttr.Value.String()
						case "line":
							if line, ok := groupAttr.Value.Any().(int); ok {
								entry.Line = line
							}
						}
					}
				}
			case "error":
				entry.Error = attr.Value.String()
			default:
				entry.Details[attr.Key] = attr.Value.Any()
			}
			return true
		})

		// Write to Redis asynchronously to avoid blocking
		go func() {
			if err := h.redisWriter(entry); err != nil {
				// If Redis write fails, we don't want to crash the application
				// Just log to stderr as fallback
				fmt.Fprintf(os.Stderr, "Failed to write error log to Redis: %v\n", err)
			}
		}()
	}

	return nil
}

// WithAttrs implements slog.Handler
func (h *RedisErrorLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &RedisErrorLogHandler{
		fileHandler: h.fileHandler.WithAttrs(attrs),
		source:      h.source,
		nodeID:      h.nodeID,
		redisWriter: h.redisWriter,
	}
}

// WithGroup implements slog.Handler
func (h *RedisErrorLogHandler) WithGroup(name string) slog.Handler {
	return &RedisErrorLogHandler{
		fileHandler: h.fileHandler.WithGroup(name),
		source:      h.source,
		nodeID:      h.nodeID,
		redisWriter: h.redisWriter,
	}
}

func init() {
	if l == nil {
		l = InitLogger()
	}
}
