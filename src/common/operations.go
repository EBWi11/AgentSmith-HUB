package common

import (
	"encoding/json"
	"time"

	"AgentSmith-HUB/logger"
)

// RecordProjectOperation records a project operation to Redis only
func RecordProjectOperation(operationType OperationType, projectID, status, errorMsg string, details map[string]interface{}) {
	record := OperationRecord{
		Type:      operationType,
		Timestamp: time.Now(),
		ProjectID: projectID,
		Status:    status,
		Error:     errorMsg,
		Details:   details,
	}

	// Serialize record to JSON
	jsonData, err := json.Marshal(record)
	if err != nil {
		logger.Error("Failed to marshal operation record", "operation", operationType, "project", projectID, "error", err)
		return
	}

	// Store to Redis list with size limit
	if err := RedisLPush("cluster:ops_history", string(jsonData), 10000); err != nil {
		logger.Error("Failed to record project operation to Redis", "operation", operationType, "project", projectID, "error", err)
		return
	}

	// Set TTL for the entire list to 31 days (31 * 24 * 60 * 60 = 2,678,400 seconds)
	if err := RedisExpire("cluster:ops_history", 31*24*60*60); err != nil {
		logger.Warn("Failed to set TTL for operations history", "error", err)
	}

	logger.Info("Operation recorded to Redis", "type", record.Type, "project", record.ProjectID, "status", record.Status)
}
