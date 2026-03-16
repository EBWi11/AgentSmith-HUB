package memory

import "time"

type Scope struct {
	AgentID             string   `yaml:"agent_id" json:"agent_id"`
	ProjectID           string   `yaml:"project_id,omitempty" json:"project_id,omitempty"`
	ProjectNodeSequence string   `yaml:"project_node_sequence" json:"project_node_sequence"`
	InputIDs            []string `yaml:"input_ids,omitempty" json:"input_ids,omitempty"`
}

type Summary struct {
	Category          string    `yaml:"category,omitempty" json:"category,omitempty"`
	Summary           string    `yaml:"summary" json:"summary"`
	Confidence        float64   `yaml:"confidence,omitempty" json:"confidence,omitempty"`
	SourceOperationID string    `yaml:"source_operation_id,omitempty" json:"source_operation_id,omitempty"`
	UpdatedAt         time.Time `yaml:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type RecentRevert struct {
	OperationID       string    `yaml:"operation_id,omitempty" json:"operation_id,omitempty"`
	RevertOperationID string    `yaml:"revert_operation_id,omitempty" json:"revert_operation_id,omitempty"`
	RulesetID         string    `yaml:"ruleset_id,omitempty" json:"ruleset_id,omitempty"`
	RuleID            string    `yaml:"rule_id,omitempty" json:"rule_id,omitempty"`
	Reason            string    `yaml:"reason,omitempty" json:"reason,omitempty"`
	SourceOperationID string    `yaml:"source_operation_id,omitempty" json:"source_operation_id,omitempty"`
	CreatedAt         time.Time `yaml:"created_at,omitempty" json:"created_at,omitempty"`
}

type Config struct {
	Scope             Scope          `yaml:"scope" json:"scope"`
	UpdatedAt         time.Time      `yaml:"updated_at" json:"updated_at"`
	Version           int            `yaml:"version" json:"version"`
	Summaries         []Summary      `yaml:"summaries,omitempty" json:"summaries,omitempty"`
	AvoidPatterns     []string       `yaml:"avoid_patterns,omitempty" json:"avoid_patterns,omitempty"`
	PreferredPatterns []string       `yaml:"preferred_patterns,omitempty" json:"preferred_patterns,omitempty"`
	Signals           []string       `yaml:"signals,omitempty" json:"signals,omitempty"`
	RecentReverts     []RecentRevert `yaml:"recent_reverts,omitempty" json:"recent_reverts,omitempty"`
}

type ExtractorResult struct {
	Summary           string   `json:"summary"`
	Category          string   `json:"category,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
	Signals           []string `json:"signals,omitempty"`
	AvoidPatterns     []string `json:"avoid_patterns,omitempty"`
	PreferredPatterns []string `json:"preferred_patterns,omitempty"`
	InputIDs          []string `json:"input_ids,omitempty"`
}
