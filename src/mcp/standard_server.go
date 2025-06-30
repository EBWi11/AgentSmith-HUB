package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// PromptConfig defines the structure for prompt configurations
type PromptConfig struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Arguments   []common.MCPPromptArg `json:"arguments"`
	Template    string                `json:"template"`
}

// StandardMCPServer wraps the mcp-go server with our custom logic
type StandardMCPServer struct {
	server         *server.MCPServer
	httpServer     *server.StreamableHTTPServer
	apiMapper      *APIMapper
	baseURL        string
	token          string
	promptDefs     map[string]PromptConfig
	promptHandlers map[string]func(map[string]interface{}) (common.MCPGetPromptResult, error)
}

// NewStandardMCPServer creates a new server using mcp-go library
func NewStandardMCPServer() *StandardMCPServer {
	// Create mcp-go server
	s := server.NewMCPServer(
		"AgentSmith-HUB",
		"0.1.2",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
	)

	std := &StandardMCPServer{
		server:         s,
		httpServer:     nil, // We'll use Echo instead
		apiMapper:      NewAPIMapper("http://localhost:8080", ""),
		baseURL:        "http://localhost:8080",
		token:          "",
		promptDefs:     make(map[string]PromptConfig),
		promptHandlers: make(map[string]func(map[string]interface{}) (common.MCPGetPromptResult, error)),
	}

	// Start with basic tools
	std.registerBasicTools()

	// Initialize template system
	std.initializeTemplateSystem()

	// Initialize prompt handlers
	std.initializePromptHandlers()

	logger.Info("Standard MCP server initialized with mcp-go")
	return std
}

// registerBasicTools registers essential tools - removed duplicate registration
func (s *StandardMCPServer) registerBasicTools() {
	// Note: Basic tools will be registered through MigrateAllTools()
	// This prevents duplicate registration and ensures consistency
	logger.Info("Basic tools will be registered through API mapper migration")
}

// initializeTemplateSystem initializes the response template system
func (s *StandardMCPServer) initializeTemplateSystem() {
	// Load response templates from config
	if err := LoadResponseTemplates("mcp_config/response_templates.json"); err != nil {
		logger.Warn("Could not load response templates, trying parent path", "error", err)
		if err := LoadResponseTemplates("../mcp_config/response_templates.json"); err != nil {
			logger.Warn("Could not load response templates from any path", "error", err)
		} else {
			logger.Info("Response templates loaded from parent directory")
		}
	} else {
		logger.Info("Response templates loaded successfully")
	}
}

// initializePromptHandlers initializes basic prompt handlers
func (s *StandardMCPServer) initializePromptHandlers() {
	// Register actual prompt handlers instead of placeholders
	s.promptHandlers["analyze_project"] = s.handleAnalyzeProjectPrompt
	s.promptHandlers["debug_component"] = s.handleDebugComponentPrompt
	s.promptHandlers["plugin_development_guide"] = s.handlePluginDevelopmentGuide
	s.promptHandlers["plugin_usage_guide"] = s.handlePluginUsageGuide
	s.promptHandlers["plugin_troubleshooting"] = s.handlePluginTroubleshooting
	s.promptHandlers["plugin_best_practices"] = s.handlePluginBestPractices

	// Load prompts from config file if available (try current path first, then parent path)
	if err := s.loadPrompts("mcp_config/prompts.json"); err != nil {
		logger.Warn("Could not load prompts from current path, trying parent path", "error", err)
		if err := s.loadPrompts("../mcp_config/prompts.json"); err != nil {
			logger.Warn("Could not load prompts from config file", "error", err)
		}
	}

	logger.Info("Prompt handlers initialized for standard MCP server", "handlers", len(s.promptHandlers))
}

// handleAnalyzeProjectPrompt provides project analysis guidance
func (s *StandardMCPServer) handleAnalyzeProjectPrompt(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	projectID, hasProject := args["project_id"].(string)

	var promptText string
	if hasProject {
		promptText = fmt.Sprintf("Analyze the AgentSmith-HUB project '%s' and provide:\n\n1. **Project Overview**: Components, data flow, and architecture\n2. **Security Assessment**: Current rules and detection capabilities\n3. **Performance Analysis**: Throughput, bottlenecks, and optimization opportunities\n4. **Recommendations**: Improvements and best practices\n\nUse the following MCP tools to gather information:\n- get_project to retrieve project details\n- get_component_usage to check dependencies\n- get_metrics for performance data\n- get_samplers_data for data analysis", projectID)
	} else {
		promptText = "To analyze an AgentSmith-HUB project, please provide:\n\n**project_id**: The ID of the project to analyze\n\nI'll then provide comprehensive analysis including components, security posture, performance metrics, and optimization recommendations."
	}

	return common.MCPGetPromptResult{
		Messages: []common.MCPPromptMessage{
			{
				Role: "user",
				Content: common.MCPPromptContent{
					Type: "text",
					Text: promptText,
				},
			},
		},
	}, nil
}

// handleDebugComponentPrompt provides component debugging guidance
func (s *StandardMCPServer) handleDebugComponentPrompt(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	componentType, hasType := args["component_type"].(string)
	componentID, hasID := args["component_id"].(string)

	var promptText string
	if hasType && hasID {
		promptText = fmt.Sprintf("Debug the %s component '%s' using this systematic approach:\n\n1. **Validate Configuration**: Check syntax and structure\n2. **Test Connectivity**: Verify external connections (if applicable)\n3. **Check Dependencies**: Ensure all referenced components exist\n4. **Review Logs**: Check for errors and warnings\n5. **Performance Check**: Monitor resource usage\n\nRecommended MCP tools:\n- get_%s to retrieve component details\n- validate_component for syntax checking\n- test_component for functionality testing\n- get_error_logs for troubleshooting", componentType, componentID, componentType)
	} else {
		promptText = "To debug an AgentSmith-HUB component, please provide:\n\n**component_type**: Type of component (input/output/plugin/ruleset/project)\n**component_id**: ID of the specific component\n\nI'll then provide targeted debugging guidance and recommended tools."
	}

	return common.MCPGetPromptResult{
		Messages: []common.MCPPromptMessage{
			{
				Role: "user",
				Content: common.MCPPromptContent{
					Type: "text",
					Text: promptText,
				},
			},
		},
	}, nil
}

// handlePluginDevelopmentGuide provides comprehensive plugin development guidance
func (s *StandardMCPServer) handlePluginDevelopmentGuide(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	// Get the template from loaded prompts
	if promptConfig, exists := s.promptDefs["plugin_development_guide"]; exists {
		return common.MCPGetPromptResult{
			Messages: []common.MCPPromptMessage{
				{
					Role: "user",
					Content: common.MCPPromptContent{
						Type: "text",
						Text: promptConfig.Template,
					},
				},
			},
		}, nil
	}

	// Fallback if template not loaded
	return common.MCPGetPromptResult{
		Messages: []common.MCPPromptMessage{
			{
				Role: "user",
				Content: common.MCPPromptContent{
					Type: "text",
					Text: "🔧 AGENTSMITH-HUB PLUGIN DEVELOPMENT GUIDE 🔧\n\nMANDATORY REQUIREMENTS:\n• Package: must be 'plugin'\n• Function: must contain 'Eval' function\n• Returns: (bool, error) for checknode or (interface{}, bool, error) for data processing\n• Imports: Only Go standard library allowed\n\nUse create_plugin MCP tool to create your plugin with proper validation.",
				},
			},
		},
	}, nil
}

// handlePluginUsageGuide provides plugin usage guidance
func (s *StandardMCPServer) handlePluginUsageGuide(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	// Get the template from loaded prompts
	if promptConfig, exists := s.promptDefs["plugin_usage_guide"]; exists {
		return common.MCPGetPromptResult{
			Messages: []common.MCPPromptMessage{
				{
					Role: "user",
					Content: common.MCPPromptContent{
						Type: "text",
						Text: promptConfig.Template,
					},
				},
			},
		}, nil
	}

	// Fallback if template not loaded
	return common.MCPGetPromptResult{
		Messages: []common.MCPPromptMessage{
			{
				Role: "user",
				Content: common.MCPPromptContent{
					Type: "text",
					Text: "🎯 PLUGIN USAGE IN RULESETS GUIDE 🎯\n\nCONTEXTS:\n• CheckNode: <node type=\"PLUGIN\">plugin_name(_$ORIDATA)</node>\n• Append: <append type=\"PLUGIN\">plugin_name(\"param\")</append>\n• Standalone: <plugin>plugin_name(_$ORIDATA)</plugin>\n\nUse add_ruleset_rule MCP tool to integrate plugins into rules.",
				},
			},
		},
	}, nil
}

// handlePluginTroubleshooting provides plugin troubleshooting guidance
func (s *StandardMCPServer) handlePluginTroubleshooting(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	// Get the template from loaded prompts
	if promptConfig, exists := s.promptDefs["plugin_troubleshooting"]; exists {
		return common.MCPGetPromptResult{
			Messages: []common.MCPPromptMessage{
				{
					Role: "user",
					Content: common.MCPPromptContent{
						Type: "text",
						Text: promptConfig.Template,
					},
				},
			},
		}, nil
	}

	// Fallback if template not loaded
	return common.MCPGetPromptResult{
		Messages: []common.MCPPromptMessage{
			{
				Role: "user",
				Content: common.MCPPromptContent{
					Type: "text",
					Text: "🔧 PLUGIN TROUBLESHOOTING GUIDE 🔧\n\nCOMMON ISSUES:\n• Package name error: Change to 'package plugin'\n• Function not found: Must be named 'Eval'\n• Wrong return type: Use (bool, error) or (interface{}, bool, error)\n\nUse test_plugin and get_plugin_parameters MCP tools for debugging.",
				},
			},
		},
	}, nil
}

// handlePluginBestPractices provides plugin best practices guidance
func (s *StandardMCPServer) handlePluginBestPractices(args map[string]interface{}) (common.MCPGetPromptResult, error) {
	// Get the template from loaded prompts
	if promptConfig, exists := s.promptDefs["plugin_best_practices"]; exists {
		return common.MCPGetPromptResult{
			Messages: []common.MCPPromptMessage{
				{
					Role: "user",
					Content: common.MCPPromptContent{
						Type: "text",
						Text: promptConfig.Template,
					},
				},
			},
		}, nil
	}

	// Fallback if template not loaded
	return common.MCPGetPromptResult{
		Messages: []common.MCPPromptMessage{
			{
				Role: "user",
				Content: common.MCPPromptContent{
					Type: "text",
					Text: "⭐ PLUGIN DEVELOPMENT BEST PRACTICES ⭐\n\nDESIGN PRINCIPLES:\n• Single Responsibility: One clear purpose per plugin\n• Defensive Programming: Validate all inputs\n• Performance: Use early returns and avoid expensive operations\n• Error Handling: Provide meaningful error messages\n\nTest thoroughly with test_plugin before deploying.",
				},
			},
		},
	}, nil
}

// loadPrompts loads prompt definitions from file
func (s *StandardMCPServer) loadPrompts(path string) error {
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
			Template:    p.Template,
		}
	}

	logger.Info("Successfully loaded MCP prompts from file", "count", len(s.promptDefs))
	return nil
}

// AddToolFromAPIMapper adds a tool from our existing API mapper
func (s *StandardMCPServer) AddToolFromAPIMapper(toolName string, description string, args map[string]common.MCPToolArg) error {
	// Prepare all tool options including description and parameters
	var toolOptions []mcp.ToolOption
	toolOptions = append(toolOptions, mcp.WithDescription(description))

	// Add parameters
	for argName, argDef := range args {
		switch argDef.Type {
		case "string":
			if argDef.Required {
				toolOptions = append(toolOptions, mcp.WithString(argName,
					mcp.Required(),
					mcp.Description(argDef.Description),
				))
			} else {
				toolOptions = append(toolOptions, mcp.WithString(argName,
					mcp.Description(argDef.Description),
				))
			}
		case "number":
			if argDef.Required {
				toolOptions = append(toolOptions, mcp.WithNumber(argName,
					mcp.Required(),
					mcp.Description(argDef.Description),
				))
			} else {
				toolOptions = append(toolOptions, mcp.WithNumber(argName,
					mcp.Description(argDef.Description),
				))
			}
		case "boolean":
			if argDef.Required {
				toolOptions = append(toolOptions, mcp.WithBoolean(argName,
					mcp.Required(),
					mcp.Description(argDef.Description),
				))
			} else {
				toolOptions = append(toolOptions, mcp.WithBoolean(argName,
					mcp.Description(argDef.Description),
				))
			}
		default:
			// Default to string for unknown types
			if argDef.Required {
				toolOptions = append(toolOptions, mcp.WithString(argName,
					mcp.Required(),
					mcp.Description(argDef.Description),
				))
			} else {
				toolOptions = append(toolOptions, mcp.WithString(argName,
					mcp.Description(argDef.Description),
				))
			}
		}
	}

	// Create tool with all options at once
	tool := mcp.NewTool(toolName, toolOptions...)

	// Create handler that calls our API mapper
	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract arguments
		arguments := make(map[string]interface{})
		if req.Params.Arguments != nil {
			if args, ok := req.Params.Arguments.(map[string]interface{}); ok {
				arguments = args
			}
		}

		// Call API mapper
		result, err := s.apiMapper.CallAPITool(toolName, arguments)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Tool %s failed: %v", toolName, err)), nil
		}

		// Convert result
		if result.IsError {
			errorMsg := fmt.Sprintf("Tool %s failed", toolName)
			if len(result.Content) > 0 {
				errorMsg = result.Content[0].Text
			}
			return mcp.NewToolResultError(errorMsg), nil
		}

		// Return successful result
		if len(result.Content) > 0 {
			return mcp.NewToolResultText(result.Content[0].Text), nil
		}
		return mcp.NewToolResultText("Tool executed successfully"), nil
	}

	s.server.AddTool(tool, handler)
	logger.Info("Added tool from API mapper", "tool", toolName)
	return nil
}

// MigrateAllTools migrates all tools from the existing API mapper
func (s *StandardMCPServer) MigrateAllTools() error {
	tools := s.apiMapper.GetAllAPITools()
	migrated := 0
	failed := 0

	for _, tool := range tools {
		err := s.AddToolFromAPIMapper(tool.Name, tool.Description, tool.InputSchema)
		if err != nil {
			logger.Error("Failed to migrate tool", "tool", tool.Name, "error", err)
			failed++
		} else {
			migrated++
		}
	}

	logger.Info("Tool migration completed", "migrated", migrated, "failed", failed, "total", len(tools))
	return nil
}

// StartMigration starts the migration process from old to new implementation
func (s *StandardMCPServer) StartMigration() error {
	logger.Info("Starting migration of tools to standard MCP server")

	// Migrate all tools in batches
	err := s.MigrateAllTools()
	if err != nil {
		logger.Error("Migration failed", "error", err)
		return err
	}

	logger.Info("Migration completed successfully")
	return nil
}

// UpdateConfig updates server configuration
func (s *StandardMCPServer) UpdateConfig(baseURL, token string) {
	if baseURL != "" {
		s.baseURL = baseURL
		s.apiMapper.baseURL = baseURL
	}
	if token != "" {
		s.token = token
		s.apiMapper.token = token
	}
	logger.Info("Standard MCP server configuration updated", "baseURL", s.baseURL)
}

// GetMCPGoServer returns the underlying mcp-go server
func (s *StandardMCPServer) GetMCPGoServer() *server.MCPServer {
	return s.server
}

// GetAPIMapper returns the API mapper
func (s *StandardMCPServer) GetAPIMapper() *APIMapper {
	return s.apiMapper
}

// HandleJSONRPCRequest handles a JSON-RPC request using mcp-go server
func (s *StandardMCPServer) HandleJSONRPCRequest(requestData []byte) ([]byte, error) {
	// Parse the JSON-RPC request
	var request map[string]interface{}
	if err := json.Unmarshal(requestData, &request); err != nil {
		return nil, fmt.Errorf("invalid JSON-RPC request: %v", err)
	}

	method, ok := request["method"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid method in request")
	}

	id := request["id"]

	// Handle different MCP methods
	switch method {
	case "initialize":
		return s.handleInitialize(id)
	case "tools/list":
		return s.handleToolsList(id)
	case "tools/call":
		return s.handleToolsCall(id, request)
	case "prompts/list":
		return s.handlePromptsList(id)
	case "prompts/get":
		return s.handlePromptsGet(id, request)
	case "resources/list":
		return s.handleResourcesList(id)
	case "resources/read":
		return s.handleResourcesRead(id, request)
	default:
		// For other methods, return a generic success response
		return s.createJSONRPCResponse(id, map[string]interface{}{
			"status":  "success",
			"message": fmt.Sprintf("Method %s handled by mcp-go", method),
		})
	}
}

// handleInitialize handles the initialize method
func (s *StandardMCPServer) handleInitialize(id interface{}) ([]byte, error) {
	// Always return our supported protocol version
	result := map[string]interface{}{
		"protocolVersion": common.MCPVersion,
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": true,
			},
			"resources": map[string]interface{}{
				"listChanged": true,
				"subscribe":   true,
			},
			"prompts": map[string]interface{}{
				"listChanged": true,
			},
		},
		"serverInfo": map[string]interface{}{
			"name":    "AgentSmith-HUB",
			"version": "0.1.2",
		},
	}
	return s.createJSONRPCResponse(id, result)
}

// handleToolsList handles the tools/list method
func (s *StandardMCPServer) handleToolsList(id interface{}) ([]byte, error) {
	tools := s.apiMapper.GetAllAPITools()
	var toolsList []map[string]interface{}

	for _, tool := range tools {
		// Convert tool schema to JSON Schema format
		properties := make(map[string]interface{})
		required := make([]string, 0)

		for argName, argDef := range tool.InputSchema {
			properties[argName] = map[string]interface{}{
				"type":        argDef.Type,
				"description": argDef.Description,
			}
			if argDef.Required {
				required = append(required, argName)
			}
		}

		schema := map[string]interface{}{
			"type":       "object",
			"properties": properties,
		}
		if len(required) > 0 {
			schema["required"] = required
		}

		toolsList = append(toolsList, map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": schema,
		})
	}

	result := map[string]interface{}{
		"tools": toolsList,
	}
	return s.createJSONRPCResponse(id, result)
}

// handleToolsCall handles the tools/call method
func (s *StandardMCPServer) handleToolsCall(id interface{}, request map[string]interface{}) ([]byte, error) {
	params, ok := request["params"].(map[string]interface{})
	if !ok {
		return s.createJSONRPCError(id, -32602, "Invalid params", "Missing or invalid params")
	}

	toolName, ok := params["name"].(string)
	if !ok {
		return s.createJSONRPCError(id, -32602, "Invalid params", "Missing tool name")
	}

	arguments := make(map[string]interface{})
	if args, ok := params["arguments"].(map[string]interface{}); ok {
		arguments = args
	}

	// Call the API tool through our mapper
	result, err := s.apiMapper.CallAPITool(toolName, arguments)
	if err != nil {
		return s.createJSONRPCError(id, -32603, "Internal error", err.Error())
	}

	if result.IsError {
		errorMsg := "Tool execution failed"
		if len(result.Content) > 0 {
			errorMsg = result.Content[0].Text
		}
		return s.createJSONRPCError(id, -32603, "Tool execution failed", errorMsg)
	}

	// Convert tool result to MCP format
	var content []map[string]interface{}
	for _, c := range result.Content {
		content = append(content, map[string]interface{}{
			"type": "text", // MCP spec: content type should be "text" or "image"
			"text": c.Text,
		})
	}

	toolResult := map[string]interface{}{
		"content": content,
		"isError": false,
	}

	return s.createJSONRPCResponse(id, toolResult)
}

// handlePromptsList handles the prompts/list method
func (s *StandardMCPServer) handlePromptsList(id interface{}) ([]byte, error) {
	var promptsList []map[string]interface{}

	for _, promptConfig := range s.promptDefs {
		// Convert arguments to JSON Schema format
		var arguments []map[string]interface{}
		for _, arg := range promptConfig.Arguments {
			argDef := map[string]interface{}{
				"name":        arg.Name,
				"description": arg.Description,
				"required":    arg.Required,
			}
			arguments = append(arguments, argDef)
		}

		promptsList = append(promptsList, map[string]interface{}{
			"name":        promptConfig.Name,
			"description": promptConfig.Description,
			"arguments":   arguments,
		})
	}

	result := map[string]interface{}{
		"prompts": promptsList,
	}
	return s.createJSONRPCResponse(id, result)
}

// handlePromptsGet handles the prompts/get method
func (s *StandardMCPServer) handlePromptsGet(id interface{}, request map[string]interface{}) ([]byte, error) {
	params, ok := request["params"].(map[string]interface{})
	if !ok {
		return s.createJSONRPCError(id, -32602, "Invalid params", "Missing or invalid params")
	}

	promptName, ok := params["name"].(string)
	if !ok {
		return s.createJSONRPCError(id, -32602, "Invalid params", "Missing prompt name")
	}

	promptConfig, exists := s.promptDefs[promptName]
	if !exists {
		return s.createJSONRPCError(id, -32602, "Prompt not found", fmt.Sprintf("Prompt '%s' not found", promptName))
	}

	// Extract arguments from params if provided
	arguments := make(map[string]interface{})
	if args, ok := params["arguments"].(map[string]interface{}); ok {
		arguments = args
	}

	// Call prompt handler
	if handler, exists := s.promptHandlers[promptName]; exists {
		result, err := handler(arguments)
		if err != nil {
			return s.createJSONRPCError(id, -32603, "Prompt execution failed", err.Error())
		}

		// Convert result to MCP format
		var messages []map[string]interface{}
		for _, msg := range result.Messages {
			messages = append(messages, map[string]interface{}{
				"role": msg.Role,
				"content": map[string]interface{}{
					"type": msg.Content.Type,
					"text": msg.Content.Text,
				},
			})
		}

		promptResult := map[string]interface{}{
			"description": promptConfig.Description,
			"messages":    messages,
		}

		return s.createJSONRPCResponse(id, promptResult)
	}

	// Default response if no handler found
	defaultResult := map[string]interface{}{
		"description": promptConfig.Description,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": map[string]interface{}{
					"type": "text",
					"text": fmt.Sprintf("Prompt '%s' is available but handler not fully implemented yet.", promptName),
				},
			},
		},
	}

	return s.createJSONRPCResponse(id, defaultResult)
}

// handleResourcesList handles the resources/list method
func (s *StandardMCPServer) handleResourcesList(id interface{}) ([]byte, error) {
	var resources []map[string]interface{}

	// Get projects from API
	if projectResult, err := s.apiMapper.CallAPITool("get_projects", map[string]interface{}{}); err == nil && !projectResult.IsError {
		if len(projectResult.Content) > 0 {
			var projects []map[string]interface{}
			if err := json.Unmarshal([]byte(projectResult.Content[0].Text), &projects); err == nil {
				for _, project := range projects {
					if projectId, ok := project["id"].(string); ok {
						resources = append(resources, map[string]interface{}{
							"uri":         fmt.Sprintf("hub://project/%s", projectId),
							"name":        fmt.Sprintf("Project: %s", projectId),
							"description": fmt.Sprintf("Project configuration and data flow for %s", projectId),
							"mimeType":    "application/yaml",
							"annotations": map[string]interface{}{
								"type":   "project",
								"status": project["status"],
							},
						})
					}
				}
			}
		}
	}

	// Get inputs from API
	if inputResult, err := s.apiMapper.CallAPITool("get_inputs", map[string]interface{}{}); err == nil && !inputResult.IsError {
		if len(inputResult.Content) > 0 {
			var inputs []map[string]interface{}
			if err := json.Unmarshal([]byte(inputResult.Content[0].Text), &inputs); err == nil {
				for _, input := range inputs {
					if inputId, ok := input["id"].(string); ok {
						resources = append(resources, map[string]interface{}{
							"uri":         fmt.Sprintf("hub://input/%s", inputId),
							"name":        fmt.Sprintf("Input: %s", inputId),
							"description": fmt.Sprintf("Input component configuration for %s", inputId),
							"mimeType":    "application/yaml",
							"annotations": map[string]interface{}{
								"type": "input",
							},
						})
					}
				}
			}
		}
	}

	// Get outputs from API
	if outputResult, err := s.apiMapper.CallAPITool("get_outputs", map[string]interface{}{}); err == nil && !outputResult.IsError {
		if len(outputResult.Content) > 0 {
			var outputs []map[string]interface{}
			if err := json.Unmarshal([]byte(outputResult.Content[0].Text), &outputs); err == nil {
				for _, output := range outputs {
					if outputId, ok := output["id"].(string); ok {
						resources = append(resources, map[string]interface{}{
							"uri":         fmt.Sprintf("hub://output/%s", outputId),
							"name":        fmt.Sprintf("Output: %s", outputId),
							"description": fmt.Sprintf("Output component configuration for %s", outputId),
							"mimeType":    "application/yaml",
							"annotations": map[string]interface{}{
								"type": "output",
							},
						})
					}
				}
			}
		}
	}

	// Get rulesets from API
	if rulesetResult, err := s.apiMapper.CallAPITool("get_rulesets", map[string]interface{}{}); err == nil && !rulesetResult.IsError {
		if len(rulesetResult.Content) > 0 {
			var rulesets []map[string]interface{}
			if err := json.Unmarshal([]byte(rulesetResult.Content[0].Text), &rulesets); err == nil {
				for _, ruleset := range rulesets {
					if rulesetId, ok := ruleset["id"].(string); ok {
						resources = append(resources, map[string]interface{}{
							"uri":         fmt.Sprintf("hub://ruleset/%s", rulesetId),
							"name":        fmt.Sprintf("Ruleset: %s", rulesetId),
							"description": fmt.Sprintf("Security ruleset configuration for %s", rulesetId),
							"mimeType":    "application/xml",
							"annotations": map[string]interface{}{
								"type": "ruleset",
							},
						})
					}
				}
			}
		}
	}

	// Get plugins from API
	if pluginResult, err := s.apiMapper.CallAPITool("get_plugins", map[string]interface{}{}); err == nil && !pluginResult.IsError {
		if len(pluginResult.Content) > 0 {
			var plugins []map[string]interface{}
			if err := json.Unmarshal([]byte(pluginResult.Content[0].Text), &plugins); err == nil {
				for _, plugin := range plugins {
					if pluginId, ok := plugin["id"].(string); ok {
						resources = append(resources, map[string]interface{}{
							"uri":         fmt.Sprintf("hub://plugin/%s", pluginId),
							"name":        fmt.Sprintf("Plugin: %s", pluginId),
							"description": fmt.Sprintf("Security plugin implementation for %s", pluginId),
							"mimeType":    "text/x-go",
							"annotations": map[string]interface{}{
								"type": "plugin",
							},
						})
					}
				}
			}
		}
	}

	// Add system monitoring resources
	resources = append(resources, map[string]interface{}{
		"uri":         "hub://metrics/qps",
		"name":        "Real-time QPS Metrics",
		"description": "Current system QPS (Queries Per Second) data with component-level breakdown and performance statistics",
		"mimeType":    "application/json",
		"annotations": map[string]interface{}{
			"type":     "metrics",
			"category": "performance",
			"realtime": true,
		},
	})

	resources = append(resources, map[string]interface{}{
		"uri":         "hub://metrics/system",
		"name":        "System Performance Metrics",
		"description": "Current system performance metrics including CPU usage, memory consumption, goroutine count, and disk usage",
		"mimeType":    "application/json",
		"annotations": map[string]interface{}{
			"type":     "metrics",
			"category": "system",
			"realtime": true,
		},
	})

	resources = append(resources, map[string]interface{}{
		"uri":         "hub://cluster/status",
		"name":        "Cluster Health Status",
		"description": "Comprehensive cluster status including node health, leader election state, connectivity matrix, and operational readiness",
		"mimeType":    "application/json",
		"annotations": map[string]interface{}{
			"type":     "cluster",
			"category": "infrastructure",
			"realtime": true,
		},
	})

	// Add knowledge base resources
	resources = append(resources, map[string]interface{}{
		"uri":         "hub://docs/ruleset-syntax",
		"name":        "Ruleset Syntax Guide",
		"description": "ESSENTIAL reference containing comprehensive syntax documentation AND performance optimization guidelines for writing MCP rules",
		"mimeType":    "text/markdown",
		"annotations": map[string]interface{}{
			"type":     "documentation",
			"category": "reference",
			"priority": 1.0,
		},
	})

	resources = append(resources, map[string]interface{}{
		"uri":         "hub://templates/rulesets",
		"name":        "Ruleset Templates Library",
		"description": "Collection of well-tested ruleset templates including LLM-optimized patterns, performance-tuned configurations, and best practices",
		"mimeType":    "application/json",
		"annotations": map[string]interface{}{
			"type":     "templates",
			"category": "rulesets",
			"priority": 0.8,
		},
	})

	resources = append(resources, map[string]interface{}{
		"uri":         "hub://logs/errors",
		"name":        "System Error Logs",
		"description": "Recent system error logs and debugging information for troubleshooting and system analysis",
		"mimeType":    "application/json",
		"annotations": map[string]interface{}{
			"type":     "logs",
			"category": "debugging",
			"realtime": true,
		},
	})

	resources = append(resources, map[string]interface{}{
		"uri":         "hub://status/health",
		"name":        "System Health Report",
		"description": "Comprehensive system health assessment including component status, performance metrics, and optimization recommendations",
		"mimeType":    "application/json",
		"annotations": map[string]interface{}{
			"type":      "report",
			"category":  "health",
			"generated": true,
		},
	})

	result := map[string]interface{}{
		"resources": resources,
	}
	return s.createJSONRPCResponse(id, result)
}

// handleResourcesRead handles the resources/read method
func (s *StandardMCPServer) handleResourcesRead(id interface{}, request map[string]interface{}) ([]byte, error) {
	params, ok := request["params"].(map[string]interface{})
	if !ok {
		return s.createJSONRPCError(id, -32602, "Invalid params", "Missing or invalid params")
	}

	uri, ok := params["uri"].(string)
	if !ok {
		return s.createJSONRPCError(id, -32602, "Invalid params", "Missing resource URI")
	}

	// Parse URI: hub://type/id
	if !strings.HasPrefix(uri, "hub://") {
		return s.createJSONRPCError(id, -32602, "Invalid URI", "URI must start with hub://")
	}

	parts := strings.Split(uri[6:], "/") // Remove "hub://" prefix
	if len(parts) != 2 {
		return s.createJSONRPCError(id, -32602, "Invalid URI format", "URI format should be hub://type/id")
	}

	resourceType := parts[0]
	resourceID := parts[1]

	var content string
	var mimeType string

	// Get resource content using API tools
	switch resourceType {
	case "project":
		if result, err := s.apiMapper.CallAPITool("get_project", map[string]interface{}{"id": resourceID}); err == nil && !result.IsError {
			if len(result.Content) > 0 {
				content = result.Content[0].Text
				mimeType = "application/yaml"
			}
		}
	case "input":
		if result, err := s.apiMapper.CallAPITool("get_input", map[string]interface{}{"id": resourceID}); err == nil && !result.IsError {
			if len(result.Content) > 0 {
				content = result.Content[0].Text
				mimeType = "application/yaml"
			}
		}
	case "output":
		if result, err := s.apiMapper.CallAPITool("get_output", map[string]interface{}{"id": resourceID}); err == nil && !result.IsError {
			if len(result.Content) > 0 {
				content = result.Content[0].Text
				mimeType = "application/yaml"
			}
		}
	case "ruleset":
		if result, err := s.apiMapper.CallAPITool("get_ruleset", map[string]interface{}{"id": resourceID}); err == nil && !result.IsError {
			if len(result.Content) > 0 {
				content = result.Content[0].Text
				mimeType = "application/xml"
			}
		}
	case "plugin":
		if result, err := s.apiMapper.CallAPITool("get_plugin", map[string]interface{}{"id": resourceID}); err == nil && !result.IsError {
			if len(result.Content) > 0 {
				content = result.Content[0].Text
				mimeType = "text/x-go"
			}
		}
	case "metrics":
		// Handle metrics resources
		switch resourceID {
		case "qps":
			if result, err := s.apiMapper.CallAPITool("get_qps_data", map[string]interface{}{}); err == nil && !result.IsError {
				if len(result.Content) > 0 {
					content = result.Content[0].Text
					mimeType = "application/json"
				}
			}
		case "system":
			if result, err := s.apiMapper.CallAPITool("get_system_metrics", map[string]interface{}{}); err == nil && !result.IsError {
				if len(result.Content) > 0 {
					content = result.Content[0].Text
					mimeType = "application/json"
				}
			}
		default:
			return s.createJSONRPCError(id, -32602, "Unknown metrics resource", fmt.Sprintf("Metrics resource '%s' not supported", resourceID))
		}
	case "cluster":
		// Handle cluster resources
		switch resourceID {
		case "status":
			if result, err := s.apiMapper.CallAPITool("get_cluster_status", map[string]interface{}{}); err == nil && !result.IsError {
				if len(result.Content) > 0 {
					content = result.Content[0].Text
					mimeType = "application/json"
				}
			}
		default:
			return s.createJSONRPCError(id, -32602, "Unknown cluster resource", fmt.Sprintf("Cluster resource '%s' not supported", resourceID))
		}
	case "docs":
		// Handle documentation resources
		switch resourceID {
		case "ruleset-syntax":
			if result, err := s.apiMapper.CallAPITool("get_ruleset_syntax_guide", map[string]interface{}{}); err == nil && !result.IsError {
				if len(result.Content) > 0 {
					content = result.Content[0].Text
					mimeType = "text/markdown"
				}
			}
		default:
			return s.createJSONRPCError(id, -32602, "Unknown documentation resource", fmt.Sprintf("Documentation resource '%s' not supported", resourceID))
		}
	case "templates":
		// Handle template resources
		switch resourceID {
		case "rulesets":
			if result, err := s.apiMapper.CallAPITool("get_ruleset_templates", map[string]interface{}{}); err == nil && !result.IsError {
				if len(result.Content) > 0 {
					content = result.Content[0].Text
					mimeType = "application/json"
				}
			}
		default:
			return s.createJSONRPCError(id, -32602, "Unknown template resource", fmt.Sprintf("Template resource '%s' not supported", resourceID))
		}
	case "logs":
		// Handle logs resources
		switch resourceID {
		case "errors":
			if result, err := s.apiMapper.CallAPITool("get_error_logs", map[string]interface{}{}); err == nil && !result.IsError {
				if len(result.Content) > 0 {
					content = result.Content[0].Text
					mimeType = "application/json"
				}
			}
		default:
			return s.createJSONRPCError(id, -32602, "Unknown logs resource", fmt.Sprintf("Logs resource '%s' not supported", resourceID))
		}
	case "status":
		// Handle status resources
		switch resourceID {
		case "health":
			if result, err := s.apiMapper.CallAPITool("system_health_check", map[string]interface{}{}); err == nil && !result.IsError {
				if len(result.Content) > 0 {
					content = result.Content[0].Text
					mimeType = "application/json"
				}
			}
		default:
			return s.createJSONRPCError(id, -32602, "Unknown status resource", fmt.Sprintf("Status resource '%s' not supported", resourceID))
		}
	default:
		return s.createJSONRPCError(id, -32602, "Unknown resource type", fmt.Sprintf("Resource type '%s' not supported", resourceType))
	}

	if content == "" {
		return s.createJSONRPCError(id, -32603, "Resource not found", fmt.Sprintf("Could not read resource %s", uri))
	}

	contents := []map[string]interface{}{
		{
			"uri":      uri,
			"mimeType": mimeType,
			"text":     content,
		},
	}

	result := map[string]interface{}{
		"contents": contents,
	}

	return s.createJSONRPCResponse(id, result)
}

// createJSONRPCResponse creates a JSON-RPC response
func (s *StandardMCPServer) createJSONRPCResponse(id interface{}, result interface{}) ([]byte, error) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	return json.Marshal(response)
}

// createJSONRPCError creates a JSON-RPC error response
func (s *StandardMCPServer) createJSONRPCError(id interface{}, code int, message string, data interface{}) ([]byte, error) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"data":    data,
		},
	}
	return json.Marshal(response)
}
