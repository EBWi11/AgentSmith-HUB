package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SyncListener handles sync commands for followers
type SyncListener struct {
	nodeID           string
	stopChan         chan struct{}
	currentVersion   int64
	baseVersion      string
	executionFlagTTL time.Duration // TTL for execution flag, default 5 minutes
	mu               sync.RWMutex
}

var GlobalSyncListener *SyncListener

// InitSyncListener initializes the sync listener
func InitSyncListener(nodeID string) {
	GlobalSyncListener = &SyncListener{
		nodeID:           nodeID,
		stopChan:         make(chan struct{}),
		currentVersion:   0,  // Default to 0 for new followers
		executionFlagTTL: 75, // 75 seconds TTL for execution flags
		baseVersion:      "0",
	}
}

func (sl *SyncListener) GetCurrentVersion() string {
	return fmt.Sprintf("%s.%d", sl.baseVersion, sl.currentVersion)
}

// Start starts the sync listener (follower only)
func (sl *SyncListener) Start() {
	if common.IsCurrentNodeLeader() {
		return
	}
	go sl.listenSyncCommands()
}

// listenSyncCommands listens for sync commands from leader
func (sl *SyncListener) listenSyncCommands() {
	client := common.GetRedisClient()
	if client == nil {
		logger.Error("Redis client not available for sync listener")
		return
	}

	pubsub := client.Subscribe(context.Background(), "cluster:sync_command")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case msg := <-ch:
			var syncCmd map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &syncCmd); err != nil {
				logger.Error("Failed to unmarshal sync command", "error", err)
				continue
			}

			// Check if command is for this node
			// Commands without node_id are broadcast commands (like publish_complete)
			if nodeID, ok := syncCmd["node_id"].(string); ok && nodeID != sl.nodeID {
				continue
			}

			// Handle sync command
			sl.handleSyncCommand(syncCmd)

		case <-sl.stopChan:
			return
		}
	}
}

// handleSyncCommand handles a sync command
func (sl *SyncListener) handleSyncCommand(syncCmd map[string]interface{}) {
	action, _ := syncCmd["action"].(string)
	leaderVersion, _ := syncCmd["leader_version"].(string)

	// Handle both publish_complete and sync commands
	if action != "publish_complete" && action != "sync" {
		return
	}

	// Check if sync is needed
	if sl.GetCurrentVersion() == leaderVersion {
		return
	}

	if err := sl.SyncInstructions(leaderVersion); err != nil {
		logger.Error("Failed to sync instructions", "error", err)
	}
}

func (sl *SyncListener) SyncInstructions(toVersion string) error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	// Set execution flag to indicate this follower is executing instructions
	if err := sl.SetFollowerExecutionFlag(common.GetNodeID()); err != nil {
		logger.Error("Failed to set execution flag", "error", err)
	}

	// Ensure flag is cleared when done (with defer for safety)
	defer func() {
		if err := sl.ClearFollowerExecutionFlag(common.GetNodeID()); err != nil {
			logger.Error("Failed to clear execution flag", "error", err)
		}
	}()

	leaderParts := strings.Split(toVersion, ".")
	if len(leaderParts) != 2 {
		return fmt.Errorf("invalid target version format: %s", toVersion)
	}

	endVersion, err := strconv.ParseInt(leaderParts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid target version number: %s", leaderParts[1])
	}

	// Check if session has changed (leader restart) or if this is a new follower
	if sl.baseVersion != leaderParts[0] {
		logger.Info("Follower needs full sync", "from", sl.GetCurrentVersion(), "to", toVersion)

		// Clear all local components and projects
		if err := sl.clearAllLocalComponents(); err != nil {
			logger.Error("Failed to clear local components", "error", err)
			return fmt.Errorf("failed to clear local components: %w", err)
		}

		// Start from version 0, so we'll sync from version 1 to endVersion
		sl.currentVersion = 0
	}

	// Track instruction execution details
	var processedInstructions []string
	var failedInstructions []string

	// Process instructions from startVersion+1 to endVersion
	// Instructions are numbered from 1 onwards (version 0 is temporary state)
	for version := sl.currentVersion + 1; version <= endVersion; version++ {
		// Refresh execution flag during long operations
		if err := sl.RefreshFollowerExecutionFlag(common.GetNodeID()); err != nil {
			logger.Warn("Failed to refresh execution flag", "error", err)
		}

		// Get instruction from Redis
		key := fmt.Sprintf("cluster:instruction:%d", version)
		data, err := common.RedisGet(key)
		if data == "DELETED" {
			continue
		}

		if err != nil {
			logger.Error("Instruction not found (likely compacted), skipping", "version", version)
			continue
		}

		var instruction Instruction
		if err := json.Unmarshal([]byte(data), &instruction); err != nil {
			logger.Error("Failed to unmarshal instruction", "version", version, "error", err)
			failedInstructions = append(failedInstructions, fmt.Sprintf("v%d: unmarshal error", version))
			continue // Continue processing other instructions
		}

		// Apply instruction - execute once regardless of success/failure
		if err := sl.applyInstruction(version); err != nil {
			logger.Error("Failed to apply instruction", "version", version, "component", instruction.ComponentName, "operation", instruction.Operation, "error", err)
			failedInstructions = append(failedInstructions, fmt.Sprintf("v%d: %s %s %s (failed: %v)", version, instruction.Operation, instruction.ComponentType, instruction.ComponentName, err))
		} else {
			// Record successfully applied instruction details
			instructionDesc := fmt.Sprintf("v%d: %s %s %s", version, instruction.Operation, instruction.ComponentType, instruction.ComponentName)
			instructionDesc += fmt.Sprintf(" (content: %d chars)", len(instruction.Content))
			processedInstructions = append(processedInstructions, instructionDesc)
		}
	}

	sl.currentVersion = endVersion
	sl.baseVersion = leaderParts[0]

	// Log final sync result
	if len(failedInstructions) > 0 {
		logger.Warn("Instructions synced with some failures",
			"from", sl.GetCurrentVersion(),
			"to", toVersion,
			"processed", len(processedInstructions),
			"failed", len(failedInstructions),
			"successful_instructions", strings.Join(processedInstructions, "; "),
			"failed_instructions", strings.Join(failedInstructions, "; "))
	} else {
		logger.Info("Instructions synced successfully",
			"from", sl.GetCurrentVersion(),
			"to", toVersion,
			"count", len(processedInstructions),
			"instructions", strings.Join(processedInstructions, "; "))
	}

	return nil
}

// ClearFollowerExecutionFlag clears the execution flag for a follower
func (sl *SyncListener) ClearFollowerExecutionFlag(nodeID string) error {
	key := fmt.Sprintf("cluster:execution_flag:%s", nodeID)
	return common.RedisDel(key)
}

// SetFollowerExecutionFlag sets a flag indicating follower is executing instructions
func (sl *SyncListener) SetFollowerExecutionFlag(nodeID string) error {
	key := fmt.Sprintf("cluster:execution_flag:%s", nodeID)
	_, err := common.RedisSet(key, "executing", int(sl.executionFlagTTL))
	if err != nil {
		return fmt.Errorf("failed to set execution flag: %w", err)
	}
	return nil
}

// RefreshFollowerExecutionFlag refreshes the TTL of execution flag
func (sl *SyncListener) RefreshFollowerExecutionFlag(nodeID string) error {
	key := fmt.Sprintf("cluster:execution_flag:%s", nodeID)
	_, err := common.RedisSet(key, "executing", int(sl.executionFlagTTL))
	return err
}

// applyInstruction applies a single instruction
func (sl *SyncListener) applyInstruction(version int64) error {
	key := fmt.Sprintf("cluster:instruction:%d", version)
	data, err := common.RedisGet(key)
	if err != nil {
		return fmt.Errorf("failed to get instruction %d: %w", version, err)
	}

	var instruction Instruction
	if err := json.Unmarshal([]byte(data), &instruction); err != nil {
		return fmt.Errorf("failed to unmarshal instruction %d: %w", version, err)
	}

	affectedProjects := []string{}
	if instruction.Metadata != nil {
		if projects, exists := instruction.Metadata["affected_projects"]; exists {
			if projectList, ok := projects.([]interface{}); ok {
				for _, p := range projectList {
					if projectStr, ok := p.(string); ok {
						affectedProjects = append(affectedProjects, projectStr)
					}
				}
			}
		}
	}

	switch instruction.Operation {
	case "add":
		if err := sl.createComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			common.RecordComponentAdd(instruction.ComponentType, instruction.ComponentName, instruction.Content, "failed", err.Error())
			return err
		}
		common.RecordComponentAdd(instruction.ComponentType, instruction.ComponentName, instruction.Content, "success", "")
	case "delete":
		if err := sl.deleteComponentInstance(instruction.ComponentType, instruction.ComponentName); err != nil {
			return err
		}
		common.RecordComponentDelete(instruction.ComponentType, instruction.ComponentName, "success", "", affectedProjects)
	case "update":
		if err := sl.updateComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			common.RecordComponentUpdate(instruction.ComponentType, instruction.ComponentName, instruction.Content, "failed", err.Error())
			return err
		}
		common.RecordComponentUpdate(instruction.ComponentType, instruction.ComponentName, instruction.Content, "success", "")
	case "local_push":
		if err := sl.createComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			common.RecordLocalPush(instruction.ComponentType, instruction.ComponentName, instruction.Content, "failed", err.Error())
			return err
		}
		common.RecordLocalPush(instruction.ComponentType, instruction.ComponentName, instruction.Content, "success", "")
	case "push_change":
		if err := sl.createComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			common.RecordChangePush(instruction.ComponentType, instruction.ComponentName, "", instruction.Content, "", "failed", err.Error())
			return err
		}
		common.RecordChangePush(instruction.ComponentType, instruction.ComponentName, "", instruction.Content, "", "success", "")
	case "start":
		return globalProjectCmdHandler.ExecuteCommandWithOptions(instruction.ComponentName, "start", true)
	case "stop":
		return globalProjectCmdHandler.ExecuteCommandWithOptions(instruction.ComponentName, "stop", true)
	case "restart":
		return globalProjectCmdHandler.ExecuteCommandWithOptions(instruction.ComponentName, "restart", true)
	default:
		return fmt.Errorf("unknown operation: %s", instruction.Operation)
	}

	for _, p := range affectedProjects {
		err := globalProjectCmdHandler.ExecuteCommandWithOptions(p, "restart", true)
		if err != nil {
			return err
		}
	}

	return nil
}

// clearAllLocalComponents clears all local components and projects when leader session changes
func (sl *SyncListener) clearAllLocalComponents() error {
	logger.Info("Clearing all local components and projects due to leader session change")

	// Stop and close all running projects first
	// Collect running projects first to avoid deadlock
	var runningProjects []*project.Project
	project.ForEachProject(func(projectName string, proj *project.Project) bool {
		if proj.Status == common.StatusRunning || proj.Status == common.StatusStarting || proj.Status == common.StatusError {
			runningProjects = append(runningProjects, proj)
		}
		return true
	})

	// Stop projects without holding locks
	for _, proj := range runningProjects {
		logger.Info("Stopping project due to session change", "project", proj.Id)
		if err := proj.Stop(); err != nil {
			logger.Warn("Failed to stop project during session change", "project", proj.Id, "error", err)
		}
	}

	// Clear global component config maps
	common.ClearAllRawConfigsForAllTypes()

	logger.Info("Successfully cleared and released all local components and projects")
	return nil
}

// createComponentInstance creates actual component instances from configuration
func (sl *SyncListener) createComponentInstance(componentType, componentName, content string) error {
	switch componentType {
	case "input":
		// Import the input package at the top if not already imported
		inp, err := input.NewInput("", content, componentName)
		if err != nil {
			return fmt.Errorf("failed to create input instance %s: %w", componentName, err)
		}
		project.SetInput(componentName, inp)
		logger.Debug("Created input instance", "name", componentName)

	case "output":
		// Import the output package at the top if not already imported
		out, err := output.NewOutput("", content, componentName)
		if err != nil {
			return fmt.Errorf("failed to create output instance %s: %w", componentName, err)
		}
		project.SetOutput(componentName, out)
		logger.Debug("Created output instance", "name", componentName)

	case "ruleset":
		// Import the rules_engine package at the top if not already imported
		rs, err := rules_engine.NewRuleset("", content, componentName)
		if err != nil {
			return fmt.Errorf("failed to create ruleset instance %s: %w", componentName, err)
		}
		project.SetRuleset(componentName, rs)
		logger.Debug("Created ruleset instance", "name", componentName)

	case "project":
		// For projects, we create the project instance
		proj, err := project.NewProject("", content, componentName, false)
		if err != nil {
			return fmt.Errorf("failed to create project instance %s: %w", componentName, err)
		}
		project.SetProject(componentName, proj)
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
func (sl *SyncListener) deleteComponentInstance(componentType, componentName string) error {
	switch componentType {
	case "input":
		project.DeleteInput(componentName)
		logger.Debug("Deleted input instance", "name", componentName)

	case "output":
		project.DeleteOutput(componentName)
		logger.Debug("Deleted output instance", "name", componentName)

	case "ruleset":
		project.DeleteRuleset(componentName)
		logger.Debug("Deleted ruleset instance", "name", componentName)

	case "project":
		if proj, exists := project.GetProject(componentName); exists {
			// Stop the project first if it's running
			if proj.Status == common.StatusRunning {
				proj.Stop()
			}
		}
		project.DeleteProject(componentName)
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
func (sl *SyncListener) updateComponentInstance(componentType, componentName, content string) error {
	// For updates, we delete the old instance and create a new one
	if err := sl.deleteComponentInstance(componentType, componentName); err != nil {
		logger.Warn("Failed to delete old component instance during update", "type", componentType, "name", componentName, "error", err)
	}

	return sl.createComponentInstance(componentType, componentName, content)
}

// Stop stops the sync listener
func (sl *SyncListener) Stop() {
	close(sl.stopChan)
	_ = sl.ClearFollowerExecutionFlag(sl.nodeID)
}
