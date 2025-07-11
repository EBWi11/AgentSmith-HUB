package common

import (
	"sync"
	"time"
)

// CheckCoreCache for rule engine
type CheckCoreCache struct {
	Exist bool
	Data  string
}

type HubConfig struct {
	Redis         string `yaml:"redis"`
	RedisPassword string `yaml:"redis_password,omitempty"`
	ConfigRoot    string
	Leader        string
	LocalIP       string
	Token         string
}

// Operation types for project operations
type OperationType string

const (
	OpTypeChangePush     OperationType = "change_push"
	OpTypeLocalPush      OperationType = "local_push"
	OpTypeProjectStart   OperationType = "project_start"
	OpTypeProjectStop    OperationType = "project_stop"
	OpTypeProjectRestart OperationType = "project_restart"
)

// OperationRecord represents a single operation record
type OperationRecord struct {
	Type          OperationType          `json:"type"`
	Timestamp     time.Time              `json:"timestamp"`
	ComponentType string                 `json:"component_type,omitempty"`
	ComponentID   string                 `json:"component_id,omitempty"`
	ProjectID     string                 `json:"project_id,omitempty"`
	Diff          string                 `json:"diff,omitempty"`
	OldContent    string                 `json:"old_content,omitempty"`
	NewContent    string                 `json:"new_content,omitempty"`
	Status        string                 `json:"status"`
	Error         string                 `json:"error,omitempty"`
	Details       map[string]interface{} `json:"details,omitempty"`
}

// Cluster startup coordination constants
const (
	ClusterLeaderReadyKey    = "cluster:leader:ready"
	ClusterStartupTimeoutSec = 60 // Wait up to 60 seconds for leader to be ready
)

// StartupCoordinator manages cluster startup coordination
type StartupCoordinator struct {
	isLeader     bool
	leaderReady  bool
	startupMutex sync.RWMutex
}

// Component update states
type ComponentUpdateState int

const (
	UpdateStateIdle ComponentUpdateState = iota
	UpdateStatePreparing
	UpdateStateUpdating
	UpdateStateCompleting
	UpdateStateFailed
)

// ComponentUpdateManager manages component update operations
type ComponentUpdateManager struct {
	activeUpdates map[string]*ComponentUpdateOperation
	mutex         sync.RWMutex
}

// ComponentUpdateOperation represents an ongoing component update
type ComponentUpdateOperation struct {
	ComponentType    string
	ComponentID      string
	State            ComponentUpdateState
	StartTime        time.Time
	LastUpdate       time.Time
	AffectedProjects []string
	Lock             *DistributedLock
	mutex            sync.RWMutex
}

// Global component update manager
var GlobalComponentUpdateManager *ComponentUpdateManager
