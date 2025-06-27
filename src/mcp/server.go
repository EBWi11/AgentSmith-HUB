package mcp

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type PromptConfig struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Arguments   []common.MCPPromptArg `json:"arguments"`
	Template    string                `json:"template"`
}

type PromptFile struct {
	Prompts []PromptConfig `json:"prompts"`
}

type MCPServer struct {
	initialized    bool
	capabilities   common.MCPServerCapabilities
	serverInfo     common.MCPImplementationInfo
	apiMapper      *APIMapper
	promptDefs     map[string]PromptConfig
	promptHandlers map[string]func(map[string]interface{}) (common.MCPGetPromptResult, error)
}

func NewMCPServer() *MCPServer {
	apiMapper := NewAPIMapper("http://localhost:8080", "")

	s := &MCPServer{
		initialized: false,
		capabilities: common.MCPServerCapabilities{
			Resources: &common.MCPResourceCapability{},
			Tools:     &common.MCPToolsCapability{},
			Prompts:   &common.MCPPromptsCapability{},
		},
		serverInfo: common.MCPImplementationInfo{
			Name:    "AgentSmith-HUB",
			Version: "0.1.2",
		},
		apiMapper:      apiMapper,
		promptDefs:     make(map[string]PromptConfig),
		promptHandlers: make(map[string]func(map[string]interface{}) (common.MCPGetPromptResult, error)),
	}

	s.registerPromptHandlers()

	if err := s.loadPrompts("mcp_config/prompts.json"); err != nil {
		logger.Error("Failed to load MCP prompts, prompts will be unavailable.", "error", err)
	}

	return s
}

func (s *MCPServer) registerPromptHandlers() {
	s.promptHandlers["analyze_project"] = s.promptAnalyzeProject
	s.promptHandlers["debug_component"] = s.promptDebugComponent
	s.promptHandlers["optimize_performance"] = s.promptOptimizePerformance
	s.promptHandlers["create_project_guide"] = s.promptCreateProjectGuide
	s.promptHandlers["create_ruleset_guide"] = s.promptCreateRulesetGuide
	s.promptHandlers["create_input_guide"] = s.promptCreateInputGuide
	s.promptHandlers["create_output_guide"] = s.promptCreateOutputGuide
	s.promptHandlers["create_plugin_guide"] = s.promptCreatePluginGuide
	s.promptHandlers["component_workflow_guide"] = s.promptComponentWorkflowGuide
	s.promptHandlers["test_components_guide"] = s.promptTestComponentsGuide
	s.promptHandlers["troubleshoot_connectivity"] = s.promptTroubleshootConnectivity
	s.promptHandlers["analyze_performance_issues"] = s.promptAnalyzePerformanceIssues
	s.promptHandlers["manage_bulk_operations"] = s.promptManageBulkOperations
	s.promptHandlers["assess_change_impact"] = s.promptAssessChangeImpact
	s.promptHandlers["debug_error_logs"] = s.promptDebugErrorLogs
	s.promptHandlers["manage_cluster_operations"] = s.promptManageClusterOperations
	s.promptHandlers["search_system_guide"] = s.promptSearchSystemGuide
	s.promptHandlers["manage_data_sampling"] = s.promptManageDataSampling
	s.promptHandlers["manage_upgrade_operations"] = s.promptManageUpgradeOperations
	s.promptHandlers["manage_local_files"] = s.promptManageLocalFiles
	s.promptHandlers["guide_ruleset_plugins"] = s.promptGuideRulesetPlugins
	s.promptHandlers["security_audit"] = s.promptSecurityAudit
	s.promptHandlers["cluster_health_check"] = s.promptClusterHealthCheck
	s.promptHandlers["migration_guide"] = s.promptMigrationGuide
	s.promptHandlers["capacity_planning"] = s.promptCapacityPlanning
	s.promptHandlers["guide_component_update_workflow"] = s.promptGuideComponentUpdateWorkflow
}

func (s *MCPServer) loadPrompts(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("could not read prompt file: %w", err)
	}

	var promptFile struct {
		Prompts []common.MCPPrompt `json:"prompts"`
	}
	if err := json.Unmarshal(data, &promptFile); err != nil {
		return fmt.Errorf("could not parse prompt file: %w", err)
	}

	for _, p := range promptFile.Prompts {
		s.promptDefs[p.Name] = PromptConfig{
			Name:        p.Name,
			Description: p.Description,
			Arguments:   p.Arguments,
		}
	}

	logger.Info("Successfully loaded MCP prompts from file", "count", len(s.promptDefs))
	return nil
}

// UpdateConfig updates the MCP server configuration
func (s *MCPServer) UpdateConfig(baseURL, token string) {
	if baseURL != "" {
		s.apiMapper.baseURL = baseURL
	}
	if token != "" {
		s.apiMapper.token = token
	}
	logger.Info("MCP server configuration updated", "baseURL", s.apiMapper.baseURL)
}

// GetAPIMapper returns the API mapper instance
func (s *MCPServer) GetAPIMapper() *APIMapper {
	return s.apiMapper
}

// Handle MCP message
func (s *MCPServer) HandleMessage(message *common.MCPMessage) (*common.MCPMessage, error) {
	logger.Info("MCP message received", "method", message.Method, "id", message.ID)

	switch message.Method {
	case common.MCPInitialize:
		return s.handleInitialize(message)
	case common.MCPListResources:
		return s.handleListResources(message)
	case common.MCPReadResource:
		return s.handleReadResource(message)
	case common.MCPListTools:
		return s.handleListTools(message)
	case common.MCPCallTool:
		return s.handleCallTool(message)
	case common.MCPListPrompts:
		return s.handleListPrompts(message)
	case common.MCPGetPrompt:
		return s.handleGetPrompt(message)
	default:
		return s.createErrorResponse(message.ID, -32601, "Method not found", nil)
	}
}

// Initialize MCP Server
func (s *MCPServer) handleInitialize(message *common.MCPMessage) (*common.MCPMessage, error) {
	var params common.MCPInitializeParams
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return s.createErrorResponse(message.ID, -32602, "Invalid params", err.Error())
	}

	// Check protocol version compatibility
	if params.ProtocolVersion != common.MCPVersion {
		logger.Warn("MCP protocol version mismatch", "client", params.ProtocolVersion, "server", common.MCPVersion)
	}

	s.initialized = true

	result := common.MCPInitializeResult{
		ProtocolVersion: common.MCPVersion,
		Capabilities:    s.capabilities,
		ServerInfo:      s.serverInfo,
	}

	resultBytes, _ := json.Marshal(result)
	return &common.MCPMessage{
		JSONRpc: "2.0",
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// List all available resources
func (s *MCPServer) handleListResources(message *common.MCPMessage) (*common.MCPMessage, error) {
	if !s.initialized {
		return s.createErrorResponse(message.ID, -32002, "Server not initialized", nil)
	}

	resources := []common.MCPResource{}

	// Helper function to get file info
	getFileInfo := func(path string) (*time.Time, int64) {
		if path == "" {
			return nil, 0
		}
		fi, err := os.Stat(path)
		if err != nil {
			return nil, 0
		}
		mt := fi.ModTime()
		return &mt, fi.Size()
	}

	// Add project resources
	for _, proj := range project.GlobalProject.Projects {
		modTime, size := getFileInfo(proj.Config.Path)
		resources = append(resources, common.MCPResource{
			URI:          fmt.Sprintf("hub://project/%s", proj.Id),
			Name:         fmt.Sprintf("Project: %s", proj.Id),
			Description:  fmt.Sprintf("Project configuration and status for %s", proj.Id),
			MimeType:     "application/yaml",
			LastModified: modTime,
			Size:         size,
			Annotations: map[string]string{
				"type":   "project",
				"status": string(proj.Status),
			},
		})
	}

	// Add input resources
	for _, input := range project.GlobalProject.Inputs {
		modTime, size := getFileInfo(input.Path)
		resources = append(resources, common.MCPResource{
			URI:          fmt.Sprintf("hub://input/%s", input.Id),
			Name:         fmt.Sprintf("Input: %s", input.Id),
			Description:  fmt.Sprintf("Input configuration for %s", input.Id),
			MimeType:     "application/yaml",
			LastModified: modTime,
			Size:         size,
			Annotations:  map[string]string{"type": "input"},
		})
	}

	// Add output resources
	for _, output := range project.GlobalProject.Outputs {
		modTime, size := getFileInfo(output.Path)
		resources = append(resources, common.MCPResource{
			URI:          fmt.Sprintf("hub://output/%s", output.Id),
			Name:         fmt.Sprintf("Output: %s", output.Id),
			Description:  fmt.Sprintf("Output configuration for %s", output.Id),
			MimeType:     "application/yaml",
			LastModified: modTime,
			Size:         size,
			Annotations:  map[string]string{"type": "output"},
		})
	}

	// Add plugin resources (Plugins might not have a direct file path in the same way)
	// For now, we omit LastModified and Size for plugins if path is not available.
	for _, p := range plugin.Plugins {
		resources = append(resources, common.MCPResource{
			URI:         fmt.Sprintf("hub://plugin/%s", p.Name),
			Name:        fmt.Sprintf("Plugin: %s", p.Name),
			Description: fmt.Sprintf("Plugin implementation for %s", p.Name),
			MimeType:    "text/plain",
			Annotations: map[string]string{
				"type":        "plugin",
				"plugin_type": fmt.Sprintf("%d", p.Type),
			},
		})
	}

	// Add ruleset resources
	for _, ruleset := range project.GlobalProject.Rulesets {
		modTime, size := getFileInfo(ruleset.Path)
		resources = append(resources, common.MCPResource{
			URI:          fmt.Sprintf("hub://ruleset/%s", ruleset.RulesetID),
			Name:         fmt.Sprintf("Ruleset: %s", ruleset.RulesetID),
			Description:  fmt.Sprintf("Ruleset configuration for %s", ruleset.RulesetID),
			MimeType:     "application/xml",
			LastModified: modTime,
			Size:         size,
			Annotations:  map[string]string{"type": "ruleset"},
		})
	}

	result := common.MCPListResourcesResult{
		Resources: resources,
	}

	resultBytes, _ := json.Marshal(result)
	return &common.MCPMessage{
		JSONRpc: "2.0",
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// Read specific resource content
func (s *MCPServer) handleReadResource(message *common.MCPMessage) (*common.MCPMessage, error) {
	if !s.initialized {
		return s.createErrorResponse(message.ID, -32002, "Server not initialized", nil)
	}

	var params common.MCPReadResourceParams
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return s.createErrorResponse(message.ID, -32602, "Invalid params", err.Error())
	}

	// Parse URI: hub://type/id
	parts := strings.Split(params.URI, "/")
	if len(parts) != 4 || parts[0] != "hub:" || parts[1] != "" {
		return s.createErrorResponse(message.ID, -32602, "Invalid URI format", nil)
	}

	resourceType := parts[2]
	resourceID := parts[3]

	var content string
	var mimeType string
	var err error

	switch resourceType {
	case "project":
		content, mimeType, err = s.getProjectContent(resourceID)
	case "input":
		content, mimeType, err = s.getInputContent(resourceID)
	case "output":
		content, mimeType, err = s.getOutputContent(resourceID)
	case "plugin":
		content, mimeType, err = s.getPluginContent(resourceID)
	case "ruleset":
		content, mimeType, err = s.getRulesetContent(resourceID)
	default:
		return s.createErrorResponse(message.ID, -32602, "Unknown resource type", nil)
	}

	if err != nil {
		return s.createErrorResponse(message.ID, -32603, "Failed to read resource", err.Error())
	}

	contents := []common.MCPResourceContents{
		{
			URI:      params.URI,
			MimeType: mimeType,
			Text:     content,
		},
	}

	result := common.MCPReadResourceResult{
		Contents: contents,
	}

	resultBytes, _ := json.Marshal(result)
	return &common.MCPMessage{
		JSONRpc: "2.0",
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// List available tools
func (s *MCPServer) handleListTools(message *common.MCPMessage) (*common.MCPMessage, error) {
	if !s.initialized {
		return s.createErrorResponse(message.ID, -32002, "Server not initialized", nil)
	}

	// Get all API-mapped tools
	tools := s.apiMapper.GetAllAPITools()

	result := common.MCPListToolsResult{
		Tools: tools,
	}

	resultBytes, _ := json.Marshal(result)
	return &common.MCPMessage{
		JSONRpc: "2.0",
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// Call a tool
func (s *MCPServer) handleCallTool(message *common.MCPMessage) (*common.MCPMessage, error) {
	if !s.initialized {
		return s.createErrorResponse(message.ID, -32002, "Server not initialized", nil)
	}

	var params common.MCPCallToolParams
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return s.createErrorResponse(message.ID, -32602, "Invalid params", err.Error())
	}

	var result common.MCPToolResult
	var err error

	// Update API mapper with current configuration if available
	if common.Config != nil {
		if common.Config.Token != "" {
			s.apiMapper.token = common.Config.Token
		}
		// Try to determine the correct base URL from the current server configuration
		// This could be improved to use actual server configuration
		if s.apiMapper.baseURL == "http://localhost:8080" {
			// Use a more flexible approach for base URL
			s.apiMapper.baseURL = "http://localhost:8080" // Default fallback
		}
	}

	// Call the API-mapped tool
	result, err = s.apiMapper.CallAPITool(params.Name, params.Arguments)

	if err != nil {
		result = common.MCPToolResult{
			Content: []common.MCPToolContent{
				{
					Format: "text",
					Text:   fmt.Sprintf("Error: %v", err),
				},
			},
			IsError: true,
		}
	}

	resultBytes, _ := json.Marshal(result)
	return &common.MCPMessage{
		JSONRpc: "2.0",
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// List available prompts
func (s *MCPServer) handleListPrompts(message *common.MCPMessage) (*common.MCPMessage, error) {
	if !s.initialized {
		return s.createErrorResponse(message.ID, -32002, "Server not initialized", nil)
	}

	var prompts []common.MCPPrompt
	for _, pDef := range s.promptDefs {
		prompts = append(prompts, common.MCPPrompt{
			Name:        pDef.Name,
			Description: pDef.Description,
			Arguments:   pDef.Arguments,
		})
	}

	result := common.MCPListPromptsResult{
		Prompts: prompts,
	}

	resultBytes, _ := json.Marshal(result)
	return &common.MCPMessage{
		JSONRpc: "2.0",
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// Get a specific prompt
func (s *MCPServer) handleGetPrompt(message *common.MCPMessage) (*common.MCPMessage, error) {
	if !s.initialized {
		return s.createErrorResponse(message.ID, -32002, "Server not initialized", nil)
	}

	var params common.MCPGetPromptParams
	if err := json.Unmarshal(message.Params, &params); err != nil {
		return s.createErrorResponse(message.ID, -32602, "Invalid params", err.Error())
	}

	handler, ok := s.promptHandlers[params.Name]
	if !ok {
		return s.createErrorResponse(message.ID, -32601, "Prompt not found", nil)
	}

	result, err := handler(params.Arguments)
	if err != nil {
		return s.createErrorResponse(message.ID, -32603, "Failed to generate prompt", err.Error())
	}

	resultBytes, _ := json.Marshal(result)
	return &common.MCPMessage{
		JSONRpc: "2.0",
		ID:      message.ID,
		Result:  resultBytes,
	}, nil
}

// Helper function to create error response
func (s *MCPServer) createErrorResponse(id interface{}, code int, message string, data interface{}) (*common.MCPMessage, error) {
	return &common.MCPMessage{
		JSONRpc: "2.0",
		ID:      id,
		Error: &common.MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}, nil
}
