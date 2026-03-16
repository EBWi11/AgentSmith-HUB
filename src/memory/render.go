package memory

import (
	"fmt"
	"strings"
)

func RenderPromptBlock(cfg *Config) string {
	if cfg == nil {
		return ""
	}

	lines := []string{
		"## Memory Guidance",
		"Use these project-scoped lessons when deciding whether and how to add rules.",
	}

	if len(cfg.Scope.InputIDs) > 0 {
		lines = append(lines, fmt.Sprintf("Relevant inputs: %s", strings.Join(cfg.Scope.InputIDs, ", ")))
	}

	if len(cfg.Summaries) > 0 {
		lines = append(lines, "Recent lessons:")
		for _, item := range cfg.Summaries {
			if item.Summary == "" {
				continue
			}
			if item.Category != "" {
				lines = append(lines, fmt.Sprintf("- [%s] %s", item.Category, item.Summary))
			} else {
				lines = append(lines, "- "+item.Summary)
			}
		}
	}

	if len(cfg.AvoidPatterns) > 0 {
		lines = append(lines, "Avoid patterns:")
		for _, item := range cfg.AvoidPatterns {
			lines = append(lines, "- "+item)
		}
	}

	if len(cfg.PreferredPatterns) > 0 {
		lines = append(lines, "Preferred patterns:")
		for _, item := range cfg.PreferredPatterns {
			lines = append(lines, "- "+item)
		}
	}

	if len(cfg.Signals) > 0 {
		lines = append(lines, "Observed signals:")
		for _, item := range cfg.Signals {
			lines = append(lines, "- "+item)
		}
	}

	return strings.Join(lines, "\n")
}
