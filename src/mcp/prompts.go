package mcp

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
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

	analysisDepth, _ := args["analysis_depth"].(string)
	focusAreas, _ := args["focus_areas"].(string)

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

	promptText := fmt.Sprintf(promptDef.Template, projectID, analysisDepth, focusAreas, proj.Id, proj.Status, componentDetails.String(), proj.Config.RawConfig, errorInfo)

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
	debugLevel, _ := args["debug_level"].(string)

	promptText := fmt.Sprintf(promptDef.Template, componentType, componentID, issueDescription, debugLevel)

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

	getComponentList := func(componentMap interface{}) string {
		var keys []string
		switch m := componentMap.(type) {
		case map[string]*input.Input:
			for k := range m {
				keys = append(keys, k)
			}
		case map[string]*output.Output:
			for k := range m {
				keys = append(keys, k)
			}
		case map[string]*rules_engine.Ruleset:
			for k := range m {
				keys = append(keys, k)
			}
		default:
			return "    - (Unknown component type)"
		}
		if len(keys) == 0 {
			return "    - (No components of this type available)"
		}
		sort.Strings(keys)
		var builder strings.Builder
		for _, k := range keys {
			builder.WriteString(fmt.Sprintf("    - %s\n", k))
		}
		return strings.TrimRight(builder.String(), "\n")
	}

	availableInputs := getComponentList(project.GlobalProject.Inputs)
	availableRulesets := getComponentList(project.GlobalProject.Rulesets)
	availableOutputs := getComponentList(project.GlobalProject.Outputs)

	promptText := fmt.Sprintf(promptDef.Template, useCase, availableInputs, availableRulesets, availableOutputs)
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

// promptCreatePluginGuide generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptCreatePluginGuide(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["create_plugin_guide"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: create_plugin_guide")
	}
	pluginID, _ := args["plugin_id"].(string)
	pluginPurpose, _ := args["plugin_purpose"].(string)
	returnType, _ := args["return_type"].(string)

	promptText := fmt.Sprintf(promptDef.Template, pluginID, pluginPurpose, returnType)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptComponentWorkflowGuide generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptComponentWorkflowGuide(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["component_workflow_guide"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: component_workflow_guide")
	}
	componentType, _ := args["component_type"].(string)
	workflowStage, _ := args["workflow_stage"].(string)
	componentID, _ := args["component_id"].(string)

	promptText := fmt.Sprintf(promptDef.Template, componentType, workflowStage, componentID)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptTestComponentsGuide generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptTestComponentsGuide(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["test_components_guide"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: test_components_guide")
	}
	componentType, _ := args["component_type"].(string)
	componentID, _ := args["component_id"].(string)
	testType, _ := args["test_type"].(string)
	testDataType, _ := args["test_data_type"].(string)

	promptText := fmt.Sprintf(promptDef.Template, componentType, componentID, testType, testDataType)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptTroubleshootConnectivity generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptTroubleshootConnectivity(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["troubleshoot_connectivity"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: troubleshoot_connectivity")
	}
	componentType, _ := args["component_type"].(string)
	componentID, _ := args["component_id"].(string)
	errorSymptoms, _ := args["error_symptoms"].(string)
	networkEnvironment, _ := args["network_environment"].(string)

	promptText := fmt.Sprintf(promptDef.Template, componentType, componentID, errorSymptoms, networkEnvironment)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptAnalyzePerformanceIssues generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptAnalyzePerformanceIssues(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["analyze_performance_issues"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: analyze_performance_issues")
	}
	issueType, _ := args["issue_type"].(string)
	componentScope, _ := args["component_scope"].(string)
	analysisDepth, _ := args["analysis_depth"].(string)
	timeRange, _ := args["time_range"].(string)

	promptText := fmt.Sprintf(promptDef.Template, issueType, componentScope, analysisDepth, timeRange)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptManageBulkOperations generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptManageBulkOperations(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["manage_bulk_operations"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: manage_bulk_operations")
	}
	operationType, _ := args["operation_type"].(string)
	targetScope, _ := args["target_scope"].(string)
	batchSize, _ := args["batch_size"].(string)
	safetyLevel, _ := args["safety_level"].(string)

	promptText := fmt.Sprintf(promptDef.Template, operationType, targetScope, batchSize, safetyLevel)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptAssessChangeImpact generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptAssessChangeImpact(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["assess_change_impact"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: assess_change_impact")
	}
	changeType, _ := args["change_type"].(string)
	componentType, _ := args["component_type"].(string)
	componentID, _ := args["component_id"].(string)
	assessmentDepth, _ := args["assessment_depth"].(string)

	promptText := fmt.Sprintf(promptDef.Template, changeType, componentType, componentID, assessmentDepth)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptDebugErrorLogs generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptDebugErrorLogs(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["debug_error_logs"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: debug_error_logs")
	}
	logScope, _ := args["log_scope"].(string)
	componentFilter, _ := args["component_filter"].(string)
	severityLevel, _ := args["severity_level"].(string)
	timeRange, _ := args["time_range"].(string)

	promptText := fmt.Sprintf(promptDef.Template, logScope, componentFilter, severityLevel, timeRange)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptManageClusterOperations generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptManageClusterOperations(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["manage_cluster_operations"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: manage_cluster_operations")
	}
	operationType, _ := args["operation_type"].(string)
	targetNodes, _ := args["target_nodes"].(string)
	maintenanceWindow, _ := args["maintenance_window"].(string)

	promptText := fmt.Sprintf(promptDef.Template, operationType, targetNodes, maintenanceWindow)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptSearchSystemGuide generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptSearchSystemGuide(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["search_system_guide"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: search_system_guide")
	}
	searchType, _ := args["search_type"].(string)
	searchTarget, _ := args["search_target"].(string)
	searchScope, _ := args["search_scope"].(string)

	promptText := fmt.Sprintf(promptDef.Template, searchType, searchTarget, searchScope)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptManageDataSampling generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptManageDataSampling(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["manage_data_sampling"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: manage_data_sampling")
	}
	samplingPurpose, _ := args["sampling_purpose"].(string)
	dataSource, _ := args["data_source"].(string)
	analysisDepth, _ := args["analysis_depth"].(string)

	promptText := fmt.Sprintf(promptDef.Template, samplingPurpose, dataSource, analysisDepth)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptManageUpgradeOperations generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptManageUpgradeOperations(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["manage_upgrade_operations"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: manage_upgrade_operations")
	}
	upgradeType, _ := args["upgrade_type"].(string)
	componentType, _ := args["component_type"].(string)
	safetyLevel, _ := args["safety_level"].(string)

	promptText := fmt.Sprintf(promptDef.Template, upgradeType, componentType, safetyLevel)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}

// promptManageLocalFiles generates the final prompt text by injecting dynamic data into a template.
func (s *MCPServer) promptManageLocalFiles(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	promptDef, ok := s.promptDefs["manage_local_files"]
	if !ok {
		return common.MCPGetPromptResult{}, fmt.Errorf("prompt definition not found: manage_local_files")
	}
	operationType, _ := args["operation_type"].(string)
	fileScope, _ := args["file_scope"].(string)
	syncStrategy, _ := args["sync_strategy"].(string)

	promptText := fmt.Sprintf(promptDef.Template, operationType, fileScope, syncStrategy)

	return common.MCPGetPromptResult{
		Description: promptDef.Description,
		Messages:    []common.MCPPromptMessage{{Role: "user", Content: common.MCPPromptContent{Type: "text", Text: promptText}}},
	}, nil
}
