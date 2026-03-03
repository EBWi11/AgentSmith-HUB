package agent

import "AgentSmith-HUB/skill"

// SkillAdapter wraps a skill.SkillImplementation so it can be used
// inside the agent's tool/function-call machinery.
// It bridges the skill package's ToolDefinition type to the agent's ToolDefinition type.
type SkillAdapter struct {
	impl skill.SkillImplementation
}

func newSkillAdapter(impl skill.SkillImplementation) *SkillAdapter {
	return &SkillAdapter{impl: impl}
}

func (a *SkillAdapter) Name() string        { return a.impl.Name() }
func (a *SkillAdapter) Description() string  { return a.impl.Description() }

func (a *SkillAdapter) Functions() []ToolDefinition {
	src := a.impl.Functions()
	out := make([]ToolDefinition, len(src))
	for i, f := range src {
		out[i] = ToolDefinition{
			Type: f.Type,
			Function: FunctionDef{
				Name:        f.Function.Name,
				Description: f.Function.Description,
				Parameters:  f.Function.Parameters,
			},
		}
	}
	return out
}

func (a *SkillAdapter) Execute(functionName string, arguments map[string]interface{}) (string, error) {
	return a.impl.Execute(functionName, arguments)
}
