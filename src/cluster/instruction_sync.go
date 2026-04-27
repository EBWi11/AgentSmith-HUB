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
	Version       int64                  `json:"version"`
	ComponentName string                 `json:"component_name"`
	ComponentType string                 `json:"component_type"` // project, input, output, ruleset, plugin
	Content       string                 `json:"content"`
	Operation     string                 `json:"operation"`    // add, delete, start, restart, stop, update, local_push, push_change
	Dependencies  []string               `json:"dependencies"` // affected projects that need restart
	Metadata      map[string]interface{} `json:"metadata"`     // additional operation metadata
	Timestamp     int64                  `json:"timestamp"`
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

type instructionSnapshot struct {
	version     int64
	raw         string
	instruction *Instruction
}

type compactionWrite struct {
	version int64
	value   string
}

// InstructionManager manages version-based synchronization
type InstructionManager struct {
	currentVersion        int64
	baseVersion           string
	mu                    sync.RWMutex
	maxInstructions       int64 // trigger compaction when enough new instructions accumulated
	lastCompactionVersion int64
	queue                 chan *PendingInstruction
	workerStopped         chan struct{}
	once                  sync.Once
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
		currentVersion:        0,                   // Start with version 0 (temporary state)
		baseVersion:           generateSessionID(), // Session identifier (6-char random string)
		maxInstructions:       2000,                // compact after 2000 new instructions
		lastCompactionVersion: 0,
		queue:                 make(chan *PendingInstruction, 1000), // buffer for 1000 pending instructions
		workerStopped:         make(chan struct{}),
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

	_, err := clusterRedisSet("cluster:leader_version", im.getCurrentVersionUnsafe(), 0)
	if err != nil {
		im.currentVersion = ori
		return 0, fmt.Errorf("failed to update cluster version in Redis: %w", err)
	}

	return ori, nil
}

// loadAllInstructions loads all instructions from Redis and fails fast if history is incomplete.
func (im *InstructionManager) loadAllInstructions(maxVersion int64) ([]instructionSnapshot, error) {
	instructions := make([]instructionSnapshot, 0, maxVersion)

	for version := int64(1); version <= maxVersion; version++ {
		key := fmt.Sprintf("cluster:instruction:%d", version)
		data, err := clusterRedisGet(key)
		if err != nil {
			return nil, fmt.Errorf("failed to load instruction v%d: %w", version, err)
		}

		var instruction Instruction
		if err := json.Unmarshal([]byte(data), &instruction); err != nil {
			return nil, fmt.Errorf("failed to unmarshal instruction v%d: %w", version, err)
		}

		instructions = append(instructions, instructionSnapshot{
			version:     version,
			raw:         data,
			instruction: &instruction,
		})
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

	instruction := Instruction{
		ComponentName: componentName,
		ComponentType: componentType,
		Content:       content,
		Operation:     operation,
		Dependencies:  dependencies,
		Metadata:      metadata,
		Timestamp:     time.Now().Unix(),
	}

	var err error
	if im.shouldCompactLocked() {
		err = im.CompactAndSaveInstructions(&instruction)
	} else {
		err = im.appendInstructionLocked(&instruction)
	}
	if err != nil {
		logger.Error("Failed to compact and save instructions", "error", err)

		// Record failed instruction
		clusterRecordInstruction(
			common.OpTypeInstructionPublish,
			operation,
			componentName,
			componentType,
			"failed",
			err.Error(),
			content,
			map[string]interface{}{
				"version":      im.currentVersion,
				"dependencies": dependencies,
				"metadata":     metadata,
				"role":         "leader",
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
		_ = clusterRedisPublish("cluster:sync_command", string(data))
	}
	logger.Info("Instruction published", "version", im.currentVersion, "component", componentName, "operation", operation)

	// Record successful instruction
	clusterRecordInstruction(
		common.OpTypeInstructionPublish,
		operation,
		componentName,
		componentType,
		"success",
		"",
		content,
		map[string]interface{}{
			"version":      im.currentVersion,
			"dependencies": dependencies,
			"metadata":     metadata,
			"role":         "leader",
		},
	)

	return nil
}

func (im *InstructionManager) shouldCompactLocked() bool {
	if im.maxInstructions <= 0 {
		return false
	}
	return im.currentVersion-im.lastCompactionVersion >= im.maxInstructions
}

func (im *InstructionManager) appendInstructionLocked(instruction *Instruction) error {
	nextVersion := im.currentVersion + 1
	instruction.Version = nextVersion

	data, err := json.Marshal(instruction)
	if err != nil {
		return fmt.Errorf("failed to marshal instruction: %w", err)
	}

	key := fmt.Sprintf("cluster:instruction:%d", nextVersion)
	if _, err := clusterRedisSet(key, string(data), 0); err != nil {
		return fmt.Errorf("failed to store instruction: %w", err)
	}

	if _, err := im.setCurrentVersion(nextVersion); err != nil {
		_ = clusterRedisDel(key)
		return fmt.Errorf("failed to advance leader version: %w", err)
	}

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
	snapshots, err := im.loadAllInstructions(originalVersion)
	if err != nil {
		_, _ = im.setCurrentVersion(originalVersion)
		return fmt.Errorf("failed to load instructions: %w", err)
	}

	instructions := make([]*Instruction, 0, len(snapshots)+1)
	for _, snapshot := range snapshots {
		instructions = append(instructions, snapshot.instruction)
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

	writes := make([]compactionWrite, 0, len(delInstructions)+1)

	for i, instruction := range instructions {
		instruction.Version = int64(i + 1)

		if delInstructions[i] {
			writes = append(writes, compactionWrite{
				version: instruction.Version,
				value:   GetDeletedIntentionsString(),
			})
		}

		if i+1 == instructionsLen {
			data, err := json.Marshal(instruction)
			if err != nil {
				rollbackErr := im.rollbackCompactionLocked(originalVersion, nil, snapshots, fmt.Errorf("failed to marshal compacted instruction v%d: %w", instruction.Version, err))
				if rollbackErr != nil {
					return rollbackErr
				}
				return fmt.Errorf("failed to marshal compacted instruction v%d: %w", instruction.Version, err)
			}

			writes = append(writes, compactionWrite{
				version: instruction.Version,
				value:   string(data),
			})
		}
	}

	appliedWrites := make([]compactionWrite, 0, len(writes))
	for _, write := range writes {
		key := fmt.Sprintf("cluster:instruction:%d", write.version)
		err := clusterRetryWithExponentialBackoff(func() error {
			_, e := clusterRedisSet(key, write.value, 0)
			return e
		}, 3, 100*time.Millisecond)
		if err != nil {
			rollbackErr := im.rollbackCompactionLocked(originalVersion, appliedWrites, snapshots, fmt.Errorf("failed to store compacted instruction v%d after retries: %w", write.version, err))
			if rollbackErr != nil {
				return rollbackErr
			}
			return fmt.Errorf("failed to store compacted instruction v%d after retries: %w", write.version, err)
		}
		appliedWrites = append(appliedWrites, write)
	}

	_, err = im.setCurrentVersion(int64(len(instructions)))
	if err != nil {
		rollbackErr := im.rollbackCompactionLocked(originalVersion, appliedWrites, snapshots, fmt.Errorf("failed to update version after compaction: %w", err))
		if rollbackErr != nil {
			return rollbackErr
		}
		return fmt.Errorf("failed to update version after compaction: %w", err)
	}

	im.lastCompactionVersion = im.currentVersion

	logger.Info("Compaction completed successfully",
		"original_version", originalVersion,
		"new_version", int64(len(instructions)),
		"old_instructions_count", originalLen)

	return nil
}

func (im *InstructionManager) rollbackCompactionLocked(originalVersion int64, appliedWrites []compactionWrite, snapshots []instructionSnapshot, cause error) error {
	logger.Error("Compaction failed, rolling back",
		"original_version", originalVersion,
		"applied_writes", len(appliedWrites),
		"error", cause)

	originalByVersion := make(map[int64]string, len(snapshots))
	for _, snapshot := range snapshots {
		originalByVersion[snapshot.version] = snapshot.raw
	}

	var restoreFailures []string
	for i := len(appliedWrites) - 1; i >= 0; i-- {
		write := appliedWrites[i]
		key := fmt.Sprintf("cluster:instruction:%d", write.version)

		if write.version > originalVersion {
			err := clusterRetryWithExponentialBackoff(func() error {
				return clusterRedisDel(key)
			}, 3, 100*time.Millisecond)
			if err != nil {
				restoreFailures = append(restoreFailures, fmt.Sprintf("delete v%d: %v", write.version, err))
			}
			continue
		}

		originalRaw, exists := originalByVersion[write.version]
		if !exists {
			restoreFailures = append(restoreFailures, fmt.Sprintf("missing original snapshot for v%d", write.version))
			continue
		}

		err := clusterRetryWithExponentialBackoff(func() error {
			_, e := clusterRedisSet(key, originalRaw, 0)
			return e
		}, 3, 100*time.Millisecond)
		if err != nil {
			restoreFailures = append(restoreFailures, fmt.Sprintf("restore v%d: %v", write.version, err))
		}
	}

	_, versionErr := im.setCurrentVersion(originalVersion)

	if len(restoreFailures) > 0 && versionErr != nil {
		return fmt.Errorf("%w (history rollback failed: %s; version rollback failed: %v)",
			cause, strings.Join(restoreFailures, "; "), versionErr)
	}
	if len(restoreFailures) > 0 {
		return fmt.Errorf("%w (history rollback failed: %s)", cause, strings.Join(restoreFailures, "; "))
	}
	if versionErr != nil {
		return fmt.Errorf("%w (version rollback failed: %v)", cause, versionErr)
	}

	return cause
}

func (im *InstructionManager) PublishInstruction(componentName, componentType, content, operation string, dependencies []string, metadata map[string]interface{}) (err error) {
	if !common.IsCurrentNodeLeader() {
		return fmt.Errorf("only leader can initialize instructions")
	}

	if componentName == "" || componentType == "" || operation == "" {
		return fmt.Errorf("component name, type, and operation are required")
	}

	if im.queue == nil {
		return fmt.Errorf("instruction queue not initialized")
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

	defer func() {
		if r := recover(); r != nil {
			logger.Error("Instruction queue is closed", "panic", r)
			err = fmt.Errorf("instruction queue is closed")
		}
	}()

	im.queue <- pending
	return <-resultChan
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
	oldVersionStr, err := clusterRedisGet("cluster:leader_version")
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
						if delErr := clusterRedisDel(key); delErr != nil {
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
						if delErr := clusterRedisDel(key); delErr == nil {
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
		instruction := Instruction{
			Version:       instructionCount, // Next version number
			ComponentName: componentName,
			ComponentType: componentType,
			Content:       content,
			Operation:     operation,
			Dependencies:  dependencies,
			Metadata:      metadata,
			Timestamp:     time.Now().Unix(),
		}

		// Store instruction in Redis
		key := fmt.Sprintf("cluster:instruction:%d", instructionCount)
		data, err := json.Marshal(instruction)
		if err != nil {
			return fmt.Errorf("failed to marshal instruction: %w", err)
		}

		if _, err := clusterRedisSet(key, string(data), 0); err != nil {
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

	userIntentions, err := common.GetAllProjectUserIntentions()
	if err != nil {
		return fmt.Errorf("failed to load project user intentions during initialization: %w", err)
	}
	for projectID, wantRunning := range userIntentions {
		if wantRunning {
			if err := publishInstructionDirectly(projectID, "project", "", "start", nil, nil); err != nil {
				logger.Error("Failed to publish project start instruction", "project", projectID, "error", err)
				failedComponents = append(failedComponents, fmt.Sprintf("project_start:%s", projectID))
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

	im.lastCompactionVersion = instructionCount

	logger.Info("Leader instructions initialization completed successfully",
		"final_version", im.getCurrentVersionUnsafe(),
		"instruction_count", instructionCount)
	return nil
}

// GetActiveFollowers returns list of followers currently executing instructions
func (im *InstructionManager) GetActiveFollowers() ([]string, error) {
	pattern := "cluster:execution_flag:*"
	keys, err := clusterRedisKeys(pattern)
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
	if err := clusterRedisDel(executionFlagKey); err != nil {
		logger.Error("Failed to clear execution flag for kicked follower", "follower_id", followerID, "error", err)
	}

	// Mark follower for full resync (24 hour TTL)
	resyncFlagKey := fmt.Sprintf("cluster:resync_required:%s", followerID)
	if _, err := clusterRedisSet(resyncFlagKey, "kicked_for_slow_sync", 86400); err != nil {
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
				_ = clusterRedisDel(key)
			}
		}
		_ = clusterRedisDel("cluster:leader_version")
		logger.Info("Leader cleanup completed")
	} else {
		logger.Info("Follower stopping instruction manager (not cleaning up cluster instructions)")
	}
}
