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
	memoryDirName          = "memory"
	memoryPNSDirName       = "pns"
	maxRecentRevertsPerPNS = 20
	maxSummariesPerPNS     = 20
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
	return &cfg, nil
}

func ParseConfig(raw string) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
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
	cfg.AvoidPatterns = uniqueTrimmed(cfg.AvoidPatterns)
	cfg.PreferredPatterns = uniqueTrimmed(cfg.PreferredPatterns)
	cfg.Signals = uniqueTrimmed(cfg.Signals)
	cfg.Summaries = trimSummaries(cfg.Summaries)
	cfg.RecentReverts = trimRecentReverts(cfg.RecentReverts)

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

func AppendRevertMemory(scope Scope, result ExtractorResult, revert RecentRevert, sourceOperationID string) (*Config, error) {
	existing, err := LoadPNSMemory(scope.ProjectNodeSequence)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		existing = &Config{
			Scope:   scope,
			Version: 1,
		}
	}

	if existing.Scope.AgentID == "" {
		existing.Scope.AgentID = scope.AgentID
	}
	if existing.Scope.ProjectID == "" {
		existing.Scope.ProjectID = scope.ProjectID
	}
	if existing.Scope.ProjectNodeSequence == "" {
		existing.Scope.ProjectNodeSequence = scope.ProjectNodeSequence
	}
	if len(existing.Scope.InputIDs) == 0 {
		existing.Scope.InputIDs = uniqueTrimmed(scope.InputIDs)
	}
	if len(result.InputIDs) > 0 {
		existing.Scope.InputIDs = uniqueTrimmed(append(existing.Scope.InputIDs, result.InputIDs...))
	}

	if strings.TrimSpace(result.Summary) != "" {
		existing.Summaries = append(existing.Summaries, Summary{
			Category:          strings.TrimSpace(result.Category),
			Summary:           strings.TrimSpace(result.Summary),
			Confidence:        result.Confidence,
			SourceOperationID: sourceOperationID,
			UpdatedAt:         time.Now(),
		})
	}

	existing.AvoidPatterns = uniqueTrimmed(append(existing.AvoidPatterns, result.AvoidPatterns...))
	existing.PreferredPatterns = uniqueTrimmed(append(existing.PreferredPatterns, result.PreferredPatterns...))
	existing.Signals = uniqueTrimmed(append(existing.Signals, result.Signals...))

	if revert.CreatedAt.IsZero() {
		revert.CreatedAt = time.Now()
	}
	existing.RecentReverts = append([]RecentRevert{revert}, existing.RecentReverts...)

	if err := SavePNSMemory(existing); err != nil {
		return nil, err
	}
	return existing, nil
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

func trimRecentReverts(items []RecentRevert) []RecentRevert {
	if len(items) == 0 {
		return nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > maxRecentRevertsPerPNS {
		items = items[:maxRecentRevertsPerPNS]
	}
	return items
}
