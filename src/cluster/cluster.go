package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"time"
)

// Global cluster state variables have been removed
// Use common.IsCurrentNodeLeader() and common.GetNodeID() instead

// ClusterManager represents the simplified cluster manager
type ClusterManager struct {
	instructionManager *InstructionManager
	heartbeatManager   *HeartbeatManager
	syncListener       *SyncListener
}

var GlobalClusterManager *ClusterManager

// InitCluster initializes the cluster system
func InitCluster(nodeID string, isLeader bool) {
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
		if common.IsCurrentNodeLeader() {
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
	if cm.instructionManager != nil && !common.IsCurrentNodeLeader() {
		if err := cm.instructionManager.ClearFollowerExecutionFlag(common.GetNodeID()); err != nil {
			logger.Warn("Failed to clear execution flag during shutdown", "error", err)
		} else {
			logger.Info("Cleared execution flag during shutdown")
		}
	}

	logger.Info("Cluster stopped")
}

// GetClusterStatus returns cluster status
// Returns both old format (is_leader, node_id) and new format (self_id, self_address, status) for compatibility
func GetClusterStatus() map[string]interface{} {
	status := map[string]interface{}{
		// Legacy fields - keep for backward compatibility
		"is_leader": common.IsCurrentNodeLeader(),
		"node_id":   common.GetNodeID(),
		// New fields - required by frontend ClusterStatus.vue
		"self_id":      common.GetNodeID(),
		"self_address": common.GetNodeID(),
		"status":       "follower", // Default to follower
		"nodes":        make(map[string]interface{}),
	}

	// Set current node status based on leader flag
	if common.IsCurrentNodeLeader() {
		status["status"] = "leader"
	}

	nodeList := make(map[string]interface{})

	// Always include current node in the list
	if common.IsCurrentNodeLeader() {
		// Leader node - always show, regardless of GlobalInstructionManager state
		version := "unknown"
		if GlobalInstructionManager != nil {
			version = GlobalInstructionManager.GetCurrentVersion()
		}
		nodeList[common.GetNodeID()] = map[string]interface{}{
			"version":   version,
			"timestamp": time.Now().Unix(),
			"online":    true,
			"role":      "leader",
		}
	} else {
		// Follower node
		nodeList[common.GetNodeID()] = map[string]interface{}{
			"version":   "follower", // Followers don't track version
			"timestamp": time.Now().Unix(),
			"online":    true,
			"role":      "follower",
		}
	}

	// Add other follower nodes (only for leader)
	if common.IsCurrentNodeLeader() && GlobalHeartbeatManager != nil {
		nodes := GlobalHeartbeatManager.GetNodes()
		now := time.Now().Unix()
		for nodeID, heartbeat := range nodes {
			// Calculate health status based on heartbeat timing
			// Healthy: last heartbeat within 10 seconds (missed < 2 heartbeats)
			// Unhealthy: last heartbeat between 10-120 seconds (missed 2-24 heartbeats)
			// Offline: > 120 seconds (will be removed by cleanup, won't appear here)
			timeSinceLastHeartbeat := now - heartbeat.Timestamp
			isHealthy := timeSinceLastHeartbeat <= 10

			nodeList[nodeID] = map[string]interface{}{
				"version":   heartbeat.Version,
				"timestamp": heartbeat.Timestamp,
				"online":    true,
				"role":      "follower",
				"healthy":   isHealthy, // Add health status
			}
		}
	}

	// Convert nodeList object to array for frontend compatibility
	// Frontend ClusterStatus.vue expects nodes as array with specific field names
	nodeArray := make([]map[string]interface{}, 0)
	for nodeID, nodeInfo := range nodeList {
		if nodeMap, ok := nodeInfo.(map[string]interface{}); ok {
			// Add required fields for frontend
			nodeMap["id"] = nodeID      // Frontend expects 'id' field
			nodeMap["address"] = nodeID // Frontend expects 'address' field (use nodeID as address)

			// Convert role to status for frontend compatibility
			if role, exists := nodeMap["role"]; exists {
				nodeMap["status"] = role // Frontend expects 'status' field instead of 'role'
			}

			// Add health status field
			// For follower nodes, use the calculated health status; for leader, always healthy
			if healthy, exists := nodeMap["healthy"]; exists {
				nodeMap["is_healthy"] = healthy // Use calculated health status for followers
			} else {
				nodeMap["is_healthy"] = nodeMap["online"] // Leader is always healthy if online
			}

			// Convert timestamp to proper format
			if timestamp, exists := nodeMap["timestamp"]; exists {
				nodeMap["last_seen"] = timestamp // Frontend expects 'last_seen' field
			}

			nodeArray = append(nodeArray, nodeMap)
		}
	}

	// Set nodes as array (frontend expects array, not object)
	status["nodes"] = nodeArray

	if GlobalInstructionManager != nil {
		status["version"] = GlobalInstructionManager.GetCurrentVersion()
	}

	return status
}

// ProjectCommandHandler interface for project operations
type ProjectCommandHandler interface {
	ExecuteCommand(projectID, action string) error
	ExecuteCommandWithOptions(projectID, action string, recordOperation bool) error
}

// Global project command handler
var globalProjectCmdHandler ProjectCommandHandler

// SetProjectCommandHandler sets the global project command handler
func SetProjectCommandHandler(handler ProjectCommandHandler) {
	globalProjectCmdHandler = handler
}
