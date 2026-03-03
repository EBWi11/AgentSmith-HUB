package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
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
	nodeID            string
	isLeader          bool
	nodes             map[string]HeartbeatData
	mu                sync.RWMutex
	stopChan          chan struct{}
	heartbeatInterval time.Duration // Randomized heartbeat interval for followers
}

var GlobalHeartbeatManager *HeartbeatManager

// InitHeartbeatManager initializes the heartbeat manager
func InitHeartbeatManager(nodeID string, isLeader bool) {
	// Calculate randomized heartbeat interval for followers
	// Base interval: 5 seconds, random jitter: 0-4 seconds (5-9 seconds total)
	baseInterval := 5 * time.Second
	jitter := time.Duration(rand.Int63n(4000)) * time.Millisecond // Use proper random generator
	heartbeatInterval := baseInterval + jitter

	GlobalHeartbeatManager = &HeartbeatManager{
		nodeID:            nodeID,
		isLeader:          isLeader,
		nodes:             make(map[string]HeartbeatData),
		stopChan:          make(chan struct{}),
		heartbeatInterval: heartbeatInterval,
	}

	if !isLeader {
		logger.Info("Follower heartbeat initialized with randomized interval",
			"node_id", nodeID, "interval", heartbeatInterval)
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

	// Use same randomized interval as followers for consistency
	ticker := time.NewTicker(hm.heartbeatInterval)
	defer ticker.Stop()

	logger.Info("Starting leader system metrics update with randomized interval",
		"node_id", hm.nodeID, "interval", hm.heartbeatInterval)

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
	// Use randomized heartbeat interval to avoid heartbeat storms
	ticker := time.NewTicker(hm.heartbeatInterval)
	defer ticker.Stop()

	logger.Info("Starting follower heartbeat with randomized interval",
		"node_id", hm.nodeID, "interval", hm.heartbeatInterval)

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

	// Check if leader kicked us out and requires full resync
	resyncFlagKey := fmt.Sprintf("cluster:resync_required:%s", hm.nodeID)
	if resyncFlag, err := common.RedisGet(resyncFlagKey); err == nil && resyncFlag != "" {
		logger.Error("Follower was kicked out by leader, resetting for full resync",
			"reason", resyncFlag,
			"node_id", hm.nodeID)

		// Clear the resync flag first (with defer to ensure it's always cleared)
		defer func() {
			if err := common.RedisDel(resyncFlagKey); err != nil {
				logger.Error("Failed to clear resync flag", "error", err)
			}
		}()

		// Reset follower to version 0 to trigger full resync (with panic recovery)
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Panic during follower reset", "panic", r)
				}
			}()

			if GlobalSyncListener != nil {
				GlobalSyncListener.ResetForFullResync()
			}
		}()

		logger.Info("Follower reset completed, will perform full resync on next heartbeat")
	}

	currentVersion := "0.0"
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

	// Leader should track itself in Redis for node enumeration
	hm.trackNodeInRedis(hm.nodeID)

	// Retry loop with exponential backoff for Redis connection failures
	retryCount := 0
	maxRetryDelay := 30 * time.Second

	for {
		select {
		case <-hm.stopChan:
			return
		default:
		}

		client := common.GetRedisClient()
		if client == nil {
			logger.Error("Redis client not available for heartbeat listener")
			retryDelay := time.Duration(1<<uint(retryCount)) * time.Second
			if retryDelay > maxRetryDelay {
				retryDelay = maxRetryDelay
			}
			time.Sleep(retryDelay)
			retryCount++
			continue
		}

		pubsub := client.Subscribe(context.Background(), "cluster:heartbeat")
		retryCount = 0 // Reset retry count on successful connection

		// Listen for messages
		ch := pubsub.Channel()
		disconnected := false

		for !disconnected {
			select {
			case msg, ok := <-ch:
				if !ok {
					// Channel closed, need to reconnect
					logger.Warn("Heartbeat pub/sub channel closed, reconnecting...")
					disconnected = true
					break
				}

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
				pubsub.Close()
				return
			}
		}

		// Clean up before reconnecting
		pubsub.Close()
		time.Sleep(2 * time.Second)
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
		logger.Error("Failed to track node in Redis", "node_id", nodeID, "error", err)
	}
}

// checkVersionSync checks if follower needs version sync
func (hm *HeartbeatManager) checkVersionSync(heartbeat HeartbeatData) {
	if GlobalInstructionManager == nil {
		return
	}

	// Skip sync if leader is in compaction mode
	if GlobalInstructionManager.IsCompacting() {
		return
	}

	leaderVersion := GlobalInstructionManager.GetCurrentVersion()
	if heartbeat.Version != leaderVersion {
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
				timeSinceLastHeartbeat := now - heartbeat.Timestamp

				// Node offline detection threshold: 60 seconds
				// Optimized from 120s to 60s for faster offline detection
				// With heartbeat every ~5 seconds, 60 seconds allows sufficient time
				// for network recovery while detecting true failures faster
				//
				// Health status (sent to frontend):
				// - Healthy: last heartbeat within 10 seconds
				// - Not Healthy: last heartbeat > 10 seconds (red indicator in UI)
				if timeSinceLastHeartbeat > 60 {
					delete(hm.nodes, nodeID)
					logger.Info("Removed offline node from cluster",
						"node_id", nodeID,
						"last_heartbeat_seconds_ago", timeSinceLastHeartbeat,
						"last_version", heartbeat.Version)
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
