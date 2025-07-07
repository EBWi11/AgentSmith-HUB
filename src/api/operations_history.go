package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// OperationType represents the type of operation
type OperationType string

const (
	OpTypeChangePush     OperationType = "change_push"
	OpTypeLocalPush      OperationType = "local_push"
	OpTypeProjectStart   OperationType = "project_start"
	OpTypeProjectStop    OperationType = "project_stop"
	OpTypeProjectRestart OperationType = "project_restart"
)

// OperationRecord represents a single operation record
type OperationRecord struct {
	Type          OperationType          `json:"type"`
	Timestamp     time.Time              `json:"timestamp"`
	ComponentType string                 `json:"component_type,omitempty"`
	ComponentID   string                 `json:"component_id,omitempty"`
	ProjectID     string                 `json:"project_id,omitempty"`
	Diff          string                 `json:"diff,omitempty"`
	OldContent    string                 `json:"old_content,omitempty"`
	NewContent    string                 `json:"new_content,omitempty"`
	Status        string                 `json:"status"`
	Error         string                 `json:"error,omitempty"`
	UserIP        string                 `json:"user_ip,omitempty"`
	Details       map[string]interface{} `json:"details,omitempty"`
}

// OperationHistoryFilter represents filter parameters
type OperationHistoryFilter struct {
	StartTime     time.Time     `json:"start_time"`
	EndTime       time.Time     `json:"end_time"`
	OperationType OperationType `json:"operation_type"`
	ComponentType string        `json:"component_type"`
	ComponentID   string        `json:"component_id"`
	ProjectID     string        `json:"project_id"`
	Status        string        `json:"status"`
	Keyword       string        `json:"keyword"`
	Limit         int           `json:"limit"`
	Offset        int           `json:"offset"`
}

// OperationHistoryResponse represents the response
type OperationHistoryResponse struct {
	Operations []OperationRecord `json:"operations"`
	TotalCount int               `json:"total_count"`
	HasMore    bool              `json:"has_more"`
}

// getOperationsLogDir returns the operations history log directory
func getOperationsLogDir() string {
	if runtime.GOOS == "darwin" {
		return "/tmp/hub_logs/operations"
	}
	return "/var/log/hub_logs/operations"
}

// ensureOperationsLogDir creates the operations log directory if it doesn't exist
func ensureOperationsLogDir() error {
	logDir := getOperationsLogDir()
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			// Fallback to local directory
			localLogDir := "./logs/operations"
			if _, err := os.Stat(localLogDir); os.IsNotExist(err) {
				if err := os.MkdirAll(localLogDir, 0755); err != nil {
					return fmt.Errorf("failed to create any operations log directory: %w", err)
				}
			}
			return nil
		}
	}
	return nil
}

// getOperationsLogPath returns the full path for operations log file
func getOperationsLogPath(month string) string {
	logDir := getOperationsLogDir()

	// Try to ensure operations log directory exists
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		if err := os.MkdirAll(logDir, 0755); err == nil {
			return filepath.Join(logDir, fmt.Sprintf("operations_%s.log", month))
		}
		// Failed to create system directory, fall back to local
	} else if err == nil {
		return filepath.Join(logDir, fmt.Sprintf("operations_%s.log", month))
	}

	// Fallback to local directory
	localLogDir := "./logs/operations"
	if _, err := os.Stat(localLogDir); os.IsNotExist(err) {
		if err := os.MkdirAll(localLogDir, 0755); err != nil {
			logger.Error("Failed to create operations log directory", "system_dir", logDir, "local_dir", localLogDir, "error", err)
		}
	}

	return filepath.Join(localLogDir, fmt.Sprintf("operations_%s.log", month))
}

// recordOperation records an operation to the monthly log file
func recordOperation(record OperationRecord) error {
	// Get monthly log file path
	monthKey := record.Timestamp.Format("2006-01")
	logPath := getOperationsLogPath(monthKey)

	// Serialize record to JSON
	jsonData, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal operation record: %w", err)
	}

	// Append to log file
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open operations log file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(string(jsonData) + "\n")
	if err != nil {
		return fmt.Errorf("failed to write to operations log file: %w", err)
	}

	// Followers publish to Redis for leader aggregation; leader writes log file only
	if !cluster.IsLeader {
		_ = common.RedisLPush("cluster:ops_history", string(jsonData), 50000)
	}

	logger.Info("Operation recorded", "type", record.Type, "component", record.ComponentType, "id", record.ComponentID, "project", record.ProjectID)
	return nil
}

// readOperationsFromFile reads operations from a specific log file
func readOperationsFromFile(filePath string, filter OperationHistoryFilter) ([]OperationRecord, error) {
	var operations []OperationRecord

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return operations, nil // Return empty if file doesn't exist
		}
		return nil, fmt.Errorf("failed to open operations log file %s: %w", filePath, err)
	}
	defer file.Close()

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read operations log file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse the operation log line
		var operation OperationRecord
		if err := json.Unmarshal([]byte(line), &operation); err != nil {
			// Reduce log verbosity: only log critical parsing errors
			// logger.Debug("Failed to parse operation log line", "line", line, "error", err)
			continue
		}

		// Apply time filters
		if !filter.StartTime.IsZero() && operation.Timestamp.Before(filter.StartTime) {
			// Reduce log verbosity: only log if needed for debugging
			// logger.Debug("Record filtered out by start_time",
			// 	"operation_time", operation.Timestamp,
			// 	"start_time", filter.StartTime)
			continue
		}

		if !filter.EndTime.IsZero() && operation.Timestamp.After(filter.EndTime) {
			// Reduce log verbosity: only log if needed for debugging
			// logger.Debug("Record filtered out by end_time",
			// 	"operation_time", operation.Timestamp,
			// 	"end_time", filter.EndTime)
			continue
		}

		// Apply filters
		if !matchesOperationFilter(operation, filter) {
			continue
		}

		operations = append(operations, operation)
	}

	return operations, nil
}

// matchesOperationFilter checks if a record matches the filter criteria
func matchesOperationFilter(record OperationRecord, filter OperationHistoryFilter) bool {
	// Time range filter
	if !filter.StartTime.IsZero() && record.Timestamp.Before(filter.StartTime) {
		logger.Debug("Record filtered out by start_time",
			"record_time", record.Timestamp.Format(time.RFC3339),
			"filter_start", filter.StartTime.Format(time.RFC3339))
		return false
	}
	if !filter.EndTime.IsZero() && record.Timestamp.After(filter.EndTime) {
		logger.Debug("Record filtered out by end_time",
			"record_time", record.Timestamp.Format(time.RFC3339),
			"filter_end", filter.EndTime.Format(time.RFC3339))
		return false
	}

	// Operation type filter
	if filter.OperationType != "" && record.Type != filter.OperationType {
		return false
	}

	// Component type filter
	if filter.ComponentType != "" && record.ComponentType != filter.ComponentType {
		return false
	}

	// Component ID filter
	if filter.ComponentID != "" && record.ComponentID != filter.ComponentID {
		return false
	}

	// Project ID filter
	if filter.ProjectID != "" && record.ProjectID != filter.ProjectID {
		return false
	}

	// Status filter
	if filter.Status != "" && record.Status != filter.Status {
		return false
	}

	// Keyword filter
	if filter.Keyword != "" {
		keyword := strings.ToLower(filter.Keyword)
		if !strings.Contains(strings.ToLower(record.ComponentID), keyword) &&
			!strings.Contains(strings.ToLower(record.ProjectID), keyword) &&
			!strings.Contains(strings.ToLower(record.Error), keyword) &&
			!strings.Contains(strings.ToLower(record.Diff), keyword) {
			return false
		}
	}

	return true
}

// getOperationHistory retrieves operations based on filter criteria
func getOperationHistory(filter OperationHistoryFilter) ([]OperationRecord, error) {
	var allOperations []OperationRecord

	// Fetch logs cached in Redis list first (cluster-wide)
	redisLines, _ := common.RedisLRange("cluster:ops_history", 0, 49999)
	for _, line := range redisLines {
		var op OperationRecord
		if err := json.Unmarshal([]byte(line), &op); err == nil {
			if matchesOperationFilter(op, filter) {
				allOperations = append(allOperations, op)
			}
		}
	}

	// Then read local monthly files

	// Determine which log files to read based on time range
	var monthsToRead []string

	if filter.StartTime.IsZero() && filter.EndTime.IsZero() {
		// No time filter, read current month and previous month
		now := time.Now()
		monthsToRead = append(monthsToRead, now.Format("2006-01"))
		prevMonth := now.AddDate(0, -1, 0)
		monthsToRead = append(monthsToRead, prevMonth.Format("2006-01"))
	} else {
		// Generate list of months to read based on time range
		start := filter.StartTime
		if start.IsZero() {
			start = time.Now().AddDate(0, -3, 0) // Default to 3 months ago
		}

		end := filter.EndTime
		if end.IsZero() {
			end = time.Now()
		}

		current := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
		endMonth := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location())

		for current.Before(endMonth) || current.Equal(endMonth) {
			monthsToRead = append(monthsToRead, current.Format("2006-01"))
			current = current.AddDate(0, 1, 0)
		}
	}

	// Read operations from each month file
	for _, month := range monthsToRead {
		logPath := getOperationsLogPath(month)
		operations, err := readOperationsFromFile(logPath, filter)
		if err != nil {
			logger.Error("Failed to read operations from file", "path", logPath, "error", err)
			continue
		}
		allOperations = append(allOperations, operations...)
	}

	// Sort by timestamp (newest first)
	sort.Slice(allOperations, func(i, j int) bool {
		return allOperations[i].Timestamp.After(allOperations[j].Timestamp)
	})

	return allOperations, nil
}

// RecordChangePush records a change push operation
func RecordChangePush(componentType, componentID, oldContent, newContent, diff, status, errorMsg string) {
	record := OperationRecord{
		Type:          OpTypeChangePush,
		Timestamp:     time.Now(),
		ComponentType: componentType,
		ComponentID:   componentID,
		OldContent:    oldContent,
		NewContent:    newContent,
		Diff:          diff,
		Status:        status,
		Error:         errorMsg,
	}

	if err := recordOperation(record); err != nil {
		logger.Error("Failed to record change push operation", "error", err)
	}
}

// RecordLocalPush records a local push operation
func RecordLocalPush(componentType, componentID, content, status, errorMsg string) {
	record := OperationRecord{
		Type:          OpTypeLocalPush,
		Timestamp:     time.Now(),
		ComponentType: componentType,
		ComponentID:   componentID,
		NewContent:    content,
		Status:        status,
		Error:         errorMsg,
	}

	if err := recordOperation(record); err != nil {
		logger.Error("Failed to record local push operation", "error", err)
	}
}

// RecordProjectOperation records a project operation
func RecordProjectOperation(operationType OperationType, projectID, status, errorMsg string, details map[string]interface{}) {
	record := OperationRecord{
		Type:      operationType,
		Timestamp: time.Now(),
		ProjectID: projectID,
		Status:    status,
		Error:     errorMsg,
		Details:   details,
	}

	if err := recordOperation(record); err != nil {
		logger.Error("Failed to record project operation", "operation", operationType, "project", projectID, "error", err)
	}
}

// API Handlers

// GetOperationsHistory handles GET /operations-history
func GetOperationsHistory(c echo.Context) error {
	var filter OperationHistoryFilter

	// Parse query parameters
	if startTimeStr := c.QueryParam("start_time"); startTimeStr != "" {
		if startTime, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			filter.StartTime = startTime
			logger.Info("Parsed start_time", "input", startTimeStr, "parsed", startTime.Format(time.RFC3339))
		} else {
			logger.Error("Failed to parse start_time", "input", startTimeStr, "error", err)
		}
	}

	if endTimeStr := c.QueryParam("end_time"); endTimeStr != "" {
		if endTime, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			filter.EndTime = endTime
			logger.Info("Parsed end_time", "input", endTimeStr, "parsed", endTime.Format(time.RFC3339))
		} else {
			logger.Error("Failed to parse end_time", "input", endTimeStr, "error", err)
		}
	}

	filter.OperationType = OperationType(c.QueryParam("operation_type"))
	filter.ComponentType = c.QueryParam("component_type")
	filter.ComponentID = c.QueryParam("component_id")
	filter.ProjectID = c.QueryParam("project_id")
	filter.Status = c.QueryParam("status")
	filter.Keyword = c.QueryParam("keyword")

	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = limit
		}
	}
	if filter.Limit <= 0 {
		filter.Limit = 100 // Default limit
	}

	if offsetStr := c.QueryParam("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			filter.Offset = offset
		}
	}

	// Log filter parameters for debugging
	logger.Debug("Operations history filter",
		"start_time", filter.StartTime.Format(time.RFC3339),
		"end_time", filter.EndTime.Format(time.RFC3339),
		"operation_type", filter.OperationType,
		"component_type", filter.ComponentType,
		"status", filter.Status,
		"keyword", filter.Keyword,
		"limit", filter.Limit,
		"offset", filter.Offset)

	// Get operations
	operations, err := getOperationHistory(filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to retrieve operations history: %v", err),
		})
	}

	logger.Debug("Retrieved operations", "count", len(operations))

	totalCount := len(operations)

	// Apply pagination
	start := filter.Offset
	end := start + filter.Limit

	if start > totalCount {
		start = totalCount
	}
	if end > totalCount {
		end = totalCount
	}

	paginatedOperations := operations[start:end]
	hasMore := end < totalCount

	response := OperationHistoryResponse{
		Operations: paginatedOperations,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}

	return c.JSON(http.StatusOK, response)
}

// GetOperationsStats handles GET /operations-stats
func GetOperationsStats(c echo.Context) error {
	// Get operations for the last 30 days
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -30)

	filter := OperationHistoryFilter{
		StartTime: startTime,
		EndTime:   endTime,
	}

	operations, err := getOperationHistory(filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to retrieve operations stats: %v", err),
		})
	}

	// Calculate statistics
	stats := map[string]interface{}{
		"total_operations":  len(operations),
		"by_type":           map[string]int{},
		"by_status":         map[string]int{},
		"by_component_type": map[string]int{},
		"recent_operations": []OperationRecord{},
	}

	typeStats := make(map[string]int)
	statusStats := make(map[string]int)
	componentTypeStats := make(map[string]int)

	for _, op := range operations {
		typeStats[string(op.Type)]++
		statusStats[op.Status]++
		if op.ComponentType != "" {
			componentTypeStats[op.ComponentType]++
		}
	}

	stats["by_type"] = typeStats
	stats["by_status"] = statusStats
	stats["by_component_type"] = componentTypeStats

	// Get recent operations (last 10)
	recentCount := 10
	if len(operations) < recentCount {
		recentCount = len(operations)
	}
	stats["recent_operations"] = operations[:recentCount]

	return c.JSON(http.StatusOK, stats)
}
