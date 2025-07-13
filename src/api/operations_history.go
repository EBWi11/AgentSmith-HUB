package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

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

// OperationHistoryFilter is an alias for common.OperationHistoryFilter for backward compatibility
type OperationHistoryFilter = common.OperationHistoryFilter

// OperationHistoryResponse represents the response
type OperationHistoryResponse struct {
	Operations []common.OperationRecord `json:"operations"`
	TotalCount int                      `json:"total_count"`
	HasMore    bool                     `json:"has_more"`
}

// ClusterOperationHistoryResponse represents aggregated operations from cluster
type ClusterOperationHistoryResponse struct {
	Operations []common.OperationRecord `json:"operations"`
	NodeStats  map[string]NodeOpStat    `json:"node_stats"`
	TotalCount int                      `json:"total_count"`
	HasMore    bool                     `json:"has_more"`
}

// NodeOpStat represents operation statistics for a node
type NodeOpStat struct {
	NodeID             string `json:"node_id"`
	ChangePushOps      int    `json:"change_push_ops"`
	LocalPushOps       int    `json:"local_push_ops"`
	ComponentDeleteOps int    `json:"component_delete_ops"`
	ProjectStartOps    int    `json:"project_start_ops"`
	ProjectStopOps     int    `json:"project_stop_ops"`
	ProjectRestartOps  int    `json:"project_restart_ops"`
	TotalOps           int    `json:"total_ops"`
	SuccessOps         int    `json:"success_ops"`
	FailedOps          int    `json:"failed_ops"`
}

// getUnifiedOperationHistory retrieves operations using the new efficient filtering function
func getUnifiedOperationHistory(filter common.OperationHistoryFilter) ([]common.OperationRecord, int, error) {
	// Use the new efficient filtering function from common package
	return common.GetOperationsFromRedisWithFilter(filter)
}

// calculateNodeStats calculates node statistics from operations
func calculateNodeStats(operations []common.OperationRecord) map[string]NodeOpStat {
	nodeStats := make(map[string]NodeOpStat)

	for _, op := range operations {
		nodeID := ""
		if op.Details != nil {
			if nodeIDValue, exists := op.Details["node_id"]; exists {
				if nodeIDStr, ok := nodeIDValue.(string); ok {
					nodeID = nodeIDStr
				}
			}
		}

		// Skip operations without node_id
		if nodeID == "" {
			continue
		}

		stat, exists := nodeStats[nodeID]
		if !exists {
			stat = NodeOpStat{NodeID: nodeID}
		}

		switch op.Type {
		case common.OpTypeChangePush:
			stat.ChangePushOps++
		case common.OpTypeLocalPush:
			stat.LocalPushOps++
		case common.OpTypeComponentDelete:
			stat.ComponentDeleteOps++
		case common.OpTypeProjectStart:
			stat.ProjectStartOps++
		case common.OpTypeProjectStop:
			stat.ProjectStopOps++
		case common.OpTypeProjectRestart:
			stat.ProjectRestartOps++
		}
		stat.TotalOps++
		if op.Status == "success" {
			stat.SuccessOps++
		} else {
			stat.FailedOps++
		}
		nodeStats[nodeID] = stat
	}

	return nodeStats
}

// RecordChangePush records a change push operation to Redis
func RecordChangePush(componentType, componentID, oldContent, newContent, diff, status, errorMsg string) {
	// Create details map with execution node information
	details := map[string]interface{}{
		"node_id":      common.Config.LocalIP,
		"node_address": common.Config.LocalIP,
		"executed_by":  common.Config.LocalIP,
	}

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
		Details:       details,
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
	// Create details map with execution node information
	details := map[string]interface{}{
		"node_id":      common.Config.LocalIP,
		"node_address": common.Config.LocalIP,
		"executed_by":  common.Config.LocalIP,
	}

	record := common.OperationRecord{
		Type:          common.OpTypeLocalPush,
		Timestamp:     time.Now(),
		ComponentType: componentType,
		ComponentID:   componentID,
		NewContent:    content,
		Status:        status,
		Error:         errorMsg,
		Details:       details,
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

// RecordComponentDelete records a component deletion operation to Redis
func RecordComponentDelete(componentType, componentID, status, errorMsg string, affectedProjects []string) {
	// Create details map with execution node information
	details := map[string]interface{}{
		"node_id":           common.Config.LocalIP,
		"node_address":      common.Config.LocalIP,
		"executed_by":       common.Config.LocalIP,
		"affected_projects": affectedProjects,
	}

	record := common.OperationRecord{
		Type:          common.OpTypeComponentDelete,
		Timestamp:     time.Now(),
		ComponentType: componentType,
		ComponentID:   componentID,
		Status:        status,
		Error:         errorMsg,
		Details:       details,
	}

	// Serialize record to JSON and store to Redis
	if jsonData, err := json.Marshal(record); err == nil {
		if err := common.RedisLPush("cluster:ops_history", string(jsonData), 10000); err != nil {
			logger.Error("Failed to record component delete operation to Redis", "error", err)
		} else {
			// Set TTL for the entire list to 31 days
			if err := common.RedisExpire("cluster:ops_history", 31*24*60*60); err != nil {
				logger.Warn("Failed to set TTL for operations history", "error", err)
			}
			logger.Info("Component delete operation recorded to Redis", "type", record.Type, "component", record.ComponentType, "id", record.ComponentID)
		}
	} else {
		logger.Error("Failed to marshal component delete operation", "error", err)
	}
}

// RecordProjectOperation records a project operation
func RecordProjectOperation(operationType OperationType, projectID, status, errorMsg string, details map[string]interface{}) {
	// Delegate to common package
	common.RecordProjectOperation(common.OperationType(operationType), projectID, status, errorMsg, details)
}

// API Handlers

// GetOperationsHistory handles GET /operations-history - unified endpoint for all nodes
func GetOperationsHistory(c echo.Context) error {
	var filter common.OperationHistoryFilter

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
	filter.NodeID = c.QueryParam("node_id")

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

	// Get operations using the new efficient filtering function
	operations, totalCount, err := getUnifiedOperationHistory(filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to retrieve operations history: %v", err),
		})
	}

	// Calculate hasMore correctly
	hasMore := filter.Offset+len(operations) < totalCount

	response := OperationHistoryResponse{
		Operations: operations,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}

	return c.JSON(http.StatusOK, response)
}

// GetClusterOperationsHistory handles GET /cluster-operations-history - DEPRECATED but kept for backward compatibility
func GetClusterOperationsHistory(c echo.Context) error {
	// Redirect to unified endpoint - no longer restricted to leader only
	logger.Warn("GetClusterOperationsHistory is deprecated - use GetOperationsHistory instead")
	return GetOperationsHistory(c)
}

// GetOperationsStats handles GET /operations-stats
func GetOperationsStats(c echo.Context) error {
	// Get operations for the last 30 days
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -30)

	filter := common.OperationHistoryFilter{
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     10000, // Get more for stats
	}

	operations, totalCount, err := getUnifiedOperationHistory(filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to retrieve operations stats: %v", err),
		})
	}

	// Calculate statistics
	stats := map[string]interface{}{
		"total_operations":  totalCount,
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

// GetOperationsHistoryNodes handles GET /operations-history/nodes - returns all nodes that have operations history
func GetOperationsHistoryNodes(c echo.Context) error {
	// Get all known nodes from Redis (tracked by leader heartbeat)
	nodes, err := common.GetKnownNodes()
	if err != nil {
		logger.Error("Failed to get known nodes", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve known nodes: " + err.Error(),
		})
	}

	response := map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	}

	return c.JSON(http.StatusOK, response)
}
