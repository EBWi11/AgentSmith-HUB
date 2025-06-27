package mcp

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"fmt"
	"sort"
	"strings"
)

// promptAnalyzeProject generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptAnalyzeProject(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["analyze_project"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: analyze_project")
	}

	projectID, ok := args["project_id"].(string)
	if !ok || projectID == "" {
		return common.MCPGetPromptResult{}, fmt.Errorf("project_id is required")
	}

	proj := project.GlobalProject.Projects[projectID]
	if proj == nil {
		return common.MCPGetPromptResult{}, fmt.Errorf("project not found: %s", projectID)
	}

	var componentDetails strings.Builder
	componentDetails.WriteString("### Component Dependencies:\n")
	componentDetails.WriteString("**Inputs Used:**\n" + getComponentListForPrompt(proj.Inputs) + "\n")
	componentDetails.WriteString("**Outputs Used:**\n" + getComponentListForPrompt(proj.Outputs) + "\n")
	componentDetails.WriteString("**Rulesets Used:**\n" + getComponentListForPrompt(proj.Rulesets))

	errorInfo := ""
	if proj.Err != nil {
		errorInfo = fmt.Sprintf("\n### Current Error:\n%s\n", proj.Err.Error())
	}

	promptText := fmt.Sprintf(promptDef.Template, proj.Id, proj.Status, componentDetails.String(), proj.Config.RawConfig, errorInfo)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptDebugComponent generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptDebugComponent(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["debug_component"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: debug_component")
	}

	componentType, _ := args["component_type"].(string)
	componentID, _ := args["component_id"].(string)
	issueDescription, _ := args["issue_description"].(string)
	errorLogs, _ := args["error_logs"].(string)

	var componentConfig, componentTypeDescription, codeBlockType, issueStr, errorStr string
	codeBlockType = "yaml"

	switch componentType {
	case "input":
		if c := project.GlobalProject.Inputs[componentID]; c != nil {
			componentConfig = c.Config.RawConfig
		}
	case "output":
		if c := project.GlobalProject.Outputs[componentID]; c != nil {
			componentConfig = c.Config.RawConfig
		}
	case "plugin":
		if p := plugin.Plugins[componentID]; p != nil {
			componentConfig = string(p.Payload)
			codeBlockType = "go"
		}
	case "ruleset":
		if r := project.GlobalProject.Rulesets[componentID]; r != nil {
			componentConfig = r.RawConfig
			codeBlockType = "xml"
		}
	}

	if issueDescription != "" {
		issueStr = fmt.Sprintf("\n## Issue Description:\n%s", issueDescription)
	}
	if errorLogs != "" {
		errorStr = fmt.Sprintf("\n## Error Logs:\n```\n%s\n```", errorLogs)
	}

	promptText := fmt.Sprintf(promptDef.Template, componentID, componentType, componentTypeDescription, codeBlockType, componentConfig, issueStr, errorStr)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptOptimizePerformance generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptOptimizePerformance(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["optimize_performance"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: optimize_performance")
	}

	focusArea, _ := args["focus_area"].(string)
	focusStr := ""
	if focusArea != "" {
		focusStr = fmt.Sprintf("### Focus Area: %s\n\n", focusArea)
	}

	projectCount := len(project.GlobalProject.Projects)
	runningProjects := 0
	errorProjects := 0
	for _, proj := range project.GlobalProject.Projects {
		if proj.Status == project.ProjectStatusRunning {
			runningProjects++
		} else if proj.Status == project.ProjectStatusError {
			errorProjects++
		}
	}
	healthRate := 0.0
	if projectCount > 0 {
		healthRate = (float64(runningProjects) / float64(projectCount)) * 100
	}
	scale := "Small Scale"
	if projectCount > 50 {
		scale = "Large Scale"
	} else if projectCount > 10 {
		scale = "Medium Scale"
	}

	promptText := fmt.Sprintf(promptDef.Template, projectCount, runningProjects, errorProjects, healthRate, len(project.GlobalProject.Inputs), len(project.GlobalProject.Outputs), len(plugin.Plugins), len(project.GlobalProject.Rulesets), scale, focusStr)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptCreateProjectGuide generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptCreateProjectGuide(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["create_project_guide"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: create_project_guide")
	}

	useCase, _ := args["use_case"].(string)
	inputComponents, _ := args["input_components"].(string)
	processingLogic, _ := args["processing_logic"].(string)
	outputComponents, _ := args["output_components"].(string)

	availableInputs := getComponentListForPrompt(project.GlobalProject.Inputs)
	availableRulesets := getComponentListForPrompt(project.GlobalProject.Rulesets)
	availableOutputs := getComponentListForPrompt(project.GlobalProject.Outputs)

	promptText := fmt.Sprintf(promptDef.Template, useCase, inputComponents, processingLogic, outputComponents, availableInputs, availableRulesets, availableOutputs)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptCreateInputGuide generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptCreateInputGuide(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["create_input_guide"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: create_input_guide")
	}
	inputType, _ := args["input_type"].(string)
	useCase, _ := args["use_case"].(string)

	promptText := fmt.Sprintf(promptDef.Template, inputType, useCase)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptCreateOutputGuide generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptCreateOutputGuide(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["create_output_guide"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: create_output_guide")
	}
	outputType, _ := args["output_type"].(string)
	useCase, _ := args["use_case"].(string)

	promptText := fmt.Sprintf(promptDef.Template, outputType, useCase)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptCreateRulesetGuide generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptCreateRulesetGuide(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["create_ruleset_guide"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: create_ruleset_guide")
	}
	useCase, _ := args["use_case"].(string)
	targetData, _ := args["target_data"].(string)
	detectionLogic, _ := args["detection_logic"].(string)

	promptText := fmt.Sprintf(promptDef.Template, useCase, targetData, detectionLogic)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptGuideRulesetPlugins generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptGuideRulesetPlugins(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["guide_ruleset_plugins"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: guide_ruleset_plugins")
	}
	goal, _ := args["goal"].(string)

	var availablePluginsBuilder strings.Builder
	if len(plugin.Plugins) > 0 {
		for name := range plugin.Plugins {
			availablePluginsBuilder.WriteString(fmt.Sprintf("    - %s\n", name))
		}
	} else {
		availablePluginsBuilder.WriteString("    - (No custom plugins loaded)")
	}
	availablePlugins := strings.TrimRight(availablePluginsBuilder.String(), "\n")

	promptText := fmt.Sprintf(promptDef.Template, goal, availablePlugins)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptSecurityAudit generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptSecurityAudit(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["security_audit"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: security_audit")
	}
	scope, _ := args["scope"].(string)
	targetID, _ := args["target_id"].(string)

	var authAnalysis strings.Builder
	authAnalysis.WriteString("### Authentication Analysis:\n")
	hasAuth := false
	for _, input := range project.GlobalProject.Inputs {
		if strings.Contains(strings.ToLower(input.Config.RawConfig), "auth") ||
			strings.Contains(strings.ToLower(input.Config.RawConfig), "token") ||
			strings.Contains(strings.ToLower(input.Config.RawConfig), "credential") {
			hasAuth = true
			break
		}
	}
	if hasAuth {
		authAnalysis.WriteString("- Authentication configurations detected in components\n")
	} else {
		authAnalysis.WriteString("- No explicit authentication configurations found\n")
	}

	promptText := fmt.Sprintf(promptDef.Template, scope, targetID, len(project.GlobalProject.Projects), len(project.GlobalProject.Inputs), len(project.GlobalProject.Outputs), authAnalysis.String())

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptClusterHealthCheck generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptClusterHealthCheck(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["cluster_health_check"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: cluster_health_check")
	}
	checkType, _ := args["check_type"].(string)

	promptText := fmt.Sprintf(promptDef.Template, checkType)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptMigrationGuide generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptMigrationGuide(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["migration_guide"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: migration_guide")
	}
	migrationType, _ := args["migration_type"].(string)
	fromVersion, _ := args["from_version"].(string)
	toVersion, _ := args["to_version"].(string)

	promptText := fmt.Sprintf(promptDef.Template, migrationType, fromVersion, toVersion)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptCapacityPlanning generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptCapacityPlanning(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["capacity_planning"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: capacity_planning")
	}
	timeHorizon, _ := args["time_horizon"].(string)
	expectedGrowth, _ := args["expected_growth"].(string)

	promptText := fmt.Sprintf(promptDef.Template, timeHorizon, expectedGrowth)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptGuideComponentUpdateWorkflow generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptGuideComponentUpdateWorkflow(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["guide_component_update_workflow"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: guide_component_update_workflow")
	}

	// This prompt is static, so no dynamic data is needed.
	promptText := promptDef.Template

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// getComponentListForPrompt formats a map of components into a string list for prompt display.
func getComponentListForPrompt[T any](componentMap map[string]T) string {
	if len(componentMap) == 0 {
		return "    - (No components of this type available)"
	}
	var builder strings.Builder
	// Sort keys for consistent output
	keys := make([]string, 0, len(componentMap))
	for k := range componentMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		builder.WriteString(fmt.Sprintf("    - %s\n", k))
	}
	return strings.TrimRight(builder.String(), "\n")
}
