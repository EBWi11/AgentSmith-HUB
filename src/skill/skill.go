package skill

import (
	"AgentSmith-HUB/common"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SkillConfig is the YAML configuration for a skill component.
type SkillConfig struct {
	Type        string                 `yaml:"type"`        // "builtin" or "knowledge"
	BuiltinRef  string                 `yaml:"builtin_ref"` // for type=builtin: reference to Go implementation
	Description string                 `yaml:"description"`
	Content     string                 `yaml:"content"`     // for type=knowledge: the reference/prompt text
	Config      map[string]interface{} `yaml:"config"`
}

// Skill is a top-level component (like Input, Ruleset, Agent).
type Skill struct {
	Id              string        `json:"id"`
	Status          common.Status `json:"status"`
	StatusChangedAt *time.Time    `json:"status_changed_at,omitempty"`
	Err             error         `json:"-"`
	Path            string        `json:"path"`
	RawConfig       string        `json:"-"`
	SkillConfig     *SkillConfig  `json:"config"`

	impl SkillImplementation
}

// SkillImplementation is the interface that every skill backend must satisfy.
// Agents consume skills through this interface.
type SkillImplementation interface {
	Name() string
	Description() string
	Functions() []ToolDefinition
	Execute(functionName string, arguments map[string]interface{}) (string, error)
}

// ToolDefinition / FunctionDef mirror the agent LLM types so that
// the skill package is self-contained (avoids importing agent).
type ToolDefinition struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Impl returns the underlying implementation for agent consumption.
func (s *Skill) Impl() SkillImplementation {
	return s.impl
}

func NewSkill(filePath, raw, id string) (*Skill, error) {
	if err := Verify(filePath, raw); err != nil {
		return nil, err
	}

	configData, err := loadConfig(filePath, raw)
	if err != nil {
		return nil, err
	}

	var cfg SkillConfig
	if err := yaml.Unmarshal([]byte(configData), &cfg); err != nil {
		return nil, fmt.Errorf("parse skill config: %w", err)
	}

	s := &Skill{
		Id:          id,
		Status:      common.StatusStopped,
		Path:        filePath,
		RawConfig:   configData,
		SkillConfig: &cfg,
	}

	if err := s.resolveImplementation(); err != nil {
		return nil, fmt.Errorf("resolve skill implementation: %w", err)
	}

	s.SetStatus(common.StatusStopped, nil)
	return s, nil
}

func Verify(filePath, raw string) error {
	configData, err := loadConfig(filePath, raw)
	if err != nil {
		return err
	}

	var cfg SkillConfig
	if err := yaml.Unmarshal([]byte(configData), &cfg); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	if cfg.Type == "" {
		return fmt.Errorf("type is required (builtin or knowledge)")
	}

	switch cfg.Type {
	case "builtin":
		if cfg.BuiltinRef == "" {
			return fmt.Errorf("builtin_ref is required when type is builtin")
		}
		if _, ok := builtinRegistry[cfg.BuiltinRef]; !ok {
			available := make([]string, 0, len(builtinRegistry))
			for k := range builtinRegistry {
				available = append(available, k)
			}
			return fmt.Errorf("unknown builtin_ref: %s (available: %v)", cfg.BuiltinRef, available)
		}
	case "knowledge":
		if strings.TrimSpace(cfg.Content) == "" {
			return fmt.Errorf("content is required when type is knowledge")
		}
	default:
		return fmt.Errorf("unsupported skill type: %s (supported: builtin, knowledge)", cfg.Type)
	}

	return nil
}

func (s *Skill) SetStatus(status common.Status, err error) {
	s.Err = err
	s.Status = status
	t := time.Now()
	s.StatusChangedAt = &t
}

func (s *Skill) resolveImplementation() error {
	cfg := s.SkillConfig

	switch cfg.Type {
	case "builtin":
		factory, ok := builtinRegistry[cfg.BuiltinRef]
		if !ok {
			return fmt.Errorf("unknown builtin skill: %s", cfg.BuiltinRef)
		}
		s.impl = factory(cfg.Config)
		return nil
	case "knowledge":
		s.impl = newKnowledgeSkill(s.Id, cfg.Description, cfg.Content)
		return nil
	default:
		return fmt.Errorf("unsupported skill type: %s", cfg.Type)
	}
}

func loadConfig(filePath, raw string) (string, error) {
	if raw != "" {
		return raw, nil
	}
	if filePath == "" {
		return "", fmt.Errorf("no config source: both path and raw are empty")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read skill config %s: %w", filePath, err)
	}
	return string(data), nil
}
