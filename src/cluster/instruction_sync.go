package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
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

// InstructionCompactionRule defines rules for instruction compaction
type InstructionCompactionRule struct {
	ComponentType string
	ComponentName string
	Operations    []string // operations that can be compacted
}

// InstructionManager manages version-based synchronization
type InstructionManager struct {
	currentVersion int64
	baseVersion    string
	mu             sync.RWMutex
	// Add compaction settings
	compactionEnabled   bool
	maxInstructions     int64 // trigger compaction when exceeding this number
	compactionThreshold int64 // minimum instructions before compaction
	// Add follower execution tracking
	executionFlagTTL int64 // TTL for execution flags in seconds
}

var GlobalInstructionManager *InstructionManager

// InitInstructionManager initializes the instruction manager
func InitInstructionManager() {
	GlobalInstructionManager = &InstructionManager{
		currentVersion:      0,
		baseVersion:         fmt.Sprintf("v%d", time.Now().Unix()),
		compactionEnabled:   true,
		maxInstructions:     1000, // compact when > 1000 instructions
		compactionThreshold: 100,  // don't compact if < 100 instructions
		executionFlagTTL:    30,   // 30 seconds TTL for execution flags
	}
}

// GetCurrentVersion returns current version string
func (im *InstructionManager) GetCurrentVersion() string {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return fmt.Sprintf("%s.%d", im.baseVersion, im.currentVersion)
}

// CompactInstructions performs instruction compaction to reduce Redis storage
// This method implements the algorithm you described:
// 1. Wait for all followers to finish executing
// 2. Set version to 0 (followers skip processing)
// 3. Analyze and remove redundant instructions
// 4. Increment version properly
func (im *InstructionManager) CompactInstructions() error {
	if !common.IsCurrentNodeLeader() {
		return fmt.Errorf("only leader can compact instructions")
	}

	logger.Info("Starting instruction compaction", "current_version", im.currentVersion)

	// Step 1: Wait for all followers to finish executing instructions
	if err := im.WaitForAllFollowersIdle(30 * time.Second); err != nil {
		logger.Warn("Some followers may still be executing, proceeding with caution", "error", err)
		// Continue with compaction but log the warning
	}

	im.mu.Lock()
	originalVersion := im.currentVersion
	im.mu.Unlock()

	// Step 2: Set version to 0 to signal followers to skip
	im.mu.Lock()
	im.currentVersion = 0
	im.mu.Unlock()

	// Notify followers that compaction is starting
	compactionSignal := map[string]interface{}{
		"action":           "compaction_start",
		"original_version": originalVersion,
		"timestamp":        time.Now().Unix(),
	}
	if data, err := json.Marshal(compactionSignal); err == nil {
		_ = common.RedisPublish("cluster:compaction", string(data))
	}

	// Step 2: Load all existing instructions
	instructions, err := im.loadAllInstructions(originalVersion)
	if err != nil {
		// Restore version on error
		im.currentVersion = originalVersion
		return fmt.Errorf("failed to load instructions: %w", err)
	}

	// Step 3: Perform compaction analysis
	compactedInstructions := im.analyzeAndCompact(instructions)

	// Step 4: Clear old instructions from Redis
	if err := im.clearOldInstructions(originalVersion); err != nil {
		logger.Warn("Failed to clear old instructions", "error", err)
	}

	// Step 5: Store compacted instructions with new version numbers (starting from 1)
	newVersion := int64(1)
	for _, instruction := range compactedInstructions {
		instruction.Version = newVersion
		instruction.Timestamp = time.Now().Unix()

		key := fmt.Sprintf("cluster:instruction:%d", newVersion)
		data, err := json.Marshal(instruction)
		if err != nil {
			logger.Error("Failed to marshal compacted instruction", "version", newVersion, "error", err)
			continue
		}

		if _, err := common.RedisSet(key, string(data), 86400); err != nil {
			logger.Error("Failed to store compacted instruction", "version", newVersion, "error", err)
			continue
		}

		newVersion++
	}

	// Step 6: Update current version
	im.mu.Lock()
	im.currentVersion = newVersion - 1
	im.mu.Unlock()

	// Step 7: Notify followers that compaction is complete
	compactionComplete := map[string]interface{}{
		"action":      "compaction_complete",
		"new_version": im.GetCurrentVersion(),
		"timestamp":   time.Now().Unix(),
	}
	if data, err := json.Marshal(compactionComplete); err == nil {
		_ = common.RedisPublish("cluster:compaction", string(data))
	}

	logger.Info("Instruction compaction completed",
		"original_instructions", len(instructions),
		"compacted_instructions", len(compactedInstructions),
		"new_version", im.GetCurrentVersion(),
		"compression_ratio", fmt.Sprintf("%.2f%%", float64(len(compactedInstructions))/float64(len(instructions))*100))

	return nil
}

// loadAllInstructions loads all instructions from Redis
func (im *InstructionManager) loadAllInstructions(maxVersion int64) ([]*Instruction, error) {
	var instructions []*Instruction

	for version := int64(1); version <= maxVersion; version++ {
		key := fmt.Sprintf("cluster:instruction:%d", version)
		data, err := common.RedisGet(key)
		if err != nil {
			logger.Debug("Instruction not found", "version", version, "error", err)
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

// analyzeAndCompact performs the core compaction logic
// Since leader has verify mechanism, runtime operations are already safe to group by component
// Only leader initialization needs special ordering (handled in InitializeLeaderInstructions)
func (im *InstructionManager) analyzeAndCompact(instructions []*Instruction) []*Instruction {
	// Group instructions by component - safe because:
	// 1. Leader initialization already handles proper dependency order
	// 2. Runtime operations are verified before submission, ensuring safety
	componentGroups := make(map[string][]*Instruction)

	for _, instruction := range instructions {
		key := fmt.Sprintf("%s:%s", instruction.ComponentType, instruction.ComponentName)
		componentGroups[key] = append(componentGroups[key], instruction)
	}

	var compactedInstructions []*Instruction

	// Process each component group
	for _, group := range componentGroups {
		compacted := im.compactComponentInstructions(group)
		compactedInstructions = append(compactedInstructions, compacted...)
	}

	// Sort by original timestamp to maintain order
	sort.Slice(compactedInstructions, func(i, j int) bool {
		return compactedInstructions[i].Timestamp < compactedInstructions[j].Timestamp
	})

	return compactedInstructions
}

// compactComponentInstructions compacts instructions for a single component to keep only the final state
// The goal is to eliminate all intermediate states and keep only what's needed to reach the final state
func (im *InstructionManager) compactComponentInstructions(instructions []*Instruction) []*Instruction {
	if len(instructions) <= 1 {
		return instructions
	}

	// Sort by version to process in chronological order
	sort.Slice(instructions, func(i, j int) bool {
		return instructions[i].Version < instructions[j].Version
	})

	// Analyze the final state by scanning all instructions
	var result []*Instruction

	// Find the final state for each aspect of the component
	var finalDefinition *Instruction   // Final content/configuration
	var finalControlState *Instruction // Final start/stop/restart state
	var hasDelete bool

	// Scan through all instructions to determine final state
	for _, inst := range instructions {
		switch inst.Operation {
		case "add", "update", "local_push", "push_change":
			// These operations define the component content/configuration
			finalDefinition = inst
			hasDelete = false // Reset delete flag if component is redefined

		case "delete":
			// Delete operation makes component non-existent
			hasDelete = true
			finalDefinition = nil
			finalControlState = nil

		case "start", "stop", "restart":
			// These operations control the component state (only for projects)
			if !hasDelete {
				finalControlState = inst
			}
		}
	}

	// Build result based on final state
	if hasDelete {
		// If component is deleted, only keep the delete instruction
		for _, inst := range instructions {
			if inst.Operation == "delete" {
				result = append(result, inst)
				break
			}
		}
	} else {
		// Component exists, keep final definition and final control state
		if finalDefinition != nil {
			result = append(result, finalDefinition)
		}

		if finalControlState != nil {
			result = append(result, finalControlState)
		}
	}

	// If no meaningful operations found, return the last instruction to be safe
	if len(result) == 0 {
		result = append(result, instructions[len(instructions)-1])
	}

	return result
}

// clearOldInstructions removes old instructions from Redis
func (im *InstructionManager) clearOldInstructions(maxVersion int64) error {
	var errors []string

	for version := int64(1); version <= maxVersion; version++ {
		key := fmt.Sprintf("cluster:instruction:%d", version)
		if err := common.RedisDel(key); err != nil {
			errors = append(errors, fmt.Sprintf("version %d: %v", version, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to clear some instructions: %s", strings.Join(errors, "; "))
	}

	return nil
}

// shouldTriggerCompaction checks if compaction should be triggered
// Only trigger compaction when instruction count exceeds threshold
func (im *InstructionManager) shouldTriggerCompaction() bool {
	if !im.compactionEnabled {
		return false
	}

	// Only trigger compaction when we have accumulated enough instructions
	// This avoids excessive compaction overhead while maintaining system efficiency
	return im.currentVersion >= im.compactionThreshold
}

// PublishInstruction publishes a new instruction (leader only)
func (im *InstructionManager) PublishInstruction(componentName, componentType, content, operation string, dependencies []string, metadata map[string]interface{}) error {
	if !common.IsCurrentNodeLeader() {
		return fmt.Errorf("only leader can publish instructions")
	}

	// Input validation
	if componentName == "" || componentType == "" || operation == "" {
		return fmt.Errorf("component name, type, and operation are required")
	}

	// Check if compaction should be triggered before publishing
	if im.shouldTriggerCompaction() {
		logger.Info("Triggering instruction compaction before publishing new instruction")
		if err := im.CompactInstructions(); err != nil {
			logger.Error("Failed to compact instructions", "error", err)
			// Continue with publishing even if compaction fails
		}
	}

	im.mu.Lock()
	im.currentVersion++
	version := im.currentVersion
	im.mu.Unlock()

	// Determine if this operation requires project restart
	requiresRestart := im.operationRequiresRestart(operation, componentType)

	instruction := Instruction{
		Version:         version,
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
	key := fmt.Sprintf("cluster:instruction:%d", version)
	data, err := json.Marshal(instruction)
	if err != nil {
		// Rollback version on failure
		im.mu.Lock()
		im.currentVersion--
		im.mu.Unlock()
		return fmt.Errorf("failed to marshal instruction: %w", err)
	}

	if _, err := common.RedisSet(key, string(data), 86400); err != nil {
		// Rollback version on failure
		im.mu.Lock()
		im.currentVersion--
		im.mu.Unlock()
		return fmt.Errorf("failed to store instruction: %w", err)
	}

	logger.Debug("Published instruction", "version", version, "component", componentName, "operation", operation, "requires_restart", requiresRestart)
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

// PublishComponentUpdate publishes component update instruction
func (im *InstructionManager) PublishComponentUpdate(componentType, componentName, content string, affectedProjects []string) error {
	metadata := map[string]interface{}{
		"affected_projects": affectedProjects,
	}
	return im.PublishInstruction(componentName, componentType, content, "update", affectedProjects, metadata)
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
	if !common.IsCurrentNodeLeader() {
		return fmt.Errorf("only leader can initialize instructions")
	}

	logger.Info("Initializing leader instructions")

	// Keep version at 0 during initialization so followers skip processing
	im.mu.Lock()
	im.currentVersion = 0
	im.mu.Unlock()

	var instructionCount int64 = 0

	// Defer version restoration
	defer func() {
		im.mu.Lock()
		// Set version to the number of instructions we've created
		im.currentVersion = instructionCount
		im.mu.Unlock()
		logger.Info("Leader instructions initialized", "version", im.GetCurrentVersion())
	}()

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
	common.GlobalMu.RLock()
	if common.AllInputsRawConfig != nil {
		for inputID, config := range common.AllInputsRawConfig {
			if err := publishInstructionDirectly(inputID, "input", config, "add", nil, nil); err != nil {
				logger.Error("Failed to publish input add instruction", "input", inputID, "error", err)
			}
		}
	}
	common.GlobalMu.RUnlock()

	// 2. Add all outputs (projects depend on outputs)
	common.GlobalMu.RLock()
	if common.AllOutputsRawConfig != nil {
		for outputID, config := range common.AllOutputsRawConfig {
			if err := publishInstructionDirectly(outputID, "output", config, "add", nil, nil); err != nil {
				logger.Error("Failed to publish output add instruction", "output", outputID, "error", err)
			}
		}
	}
	common.GlobalMu.RUnlock()

	// 3. Add all plugins (rulesets may depend on plugins)
	common.GlobalMu.RLock()
	if common.AllPluginsRawConfig != nil {
		for pluginID, config := range common.AllPluginsRawConfig {
			if err := publishInstructionDirectly(pluginID, "plugin", config, "add", nil, nil); err != nil {
				logger.Error("Failed to publish plugin add instruction", "plugin", pluginID, "error", err)
			}
		}
	}
	common.GlobalMu.RUnlock()

	// 4. Add all rulesets (projects depend on rulesets)
	common.GlobalMu.RLock()
	if common.AllRulesetsRawConfig != nil {
		for rulesetID, config := range common.AllRulesetsRawConfig {
			if err := publishInstructionDirectly(rulesetID, "ruleset", config, "add", nil, nil); err != nil {
				logger.Error("Failed to publish ruleset add instruction", "ruleset", rulesetID, "error", err)
			}
		}
	}
	common.GlobalMu.RUnlock()

	// 5. Add all projects LAST (projects depend on all above components)
	common.GlobalMu.RLock()
	if common.AllProjectRawConfig != nil {
		for projectID, config := range common.AllProjectRawConfig {
			if err := publishInstructionDirectly(projectID, "project", config, "add", nil, nil); err != nil {
				logger.Error("Failed to publish project add instruction", "project", projectID, "error", err)
			}
		}
	}
	common.GlobalMu.RUnlock()

	// 6. Start running projects (只有leader当前运行的项目才发送启动指令)
	if runningProjects, err := common.RedisHGetAll("cluster:proj_states:" + common.Config.LocalIP); err == nil {
		for projectID, status := range runningProjects {
			if status == "running" {
				if err := publishInstructionDirectly(projectID, "project", "", "start", nil, nil); err != nil {
					logger.Error("Failed to publish project start instruction", "project", projectID, "error", err)
				}
			}
		}
	}

	// Update the final version count - this will be handled by the defer function

	return nil
}

// SyncInstructions syncs instructions from a specific version to target version (follower only)
func (im *InstructionManager) SyncInstructions(fromVersion, toVersion string) error {
	if common.IsCurrentNodeLeader() {
		return fmt.Errorf("leader doesn't need to sync instructions")
	}

	// Set execution flag to indicate this follower is executing instructions
	if err := im.SetFollowerExecutionFlag(common.GetNodeID()); err != nil {
		logger.Warn("Failed to set execution flag", "error", err)
	}

	// Ensure flag is cleared when done (with defer for safety)
	defer func() {
		if err := im.ClearFollowerExecutionFlag(common.GetNodeID()); err != nil {
			logger.Warn("Failed to clear execution flag", "error", err)
		}
	}()

	// Parse version
	parts := strings.Split(fromVersion, ".")
	if len(parts) != 2 {
		return fmt.Errorf("invalid version format: %s", fromVersion)
	}

	startVersion, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid version number: %s", parts[1])
	}

	// Parse target version (leader version)
	leaderParts := strings.Split(toVersion, ".")
	if len(leaderParts) != 2 {
		return fmt.Errorf("invalid target version format: %s", toVersion)
	}

	endVersion, err := strconv.ParseInt(leaderParts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid target version number: %s", leaderParts[1])
	}

	// Track successfully applied instructions
	var lastSuccessfulVersion int64 = startVersion

	// Sync instructions one by one
	for version := startVersion + 1; version <= endVersion; version++ {
		// Skip version 0 (compaction in progress signal - followers should ignore)
		if version == 0 {
			logger.Info("Skipping version 0 (compaction in progress)")
			continue
		}

		// Refresh execution flag during long operations
		if err := im.RefreshFollowerExecutionFlag(common.GetNodeID()); err != nil {
			logger.Warn("Failed to refresh execution flag", "error", err)
		}

		// Apply instruction and track success
		if err := im.applyInstruction(version); err != nil {
			logger.Error("Failed to apply instruction", "version", version, "error", err)

			// Don't update version if instruction failed
			// Only update to the last successful version
			if lastSuccessfulVersion > startVersion {
				im.mu.Lock()
				im.currentVersion = lastSuccessfulVersion
				im.baseVersion = leaderParts[0]
				im.mu.Unlock()

				logger.Info("Updated version to last successful instruction",
					"from", fromVersion,
					"to", fmt.Sprintf("%s.%d", leaderParts[0], lastSuccessfulVersion),
					"failed_at", version)
			}

			return fmt.Errorf("instruction sync failed at version %d: %w", version, err)
		}

		// Mark this version as successfully applied
		lastSuccessfulVersion = version
	}

	// Update local version only after all instructions are successfully applied
	im.mu.Lock()
	im.currentVersion = endVersion
	im.baseVersion = leaderParts[0]
	im.mu.Unlock()

	logger.Info("Instructions synced successfully", "from", fromVersion, "to", toVersion)
	return nil
}

// createComponentInstance creates actual component instances from configuration
func (im *InstructionManager) createComponentInstance(componentType, componentName, content string) error {
	switch componentType {
	case "input":
		// Import the input package at the top if not already imported
		inp, err := input.NewInput("", content, componentName)
		if err != nil {
			return fmt.Errorf("failed to create input instance %s: %w", componentName, err)
		}
		project.GlobalProject.Inputs[componentName] = inp
		logger.Debug("Created input instance", "name", componentName)

	case "output":
		// Import the output package at the top if not already imported
		out, err := output.NewOutput("", content, componentName)
		if err != nil {
			return fmt.Errorf("failed to create output instance %s: %w", componentName, err)
		}
		project.GlobalProject.Outputs[componentName] = out
		logger.Debug("Created output instance", "name", componentName)

	case "ruleset":
		// Import the rules_engine package at the top if not already imported
		rs, err := rules_engine.NewRuleset("", content, componentName)
		if err != nil {
			return fmt.Errorf("failed to create ruleset instance %s: %w", componentName, err)
		}
		project.GlobalProject.Rulesets[componentName] = rs
		logger.Debug("Created ruleset instance", "name", componentName)

	case "project":
		// For projects, we create the project instance
		proj, err := project.NewProject("", content, componentName)
		if err != nil {
			return fmt.Errorf("failed to create project instance %s: %w", componentName, err)
		}
		project.GlobalProject.Projects[componentName] = proj
		logger.Debug("Created project instance", "name", componentName)

	case "plugin":
		// For plugins, we just store the content as plugins are handled differently
		// Import the plugin package at the top if not already imported
		err := plugin.NewPlugin("", content, componentName, plugin.YAEGI_PLUGIN)
		if err != nil {
			return fmt.Errorf("failed to create plugin instance %s: %w", componentName, err)
		}
		logger.Debug("Created plugin instance", "name", componentName)

	default:
		return fmt.Errorf("unsupported component type: %s", componentType)
	}

	return nil
}

// deleteComponentInstance deletes actual component instances
func (im *InstructionManager) deleteComponentInstance(componentType, componentName string) error {
	switch componentType {
	case "input":
		delete(project.GlobalProject.Inputs, componentName)
		logger.Debug("Deleted input instance", "name", componentName)

	case "output":
		delete(project.GlobalProject.Outputs, componentName)
		logger.Debug("Deleted output instance", "name", componentName)

	case "ruleset":
		delete(project.GlobalProject.Rulesets, componentName)
		logger.Debug("Deleted ruleset instance", "name", componentName)

	case "project":
		if proj, exists := project.GlobalProject.Projects[componentName]; exists {
			// Stop the project first if it's running
			if proj.Status == project.ProjectStatusRunning {
				proj.Stop()
			}
		}
		delete(project.GlobalProject.Projects, componentName)
		logger.Debug("Deleted project instance", "name", componentName)

	case "plugin":
		// For plugins, we need to handle differently based on the plugin system
		// This might need specific plugin cleanup logic
		logger.Debug("Deleted plugin instance", "name", componentName)

	default:
		return fmt.Errorf("unsupported component type: %s", componentType)
	}

	return nil
}

// updateComponentInstance updates existing component instances with new configuration
func (im *InstructionManager) updateComponentInstance(componentType, componentName, content string) error {
	// For updates, we delete the old instance and create a new one
	if err := im.deleteComponentInstance(componentType, componentName); err != nil {
		logger.Warn("Failed to delete old component instance during update", "type", componentType, "name", componentName, "error", err)
	}

	return im.createComponentInstance(componentType, componentName, content)
}

// applyInstruction applies a single instruction
func (im *InstructionManager) applyInstruction(version int64) error {
	key := fmt.Sprintf("cluster:instruction:%d", version)
	data, err := common.RedisGet(key)
	if err != nil {
		return fmt.Errorf("failed to get instruction %d: %w", version, err)
	}

	var instruction Instruction
	if err := json.Unmarshal([]byte(data), &instruction); err != nil {
		return fmt.Errorf("failed to unmarshal instruction %d: %w", version, err)
	}

	logger.Debug("Applying instruction", "version", version, "component", instruction.ComponentName, "operation", instruction.Operation)

	switch instruction.Operation {
	case "add":
		// First store config in memory
		if err := common.GlobalComponentOperations.CreateComponentMemoryOnly(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			return err
		}
		// Then create actual component instances for follower
		return im.createComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content)
	case "delete":
		// First delete component instance
		if err := im.deleteComponentInstance(instruction.ComponentType, instruction.ComponentName); err != nil {
			logger.Warn("Failed to delete component instance", "type", instruction.ComponentType, "name", instruction.ComponentName, "error", err)
		}
		// Then remove from memory
		return common.GlobalComponentOperations.DeleteComponentMemoryOnly(instruction.ComponentType, instruction.ComponentName)
	case "update":
		// First update config in memory
		if err := common.GlobalComponentOperations.UpdateComponentMemoryOnly(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			return err
		}
		// Then update component instance
		return im.updateComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content)
	case "local_push":
		// First store config in memory
		if err := common.GlobalComponentOperations.CreateComponentMemoryOnly(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			return err
		}
		// Then create actual component instances for follower
		return im.createComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content)
	case "push_change":
		// First store config in memory
		if err := common.GlobalComponentOperations.CreateComponentMemoryOnly(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			return err
		}
		// Then create actual component instances for follower
		return im.createComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content)
	case "start":
		if instruction.ComponentType == "project" {
			if globalProjectCmdHandler != nil {
				// Follower nodes should not record operations when executing cluster instructions
				// as the leader has already recorded these operations
				return globalProjectCmdHandler.ExecuteCommandWithOptions(instruction.ComponentName, "start", false)
			}
			return fmt.Errorf("project handler not initialized")
		}
	case "stop":
		if instruction.ComponentType == "project" {
			if globalProjectCmdHandler != nil {
				// Follower nodes should not record operations when executing cluster instructions
				// as the leader has already recorded these operations
				return globalProjectCmdHandler.ExecuteCommandWithOptions(instruction.ComponentName, "stop", false)
			}
			return fmt.Errorf("project handler not initialized")
		}
	case "restart":
		if instruction.ComponentType == "project" {
			// Check if this project is currently running before restarting
			// This ensures we only restart projects that were actually running
			common.GlobalMu.RLock()
			proj, exists := project.GlobalProject.Projects[instruction.ComponentName]
			common.GlobalMu.RUnlock()

			if !exists {
				logger.Warn("Project not found for restart", "project", instruction.ComponentName)
				return fmt.Errorf("project %s not found", instruction.ComponentName)
			}

			// Only restart if the project is currently running
			if proj.Status != project.ProjectStatusRunning {
				logger.Debug("Skipping restart for non-running project", "project", instruction.ComponentName, "status", proj.Status)
				return nil
			}

			logger.Debug("Restarting running project", "project", instruction.ComponentName)

			if globalProjectCmdHandler != nil {
				// Follower nodes should not record operations when executing cluster instructions
				// as the leader has already recorded these operations
				return globalProjectCmdHandler.ExecuteCommandWithOptions(instruction.ComponentName, "restart", false)
			}
			return fmt.Errorf("project handler not initialized")
		}
	default:
		return fmt.Errorf("unknown operation: %s", instruction.Operation)
	}

	return nil
}

// SetFollowerExecutionFlag sets a flag indicating follower is executing instructions
func (im *InstructionManager) SetFollowerExecutionFlag(nodeID string) error {
	key := fmt.Sprintf("cluster:execution_flag:%s", nodeID)
	_, err := common.RedisSet(key, "executing", int(im.executionFlagTTL))
	if err != nil {
		return fmt.Errorf("failed to set execution flag: %w", err)
	}
	return nil
}

// ClearFollowerExecutionFlag clears the execution flag for a follower
func (im *InstructionManager) ClearFollowerExecutionFlag(nodeID string) error {
	key := fmt.Sprintf("cluster:execution_flag:%s", nodeID)
	return common.RedisDel(key)
}

// RefreshFollowerExecutionFlag refreshes the TTL of execution flag
func (im *InstructionManager) RefreshFollowerExecutionFlag(nodeID string) error {
	key := fmt.Sprintf("cluster:execution_flag:%s", nodeID)
	_, err := common.RedisSet(key, "executing", int(im.executionFlagTTL))
	return err
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

		logger.Debug("Waiting for followers to finish", "active_followers", activeFollowers)
		time.Sleep(checkInterval)
	}

	activeFollowers, _ := im.GetActiveFollowers()
	return fmt.Errorf("timeout waiting for followers to become idle, still active: %v", activeFollowers)
}
