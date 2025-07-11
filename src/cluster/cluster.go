package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"

	"github.com/cespare/xxhash/v2"
)

// NodeStatus represents the status of a cluster node
type NodeStatus string

var IsLeader bool
var NodeID string

// ClusterConfig represents cluster configuration
type ClusterConfig struct {
	NodeID     string `json:"node_id"`
	ListenAddr string `json:"listen_addr"`
}

const (
	NodeStatusLeader   NodeStatus = "leader"
	NodeStatusFollower NodeStatus = "follower"
)

// NodeInfo represents information about a cluster node
type NodeInfo struct {
	ID             string     `json:"id"`
	Address        string     `json:"address"`
	Status         NodeStatus `json:"status"`
	LastSeen       time.Time  `json:"last_seen"`
	IsHealthy      bool       `json:"is_healthy"`
	MissCount      int        `json:"miss_count"`                 // Count of consecutive missed heartbeats
	ConfigVersion  string     `json:"config_version,omitempty"`   // Current config version
	LastConfigSync time.Time  `json:"last_config_sync,omitempty"` // Last successful config sync time
}

// ProjectStatus represents a project's current status
type ProjectStatus struct {
	ID              string     `json:"id"`
	Status          string     `json:"status"`
	StatusChangedAt *time.Time `json:"status_changed_at,omitempty"`
}

// HeartbeatMessage represents a heartbeat message sent to the leader
type HeartbeatMessage struct {
	NodeID        string    `json:"node_id"`
	NodeAddr      string    `json:"node_addr"`
	Timestamp     time.Time `json:"timestamp"`
	Status        string    `json:"status"`
	ConfigVersion string    `json:"config_version,omitempty"` // Add config version for drift detection
}

var ClusterInstance *ClusterManager

// Global configuration update tracking
var (
	lastConfigUpdateTime time.Time
	configUpdateMutex    sync.RWMutex
)

// ClusterManager manages the cluster state
type ClusterManager struct {
	Mu sync.RWMutex

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
	HeartbeatInterval     time.Duration
	HeartbeatTimeout      time.Duration
	CleanupInterval       time.Duration
	MaxMissCount          int // Maximum allowed consecutive missed heartbeats
	stopChan              chan struct{}
	stopHeartbeatMonitor  chan struct{}
	stopFollowerHeartbeat chan struct{}
	startTime             time.Time

	// Node project states storage (only used by leader)
	NodeProjectStates map[string][]ProjectStatus

	stopSubProj chan struct{}

	stopProjCmdSub    chan struct{}
	stopReconcile     chan struct{}
	stopComponentSync chan struct{}
}

var (
	// GlobalMu protects cluster-wide state
	GlobalMu sync.RWMutex
	// Nodes tracks all nodes in the cluster
	Nodes = make(map[string]time.Time)
	// Leader is the current leader node
	Leader string
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
		NodeProjectStates: make(map[string][]ProjectStatus),
	}

	return ClusterInstance
}

// RegisterNode registers a new node in the cluster
func (cm *ClusterManager) RegisterNode(nodeID, address string) {
	cm.Mu.Lock()
	defer cm.Mu.Unlock()

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
	cm.Mu.Lock()
	defer cm.Mu.Unlock()

	if node, exists := cm.Nodes[nodeID]; exists {
		node.LastSeen = time.Now()
		node.IsHealthy = true
		node.MissCount = 0 // Reset missed heartbeat counter
	}
}

// CheckNodeHealth checks if a node is healthy based on its last heartbeat
func (cm *ClusterManager) CheckNodeHealth(nodeID string) bool {
	cm.Mu.RLock()
	defer cm.Mu.RUnlock()

	if node, exists := cm.Nodes[nodeID]; exists {
		return time.Since(node.LastSeen) < cm.HeartbeatTimeout
	}
	return false
}

// SetLeader sets the leader node
func (cm *ClusterManager) SetLeader(leaderID, leaderAddress string) {
	cm.Mu.Lock()
	defer cm.Mu.Unlock()

	cm.LeaderID = leaderID
	cm.LeaderAddress = leaderAddress

	if cm.LeaderID == cm.SelfID && cm.LeaderAddress == cm.SelfAddress {
		cm.Status = NodeStatusLeader
		IsLeader = true
	} else {
		cm.Status = NodeStatusFollower
		IsLeader = false
	}
}

// IsLeader checks if this node is the leader
func (cm *ClusterManager) IsLeader() bool {
	cm.Mu.RLock()
	defer cm.Mu.RUnlock()
	return cm.Status == NodeStatusLeader
}

// GetClusterStatus returns the current cluster status
func (cm *ClusterManager) GetClusterStatus() map[string]interface{} {
	cm.Mu.RLock()
	defer cm.Mu.RUnlock()

	status := make(map[string]interface{})
	status["self_id"] = cm.SelfID
	status["self_address"] = cm.SelfAddress
	status["status"] = cm.Status
	status["leader_id"] = cm.LeaderID
	status["leader_address"] = cm.LeaderAddress

	nodes := make([]map[string]interface{}, 0)
	for _, node := range cm.Nodes {
		// Only include healthy nodes in the cluster status
		if node.IsHealthy {
			nodes = append(nodes, map[string]interface{}{
				"id":         node.ID,
				"address":    node.Address,
				"status":     node.Status,
				"last_seen":  node.LastSeen,
				"is_healthy": node.IsHealthy,
				"miss_count": node.MissCount,
			})
		}
	}
	status["nodes"] = nodes

	return status
}

// StartHeartbeatLoop starts the heartbeat sending loop
func (cm *ClusterManager) StartHeartbeatLoop() {
	if cm.IsLeader() {
		return
	}

	go func() {
		ticker := time.NewTicker(cm.HeartbeatInterval)
		defer ticker.Stop()

		consecutiveFailures := 0
		maxConsecutiveFailures := 5

		for {
			select {
			case <-ticker.C:
				if err := cm.SendRedisHeartbeat(); err != nil {
					consecutiveFailures++
					logger.Error("Failed to send heartbeat", "error", err, "consecutive_failures", consecutiveFailures)

					// If we're a follower and can't reach Redis consistently, try to reconnect
					cm.Mu.Lock()
					isFollower := cm.Status == NodeStatusFollower
					cm.Mu.Unlock()

					if isFollower && consecutiveFailures >= maxConsecutiveFailures {
						logger.Warn("Too many consecutive heartbeat failures, attempting Redis reconnection", "failures", consecutiveFailures)
						// Try to reconnect to Redis
						if err := common.RedisPing(); err != nil {
							logger.Error("Redis ping failed during reconnection attempt", "error", err)
							// TODO: Implement leader election logic or enter degraded mode
							logger.Warn("Lost connection to Redis, cluster coordination may be affected")
						} else {
							logger.Info("Redis reconnection successful")
							consecutiveFailures = 0 // Reset counter on successful reconnection
						}
					}
				} else {
					// Successful heartbeat, reset failure counter
					if consecutiveFailures > 0 {
						logger.Info("Heartbeat successful after failures", "previous_failures", consecutiveFailures)
						consecutiveFailures = 0
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
	cm.Mu.Lock()
	defer cm.Mu.Unlock()

	// Refresh LastSeen of known nodes from Redis hash (fallback when Pub/Sub lost)
	hmap, err := common.RedisHGetAll("cluster:heartbeats")
	if err == nil {
		for nodeID, payload := range hmap {
			if nodeID == cm.SelfID {
				continue
			}
			if existing, exists := cm.Nodes[nodeID]; exists {
				var hb HeartbeatMessage
				if jsonErr := json.Unmarshal([]byte(payload), &hb); jsonErr == nil {
					// Ignore records with zero timestamp (parse failure)
					if hb.Timestamp.IsZero() {
						continue
					}
					// Only treat as healthy if the heartbeat timestamp is recent
					if time.Since(hb.Timestamp) <= cm.HeartbeatTimeout {
						existing.LastSeen = hb.Timestamp
						existing.IsHealthy = true
						existing.MissCount = 0
					}
				}
			}
		}
	}

	// Health check based on LastSeen timestamp
	now := time.Now()
	for nodeID, node := range cm.Nodes {
		if now.Sub(node.LastSeen) > cm.HeartbeatTimeout {
			node.MissCount++
			node.IsHealthy = false
			if node.MissCount >= cm.MaxMissCount {
				logger.Warn("Removing unhealthy node", "node_id", nodeID, "miss", node.MissCount)
				delete(cm.Nodes, nodeID)
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

// StartLeaderServices starts additional services that only run on the leader node
func (cm *ClusterManager) StartLeaderServices() {
	if !cm.IsLeader() {
		return
	}

	// Start project status subscriber to track follower project states
	cm.startProjectStatusSubscriber()

	// Start project reconciler to ensure follower states match leader expectations
	cm.startProjectReconciler()

	logger.Info("Leader-specific services started")
}

// Stop stops all background processes
func (cm *ClusterManager) Stop() error {
	logger.Info("Stopping cluster manager")

	cm.Mu.Lock()
	defer cm.Mu.Unlock()

	// Stop background ticker loops (heartbeat send/cleanup etc.)
	if cm.stopChan != nil {
		close(cm.stopChan)
		cm.stopChan = nil
	}
	// Stop heartbeat monitoring
	if cm.stopHeartbeatMonitor != nil {
		close(cm.stopHeartbeatMonitor)
		cm.stopHeartbeatMonitor = nil
	}

	// Stop component sync subscriber
	if cm.stopComponentSync != nil {
		close(cm.stopComponentSync)
		cm.stopComponentSync = nil
	}

	// Stop project command subscriber
	if cm.stopProjCmdSub != nil {
		close(cm.stopProjCmdSub)
		cm.stopProjCmdSub = nil
	}

	// Stop project status subscriber
	if cm.stopSubProj != nil {
		close(cm.stopSubProj)
		cm.stopSubProj = nil
	}

	// Stop project reconciler
	if cm.stopReconcile != nil {
		close(cm.stopReconcile)
		cm.stopReconcile = nil
	}

	// Stop QPS manager if this is the leader
	if IsLeader {
		common.StopQPSManager()
		logger.Info("QPS manager stopped")

		common.StopClusterSystemManager()
		logger.Info("Cluster system manager stopped")
	}

	// Stop system monitor
	common.StopSystemMonitor()
	logger.Info("System monitor stopped")

	// Stop follower heartbeat if this is a follower
	if cm.stopFollowerHeartbeat != nil {
		close(cm.stopFollowerHeartbeat)
		cm.stopFollowerHeartbeat = nil
	}

	logger.Info("Cluster manager stopped")
	return nil
}

// Start starts all background processes
func (cm *ClusterManager) Start() {
	// Start heartbeat loop if this is not the leader
	if !cm.IsLeader() {
		cm.StartHeartbeatLoop()

		// Start component sync subscriber
		cm.startComponentSyncSubscriber()

		// Start project command subscriber
		cm.startProjectCommandSubscriber()
	}

	// Followers need a separate cleanup loop; leader already cleans via monitorHeartbeats
	if !cm.IsLeader() {
		cm.StartCleanupLoop()
	}
}

// StartAsLeader starts this node as cluster leader
func StartAsLeader(config *ClusterConfig) error {
	logger.Info("Starting as cluster leader")

	IsLeader = true
	NodeID = config.NodeID

	cm := &ClusterManager{
		SelfID:                config.NodeID,
		SelfAddress:           config.ListenAddr,
		Status:                NodeStatusLeader,
		Nodes:                 make(map[string]*NodeInfo),
		HeartbeatInterval:     5 * time.Second,
		HeartbeatTimeout:      15 * time.Second,
		CleanupInterval:       10 * time.Second,
		MaxMissCount:          3,
		stopChan:              make(chan struct{}),
		stopHeartbeatMonitor:  make(chan struct{}),
		stopFollowerHeartbeat: make(chan struct{}),
		startTime:             time.Now(),
	}

	ClusterInstance = cm

	// Initialize QPS manager for leader
	common.InitQPSManager()
	logger.Info("QPS manager initialized for leader")

	// Initialize cluster system manager for leader
	common.InitClusterSystemManager()
	logger.Info("Cluster system manager initialized for leader")

	// Start heartbeat monitoring (for monitoring followers)
	go cm.monitorHeartbeats()

	// Start project states sync loop
	cm.StartProjectStatesSyncLoop()

	// Start Redis Pub/Sub subscriber for heartbeats
	cm.StartRedisHeartbeatSubscriber()

	cm.startProjectStatusSubscriber()

	// Start reconcile loop for project states
	cm.startProjectReconciler()

	// One-time discovery of existing followers from heartbeat hash
	cm.discoverFollowersFromHash()

	logger.Info("Cluster leader started", "node_id", config.NodeID, "listen_addr", config.ListenAddr)
	return nil
}

func (cm *ClusterManager) monitorHeartbeats() {
	ticker := time.NewTicker(cm.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Leader监控follower的心跳状态，而不是发送心跳
			cm.cleanupUnhealthyNodes()
		case <-cm.stopHeartbeatMonitor:
			return
		}
	}
}

// StartProjectStatesSyncLoop starts leader's project states synchronization loop
func (cm *ClusterManager) StartProjectStatesSyncLoop() {
	if !cm.IsLeader() {
		return // Only leader does project states sync
	}

	logger.Info("Starting project states sync loop as leader")
	// Execute sync immediately on startup
	go cm.syncAllNodesProjectStates()

	go func() {
		ticker := time.NewTicker(10 * time.Second) // Sync every 10 seconds
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cm.syncAllNodesProjectStates()
			case <-cm.stopChan:
				return
			}
		}
	}()
}

// syncAllNodesProjectStates syncs project states from Redis (Redis-only approach)
func (cm *ClusterManager) syncAllNodesProjectStates() {
	// Get all healthy nodes including self
	cm.Mu.RLock()
	allNodes := make(map[string]*NodeInfo)
	allNodes[cm.SelfID] = &NodeInfo{
		ID:        cm.SelfID,
		Address:   cm.SelfAddress,
		Status:    cm.Status,
		IsHealthy: true,
	}
	for nodeID, node := range cm.Nodes {
		if node.IsHealthy {
			allNodes[nodeID] = node
		}
	}
	cm.Mu.RUnlock()

	// Sync project states from Redis for each healthy node
	for nodeID := range allNodes {
		projectStates := cm.getProjectStatesFromRedis(nodeID)

		cm.Mu.Lock()
		if len(projectStates) >= 0 { // Even empty states are valid
			cm.NodeProjectStates[nodeID] = projectStates
		}
		cm.Mu.Unlock()
	}
}

// getProjectStatesFromRedis fetches project states from Redis for a specific node
func (cm *ClusterManager) getProjectStatesFromRedis(nodeID string) []ProjectStatus {
	// Get project states from Redis hash
	hashKey := "cluster:proj_states:" + nodeID
	projectStateMap, err := common.RedisHGetAll(hashKey)
	if err != nil {
		// No error logging for Redis misses, it's normal for new nodes
		return []ProjectStatus{}
	}

	// Load timestamp map (may be empty on old data)
	tsMap, _ := common.RedisHGetAll("cluster:proj_status_ts:" + nodeID)

	// Convert to ProjectStatus format (only stable states)
	projectStates := make([]ProjectStatus, 0, len(projectStateMap))
	for projectID, status := range projectStateMap {
		// Only include stable states in cluster coordination
		if status == "running" || status == "stopped" {
			var tsPtr *time.Time
			if tsStr, ok := tsMap[projectID]; ok {
				if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
					tsPtr = &t
				}
			}
			projectState := ProjectStatus{
				ID:              projectID,
				Status:          status,
				StatusChangedAt: tsPtr,
			}
			projectStates = append(projectStates, projectState)
		}
	}

	return projectStates
}

// startProjectCommandSubscriber listens for project commands from leader (follower only)
func (cm *ClusterManager) startProjectCommandSubscriber() {
	if cm.IsLeader() {
		return // leader doesn't need to subscribe to its own commands
	}

	client := common.GetRedisClient()
	if client == nil {
		return
	}

	pubsub := client.Subscribe(context.Background(), "cluster:proj_cmd")
	if cm.stopProjCmdSub == nil {
		cm.stopProjCmdSub = make(chan struct{})
	}

	go func() {
		ch := pubsub.Channel()
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					return
				}

				var cmd map[string]string
				if err := json.Unmarshal([]byte(msg.Payload), &cmd); err != nil {
					continue
				}

				// Only process commands directed at this node
				if cmd["node_id"] != cm.SelfID {
					continue
				}

				projectID := cmd["project_id"]
				action := cmd["action"]

				logger.Info("Received project command", "project_id", projectID, "action", action)

				// Execute the command
				cm.executeProjectCommand(projectID, action)

			case <-cm.stopProjCmdSub:
				_ = pubsub.Close()
				return
			}
		}
	}()
}

// ProjectCommandHandler is an interface for handling project commands
// This interface is implemented in the project package to avoid circular dependencies
type ProjectCommandHandler interface {
	ExecuteCommand(projectID, action string) error
}

// Global project command handler (set by project package)
var globalProjectCmdHandler ProjectCommandHandler

// SetProjectCommandHandler sets the global project command handler
func SetProjectCommandHandler(handler ProjectCommandHandler) {
	globalProjectCmdHandler = handler
}

// executeProjectCommand executes a project command on follower node
func (cm *ClusterManager) executeProjectCommand(projectID, action string) {
	logger.Info("Executing project command", "project_id", projectID, "action", action)

	if globalProjectCmdHandler == nil {
		logger.Error("Project command handler not initialized", "project_id", projectID, "action", action)
		// Report failure status to leader
		cm.reportProjectCommandFailure(projectID, action, "Project command handler not initialized")
		return
	}

	err := globalProjectCmdHandler.ExecuteCommand(projectID, action)
	if err != nil {
		logger.Error("Failed to execute project command", "project_id", projectID, "action", action, "error", err)
		// Report failure status to leader
		cm.reportProjectCommandFailure(projectID, action, err.Error())
	} else {
		logger.Info("Successfully executed project command", "project_id", projectID, "action", action)
	}
}

// reportProjectCommandFailure reports command execution failure to leader
func (cm *ClusterManager) reportProjectCommandFailure(projectID, action, errorMsg string) {
	// Determine the expected status based on the failed action
	var actualStatus string
	switch action {
	case "start":
		actualStatus = "stopped" // If start failed, project remains stopped
	case "stop":
		actualStatus = "running" // If stop failed, project remains running
	case "restart":
		actualStatus = "unknown" // Restart failure could leave project in any state
	default:
		actualStatus = "unknown"
	}

	// Report the actual status to Redis for leader visibility
	if common.GetRedisClient() != nil {
		hashKey := "cluster:proj_states:" + cm.SelfID
		if err := common.RedisHSet(hashKey, projectID, actualStatus); err != nil {
			logger.Error("Failed to report project command failure status", "project_id", projectID, "error", err)
		}

		// Also publish the failure event
		evt := map[string]string{
			"node_id":    cm.SelfID,
			"project_id": projectID,
			"status":     actualStatus,
			"error":      errorMsg,
		}
		if data, err := json.Marshal(evt); err == nil {
			_ = common.RedisPublish("cluster:proj_status", string(data))
		}
	}
}

// ===================== Redis-based Heartbeat =====================

// SendRedisHeartbeat writes a TTL-based heartbeat key to Redis. Followers call this instead of HTTP API.
func (cm *ClusterManager) SendRedisHeartbeat() error {
	// Store basic info for debugging/metrics (address + timestamp)
	hb := HeartbeatMessage{
		NodeID:        cm.SelfID,
		NodeAddr:      cm.SelfAddress,
		Timestamp:     time.Now().UTC(),
		Status:        "active",
		ConfigVersion: calculateConfigVersion(), // Add config version for drift detection
	}

	jsonData, err := json.Marshal(hb)
	if err != nil {
		return err
	}

	// Store to hash for state snapshot
	if err := common.RedisHSet("cluster:heartbeats", cm.SelfID, string(jsonData)); err != nil {
		return err
	}

	// NOTE: 不再每次刷新整个 hash 的 TTL；让 leader 通过 hb.Timestamp 判断健康
	// Publish for real-time update
	if err := common.RedisPublish("cluster:heartbeat", string(jsonData)); err != nil {
		return err
	}
	return nil
}

// calculateConfigVersion computes a hash of the current configuration content for drift detection
func calculateConfigVersion() string {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	// Use xxhash for fast content hashing (non-cryptographic)
	hash := xxhash.New()

	// Add a base string to ensure non-empty hash even with empty configs
	hash.Write([]byte("agentsmith-hub-config-v1"))

	// Hash all component configurations in deterministic order
	// Inputs
	if common.AllInputsRawConfig != nil {
		inputKeys := make([]string, 0, len(common.AllInputsRawConfig))
		for k := range common.AllInputsRawConfig {
			inputKeys = append(inputKeys, k)
		}
		sort.Strings(inputKeys)
		for _, k := range inputKeys {
			hash.Write([]byte("input:" + k + ":" + common.AllInputsRawConfig[k]))
		}
	}

	// Outputs
	if common.AllOutputsRawConfig != nil {
		outputKeys := make([]string, 0, len(common.AllOutputsRawConfig))
		for k := range common.AllOutputsRawConfig {
			outputKeys = append(outputKeys, k)
		}
		sort.Strings(outputKeys)
		for _, k := range outputKeys {
			hash.Write([]byte("output:" + k + ":" + common.AllOutputsRawConfig[k]))
		}
	}

	// Rulesets
	if common.AllRulesetsRawConfig != nil {
		rulesetKeys := make([]string, 0, len(common.AllRulesetsRawConfig))
		for k := range common.AllRulesetsRawConfig {
			rulesetKeys = append(rulesetKeys, k)
		}
		sort.Strings(rulesetKeys)
		for _, k := range rulesetKeys {
			hash.Write([]byte("ruleset:" + k + ":" + common.AllRulesetsRawConfig[k]))
		}
	}

	// Projects
	if common.AllProjectRawConfig != nil {
		projectKeys := make([]string, 0, len(common.AllProjectRawConfig))
		for k := range common.AllProjectRawConfig {
			projectKeys = append(projectKeys, k)
		}
		sort.Strings(projectKeys)
		for _, k := range projectKeys {
			hash.Write([]byte("project:" + k + ":" + common.AllProjectRawConfig[k]))
		}
	}

	// Plugins
	if common.AllPluginsRawConfig != nil {
		pluginKeys := make([]string, 0, len(common.AllPluginsRawConfig))
		for k := range common.AllPluginsRawConfig {
			pluginKeys = append(pluginKeys, k)
		}
		sort.Strings(pluginKeys)
		for _, k := range pluginKeys {
			hash.Write([]byte("plugin:" + k + ":" + common.AllPluginsRawConfig[k]))
		}
	}

	// Include project running states in configuration version
	// This ensures that project start/stop operations change the config version
	if ClusterInstance != nil {
		nodeID := ClusterInstance.SelfID
		if nodeID != "" {
			projectStates, err := common.RedisHGetAll("cluster:proj_states:" + nodeID)
			if err == nil && len(projectStates) > 0 {
				// Sort project states for deterministic hashing
				stateKeys := make([]string, 0, len(projectStates))
				for k := range projectStates {
					stateKeys = append(stateKeys, k)
				}
				sort.Strings(stateKeys)
				for _, k := range stateKeys {
					hash.Write([]byte("proj_state:" + k + ":" + projectStates[k]))
				}
			}
		}
	}

	// Return xxhash as hex string (16 characters for readability)
	return fmt.Sprintf("%016x", hash.Sum64())
}

// triggerNodeConfigSync forces a specific node to resync its configuration
func triggerNodeConfigSync(nodeID string) {
	// Step 1: First sync project states (which projects should be running)
	syncProjectStatesToNode(nodeID)

	// Step 2: Then sync all component configurations
	componentTypes := []string{"input", "output", "ruleset", "plugin"} // Remove "project" from config sync

	for _, componentType := range componentTypes {
		// Get all components of this type
		var componentMap map[string]string
		common.GlobalMu.RLock()
		switch componentType {
		case "input":
			componentMap = common.AllInputsRawConfig
		case "output":
			componentMap = common.AllOutputsRawConfig
		case "ruleset":
			componentMap = common.AllRulesetsRawConfig
		case "plugin":
			componentMap = common.AllPluginsRawConfig
		}
		common.GlobalMu.RUnlock()

		// Publish sync events for all components of this type
		// Note: This is syncing existing config, not updating it, so no timestamp update needed
		for id, content := range componentMap {
			PublishComponentSync(&CompSyncEvt{
				Op:   "update",
				Type: componentType,
				ID:   id,
				Raw:  content,
			})
		}
	}

	// Step 3: Finally sync running projects (config + start commands)
	// This happens after a small delay to ensure components are synced first
	go func() {
		time.Sleep(1 * time.Second) // Wait for component sync to complete
		syncRunningProjectsToNode(nodeID)
	}()

	logger.Info("Triggered full configuration sync for node", "node_id", nodeID)
}

// syncProjectStatesToNode syncs project states (which projects should be running) to a specific node
func syncProjectStatesToNode(nodeID string) {
	// Get running projects from leader's Redis state
	leaderProjectStates, err := common.RedisHGetAll("cluster:proj_states:" + ClusterInstance.SelfID)
	if err != nil {
		logger.Error("Failed to get leader project states", "error", err)
		return
	}

	// First, send stop commands for all projects to clear follower state
	for projectID := range leaderProjectStates {
		publishProjCmd(nodeID, projectID, "stop")
	}

	logger.Info("Synced project states to node", "node_id", nodeID, "projects", len(leaderProjectStates))
}

// syncRunningProjectsToNode syncs only the running projects to a specific node
func syncRunningProjectsToNode(nodeID string) {
	// Get running projects from leader's Redis state
	leaderProjectStates, err := common.RedisHGetAll("cluster:proj_states:" + ClusterInstance.SelfID)
	if err != nil {
		logger.Error("Failed to get leader project states", "error", err)
		return
	}

	// For each running project, sync its configuration and start it
	for projectID, state := range leaderProjectStates {
		if state == "running" {
			// First sync the project configuration
			common.GlobalMu.RLock()
			projectConfig, exists := common.AllProjectRawConfig[projectID]
			common.GlobalMu.RUnlock()

			if exists {
				// Sync project configuration
				// Note: This is syncing existing config, not updating it, so no timestamp update needed
				PublishComponentSync(&CompSyncEvt{
					Op:   "update",
					Type: "project",
					ID:   projectID,
					Raw:  projectConfig,
				})

				// Then start the project
				publishProjCmd(nodeID, projectID, "start")
			}
		}
	}

	logger.Info("Synced running projects to node", "node_id", nodeID, "count", len(leaderProjectStates))
}

// SyncProjectStateChange syncs project state changes (start/stop/delete/restart) to all followers
func SyncProjectStateChange(projectID, action string) {
	if !IsLeader {
		return // Only leader can sync project state changes
	}

	// Get all healthy follower nodes
	if ClusterInstance == nil {
		return
	}

	ClusterInstance.Mu.RLock()
	nodes := make(map[string]*NodeInfo)
	for k, v := range ClusterInstance.Nodes {
		if v.IsHealthy {
			nodes[k] = v
		}
	}
	ClusterInstance.Mu.RUnlock()

	// Send command to each follower node
	for nodeID := range nodes {
		switch action {
		case "start":
			// Sync project config and send start command
			common.GlobalMu.RLock()
			projectConfig, exists := common.AllProjectRawConfig[projectID]
			common.GlobalMu.RUnlock()

			if exists {
				// Note: This is syncing config as part of a state change, not just passive sync
				PublishComponentSync(&CompSyncEvt{
					Op:        "update",
					Type:      "project",
					ID:        projectID,
					Raw:       projectConfig,
					IsRunning: true,
				})
			}
			publishProjCmd(nodeID, projectID, "start")

		case "stop":
			publishProjCmd(nodeID, projectID, "stop")

		case "restart":
			// For restart, first stop then start the project
			publishProjCmd(nodeID, projectID, "stop")
			// Add a small delay to ensure stop is processed
			time.Sleep(100 * time.Millisecond)

			// Sync project config and send start command
			common.GlobalMu.RLock()
			projectConfig, exists := common.AllProjectRawConfig[projectID]
			common.GlobalMu.RUnlock()

			if exists {
				// Note: This is syncing config as part of a state change, not just passive sync
				PublishComponentSync(&CompSyncEvt{
					Op:        "update",
					Type:      "project",
					ID:        projectID,
					Raw:       projectConfig,
					IsRunning: true,
				})
			}
			publishProjCmd(nodeID, projectID, "start")

		case "delete":
			// Stop project first, then remove config
			publishProjCmd(nodeID, projectID, "stop")
			// Note: This is a config change (deletion), not just passive sync
			PublishComponentSync(&CompSyncEvt{
				Op:   "delete",
				Type: "project",
				ID:   projectID,
			})
		}
	}

	// Update config timestamp since this represents actual project state changes
	UpdateConfigTimestamp()

	logger.Info("Synced project state change to all followers", "project_id", projectID, "action", action)
}

// triggerForcedNodeSync forces a specific node to resync its configuration
func triggerForcedNodeSync(nodeID string) {
	logger.Warn("Forcing node config sync due to timeout or drift", "node_id", nodeID)

	// For severely outdated nodes (>1 min), send stop command for all projects first
	cm := ClusterInstance
	if cm != nil {
		cm.Mu.RLock()
		node, exists := cm.Nodes[nodeID]
		cm.Mu.RUnlock()

		if exists && time.Since(node.LastConfigSync) > time.Minute {
			logger.Warn("Node severely outdated, stopping all projects before sync",
				"node_id", nodeID,
				"last_sync", node.LastConfigSync)

			// Get all projects from leader and send stop commands
			if leaderStates, err := common.RedisHGetAll("cluster:proj_states:" + cm.SelfID); err == nil {
				for projectID := range leaderStates {
					publishProjCmd(nodeID, projectID, "stop")
				}

				// Wait a moment for stops to process
				time.Sleep(2 * time.Second)
			}
		}
	}

	// Trigger full configuration sync
	triggerNodeConfigSync(nodeID)
}

// UpdateConfigTimestamp should be called whenever leader updates configuration
func UpdateConfigTimestamp() {
	configUpdateMutex.Lock()
	defer configUpdateMutex.Unlock()
	lastConfigUpdateTime = time.Now()
}

// GetLastConfigUpdateTime returns the last configuration update time
func GetLastConfigUpdateTime() time.Time {
	configUpdateMutex.RLock()
	defer configUpdateMutex.RUnlock()
	return lastConfigUpdateTime
}

// StartRedisHeartbeatSubscriber subscribes to Redis channel for real-time heartbeats (leader only)
func (cm *ClusterManager) StartRedisHeartbeatSubscriber() {
	if !cm.IsLeader() {
		return
	}

	client := common.GetRedisClient()
	if client == nil {
		logger.Error("Redis client not initialized, heartbeat subscriber not started")
		return
	}

	pubsub := client.Subscribe(context.Background(), "cluster:heartbeat")
	cm.stopHeartbeatMonitor = make(chan struct{}) // reuse monitor stop channel if nil

	go func() {
		ch := pubsub.Channel()
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					logger.Warn("Heartbeat pubsub channel closed")
					return
				}
				var hb HeartbeatMessage
				if err := json.Unmarshal([]byte(msg.Payload), &hb); err == nil {
					cm.Mu.Lock()
					node, exists := cm.Nodes[hb.NodeID]
					if !exists {
						cm.Nodes[hb.NodeID] = &NodeInfo{
							ID:        hb.NodeID,
							Address:   hb.NodeAddr,
							Status:    NodeStatusFollower,
							LastSeen:  hb.Timestamp, // Use actual heartbeat timestamp
							IsHealthy: true,
							MissCount: 0,
						}
						logger.Info("Discovered follower via PubSub", "node_id", hb.NodeID, "addr", hb.NodeAddr)
					} else {
						node.LastSeen = hb.Timestamp // Use actual heartbeat timestamp
						node.IsHealthy = true
						node.MissCount = 0

						// Update node's config version
						if hb.ConfigVersion != "" {
							node.ConfigVersion = hb.ConfigVersion
						}

						// Check for configuration drift
						leaderVersion := calculateConfigVersion()
						configOutdated := false

						if hb.ConfigVersion != "" && hb.ConfigVersion != leaderVersion {
							logger.Warn("Configuration drift detected",
								"node_id", hb.NodeID,
								"node_version", hb.ConfigVersion,
								"leader_version", leaderVersion)
							configOutdated = true
						}

						// Check if node hasn't synced for more than 1 minute
						lastUpdate := GetLastConfigUpdateTime()
						if !lastUpdate.IsZero() && time.Since(node.LastConfigSync) > time.Minute {
							logger.Warn("Node config sync timeout detected",
								"node_id", hb.NodeID,
								"last_sync", node.LastConfigSync,
								"last_update", lastUpdate)
							configOutdated = true
						}

						// Trigger forced sync if needed
						if configOutdated {
							go triggerForcedNodeSync(hb.NodeID)
						} else {
							// Update last sync time if config is up to date
							node.LastConfigSync = time.Now()
						}
					}
					cm.Mu.Unlock()
				}
			case <-cm.stopHeartbeatMonitor:
				_ = pubsub.Close()
				return
			}
		}
	}()
}

// startProjectStatusSubscriber listens to project status events and updates NodeProjectStates (leader only)
func (cm *ClusterManager) startProjectStatusSubscriber() {
	if !cm.IsLeader() {
		return
	}
	client := common.GetRedisClient()
	if client == nil {
		return
	}
	pub := client.Subscribe(context.Background(), "cluster:proj_status")
	if cm.stopSubProj == nil {
		cm.stopSubProj = make(chan struct{})
	}
	go func() {
		ch := pub.Channel()
		for {
			select {
			case m, ok := <-ch:
				if !ok {
					return
				}
				var evt struct {
					NodeID          string `json:"node_id"`
					ProjectID       string `json:"project_id"`
					Status          string `json:"status"`
					StatusChangedAt string `json:"status_changed_at,omitempty"`
				}
				if err := json.Unmarshal([]byte(m.Payload), &evt); err == nil {
					// Only process stable states for cluster coordination
					if evt.Status != "running" && evt.Status != "stopped" {
						logger.Debug("Skipping transient project status event", "node_id", evt.NodeID, "project_id", evt.ProjectID, "status", evt.Status)
						continue
					}

					cm.Mu.Lock()
					if cm.NodeProjectStates == nil {
						cm.NodeProjectStates = make(map[string][]ProjectStatus)
					}
					list := cm.NodeProjectStates[evt.NodeID]
					updated := false

					// Parse timestamp if provided; keep nil on failure so we don't overwrite with inaccurate time
					var statusChangedAt *time.Time
					if evt.StatusChangedAt != "" {
						if parsedTime, err := time.Parse(time.RFC3339, evt.StatusChangedAt); err == nil {
							statusChangedAt = &parsedTime
						}
					}

					for i := range list {
						if list[i].ID == evt.ProjectID {
							list[i].Status = evt.Status
							if statusChangedAt != nil {
								// Only overwrite timestamp when we have a valid parsed value
								list[i].StatusChangedAt = statusChangedAt
							}
							updated = true
							break
						}
					}
					if !updated {
						list = append(list, ProjectStatus{ID: evt.ProjectID, Status: evt.Status, StatusChangedAt: statusChangedAt})
					}
					cm.NodeProjectStates[evt.NodeID] = list
					cm.Mu.Unlock()
				}
			case <-cm.stopSubProj:
				_ = pub.Close()
				return
			}
		}
	}()
}

// startProjectReconciler periodically checks follower project states and publishes commands if mismatch
func (cm *ClusterManager) startProjectReconciler() {
	if !cm.IsLeader() {
		return
	}
	if cm.stopReconcile == nil {
		cm.stopReconcile = make(chan struct{})
	}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cm.reconcileProjectStates()
			case <-cm.stopReconcile:
				return
			}
		}
	}()
}

// reconcileProjectStates compares desired leader project status vs follower reported and publishes commands
func (cm *ClusterManager) reconcileProjectStates() {
	// Snapshot follower-reported states
	cm.Mu.RLock()
	snapshot := make(map[string][]ProjectStatus, len(cm.NodeProjectStates))
	for nodeID, list := range cm.NodeProjectStates {
		cp := make([]ProjectStatus, len(list))
		copy(cp, list)
		snapshot[nodeID] = cp
	}
	cm.Mu.RUnlock()

	// Desired statuses from leader hash (only stable states)
	desired := make(map[string]string)
	if leaderHash, err := common.RedisHGetAll("cluster:proj_states:" + cm.SelfID); err == nil {
		for pid, status := range leaderHash {
			// Only consider stable states for reconciliation
			if status == "running" || status == "stopped" {
				desired[pid] = status
			}
		}
	}

	// Compare and publish commands only when there's a real mismatch
	for nodeID, states := range snapshot {
		// Skip self node (leader)
		if nodeID == cm.SelfID {
			continue
		}

		for _, st := range states {
			// Only consider stable states for reconciliation
			if st.Status != "running" && st.Status != "stopped" {
				logger.Debug("Skipping reconciliation for transient project status", "node_id", nodeID, "project_id", st.ID, "status", st.Status)
				continue
			}

			want, ok := desired[st.ID]
			if !ok {
				// Unknown project on follower, instruct stop only if not already stopped
				if st.Status != "stopped" {
					logger.Info("Unknown project on follower, sending stop command", "node_id", nodeID, "project_id", st.ID, "current_status", st.Status)
					publishProjCmd(nodeID, st.ID, "stop")
				}
				continue
			}

			// Only send command if there's an actual status mismatch
			if want != st.Status {
				// Avoid sending redundant commands
				// Check if the status changed recently to avoid command spam
				if st.StatusChangedAt != nil && time.Since(*st.StatusChangedAt) < 5*time.Second {
					// Status was recently updated, skip reconciliation to avoid command spam
					continue
				}

				// Convert status to action
				var action string
				switch want {
				case "running":
					action = "start"
				case "stopped":
					action = "stop"
				default:
					action = "stop" // Default to stop for unknown states
				}

				logger.Info("Project status mismatch detected, sending reconciliation command",
					"node_id", nodeID,
					"project_id", st.ID,
					"desired_status", want,
					"current_status", st.Status,
					"action", action)
				publishProjCmd(nodeID, st.ID, action)
			}
		}

		// Check for missing projects on follower (projects that exist on leader but not on follower)
		followerProjects := make(map[string]bool)
		for _, st := range states {
			followerProjects[st.ID] = true
		}

		for projectID, desiredStatus := range desired {
			if !followerProjects[projectID] && desiredStatus == "running" {
				// Project should be running but doesn't exist on follower
				logger.Info("Missing running project on follower, sending start command",
					"node_id", nodeID,
					"project_id", projectID,
					"desired_status", desiredStatus)
				publishProjCmd(nodeID, projectID, "start")
			}
		}
	}
}

// discoverFollowersFromHash adds any nodes found in Redis heartbeat hash to cm.Nodes (only at startup)
func (cm *ClusterManager) discoverFollowersFromHash() {
	if !cm.IsLeader() {
		return
	}
	hmap, err := common.RedisHGetAll("cluster:heartbeats")
	if err != nil {
		return
	}
	for nodeID, payload := range hmap {
		if nodeID == cm.SelfID {
			continue
		}
		var hb HeartbeatMessage
		if jsonErr := json.Unmarshal([]byte(payload), &hb); jsonErr != nil {
			continue
		}
		cm.Nodes[nodeID] = &NodeInfo{
			ID:        nodeID,
			Address:   hb.NodeAddr,
			Status:    NodeStatusFollower,
			LastSeen:  hb.Timestamp,
			IsHealthy: true,
			MissCount: 0,
		}
	}
}
