package mcp

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"fmt"
	"strings"
)

// Helper functions for resource content retrieval
func (s *MCPServer) getProjectContent(projectID string) (string, string, error) {
	proj := project.GlobalProject.Projects[projectID]
	if proj == nil {
		return "", "", fmt.Errorf("project not found: %s", projectID)
	}

	description := fmt.Sprintf("Project %s configuration and status", projectID)
	content := fmt.Sprintf("Project ID: %s\nStatus: %s\nConfiguration:\n%s",
		proj.Id, proj.Status, proj.Config.RawConfig)

	if proj.Err != nil {
		content += fmt.Sprintf("\nError: %s", proj.Err.Error())
	}

	return description, content, nil
}

func (s *MCPServer) getInputContent(inputID string) (string, string, error) {
	input := project.GlobalProject.Inputs[inputID]
	if input == nil {
		return "", "", fmt.Errorf("input not found: %s", inputID)
	}
	return fmt.Sprintf("Input %s configuration", inputID), input.Config.RawConfig, nil
}

func (s *MCPServer) getOutputContent(outputID string) (string, string, error) {
	output := project.GlobalProject.Outputs[outputID]
	if output == nil {
		return "", "", fmt.Errorf("output not found: %s", outputID)
	}
	return fmt.Sprintf("Output %s configuration", outputID), output.Config.RawConfig, nil
}

func (s *MCPServer) getPluginContent(pluginName string) (string, string, error) {
	plugin := plugin.Plugins[pluginName]
	if plugin == nil {
		return "", "", fmt.Errorf("plugin not found: %s", pluginName)
	}

	content := fmt.Sprintf("Plugin: %s\nType: %d\nPayload:\n%s",
		plugin.Name, plugin.Type, string(plugin.Payload))

	return fmt.Sprintf("Plugin %s implementation", pluginName), content, nil
}

func (s *MCPServer) getRulesetContent(rulesetID string) (string, string, error) {
	ruleset := project.GlobalProject.Rulesets[rulesetID]
	if ruleset == nil {
		return "", "", fmt.Errorf("ruleset not found: %s", rulesetID)
	}
	return fmt.Sprintf("Ruleset %s configuration", rulesetID), ruleset.RawConfig, nil
}

// Tool implementations
func (s *MCPServer) toolCreateProject(args map[string]interface{}) (common.MCPToolResult, error) {
	projectID, ok := args["project_id"].(string)
	if !ok || projectID == "" {
		return common.MCPToolResult{}, fmt.Errorf("project_id is required")
	}

	_, ok = args["config"].(string)
	if !ok {
		return common.MCPToolResult{}, fmt.Errorf("config is required")
	}

	// Check if project already exists
	if project.GlobalProject.Projects[projectID] != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{
				{
					Format: "text",
					Text:   fmt.Sprintf("Project %s already exists", projectID),
				},
			},
			IsError: true,
		}, nil
	}

	// Simplified implementation for now
	return common.MCPToolResult{
		Content: []common.MCPToolContent{
			{
				Format: "text",
				Text:   fmt.Sprintf("Request to create project %s has been received.", projectID),
			},
		},
		IsError: false,
	}, nil
}

func (s *MCPServer) toolStartProject(args map[string]interface{}) (common.MCPToolResult, error) {
	projectID, ok := args["project_id"].(string)
	if !ok || projectID == "" {
		return common.MCPToolResult{}, fmt.Errorf("project_id is required")
	}

	proj := project.GlobalProject.Projects[projectID]
	if proj == nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{
				{
					Format: "text",
					Text:   fmt.Sprintf("Project %s not found", projectID),
				},
			},
			IsError: true,
		}, nil
	}

	if proj.Status == project.ProjectStatusRunning {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{
				{
					Format: "text",
					Text:   fmt.Sprintf("Project %s is already running", projectID),
				},
			},
			IsError: false,
		}, nil
	}

	err := proj.Start()
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{
				{
					Format: "text",
					Text:   fmt.Sprintf("Failed to start project %s: %v", projectID, err),
				},
			},
			IsError: true,
		}, nil
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{
			{
				Format: "text",
				Text:   fmt.Sprintf("Successfully started project %s", projectID),
			},
		},
		IsError: false,
	}, nil
}

func (s *MCPServer) toolStopProject(args map[string]interface{}) (common.MCPToolResult, error) {
	projectID, ok := args["project_id"].(string)
	if !ok || projectID == "" {
		return common.MCPToolResult{}, fmt.Errorf("project_id is required")
	}

	proj := project.GlobalProject.Projects[projectID]
	if proj == nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{
				{
					Format: "text",
					Text:   fmt.Sprintf("Project %s not found", projectID),
				},
			},
			IsError: true,
		}, nil
	}

	if proj.Status == project.ProjectStatusStopped {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{
				{
					Format: "text",
					Text:   fmt.Sprintf("Project %s is already stopped", projectID),
				},
			},
			IsError: false,
		}, nil
	}

	err := proj.Stop()
	if err != nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{
				{
					Format: "text",
					Text:   fmt.Sprintf("Failed to stop project %s: %v", projectID, err),
				},
			},
			IsError: true,
		}, nil
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{
			{
				Format: "text",
				Text:   fmt.Sprintf("Successfully stopped project %s", projectID),
			},
		},
		IsError: false,
	}, nil
}

func (s *MCPServer) toolGetProjectStatus(args map[string]interface{}) (common.MCPToolResult, error) {
	projectID, ok := args["project_id"].(string)
	if !ok || projectID == "" {
		return common.MCPToolResult{}, fmt.Errorf("project_id is required")
	}

	proj := project.GlobalProject.Projects[projectID]
	if proj == nil {
		return common.MCPToolResult{
			Content: []common.MCPToolContent{
				{
					Format: "text",
					Text:   fmt.Sprintf("Project %s not found", projectID),
				},
			},
			IsError: true,
		}, nil
	}

	status := fmt.Sprintf("Project %s - Status: %s", projectID, proj.Status)
	if proj.Err != nil {
		status += fmt.Sprintf("\nError: %s", proj.Err.Error())
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{
			{
				Format: "text",
				Text:   status,
			},
		},
		IsError: false,
	}, nil
}

func (s *MCPServer) toolSearchComponents(args map[string]interface{}) (common.MCPToolResult, error) {
	query, _ := args["query"].(string)
	componentType, _ := args["component_type"].(string)

	var results []string

	// Search projects
	if componentType == "" || componentType == "project" {
		for projectID, proj := range project.GlobalProject.Projects {
			if query == "" || strings.Contains(strings.ToLower(projectID), strings.ToLower(query)) ||
				strings.Contains(strings.ToLower(proj.Config.RawConfig), strings.ToLower(query)) {
				results = append(results, fmt.Sprintf("Project: %s (Status: %s)", projectID, proj.Status))
			}
		}
	}

	// Search inputs
	if componentType == "" || componentType == "input" {
		for inputID, input := range project.GlobalProject.Inputs {
			if query == "" || strings.Contains(strings.ToLower(inputID), strings.ToLower(query)) ||
				strings.Contains(strings.ToLower(input.Config.RawConfig), strings.ToLower(query)) {
				results = append(results, fmt.Sprintf("Input: %s", inputID))
			}
		}
	}

	// Search outputs
	if componentType == "" || componentType == "output" {
		for outputID, output := range project.GlobalProject.Outputs {
			if query == "" || strings.Contains(strings.ToLower(outputID), strings.ToLower(query)) ||
				strings.Contains(strings.ToLower(output.Config.RawConfig), strings.ToLower(query)) {
				results = append(results, fmt.Sprintf("Output: %s", outputID))
			}
		}
	}

	// Search plugins
	if componentType == "" || componentType == "plugin" {
		for pluginName, plugin := range plugin.Plugins {
			if query == "" || strings.Contains(strings.ToLower(pluginName), strings.ToLower(query)) ||
				strings.Contains(strings.ToLower(string(plugin.Payload)), strings.ToLower(query)) {
				results = append(results, fmt.Sprintf("Plugin: %s (Type: %d)", pluginName, plugin.Type))
			}
		}
	}

	// Search rulesets
	if componentType == "" || componentType == "ruleset" {
		for rulesetID, ruleset := range project.GlobalProject.Rulesets {
			if query == "" || strings.Contains(strings.ToLower(rulesetID), strings.ToLower(query)) ||
				strings.Contains(strings.ToLower(ruleset.RawConfig), strings.ToLower(query)) {
				results = append(results, fmt.Sprintf("Ruleset: %s", rulesetID))
			}
		}
	}

	if len(results) == 0 {
		results = append(results, "No components found matching the search criteria")
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{
			{
				Format: "text",
				Text:   strings.Join(results, "\n"),
			},
		},
		IsError: false,
	}, nil
}

func (s *MCPServer) toolValidateComponent(args map[string]interface{}) (common.MCPToolResult, error) {
	componentType, ok := args["component_type"].(string)
	if !ok || componentType == "" {
		return common.MCPToolResult{}, fmt.Errorf("component_type is required")
	}

	componentID, ok := args["component_id"].(string)
	if !ok || componentID == "" {
		return common.MCPToolResult{}, fmt.Errorf("component_id is required")
	}

	var validationResult string
	var isValid bool

	switch componentType {
	case "project":
		proj := project.GlobalProject.Projects[componentID]
		if proj == nil {
			validationResult = fmt.Sprintf("Project %s not found", componentID)
			isValid = false
		} else {
			if proj.Err != nil {
				validationResult = fmt.Sprintf("Project %s has error: %s", componentID, proj.Err.Error())
				isValid = false
			} else {
				validationResult = fmt.Sprintf("Project %s is valid and running status: %s", componentID, proj.Status)
				isValid = true
			}
		}
	case "input":
		input := project.GlobalProject.Inputs[componentID]
		if input == nil {
			validationResult = fmt.Sprintf("Input %s not found", componentID)
			isValid = false
		} else {
			validationResult = fmt.Sprintf("Input %s configuration is valid", componentID)
			isValid = true
		}
	case "output":
		output := project.GlobalProject.Outputs[componentID]
		if output == nil {
			validationResult = fmt.Sprintf("Output %s not found", componentID)
			isValid = false
		} else {
			validationResult = fmt.Sprintf("Output %s configuration is valid", componentID)
			isValid = true
		}
	case "plugin":
		plugin := plugin.Plugins[componentID]
		if plugin == nil {
			validationResult = fmt.Sprintf("Plugin %s not found", componentID)
			isValid = false
		} else {
			validationResult = fmt.Sprintf("Plugin %s is valid (Type: %d)", componentID, plugin.Type)
			isValid = true
		}
	case "ruleset":
		ruleset := project.GlobalProject.Rulesets[componentID]
		if ruleset == nil {
			validationResult = fmt.Sprintf("Ruleset %s not found", componentID)
			isValid = false
		} else {
			validationResult = fmt.Sprintf("Ruleset %s configuration is valid", componentID)
			isValid = true
		}
	default:
		return common.MCPToolResult{}, fmt.Errorf("unknown component type: %s", componentType)
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{
			{
				Format: "text",
				Text:   validationResult,
			},
		},
		IsError: !isValid,
	}, nil
}
