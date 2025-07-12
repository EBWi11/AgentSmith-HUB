package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"context"
	"encoding/json"
)

// SyncListener handles sync commands for followers
type SyncListener struct {
	nodeID   string
	stopChan chan struct{}
}

var GlobalSyncListener *SyncListener

// InitSyncListener initializes the sync listener
func InitSyncListener(nodeID string) {
	GlobalSyncListener = &SyncListener{
		nodeID:   nodeID,
		stopChan: make(chan struct{}),
	}
}

// Start starts the sync listener (follower only)
func (sl *SyncListener) Start() {
	if common.IsCurrentNodeLeader() {
		return
	}

	go sl.listenSyncCommands()
	go sl.listenCompactionSignals() // Add compaction signal listener
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
			if nodeID, ok := syncCmd["node_id"].(string); !ok || nodeID != sl.nodeID {
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

	if action != "sync" {
		logger.Warn("Unknown sync action", "action", action)
		return
	}

	logger.Debug("Received sync command", "leader_version", leaderVersion)

	// Get current version
	currentVersion := "v0.0" // Default to v0.0 for new followers
	if GlobalInstructionManager != nil {
		currentVersion = GlobalInstructionManager.GetCurrentVersion()
	}

	// Check if sync is needed
	if currentVersion == leaderVersion {
		logger.Debug("Already up to date", "version", currentVersion)
		return
	}

	// Sync instructions
	if GlobalInstructionManager != nil {
		if err := GlobalInstructionManager.SyncInstructions(currentVersion, leaderVersion); err != nil {
			logger.Error("Failed to sync instructions", "error", err)
		}
		// Note: Success logging is handled inside SyncInstructions method with detailed instruction info
	}
}

// listenCompactionSignals listens for compaction signals from leader
func (sl *SyncListener) listenCompactionSignals() {
	client := common.GetRedisClient()
	if client == nil {
		logger.Error("Redis client not available for compaction listener")
		return
	}

	pubsub := client.Subscribe(context.Background(), "cluster:compaction")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case msg := <-ch:
			var compactionSignal map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &compactionSignal); err != nil {
				logger.Error("Failed to unmarshal compaction signal", "error", err)
				continue
			}

			sl.handleCompactionSignal(compactionSignal)

		case <-sl.stopChan:
			return
		}
	}
}

// handleCompactionSignal handles compaction signals from leader
func (sl *SyncListener) handleCompactionSignal(signal map[string]interface{}) {
	action, _ := signal["action"].(string)

	switch action {
	case "compaction_start":
		originalVersion, _ := signal["original_version"].(float64) // JSON numbers are float64
		logger.Debug("Leader started instruction compaction", "original_version", int64(originalVersion))

		// Followers should pause processing version 0 and wait for compaction_complete
		// This is handled automatically by the SyncInstructions method which skips version 0

	case "compaction_complete":
		newVersion, _ := signal["new_version"].(string)
		logger.Debug("Leader completed instruction compaction", "new_version", newVersion)

		// Trigger immediate sync to get the compacted instructions
		if GlobalInstructionManager != nil {
			currentVersion := GlobalInstructionManager.GetCurrentVersion()
			if err := GlobalInstructionManager.SyncInstructions(currentVersion, newVersion); err != nil {
				logger.Error("Failed to sync after compaction", "error", err)
			} else {
				logger.Debug("Successfully synced compacted instructions", "new_version", newVersion)
			}
		}

	default:
		logger.Warn("Unknown compaction action", "action", action)
	}
}

// Stop stops the sync listener
func (sl *SyncListener) Stop() {
	close(sl.stopChan)
}
