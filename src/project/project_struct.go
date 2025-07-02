package project

import (
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/rules_engine"
	"sync"
	"time"
)

// ProjectStatus represents the current status of a project
type ProjectStatus string

const (
	ProjectStatusStopped  ProjectStatus = "stopped"
	ProjectStatusStarting ProjectStatus = "starting"
	ProjectStatusRunning  ProjectStatus = "running"
	ProjectStatusStopping ProjectStatus = "stopping"
	ProjectStatusError    ProjectStatus = "error"
)

type GlobalProjectInfo struct {
	Projects map[string]*Project
	Inputs   map[string]*input.Input
	Outputs  map[string]*output.Output
	Rulesets map[string]*rules_engine.Ruleset

	ProjectsNew map[string]string
	InputsNew   map[string]string
	OutputsNew  map[string]string
	RulesetsNew map[string]string

	msgChans        map[string]chan map[string]interface{}
	msgChansCounter map[string]int

	// Dedicated lock for project lifecycle management to reduce lock contention
	ProjectMu sync.RWMutex
}

// ProjectConfig holds the configuration for a project
type ProjectConfig struct {
	Id        string
	Content   string `yaml:"content"`
	RawConfig string
	Path      string
}

// Project represents a project
type Project struct {
	Id              string        `json:"id"`
	Status          ProjectStatus `json:"status"`
	StatusChangedAt *time.Time    `json:"status_changed_at,omitempty"`
	Err             error         `json:"-"`

	Config *ProjectConfig `json:"config"`

	// Components
	Inputs   map[string]*input.Input          `json:"-"`
	Outputs  map[string]*output.Output        `json:"-"`
	Rulesets map[string]*rules_engine.Ruleset `json:"-"`

	// Data flow
	MsgChannels []string `json:"-"`

	// Metrics
	metrics     *ProjectMetrics `json:"-"`
	metricsStop chan struct{}   `json:"-"`

	// For graceful shutdown
	stopChan chan struct{}  `json:"-"`
	wg       sync.WaitGroup `json:"-"`

	// Dependencies tracking
	DependsOn      []string `json:"-"` // Projects this project depends on
	DependedBy     []string `json:"-"` // Projects that depend on this project
	SharedInputs   []string `json:"-"` // Inputs shared with other projects
	SharedOutputs  []string `json:"-"` // Outputs shared with other projects
	SharedRulesets []string `json:"-"` // Rulesets shared with other projects
}

// ProjectMetrics holds runtime metrics for the project
type ProjectMetrics struct {
	InputQPS  map[string]uint64
	OutputQPS map[string]uint64
	mu        sync.RWMutex
}

// ProjectNode interface
type ProjectNode interface {
	Start() error
	Stop() error
}
