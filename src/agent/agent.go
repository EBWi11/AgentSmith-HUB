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
	Model        string      `yaml:"model"`
	Temperature  float64     `yaml:"temperature"`
	MaxTokens    int         `yaml:"max_tokens"`
	SystemPrompt string      `yaml:"system_prompt"`
	Skills       []string    `yaml:"skills"`
	Tools        interface{} `yaml:"tools"` // "all" or []string
	MaxRounds    int         `yaml:"max_rounds"`
	Timeout      string      `yaml:"timeout"`

	// Reasoning / thinking configuration for models that support it (e.g. kimi-k2.5).
	// reasoning_mode:
	//   - "disabled" (default): never send provider-specific reasoning params
	//   - "enabled"           : always send reasoning params for supported models
	//   - "auto"              : enable reasoning based on model name heuristics
	ReasoningMode         string `yaml:"reasoning_mode"`
	ReasoningBudgetTokens int    `yaml:"reasoning_budget_tokens"`

	RawConfig string `yaml:"-"`
	Path      string `yaml:"-"`
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

	skills           map[string]*SkillAdapter
	toolDefs         []ToolDefinition
	processTotal     uint64
	processLatencyNs uint64 // cumulative nanoseconds for processed messages (for avg latency)
	// lastReportedTotal is used for incremental statistics collection (QPS / MSG/D)
	lastReportedTotal uint64
	sampler           *common.Sampler
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

// ParseSkillIDsFromRaw parses agent raw YAML and returns the skill ID list (for reference checks).
func ParseSkillIDsFromRaw(raw string) ([]string, error) {
	var cfg struct {
		Skills []string `yaml:"skills"`
	}
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	return cfg.Skills, nil
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

// GetIncrementAndUpdate returns the increment in processed messages since last call
// and updates the baseline. This mirrors the behavior of Input/Output/Ruleset
// components so that the daily stats manager can treat agents uniformly.
func (a *Agent) GetIncrementAndUpdate() uint64 {
	current := atomic.LoadUint64(&a.processTotal)
	last := atomic.LoadUint64(&a.lastReportedTotal)

	// Use CAS to atomically update lastReportedTotal
	// If CAS fails, we simply return 0 - one missed stat collection is not critical
	if atomic.CompareAndSwapUint64(&a.lastReportedTotal, last, current) {
		return current - last
	}
	return 0
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
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.MaxRounds == 0 {
		cfg.MaxRounds = 5
	}
	if strings.TrimSpace(cfg.Timeout) == "" {
		cfg.Timeout = "60s"
	}

	// Normalize reasoning mode; default to "disabled" for safety if empty.
	cfg.ReasoningMode = strings.ToLower(strings.TrimSpace(cfg.ReasoningMode))
	if cfg.ReasoningMode == "" {
		cfg.ReasoningMode = "disabled"
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
	toolLogger := logger.GetAgentLogger()
	name := call.Function.Name
	rawArgs := call.Function.Arguments
	args := parseJSONArgs(rawArgs)

	if strings.HasPrefix(name, "skill__") {
		trimmed := strings.TrimPrefix(name, "skill__")
		parts := strings.SplitN(trimmed, "__", 2)
		if len(parts) == 2 {
			skillID := parts[0]
			funcName := parts[1]
			if sk, ok := a.skills[skillID]; ok {
				start := time.Now()
				toolLogger.Info("Agent tool call start",
					"agent", a.Id,
					"project_node_sequence", a.ProjectNodeSequence,
					"kind", "skill",
					"skill", skillID,
					"function", funcName,
					"tool_call_id", call.ID,
					"args", truncateForLog(rawArgs),
				)

				result, err := sk.Execute(funcName, args)
				if err != nil {
					toolLogger.Error("Skill execution failed",
						"agent", a.Id,
						"project_node_sequence", a.ProjectNodeSequence,
						"skill", skillID,
						"function", funcName,
						"tool_call_id", call.ID,
						"error", err,
						"duration_ms", time.Since(start).Milliseconds(),
					)
					return jsonError(err.Error())
				}
				toolLogger.Info("Agent tool call success",
					"agent", a.Id,
					"project_node_sequence", a.ProjectNodeSequence,
					"kind", "skill",
					"skill", skillID,
					"function", funcName,
					"tool_call_id", call.ID,
					"duration_ms", time.Since(start).Milliseconds(),
					"result", truncateForLog(result),
				)
				return result
			}
		}
		return jsonError("skill not found")
	}

	if strings.HasPrefix(name, "tool_") {
		pluginName := strings.TrimPrefix(name, "tool_")
		start := time.Now()
		toolLogger.Info("Agent tool call start",
			"agent", a.Id,
			"project_node_sequence", a.ProjectNodeSequence,
			"kind", "plugin",
			"plugin", pluginName,
			"tool_call_id", call.ID,
			"args", truncateForLog(rawArgs),
		)

		var extraArgs []interface{}
		if pluginName == "addRule" {
			agentContextRaw, _ := json.Marshal(common.AgentOperationContext{
				AgentID:             a.Id,
				AgentRunID:          common.NewOperationID(),
				ToolName:            pluginName,
				ToolCallID:          call.ID,
				ProjectNodeSequence: a.ProjectNodeSequence,
			})
			extraArgs = append(extraArgs, string(agentContextRaw))
		}

		result, err := executePlugin(pluginName, args, extraArgs...)
		if err != nil {
			toolLogger.Error("Tool execution failed",
				"agent", a.Id,
				"project_node_sequence", a.ProjectNodeSequence,
				"plugin", pluginName,
				"tool_call_id", call.ID,
				"error", err,
				"duration_ms", time.Since(start).Milliseconds(),
			)
			return jsonError(err.Error())
		}
		toolLogger.Info("Agent tool call success",
			"agent", a.Id,
			"project_node_sequence", a.ProjectNodeSequence,
			"kind", "plugin",
			"plugin", pluginName,
			"tool_call_id", call.ID,
			"duration_ms", time.Since(start).Milliseconds(),
			"result", truncateForLog(result),
		)
		return result
	}

	return jsonError("unknown function")
}

const maxToolLogPayloadLen = 2048

// truncateForLog limits long argument / result payloads so logs remain readable.
func truncateForLog(s string) string {
	if len(s) <= maxToolLogPayloadLen {
		return s
	}
	return s[:maxToolLogPayloadLen] + fmt.Sprintf("... (truncated, %d bytes total)", len(s))
}
