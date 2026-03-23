package agent

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// MemoryNotes is durable guidance for an agent, stored as a YAML sequence (recommended)
// or legacy multi-line string. Each non-empty line becomes one list element.
type MemoryNotes []string

// IsEmpty reports whether there is no stored guidance.
func (m MemoryNotes) IsEmpty() bool {
	return len(m) == 0
}

// String joins lines for LLM context and prompts (plain text).
func (m MemoryNotes) String() string {
	if len(m) == 0 {
		return ""
	}
	return strings.Join(m, "\n")
}

// MemoryNotesFromSummaryText splits model or API output into trimmed non-empty lines.
func MemoryNotesFromSummaryText(s string) MemoryNotes {
	return splitMemoryNotesFromString(s)
}

func splitMemoryNotesFromString(s string) MemoryNotes {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return MemoryNotes(out)
}

func normalizeMemoryLines(lines []string) MemoryNotes {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return MemoryNotes(out)
}

// UnmarshalYAML accepts legacy scalar (multi-line string) or YAML sequence ([]string).
func (m *MemoryNotes) UnmarshalYAML(value *yaml.Node) error {
	if m == nil {
		return fmt.Errorf("memory_notes: nil receiver")
	}
	if value == nil {
		*m = nil
		return nil
	}
	switch value.Kind {
	case yaml.DocumentNode:
		if len(value.Content) != 1 {
			return fmt.Errorf("memory_notes: invalid document")
		}
		return m.UnmarshalYAML(value.Content[0])
	case yaml.AliasNode:
		if value.Alias != nil {
			return m.UnmarshalYAML(value.Alias)
		}
		return fmt.Errorf("memory_notes: broken alias")
	case yaml.ScalarNode:
		var s string
		if err := value.Decode(&s); err != nil {
			return err
		}
		*m = splitMemoryNotesFromString(s)
		return nil
	case yaml.SequenceNode:
		var raw []string
		if err := value.Decode(&raw); err != nil {
			return err
		}
		*m = normalizeMemoryLines(raw)
		return nil
	default:
		return fmt.Errorf("memory_notes: expected string or sequence, got YAML kind %v", value.Kind)
	}
}

// MarshalYAML writes memory_notes as a YAML sequence for readability.
func (m MemoryNotes) MarshalYAML() (interface{}, error) {
	if len(m) == 0 {
		return nil, nil
	}
	return []string(m), nil
}
