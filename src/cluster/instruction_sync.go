package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Instruction represents a single operation
type Instruction struct {
	Version         int64                  `json:"version"`
	ComponentName   string                 `json:"component_name"`
	ComponentType   string                 `json:"component_type"` // project, input, output, ruleset, plugin
	Content         string                 `json:"content"`
	Operation       string                 `json:"operation"`    // add, delete, start, restart, stop, update, local_push, push_change
	Dependencies    []string               `json:"dependencies"` // affected projects that need restart
	Metadata        map[string]interface{} `json:"metadata"`     // additional operation metadata
	Timestamp       int64                  `json:"timestamp"`
	RequiresRestart bool                   `json:"requires_restart"` // whether this operation requires project restart
}

var CUD_OPERATION = map[string]bool{
	"add":         true,
	"delete":      true,
	"update":      true,
	"push_change": true,
	"local_push":  true,
}

var PROJECT_OPERATION = map[string]bool{
	"start":   true,
	"stop":    true,
	"restart": true,
}

func GetDeletedIntentionsString() string {
	return "{\"component_type\":\"DELETE\"}"
}

func CheckDeletedIntention(i *Instruction) bool {
	if i.ComponentType == "DELETE" {
		return true
	}
	return false
}

// InstructionCompactionRule defines rules for instruction compaction
type InstructionCompactionRule struct {
	ComponentType string
	ComponentName string
	Operations    []string // operations that can be compacted
}

// PendingInstruction represents an instruction waiting to be processed
type PendingInstruction struct {
	ComponentName string
	ComponentType string
	Content       string
	Operation     string
	Dependencies  []string
	Metadata      map[string]interface{}
	ResultChan    chan error // channel to return the result
}

// InstructionManager manages version-based synchronization
type InstructionManager struct {
	currentVersion  int64
	baseVersion     string
	mu              sync.RWMutex
	maxInstructions int64 // trigger compaction when exceeding this number
	queue           chan *PendingInstruction
	workerStopped   chan struct{}
	once            sync.Once
}

func (im *InstructionManager) NewInstructionManagerFollower() *InstructionManager {
	return &InstructionManager{
		currentVersion: 0,
		baseVersion:    "0",
		queue:          make(chan *PendingInstruction, 1000), // buffer for 1000 pending instructions
		workerStopped:  make(chan struct{}),
	}
}

var GlobalInstructionManager *InstructionManager

// generateSessionID generates an 8-character random session identifier
func generateSessionID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)

	// Generate random bytes
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to time-based generation if crypto/rand fails
		return fmt.Sprintf("t%07d", time.Now().Unix()%10000000)
	}

	// Convert random bytes to charset characters
	for i := range b {
		b[i] = charset[randomBytes[i]%byte(len(charset))]
	}
	return string(b)
}

// InitInstructionManager initializes the instruction manager
func InitInstructionManager() {
	GlobalInstructionManager = &InstructionManager{
		currentVersion:  0,                                    // Start with version 0 (temporary state)
		baseVersion:     generateSessionID(),                  // Session identifier (6-char random string)
		maxInstructions: 2000,                                 // compact when > 1000 instructions
		queue:           make(chan *PendingInstruction, 1000), // buffer for 1000 pending instructions
		workerStopped:   make(chan struct{}),
	}

	// Start the queue worker
	GlobalInstructionManager.startWorker()
}

// GetCurrentVersion returns current version string
func (im *InstructionManager) GetCurrentVersion() string {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return fmt.Sprintf("%s.%d", im.baseVersion, im.currentVersion)
}

// getCurrentVersionUnsafe returns version string without locking (must be called with lock held)
func (im *InstructionManager) getCurrentVersionUnsafe() string {
	return fmt.Sprintf("%s.%d", im.baseVersion, im.currentVersion)
}

// IsCompacting returns whether instruction manager is currently in compaction mode
// During compaction, currentVersion is temporarily set to 0
func (im *InstructionManager) IsCompacting() bool {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return im.currentVersion == 0
}

// setCurrentVersion updates version and persists to Redis (must be called with lock held)
func (im *InstructionManager) setCurrentVersion(veresion int64) (int64, error) {
	ori := im.currentVersion
	im.currentVersion = veresion

	_, err := common.RedisSet("cluster:leader_version", im.getCurrentVersionUnsafe(), 0)
	if err != nil {
		im.currentVersion = ori
		return 0, fmt.Errorf("failed to update cluster version in Redis: %w", err)
	}

	return ori, nil
}

// loadAllInstructions loads all instructions from Redis
func (im *InstructionManager) loadAllInstructions(maxVersion int64) ([]*Instruction, error) {
	var instructions []*Instruction

	for version := int64(1); version <= maxVersion; version++ {
		key := fmt.Sprintf("cluster:instruction:%d", version)
		data, err := common.RedisGet(key)
		if err != nil {
			continue
		}

		var instruction Instruction
		if err := json.Unmarshal([]byte(data), &instruction); err != nil {
			logger.Error("Failed to unmarshal instruction", "version", version, "error", err)
			continue
		}

		instructions = append(instructions, &instruction)
	}

	return instructions, nil
}

// startWorker starts the queue worker to process instructions sequentially
func (im *InstructionManager) startWorker() {
	go func() {
		defer close(im.workerStopped)

		for pending := range im.queue {
			// Process the instruction synchronously with lock
			err := im.processInstructionInternal(
				pending.ComponentName,
				pending.ComponentType,
				pending.Content,
				pending.Operation,
				pending.Dependencies,
				pending.Metadata,
			)

			// Send result back to caller
			pending.ResultChan <- err
		}
	}()
}

// processInstructionInternal processes an instruction with proper locking
func (im *InstructionManager) processInstructionInternal(componentName, componentType, content, operation string, dependencies []string, metadata map[string]interface{}) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	if !common.IsCurrentNodeLeader() {
		return fmt.Errorf("only leader can initialize instructions")
	}

	if componentName == "" || componentType == "" || operation == "" {
		return fmt.Errorf("component name, type, and operation are required")
	}

	requiresRestart := im.operationRequiresRestart(operation, componentType)
	instruction := Instruction{
		ComponentName:   componentName,
		ComponentType:   componentType,
		Content:         content,
		Operation:       operation,
		Dependencies:    dependencies,
		Metadata:        metadata,
		Timestamp:       time.Now().Unix(),
		RequiresRestart: requiresRestart,
	}

	err := im.CompactAndSaveInstructions(&instruction)
	if err != nil {
		logger.Error("Failed to compact and save instructions", "error", err)

		// Record failed instruction
		common.RecordClusterInstruction(
			common.OpTypeInstructionPublish,
			operation,
			componentName,
			componentType,
			"failed",
			err.Error(),
			content,
			map[string]interface{}{
				"version":          im.currentVersion,
				"requires_restart": requiresRestart,
				"dependencies":     dependencies,
				"metadata":         metadata,
				"role":             "leader",
			},
		)

		return err // Return error instead of nil
	}

	// Only send publish_complete if compaction succeeded
	publishComplete := map[string]interface{}{
		"action":         "publish_complete",
		"leader_version": im.getCurrentVersionUnsafe(),
		"timestamp":      time.Now().Unix(),
	}

	if data, err := json.Marshal(publishComplete); err == nil {
		_ = common.RedisPublish("cluster:sync_command", string(data))
	}
	logger.Info("Instruction published", "version", im.currentVersion, "component", componentName, "operation", operation)

	// Record successful instruction
	common.RecordClusterInstruction(
		common.OpTypeInstructionPublish,
		operation,
		componentName,
		componentType,
		"success",
		"",
		content,
		map[string]interface{}{
			"version":          im.currentVersion,
			"requires_restart": requiresRestart,
			"dependencies":     dependencies,
			"metadata":         metadata,
			"role":             "leader",
		},
	)

	return nil
}

func (im *InstructionManager) CompactAndSaveInstructions(new *Instruction) error {
	// Wait for all followers to complete their current synchronization
	// Timeout is 45s (execution flag TTL is 30s, plus 15s buffer)
	kickedFollowers := false
	if err := im.WaitForAllFollowersIdle(45 * time.Second); err != nil {
		logger.Error("Timeout waiting for followers to complete sync, will kick out slow followers", "error", err)

		// Get the list of slow/stuck followers
		activeFollowers, _ := im.GetActiveFollowers()

		// Kick out these followers - they will full resync on next heartbeat
		for _, followerID := range activeFollowers {
			if err := im.KickFollowerForResync(followerID); err != nil {
				logger.Error("Failed to kick follower", "follower_id", followerID, "error", err)
			} else {
				logger.Info("Kicked out slow follower for full resync", "follower_id", followerID)
			}
		}

		kickedFollowers = len(activeFollowers) > 0
		// Continue with compaction - don't block the cluster
		logger.Info("Kicked out slow followers, proceeding with compaction", "kicked_count", len(activeFollowers))
	}

	if kickedFollowers {
		logger.Info("Proceeding with instruction compaction (slow followers were kicked out)")
	}

	originalVersion, err := im.setCurrentVersion(0)
	if err != nil {
		return err
	}

	delInstructions := map[int]bool{}
	instructions, err := im.loadAllInstructions(originalVersion)
	if err != nil {
		im.currentVersion = originalVersion
		_, _ = im.setCurrentVersion(originalVersion)
		return fmt.Errorf("failed to load instructions: %w", err)
	}

	originalLen := len(instructions)
	instructions = append(instructions, new)
	instructionsLen := len(instructions)

	for i, ii := range instructions {
		if CheckDeletedIntention(ii) {
			continue
		}

		for i2 := i + 1; i2 < instructionsLen; i2++ {
			ii2 := instructions[i2]
			if (ii.ComponentType == ii2.ComponentType) && (ii.ComponentName == ii2.ComponentName) {
				if CUD_OPERATION[ii.Operation] && CUD_OPERATION[ii2.Operation] {
					delInstructions[i] = true
					break
				} else if PROJECT_OPERATION[ii.Operation] && PROJECT_OPERATION[ii2.Operation] {
					delInstructions[i] = true
					break
				}
			}
		}
	}

	// Track failed writes to ensure atomicity of compaction
	var failedWrites []int64

	for i, instruction := range instructions {
		instruction.Version = int64(i + 1)
		key := fmt.Sprintf("cluster:instruction:%d", instruction.Version)

		if delInstructions[i] {
			// Write deleted instruction marker with retry
			err := common.RetryWithExponentialBackoff(func() error {
				_, e := common.RedisSet(key, GetDeletedIntentionsString(), 0)
				return e
			}, 3, 100*time.Millisecond)

			if err != nil {
				logger.Error("Failed to store compacted instruction after retries", "version", instruction.Version, "error", err)
				failedWrites = append(failedWrites, instruction.Version)
			}
		}

		// Write new instruction
		if i+1 == instructionsLen {
			data, _ := json.Marshal(instruction)
			err := common.RetryWithExponentialBackoff(func() error {
				_, e := common.RedisSet(key, string(data), 0)
				return e
			}, 3, 100*time.Millisecond)

			if err != nil {
				logger.Error("Failed to store compacted instruction after retries", "version", instruction.Version, "error", err)
				failedWrites = append(failedWrites, instruction.Version)
			}
		}
	}

	// If any writes failed, rollback and return error
	if len(failedWrites) > 0 {
		logger.Error("Compaction failed due to Redis write failures, rolling back",
			"failed_count", len(failedWrites),
			"failed_versions", failedWrites,
			"original_version", originalVersion)

		// Rollback: restore original version
		im.currentVersion = originalVersion
		_, _ = im.setCurrentVersion(originalVersion)

		return fmt.Errorf("compaction failed: %d instructions failed to write after retries", len(failedWrites))
	}

	// All writes succeeded, update version
	_, err = im.setCurrentVersion(int64(len(instructions)))
	if err != nil {
		// If version update fails, also rollback
		logger.Error("Failed to update version after compaction, rolling back", "error", err)
		im.currentVersion = originalVersion
		_, _ = im.setCurrentVersion(originalVersion)
		return fmt.Errorf("failed to update version after compaction: %w", err)
	}

	logger.Info("Compaction completed successfully",
		"original_version", originalVersion,
		"new_version", int64(len(instructions)),
		"old_instructions_count", originalLen)

	return nil
}

func (im *InstructionManager) PublishInstruction(componentName, componentType, content, operation string, dependencies []string, metadata map[string]interface{}) error {
	if !common.IsCurrentNodeLeader() {
		return fmt.Errorf("only leader can initialize instructions")
	}

	if componentName == "" || componentType == "" || operation == "" {
		return fmt.Errorf("component name, type, and operation are required")
	}

	// Check if queue is nil (shouldn't happen but safety check)
	if im.queue == nil {
		logger.Error("Queue is nil, falling back to direct processing")
		// Fallback to direct processing if queue not initialized
		return im.processInstructionInternal(componentName, componentType, content, operation, dependencies, metadata)
	}

	// Create a result channel for this instruction
	resultChan := make(chan error, 1)

	// Create pending instruction
	pending := &PendingInstruction{
		ComponentName: componentName,
		ComponentType: componentType,
		Content:       content,
		Operation:     operation,
		Dependencies:  dependencies,
		Metadata:      metadata,
		ResultChan:    resultChan,
	}

	// Submit to queue (non-blocking if queue has buffer space)
	select {
	case im.queue <- pending:
		// Successfully queued, wait for result
		err := <-resultChan
		return err
	default:
		// Queue is full, this should rarely happen but we need to handle it
		logger.Error("Instruction queue is full, falling back to direct processing")
		return im.processInstructionInternal(componentName, componentType, content, operation, dependencies, metadata)
	}
}

// operationRequiresRestart determines if an operation requires project restart
func (im *InstructionManager) operationRequiresRestart(operation, componentType string) bool {
	switch operation {
	case "add", "delete", "update", "push_change":
		return true // These operations modify components and require restart
	case "start", "stop", "restart":
		return false // These are already project control operations
	case "local_push":
		return true // Local push changes require restart
	default:
		return false
	}
}

// PublishComponentAdd publishes component addition instruction
func (im *InstructionManager) PublishComponentAdd(componentType, componentName, content string) error {
	return im.PublishInstruction(componentName, componentType, content, "add", nil, nil)
}

// PublishComponentDelete publishes component deletion instruction
func (im *InstructionManager) PublishComponentDelete(componentType, componentName string, affectedProjects []string) error {
	metadata := map[string]interface{}{
		"affected_projects": affectedProjects,
	}
	return im.PublishInstruction(componentName, componentType, "", "delete", affectedProjects, metadata)
}

// PublishComponentLocalPush publishes local push instruction
func (im *InstructionManager) PublishComponentLocalPush(componentType, componentName, content string, affectedProjects []string) error {
	metadata := map[string]interface{}{
		"affected_projects": affectedProjects,
		"source":            "local_load",
	}
	return im.PublishInstruction(componentName, componentType, content, "local_push", affectedProjects, metadata)
}

// PublishComponentPushChange publishes push change instruction
func (im *InstructionManager) PublishComponentPushChange(componentType, componentName, content string, affectedProjects []string) error {
	metadata := map[string]interface{}{
		"affected_projects": affectedProjects,
		"source":            "pending_changes",
	}
	return im.PublishInstruction(componentName, componentType, content, "push_change", affectedProjects, metadata)
}

// PublishProjectStart publishes project start instruction
func (im *InstructionManager) PublishProjectStart(projectName string) error {
	return im.PublishInstruction(projectName, "project", "", "start", nil, nil)
}

// PublishProjectStop publishes project stop instruction
func (im *InstructionManager) PublishProjectStop(projectName string) error {
	return im.PublishInstruction(projectName, "project", "", "stop", nil, nil)
}

// PublishProjectRestart publishes project restart instruction
func (im *InstructionManager) PublishProjectRestart(projectName string) error {
	return im.PublishInstruction(projectName, "project", "", "restart", nil, nil)
}

// PublishProjectsRestart publishes multiple project restart instructions
func (im *InstructionManager) PublishProjectsRestart(projectNames []string, reason string) error {
	metadata := map[string]interface{}{
		"reason": reason,
		"batch":  true,
	}

	var errors []string
	successCount := 0

	for _, projectName := range projectNames {
		if err := im.PublishInstruction(projectName, "project", "", "restart", nil, metadata); err != nil {
			logger.Error("Failed to publish restart instruction for project",
				"project", projectName,
				"error", err)
			errors = append(errors, fmt.Sprintf("%s: %v", projectName, err))
			// Continue processing other projects instead of returning immediately
		} else {
			successCount++
		}
	}

	if len(errors) > 0 {
		logger.Error("Batch restart completed with some failures",
			"total", len(projectNames),
			"success", successCount,
			"failed", len(errors))
		return fmt.Errorf("failed to restart %d/%d projects: %s",
			len(errors), len(projectNames), strings.Join(errors, "; "))
	}

	logger.Info("Batch restart completed successfully",
		"total", len(projectNames),
		"reason", reason)
	return nil
}

// InitializeLeaderInstructions creates initial instructions for all components (leader only)
func (im *InstructionManager) InitializeLeaderInstructions() error {
	im.mu.Lock()
	defer im.mu.Unlock()

	if !common.IsCurrentNodeLeader() {
		return fmt.Errorf("only leader can initialize instructions")
	}

	logger.Info("Initializing leader instructions", "new_base_version", im.baseVersion)

	// Check if there are old instructions from previous session
	oldVersionStr, err := common.RedisGet("cluster:leader_version")
	if err == nil && oldVersionStr != "" {
		// Parse old version to get baseVersion
		parts := strings.Split(oldVersionStr, ".")
		if len(parts) == 2 {
			oldBaseVersion := parts[0]
			if oldBaseVersion != im.baseVersion {
				logger.Info("Detected old instructions from previous session, will clean up",
					"old_base_version", oldBaseVersion,
					"new_base_version", im.baseVersion,
					"old_full_version", oldVersionStr)

				// Try to parse the old currentVersion to know how many to clean
				if oldCurrentVersion, parseErr := strconv.ParseInt(parts[1], 10, 64); parseErr == nil && oldCurrentVersion > 0 {
					logger.Info("Cleaning up old instructions", "count", oldCurrentVersion)
					for v := int64(1); v <= oldCurrentVersion; v++ {
						key := fmt.Sprintf("cluster:instruction:%d", v)
						if delErr := common.RedisDel(key); delErr != nil {
							logger.Error("Failed to delete old instruction", "version", v, "error", delErr)
						}
					}
					logger.Info("Old instructions cleaned up successfully", "cleaned_count", oldCurrentVersion)
				} else {
					// If we can't parse, try to clean up a reasonable range (e.g., up to maxInstructions)
					logger.Error("Could not parse old currentVersion, will clean up to maxInstructions",
						"old_version_str", oldVersionStr,
						"max_to_clean", im.maxInstructions)
					cleanedCount := 0
					for v := int64(1); v <= im.maxInstructions; v++ {
						key := fmt.Sprintf("cluster:instruction:%d", v)
						if delErr := common.RedisDel(key); delErr == nil {
							cleanedCount++
						}
					}
					logger.Info("Old instructions cleaned up (best effort)", "cleaned_count", cleanedCount)
				}
			} else {
				logger.Info("Base version matches, no cleanup needed", "base_version", im.baseVersion)
			}
		}
	} else {
		logger.Info("No previous instructions found in Redis, starting fresh")
	}

	_, err = im.setCurrentVersion(0)
	if err != nil {
		err = fmt.Errorf("failed to write leader version to Redis during initialization: %w", err)
		return err
	}

	var instructionCount int64 = 0
	var failedComponents []string

	// Helper function to publish instruction without triggering compaction
	publishInstructionDirectly := func(componentName, componentType, content, operation string, dependencies []string, metadata map[string]interface{}) error {
		instructionCount++
		// Determine if this operation requires project restart
		requiresRestart := im.operationRequiresRestart(operation, componentType)

		// Prepare instruction with temporary version (will be set after successful write)
		instruction := Instruction{
			Version:         instructionCount, // Next version number
			ComponentName:   componentName,
			ComponentType:   componentType,
			Content:         content,
			Operation:       operation,
			Dependencies:    dependencies,
			Metadata:        metadata,
			Timestamp:       time.Now().Unix(),
			RequiresRestart: requiresRestart,
		}

		// Store instruction in Redis
		key := fmt.Sprintf("cluster:instruction:%d", instructionCount)
		data, err := json.Marshal(instruction)
		if err != nil {
			return fmt.Errorf("failed to marshal instruction: %w", err)
		}

		if _, err := common.RedisSet(key, string(data), 0); err != nil {
			return fmt.Errorf("failed to store instruction: %w", err)
		}
		return nil
	}

	// 1. Add all inputs first (projects depend on inputs)
	common.ForEachRawConfig("input", func(inputID, config string) bool {
		if err := publishInstructionDirectly(inputID, "input", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish input add instruction", "input", inputID, "error", err)
			failedComponents = append(failedComponents, fmt.Sprintf("input:%s", inputID))
		}
		return true
	})

	// 2. Add all outputs (projects depend on outputs)
	common.ForEachRawConfig("output", func(outputID, config string) bool {
		if err := publishInstructionDirectly(outputID, "output", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish output add instruction", "output", outputID, "error", err)
			failedComponents = append(failedComponents, fmt.Sprintf("output:%s", outputID))
		}
		return true
	})

	// 3. Add all plugins (rulesets may depend on plugins)
	common.ForEachRawConfig("plugin", func(pluginID, config string) bool {
		if err := publishInstructionDirectly(pluginID, "plugin", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish plugin add instruction", "plugin", pluginID, "error", err)
			failedComponents = append(failedComponents, fmt.Sprintf("plugin:%s", pluginID))
		}
		return true
	})

	// 4. Add all rulesets (projects depend on rulesets)
	common.ForEachRawConfig("ruleset", func(rulesetID, config string) bool {
		if err := publishInstructionDirectly(rulesetID, "ruleset", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish ruleset add instruction", "ruleset", rulesetID, "error", err)
			failedComponents = append(failedComponents, fmt.Sprintf("ruleset:%s", rulesetID))
		}
		return true
	})

	// 5. Add all skills before agents resolve them.
	common.ForEachRawConfig("skill", func(skillID, config string) bool {
		if err := publishInstructionDirectly(skillID, "skill", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish skill add instruction", "skill", skillID, "error", err)
			failedComponents = append(failedComponents, fmt.Sprintf("skill:%s", skillID))
		}
		return true
	})

	// 6. Add all agents before projects reference them.
	common.ForEachRawConfig("agent", func(agentID, config string) bool {
		if err := publishInstructionDirectly(agentID, "agent", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish agent add instruction", "agent", agentID, "error", err)
			failedComponents = append(failedComponents, fmt.Sprintf("agent:%s", agentID))
		}
		return true
	})

	// 7. Add all projects LAST (projects depend on all above components)
	common.ForEachRawConfig("project", func(projectID, config string) bool {
		if err := publishInstructionDirectly(projectID, "project", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish project add instruction", "project", projectID, "error", err)
			failedComponents = append(failedComponents, fmt.Sprintf("project:%s", projectID))
		}
		return true
	})

	// 8. Start running projects

	if userIntentions, err := common.GetAllProjectUserIntentions(); err == nil {
		for projectID, wantRunning := range userIntentions {
			if wantRunning {
				if err := publishInstructionDirectly(projectID, "project", "", "start", nil, nil); err != nil {
					logger.Error("Failed to publish project start instruction", "project", projectID, "error", err)
					failedComponents = append(failedComponents, fmt.Sprintf("project_start:%s", projectID))
				}
			}
		}
	}

	// Check if there were any failures during initialization
	if len(failedComponents) > 0 {
		logger.Error("Some components or operations failed during initialization",
			"failed_count", len(failedComponents),
			"failed_items", failedComponents,
			"successful_instructions", instructionCount)
		return fmt.Errorf("initialization incomplete: %d failures occurred: %v", len(failedComponents), failedComponents)
	}

	// Update final version after all instructions are published
	_, err = im.setCurrentVersion(instructionCount)
	if err != nil {
		logger.Error("Failed to update final version after initialization", "error", err)
		return fmt.Errorf("failed to update final version: %w", err)
	}

	logger.Info("Leader instructions initialization completed successfully",
		"final_version", im.getCurrentVersionUnsafe(),
		"instruction_count", instructionCount)
	return nil
}

// GetActiveFollowers returns list of followers currently executing instructions
func (im *InstructionManager) GetActiveFollowers() ([]string, error) {
	pattern := "cluster:execution_flag:*"
	keys, err := common.RedisKeys(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to get execution flags: %w", err)
	}

	var activeFollowers []string
	for _, key := range keys {
		// Extract node ID from key
		parts := strings.Split(key, ":")
		if len(parts) >= 3 {
			nodeID := parts[2]
			// Skip leader node
			if nodeID != common.GetNodeID() {
				activeFollowers = append(activeFollowers, nodeID)
			}
		}
	}

	return activeFollowers, nil
}

// KickFollowerForResync kicks out a slow/stuck follower and marks it for full resync
func (im *InstructionManager) KickFollowerForResync(followerID string) error {
	if !common.IsCurrentNodeLeader() {
		return fmt.Errorf("only leader can kick followers")
	}

	// Clear the execution flag so leader thinks it's idle
	executionFlagKey := fmt.Sprintf("cluster:execution_flag:%s", followerID)
	if err := common.RedisDel(executionFlagKey); err != nil {
		logger.Error("Failed to clear execution flag for kicked follower", "follower_id", followerID, "error", err)
	}

	// Mark follower for full resync (24 hour TTL)
	resyncFlagKey := fmt.Sprintf("cluster:resync_required:%s", followerID)
	if _, err := common.RedisSet(resyncFlagKey, "kicked_for_slow_sync", 86400); err != nil {
		return fmt.Errorf("failed to set resync flag: %w", err)
	}

	logger.Info("Follower marked for full resync", "follower_id", followerID)
	return nil
}

// WaitForAllFollowersIdle waits for all followers to finish executing instructions
func (im *InstructionManager) WaitForAllFollowersIdle(timeout time.Duration) error {
	if !common.IsCurrentNodeLeader() {
		return fmt.Errorf("only leader can wait for followers")
	}

	deadline := time.Now().Add(timeout)
	checkInterval := 500 * time.Millisecond

	logger.Info("Waiting for all followers to become idle before compaction")

	for time.Now().Before(deadline) {
		activeFollowers, err := im.GetActiveFollowers()
		if err != nil {
			logger.Error("Failed to check active followers", "error", err)
			time.Sleep(checkInterval)
			continue
		}

		if len(activeFollowers) == 0 {
			logger.Info("All followers are idle, proceeding with compaction")
			return nil
		}

		time.Sleep(checkInterval)
	}

	activeFollowers, _ := im.GetActiveFollowers()
	return fmt.Errorf("timeout waiting for followers to become idle, still active: %v", activeFollowers)
}

func (im *InstructionManager) Stop() {
	// Stop the queue worker if it's running
	if im.queue != nil {
		logger.Info("Stopping instruction queue worker")
		close(im.queue)

		// Wait for worker to stop with timeout
		select {
		case <-im.workerStopped:
			logger.Info("Instruction queue worker stopped")
		case <-time.After(5 * time.Second):
			logger.Error("Timeout waiting for instruction queue worker to stop")
		}
	}

	// Only leader should clean up cluster instructions
	// Followers should not delete instructions as they are managed by leader
	if common.IsCurrentNodeLeader() {
		logger.Info("Leader cleaning up cluster instructions during shutdown")

		im.mu.RLock()
		currentVer := im.currentVersion
		im.mu.RUnlock()

		// Delete all instructions from 1 to currentVersion
		if currentVer > 0 {
			logger.Info("Deleting instructions", "count", currentVer)
			for v := int64(1); v <= currentVer; v++ {
				key := fmt.Sprintf("cluster:instruction:%d", v)
				_ = common.RedisDel(key)
			}
		}
		_ = common.RedisDel("cluster:leader_version")
		logger.Info("Leader cleanup completed")
	} else {
		logger.Info("Follower stopping instruction manager (not cleaning up cluster instructions)")
	}
}
