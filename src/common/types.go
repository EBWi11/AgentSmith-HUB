package common

import (
	"sync"
	"time"
)

type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusError    Status = "error"
)

// CheckCoreCache for rule engine
type CheckCoreCache struct {
	Exist     bool
	Data      string      // String representation (for backward compatibility)
	TypedData interface{} // Original typed data (for type-preserving access)
}

type HubConfig struct {
	Redis         string `yaml:"redis"`
	RedisPassword string `yaml:"redis_password,omitempty"`
	PprofEnable   bool   `yaml:"pprof_enable"`
	PprofPort     string `yaml:"pprof_port"`
	APIPort       string // populated at runtime from -api_listen flag (not from yaml)
	SIMDEnabled   bool   `yaml:"simd_enabled"`
	ConfigRoot    string
	Leader        string
	LocalIP       string
	Token         string
	// OIDC/OAuth2 configuration
	OIDCEnabled       bool     `yaml:"oidc_enabled"`
	OIDCIssuer        string   `yaml:"oidc_issuer"`
	OIDCClientID      string   `yaml:"oidc_client_id"`
	OIDCUsernameClaim string   `yaml:"oidc_username_claim"`
	OIDCAllowedUsers  []string `yaml:"oidc_allowed_users"`
	OIDCRedirectURI   string   `yaml:"oidc_redirect_uri"`
	OIDCScope         string   `yaml:"oidc_scope"`
	// LLM plugin (builtin): if llm_api_key is set, llmCall plugin is registered
	LLMApiKey  string `yaml:"llm_api_key,omitempty"`
	LLMBaseURL string `yaml:"llm_base_url,omitempty"`
	LLMModel   string `yaml:"llm_model,omitempty"`
}

// Operation types for project operations
type OperationType string

const (
	OpTypeChangePush      OperationType = "change_push"
	OpTypeLocalPush       OperationType = "local_push"
	OpTypeComponentDelete OperationType = "component_delete"
	OpTypeComponentAdd    OperationType = "component_add"    // New: for component addition
	OpTypeComponentUpdate OperationType = "component_update" // New: for component update
	OpTypeProjectStart    OperationType = "project_start"
	OpTypeProjectStop     OperationType = "project_stop"
	OpTypeProjectRestart  OperationType = "project_restart"
	// Cluster instruction operations
	OpTypeInstructionPublish OperationType = "instruction_publish" // Leader发布指令
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

// Project state Redis keys - IMPORTANT: Separate expected vs actual states
const (
	// Project real state (actual runtime status) - stores the real current status per node
	// This represents what the project actually is (running, stopped, error, starting, stopping)
	// Format: cluster:proj_real:{nodeID} -> {projectID: "running|stopped|error|starting|stopping"}
	ProjectRealStateKeyPrefix = "cluster:proj_real:" // + nodeID

	// Project state change timestamps per node
	// Format: cluster:proj_ts:{nodeID} -> {projectID: "2023-12-01T10:00:00Z"}
	ProjectStateTimestampKeyPrefix = "cluster:proj_ts:" // + nodeID

	// User intention (what user wants the project to be) - GLOBAL, shared across all nodes
	// This represents user's expected state (from API calls: start/stop)
	// Format: cluster:proj_states: -> {projectID: "running"} (only "running" is stored, "stopped" projects have their keys removed)
	// Note: This key does NOT include nodeID - it's a single global hash shared by all nodes
	ProjectLegacyStateKeyPrefix = "cluster:proj_states:"
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
