package agent

import "testing"

func TestProcessMessageNormalizesExistingLLMField(t *testing.T) {
	a := &Agent{
		Id: "agent-test",
		Config: &AgentConfig{
			Model:     "test",
			Timeout:   "1s",
			MaxRounds: 0,
		},
	}

	result := a.processMessage(map[string]interface{}{
		"llm": "not-a-map",
	})

	llm, ok := result["llm"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected llm to be normalized to map, got %T", result["llm"])
	}
	block, ok := llm[a.Id].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agent error block, got %#v", llm[a.Id])
	}
	if block["error"] == "" {
		t.Fatalf("expected error message in agent block, got %#v", block)
	}
}

func TestNewFromExistingClonesConfig(t *testing.T) {
	existing := &Agent{
		Id: "agent-template",
		Config: &AgentConfig{
			Model:        "test-model",
			SystemPrompt: "base prompt",
			Tools:        []string{"tool-a"},
			MemoryNotes:  MemoryNotes{"remember-a"},
			MaxRounds:    2,
			Timeout:      "1s",
		},
		RawConfig: `model: test-model
system_prompt: base prompt
tools:
  - tool-a
memory_notes:
  - remember-a
max_rounds: 2
timeout: 1s
`,
	}

	clone, err := NewFromExisting(existing, "PROJECT.agent-template")
	if err != nil {
		t.Fatalf("NewFromExisting returned error: %v", err)
	}
	if clone.Config == existing.Config {
		t.Fatal("expected cloned agent to have an independent config pointer")
	}

	clone.Config.MemoryNotes[0] = "remember-b"
	if existing.Config.MemoryNotes[0] != "remember-a" {
		t.Fatalf("expected existing memory notes to remain unchanged, got %q", existing.Config.MemoryNotes[0])
	}
}
