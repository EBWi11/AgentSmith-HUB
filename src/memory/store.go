package memory

import (
	"AgentSmith-HUB/common"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	memoryDirName           = "memory"
	memoryPNSDirName        = "pns"
	maxRecentFeedbackPerPNS = 20
	maxSummariesPerPNS      = 20
	maxAvoidPatternsPerPNS  = 24
	maxPreferredPerPNS      = 24
	maxSignalsPerPNS        = 24
	maxInputIDsPerPNS       = 12
)

func safePNSFileName(pns string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return replacer.Replace(strings.TrimSpace(pns))
}

func PNSMemoryDir() string {
	return filepath.Join(common.Config.ConfigRoot, memoryDirName, memoryPNSDirName)
}

func PNSMemoryPath(pns string) string {
	return filepath.Join(PNSMemoryDir(), safePNSFileName(pns)+".yaml")
}

func LoadPNSMemory(pns string) (*Config, error) {
	pns = strings.TrimSpace(pns)
	if pns == "" {
		return nil, nil
	}

	path := PNSMemoryPath(pns)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read memory file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse memory file %s: %w", path, err)
	}
	if cfg.Scope.ProjectNodeSequence == "" {
		cfg.Scope.ProjectNodeSequence = pns
	}
	normalizeConfig(&cfg)
	return &cfg, nil
}

func LoadPNSMemoryRaw(pns string) (string, bool, error) {
	pns = strings.TrimSpace(pns)
	if pns == "" {
		return "", false, nil
	}

	path := PNSMemoryPath(pns)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read memory file %s: %w", path, err)
	}
	return string(raw), true, nil
}

func ParseConfig(raw string) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	normalizeConfig(&cfg)
	return &cfg, nil
}

func MarshalConfig(cfg *Config) (string, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func SavePNSMemory(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("memory config is nil")
	}
	if strings.TrimSpace(cfg.Scope.ProjectNodeSequence) == "" {
		return fmt.Errorf("memory scope.project_node_sequence is required")
	}

	cfg.UpdatedAt = time.Now()
	if cfg.Version <= 0 {
		cfg.Version = 1
	}
	normalizeConfig(cfg)

	if err := os.MkdirAll(PNSMemoryDir(), 0755); err != nil {
		return fmt.Errorf("ensure memory dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal memory yaml: %w", err)
	}

	target := PNSMemoryPath(cfg.Scope.ProjectNodeSequence)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write temp memory file: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return fmt.Errorf("replace memory file: %w", err)
	}
	return nil
}

func SavePNSMemoryRaw(pns string, raw string) error {
	pns = strings.TrimSpace(pns)
	if pns == "" {
		return fmt.Errorf("pns is required")
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		return fmt.Errorf("invalid memory yaml: %w", err)
	}
	if strings.TrimSpace(cfg.Scope.ProjectNodeSequence) == "" {
		cfg.Scope.ProjectNodeSequence = pns
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.Scope.ProjectNodeSequence), pns) {
		return fmt.Errorf("memory scope project_node_sequence mismatch: %s != %s", cfg.Scope.ProjectNodeSequence, pns)
	}
	normalizedRaw, err := MarshalConfig(cfg)
	if err != nil {
		return fmt.Errorf("marshal normalized memory yaml: %w", err)
	}
	if err := os.MkdirAll(PNSMemoryDir(), 0755); err != nil {
		return fmt.Errorf("ensure memory dir: %w", err)
	}
	target := PNSMemoryPath(pns)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte(normalizedRaw), 0644); err != nil {
		return fmt.Errorf("write temp memory file: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return fmt.Errorf("replace memory file: %w", err)
	}
	return nil
}

func DeletePNSMemory(pns string) error {
	pns = strings.TrimSpace(pns)
	if pns == "" {
		return nil
	}
	if err := os.Remove(PNSMemoryPath(pns)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete memory file %s: %w", PNSMemoryPath(pns), err)
	}
	return nil
}

func BuildUpdatedFeedbackConfig(existing *Config, scope Scope, result ExtractorResult, feedback RecentFeedback, sourceOperationID string) *Config {
	cfg := cloneConfig(existing)
	if cfg == nil {
		cfg = &Config{
			Scope:   scope,
			Version: 1,
		}
	} else {
		cfg.Version++
	}

	if cfg.Scope.AgentID == "" {
		cfg.Scope.AgentID = scope.AgentID
	}
	if cfg.Scope.ProjectID == "" {
		cfg.Scope.ProjectID = scope.ProjectID
	}
	if cfg.Scope.ProjectNodeSequence == "" {
		cfg.Scope.ProjectNodeSequence = scope.ProjectNodeSequence
	}
	cfg.Scope.InputIDs = mergeRecentUnique(cfg.Scope.InputIDs, append(result.InputIDs, scope.InputIDs...), maxInputIDsPerPNS)

	if strings.TrimSpace(result.Summary) != "" {
		cfg.Summaries = append([]Summary{{
			Category:          strings.TrimSpace(result.Category),
			Summary:           strings.TrimSpace(result.Summary),
			Confidence:        result.Confidence,
			SourceOperationID: sourceOperationID,
			UpdatedAt:         time.Now(),
		}}, cfg.Summaries...)
	}

	cfg.AvoidPatterns = mergeRecentUnique(cfg.AvoidPatterns, result.AvoidPatterns, maxAvoidPatternsPerPNS)
	cfg.PreferredPatterns = mergeRecentUnique(cfg.PreferredPatterns, result.PreferredPatterns, maxPreferredPerPNS)
	cfg.Signals = mergeRecentUnique(cfg.Signals, result.Signals, maxSignalsPerPNS)

	if feedback.CreatedAt.IsZero() {
		feedback.CreatedAt = time.Now()
	}
	cfg.RecentFeedback = append([]RecentFeedback{feedback}, cfg.RecentFeedback...)
	normalizeConfig(cfg)
	return cfg
}

func uniqueTrimmed(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func trimSummaries(items []Summary) []Summary {
	if len(items) == 0 {
		return nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	if len(items) > maxSummariesPerPNS {
		items = items[:maxSummariesPerPNS]
	}
	return items
}

func trimRecentFeedback(items []RecentFeedback) []RecentFeedback {
	if len(items) == 0 {
		return nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > maxRecentFeedbackPerPNS {
		items = items[:maxRecentFeedbackPerPNS]
	}
	return items
}

func trimStringList(items []string, max int) []string {
	if len(items) == 0 {
		return nil
	}
	if len(items) > max {
		return items[:max]
	}
	return items
}

func mergeRecentUnique(existing []string, incoming []string, max int) []string {
	merged := make([]string, 0, len(incoming)+len(existing))
	seen := make(map[string]struct{}, len(incoming)+len(existing))

	addItem := func(item string) {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			return
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		merged = append(merged, trimmed)
	}

	for _, item := range incoming {
		addItem(item)
	}
	for _, item := range existing {
		addItem(item)
	}
	return trimStringList(merged, max)
}

func normalizeConfig(cfg *Config) {
	if cfg == nil {
		return
	}
	if len(cfg.RecentFeedback) == 0 && len(cfg.LegacyRecentReverts) > 0 {
		cfg.RecentFeedback = append([]RecentFeedback(nil), cfg.LegacyRecentReverts...)
	}
	cfg.LegacyRecentReverts = nil
	cfg.AvoidPatterns = trimStringList(uniqueTrimmed(cfg.AvoidPatterns), maxAvoidPatternsPerPNS)
	cfg.PreferredPatterns = trimStringList(uniqueTrimmed(cfg.PreferredPatterns), maxPreferredPerPNS)
	cfg.Signals = trimStringList(uniqueTrimmed(cfg.Signals), maxSignalsPerPNS)
	cfg.Scope.InputIDs = trimStringList(uniqueTrimmed(cfg.Scope.InputIDs), maxInputIDsPerPNS)
	cfg.Summaries = trimSummaries(cfg.Summaries)
	cfg.RecentFeedback = trimRecentFeedback(cfg.RecentFeedback)
}

func cloneConfig(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.Scope.InputIDs = append([]string(nil), cfg.Scope.InputIDs...)
	cloned.Summaries = append([]Summary(nil), cfg.Summaries...)
	cloned.AvoidPatterns = append([]string(nil), cfg.AvoidPatterns...)
	cloned.PreferredPatterns = append([]string(nil), cfg.PreferredPatterns...)
	cloned.Signals = append([]string(nil), cfg.Signals...)
	cloned.RecentFeedback = append([]RecentFeedback(nil), cfg.RecentFeedback...)
	cloned.LegacyRecentReverts = nil
	return &cloned
}
