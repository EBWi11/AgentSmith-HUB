package cluster

import (
	"AgentSmith-HUB/logger"
)

// Global cluster state
var (
	IsLeader bool
	NodeID   string
)

// ClusterManager represents the simplified cluster manager
type ClusterManager struct {
	instructionManager *InstructionManager
	heartbeatManager   *HeartbeatManager
	syncListener       *SyncListener
}

var GlobalClusterManager *ClusterManager

// InitCluster initializes the cluster system
func InitCluster(nodeID string, isLeader bool) {
	IsLeader = isLeader
	NodeID = nodeID

	// Initialize all components
	InitInstructionManager()
	InitHeartbeatManager(nodeID, isLeader)
	InitSyncListener(nodeID)

	// Create cluster manager
	GlobalClusterManager = &ClusterManager{
		instructionManager: GlobalInstructionManager,
		heartbeatManager:   GlobalHeartbeatManager,
		syncListener:       GlobalSyncListener,
	}

	logger.Info("Cluster initialized", "node_id", nodeID, "is_leader", isLeader)
}

// Start starts the cluster system
func (cm *ClusterManager) Start() {
	if cm.instructionManager != nil {
		if IsLeader {
			// Leader: Initialize instructions for existing components
			if err := cm.instructionManager.InitializeLeaderInstructions(); err != nil {
				logger.Error("Failed to initialize leader instructions", "error", err)
			}
		}
	}

	if cm.heartbeatManager != nil {
		cm.heartbeatManager.Start()
	}

	if cm.syncListener != nil {
		cm.syncListener.Start()
	}

	logger.Info("Cluster started successfully")
}

// Stop stops the cluster system
func (cm *ClusterManager) Stop() {
	if cm.heartbeatManager != nil {
		cm.heartbeatManager.Stop()
	}

	if cm.syncListener != nil {
		cm.syncListener.Stop()
	}

	// Clear execution flag when shutting down (important for followers)
	if cm.instructionManager != nil && !IsLeader {
		if err := cm.instructionManager.ClearFollowerExecutionFlag(NodeID); err != nil {
			logger.Warn("Failed to clear execution flag during shutdown", "error", err)
		} else {
			logger.Info("Cleared execution flag during shutdown")
		}
	}

	logger.Info("Cluster stopped")
}

// GetClusterStatus returns cluster status
func GetClusterStatus() map[string]interface{} {
	status := map[string]interface{}{
		"is_leader": IsLeader,
		"node_id":   NodeID,
		"nodes":     make(map[string]interface{}),
	}

	if GlobalHeartbeatManager != nil {
		nodes := GlobalHeartbeatManager.GetNodes()
		nodeList := make(map[string]interface{})
		for nodeID, heartbeat := range nodes {
			nodeList[nodeID] = map[string]interface{}{
				"version":   heartbeat.Version,
				"timestamp": heartbeat.Timestamp,
				"online":    true,
			}
		}
		status["nodes"] = nodeList
	}

	if GlobalInstructionManager != nil {
		status["version"] = GlobalInstructionManager.GetCurrentVersion()
	}

	return status
}

// ProjectCommandHandler interface for project operations
type ProjectCommandHandler interface {
	ExecuteCommand(projectID, action string) error
}

// Global project command handler
var globalProjectCmdHandler ProjectCommandHandler

// SetProjectCommandHandler sets the global project command handler
func SetProjectCommandHandler(handler ProjectCommandHandler) {
	globalProjectCmdHandler = handler
}
