package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"crypto/rand"
	"encoding/json"
	"fmt"
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

// InstructionCompactionRule defines rules for instruction compaction
type InstructionCompactionRule struct {
	ComponentType string
	ComponentName string
	Operations    []string // operations that can be compacted
}

// InstructionManager manages version-based synchronization
type InstructionManager struct {
	currentVersion  int64
	baseVersion     string
	mu              sync.RWMutex
	maxInstructions int64 // trigger compaction when exceeding this number
}

func (im *InstructionManager) NewInstructionManagerFollower() *InstructionManager {
	return &InstructionManager{
		currentVersion: 0,
		baseVersion:    "0",
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
		currentVersion:  0,                   // Start with version 0 (temporary state)
		baseVersion:     generateSessionID(), // Session identifier (6-char random string)
		maxInstructions: 2000,                // compact when > 1000 instructions
	}
}

// GetCurrentVersion returns current version string
func (im *InstructionManager) GetCurrentVersion() string {
	return fmt.Sprintf("%s.%d", im.baseVersion, im.currentVersion)
}

func (im *InstructionManager) setCurrentVersion(veresion int64) (int64, error) {
	ori := im.currentVersion
	im.currentVersion = veresion

	_, err := common.RedisSet("cluster:leader_version", im.GetCurrentVersion(), 0)
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

func (im *InstructionManager) CompactAndSaveInstructions(new *Instruction) error {
	if err := im.WaitForAllFollowersIdle(90 * time.Second); err != nil {
		logger.Error("Some followers may still be executing, proceeding with caution", "error", err)
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

	instructions = append(instructions, new)
	for i, ii := range instructions {
		for i2, ii2 := range instructions {
			if i != i2 {
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
	}

	for i, _ := range instructions {
		if !delInstructions[i] && instructions[i].Operation == "restart" {
			instructions[i].Operation = "start"
		}
	}

	for i, instruction := range instructions {
		version := i + 1
		key := fmt.Sprintf("cluster:instruction:%d", version)

		if delInstructions[i] {
			if _, err := common.RedisSet(key, "DELETED", 0); err != nil {
				logger.Error("Failed to store compacted instruction", "version", version, "error", err)
				continue
			}
		} else {
			data, _ := json.Marshal(instruction)
			if _, err := common.RedisSet(key, string(data), 0); err != nil {
				logger.Error("Failed to store compacted instruction", "version", version, "error", err)
				continue
			}
		}
	}

	_, err = im.setCurrentVersion(int64(len(instructions)))
	if err != nil {
		return err
	}
	return nil
}

func (im *InstructionManager) PublishInstruction(componentName, componentType, content, operation string, dependencies []string, metadata map[string]interface{}) error {
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
	}

	publishComplete := map[string]interface{}{
		"action":         "publish_complete",
		"leader_version": im.GetCurrentVersion(),
		"timestamp":      time.Now().Unix(),
	}

	if data, err := json.Marshal(publishComplete); err == nil {
		_ = common.RedisPublish("cluster:sync_command", string(data))
	}
	logger.Info("Instruction published successfully", "version", im.currentVersion, "component", componentName, "operation", operation, "requires_restart", requiresRestart)

	if err == nil {
		common.RecordClusterInstruction(
			common.OpTypeInstructionPublish,
			operation,     // instruction type
			componentName, // component ID
			componentType, // component type
			"success",     // status
			"",            // no error
			content,       // instruction content
			map[string]interface{}{ // details
				"version":          im.currentVersion,
				"requires_restart": requiresRestart,
				"dependencies":     dependencies,
				"metadata":         metadata,
				"role":             "leader",
			},
		)
	} else {
		common.RecordClusterInstruction(
			common.OpTypeInstructionPublish,
			operation,     // instruction type
			componentName, // component ID
			componentType, // component type
			"failed",      // status
			err.Error(),   // no error
			content,       // instruction content
			map[string]interface{}{ // details
				"version":          im.currentVersion,
				"requires_restart": requiresRestart,
				"dependencies":     dependencies,
				"metadata":         metadata,
				"role":             "leader",
			},
		)
	}

	return nil
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

	for _, projectName := range projectNames {
		if err := im.PublishInstruction(projectName, "project", "", "restart", nil, metadata); err != nil {
			return err
		}
	}
	return nil
}

// InitializeLeaderInstructions creates initial instructions for all components (leader only)
func (im *InstructionManager) InitializeLeaderInstructions() error {
	im.mu.Lock()
	defer im.mu.Unlock()

	if !common.IsCurrentNodeLeader() {
		return fmt.Errorf("only leader can initialize instructions")
	}

	logger.Info("Initializing leader instructions")

	_, err := im.setCurrentVersion(0)
	if err != nil {
		err = fmt.Errorf("Failed to write leader version to Redis during initialization", "error", err)
		return err
	}

	var instructionCount int64 = 0

	// Helper function to publish instruction without triggering compaction
	publishInstructionDirectly := func(componentName, componentType, content, operation string, dependencies []string, metadata map[string]interface{}) error {
		instructionCount++

		// Determine if this operation requires project restart
		requiresRestart := im.operationRequiresRestart(operation, componentType)

		instruction := Instruction{
			Version:         instructionCount, // Starts from 1, not 0
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

		if _, err := common.RedisSet(key, string(data), 86400); err != nil {
			return fmt.Errorf("failed to store instruction: %w", err)
		}

		logger.Debug("Published initialization instruction", "version", instructionCount, "component", componentName, "operation", operation)
		return nil
	}

	// 1. Add all inputs first (projects depend on inputs)
	common.ForEachRawConfig("input", func(inputID, config string) bool {
		if err := publishInstructionDirectly(inputID, "input", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish input add instruction", "input", inputID, "error", err)
		}
		return true
	})

	// 2. Add all outputs (projects depend on outputs)
	common.ForEachRawConfig("output", func(outputID, config string) bool {
		if err := publishInstructionDirectly(outputID, "output", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish output add instruction", "output", outputID, "error", err)
		}
		return true
	})

	// 3. Add all plugins (rulesets may depend on plugins)
	common.ForEachRawConfig("plugin", func(pluginID, config string) bool {
		if err := publishInstructionDirectly(pluginID, "plugin", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish plugin add instruction", "plugin", pluginID, "error", err)
		}
		return true
	})

	// 4. Add all rulesets (projects depend on rulesets)
	common.ForEachRawConfig("ruleset", func(rulesetID, config string) bool {
		if err := publishInstructionDirectly(rulesetID, "ruleset", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish ruleset add instruction", "ruleset", rulesetID, "error", err)
		}
		return true
	})

	// 5. Add all projects LAST (projects depend on all above components)
	common.ForEachRawConfig("project", func(projectID, config string) bool {
		if err := publishInstructionDirectly(projectID, "project", config, "add", nil, nil); err != nil {
			logger.Error("Failed to publish project add instruction", "project", projectID, "error", err)
		}
		return true
	})

	// 6. Start running projects
	logger.Info("Reading project user intentions from Redis to send start instructions...")

	if userIntentions, err := common.GetAllProjectUserIntentions(); err == nil {
		for projectID, wantRunning := range userIntentions {
			if wantRunning {
				if err := publishInstructionDirectly(projectID, "project", "", "start", nil, nil); err != nil {
					logger.Error("Failed to publish project start instruction", "project", projectID, "error", err)
				} else {
					logger.Info("Published project start instruction", "project", projectID)
				}
			}
		}
	}

	// Update final version after all instructions are published
	_, err = im.setCurrentVersion(instructionCount)
	if err != nil {
		logger.Error("Failed to update final version after initialization", "error", err)
		return fmt.Errorf("failed to update final version: %w", err)
	}

	logger.Info("Leader instructions initialization completed", "final_version", im.GetCurrentVersion(), "instruction_count", instructionCount)
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
			logger.Warn("Failed to check active followers", "error", err)
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
	is, _ := im.loadAllInstructions(im.maxInstructions)
	for i := range is {
		key := fmt.Sprintf("cluster:instruction:%d", i)
		_ = common.RedisDel(key)
	}
	_ = common.RedisDel(fmt.Sprintf("cluster:instruction:%d", len(is)))
	_ = common.RedisDel("cluster:leader_version")
}
