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
	}
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

const defaultLeaderAPIPort = "8080"

// GetLeaderAPIBaseURL returns the base URL for the leader's API (e.g. http://<leader_ip>:8080).
// On leader: uses current node ID from memory. On follower: reads cluster:leader:node_id from Redis (written by leader at startup).
func GetLeaderAPIBaseURL() string {
	if IsCurrentNodeLeader() {
		nodeID := strings.TrimSpace(GetNodeID())
		if nodeID != "" {
			return "http://" + nodeID + ":" + defaultLeaderAPIPort
		}
		return ""
	}
	nodeID, err := RedisGet("cluster:leader:node_id")
	if err != nil || strings.TrimSpace(nodeID) == "" {
		return ""
	}
	return "http://" + strings.TrimSpace(nodeID) + ":" + defaultLeaderAPIPort
}
