package project

import (
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/rules_engine"
	"sync"
)

// ProjectStatus represents the current status of a project
type ProjectStatus string

const (
	ProjectStatusStopped ProjectStatus = "stopped"
	ProjectStatusRunning ProjectStatus = "running"
	ProjectStatusError   ProjectStatus = "error"
)

type GlobalProjectInfo struct {
	Projects        map[string]*Project
	msgChans        map[string]chan map[string]interface{}
	msgChansCounter map[string]int
}

// ProjectConfig holds the configuration for a project
type ProjectConfig struct {
	Id        string
	Content   string `yaml:"content"`
	RawConfig string
}

// Project represents a data processing project with inputs, outputs, and rules
type Project struct {
	// Basic info
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
	metrics     *ProjectMetrics
	metricsStop chan struct{}

	Err error
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
