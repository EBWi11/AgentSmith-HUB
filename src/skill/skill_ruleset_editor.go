package skill

import (
	"AgentSmith-HUB/rules_engine"
	"encoding/json"
	"fmt"
)

// RulesetAccessor provides access to rulesets without importing the project package.
// Registered at init time by the project package to avoid circular imports.
type RulesetAccessor struct {
	GetAllRulesets func() map[string]string
	GetRuleset     func(id string) (string, bool)
	SetRulesetNew  func(id string, content string)
}

var rulesetAccessor *RulesetAccessor

// RegisterRulesetAccessor is called by the project package to inject accessor functions.
func RegisterRulesetAccessor(accessor *RulesetAccessor) {
	rulesetAccessor = accessor
}

type rulesetEditorSkill struct {
	readOnly bool
}

func newRulesetEditorSkill(config map[string]interface{}) SkillImplementation {
	s := &rulesetEditorSkill{}
	if config != nil {
		if ro, ok := config["read_only"].(bool); ok {
			s.readOnly = ro
		}
	}
	return s
}

func (s *rulesetEditorSkill) Name() string {
	return "hub_ruleset_editor"
}

func (s *rulesetEditorSkill) Description() string {
	return "View and edit AgentSmith-HUB rulesets. Changes are staged as pending changes for human review."
}

func (s *rulesetEditorSkill) Functions() []ToolDefinition {
	fns := []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "list_rulesets",
				Description: "List all available rulesets with their IDs",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "read_ruleset",
				Description: "Read the XML content of a specific ruleset",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "The ruleset ID to read",
						},
					},
					"required": []string{"id"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "verify_ruleset",
				Description: "Verify that ruleset XML content is syntactically valid",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The XML content to verify",
						},
					},
					"required": []string{"content"},
				},
			},
		},
	}

	if !s.readOnly {
		fns = append(fns, ToolDefinition{
			Type: "function",
			Function: FunctionDef{
				Name:        "write_ruleset",
				Description: "Write new ruleset XML content. This creates a pending change that requires human approval.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "The ruleset ID to update",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The new XML content for the ruleset",
						},
					},
					"required": []string{"id", "content"},
				},
			},
		})
	}

	return fns
}

func (s *rulesetEditorSkill) Execute(functionName string, args map[string]interface{}) (string, error) {
	if rulesetAccessor == nil {
		return "", fmt.Errorf("ruleset accessor not registered")
	}

	switch functionName {
	case "list_rulesets":
		return s.listRulesets()
	case "read_ruleset":
		id, _ := args["id"].(string)
		if id == "" {
			return "", fmt.Errorf("id is required")
		}
		return s.readRuleset(id)
	case "write_ruleset":
		if s.readOnly {
			return "", fmt.Errorf("this skill is configured as read-only")
		}
		id, _ := args["id"].(string)
		content, _ := args["content"].(string)
		if id == "" || content == "" {
			return "", fmt.Errorf("id and content are required")
		}
		return s.writeRuleset(id, content)
	case "verify_ruleset":
		content, _ := args["content"].(string)
		if content == "" {
			return "", fmt.Errorf("content is required")
		}
		return s.verifyRuleset(content)
	default:
		return "", fmt.Errorf("unknown function: %s", functionName)
	}
}

func (s *rulesetEditorSkill) listRulesets() (string, error) {
	all := rulesetAccessor.GetAllRulesets()
	ids := make([]string, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}
	data, _ := json.Marshal(map[string]interface{}{
		"count":    len(ids),
		"rulesets": ids,
	})
	return string(data), nil
}

func (s *rulesetEditorSkill) readRuleset(id string) (string, error) {
	raw, exists := rulesetAccessor.GetRuleset(id)
	if !exists {
		return "", fmt.Errorf("ruleset not found: %s", id)
	}
	return raw, nil
}

func (s *rulesetEditorSkill) writeRuleset(id string, content string) (string, error) {
	if err := rules_engine.Verify("", content); err != nil {
		return "", fmt.Errorf("invalid ruleset XML: %w", err)
	}

	rulesetAccessor.SetRulesetNew(id, content)

	resp, _ := json.Marshal(map[string]string{
		"status": "pending_change_created",
		"id":     id,
		"note":   "Change staged for human review",
	})
	return string(resp), nil
}

func (s *rulesetEditorSkill) verifyRuleset(content string) (string, error) {
	result, err := rules_engine.ValidateWithDetails("", content)
	if err != nil {
		errResp, _ := json.Marshal(map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		})
		return string(errResp), nil
	}

	data, _ := json.Marshal(result)
	return string(data), nil
}
