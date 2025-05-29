package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// NodeStatus represents the status of a cluster node
type NodeStatus string

const (
	NodeStatusLeader   NodeStatus = "leader"
	NodeStatusFollower NodeStatus = "follower"
	NodeStatusUnknown  NodeStatus = "unknown"
)

// NodeInfo represents information about a cluster node
type NodeInfo struct {
	ID        string     `json:"id"`
	Address   string     `json:"address"`
	Status    NodeStatus `json:"status"`
	LastSeen  time.Time  `json:"last_seen"`
	IsHealthy bool       `json:"is_healthy"`
	MissCount int        `json:"miss_count"` // Count of consecutive missed heartbeats
}

// HeartbeatMessage represents a heartbeat message sent to the leader
type HeartbeatMessage struct {
	NodeID    string    `json:"node_id"`
	NodeAddr  string    `json:"node_addr"`
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"`
}

var ClusterInstance *ClusterManager

// ClusterManager manages the cluster state
type ClusterManager struct {
	mu sync.RWMutex

	// Node information
	SelfID      string
	SelfAddress string
	Status      NodeStatus

	// Leader information
	LeaderID      string
	LeaderAddress string

	// Cluster nodes
	Nodes map[string]*NodeInfo

	// Configuration
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	CleanupInterval   time.Duration
	MaxMissCount      int // Maximum allowed consecutive missed heartbeats
	stopChan          chan struct{}
}

var (
	clusterManager *ClusterManager
	once           sync.Once
)

// InitClusterManager initializes the cluster manager
func ClusterInit(selfID, selfAddress string) *ClusterManager {
	ClusterInstance = &ClusterManager{
		SelfID:            selfID,
		SelfAddress:       selfAddress,
		Status:            NodeStatusFollower,
		Nodes:             make(map[string]*NodeInfo),
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  15 * time.Second,
		CleanupInterval:   10 * time.Second,
		MaxMissCount:      3, // Remove node after 3 consecutive missed heartbeats
		stopChan:          make(chan struct{}),
	}

	return ClusterInstance
}

// RegisterNode registers a new node in the cluster
func (cm *ClusterManager) RegisterNode(nodeID, address string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.Nodes[nodeID] = &NodeInfo{
		ID:        nodeID,
		Address:   address,
		Status:    NodeStatusFollower,
		LastSeen:  time.Now(),
		IsHealthy: true,
		MissCount: 0,
	}
}

// UpdateNodeHeartbeat updates the last seen time for a node
func (cm *ClusterManager) UpdateNodeHeartbeat(nodeID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if node, exists := cm.Nodes[nodeID]; exists {
		node.LastSeen = time.Now()
		node.IsHealthy = true
		node.MissCount = 0 // Reset missed heartbeat counter
	}
}

// CheckNodeHealth checks if a node is healthy based on its last heartbeat
func (cm *ClusterManager) CheckNodeHealth(nodeID string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if node, exists := cm.Nodes[nodeID]; exists {
		return time.Since(node.LastSeen) < cm.HeartbeatTimeout
	}
	return false
}

// SetLeader sets the leader node
func (cm *ClusterManager) SetLeader(leaderID, leaderAddress string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.LeaderID = leaderID
	cm.LeaderAddress = leaderAddress
}

// IsLeader checks if this node is the leader
func (cm *ClusterManager) IsLeader() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Status == NodeStatusLeader
}

// GetClusterStatus returns the current cluster status
func (cm *ClusterManager) GetClusterStatus() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	status := make(map[string]interface{})
	status["self_id"] = cm.SelfID
	status["self_address"] = cm.SelfAddress
	status["status"] = cm.Status
	status["leader_id"] = cm.LeaderID
	status["leader_address"] = cm.LeaderAddress

	nodes := make([]map[string]interface{}, 0)
	for _, node := range cm.Nodes {
		nodes = append(nodes, map[string]interface{}{
			"id":         node.ID,
			"address":    node.Address,
			"status":     node.Status,
			"last_seen":  node.LastSeen,
			"is_healthy": node.IsHealthy,
			"miss_count": node.MissCount,
		})
	}
	status["nodes"] = nodes

	return status
}

// SendHeartbeat sends a heartbeat to the leader
func (cm *ClusterManager) SendHeartbeat() error {
	cm.mu.RLock()
	leaderAddr := cm.LeaderAddress
	selfID := cm.SelfID
	selfAddr := cm.SelfAddress
	cm.mu.RUnlock()

	if leaderAddr == "" {
		return fmt.Errorf("no leader address available")
	}

	// Prepare heartbeat message
	msg := HeartbeatMessage{
		NodeID:    selfID,
		NodeAddr:  selfAddr,
		Timestamp: time.Now(),
		Status:    "active",
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat data: %w", err)
	}

	// Send heartbeat to leader
	heartbeatURL := fmt.Sprintf("http://%s/cluster/heartbeat", leaderAddr)
	resp, err := http.Post(heartbeatURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat request failed with status: %d", resp.StatusCode)
	}

	return nil
}

// StartHeartbeatLoop starts the heartbeat sending loop
func (cm *ClusterManager) StartHeartbeatLoop() {
	go func() {
		ticker := time.NewTicker(cm.HeartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := cm.SendHeartbeat(); err != nil {
					// Log error but continue
					fmt.Printf("Failed to send heartbeat: %v\n", err)

					// If we're a follower and can't reach the leader, we might need to start an election
					cm.mu.RLock()
					isFollower := cm.Status == NodeStatusFollower
					cm.mu.RUnlock()

					if isFollower {
						// TODO: Implement leader election logic
						fmt.Printf("Lost connection to leader, might need to start election\n")
					}
				}
			case <-cm.stopChan:
				return
			}
		}
	}()
}

// cleanupUnhealthyNodes removes nodes that haven't sent heartbeats for too long
func (cm *ClusterManager) cleanupUnhealthyNodes() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	for nodeID, node := range cm.Nodes {
		// Check if node has exceeded heartbeat timeout
		if now.Sub(node.LastSeen) > cm.HeartbeatTimeout {
			node.MissCount++ // Increment missed heartbeat counter
			node.IsHealthy = false

			// Remove node if it has exceeded the maximum allowed missed heartbeats
			if node.MissCount >= cm.MaxMissCount {
				fmt.Printf("Removing unhealthy node: %s (last seen: %v, miss count: %d)\n",
					nodeID, node.LastSeen, node.MissCount)
				delete(cm.Nodes, nodeID)
			} else {
				fmt.Printf("Node %s missed heartbeat (count: %d)\n", nodeID, node.MissCount)
			}
		}
	}
}

// StartCleanupLoop starts the cleanup loop for unhealthy nodes
func (cm *ClusterManager) StartCleanupLoop() {
	go func() {
		ticker := time.NewTicker(cm.CleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cm.cleanupUnhealthyNodes()
			case <-cm.stopChan:
				return
			}
		}
	}()
}

// Stop stops all background processes
func (cm *ClusterManager) Stop() {
	close(cm.stopChan)
}

// Start starts all background processes
func (cm *ClusterManager) Start() {
	// Start heartbeat loop if this is not the leader
	if !cm.IsLeader() {
		cm.StartHeartbeatLoop()
	}

	// Start cleanup loop
	cm.StartCleanupLoop()
}
