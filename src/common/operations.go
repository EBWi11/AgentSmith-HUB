package common

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"AgentSmith-HUB/logger"
)

const (
	operationsHistoryKey = "cluster:ops_history"
	operationsRecordKey  = "cluster:ops:record:"
	operationsStateKey   = "cluster:ops:state:"
	operationsRevertKey  = "cluster:ops:revert_of:"
	operationsTTLSeconds = 31 * 24 * 60 * 60
)

func NewOperationID() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("op_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("op_%d_%s", time.Now().UnixNano(), hex.EncodeToString(buf[:]))
}

func applyOperationDefaults(record *OperationRecord) {
	if record.OperationID == "" {
		record.OperationID = NewOperationID()
	}
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}
	if record.Details == nil {
		record.Details = make(map[string]interface{}, 3)
	}
	if _, exists := record.Details["node_id"]; !exists {
		record.Details["node_id"] = Config.LocalIP
	}
	if _, exists := record.Details["node_address"]; !exists {
		record.Details["node_address"] = Config.LocalIP
	}
	if _, exists := record.Details["executed_by"]; !exists {
		record.Details["executed_by"] = Config.LocalIP
	}
}

func operationRecordKey(operationID string) string { return operationsRecordKey + operationID }
func operationStateHashKey(operationID string) string {
	return operationsStateKey + operationID
}
func operationRevertKey(operationID string) string { return operationsRevertKey + operationID }

func RecordOperation(record OperationRecord) (string, error) {
	applyOperationDefaults(&record)

	jsonData, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("marshal operation record: %w", err)
	}

	if _, err := RedisSet(operationRecordKey(record.OperationID), string(jsonData), operationsTTLSeconds); err != nil {
		return "", fmt.Errorf("write operation record: %w", err)
	}
	if err := RedisHSet(operationStateHashKey(record.OperationID), "reverted", fmt.Sprintf("%t", record.Reverted)); err != nil {
		return "", fmt.Errorf("write operation state reverted: %w", err)
	}
	if record.RevertedByOperationID != "" {
		if err := RedisHSet(operationStateHashKey(record.OperationID), "reverted_by_operation_id", record.RevertedByOperationID); err != nil {
			return "", fmt.Errorf("write operation state reverted_by_operation_id: %w", err)
		}
	}
	if err := RedisHSet(operationStateHashKey(record.OperationID), "status", record.Status); err != nil {
		return "", fmt.Errorf("write operation state status: %w", err)
	}
	if err := RedisHSet(operationStateHashKey(record.OperationID), "updated_at", record.Timestamp.Format(time.RFC3339Nano)); err != nil {
		return "", fmt.Errorf("write operation state updated_at: %w", err)
	}
	if err := RedisExpire(operationStateHashKey(record.OperationID), operationsTTLSeconds); err != nil {
		logger.Warn("Failed to set operation state TTL", "operation_id", record.OperationID, "error", err)
	}

	if err := RedisLPush(operationsHistoryKey, string(jsonData), 10000); err != nil {
		return "", fmt.Errorf("append operation history: %w", err)
	}
	if err := RedisExpire(operationsHistoryKey, operationsTTLSeconds); err != nil {
		logger.Warn("Failed to set operations history TTL", "error", err)
	}

	if record.RevertsOperationID != "" {
		if _, err := RedisSet(operationRevertKey(record.RevertsOperationID), record.OperationID, operationsTTLSeconds); err != nil {
			return "", fmt.Errorf("write operation revert relation: %w", err)
		}
	}

	return record.OperationID, nil
}

func GetOperationRecord(operationID string) (OperationRecord, error) {
	var record OperationRecord
	if strings.TrimSpace(operationID) == "" {
		return record, fmt.Errorf("operation id is required")
	}

	raw, err := RedisGet(operationRecordKey(operationID))
	if err != nil {
		return record, fmt.Errorf("get operation record: %w", err)
	}
	if raw == "" {
		return record, fmt.Errorf("operation not found: %s", operationID)
	}
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return record, fmt.Errorf("unmarshal operation record: %w", err)
	}
	return applyOperationState(record), nil
}

func MarkOperationReverted(operationID, revertedByOperationID string) error {
	if strings.TrimSpace(operationID) == "" {
		return fmt.Errorf("operation id is required")
	}
	if err := RedisHSet(operationStateHashKey(operationID), "reverted", "true"); err != nil {
		return err
	}
	if err := RedisHSet(operationStateHashKey(operationID), "reverted_by_operation_id", revertedByOperationID); err != nil {
		return err
	}
	if err := RedisHSet(operationStateHashKey(operationID), "updated_at", time.Now().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return RedisExpire(operationStateHashKey(operationID), operationsTTLSeconds)
}

func applyOperationState(record OperationRecord) OperationRecord {
	if record.OperationID == "" {
		return record
	}
	state, err := RedisHGetAll(operationStateHashKey(record.OperationID))
	if err != nil || len(state) == 0 {
		return record
	}
	if revertedValue, exists := state["reverted"]; exists {
		record.Reverted = strings.EqualFold(revertedValue, "true")
	}
	if revertedBy, exists := state["reverted_by_operation_id"]; exists && revertedBy != "" {
		record.RevertedByOperationID = revertedBy
	}
	if status, exists := state["status"]; exists && status != "" {
		record.Status = status
	}
	return record
}

// RecordProjectOperation records a project operation to Redis only
func RecordProjectOperation(operationType OperationType, projectID, status, errorMsg string, details map[string]interface{}) {
	record := OperationRecord{
		Type:      operationType,
		ProjectID: projectID,
		Status:    status,
		Error:     errorMsg,
		Details:   details,
	}
	if _, err := RecordOperation(record); err != nil {
		logger.Error("Failed to record project operation", "operation", operationType, "project", projectID, "error", err)
		return
	}

	logger.Info("Operation recorded to Redis", "type", record.Type, "project", record.ProjectID, "status", record.Status)
}

// RecordClusterInstruction records a cluster instruction operation to Redis
func RecordClusterInstruction(operationType OperationType, instructionType, componentID, componentType string, status, errorMsg, content string, details map[string]interface{}) {
	// Ensure details map exists and contains execution node information
	if details == nil {
		details = make(map[string]interface{}, 3)
	}

	// Always record the actual execution node (don't override if already set)
	if _, exists := details["node_id"]; !exists {
		details["node_id"] = Config.LocalIP
	}
	if _, exists := details["node_address"]; !exists {
		details["node_address"] = Config.LocalIP
	}
	if _, exists := details["executed_by"]; !exists {
		details["executed_by"] = Config.LocalIP
	}

	// Add instruction-specific details
	if instructionType != "" {
		details["instruction_type"] = instructionType
	}
	if content != "" {
		details["instruction_content"] = content
	}

	record := OperationRecord{
		Type:          operationType,
		ComponentType: componentType,
		ComponentID:   componentID,
		Status:        status,
		Error:         errorMsg,
		NewContent:    content, // Store instruction content
		Details:       details,
	}
	if _, err := RecordOperation(record); err != nil {
		logger.Error("Failed to record cluster instruction", "operation", operationType, "instruction_type", instructionType, "component", componentID, "error", err)
		return
	}

	logger.Info("Cluster instruction recorded to Redis", "type", record.Type, "instruction_type", instructionType, "component", record.ComponentID, "status", record.Status)
}

// RecordComponentAdd records a component addition operation
func RecordComponentAdd(componentType, componentID, content, status, errorMsg string) {
	details := map[string]interface{}{
		"node_id":      Config.LocalIP,
		"node_address": Config.LocalIP,
		"executed_by":  Config.LocalIP,
		"source":       "cluster_instruction",
	}

	record := OperationRecord{
		Type:          OpTypeComponentAdd,
		ComponentType: componentType,
		ComponentID:   componentID,
		NewContent:    content,
		Status:        status,
		Error:         errorMsg,
		Details:       details,
	}
	if _, err := RecordOperation(record); err != nil {
		logger.Error("Failed to record component add", "component", componentID, "error", err)
		return
	}

	logger.Info("Component add operation recorded", "type", componentType, "component", componentID, "status", status)
}

// RecordComponentUpdate records a component update operation
func RecordComponentUpdate(componentType, componentID, content, status, errorMsg string) {
	details := map[string]interface{}{
		"node_id":      Config.LocalIP,
		"node_address": Config.LocalIP,
		"executed_by":  Config.LocalIP,
		"source":       "cluster_instruction",
	}

	record := OperationRecord{
		Type:          OpTypeComponentUpdate,
		ComponentType: componentType,
		ComponentID:   componentID,
		NewContent:    content,
		Status:        status,
		Error:         errorMsg,
		Details:       details,
	}
	if _, err := RecordOperation(record); err != nil {
		logger.Error("Failed to record component update", "component", componentID, "error", err)
		return
	}

	logger.Info("Component update operation recorded", "type", componentType, "component", componentID, "status", status)
}

// RecordComponentDelete records a component deletion operation
func RecordComponentDelete(componentType, componentID, status, errorMsg string, affectedProjects []string) {
	details := map[string]interface{}{
		"node_id":           Config.LocalIP,
		"node_address":      Config.LocalIP,
		"executed_by":       Config.LocalIP,
		"source":            "cluster_instruction",
		"affected_projects": affectedProjects,
	}

	record := OperationRecord{
		Type:          OpTypeComponentDelete,
		ComponentType: componentType,
		ComponentID:   componentID,
		Status:        status,
		Error:         errorMsg,
		Details:       details,
	}
	if _, err := RecordOperation(record); err != nil {
		logger.Error("Failed to record component delete", "component", componentID, "error", err)
		return
	}

	logger.Info("Component delete operation recorded", "type", componentType, "component", componentID, "status", status)
}

// RecordChangePush records a change push operation
func RecordChangePush(componentType, componentID, oldContent, newContent, diff, status, errorMsg string) {
	details := map[string]interface{}{
		"node_id":      Config.LocalIP,
		"node_address": Config.LocalIP,
		"executed_by":  Config.LocalIP,
		"source":       "cluster_instruction",
	}

	record := OperationRecord{
		Type:          OpTypeChangePush,
		ComponentType: componentType,
		ComponentID:   componentID,
		OldContent:    oldContent,
		NewContent:    newContent,
		Diff:          diff,
		Status:        status,
		Error:         errorMsg,
		Details:       details,
	}
	if _, err := RecordOperation(record); err != nil {
		logger.Error("Failed to record change push", "component", componentID, "error", err)
		return
	}

	logger.Info("Change push operation recorded", "type", componentType, "component", componentID, "status", status)
}

// RecordLocalPush records a local push operation
func RecordLocalPush(componentType, componentID, content, status, errorMsg string) {
	details := map[string]interface{}{
		"node_id":      Config.LocalIP,
		"node_address": Config.LocalIP,
		"executed_by":  Config.LocalIP,
		"source":       "cluster_instruction",
	}

	record := OperationRecord{
		Type:          OpTypeLocalPush,
		ComponentType: componentType,
		ComponentID:   componentID,
		NewContent:    content,
		Status:        status,
		Error:         errorMsg,
		Details:       details,
	}
	if _, err := RecordOperation(record); err != nil {
		logger.Error("Failed to record local push", "component", componentID, "error", err)
		return
	}

	logger.Info("Local push operation recorded", "type", componentType, "component", componentID, "status", status)
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

// OperationHistoryFilter represents filter parameters for operations history
type OperationHistoryFilter struct {
	StartTime     time.Time     `json:"start_time"`
	EndTime       time.Time     `json:"end_time"`
	OperationType OperationType `json:"operation_type"`
	ComponentType string        `json:"component_type"`
	ComponentID   string        `json:"component_id"`
	ProjectID     string        `json:"project_id"`
	Status        string        `json:"status"`
	Keyword       string        `json:"keyword"`
	NodeID        string        `json:"node_id"`
	Limit         int           `json:"limit"`
	Offset        int           `json:"offset"`
}

// GetOperationsFromRedisWithFilter retrieves operations from Redis with server-side filtering and pagination
func GetOperationsFromRedisWithFilter(filter OperationHistoryFilter) ([]OperationRecord, int, error) {
	// Read all operations from Redis
	redisLines, err := RedisLRange(operationsHistoryKey, 0, -1)
	if err != nil {
		logger.Error("Failed to read operations from Redis", "error", err)
		return []OperationRecord{}, 0, err
	}

	var filteredOperations []OperationRecord

	// Parse and filter operations
	for _, line := range redisLines {
		var op OperationRecord
		if err := json.Unmarshal([]byte(line), &op); err != nil {
			logger.Error("Failed to unmarshal operation record", "error", err)
			continue
		}
		op = applyOperationState(op)

		// Apply filters
		if !matchesOperationFilter(op, filter) {
			continue
		}

		filteredOperations = append(filteredOperations, op)
	}

	// Sort by timestamp (newest first) - use efficient sort
	sort.Slice(filteredOperations, func(i, j int) bool {
		return filteredOperations[i].Timestamp.After(filteredOperations[j].Timestamp)
	})

	totalCount := len(filteredOperations)

	// Apply pagination
	start := filter.Offset
	end := start + filter.Limit

	if start > totalCount {
		start = totalCount
	}
	if end > totalCount {
		end = totalCount
	}

	paginatedOperations := filteredOperations[start:end]

	return paginatedOperations, totalCount, nil
}

// matchesOperationFilter checks if an operation record matches the filter criteria
func matchesOperationFilter(record OperationRecord, filter OperationHistoryFilter) bool {
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

	// Node ID filter
	if filter.NodeID != "" && filter.NodeID != "all" {
		nodeID := ""
		if record.Details != nil {
			if nodeIDValue, exists := record.Details["node_id"]; exists {
				if nodeIDStr, ok := nodeIDValue.(string); ok {
					nodeID = nodeIDStr
				}
			}
		}
		if nodeID != filter.NodeID {
			return false
		}
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
