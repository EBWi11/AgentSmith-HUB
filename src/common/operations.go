package common

import (
	"encoding/json"
	"fmt"
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

// InitComponentUpdateManager initializes the global component update manager
func InitComponentUpdateManager() {
	GlobalComponentUpdateManager = &ComponentUpdateManager{
		activeUpdates: make(map[string]*ComponentUpdateOperation),
	}
}

// StartComponentUpdate starts a new component update operation
func (cum *ComponentUpdateManager) StartComponentUpdate(componentType, componentID string, affectedProjects []string) (*ComponentUpdateOperation, error) {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()

	key := fmt.Sprintf("%s:%s", componentType, componentID)

	// Check if update is already in progress
	if existing, exists := cum.activeUpdates[key]; exists {
		if existing.State != UpdateStateFailed && existing.State != UpdateStateIdle {
			return nil, fmt.Errorf("component update already in progress")
		}
	}

	// Create distributed lock
	lockKey := fmt.Sprintf("update_%s_%s", componentType, componentID)
	lock := NewDistributedLock(lockKey, 5*time.Minute)

	// Try to acquire lock
	if err := lock.TryAcquire(10 * time.Second); err != nil {
		return nil, fmt.Errorf("failed to acquire update lock: %w", err)
	}

	// Create update operation
	operation := &ComponentUpdateOperation{
		ComponentType:    componentType,
		ComponentID:      componentID,
		State:            UpdateStatePreparing,
		StartTime:        time.Now(),
		LastUpdate:       time.Now(),
		AffectedProjects: affectedProjects,
		Lock:             lock,
	}

	cum.activeUpdates[key] = operation
	return operation, nil
}

// CompleteComponentUpdate completes a component update operation
func (cum *ComponentUpdateManager) CompleteComponentUpdate(componentType, componentID string, success bool) error {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()

	key := fmt.Sprintf("%s:%s", componentType, componentID)
	operation, exists := cum.activeUpdates[key]
	if !exists {
		return fmt.Errorf("no active update operation found")
	}

	// Update state
	operation.mutex.Lock()
	if success {
		operation.State = UpdateStateIdle
	} else {
		operation.State = UpdateStateFailed
	}
	operation.LastUpdate = time.Now()
	operation.mutex.Unlock()

	// Release lock
	if operation.Lock != nil {
		operation.Lock.Release()
	}

	// Remove from active updates
	delete(cum.activeUpdates, key)

	return nil
}

// UpdateOperationState updates the state of an ongoing operation
func (operation *ComponentUpdateOperation) UpdateState(newState ComponentUpdateState) {
	operation.mutex.Lock()
	defer operation.mutex.Unlock()

	operation.State = newState
	operation.LastUpdate = time.Now()
}

// GetActiveUpdates returns a copy of active update operations
func (cum *ComponentUpdateManager) GetActiveUpdates() map[string]*ComponentUpdateOperation {
	cum.mutex.RLock()
	defer cum.mutex.RUnlock()

	result := make(map[string]*ComponentUpdateOperation)
	for k, v := range cum.activeUpdates {
		result[k] = v
	}
	return result
}

// CleanupStaleUpdates removes stale update operations
func (cum *ComponentUpdateManager) CleanupStaleUpdates(maxAge time.Duration) {
	cum.mutex.Lock()
	defer cum.mutex.Unlock()

	now := time.Now()
	for key, operation := range cum.activeUpdates {
		if now.Sub(operation.LastUpdate) > maxAge {
			logger.Warn("Cleaning up stale component update operation", "key", key, "age", now.Sub(operation.LastUpdate))

			// Release lock if still held
			if operation.Lock != nil {
				operation.Lock.Release()
			}

			delete(cum.activeUpdates, key)
		}
	}
}
