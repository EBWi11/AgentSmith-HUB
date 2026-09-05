package project

import (
	"strings"
	"testing"
)

func TestParseContentRejectsInvalidGraphs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name: "cycle",
			content: "RULESET.a -> AGENT.b\n" +
				"AGENT.b -> RULESET.a",
			wantErr: "cycle detected",
		},
		{
			name: "duplicate edge",
			content: "INPUT.a -> RULESET.b\n" +
				"INPUT.a -> RULESET.b",
			wantErr: "duplicate data flow",
		},
		{
			name:    "input as destination",
			content: "RULESET.a -> INPUT.b",
			wantErr: "cannot be a destination",
		},
		{
			name:    "output as source",
			content: "OUTPUT.a -> RULESET.b",
			wantErr: "cannot be a source",
		},
		{
			name:    "malformed node",
			content: "INPUT -> OUTPUT.b",
			wantErr: "invalid node format",
		},
		{
			name:    "invalid arrow",
			content: "INPUT.a => OUTPUT.b",
			wantErr: "invalid arrow format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Project{Config: &ProjectConfig{Content: tt.content}}
			err := p.parseContent()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
