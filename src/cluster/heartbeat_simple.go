package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"context"
	"encoding/json"
	"sync"
	"time"
)

// HeartbeatData represents heartbeat information
type HeartbeatData struct {
	NodeID    string `json:"node_id"`
	Version   string `json:"version"`
	Timestamp int64  `json:"timestamp"`
}

// HeartbeatManager manages heartbeat and version sync
type HeartbeatManager struct {
	nodeID   string
	isLeader bool
	nodes    map[string]HeartbeatData
	mu       sync.RWMutex
	stopChan chan struct{}
}

var GlobalHeartbeatManager *HeartbeatManager

// InitHeartbeatManager initializes the heartbeat manager
func InitHeartbeatManager(nodeID string, isLeader bool) {
	GlobalHeartbeatManager = &HeartbeatManager{
		nodeID:   nodeID,
		isLeader: isLeader,
		nodes:    make(map[string]HeartbeatData),
		stopChan: make(chan struct{}),
	}
}

// Start starts the heartbeat manager
func (hm *HeartbeatManager) Start() {
	if hm.isLeader {
		go hm.startLeaderHeartbeat()
	} else {
		go hm.startFollowerHeartbeat()
	}
}

// startLeaderHeartbeat starts leader heartbeat services
func (hm *HeartbeatManager) startLeaderHeartbeat() {
	// Listen for follower heartbeats
	go hm.listenHeartbeats()

	// Clean up offline nodes
	go hm.cleanupOfflineNodes()
}

// startFollowerHeartbeat starts follower heartbeat services
func (hm *HeartbeatManager) startFollowerHeartbeat() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hm.sendHeartbeat()
		case <-hm.stopChan:
			return
		}
	}
}

// sendHeartbeat sends heartbeat with current version (follower only)
func (hm *HeartbeatManager) sendHeartbeat() {
	if hm.isLeader {
		return
	}

	currentVersion := "v0.0"
	if GlobalInstructionManager != nil {
		currentVersion = GlobalInstructionManager.GetCurrentVersion()
	}

	heartbeat := HeartbeatData{
		NodeID:    hm.nodeID,
		Version:   currentVersion,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(heartbeat)
	if err != nil {
		logger.Error("Failed to marshal heartbeat", "error", err)
		return
	}

	// Send heartbeat to Redis
	if err := common.RedisPublish("cluster:heartbeat", string(data)); err != nil {
		logger.Error("Failed to send heartbeat", "error", err)
	}
}

// listenHeartbeats listens for heartbeats and handles version sync (leader only)
func (hm *HeartbeatManager) listenHeartbeats() {
	if !hm.isLeader {
		return
	}

	client := common.GetRedisClient()
	if client == nil {
		logger.Error("Redis client not available")
		return
	}

	pubsub := client.Subscribe(context.Background(), "cluster:heartbeat")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case msg := <-ch:
			var heartbeat HeartbeatData
			if err := json.Unmarshal([]byte(msg.Payload), &heartbeat); err != nil {
				logger.Error("Failed to unmarshal heartbeat", "error", err)
				continue
			}

			// Skip self
			if heartbeat.NodeID == hm.nodeID {
				continue
			}

			// Update node info
			hm.mu.Lock()
			hm.nodes[heartbeat.NodeID] = heartbeat
			hm.mu.Unlock()

			// Check version and send sync command if needed
			hm.checkVersionSync(heartbeat)

		case <-hm.stopChan:
			return
		}
	}
}

// checkVersionSync checks if follower needs version sync
func (hm *HeartbeatManager) checkVersionSync(heartbeat HeartbeatData) {
	if GlobalInstructionManager == nil {
		return
	}

	leaderVersion := GlobalInstructionManager.GetCurrentVersion()
	if heartbeat.Version != leaderVersion {
		logger.Debug("Version mismatch detected",
			"node", heartbeat.NodeID,
			"follower_version", heartbeat.Version,
			"leader_version", leaderVersion)

		// Send sync command
		syncCmd := map[string]interface{}{
			"node_id":        heartbeat.NodeID,
			"action":         "sync",
			"leader_version": leaderVersion,
			"timestamp":      time.Now().Unix(),
		}

		if data, err := json.Marshal(syncCmd); err == nil {
			if err := common.RedisPublish("cluster:sync_command", string(data)); err != nil {
				logger.Error("Failed to send sync command", "node", heartbeat.NodeID, "error", err)
			}
		}
	}
}

// cleanupOfflineNodes removes offline nodes
func (hm *HeartbeatManager) cleanupOfflineNodes() {
	if !hm.isLeader {
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hm.mu.Lock()
			now := time.Now().Unix()
			for nodeID, heartbeat := range hm.nodes {
				if now-heartbeat.Timestamp > 20 {
					delete(hm.nodes, nodeID)
					logger.Debug("Removed offline node", "node_id", nodeID)
				}
			}
			hm.mu.Unlock()
		case <-hm.stopChan:
			return
		}
	}
}

// GetNodes returns current node list
func (hm *HeartbeatManager) GetNodes() map[string]HeartbeatData {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	nodes := make(map[string]HeartbeatData)
	for k, v := range hm.nodes {
		nodes[k] = v
	}
	return nodes
}

// Stop stops the heartbeat manager
func (hm *HeartbeatManager) Stop() {
	close(hm.stopChan)
}
