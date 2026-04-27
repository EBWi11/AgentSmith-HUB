package cluster

import (
	"AgentSmith-HUB/agent"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"AgentSmith-HUB/skill"
	"context"
	"encoding/json"
	"fmt"
	"slices"
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
	// syncMu serialises concurrent SyncInstructions calls.  Previously sl.mu
	// (a full write-lock) was held for the entire sync including Phase 2
	// instruction execution, which blocked GetCurrentVersion (used by the
	// heartbeat) for minutes when restarts were involved.  Now sl.mu is only
	// held in short critical sections for reading/writing version fields, while
	// syncMu ensures only one sync runs at a time.
	syncMu sync.Mutex
}

type projectRefreshPlan struct {
	projectName string
	source      string
	restart     bool
	rulesets    map[string]struct{}
	agents      map[string]struct{}
}

type deferredProjectCommand struct {
	projectName string
	operation   string
	version     int64
}

var GlobalSyncListener *SyncListener
var syncRetryDelay = 10 * time.Second

// InitSyncListener initializes the sync listener
func InitSyncListener(nodeID string) {
	GlobalSyncListener = &SyncListener{
		nodeID:           nodeID,
		stopChan:         make(chan struct{}),
		currentVersion:   0,  // Default to 0 for new followers
		executionFlagTTL: 30, // 30 seconds TTL for execution flags (reduced from 75s for faster recovery)
		baseVersion:      "0",
	}
}

func instructionComponentOrder(operation, componentType string) int {
	switch operation {
	case "delete":
		// Reverse the dependency order for destructive batches so referenced
		// projects/agents disappear before their templates are removed.
		order := map[string]int{
			"project": 0,
			"agent":   1,
			"skill":   2,
			"ruleset": 3,
			"plugin":  4,
			"output":  5,
			"input":   5,
		}
		if v, ok := order[componentType]; ok {
			return v
		}
	default:
		order := map[string]int{
			"skill":   0,
			"plugin":  1,
			"input":   2,
			"output":  2,
			"ruleset": 2,
			"agent":   3,
			"project": 4,
		}
		if v, ok := order[componentType]; ok {
			return v
		}
	}
	return 100
}

func (sl *SyncListener) GetCurrentVersion() string {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	return fmt.Sprintf("%s.%d", sl.baseVersion, sl.currentVersion)
}

// getCurrentVersionUnsafe returns version string without locking (must be called with lock held)
func (sl *SyncListener) getCurrentVersionUnsafe() string {
	return fmt.Sprintf("%s.%d", sl.baseVersion, sl.currentVersion)
}

// ResetForFullResync resets follower state to trigger full resync
// Called when follower is kicked out by leader due to slow sync
func (sl *SyncListener) ResetForFullResync() {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	logger.Info("Resetting follower for full resync",
		"old_version", sl.getCurrentVersionUnsafe())

	// Clear all local components and projects
	sl.clearAllLocalComponents()

	// Reset to version 0 (keep same baseVersion - leader will send the correct one)
	sl.currentVersion = 0

	logger.Info("Follower reset completed", "new_version", sl.getCurrentVersionUnsafe())
}

// Start starts the sync listener (follower only)
func (sl *SyncListener) Start() {
	if common.IsCurrentNodeLeader() {
		return
	}

	go sl.listenSyncCommands()
}

// waitForLeaderReadyIfNeeded waits if leader is in compaction mode (version 0)
func (sl *SyncListener) waitForLeaderReadyIfNeeded(targetVersion string) error {
	// Parse target version to check if it's 0
	parts := strings.Split(targetVersion, ".")
	if len(parts) != 2 {
		return nil // Invalid format, let SyncInstructions handle it
	}

	versionNum, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || versionNum > 0 {
		return nil // Version > 0 or parse error, proceed normally
	}

	// Leader is in compaction mode (version = 0), wait for it to complete

	maxWaitTime := 5 * time.Minute // Maximum wait time
	checkInterval := 1 * time.Second
	deadline := time.Now().Add(maxWaitTime)

	for time.Now().Before(deadline) {
		time.Sleep(checkInterval)

		// Re-read leader version
		leaderVersion, err := clusterRedisGet("cluster:leader_version")
		if err != nil {
			logger.Error("Failed to get leader version while waiting", "error", err)
			continue
		}

		// Check if compaction completed
		parts := strings.Split(leaderVersion, ".")
		if len(parts) == 2 {
			if versionNum, err := strconv.ParseInt(parts[1], 10, 64); err == nil && versionNum > 0 {
				return nil
			}
		}
	}

	logger.Error("Timeout waiting for leader compaction to complete, will try sync anyway",
		"node_id", sl.nodeID,
		"max_wait_time", maxWaitTime)
	return nil
}

// listenSyncCommands listens for sync commands from leader
func (sl *SyncListener) listenSyncCommands() {
	// Retry loop with exponential backoff for Redis connection failures
	retryCount := 0
	maxRetryDelay := 30 * time.Second

	for {
		select {
		case <-sl.stopChan:
			return
		default:
		}

		client := common.GetRedisClient()
		if client == nil {
			logger.Error("Redis client not available for sync listener")
			retryDelay := time.Duration(1<<uint(retryCount)) * time.Second
			if retryDelay > maxRetryDelay {
				retryDelay = maxRetryDelay
			}
			time.Sleep(retryDelay)
			retryCount++
			continue
		}

		pubsub := client.Subscribe(context.Background(), "cluster:sync_command")
		retryCount = 0 // Reset retry count on successful connection

		// Listen for messages
		ch := pubsub.Channel()
		disconnected := false

		for !disconnected {
			select {
			case msg, ok := <-ch:
				if !ok {
					// Channel closed, need to reconnect
					logger.Error("Sync command pub/sub channel closed, reconnecting...")
					disconnected = true
					break
				}

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
				pubsub.Close()
				return
			}
		}

		// Clean up before reconnecting
		pubsub.Close()
		time.Sleep(2 * time.Second)
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
	// Serialise concurrent sync attempts.  Previously sl.mu (a full write-lock)
	// was held for the entire function, which blocked GetCurrentVersion (used
	// by the heartbeat goroutine) while project restarts were running inside
	// Phase 2.  Now syncMu guards the "one sync at a time" invariant, while
	// sl.mu is only held in brief critical sections to read/write version fields.
	sl.syncMu.Lock()
	defer sl.syncMu.Unlock()

	// Wait if leader is in compaction mode (version 0)
	if err := sl.waitForLeaderReadyIfNeeded(toVersion); err != nil {
		return fmt.Errorf("failed to wait for leader ready: %w", err)
	}

	leaderParts := strings.Split(toVersion, ".")
	if len(leaderParts) != 2 {
		return fmt.Errorf("invalid target version format: %s", toVersion)
	}

	endVersion, err := strconv.ParseInt(leaderParts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid target version number: %s", leaderParts[1])
	}

	// PHASE 0: Detect leader session change and reset if needed.
	// Read baseVersion under a short RLock; do the expensive clearAllLocalComponents
	// outside any lock so the heartbeat is not blocked.
	sl.mu.RLock()
	baseChanged := sl.baseVersion != leaderParts[0]
	oldVersion := sl.getCurrentVersionUnsafe()
	sl.mu.RUnlock()

	if baseChanged {
		logger.Info("Follower needs full sync due to leader session change",
			"from", oldVersion,
			"to", toVersion,
			"old_base", sl.baseVersion,
			"new_base", leaderParts[0])

		// clearAllLocalComponents is slow (stops projects); run without sl.mu.
		sl.clearAllLocalComponents()

		sl.mu.Lock()
		sl.baseVersion = leaderParts[0]
		sl.currentVersion = 0
		logger.Info("Follower state reset for full resync", "new_version", sl.getCurrentVersionUnsafe())
		sl.mu.Unlock()
	}

	// PHASE 1: Read all instructions from Redis (sets execution flag to block leader compaction).
	if err := sl.SetFollowerExecutionFlag(sl.nodeID); err != nil {
		logger.Error("Failed to set execution flag", "error", err)
	}

	sl.mu.RLock()
	startVersion := sl.currentVersion
	sl.mu.RUnlock()

	var missingInstructions []int64
	var instructions []Instruction
	var compacted uint64
	readStartTime := time.Now()

	for version := startVersion + 1; version <= endVersion; version++ {
		key := fmt.Sprintf("cluster:instruction:%d", version)
		data, err := clusterRedisGet(key)
		if data == GetDeletedIntentionsString() {
			compacted++
			continue
		}

		if err != nil {
			missingInstructions = append(missingInstructions, version)
			logger.Error("Instruction not found in Redis", "version", version, "error", err)
			continue
		}

		var instruction Instruction
		if err := json.Unmarshal([]byte(data), &instruction); err != nil {
			logger.Error("Failed to unmarshal instruction", "version", version, "error", err)
			missingInstructions = append(missingInstructions, version)
			continue
		} else if instruction.ComponentType != "DELETE" {
			instructions = append(instructions, instruction)
		} else {
			compacted++
		}
	}

	_ = time.Since(readStartTime)

	// Clear execution flag immediately — allows leader to proceed with compaction.
	// Phase 2 execution does NOT need to block the leader.
	if err := sl.ClearFollowerExecutionFlag(sl.nodeID); err != nil {
		logger.Error("Failed to clear execution flag", "error", err)
	}

	logger.Info("Instructions read", "count", len(instructions), "compacted", compacted)

	// Handle missing instructions: reset and retry after a delay.
	// The delay is outside sl.mu so the heartbeat can still report the current version.
	if len(missingInstructions) > 0 {
		totalExpected := endVersion - startVersion
		missingRatio := float64(len(missingInstructions)) / float64(totalExpected)
		logger.Error("Missing instructions detected, will reset and retry after delay",
			"missing_count", len(missingInstructions),
			"total_expected", totalExpected,
			"missing_ratio", fmt.Sprintf("%.2f%%", missingRatio*100),
			"missing_versions", missingInstructions)

		sl.clearAllLocalComponents()
		sl.mu.Lock()
		sl.currentVersion = 0
		sl.baseVersion = leaderParts[0]
		sl.mu.Unlock()

		logger.Info("Sleeping 10 seconds before retry due to missing instructions")
		time.Sleep(syncRetryDelay) // outside sl.mu — heartbeat unblocked

		return fmt.Errorf("sync incomplete: %d missing instructions", len(missingInstructions))
	}

	// PHASE 2: Execute all instructions locally.
	// No locks are held here — project restarts / hot-reloads acquire their own
	// locks internally and may take seconds to minutes.  GetCurrentVersion() and
	// the heartbeat remain fully responsive throughout.
	logger.Info("Executing instructions", "count", len(instructions))
	var processedInstructions []string
	var failedInstructions []string
	refreshPlans := buildProjectRefreshPlans(instructions)
	deferredProjectCommands := buildDeferredProjectCommands(instructions)

	slices.SortStableFunc(instructions, func(a, b Instruction) int {
		oa := instructionComponentOrder(a.Operation, a.ComponentType)
		ob := instructionComponentOrder(b.Operation, b.ComponentType)
		if oa != ob {
			return oa - ob
		}
		return int(a.Version) - int(b.Version)
	})

	for _, instruction := range instructions {
		version := instruction.Version
		if version == 0 {
			continue
		}
		if instruction.ComponentType == "project" && PROJECT_OPERATION[instruction.Operation] {
			continue
		}

		if err := sl.applyInstruction(instruction); err != nil {
			logger.Error("Failed to apply instruction",
				"version", version,
				"component", instruction.ComponentName,
				"operation", instruction.Operation,
				"error", err)
			failedInstructions = append(failedInstructions,
				fmt.Sprintf("v%d: %s %s %s (failed: %v)",
					version, instruction.Operation, instruction.ComponentType, instruction.ComponentName, err))
		} else {
			processedInstructions = append(processedInstructions,
				fmt.Sprintf("v%d: %s %s %s",
					version, instruction.Operation, instruction.ComponentType, instruction.ComponentName))
		}
	}

	if len(failedInstructions) == 0 {
		if err := sl.executeProjectRefreshPlans(refreshPlans); err != nil {
			logger.Error("Failed to execute coalesced project refreshes", "error", err)
			failedInstructions = append(failedInstructions, err.Error())
		}
	}

	if len(failedInstructions) == 0 {
		if err := sl.executeDeferredProjectCommands(deferredProjectCommands); err != nil {
			logger.Error("Failed to execute deferred project commands", "error", err)
			failedInstructions = append(failedInstructions, err.Error())
		}
	}

	// PHASE 3: Update version under sl.mu (brief critical section).
	if len(failedInstructions) == 0 {
		sl.mu.Lock()
		sl.currentVersion = endVersion
		sl.baseVersion = leaderParts[0]
		version := sl.getCurrentVersionUnsafe()
		sl.mu.Unlock()
		logger.Info("Follower sync completed", "version", version, "processed", len(processedInstructions), "instruction_count", len(instructions))
		return nil
	}

	// Some instructions failed: reset and trigger full resync.
	sl.mu.RLock()
	curVer := sl.getCurrentVersionUnsafe()
	sl.mu.RUnlock()
	logger.Error("Phase 2 failed: some instructions failed, will reset and retry after delay",
		"node_id", sl.nodeID,
		"failed_count", len(failedInstructions),
		"current_version", curVer,
		"target_version", toVersion,
		"failed_instructions", strings.Join(failedInstructions, "; "))

	sl.clearAllLocalComponents()
	sl.mu.Lock()
	sl.currentVersion = 0
	sl.baseVersion = leaderParts[0]
	sl.mu.Unlock()

	logger.Info("Sleeping 10 seconds before retry due to execution failures")
	time.Sleep(syncRetryDelay) // outside sl.mu — heartbeat unblocked

	return fmt.Errorf("sync incomplete: %d failed instructions", len(failedInstructions))
}

// ClearFollowerExecutionFlag clears the execution flag for a follower
func (sl *SyncListener) ClearFollowerExecutionFlag(nodeID string) error {
	key := fmt.Sprintf("cluster:execution_flag:%s", nodeID)
	return clusterRedisDel(key)
}

// SetFollowerExecutionFlag sets/refreshes a flag indicating follower is executing instructions
func (sl *SyncListener) SetFollowerExecutionFlag(nodeID string) error {
	key := fmt.Sprintf("cluster:execution_flag:%s", nodeID)
	_, err := clusterRedisSet(key, "executing", int(sl.executionFlagTTL))
	if err != nil {
		return fmt.Errorf("failed to set execution flag: %w", err)
	}
	return nil
}

func extractAffectedProjectsAndSource(instruction Instruction) ([]string, string) {
	affectedProjects := []string{}
	source := ""
	if len(instruction.Dependencies) > 0 {
		affectedProjects = append(affectedProjects, instruction.Dependencies...)
	}
	if instruction.Metadata != nil {
		if projects, exists := instruction.Metadata["affected_projects"]; exists {
			if projectList, ok := projects.([]interface{}); ok {
				for _, p := range projectList {
					if projectStr, ok := p.(string); ok {
						exists := false
						for _, existing := range affectedProjects {
							if existing == projectStr {
								exists = true
								break
							}
						}
						if !exists {
							affectedProjects = append(affectedProjects, projectStr)
						}
					}
				}
			}
		}
		if s, exists := instruction.Metadata["source"]; exists {
			if sourceStr, ok := s.(string); ok {
				source = sourceStr
			}
		}
	}
	return affectedProjects, source
}

func shouldQueueProjectRefresh(instruction Instruction) bool {
	switch instruction.Operation {
	case "start", "stop", "restart":
		return false
	case "add", "delete":
		return instruction.ComponentType != "project"
	case "update", "push_change", "local_push":
		return true
	default:
		return false
	}
}

func buildProjectRefreshPlans(instructions []Instruction) map[string]*projectRefreshPlan {
	explicitProjectOps := make(map[string]struct{})
	for _, instruction := range instructions {
		if instruction.ComponentType == "project" && PROJECT_OPERATION[instruction.Operation] {
			explicitProjectOps[instruction.ComponentName] = struct{}{}
		}
	}

	plans := make(map[string]*projectRefreshPlan)
	for _, instruction := range instructions {
		if !shouldQueueProjectRefresh(instruction) {
			continue
		}

		affectedProjects, source := extractAffectedProjectsAndSource(instruction)
		for _, projectName := range affectedProjects {
			if _, hasExplicitOp := explicitProjectOps[projectName]; hasExplicitOp {
				continue
			}

			plan, exists := plans[projectName]
			if !exists {
				plan = &projectRefreshPlan{
					projectName: projectName,
					source:      source,
					rulesets:    make(map[string]struct{}),
					agents:      make(map[string]struct{}),
				}
				plans[projectName] = plan
			}
			if source != "" {
				plan.source = source
			}

			if instruction.ComponentType == "ruleset" && !plan.restart {
				plan.rulesets[instruction.ComponentName] = struct{}{}
				continue
			}
			if instruction.ComponentType == "agent" && !plan.restart {
				plan.agents[instruction.ComponentName] = struct{}{}
				continue
			}

			plan.restart = true
			clear(plan.rulesets)
			clear(plan.agents)
		}
	}

	return plans
}

func buildDeferredProjectCommands(instructions []Instruction) []deferredProjectCommand {
	latestDeleteVersion := make(map[string]int64)
	latestCommandByProject := make(map[string]deferredProjectCommand)

	for _, instruction := range instructions {
		if instruction.ComponentType != "project" {
			continue
		}

		if instruction.Operation == "delete" {
			if instruction.Version > latestDeleteVersion[instruction.ComponentName] {
				latestDeleteVersion[instruction.ComponentName] = instruction.Version
			}
			continue
		}

		if !PROJECT_OPERATION[instruction.Operation] {
			continue
		}

		current, exists := latestCommandByProject[instruction.ComponentName]
		if !exists || instruction.Version > current.version {
			latestCommandByProject[instruction.ComponentName] = deferredProjectCommand{
				projectName: instruction.ComponentName,
				operation:   instruction.Operation,
				version:     instruction.Version,
			}
		}
	}

	commands := make([]deferredProjectCommand, 0, len(latestCommandByProject))
	for _, command := range latestCommandByProject {
		if deleteVersion := latestDeleteVersion[command.projectName]; deleteVersion > command.version {
			continue
		}
		commands = append(commands, command)
	}

	slices.SortStableFunc(commands, func(a, b deferredProjectCommand) int {
		if a.version == b.version {
			return strings.Compare(a.projectName, b.projectName)
		}
		if a.version < b.version {
			return -1
		}
		return 1
	})

	return commands
}

func (sl *SyncListener) executeProjectRefreshPlans(plans map[string]*projectRefreshPlan) error {
	for _, plan := range plans {
		proj, exists := project.GetProject(plan.projectName)
		if !exists {
			logger.Error("Follower: Project to refresh not found", "project", plan.projectName)
			continue
		}

		triggerSource := plan.source
		if triggerSource == "" {
			triggerSource = "cluster_sync"
		}

		if plan.restart || (len(plan.rulesets) == 0 && len(plan.agents) == 0) {
			if err := proj.Restart(false, triggerSource); err != nil {
				return fmt.Errorf("failed to restart affected project %s: %w", plan.projectName, err)
			}
			continue
		}

		rulesetIDs := make([]string, 0, len(plan.rulesets))
		for rulesetID := range plan.rulesets {
			rulesetIDs = append(rulesetIDs, rulesetID)
		}
		slices.Sort(rulesetIDs)

		for _, rulesetID := range rulesetIDs {
			if err := proj.HotReloadRuleset(rulesetID, triggerSource); err != nil {
				logger.Error("Follower: ruleset hot reload failed, falling back to project restart",
					"project", plan.projectName,
					"ruleset", rulesetID,
					"error", err)
				if restartErr := proj.Restart(false, triggerSource+"_fallback"); restartErr != nil {
					return fmt.Errorf("failed to fallback restart affected project %s after ruleset hot reload error: %w", plan.projectName, restartErr)
				}
				break
			}
		}

		agentIDs := make([]string, 0, len(plan.agents))
		for agentID := range plan.agents {
			agentIDs = append(agentIDs, agentID)
		}
		slices.Sort(agentIDs)
		for _, agentID := range agentIDs {
			if err := proj.HotReloadAgent(agentID, triggerSource); err != nil {
				logger.Error("Follower: agent hot reload failed, falling back to project restart",
					"project", plan.projectName,
					"agent", agentID,
					"error", err)
				if restartErr := proj.Restart(false, triggerSource+"_fallback"); restartErr != nil {
					return fmt.Errorf("failed to fallback restart affected project %s after agent hot reload error: %w", plan.projectName, restartErr)
				}
				break
			}
		}
	}
	return nil
}

func (sl *SyncListener) executeDeferredProjectCommands(commands []deferredProjectCommand) error {
	if globalProjectCmdHandler == nil {
		return fmt.Errorf("project command handler not initialized")
	}

	for _, command := range commands {
		if _, exists := project.GetProject(command.projectName); !exists {
			return fmt.Errorf("deferred project command v%d %s %s references missing project",
				command.version, command.operation, command.projectName)
		}

		if err := globalProjectCmdHandler.ExecuteCommandWithOptions(command.projectName, command.operation, true); err != nil {
			return fmt.Errorf("failed to execute deferred project command v%d %s %s: %w",
				command.version, command.operation, command.projectName, err)
		}
	}

	return nil
}

// applyInstruction applies a single instruction snapshot read during Phase 1.
func (sl *SyncListener) applyInstruction(instruction Instruction) error {
	switch instruction.Operation {
	case "add":
		if err := sl.createComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			clusterRecordComponentAdd(instruction.ComponentType, instruction.ComponentName, instruction.Content, "failed", err.Error())
			return err
		}
		clusterRecordComponentAdd(instruction.ComponentType, instruction.ComponentName, instruction.Content, "success", "")
	case "delete":
		if err := sl.deleteComponentInstance(instruction.ComponentType, instruction.ComponentName); err != nil {
			return err
		}
	case "update":
		if err := sl.updateComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			clusterRecordComponentUpdate(instruction.ComponentType, instruction.ComponentName, instruction.Content, "failed", err.Error())
			return err
		}
	case "local_push":
		if instruction.ComponentType == "project" {
			if err := sl.updateComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
				clusterRecordLocalPush(instruction.ComponentType, instruction.ComponentName, instruction.Content, "failed", err.Error())
				return err
			}
			break
		}
		if err := sl.createComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			clusterRecordLocalPush(instruction.ComponentType, instruction.ComponentName, instruction.Content, "failed", err.Error())
			return err
		}
	case "push_change":
		if instruction.ComponentType == "project" {
			if err := sl.updateComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
				clusterRecordChangePush(instruction.ComponentType, instruction.ComponentName, "", instruction.Content, "", "failed", err.Error())
				return err
			}
			break
		}
		if err := sl.createComponentInstance(instruction.ComponentType, instruction.ComponentName, instruction.Content); err != nil {
			clusterRecordChangePush(instruction.ComponentType, instruction.ComponentName, "", instruction.Content, "", "failed", err.Error())
			return err
		}
	default:
		return fmt.Errorf("unknown operation: %s", instruction.Operation)
	}

	return nil
}

// clearAllLocalComponents clears all local components and projects when leader session changes
// This function never fails - it will try best effort to clean everything
// IMPORTANT: This ensures complete cleanup of all running resources before full resync
func (sl *SyncListener) clearAllLocalComponents() {
	logger.Info("Follower resetting all components",
		"node_id", sl.nodeID,
		"reason", "full_resync_required")

	// Step 1: Stop ALL projects (running, starting, error state, even stopped ones)
	// This ensures all inputs/outputs/channels are properly closed
	var allProjects []*project.Project
	project.ForEachProject(func(projectName string, proj *project.Project) bool {
		allProjects = append(allProjects, proj)
		return true
	})

	// Stop projects one by one, wait for each to complete
	stoppedCount := 0
	failedCount := 0
	for _, proj := range allProjects {

		// Force stop regardless of current status
		if err := proj.Stop(true); err != nil {
			logger.Error("Failed to stop project during cleanup, will force delete anyway",
				"project", proj.Id,
				"error", err)
			failedCount++
		} else {
			stoppedCount++
		}

		// Give a brief moment for resources to be released
		time.Sleep(100 * time.Millisecond)
	}

	// Step 2: Collect all component IDs before deletion
	var projectIDs, inputIDs, outputIDs, rulesetIDs, pluginIDs, agentIDs, skillIDs []string

	project.ForEachProject(func(projectName string, _ *project.Project) bool {
		projectIDs = append(projectIDs, projectName)
		return true
	})

	for id := range project.GetAllInputs() {
		inputIDs = append(inputIDs, id)
	}

	for id := range project.GetAllOutputs() {
		outputIDs = append(outputIDs, id)
	}

	for id := range project.GetAllRulesets() {
		rulesetIDs = append(rulesetIDs, id)
	}

	common.ForEachRawConfig("plugin", func(pluginID, _ string) bool {
		pluginIDs = append(pluginIDs, pluginID)
		return true
	})

	project.ForEachAgent(func(id string, _ *agent.Agent) bool {
		agentIDs = append(agentIDs, id)
		return true
	})

	project.ForEachSkill(func(id string, _ *skill.Skill) bool {
		skillIDs = append(skillIDs, id)
		return true
	})

	// Step 3: Delete all component instances
	// Order: projects first, then agents, then skills, then the rest

	for _, id := range projectIDs {
		project.DeleteProject(id)
	}

	for _, id := range agentIDs {
		project.DeleteAgent(id)
	}

	for _, id := range skillIDs {
		project.DeleteSkill(id)
	}

	for _, id := range inputIDs {
		project.DeleteInput(id)
	}

	for _, id := range outputIDs {
		project.DeleteOutput(id)
	}

	for _, id := range rulesetIDs {
		project.DeleteRuleset(id)
	}

	// Step 4: Clear all raw config maps (memory cleanup)
	// This includes plugins, inputs, outputs, rulesets, projects
	plugin.ResetManagedPluginsForResync()
	common.ClearAllRawConfigsForAllTypes()

	// Step 5: Give system a moment to fully release all resources
	time.Sleep(500 * time.Millisecond)

	logger.Info("Follower reset complete")
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

	case "output":
		// Import the output package at the top if not already imported
		out, err := output.NewOutput("", content, componentName)
		if err != nil {
			return fmt.Errorf("failed to create output instance %s: %w", componentName, err)
		}
		project.SetOutput(componentName, out)

	case "ruleset":
		// Import the rules_engine package at the top if not already imported
		rs, err := rules_engine.NewRuleset("", content, componentName)
		if err != nil {
			return fmt.Errorf("failed to create ruleset instance %s: %w", componentName, err)
		}
		project.SetRuleset(componentName, rs)

	case "project":
		// For projects, we create the project instance
		proj, err := project.NewProject("", content, componentName, false)
		if err != nil {
			return fmt.Errorf("failed to create project instance %s: %w", componentName, err)
		}
		project.SetProject(componentName, proj)

	case "plugin":
		err := plugin.NewPlugin("", content, componentName, plugin.YAEGI_PLUGIN)
		if err != nil {
			return fmt.Errorf("failed to create plugin instance %s: %w", componentName, err)
		}

	case "skill":
		s, err := skill.NewSkill("", content, componentName)
		if err != nil {
			return fmt.Errorf("failed to create skill instance %s: %w", componentName, err)
		}
		project.SetSkill(componentName, s)

	case "agent":
		a, err := agent.NewAgent("", content, componentName)
		if err != nil {
			return fmt.Errorf("failed to create agent instance %s: %w", componentName, err)
		}
		project.SetAgent(componentName, a)

	default:
		return fmt.Errorf("unsupported component type: %s", componentType)
	}

	common.SetRawConfig(componentType, componentName, content)
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
			// Stop(true) handles non-stoppable states gracefully; no need to
			// read proj.Status without a lock before calling it.
			_ = proj.Stop(true)
		}
		project.DeleteProject(componentName)
		logger.Debug("Deleted project instance", "name", componentName)

	case "plugin":
		if _, err := project.SafeDeletePluginComponent(componentName); err != nil {
			return fmt.Errorf("failed to delete plugin %s: %w", componentName, err)
		}
		logger.Debug("Deleted plugin instance", "name", componentName)

	case "skill":
		if _, err := project.SafeDeleteSkillComponent(componentName); err != nil {
			return fmt.Errorf("failed to delete skill %s: %w", componentName, err)
		}
		logger.Debug("Deleted skill instance", "name", componentName)

	case "agent":
		if _, err := project.SafeDeleteAgentComponent(componentName); err != nil {
			return fmt.Errorf("failed to delete agent %s: %w", componentName, err)
		}
		logger.Debug("Deleted agent instance", "name", componentName)

	default:
		return fmt.Errorf("unsupported component type: %s", componentType)
	}

	common.DeleteRawConfig(componentType, componentName)
	return nil
}

// updateComponentInstance updates existing component instances with new configuration
func (sl *SyncListener) updateComponentInstance(componentType, componentName, content string) error {
	// For updates, we delete the old instance and create a new one
	if err := sl.deleteComponentInstance(componentType, componentName); err != nil {
		logger.Error("Failed to delete old component instance during update", "type", componentType, "name", componentName, "error", err)
	}

	return sl.createComponentInstance(componentType, componentName, content)
}

// Stop stops the sync listener
func (sl *SyncListener) Stop() {
	close(sl.stopChan)
	_ = sl.ClearFollowerExecutionFlag(sl.nodeID)
}
