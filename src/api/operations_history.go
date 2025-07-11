package api

import (
	"AgentSmith-HUB/cluster"
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

// ClusterOperationHistoryResponse represents aggregated operations from cluster
type ClusterOperationHistoryResponse struct {
	Operations []common.OperationRecord `json:"operations"`
	NodeStats  map[string]NodeOpStat    `json:"node_stats"`
	TotalCount int                      `json:"total_count"`
	HasMore    bool                     `json:"has_more"`
}

// NodeOpStat represents operation statistics for a node
type NodeOpStat struct {
	NodeID            string `json:"node_id"`
	ChangePushOps     int    `json:"change_push_ops"`
	LocalPushOps      int    `json:"local_push_ops"`
	ProjectStartOps   int    `json:"project_start_ops"`
	ProjectStopOps    int    `json:"project_stop_ops"`
	ProjectRestartOps int    `json:"project_restart_ops"`
	TotalOps          int    `json:"total_ops"`
	SuccessOps        int    `json:"success_ops"`
	FailedOps         int    `json:"failed_ops"`
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

		// Check node ID from details
		nodeID := ""
		if record.Details != nil {
			if nodeIDValue, exists := record.Details["node_id"]; exists {
				if nodeIDStr, ok := nodeIDValue.(string); ok {
					nodeID = nodeIDStr
				}
			}
		}

		if !strings.Contains(strings.ToLower(record.ComponentID), keyword) &&
			!strings.Contains(strings.ToLower(record.ProjectID), keyword) &&
			!strings.Contains(strings.ToLower(record.Error), keyword) &&
			!strings.Contains(strings.ToLower(record.Diff), keyword) &&
			!strings.Contains(strings.ToLower(nodeID), keyword) {
			return false
		}
	}

	return true
}

// getOperationHistory retrieves operations from Redis only (local node)
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
				// Set node information for local operations
				if op.Details == nil {
					op.Details = make(map[string]interface{})
				}
				op.Details["node_id"] = common.Config.LocalIP
				if cluster.ClusterInstance != nil && cluster.ClusterInstance.SelfAddress != "" {
					op.Details["node_address"] = cluster.ClusterInstance.SelfAddress
				} else {
					op.Details["node_address"] = common.Config.LocalIP
				}
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

// getFollowerOperationHistory fetches operation history from a follower node via HTTP
func getFollowerOperationHistory(nodeAddress string, filter OperationHistoryFilter) ([]common.OperationRecord, error) {
	// Build query parameters
	params := make(map[string]string)
	if !filter.StartTime.IsZero() {
		params["start_time"] = filter.StartTime.Format(time.RFC3339)
	}
	if !filter.EndTime.IsZero() {
		params["end_time"] = filter.EndTime.Format(time.RFC3339)
	}
	if filter.OperationType != "" {
		params["operation_type"] = string(filter.OperationType)
	}
	if filter.ComponentType != "" {
		params["component_type"] = filter.ComponentType
	}
	if filter.ComponentID != "" {
		params["component_id"] = filter.ComponentID
	}
	if filter.ProjectID != "" {
		params["project_id"] = filter.ProjectID
	}
	if filter.Status != "" {
		params["status"] = filter.Status
	}
	if filter.Keyword != "" {
		params["keyword"] = filter.Keyword
	}
	if filter.Limit > 0 {
		params["limit"] = fmt.Sprintf("%d", filter.Limit)
	}
	if filter.Offset > 0 {
		params["offset"] = fmt.Sprintf("%d", filter.Offset)
	}

	// Make HTTP request to follower
	url := fmt.Sprintf("http://%s/operations-history", nodeAddress)
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add query parameters
	q := req.URL.Query()
	for key, value := range params {
		q.Add(key, value)
	}
	req.URL.RawQuery = q.Encode()

	// Add authentication token
	req.Header.Set("token", common.Config.Token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request follower operations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("follower returned status %d", resp.StatusCode)
	}

	var response OperationHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode follower response: %w", err)
	}

	return response.Operations, nil
}

// aggregateClusterOperationHistory collects operation history from all cluster nodes
func aggregateClusterOperationHistory(filter OperationHistoryFilter) ([]common.OperationRecord, map[string]NodeOpStat, error) {
	var allOperations []common.OperationRecord
	nodeStats := make(map[string]NodeOpStat)

	// Get leader's own operations first
	leaderOps, err := getOperationHistory(filter)
	if err != nil {
		logger.Error("Failed to get leader operation history", "error", err)
	} else {
		allOperations = append(allOperations, leaderOps...)

		// Calculate leader stats
		leaderStat := NodeOpStat{
			NodeID: common.Config.LocalIP,
		}
		for _, op := range leaderOps {
			switch op.Type {
			case common.OpTypeChangePush:
				leaderStat.ChangePushOps++
			case common.OpTypeLocalPush:
				leaderStat.LocalPushOps++
			case common.OpTypeProjectStart:
				leaderStat.ProjectStartOps++
			case common.OpTypeProjectStop:
				leaderStat.ProjectStopOps++
			case common.OpTypeProjectRestart:
				leaderStat.ProjectRestartOps++
			}
			leaderStat.TotalOps++
			if op.Status == "success" {
				leaderStat.SuccessOps++
			} else {
				leaderStat.FailedOps++
			}
		}
		nodeStats[common.Config.LocalIP] = leaderStat
	}

	// Get operations from follower nodes
	if cluster.ClusterInstance != nil {
		cluster.ClusterInstance.Mu.RLock()
		nodes := make(map[string]*cluster.NodeInfo)
		for k, v := range cluster.ClusterInstance.Nodes {
			nodes[k] = v
		}
		cluster.ClusterInstance.Mu.RUnlock()

		for nodeID, nodeInfo := range nodes {
			if nodeID == common.Config.LocalIP || !nodeInfo.IsHealthy {
				continue // Skip self and unhealthy nodes
			}

			// Attempt Redis cache first
			var followerOps []common.OperationRecord
			if cached, err := common.RedisGet("cluster:operations:" + nodeID); err == nil && cached != "" {
				_ = json.Unmarshal([]byte(cached), &followerOps)
			}

			// Fallback to HTTP if cache missing
			if len(followerOps) == 0 {
				fops, err := getFollowerOperationHistory(nodeInfo.Address, filter)
				if err != nil {
					logger.Error("Failed to get follower operation history", "node", nodeID, "error", err)
					continue
				}
				followerOps = fops
			}

			if len(followerOps) == 0 {
				continue
			}

			// Set node information for follower operations
			for i := range followerOps {
				if followerOps[i].Details == nil {
					followerOps[i].Details = make(map[string]interface{})
				}
				followerOps[i].Details["node_id"] = nodeID
				followerOps[i].Details["node_address"] = nodeInfo.Address
			}

			allOperations = append(allOperations, followerOps...)

			// Calculate follower stats
			followerStat := NodeOpStat{
				NodeID: nodeID,
			}
			for _, op := range followerOps {
				switch op.Type {
				case common.OpTypeChangePush:
					followerStat.ChangePushOps++
				case common.OpTypeLocalPush:
					followerStat.LocalPushOps++
				case common.OpTypeProjectStart:
					followerStat.ProjectStartOps++
				case common.OpTypeProjectStop:
					followerStat.ProjectStopOps++
				case common.OpTypeProjectRestart:
					followerStat.ProjectRestartOps++
				}
				followerStat.TotalOps++
				if op.Status == "success" {
					followerStat.SuccessOps++
				} else {
					followerStat.FailedOps++
				}
			}
			nodeStats[nodeID] = followerStat
		}
	}

	// Sort all operations by timestamp (newest first)
	sort.Slice(allOperations, func(i, j int) bool {
		return allOperations[i].Timestamp.After(allOperations[j].Timestamp)
	})

	return allOperations, nodeStats, nil
}

// storeLocalOperationsToRedis caches latest operations for leader aggregation
func storeLocalOperationsToRedis(nodeID string, operations []common.OperationRecord) {
	if len(operations) == 0 {
		return
	}
	data, err := json.Marshal(operations)
	if err != nil {
		return
	}
	// Keep for 31 days; refreshed on each upload
	_, _ = common.RedisSet("cluster:operations:"+nodeID, string(data), 31*24*60*60)
}

// StartOperationHistoryUploader starts periodic operation history upload for follower nodes
func StartOperationHistoryUploader() {
	if cluster.IsLeader {
		return // Leader doesn't need to upload operations
	}

	go func() {
		ticker := time.NewTicker(60 * time.Second) // Upload every 60 seconds
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				uploadOperationHistoryToRedis()
			}
		}
	}()

	logger.Info("Operation history uploader started for follower node")
}

// uploadOperationHistoryToRedis uploads recent operation history to Redis
func uploadOperationHistoryToRedis() {
	filter := OperationHistoryFilter{
		StartTime: time.Now().Add(-5 * time.Minute), // Last 5 minutes
		EndTime:   time.Now(),
		Limit:     100,
	}

	operations, err := getOperationHistory(filter)
	if err != nil {
		logger.Error("Failed to get local operation history for upload", "error", err)
		return
	}

	if len(operations) > 0 {
		storeLocalOperationsToRedis(common.Config.LocalIP, operations)
		logger.Debug("Uploaded operation history to Redis", "count", len(operations))
	}
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

	// Cache to Redis for leader aggregation
	storeLocalOperationsToRedis(common.Config.LocalIP, operations)

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

// GetClusterOperationsHistory handles GET /cluster-operations-history (leader only)
func GetClusterOperationsHistory(c echo.Context) error {
	// Only allow on leader node
	if !cluster.IsLeader {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "This endpoint is only available on the leader node",
		})
	}

	var filter OperationHistoryFilter

	// Parse query parameters (same as GetOperationsHistory)
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

	// Node ID filter for cluster operations
	nodeID := c.QueryParam("node_id")

	// Collect operations from all cluster nodes
	allOperations, nodeStats, err := aggregateClusterOperationHistory(filter)
	if err != nil {
		logger.Error("Failed to aggregate cluster operation history", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to aggregate cluster operation history: " + err.Error(),
		})
	}

	// Apply node filter if specified
	if nodeID != "" && nodeID != "all" {
		var filteredOps []common.OperationRecord
		for _, op := range allOperations {
			if op.Details != nil {
				if opNodeID, ok := op.Details["node_id"].(string); ok && opNodeID == nodeID {
					filteredOps = append(filteredOps, op)
				}
			}
		}
		allOperations = filteredOps
	}

	totalCount := len(allOperations)

	// Apply pagination
	start := filter.Offset
	end := start + filter.Limit

	if start > totalCount {
		start = totalCount
	}
	if end > totalCount {
		end = totalCount
	}

	paginatedOperations := allOperations[start:end]
	hasMore := end < totalCount

	response := ClusterOperationHistoryResponse{
		Operations: paginatedOperations,
		NodeStats:  nodeStats,
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
