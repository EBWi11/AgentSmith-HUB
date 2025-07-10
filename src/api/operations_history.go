package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// OperationType represents the type of operation
// Type aliases for backward compatibility
type OperationType = common.OperationType

const (
	OpTypeChangePush     = common.OpTypeChangePush
	OpTypeLocalPush      = common.OpTypeLocalPush
	OpTypeProjectStart   = common.OpTypeProjectStart
	OpTypeProjectStop    = common.OpTypeProjectStop
	OpTypeProjectRestart = common.OpTypeProjectRestart
)

// OperationRecord is an alias for common.OperationRecord for backward compatibility
type OperationRecord = common.OperationRecord

// OperationHistoryFilter represents filter parameters
type OperationHistoryFilter struct {
	StartTime     time.Time            `json:"start_time"`
	EndTime       time.Time            `json:"end_time"`
	OperationType common.OperationType `json:"operation_type"`
	ComponentType string               `json:"component_type"`
	ComponentID   string               `json:"component_id"`
	ProjectID     string               `json:"project_id"`
	Status        string               `json:"status"`
	Keyword       string               `json:"keyword"`
	Limit         int                  `json:"limit"`
	Offset        int                  `json:"offset"`
}

// OperationHistoryResponse represents the response
type OperationHistoryResponse struct {
	Operations []common.OperationRecord `json:"operations"`
	TotalCount int                      `json:"total_count"`
	HasMore    bool                     `json:"has_more"`
}

// matchesOperationFilter checks if a record matches the filter criteria
func matchesOperationFilter(record common.OperationRecord, filter OperationHistoryFilter) bool {
	// Time range filter
	if !filter.StartTime.IsZero() && record.Timestamp.Before(filter.StartTime) {
		return false
	}
	if !filter.EndTime.IsZero() && record.Timestamp.After(filter.EndTime) {
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

// getOperationHistory retrieves operations from Redis only
func getOperationHistory(filter OperationHistoryFilter) ([]common.OperationRecord, error) {
	var allOperations []common.OperationRecord

	// Read from Redis (cluster-wide operations history)
	redisLines, err := common.RedisLRange("cluster:ops_history", 0, 99999)
	if err != nil {
		logger.Error("Failed to read operations from Redis", "error", err)
		return allOperations, nil // Return empty list instead of error
	}

	for _, line := range redisLines {
		var op common.OperationRecord
		if err := json.Unmarshal([]byte(line), &op); err == nil {
			if matchesOperationFilter(op, filter) {
				allOperations = append(allOperations, op)
			}
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(allOperations, func(i, j int) bool {
		return allOperations[i].Timestamp.After(allOperations[j].Timestamp)
	})

	return allOperations, nil
}

// RecordChangePush records a change push operation to Redis
func RecordChangePush(componentType, componentID, oldContent, newContent, diff, status, errorMsg string) {
	record := common.OperationRecord{
		Type:          common.OpTypeChangePush,
		Timestamp:     time.Now(),
		ComponentType: componentType,
		ComponentID:   componentID,
		OldContent:    oldContent,
		NewContent:    newContent,
		Diff:          diff,
		Status:        status,
		Error:         errorMsg,
	}

	// Serialize record to JSON and store to Redis
	if jsonData, err := json.Marshal(record); err == nil {
		if err := common.RedisLPush("cluster:ops_history", string(jsonData), 10000); err != nil {
			logger.Error("Failed to record change push operation to Redis", "error", err)
		} else {
			// Set TTL for the entire list to 31 days
			if err := common.RedisExpire("cluster:ops_history", 31*24*60*60); err != nil {
				logger.Warn("Failed to set TTL for operations history", "error", err)
			}
			logger.Info("Change push operation recorded to Redis", "type", record.Type, "component", record.ComponentType, "id", record.ComponentID)
		}
	} else {
		logger.Error("Failed to marshal change push operation", "error", err)
	}
}

// RecordLocalPush records a local push operation to Redis
func RecordLocalPush(componentType, componentID, content, status, errorMsg string) {
	record := common.OperationRecord{
		Type:          common.OpTypeLocalPush,
		Timestamp:     time.Now(),
		ComponentType: componentType,
		ComponentID:   componentID,
		NewContent:    content,
		Status:        status,
		Error:         errorMsg,
	}

	// Serialize record to JSON and store to Redis
	if jsonData, err := json.Marshal(record); err == nil {
		if err := common.RedisLPush("cluster:ops_history", string(jsonData), 10000); err != nil {
			logger.Error("Failed to record local push operation to Redis", "error", err)
		} else {
			// Set TTL for the entire list to 31 days
			if err := common.RedisExpire("cluster:ops_history", 31*24*60*60); err != nil {
				logger.Warn("Failed to set TTL for operations history", "error", err)
			}
			logger.Info("Local push operation recorded to Redis", "type", record.Type, "component", record.ComponentType, "id", record.ComponentID)
		}
	} else {
		logger.Error("Failed to marshal local push operation", "error", err)
	}
}

// RecordProjectOperation records a project operation
func RecordProjectOperation(operationType OperationType, projectID, status, errorMsg string, details map[string]interface{}) {
	// Delegate to common package
	common.RecordProjectOperation(common.OperationType(operationType), projectID, status, errorMsg, details)
}

// API Handlers

// GetOperationsHistory handles GET /operations-history
func GetOperationsHistory(c echo.Context) error {
	var filter OperationHistoryFilter

	// Parse query parameters
	if startTimeStr := c.QueryParam("start_time"); startTimeStr != "" {
		if startTime, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			filter.StartTime = startTime
		} else {
			logger.Error("Failed to parse start_time", "input", startTimeStr, "error", err)
		}
	}

	if endTimeStr := c.QueryParam("end_time"); endTimeStr != "" {
		if endTime, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			filter.EndTime = endTime
		} else {
			logger.Error("Failed to parse end_time", "input", endTimeStr, "error", err)
		}
	}

	filter.OperationType = common.OperationType(c.QueryParam("operation_type"))
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

	// Get operations from Redis
	operations, err := getOperationHistory(filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to retrieve operations history: %v", err),
		})
	}

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
		"recent_operations": []common.OperationRecord{},
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
