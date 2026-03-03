package agent

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/skill"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// SkillResolver is a callback that resolves a skill ID to its implementation.
// Registered by the project package at init time to avoid circular imports.
var SkillResolver func(id string) (*skill.Skill, bool)

// RegisterSkillResolver sets the function used to look up skill components by ID.
func RegisterSkillResolver(fn func(id string) (*skill.Skill, bool)) {
	SkillResolver = fn
}

type AgentConfig struct {
	Model        string                `yaml:"model"`
	Temperature  float64               `yaml:"temperature"`
	MaxTokens    int                   `yaml:"max_tokens"`
	SystemPrompt string                `yaml:"system_prompt"`
	Skills       []string              `yaml:"skills"`
	Tools        interface{}           `yaml:"tools"` // "all" or []string
	Batch        AgentBatchConfig      `yaml:"batch"`
	Distributed  AgentDistributedConfig `yaml:"distributed"`
	RawConfig    string                `yaml:"-"`
	Path         string                `yaml:"-"`
}

type AgentBatchConfig struct {
	Size      int    `yaml:"size"`
	Timeout   string `yaml:"timeout"`
	MaxRounds int    `yaml:"max_rounds"`
}

// AgentDistributedConfig controls how the agent behaves across multiple HUB instances.
type AgentDistributedConfig struct {
	// Mode: "independent" (default) - each instance processes its own stream independently.
	//       "leader_only" - only the cluster leader runs this agent.
	Mode string `yaml:"mode"`
	// RateLimitRPS: per-instance LLM request rate limit (requests per second). 0 = unlimited.
	RateLimitRPS float64 `yaml:"rate_limit_rps"`
}

type Agent struct {
	Id              string        `json:"id"`
	Status          common.Status `json:"status"`
	StatusChangedAt *time.Time    `json:"status_changed_at,omitempty"`
	Err             error         `json:"-"`
	Config          *AgentConfig  `json:"config"`
	Path            string        `json:"path"`

	UpStream   map[string]*chan map[string]interface{} `json:"-"`
	DownStream map[string]*chan map[string]interface{} `json:"-"`

	stopChan chan struct{}
	wg       sync.WaitGroup

	skills            map[string]*SkillAdapter
	toolDefs          []ToolDefinition
	processTotal      uint64
	sampler           *common.Sampler
	rateLimitInterval time.Duration
	RawConfig         string `json:"-"`

	ProjectNodeSequence string `json:"project_node_sequence,omitempty"`
}

func NewAgent(filePath, raw, id string) (*Agent, error) {
	if err := Verify(filePath, raw); err != nil {
		return nil, err
	}

	configData, err := loadConfig(filePath, raw)
	if err != nil {
		return nil, err
	}

	var cfg AgentConfig
	if err := yaml.Unmarshal([]byte(configData), &cfg); err != nil {
		return nil, fmt.Errorf("parse agent config: %w", err)
	}

	cfg.RawConfig = configData
	cfg.Path = filePath
	applyDefaults(&cfg)

	a := &Agent{
		Id:         id,
		Status:     common.StatusStopped,
		Config:     &cfg,
		Path:       filePath,
		UpStream:   make(map[string]*chan map[string]interface{}),
		DownStream: make(map[string]*chan map[string]interface{}),
		skills:     make(map[string]*SkillAdapter),
		RawConfig:  configData,
	}

	if err := a.resolveSkills(); err != nil {
		return nil, fmt.Errorf("resolve skills: %w", err)
	}

	a.toolDefs = buildPluginToolDefinitions(cfg.Tools)

	return a, nil
}

// NewFromExisting creates a new Agent instance from an existing one for a given PNS.
func NewFromExisting(existing *Agent, pns string) (*Agent, error) {
	if existing == nil {
		return nil, fmt.Errorf("existing agent is nil")
	}

	if err := Verify(existing.Path, existing.RawConfig); err != nil {
		return nil, err
	}

	a := &Agent{
		Id:                  existing.Id,
		Status:              common.StatusStopped,
		Config:              existing.Config,
		Path:                existing.Path,
		UpStream:            make(map[string]*chan map[string]interface{}),
		DownStream:          make(map[string]*chan map[string]interface{}),
		skills:              make(map[string]*SkillAdapter),
		RawConfig:           existing.RawConfig,
		ProjectNodeSequence: pns,
	}

	if err := a.resolveSkills(); err != nil {
		return nil, fmt.Errorf("resolve skills: %w", err)
	}

	a.toolDefs = buildPluginToolDefinitions(a.Config.Tools)

	return a, nil
}

func Verify(filePath, raw string) error {
	configData, err := loadConfig(filePath, raw)
	if err != nil {
		return err
	}

	var cfg AgentConfig
	if err := yaml.Unmarshal([]byte(configData), &cfg); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		return fmt.Errorf("system_prompt is required")
	}

	if SkillResolver != nil {
		for _, sk := range cfg.Skills {
			if _, ok := SkillResolver(sk); !ok {
				return fmt.Errorf("unknown skill: %s", sk)
			}
		}
	}

	if cfg.Batch.Timeout != "" {
		if _, err := time.ParseDuration(cfg.Batch.Timeout); err != nil {
			return fmt.Errorf("invalid batch.timeout: %w", err)
		}
	}

	return nil
}

func (a *Agent) SetStatus(status common.Status, err error) {
	a.Err = err
	a.Status = status
	t := time.Now()
	a.StatusChangedAt = &t
}

func (a *Agent) GetProcessTotal() uint64 {
	return atomic.LoadUint64(&a.processTotal)
}

func (a *Agent) resolveSkills() error {
	if SkillResolver == nil {
		if len(a.Config.Skills) > 0 {
			return fmt.Errorf("skill resolver not registered, cannot resolve skills")
		}
		return nil
	}

	for _, skillID := range a.Config.Skills {
		sk, ok := SkillResolver(skillID)
		if !ok {
			return fmt.Errorf("skill not found: %s", skillID)
		}
		impl := sk.Impl()
		if impl == nil {
			return fmt.Errorf("skill %s has no implementation", skillID)
		}
		a.skills[skillID] = newSkillAdapter(impl)
	}
	return nil
}

func loadConfig(filePath, raw string) (string, error) {
	if raw != "" {
		return raw, nil
	}
	if filePath == "" {
		return "", fmt.Errorf("no config source: both path and raw are empty")
	}
	data, err := readFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read agent config %s: %w", filePath, err)
	}
	return string(data), nil
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func applyDefaults(cfg *AgentConfig) {
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.3
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.Batch.Size == 0 {
		cfg.Batch.Size = 10
	}
	if cfg.Batch.Timeout == "" {
		cfg.Batch.Timeout = "30s"
	}
	if cfg.Batch.MaxRounds == 0 {
		cfg.Batch.MaxRounds = 5
	}
	if cfg.Distributed.Mode == "" {
		cfg.Distributed.Mode = "independent"
	}
}

func (a *Agent) buildAllToolDefinitions() []ToolDefinition {
	var defs []ToolDefinition

	for skillID, sk := range a.skills {
		for _, fn := range sk.Functions() {
			def := fn
			def.Function.Name = "skill__" + skillID + "__" + fn.Function.Name
			defs = append(defs, def)
		}
	}

	defs = append(defs, a.toolDefs...)

	return defs
}

func jsonError(msg string) string {
	data, _ := json.Marshal(map[string]string{"error": msg})
	return string(data)
}

func (a *Agent) executeFunctionCall(call ToolCall) string {
	name := call.Function.Name
	args := parseJSONArgs(call.Function.Arguments)

	if strings.HasPrefix(name, "skill__") {
		trimmed := strings.TrimPrefix(name, "skill__")
		parts := strings.SplitN(trimmed, "__", 2)
		if len(parts) == 2 {
			skillID := parts[0]
			funcName := parts[1]
			if sk, ok := a.skills[skillID]; ok {
				result, err := sk.Execute(funcName, args)
				if err != nil {
					logger.Error("Skill execution failed", "skill", skillID, "function", funcName, "error", err)
					return jsonError(err.Error())
				}
				return result
			}
		}
		return jsonError("skill not found")
	}

	if strings.HasPrefix(name, "tool_") {
		pluginName := strings.TrimPrefix(name, "tool_")
		result, err := executePlugin(pluginName, args)
		if err != nil {
			logger.Error("Tool execution failed", "plugin", pluginName, "error", err)
			return jsonError(err.Error())
		}
		return result
	}

	return jsonError("unknown function")
}
