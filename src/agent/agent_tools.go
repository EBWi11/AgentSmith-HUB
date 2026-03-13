package agent

import (
	"AgentSmith-HUB/local_plugin"
	"AgentSmith-HUB/plugin"
	"encoding/json"
	"fmt"
)

func buildPluginToolDefinitions(toolsConfig interface{}) []ToolDefinition {
	var defs []ToolDefinition

	useAll := false
	var allowList map[string]bool

	switch v := toolsConfig.(type) {
	case string:
		if v == "all" {
			useAll = true
		}
	case []interface{}:
		allowList = make(map[string]bool, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				allowList[s] = true
			}
		}
	case []string:
		allowList = make(map[string]bool, len(v))
		for _, s := range v {
			allowList[s] = true
		}
	case nil:
		return defs
	}

	plugin.PluginsMu.RLock()
	defer plugin.PluginsMu.RUnlock()

	for name, p := range plugin.Plugins {
		if !useAll && (allowList == nil || !allowList[name]) {
			continue
		}
		if p.Err != nil {
			continue
		}

		params := p.Parameters
		if agentParams := plugin.AgentToolParameters[name]; len(agentParams) > 0 {
			params = agentParams
		}
		desc := fmt.Sprintf("Plugin: %s", name)
		if d := local_plugin.LocalPluginDesc[name]; d != "" {
			desc = d
		}
		defs = append(defs, ToolDefinition{
			Type: "function",
			Function: FunctionDef{
				Name:        "tool_" + name,
				Description: desc,
				Parameters:  buildParametersSchema(params),
			},
		})
	}

	return defs
}

func buildParametersSchema(params []plugin.PluginParameter) map[string]interface{} {
	if len(params) == 0 {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	properties := make(map[string]interface{})
	var required []string

	for _, p := range params {
		propType := "string"
		switch p.Type {
		case "int", "int64", "float64":
			propType = "number"
		case "bool":
			propType = "boolean"
		}
		properties[p.Name] = map[string]interface{}{
			"type":        propType,
			"description": p.Name,
		}
		if p.Required {
			required = append(required, p.Name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func executePlugin(name string, args map[string]interface{}, extraArgs ...interface{}) (string, error) {
	plugin.PluginsMu.RLock()
	p, exists := plugin.Plugins[name]
	plugin.PluginsMu.RUnlock()

	if !exists {
		return "", fmt.Errorf("plugin not found: %s", name)
	}

	paramList := p.Parameters
	if agentParams := plugin.AgentToolParameters[name]; len(agentParams) > 0 {
		paramList = agentParams
	}

	// Build function args from the map in parameter order
	var funcArgs []interface{}
	for _, param := range paramList {
		if val, ok := args[param.Name]; ok {
			funcArgs = append(funcArgs, val)
		} else {
			funcArgs = append(funcArgs, nil)
		}
	}
	funcArgs = append(funcArgs, extraArgs...)

	// Try interface+bool return first, fall back to bool-only
	if p.ReturnType == "interface{}" {
		result, ok, err := p.FuncEvalOther(funcArgs...)
		if err != nil {
			return "", err
		}
		if !ok {
			return `{"result": null, "match": false}`, nil
		}
		data, _ := json.Marshal(map[string]interface{}{"result": result, "match": ok})
		return string(data), nil
	}

	boolResult, err := p.FuncEvalCheckNode(funcArgs...)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"match": %t}`, boolResult), nil
}

func parseJSONArgs(raw string) map[string]interface{} {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return make(map[string]interface{})
	}
	return args
}
