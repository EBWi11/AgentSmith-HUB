package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// ErrorLogEntry represents a single error log entry
type ErrorLogEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Level       string    `json:"level"`
	Message     string    `json:"message"`
	Source      string    `json:"source"`       // "hub" or "plugin"
	NodeID      string    `json:"node_id"`      // cluster node identifier
	NodeAddress string    `json:"node_address"` // cluster node address
	Context     string    `json:"context"`      // additional context from log
	Line        int       `json:"line"`         // line number in log file
}

// ErrorLogFilter represents filter parameters for error logs
type ErrorLogFilter struct {
	Source    string    `json:"source"`     // "hub", "plugin", or "all"
	NodeID    string    `json:"node_id"`    // specific node or "all"
	StartTime time.Time `json:"start_time"` // start time filter
	EndTime   time.Time `json:"end_time"`   // end time filter
	Keyword   string    `json:"keyword"`    // keyword search
	Limit     int       `json:"limit"`      // limit number of results
	Offset    int       `json:"offset"`     // pagination offset
}

// ErrorLogResponse represents the response for error log queries
type ErrorLogResponse struct {
	Logs       []ErrorLogEntry `json:"logs"`
	TotalCount int             `json:"total_count"`
	HasMore    bool            `json:"has_more"`
}

// ClusterErrorLogResponse represents aggregated error logs from cluster
type ClusterErrorLogResponse struct {
	Logs       []ErrorLogEntry     `json:"logs"`
	NodeStats  map[string]NodeStat `json:"node_stats"`
	TotalCount int                 `json:"total_count"`
}

// NodeStat represents error statistics for a node
type NodeStat struct {
	NodeID       string `json:"node_id"`
	HubErrors    int    `json:"hub_errors"`
	PluginErrors int    `json:"plugin_errors"`
	TotalErrors  int    `json:"total_errors"`
}

var (
	// Regular expressions for parsing different log formats
	hubLogRegex    = regexp.MustCompile(`^{"time":"([^"]+)","level":"([^"]+)","msg":"([^"]+)"`)
	pluginLogRegex = regexp.MustCompile(`^{"time":"([^"]+)","level":"([^"]+)","msg":"([^"]+)"`)

	// Error level patterns
	errorLevels = []string{"ERROR", "error", "Error", "FATAL", "fatal", "Fatal"}
)

// getLogDir returns the appropriate log directory based on the operating system
// This mirrors the function in logger package to ensure consistency
func getLogDir() string {
	if runtime.GOOS == "darwin" {
		return "/tmp/hub_logs"
	}
	return "/var/log/hub_logs"
}

// getLogPath returns the full path for a specific log file
// It ensures the directory exists and creates it if necessary
func getLogPath(filename string) string {
	logDir := getLogDir()

	// Try to ensure system log directory exists
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		if err := os.MkdirAll(logDir, 0755); err == nil {
			// Successfully created system log directory
			return filepath.Join(logDir, filename)
		}
		// Failed to create system directory, fall back to local
	} else if err == nil {
		// System log directory exists
		return filepath.Join(logDir, filename)
	}

	// Fallback to local directory - ensure it exists
	localLogDir := "./logs"
	if _, err := os.Stat(localLogDir); os.IsNotExist(err) {
		if err := os.MkdirAll(localLogDir, 0755); err != nil {
			// If we can't create any directory, still return the path
			// The file operations will fail later with appropriate errors
			logger.Error("Failed to create any log directory", "system_dir", logDir, "local_dir", localLogDir, "error", err)
		}
	}

	return filepath.Join(localLogDir, filename)
}

// readErrorLogsFromFile reads error logs from a specific file
func readErrorLogsFromFile(filePath string, source string, filter ErrorLogFilter) ([]ErrorLogEntry, error) {
	var logs []ErrorLogEntry

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("Log file does not exist", "file", filePath)
			return logs, nil // Return empty logs instead of error for missing files
		}
		return nil, fmt.Errorf("failed to open log file %s: %w", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Increase the scanner buffer size to handle long log lines
	// Default buffer size is 64KB, we increase it to 5MB
	const maxCapacity = 1024 * 1024 * 5 // 5MB
	buf := make([]byte, 0, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		entry, err := parseLogLine(line, source, lineNum)
		if err != nil {
			// Skip unparseable lines but log the error
			// logger.Debug("Failed to parse log line", "line", lineNum, "content", line, "error", err)
			continue
		}

		// Check if this is an error level log
		if !isErrorLevel(entry.Level) {
			continue
		}

		// Apply filters
		if !matchesFilter(entry, filter) {
			continue
		}

		// Set node information
		entry.NodeID = common.Config.LocalIP
		entry.NodeAddress = common.Config.LocalIP

		logs = append(logs, entry)
	}

	if err := scanner.Err(); err != nil {
		// Check if it's a "token too long" error
		if strings.Contains(err.Error(), "token too long") {
			logger.Warn("Log file contains lines that are too long, some lines may be skipped",
				"file", filePath, "error", err)
			// Return what we have parsed so far instead of failing completely
			return logs, nil
		}
		return nil, fmt.Errorf("error reading log file %s: %w", filePath, err)
	}

	return logs, nil
}

// parseLogLine parses a single log line and returns an ErrorLogEntry
func parseLogLine(line string, source string, lineNum int) (ErrorLogEntry, error) {
	var entry ErrorLogEntry
	entry.Source = source
	entry.Line = lineNum

	// Try to parse as JSON first (structured logging)
	if strings.HasPrefix(line, "{") {
		var logData map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logData); err == nil {
			// Extract timestamp
			if timeStr, ok := logData["time"].(string); ok {
				if parsedTime, err := time.Parse(time.RFC3339, timeStr); err == nil {
					entry.Timestamp = parsedTime
				} else if parsedTime, err := time.Parse("2006-01-02T15:04:05.000Z", timeStr); err == nil {
					entry.Timestamp = parsedTime
				}
			}

			// Extract level
			if level, ok := logData["level"].(string); ok {
				entry.Level = strings.ToUpper(level)
			}

			// Extract message
			if msg, ok := logData["msg"].(string); ok {
				entry.Message = msg
			} else if message, ok := logData["message"].(string); ok {
				entry.Message = message
			}

			// Extract additional context
			context := make(map[string]interface{})
			for k, v := range logData {
				if k != "time" && k != "level" && k != "msg" && k != "message" {
					context[k] = v
				}
			}
			if len(context) > 0 {
				if contextBytes, err := json.Marshal(context); err == nil {
					entry.Context = string(contextBytes)
				}
			}

			return entry, nil
		}
	}

	// Fall back to regex parsing for non-JSON logs
	var regex *regexp.Regexp
	if source == "hub" {
		regex = hubLogRegex
	} else {
		regex = pluginLogRegex
	}

	matches := regex.FindStringSubmatch(line)
	if len(matches) >= 4 {
		// Parse timestamp
		if parsedTime, err := time.Parse(time.RFC3339, matches[1]); err == nil {
			entry.Timestamp = parsedTime
		} else if parsedTime, err := time.Parse("2006-01-02T15:04:05.000Z", matches[1]); err == nil {
			entry.Timestamp = parsedTime
		}

		entry.Level = strings.ToUpper(matches[2])
		entry.Message = matches[3]

		// Store the full line as context for regex-parsed logs
		entry.Context = line

		return entry, nil
	}

	// If regex doesn't match, try to extract basic info
	entry.Timestamp = time.Now() // Use current time as fallback
	entry.Level = "UNKNOWN"
	entry.Message = line
	entry.Context = line

	return entry, nil
}

// isErrorLevel checks if the log level indicates an error
func isErrorLevel(level string) bool {
	upperLevel := strings.ToUpper(level)
	for _, errorLevel := range errorLevels {
		if strings.ToUpper(errorLevel) == upperLevel {
			return true
		}
	}
	return false
}

// matchesFilter checks if a log entry matches the given filter
func matchesFilter(entry ErrorLogEntry, filter ErrorLogFilter) bool {
	// Source filter
	if filter.Source != "" && filter.Source != "all" && filter.Source != entry.Source {
		return false
	}

	// Time range filter
	if !filter.StartTime.IsZero() && entry.Timestamp.Before(filter.StartTime) {
		return false
	}
	if !filter.EndTime.IsZero() && entry.Timestamp.After(filter.EndTime) {
		return false
	}

	// Keyword filter
	if filter.Keyword != "" {
		keyword := strings.ToLower(filter.Keyword)
		if !strings.Contains(strings.ToLower(entry.Message), keyword) &&
			!strings.Contains(strings.ToLower(entry.Context), keyword) {
			return false
		}
	}

	return true
}

// getLocalErrorLogs reads error logs from local log files
func getLocalErrorLogs(filter ErrorLogFilter) ([]ErrorLogEntry, error) {
	var allLogs []ErrorLogEntry

	// Read hub.log
	if filter.Source == "" || filter.Source == "all" || filter.Source == "hub" {
		hubLogPath := getLogPath("hub.log")
		hubLogs, err := readErrorLogsFromFile(hubLogPath, "hub", filter)
		if err != nil {
			logger.Error("Failed to read hub logs", "error", err)
			// Continue processing instead of failing completely
		} else {
			allLogs = append(allLogs, hubLogs...)
		}
	}

	// Read plugin.log
	if filter.Source == "" || filter.Source == "all" || filter.Source == "plugin" {
		pluginLogPath := getLogPath("plugin.log")
		pluginLogs, err := readErrorLogsFromFile(pluginLogPath, "plugin", filter)
		if err != nil {
			logger.Error("Failed to read plugin logs", "error", err)
			// Continue processing instead of failing completely
		} else {
			allLogs = append(allLogs, pluginLogs...)
		}
	}

	// Sort logs by timestamp (newest first)
	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i].Timestamp.After(allLogs[j].Timestamp)
	})

	return allLogs, nil
}

// storeLocalLogsToRedis caches latest logs for leader aggregation
func storeLocalLogsToRedis(nodeID string, logs []ErrorLogEntry) {
	if len(logs) == 0 {
		return
	}
	data, err := json.Marshal(logs)
	if err != nil {
		return
	}
	// Keep for 31 days; refreshed on each upload
	_, _ = common.RedisSet("cluster:error_logs:"+nodeID, string(data), 31*24*60*60)
}

// StartErrorLogUploader starts periodic error log upload for follower nodes
func StartErrorLogUploader() {
	if cluster.IsLeader {
		return // Leader doesn't need to upload logs
	}

	go func() {
		ticker := time.NewTicker(60 * time.Second) // Upload every 60 seconds
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				uploadErrorLogsToRedis()
			}
		}
	}()

	logger.Info("Error log uploader started for follower node")
}

// uploadErrorLogsToRedis uploads recent error logs to Redis
func uploadErrorLogsToRedis() {
	filter := ErrorLogFilter{
		Source:    "all",
		StartTime: time.Now().Add(-24 * time.Hour), // Last 24 hours
		EndTime:   time.Now(),
		Limit:     1000,
	}

	logs, err := getLocalErrorLogs(filter)
	if err != nil {
		logger.Error("Failed to get local error logs for upload", "error", err)
		return
	}

	if len(logs) > 0 {
		storeLocalLogsToRedis(common.Config.LocalIP, logs)
		logger.Debug("Uploaded error logs to Redis", "count", len(logs))
	}
}

// API Handlers

// getErrorLogs handles GET /error-logs
func getErrorLogs(c echo.Context) error {
	var filter ErrorLogFilter

	// Parse query parameters
	filter.Source = c.QueryParam("source")
	filter.NodeID = c.QueryParam("node_id")
	filter.Keyword = c.QueryParam("keyword")

	// Parse time filters
	if startTime := c.QueryParam("start_time"); startTime != "" {
		if parsed, err := time.Parse(time.RFC3339, startTime); err == nil {
			filter.StartTime = parsed
		}
	}
	if endTime := c.QueryParam("end_time"); endTime != "" {
		if parsed, err := time.Parse(time.RFC3339, endTime); err == nil {
			filter.EndTime = parsed
		}
	}

	// Parse pagination
	if limit := c.QueryParam("limit"); limit != "" {
		if parsed, err := strconv.Atoi(limit); err == nil && parsed > 0 {
			filter.Limit = parsed
		} else {
			filter.Limit = 100 // Default limit
		}
	} else {
		filter.Limit = 100
	}

	if offset := c.QueryParam("offset"); offset != "" {
		if parsed, err := strconv.Atoi(offset); err == nil && parsed >= 0 {
			filter.Offset = parsed
		}
	}

	// Get local logs
	logs, err := getLocalErrorLogs(filter)
	if err != nil {
		logger.Error("Failed to get local error logs", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to read error logs: " + err.Error(),
		})
	}

	// Cache to Redis for leader aggregation
	storeLocalLogsToRedis(common.Config.LocalIP, logs)

	// Apply pagination
	totalCount := len(logs)
	start := filter.Offset
	end := start + filter.Limit

	if start >= totalCount {
		logs = []ErrorLogEntry{}
	} else {
		if end > totalCount {
			end = totalCount
		}
		logs = logs[start:end]
	}

	response := ErrorLogResponse{
		Logs:       logs,
		TotalCount: totalCount,
		HasMore:    end < totalCount,
	}

	return c.JSON(http.StatusOK, response)
}

// getClusterErrorLogs handles GET /cluster-error-logs (leader only)
func getClusterErrorLogs(c echo.Context) error {
	// Only allow on leader node
	if !cluster.IsLeader {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "This endpoint is only available on the leader node",
		})
	}

	var filter ErrorLogFilter

	// Parse query parameters (same as getErrorLogs)
	filter.Source = c.QueryParam("source")
	filter.NodeID = c.QueryParam("node_id")
	filter.Keyword = c.QueryParam("keyword")

	if startTime := c.QueryParam("start_time"); startTime != "" {
		if parsed, err := time.Parse(time.RFC3339, startTime); err == nil {
			filter.StartTime = parsed
		}
	}
	if endTime := c.QueryParam("end_time"); endTime != "" {
		if parsed, err := time.Parse(time.RFC3339, endTime); err == nil {
			filter.EndTime = parsed
		}
	}

	if limit := c.QueryParam("limit"); limit != "" {
		if parsed, err := strconv.Atoi(limit); err == nil && parsed > 0 {
			filter.Limit = parsed
		} else {
			filter.Limit = 100
		}
	} else {
		filter.Limit = 100
	}

	if offset := c.QueryParam("offset"); offset != "" {
		if parsed, err := strconv.Atoi(offset); err == nil && parsed >= 0 {
			filter.Offset = parsed
		}
	}

	// Collect logs from all cluster nodes
	allLogs, nodeStats, totalCount, err := aggregateClusterErrorLogs(filter)
	if err != nil {
		logger.Error("Failed to aggregate cluster error logs", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to aggregate cluster error logs: " + err.Error(),
		})
	}

	response := ClusterErrorLogResponse{
		Logs:       allLogs,
		NodeStats:  nodeStats,
		TotalCount: totalCount,
	}

	return c.JSON(http.StatusOK, response)
}

// aggregateClusterErrorLogs collects error logs from all cluster nodes
func aggregateClusterErrorLogs(filter ErrorLogFilter) ([]ErrorLogEntry, map[string]NodeStat, int, error) {
	var allLogs []ErrorLogEntry
	nodeStats := make(map[string]NodeStat)
	totalCount := 0

	// Get leader's own logs first
	leaderLogs, err := getLocalErrorLogs(filter)
	if err != nil {
		logger.Error("Failed to get leader error logs", "error", err)
	} else {
		allLogs = append(allLogs, leaderLogs...)
		totalCount += len(leaderLogs)

		// Calculate leader stats
		leaderStat := NodeStat{
			NodeID: common.Config.LocalIP,
		}
		for _, log := range leaderLogs {
			if log.Source == "hub" {
				leaderStat.HubErrors++
			} else if log.Source == "plugin" {
				leaderStat.PluginErrors++
			}
			leaderStat.TotalErrors++
		}
		nodeStats[common.Config.LocalIP] = leaderStat
	}

	// Get all logs from Redis cache (from all nodes)
	pattern := "cluster:error_logs:*"
	keys, err := common.RedisKeys(pattern)
	if err != nil {
		logger.Error("Failed to get error log keys from Redis", "error", err)
	} else {
		for _, key := range keys {
			// Extract node ID from key
			nodeID := strings.TrimPrefix(key, "cluster:error_logs:")

			// Skip leader's own logs (already processed above)
			if nodeID == common.Config.LocalIP {
				continue
			}

			// Get logs from Redis cache
			var cachedLogs []ErrorLogEntry
			if cached, err := common.RedisGet(key); err == nil && cached != "" {
				if err := json.Unmarshal([]byte(cached), &cachedLogs); err != nil {
					logger.Error("Failed to unmarshal error logs from Redis", "node", nodeID, "error", err)
					continue
				}
			} else {
				continue
			}

			// Process each log and apply filters
			for i := range cachedLogs {
				cachedLogs[i].NodeID = nodeID
				cachedLogs[i].NodeAddress = nodeID // Use nodeID as address for now

				// Apply filters
				if matchesFilter(cachedLogs[i], filter) {
					allLogs = append(allLogs, cachedLogs[i])
				}
			}
		}
	}

	// Calculate node statistics from all collected logs
	for _, log := range allLogs {
		if log.NodeID == "" {
			continue
		}

		stat, exists := nodeStats[log.NodeID]
		if !exists {
			stat = NodeStat{NodeID: log.NodeID}
		}

		if log.Source == "hub" {
			stat.HubErrors++
		} else if log.Source == "plugin" {
			stat.PluginErrors++
		}
		stat.TotalErrors++
		nodeStats[log.NodeID] = stat
	}

	// Update total count
	totalCount = len(allLogs)

	// Sort all logs by timestamp (newest first)
	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i].Timestamp.After(allLogs[j].Timestamp)
	})

	// Apply node filter if specified
	if filter.NodeID != "" && filter.NodeID != "all" {
		var filteredLogs []ErrorLogEntry
		for _, log := range allLogs {
			if log.NodeID == filter.NodeID {
				filteredLogs = append(filteredLogs, log)
			}
		}
		allLogs = filteredLogs
		// Update total count after filtering
		totalCount = len(allLogs)
	}

	// Apply pagination
	start := filter.Offset
	end := start + filter.Limit
	if start >= len(allLogs) {
		allLogs = []ErrorLogEntry{}
	} else {
		if end > len(allLogs) {
			end = len(allLogs)
		}
		allLogs = allLogs[start:end]
	}

	return allLogs, nodeStats, totalCount, nil
}
