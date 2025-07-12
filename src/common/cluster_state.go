package common

import (
	"fmt"
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

// UpdateLeaderID updates the leader ID (for follower nodes when they detect a new leader)
func UpdateLeaderID(leaderID string) {
	globalClusterState.mu.Lock()
	defer globalClusterState.mu.Unlock()
	globalClusterState.leaderID = leaderID
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

// GetLeaderID returns the current leader ID
func GetLeaderID() string {
	globalClusterState.mu.RLock()
	defer globalClusterState.mu.RUnlock()
	return globalClusterState.leaderID
}

// GetClusterState returns the complete cluster state
func GetClusterState() (isLeader bool, nodeID string, leaderID string) {
	globalClusterState.mu.RLock()
	defer globalClusterState.mu.RUnlock()
	return globalClusterState.isLeader, globalClusterState.nodeID, globalClusterState.leaderID
}

// RequireLeader returns an error if current node is not the leader
func RequireLeader() error {
	if !IsCurrentNodeLeader() {
		return fmt.Errorf("operation requires leader node")
	}
	return nil
}

// RequireFollower returns an error if current node is not a follower
func RequireFollower() error {
	if IsCurrentNodeLeader() {
		return fmt.Errorf("operation requires follower node")
	}
	return nil
}
