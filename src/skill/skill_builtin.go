package skill

// BuiltinFactory creates a SkillImplementation from optional config overrides.
type BuiltinFactory func(config map[string]interface{}) SkillImplementation

// builtinRegistry holds all built-in skill implementations.
var builtinRegistry = map[string]BuiltinFactory{
	"hub_ruleset_editor": newRulesetEditorSkill,
}

// GetBuiltinNames returns available built-in skill reference names.
func GetBuiltinNames() []string {
	names := make([]string, 0, len(builtinRegistry))
	for k := range builtinRegistry {
		names = append(names, k)
	}
	return names
}
