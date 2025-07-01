package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"

	"github.com/mark3labs/mcp-go/server"
)

// StandardMCPServer wraps the mcp-go server with our custom logic
type StandardMCPServer struct {
	server    *server.MCPServer
	apiMapper *APIMapper
	baseURL   string
	token     string
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
		server:    s,
		apiMapper: NewAPIMapper("http://localhost:8080", ""),
		baseURL:   "http://localhost:8080",
		token:     "",
	}

	logger.Info("Standard MCP server initialized with mcp-go")
	return std
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

// HandleJSONRPCRequest handles a JSON-RPC request using simplified logic
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
	// Get system introduction from API mapper
	systemIntro, err := s.apiMapper.generateSystemIntroduction()
	var instructions string
	if err != nil {
		logger.Error("Failed to generate system introduction", "error", err)
		instructions = "AgentSmith-HUB MCP Server initialized. Use 'smart_assistant' with task='system_intro' for complete system overview."
	} else {
		if len(systemIntro.Content) > 0 {
			instructions = systemIntro.Content[0].Text
		} else {
			instructions = "AgentSmith-HUB MCP Server initialized. System overview generation completed."
		}
	}

	// Return protocol information with system introduction
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
			"name":         "AgentSmith-HUB",
			"version":      "0.1.2",
			"instructions": instructions,
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
	// Load prompts using unified config provider
	prompts, err := LoadMCPPrompts()
	if err != nil {
		return s.createJSONRPCError(id, -32603, "Failed to load prompts", err.Error())
	}

	var promptsList []map[string]interface{}
	for _, prompt := range prompts {
		// Convert arguments to JSON Schema format
		var arguments []map[string]interface{}
		for _, arg := range prompt.Arguments {
			argDef := map[string]interface{}{
				"name":        arg.Name,
				"description": arg.Description,
				"required":    arg.Required,
			}
			arguments = append(arguments, argDef)
		}

		promptsList = append(promptsList, map[string]interface{}{
			"name":        prompt.Name,
			"description": prompt.Description,
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

	// Load prompt using unified config provider
	prompt, err := GetMCPPrompt(promptName)
	if err != nil {
		return s.createJSONRPCError(id, -32602, "Prompt not found", fmt.Sprintf("Prompt '%s' not found: %v", promptName, err))
	}

	// Return prompt content as message
	promptResult := map[string]interface{}{
		"description": prompt.Description,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": map[string]interface{}{
					"type": "text",
					"text": prompt.Template,
				},
			},
		},
	}

	return s.createJSONRPCResponse(id, promptResult)
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

	// Add other resource types as needed...

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

	// Simplified resource reading - extend as needed
	content := "Resource content"
	mimeType := "text/plain"

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
