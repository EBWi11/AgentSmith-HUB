package get_config

import (
	"AgentSmith-HUB/common"
	"encoding/json"
	"fmt"
	"strings"
)

// Eval reads component configuration directly from in-process memory (no HTTP).
//
// Usage:
//   getConfig(componentType, id)   → returns the raw config string for that component
//   getConfig(componentType)       → returns a JSON map of {id: config} for all components of that type
//
// Supported componentType values: "ruleset", "input", "output", "project"
//
// Returns (result, success, error).
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) < 1 {
		return nil, false, fmt.Errorf("getConfig requires at least 1 argument: componentType [, id]")
	}

	componentType, ok := args[0].(string)
	if !ok || strings.TrimSpace(componentType) == "" {
		return nil, false, fmt.Errorf("componentType must be a non-empty string")
	}
	componentType = strings.TrimSpace(componentType)

	if !isSupportedType(componentType) {
		return nil, false, fmt.Errorf("unsupported componentType %q; supported: ruleset, input, output, project", componentType)
	}

	// Two-argument form: getConfig(type, id) → single config
	if len(args) >= 2 {
		id, ok := args[1].(string)
		if !ok || strings.TrimSpace(id) == "" {
			return nil, false, fmt.Errorf("id must be a non-empty string")
		}
		id = strings.TrimSpace(id)
		config, exists := common.GetRawConfig(componentType, id)
		if !exists {
			return nil, false, fmt.Errorf("%s %q not found", componentType, id)
		}
		return config, true, nil
	}

	// One-argument form: getConfig(type) → all configs as JSON map
	result := make(map[string]string)
	common.ForEachRawConfig(componentType, func(id, config string) bool {
		result[id] = config
		return true
	})
	if len(result) == 0 {
		data, _ := json.Marshal(map[string]interface{}{"count": 0, "items": map[string]string{}})
		return string(data), true, nil
	}
	data, _ := json.Marshal(map[string]interface{}{
		"count": len(result),
		"items": result,
	})
	return string(data), true, nil
}

func isSupportedType(t string) bool {
	switch t {
	case "ruleset", "input", "output", "project":
		return true
	}
	return false
}
