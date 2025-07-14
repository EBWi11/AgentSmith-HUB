package project

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/rules_engine"
	"sync"
	"time"
)

type FlowNode struct {
	FromPNS  string
	ToPNS    string
	Content  string
	FromType string
	ToType   string
	FromID   string
	ToID     string
	FromInit bool
	ToInit   bool
}

type GlobalProjectInfo struct {
	Projects map[string]*Project
	Inputs   map[string]*input.Input
	Outputs  map[string]*output.Output
	Rulesets map[string]*rules_engine.Ruleset

	PNSOutputs  map[string]*output.Output
	PNSRulesets map[string]*rules_engine.Ruleset

	ProjectsNew map[string]string
	InputsNew   map[string]string
	OutputsNew  map[string]string
	RulesetsNew map[string]string

	RefCount map[string]int
}

func GetRefCount(id string) int {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	return GlobalProject.RefCount[id]
}

func AddRefCount(id string) {
	common.GlobalMu.Lock()
	GlobalProject.RefCount[id] = GlobalProject.RefCount[id] + 1
	common.GlobalMu.Unlock()
}

func ReduceRefCount(id string) {
	common.GlobalMu.Lock()
	GlobalProject.RefCount[id] = GlobalProject.RefCount[id] - 1
	common.GlobalMu.Unlock()
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
	Status          common.Status `json:"status"`
	StatusChangedAt *time.Time    `json:"status_changed_at,omitempty"`
	Err             error         `json:"-"`

	Testing bool `json:"testing"`

	Config *ProjectConfig `json:"config"`

	FlowNodes []FlowNode

	// Components
	Inputs   map[string]*input.Input          `json:"-"`
	Outputs  map[string]*output.Output        `json:"-"`
	Rulesets map[string]*rules_engine.Ruleset `json:"-"`

	// Data flow
	MsgChannels map[string]*chan map[string]interface{} `json:"-"` // Channels for message passing between components
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
