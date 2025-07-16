package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HeartbeatData represents heartbeat information
type HeartbeatData struct {
	NodeID         string  `json:"node_id"`
	Version        string  `json:"version"`
	Timestamp      int64   `json:"timestamp"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryUsedMB   float64 `json:"memory_used_mb"`
	MemoryPercent  float64 `json:"memory_percent"`
	GoroutineCount int     `json:"goroutine_count"`
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

	// Update leader's own system metrics
	go hm.updateLeaderSystemMetrics()
}

// updateLeaderSystemMetrics periodically updates leader's own system metrics
func (hm *HeartbeatManager) updateLeaderSystemMetrics() {
	if !common.IsCurrentNodeLeader() {
		return
	}

	ticker := time.NewTicker(5 * time.Second) // Same frequency as follower heartbeat
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Get current system metrics for leader
			if common.GlobalSystemMonitor != nil && common.GlobalClusterSystemManager != nil {
				if metrics := common.GlobalSystemMonitor.GetCurrentMetrics(); metrics != nil {
					common.GlobalClusterSystemManager.AddSystemMetrics(metrics)
				}
			}
		case <-hm.stopChan:
			return
		}
	}
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

// sendHeartbeat sends heartbeat with current version and system metrics (follower only)
func (hm *HeartbeatManager) sendHeartbeat() {
	if common.IsCurrentNodeLeader() {
		return
	}

	currentVersion := "v0.0"
	if GlobalSyncListener != nil {
		currentVersion = GlobalSyncListener.GetCurrentVersion()
	}

	// Get current system metrics
	var cpuPercent, memoryUsedMB, memoryPercent float64
	var goroutineCount int
	if common.GlobalSystemMonitor != nil {
		if metrics := common.GlobalSystemMonitor.GetCurrentMetrics(); metrics != nil {
			cpuPercent = metrics.CPUPercent
			memoryUsedMB = metrics.MemoryUsedMB
			memoryPercent = metrics.MemoryPercent
			goroutineCount = metrics.GoroutineCount
		}
	}

	heartbeat := HeartbeatData{
		NodeID:         hm.nodeID,
		Version:        currentVersion,
		Timestamp:      time.Now().Unix(),
		CPUPercent:     cpuPercent,
		MemoryUsedMB:   memoryUsedMB,
		MemoryPercent:  memoryPercent,
		GoroutineCount: goroutineCount,
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
	if !common.IsCurrentNodeLeader() {
		return
	}

	client := common.GetRedisClient()
	if client == nil {
		logger.Error("Redis client not available")
		return
	}

	// Leader should track itself in Redis for node enumeration
	hm.trackNodeInRedis(hm.nodeID)

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

			// Check if this is a new node (not in memory)
			hm.mu.Lock()
			_, exists := hm.nodes[heartbeat.NodeID]
			if !exists {
				// New node detected, track it in Redis for node enumeration
				hm.trackNodeInRedis(heartbeat.NodeID)
				logger.Info("New follower node detected and tracked", "node_id", heartbeat.NodeID)
			}

			// Update node info in memory
			hm.nodes[heartbeat.NodeID] = heartbeat
			hm.mu.Unlock()

			// Store system metrics in cluster system manager
			if common.GlobalClusterSystemManager != nil {
				systemMetrics := &common.SystemMetrics{
					NodeID:         heartbeat.NodeID,
					CPUPercent:     heartbeat.CPUPercent,
					MemoryUsedMB:   heartbeat.MemoryUsedMB,
					MemoryPercent:  heartbeat.MemoryPercent,
					GoroutineCount: heartbeat.GoroutineCount,
					Timestamp:      time.Unix(heartbeat.Timestamp, 0),
				}
				common.GlobalClusterSystemManager.AddSystemMetrics(systemMetrics)
			}

			// Check version and send sync command if needed
			hm.checkVersionSync(heartbeat)

		case <-hm.stopChan:
			return
		}
	}
}

// trackNodeInRedis tracks a node in Redis for node enumeration (48 hours TTL)
func (hm *HeartbeatManager) trackNodeInRedis(nodeID string) {
	if nodeID == "" {
		return
	}

	key := "cluster:known_nodes:" + nodeID
	timestamp := time.Now().Unix()

	// Store node info with 48 hours TTL (48 * 60 * 60 = 172800 seconds)
	if _, err := common.RedisSet(key, timestamp, 172800); err != nil {
		logger.Warn("Failed to track node in Redis", "node_id", nodeID, "error", err)
	} else {
		logger.Debug("Tracked node in Redis for enumeration", "node_id", nodeID)
	}
}

// checkVersionSync checks if follower needs version sync
func (hm *HeartbeatManager) checkVersionSync(heartbeat HeartbeatData) {
	if GlobalInstructionManager == nil {
		return
	}

	leaderVersion := GlobalInstructionManager.GetCurrentVersion()
	// Skip sync only if leader is actually in compaction mode (currentVersion == 0)
	// Don't skip just because version ends with .0, as that could be a valid final state
	if leaderVersion != "" && strings.Contains(leaderVersion, ".") {
		parts := strings.Split(leaderVersion, ".")
		if len(parts) == 2 {
			if versionNum, err := strconv.ParseInt(parts[1], 10, 64); err == nil && versionNum == 0 {
				// Check if this is actually compaction mode by getting the raw version
				if GlobalInstructionManager != nil {
					rawVersion := GlobalInstructionManager.currentVersion
					if rawVersion == 0 {
						logger.Debug("Leader in compaction mode, skipping sync", "leader_version", leaderVersion)
						return
					}
				}
			}
		}
	}

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
	if !common.IsCurrentNodeLeader() {
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
				// Remove nodes that haven't sent heartbeat for more than 2 minutes (120 seconds)
				// With heartbeat every 5 seconds, missing 2 heartbeats means unhealthy (10s),
				// and missing 24 heartbeats means offline and should be removed (120s)
				if now-heartbeat.Timestamp > 120 {
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
