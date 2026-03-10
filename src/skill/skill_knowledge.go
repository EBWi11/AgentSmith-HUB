package skill

import "fmt"

// knowledgeSkill is a config-driven skill that provides reference content
// to the LLM via a get_reference function. No Go code needed — the entire
// skill is defined by the YAML content field.
type knowledgeSkill struct {
	name        string
	description string
	content     string
}

func newKnowledgeSkill(id, description, content string) *knowledgeSkill {
	return &knowledgeSkill{
		name:        id,
		description: description,
		content:     content,
	}
}

func (s *knowledgeSkill) Name() string        { return s.name }
func (s *knowledgeSkill) Description() string  { return s.description }

func (s *knowledgeSkill) Functions() []ToolDefinition {
	return []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "get_reference",
				Description: fmt.Sprintf("Retrieve the full reference content for: %s", s.description),
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	}
}

func (s *knowledgeSkill) Execute(functionName string, args map[string]interface{}) (string, error) {
	switch functionName {
	case "get_reference":
		return s.content, nil
	default:
		return "", fmt.Errorf("unknown function: %s", functionName)
	}
}
