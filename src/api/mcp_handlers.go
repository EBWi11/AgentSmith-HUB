package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/mcp"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

var mcpServer *mcp.MCPServer

// Session management
type MCPSession struct {
	ID        string
	CreatedAt time.Time
	LastSeen  time.Time
	Token     string
}

var (
	mcpSessions = make(map[string]*MCPSession)
	sessionMux  sync.RWMutex
)

const (
	MCPSessionHeader = "Mcp-Session-Id"
	SessionTimeout   = 30 * time.Minute
)

func init() {
	mcpServer = mcp.NewMCPServer()

	// Start session cleanup goroutine
	go sessionCleanup()
}

// generateSessionID creates a cryptographically secure session ID
func generateSessionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// createSession creates a new MCP session
func createSession(token string) *MCPSession {
	sessionMux.Lock()
	defer sessionMux.Unlock()

	sessionID := generateSessionID()
	session := &MCPSession{
		ID:        sessionID,
		CreatedAt: time.Now(),
		LastSeen:  time.Now(),
		Token:     token,
	}

	mcpSessions[sessionID] = session
	logger.Info("Created new MCP session", "sessionId", sessionID)
	return session
}

// getSession retrieves and updates an existing session
func getSession(sessionID string) *MCPSession {
	sessionMux.Lock()
	defer sessionMux.Unlock()

	session, exists := mcpSessions[sessionID]
	if !exists {
		return nil
	}

	// Check if session has expired
	if time.Since(session.LastSeen) > SessionTimeout {
		delete(mcpSessions, sessionID)
		logger.Info("Removed expired MCP session", "sessionId", sessionID)
		return nil
	}

	// Update last seen
	session.LastSeen = time.Now()
	return session
}

// deleteSession removes a session
func deleteSession(sessionID string) bool {
	sessionMux.Lock()
	defer sessionMux.Unlock()

	_, exists := mcpSessions[sessionID]
	if exists {
		delete(mcpSessions, sessionID)
		logger.Info("Deleted MCP session", "sessionId", sessionID)
		return true
	}
	return false
}

// sessionCleanup periodically removes expired sessions
func sessionCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sessionMux.Lock()
		now := time.Now()
		for sessionID, session := range mcpSessions {
			if now.Sub(session.LastSeen) > SessionTimeout {
				delete(mcpSessions, sessionID)
				logger.Info("Cleaned up expired MCP session", "sessionId", sessionID)
			}
		}
		sessionMux.Unlock()
	}
}

// isInitializeMethod checks if the request contains an initialize method
func isInitializeMethod(body interface{}) bool {
	switch v := body.(type) {
	case map[string]interface{}:
		method, ok := v["method"].(string)
		return ok && method == "initialize"
	case []interface{}:
		for _, item := range v {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if method, ok := itemMap["method"].(string); ok && method == "initialize" {
					return true
				}
			}
		}
	}
	return false
}

// InitializeMCPServer initializes the MCP server with proper configuration
func InitializeMCPServer(baseURL, token string) {
	if mcpServer != nil {
		mcpServer.UpdateConfig(baseURL, token)
	}
}

// checkLeaderMode checks if current node is leader, returns error response if not
func checkLeaderMode(c echo.Context) error {
	if !cluster.IsLeader {
		return c.JSON(http.StatusForbidden, map[string]interface{}{
			"error":           "MCP service is only available on leader nodes",
			"message":         "This node is not the cluster leader. MCP operations are restricted to the leader node.",
			"leader_required": true,
		})
	}
	return nil
}

// MCP HTTP endpoint - handles all MCP JSON-RPC requests
func handleMCP(c echo.Context) error {
	// Check if this node is the leader
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	// Handle DELETE requests for session termination
	if c.Request().Method == "DELETE" {
		return handleMCPSessionDelete(c)
	}

	// Handle GET requests for SSE connections (like Cline)
	if c.Request().Method == "GET" {
		return handleMCPSSE(c)
	}

	// Handle POST requests
	return handleMCPPost(c)
}

// handleMCPSessionDelete handles session termination via DELETE request
func handleMCPSessionDelete(c echo.Context) error {
	sessionID := c.Request().Header.Get(MCPSessionHeader)
	if sessionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Missing Mcp-Session-Id header",
		})
	}

	if deleteSession(sessionID) {
		return c.NoContent(http.StatusNoContent)
	} else {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Session not found",
		})
	}
}

// handleMCPPost handles POST requests with session management
func handleMCPPost(c echo.Context) error {
	// Parse request body
	var body interface{}
	if err := c.Bind(&body); err != nil {
		logger.Error("Failed to parse MCP request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid JSON-RPC request",
		})
	}

	// Get authentication token from 'token' header
	token := c.Request().Header.Get("token")

	// Validate token
	if token == "" {
		logger.Error("MCP request attempted without token")
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
	}

	if token != common.Config.Token {
		logger.Error("MCP request with invalid token")
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication failed",
		})
	}

	// Session management
	sessionID := c.Request().Header.Get(MCPSessionHeader)
	var session *MCPSession

	if sessionID == "" && isInitializeMethod(body) {
		// Create new session for initialize request
		session = createSession(token)
		sessionID = session.ID

		// Set session ID in response header
		c.Response().Header().Set(MCPSessionHeader, sessionID)
	} else if sessionID != "" {
		// Validate existing session
		session = getSession(sessionID)
		if session == nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Session not found or expired",
			})
		}

		// Verify token matches session
		if session.Token != token {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "Token mismatch for session",
			})
		}
	} else {
		// Non-initialize request without session
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Missing Mcp-Session-Id header for non-initialize request",
		})
	}

	// Process request based on type
	switch v := body.(type) {
	case map[string]interface{}:
		// Single JSON-RPC message
		return handleSingleMessage(c, v, session)
	case []interface{}:
		// Batch JSON-RPC messages
		return handleBatchMessages(c, v, session)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}
}

// handleSingleMessage processes a single JSON-RPC message
func handleSingleMessage(c echo.Context, messageData map[string]interface{}, session *MCPSession) error {
	// Convert to MCPMessage
	messageBytes, _ := json.Marshal(messageData)
	var message common.MCPMessage
	if err := json.Unmarshal(messageBytes, &message); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid JSON-RPC message format",
		})
	}

	// Validate JSON-RPC format
	if message.JSONRpc != "2.0" {
		logger.Error("Invalid JSON-RPC version", "version", message.JSONRpc)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid JSON-RPC version, expected 2.0",
		})
	}

	// Update MCP server configuration
	scheme := "http"
	if c.Request().TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, c.Request().Host)
	mcpServer.UpdateConfig(baseURL, session.Token)

	// Handle the MCP message
	response, err := mcpServer.HandleMessage(&message)
	if err != nil {
		logger.Error("MCP server error", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Internal server error",
		})
	}

	// For now, return JSON response (could be enhanced to support streaming)
	return c.JSON(http.StatusOK, response)
}

// handleBatchMessages processes batch JSON-RPC messages
func handleBatchMessages(c echo.Context, messagesData []interface{}, session *MCPSession) error {
	responses := make([]*common.MCPMessage, 0, len(messagesData))

	// Update MCP server configuration
	scheme := "http"
	if c.Request().TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, c.Request().Host)
	mcpServer.UpdateConfig(baseURL, session.Token)

	for _, messageData := range messagesData {
		messageMap, ok := messageData.(map[string]interface{})
		if !ok {
			errorResponse := &common.MCPMessage{
				JSONRpc: "2.0",
				Error: &common.MCPError{
					Code:    -32600,
					Message: "Invalid Request",
					Data:    "Invalid message format in batch",
				},
			}
			responses = append(responses, errorResponse)
			continue
		}

		// Convert to MCPMessage
		messageBytes, _ := json.Marshal(messageMap)
		var message common.MCPMessage
		if err := json.Unmarshal(messageBytes, &message); err != nil {
			errorResponse := &common.MCPMessage{
				JSONRpc: "2.0",
				ID:      messageMap["id"],
				Error: &common.MCPError{
					Code:    -32600,
					Message: "Invalid Request",
					Data:    "Invalid JSON-RPC message format",
				},
			}
			responses = append(responses, errorResponse)
			continue
		}

		// Validate JSON-RPC format
		if message.JSONRpc != "2.0" {
			logger.Error("Invalid JSON-RPC version in batch", "version", message.JSONRpc)
			errorResponse := &common.MCPMessage{
				JSONRpc: "2.0",
				ID:      message.ID,
				Error: &common.MCPError{
					Code:    -32600,
					Message: "Invalid Request",
					Data:    "Invalid JSON-RPC version, expected 2.0",
				},
			}
			responses = append(responses, errorResponse)
			continue
		}

		// Handle the MCP message
		response, err := mcpServer.HandleMessage(&message)
		if err != nil {
			logger.Error("MCP server error in batch", "error", err)
			errorResponse := &common.MCPMessage{
				JSONRpc: "2.0",
				ID:      message.ID,
				Error: &common.MCPError{
					Code:    -32603,
					Message: "Internal error",
					Data:    err.Error(),
				},
			}
			responses = append(responses, errorResponse)
		} else {
			responses = append(responses, response)
		}
	}

	// Return batch responses
	return c.JSON(http.StatusOK, responses)
}

// handleMCPSSE handles Server-Sent Events for MCP connections (like Cline)
func handleMCPSSE(c echo.Context) error {
	// Check if this node is the leader
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	// Get authentication token from 'token' header
	token := c.Request().Header.Get("token")

	// Validate token
	if token == "" {
		logger.Error("MCP SSE connection attempted without token")
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
	}

	if token != common.Config.Token {
		logger.Error("MCP SSE connection with invalid token")
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication failed",
		})
	}

	// Session management for SSE
	sessionID := c.Request().Header.Get(MCPSessionHeader)
	var session *MCPSession

	if sessionID != "" {
		// Validate existing session
		session = getSession(sessionID)
		if session == nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Session not found or expired",
			})
		}

		// Verify token matches session
		if session.Token != token {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "Token mismatch for session",
			})
		}
	} else {
		// Allow SSE without session for backwards compatibility
		// But log a warning
		logger.Warn("MCP SSE connection without session ID, creating temporary session")
		session = createSession(token)
		// Set session ID in response header
		c.Response().Header().Set(MCPSessionHeader, session.ID)
	}

	// Validate Origin header for security (prevent DNS rebinding attacks)
	origin := c.Request().Header.Get("Origin")
	if origin != "" {
		// For now, we'll log but not block. In production, you should validate against allowed origins
		logger.Info("MCP SSE connection from origin", "origin", origin, "sessionId", session.ID)
	}

	// Set proper SSE headers according to MCP specification
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Access-Control-Allow-Headers", "token, Content-Type, Mcp-Session-Id")

	w := c.Response()

	// Send initial connection notification (standard JSON-RPC notification)
	connectionNotification := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notification/initialized",
		"params": map[string]interface{}{
			"sessionId": session.ID,
			"serverInfo": map[string]interface{}{
				"name":    "AgentSmith-HUB",
				"version": "0.1.2",
			},
			"capabilities": map[string]interface{}{
				"resources": map[string]interface{}{},
				"tools":     map[string]interface{}{},
				"prompts":   map[string]interface{}{},
			},
			"transport": "streamable-http",
		},
	}

	notificationJSON, _ := json.Marshal(connectionNotification)
	fmt.Fprintf(w, "data: %s\n\n", notificationJSON)
	w.Flush()

	// Keep connection alive with periodic heartbeats
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	done := c.Request().Context().Done()

	for {
		select {
		case <-done:
			logger.Info("MCP SSE connection closed by client", "sessionId", session.ID)
			return nil
		case <-ticker.C:
			// Send heartbeat as standard JSON-RPC notification
			heartbeat := map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "notification/ping",
				"params": map[string]interface{}{
					"timestamp": time.Now().Unix(),
					"sessionId": session.ID,
				},
			}
			heartbeatJSON, _ := json.Marshal(heartbeat)
			fmt.Fprintf(w, "data: %s\n\n", heartbeatJSON)
			w.Flush()
		}
	}
}

// MCP Server information endpoint (for discovery)
func getMCPInfo(c echo.Context) error {
	// Check if this node is the leader
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	info := map[string]interface{}{
		"protocol": "Model Context Protocol",
		"version":  common.MCPVersion,
		"server": map[string]interface{}{
			"name":    "AgentSmith-HUB",
			"version": "1.0.0",
		},
		"capabilities": map[string]interface{}{
			"resources": map[string]interface{}{
				"subscribe":   true,
				"listChanged": true,
			},
			"tools": map[string]interface{}{
				"listChanged": true,
			},
			"prompts": map[string]interface{}{
				"listChanged": true,
			},
			"logging": map[string]interface{}{},
		},
		"transport": "http",
		"endpoint":  "/mcp",
		"node_info": map[string]interface{}{
			"is_leader": cluster.IsLeader,
			"node_id":   cluster.NodeID,
		},
	}

	return c.JSON(http.StatusOK, info)
}

// MCP Server manifest (for client discovery)
func getMCPManifest(c echo.Context) error {
	// Check if this node is the leader
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	manifest := map[string]interface{}{
		"mcpVersion":  common.MCPVersion,
		"name":        "AgentSmith-HUB MCP Server",
		"version":     "0.1.2",
		"description": "Model Context Protocol server for AgentSmith-HUB Security Data Pipe Platform (SDPP) providing access to security project configurations, components, and security management tools (Leader node only)",
		"author":      "AgentSmith-HUB Team",
		"license":     "MIT",
		"homepage":    "https://github.com/your-org/AgentSmith-HUB",
		"transport": map[string]interface{}{
			"type": "http",
			"host": c.Request().Host,
			"path": "/mcp",
		},
		"capabilities": map[string]interface{}{
			"resources": []string{
				"Projects and their configurations",
				"Input component configurations",
				"Output component configurations",
				"Plugin source code and metadata",
				"Ruleset XML configurations",
			},
			"tools": []string{
				"Full API coverage with 60+ tools for managing all HUB components",
				"create_project - Create new projects",
				"start_project - Start existing projects",
				"stop_project - Stop running projects",
				"get_project_status - Get project status information",
				"search_components - Search through component configurations",
				"validate_component - Validate component configurations",
				"And many more management tools...",
			},
			"prompts": []string{
				"analyze_project - Analyze project configurations",
				"debug_component - Debug component issues",
				"optimize_performance - Get performance optimization suggestions",
			},
		},
		"security": map[string]interface{}{
			"authentication": "token",
			"description":    "Requires authentication token in header",
		},
		"cluster": map[string]interface{}{
			"leader_only": true,
			"description": "MCP service is only available on cluster leader nodes",
		},
	}

	return c.JSON(http.StatusOK, manifest)
}

// Health check for MCP server
func mcpHealthCheck(c echo.Context) error {
	status := "healthy"
	httpStatus := http.StatusOK

	// Include leader status in health check but don't block it
	if !cluster.IsLeader {
		status = "unavailable"
		httpStatus = http.StatusServiceUnavailable
	}

	return c.JSON(httpStatus, map[string]interface{}{
		"status":    status,
		"service":   "AgentSmith-HUB MCP Server",
		"version":   "0.1.2",
		"uptime":    "running",
		"is_leader": cluster.IsLeader,
		"node_id":   cluster.NodeID,
		"message": func() string {
			if cluster.IsLeader {
				return "MCP service available"
			}
			return "MCP service only available on leader nodes"
		}(),
	})
}

// WebSocket handler for MCP (optional - for real-time communication)
func handleMCPWebSocket(c echo.Context) error {
	// Check if this node is the leader
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	// WebSocket implementation would go here if needed for real-time MCP communication
	// For now, we'll return a not implemented response
	return c.JSON(http.StatusNotImplemented, map[string]string{
		"error": "WebSocket transport not yet implemented",
		"note":  "Use HTTP transport at /mcp endpoint",
	})
}

// Batch MCP request handler (for multiple requests in one call)
func handleMCPBatch(c echo.Context) error {
	// Check if this node is the leader
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	var messages []common.MCPMessage

	// Parse batch JSON-RPC request
	if err := c.Bind(&messages); err != nil {
		logger.Error("Failed to parse MCP batch request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid JSON-RPC batch request",
		})
	}

	if len(messages) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Empty batch request",
		})
	}

	// Process each message in the batch
	responses := make([]*common.MCPMessage, 0, len(messages))
	for _, message := range messages {
		// Validate JSON-RPC format
		if message.JSONRpc != "2.0" {
			logger.Error("Invalid JSON-RPC version in batch", "version", message.JSONRpc)
			errorResponse := &common.MCPMessage{
				JSONRpc: "2.0",
				ID:      message.ID,
				Error: &common.MCPError{
					Code:    -32600,
					Message: "Invalid Request",
					Data:    "Invalid JSON-RPC version, expected 2.0",
				},
			}
			responses = append(responses, errorResponse)
			continue
		}

		// Handle the MCP message
		response, err := mcpServer.HandleMessage(&message)
		if err != nil {
			logger.Error("MCP server error in batch", "error", err)
			errorResponse := &common.MCPMessage{
				JSONRpc: "2.0",
				ID:      message.ID,
				Error: &common.MCPError{
					Code:    -32603,
					Message: "Internal error",
					Data:    err.Error(),
				},
			}
			responses = append(responses, errorResponse)
		} else {
			responses = append(responses, response)
		}
	}

	// Return all responses
	return c.JSON(http.StatusOK, responses)
}

// MCP Statistics endpoint
func getMCPStats(c echo.Context) error {
	// Check if this node is the leader
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	// Collect statistics about MCP usage
	stats := map[string]interface{}{
		"server": map[string]interface{}{
			"initialized": mcpServer != nil,
			"version":     common.MCPVersion,
		},
		"cluster": map[string]interface{}{
			"is_leader": cluster.IsLeader,
			"node_id":   cluster.NodeID,
		},
		"resources": map[string]interface{}{
			"total_projects": len(project.GlobalProject.Projects),
			"total_inputs":   len(project.GlobalProject.Inputs),
			"total_outputs":  len(project.GlobalProject.Outputs),
			"total_plugins":  len(plugin.Plugins),
			"total_rulesets": len(project.GlobalProject.Rulesets),
		},
		"capabilities": map[string]interface{}{
			"resources_available": true,
			"tools_available":     true,
			"prompts_available":   true,
		},
		"api_tools": map[string]interface{}{
			"total_tools": len(mcpServer.GetAPIMapper().GetAllAPITools()),
		},
	}

	return c.JSON(http.StatusOK, stats)
}

// getMCPInstallConfig provides MCP client installation configuration
func getMCPInstallConfig(c echo.Context) error {
	if !cluster.IsLeader {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "MCP services are only available on the leader node",
		})
	}

	// Get server host information
	host := c.Request().Host
	if host == "" {
		host = "localhost:8080"
	}

	// Determine protocol (http/https)
	protocol := "http"
	if c.Request().TLS != nil {
		protocol = "https"
	}
	if forwardedProto := c.Request().Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		protocol = forwardedProto
	}

	baseURL := fmt.Sprintf("%s://%s", protocol, host)

	// MCP server configuration for VSCode extension
	mcpConfig := map[string]interface{}{
		"name":        "AgentSmith-HUB",
		"description": "AgentSmith-HUB MCP Server - Security Data Pipe Platform (SDPP) & Security Rules Engine",
		"version":     "0.1.2",
		"server": map[string]interface{}{
			"type":    "http",
			"baseUrl": baseURL,
			"endpoints": map[string]interface{}{
				"mcp":      "/mcp",
				"batch":    "/mcp/batch",
				"info":     "/mcp/info",
				"manifest": "/mcp/manifest",
				"stats":    "/mcp/stats",
				"health":   "/mcp/health",
				"install":  "/mcp/install", // This endpoint
			},
			"authentication": map[string]interface{}{
				"type":   "header",
				"header": "token",
				"note":   "Obtain token from AgentSmith-HUB administrator",
			},
		},
		"capabilities": map[string]interface{}{
			"resources": map[string]interface{}{
				"description": "Access to project configurations, inputs, outputs, rulesets, and plugins",
				"supported":   true,
			},
			"tools": map[string]interface{}{
				"description": "Complete API access to all AgentSmith-HUB functionality",
				"count":       60, // Approximate number of tools available
				"supported":   true,
			},
			"prompts": map[string]interface{}{
				"description": "Intelligent prompts for project analysis, debugging, and optimization",
				"count":       12, // Number of available prompts
				"supported":   true,
			},
		},
		"installation": map[string]interface{}{
			"vscode": map[string]interface{}{
				"extensionId": "your-extension-id", // Replace with actual VSCode extension ID
				"settings": map[string]interface{}{
					"mcp.servers": map[string]interface{}{
						"agentsmith-hub": map[string]interface{}{
							"name":        "AgentSmith-HUB",
							"description": "Security Data Pipe Platform & Rules Engine",
							"transport": map[string]interface{}{
								"type": "http",
								"host": host,
								"port": getPortFromHost(host),
								"path": "/mcp",
								"ssl":  protocol == "https",
							},
							"authentication": map[string]interface{}{
								"type":   "bearer",
								"header": "token",
							},
							"timeout": 30000, // 30 seconds
						},
					},
				},
				"configuration": map[string]interface{}{
					"steps": []string{
						"1. Install the MCP extension for VSCode",
						"2. Add the server configuration to your VSCode settings",
						"3. Obtain authentication token from your AgentSmith-HUB administrator",
						"4. Configure the token in VSCode MCP settings",
						"5. Restart VSCode to apply the changes",
					},
				},
			},
			"claude_desktop": map[string]interface{}{
				"settings": map[string]interface{}{
					"mcpServers": map[string]interface{}{
						"agentsmith-hub": map[string]interface{}{
							"command": "node",
							"args": []string{
								"/path/to/mcp-http-client.js", // HTTP MCP client bridge
								"--server", baseURL + "/mcp",
								"--token", "${AGENTSMITH_TOKEN}", // Environment variable
							},
							"env": map[string]interface{}{
								"AGENTSMITH_TOKEN": "your-token-here",
							},
						},
					},
				},
				"configuration": map[string]interface{}{
					"steps": []string{
						"1. Set environment variable AGENTSMITH_TOKEN with your authentication token",
						"2. Add the server configuration to Claude Desktop settings",
						"3. Restart Claude Desktop to apply the changes",
					},
				},
			},
		},
		"documentation": map[string]interface{}{
			"apiDocs":    baseURL + "/api/docs",
			"quickStart": baseURL + "/docs/quickstart",
			"examples":   baseURL + "/docs/examples",
			"github":     "https://github.com/your-org/agentsmith-hub",
		},
		"support": map[string]interface{}{
			"email":   "support@your-domain.com",
			"website": "https://your-domain.com",
			"docs":    baseURL + "/docs",
		},
	}

	return c.JSON(http.StatusOK, mcpConfig)
}

// getPortFromHost extracts port from host string
func getPortFromHost(host string) int {
	// Default ports
	defaultPort := 8080

	// Try to parse port from host
	if len(host) > 0 {
		for i := len(host) - 1; i >= 0; i-- {
			if host[i] == ':' {
				if portStr := host[i+1:]; len(portStr) > 0 {
					if port, err := strconv.Atoi(portStr); err == nil {
						return port
					}
				}
				break
			}
		}
	}

	return defaultPort
}
