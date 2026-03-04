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
	Skills    []string    `yaml:"skills"`
	Tools     interface{} `yaml:"tools"` // "all" or []string
	MaxRounds int         `yaml:"max_rounds"`
	Timeout   string      `yaml:"timeout"`
	RawConfig string      `yaml:"-"`
	Path      string      `yaml:"-"`
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
	processLatencyNs  uint64 // cumulative nanoseconds for processed messages (for avg latency)
	sampler           *common.Sampler
	RawConfig         string `json:"-"`

	// Daily stats (reset when date changes); protected by dailyMu
	dailyMu        sync.Mutex
	dailyDate      string
	dailyCount     uint64
	dailyLatencyNs uint64

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

	if cfg.Timeout != "" {
		if _, err := time.ParseDuration(cfg.Timeout); err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
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

// GetAvgLatencyMs returns average processing latency in milliseconds (0 if no messages processed).
func (a *Agent) GetAvgLatencyMs() float64 {
	total := atomic.LoadUint64(&a.processTotal)
	if total == 0 {
		return 0
	}
	ns := atomic.LoadUint64(&a.processLatencyNs)
	return float64(ns) / float64(total) / 1e6
}

// RecordDailyStats adds one call and latency to today's daily stats. Call from processAndForward.
func (a *Agent) RecordDailyStats(latencyNs uint64) {
	today := time.Now().Format("2006-01-02")
	a.dailyMu.Lock()
	defer a.dailyMu.Unlock()
	if a.dailyDate != today {
		a.dailyDate = today
		a.dailyCount = 0
		a.dailyLatencyNs = 0
	}
	a.dailyCount++
	a.dailyLatencyNs += latencyNs
}

// GetDailyCallCount returns the number of agent calls today (0 if none or date changed).
func (a *Agent) GetDailyCallCount() uint64 {
	today := time.Now().Format("2006-01-02")
	a.dailyMu.Lock()
	defer a.dailyMu.Unlock()
	if a.dailyDate != today {
		return 0
	}
	return a.dailyCount
}

// GetDailyAvgLatencyMs returns average latency in ms for today's calls (0 if none).
func (a *Agent) GetDailyAvgLatencyMs() float64 {
	today := time.Now().Format("2006-01-02")
	a.dailyMu.Lock()
	defer a.dailyMu.Unlock()
	if a.dailyDate != today || a.dailyCount == 0 {
		return 0
	}
	return float64(a.dailyLatencyNs) / float64(a.dailyCount) / 1e6
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
	if cfg.MaxRounds == 0 {
		cfg.MaxRounds = 5
	}
	if cfg.Timeout == "" {
		cfg.Timeout = "30s"
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
