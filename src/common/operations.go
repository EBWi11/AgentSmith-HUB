package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"AgentSmith-HUB/logger"
)

// getOperationsLogDir returns the operations history log directory
func getOperationsLogDir() string {
	if runtime.GOOS == "darwin" {
		return "/tmp/hub_logs/operations"
	}
	return "/var/log/hub_logs/operations"
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

// recordOperationToFile records an operation to the monthly log file
func recordOperationToFile(record OperationRecord) error {
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

	logger.Info("Operation recorded", "type", record.Type, "component", record.ComponentType, "id", record.ComponentID, "project", record.ProjectID)
	return nil
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

	if err := recordOperationToFile(record); err != nil {
		logger.Error("Failed to record project operation", "operation", operationType, "project", projectID, "error", err)
	}

	// Also try to publish to Redis for cluster aggregation (if Redis is available)
	if jsonData, err := json.Marshal(record); err == nil {
		// Only publish to Redis if it's available and we're not the leader
		// We avoid importing cluster package to prevent circular dependency
		// Instead, we use a simple Redis operation without cluster checking
		_ = RedisLPush("cluster:ops_history", string(jsonData), 50000)
	}
}
