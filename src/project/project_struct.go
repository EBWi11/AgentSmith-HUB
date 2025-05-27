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
	ProjectStatusStopped ProjectStatus = "stopped"
	ProjectStatusRunning ProjectStatus = "running"
	ProjectStatusError   ProjectStatus = "error"
)

type GlobalProjectInfo struct {
	msgChans        map[string]chan map[string]interface{}
	msgChansCounter map[string]int
}

// ProjectConfig holds the configuration for a project
type ProjectConfig struct {
	Name    string `yaml:"name"`
	Id      string `yaml:"id"`
	Content string `yaml:"content"`
}

// Project represents a data processing project with inputs, outputs, and rules
type Project struct {
	// Basic info
	Name   string
	Id     string
	Status ProjectStatus
	Config *ProjectConfig

	// Components
	Inputs   map[string]*input.Input
	Outputs  map[string]*output.Output
	Rulesets map[string]*rules_engine.Ruleset

	MsgChannels []string

	// Runtime
	stopChan    chan struct{}
	wg          sync.WaitGroup
	errorChan   chan error
	lastError   error
	lastErrorMu sync.RWMutex
	startTime   time.Time
	metrics     *ProjectMetrics
	metricsStop chan struct{}
}

// ProjectMetrics holds runtime metrics for the project
type ProjectMetrics struct {
	InputQPS    map[string]uint64
	OutputQPS   map[string]uint64
	ProcessQPS  uint64
	TotalInput  uint64
	TotalOutput uint64
	mu          sync.RWMutex
}

// ProjectNode interface
type ProjectNode interface {
	Start() error
	Stop() error
}
