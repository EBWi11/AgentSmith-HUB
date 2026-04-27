package common

import (
	"fmt"
	"strings"
	"sync"
)

// ClusterState manages the centralized cluster state
type ClusterState struct {
	mu       sync.RWMutex
	isLeader bool
	nodeID   string
	leaderID string
}

// Global cluster state instance
var globalClusterState = &ClusterState{}

// SetClusterState sets the cluster state (called during initialization)
func SetClusterState(isLeader bool, nodeID string) {
	globalClusterState.mu.Lock()
	defer globalClusterState.mu.Unlock()

	globalClusterState.isLeader = isLeader
	globalClusterState.nodeID = nodeID

	if isLeader {
		globalClusterState.leaderID = nodeID
	} else {
		globalClusterState.leaderID = ""
	}
}

// SetNodeLeadership keeps the new and legacy leader state in sync.
func SetNodeLeadership(isLeader bool, nodeID string) {
	SetClusterState(isLeader, nodeID)
	SetLeaderState(isLeader, nodeID)
}

// DemoteCurrentNode clears local leader state for fail-stop scenarios.
func DemoteCurrentNode() {
	nodeID := GetNodeID()
	SetClusterState(false, nodeID)
	SetLeaderState(false, nodeID)
}

// IsCurrentNodeLeader returns whether current node is the leader
func IsCurrentNodeLeader() bool {
	globalClusterState.mu.RLock()
	defer globalClusterState.mu.RUnlock()
	return globalClusterState.isLeader
}

// GetNodeID returns the current node ID
func GetNodeID() string {
	globalClusterState.mu.RLock()
	defer globalClusterState.mu.RUnlock()
	return globalClusterState.nodeID
}

// RequireLeader returns an error if current node is not the leader
func RequireLeader() error {
	if !IsCurrentNodeLeader() {
		return fmt.Errorf("operation requires leader node")
	}
	return nil
}

const (
	// LeaderNodeIDRedisKey is the Redis key where the leader writes its node ID (IP) on startup,
	// so followers and plugins can discover the leader API address.
	LeaderNodeIDRedisKey = "cluster:leader:node_id"
	defaultLeaderAPIPort = "8080"
)

// GetLeaderAPIBaseURL returns the base URL for the leader's API (e.g. http://<leader_ip>:8080).
// On leader: uses current node ID from memory. On follower: reads LeaderNodeIDRedisKey from Redis (written by leader at startup).
func GetLeaderAPIBaseURL() string {
	port := defaultLeaderAPIPort
	if Config != nil && Config.APIPort != "" {
		port = Config.APIPort
	}
	if IsCurrentNodeLeader() {
		nodeID := strings.TrimSpace(GetNodeID())
		if nodeID != "" {
			return "http://" + nodeID + ":" + port
		}
		return ""
	}
	nodeID, err := RedisGet(LeaderNodeIDRedisKey)
	if err != nil || strings.TrimSpace(nodeID) == "" {
		return ""
	}
	return "http://" + strings.TrimSpace(nodeID) + ":" + port
}
