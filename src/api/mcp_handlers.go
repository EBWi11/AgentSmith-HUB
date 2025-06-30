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
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

var mcpServer *mcp.StandardMCPServer
var mcpInitialized bool

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
	SessionTimeout   = 48 * time.Hour
)

func init() {
	// Create MCP server but don't initialize yet (wait for proper config)
	mcpServer = mcp.NewStandardMCPServer()
	mcpInitialized = false

	// Start session cleanup goroutine
	go sessionCleanup()

	logger.Info("MCP server created, waiting for initialization")
}

// InitializeMCPServer initializes the MCP server with proper configuration
func InitializeMCPServer(baseURL, token string) {
	if mcpServer != nil && !mcpInitialized {
		mcpServer.UpdateConfig(baseURL, token)

		// Start migration of all tools
		if err := mcpServer.StartMigration(); err != nil {
			logger.Error("Failed to migrate MCP tools", "error", err)
		} else {
			mcpInitialized = true
			logger.Info("MCP server fully initialized and migrated")
		}
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

// GetMCPImplementationStatus returns current implementation status
func GetMCPImplementationStatus() map[string]interface{} {
	if mcpServer == nil {
		return map[string]interface{}{
			"initialized":      false,
			"using_standard":   false,
			"migration_status": "not_started",
		}
	}

	tools := mcpServer.GetAPIMapper().GetAllAPITools()
	return map[string]interface{}{
		"initialized":    mcpInitialized,
		"using_standard": true,
		"library":        "mcp-go",
		"tool_count":     len(tools),
		"server_info":    "AgentSmith-HUB v0.1.2",
		"migration_status": map[string]interface{}{
			"completed": mcpInitialized,
			"tools":     len(tools),
		},
	}
}

// --- Session Management ---

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

// --- Main MCP Handler ---

// MCP HTTP endpoint - handles all MCP JSON-RPC requests
func handleMCP(c echo.Context) error {
	// Check if this node is the leader
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	// Ensure MCP server is initialized
	if !mcpInitialized {
		// Try to initialize with current request context
		scheme := "http"
		if c.Request().TLS != nil {
			scheme = "https"
		}
		baseURL := fmt.Sprintf("%s://%s", scheme, c.Request().Host)

		// Use default token from config if available
		token := c.Request().Header.Get("token")
		if token != "" {
			InitializeMCPServer(baseURL, token)
		}

		// If still not initialized, return error
		if !mcpInitialized {
			return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
				"error": "MCP server not properly initialized",
				"hint":  "Server is starting up, please try again in a moment",
			})
		}
	}

	// Handle different HTTP methods
	switch c.Request().Method {
	case "DELETE":
		return handleMCPSessionDelete(c)
	case "GET":
		return handleMCPSSE(c)
	case "POST":
		return handleMCPPost(c)
	default:
		return c.JSON(http.StatusMethodNotAllowed, map[string]interface{}{
			"error": "Method not allowed",
		})
	}
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

	// Update MCP server configuration
	scheme := "http"
	if c.Request().TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, c.Request().Host)
	mcpServer.UpdateConfig(baseURL, session.Token)

	// Process request based on type
	switch v := body.(type) {
	case map[string]interface{}:
		// Single JSON-RPC message
		return handleSingleMCPMessage(c, v)
	case []interface{}:
		// Batch JSON-RPC messages
		return handleBatchMCPMessages(c, v)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}
}

// handleSingleMCPMessage processes a single JSON-RPC message using StandardMCPServer
func handleSingleMCPMessage(c echo.Context, messageData map[string]interface{}) error {
	// Convert to bytes for StandardMCPServer
	messageBytes, err := json.Marshal(messageData)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Failed to marshal message",
		})
	}

	// Use StandardMCPServer to handle the request
	responseBytes, err := mcpServer.HandleJSONRPCRequest(messageBytes)
	if err != nil {
		logger.Error("MCP server error", "error", err)

		// Create error response
		var messageID interface{}
		if id, ok := messageData["id"]; ok {
			messageID = id
		}

		errorResponse := &common.MCPMessage{
			JSONRpc: "2.0",
			ID:      messageID,
			Error: &common.MCPError{
				Code:    -32603,
				Message: "Internal error",
				Data:    err.Error(),
			},
		}
		return c.JSON(http.StatusOK, errorResponse)
	}

	// Parse and return response
	var response interface{}
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		logger.Error("Failed to parse MCP response", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to parse response",
		})
	}

	return c.JSON(http.StatusOK, response)
}

// handleBatchMCPMessages processes batch JSON-RPC messages
func handleBatchMCPMessages(c echo.Context, messagesData []interface{}) error {
	responses := make([]interface{}, 0, len(messagesData))

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

		// Convert to bytes for StandardMCPServer
		messageBytes, err := json.Marshal(messageMap)
		if err != nil {
			errorResponse := &common.MCPMessage{
				JSONRpc: "2.0",
				ID:      messageMap["id"],
				Error: &common.MCPError{
					Code:    -32600,
					Message: "Invalid Request",
					Data:    "Failed to marshal message",
				},
			}
			responses = append(responses, errorResponse)
			continue
		}

		// Handle message using StandardMCPServer
		responseBytes, err := mcpServer.HandleJSONRPCRequest(messageBytes)
		if err != nil {
			logger.Error("MCP server error in batch", "error", err)
			errorResponse := &common.MCPMessage{
				JSONRpc: "2.0",
				ID:      messageMap["id"],
				Error: &common.MCPError{
					Code:    -32603,
					Message: "Internal error",
					Data:    err.Error(),
				},
			}
			responses = append(responses, errorResponse)
		} else {
			var response interface{}
			if err := json.Unmarshal(responseBytes, &response); err != nil {
				logger.Error("Failed to parse MCP response in batch", "error", err)
				errorResponse := &common.MCPMessage{
					JSONRpc: "2.0",
					ID:      messageMap["id"],
					Error: &common.MCPError{
						Code:    -32603,
						Message: "Parse error",
						Data:    err.Error(),
					},
				}
				responses = append(responses, errorResponse)
			} else {
				responses = append(responses, response)
			}
		}
	}

	return c.JSON(http.StatusOK, responses)
}

// --- Server-Sent Events Support ---

// handleMCPSSE handles Server-Sent Events for MCP connections (like Cline)
func handleMCPSSE(c echo.Context) error {
	// Validate authentication
	token := c.Request().Header.Get("token")
	if token == "" || token != common.Config.Token {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
		})
	}

	// Create session for SSE connection
	session := createSession(token)

	// Set SSE headers
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Access-Control-Allow-Headers", "token, Content-Type, Mcp-Session-Id")
	c.Response().Header().Set(MCPSessionHeader, session.ID)

	w := c.Response()

	// Send initial connection notification
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
			deleteSession(session.ID)
			return nil
		case <-ticker.C:
			// Send heartbeat
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

// --- MCP Information Endpoints ---

// MCP Server information endpoint (for discovery)
func getMCPInfo(c echo.Context) error {
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	info := map[string]interface{}{
		"protocol": "Model Context Protocol",
		"version":  common.MCPVersion,
		"server": map[string]interface{}{
			"name":    "AgentSmith-HUB",
			"version": "0.1.2",
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
		},
		"transport": "http",
		"endpoint":  "/mcp",
		"node_info": map[string]interface{}{
			"is_leader": cluster.IsLeader,
			"node_id":   cluster.NodeID,
		},
		"implementation": GetMCPImplementationStatus(),
	}

	return c.JSON(http.StatusOK, info)
}

// MCP Server manifest (for client discovery)
func getMCPManifest(c echo.Context) error {
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	// Get current host for proper URLs
	scheme := "http"
	if c.Request().TLS != nil {
		scheme = "https"
	}
	host := c.Request().Host
	protocol := scheme

	manifest := map[string]interface{}{
		"mcpVersion":  common.MCPVersion,
		"name":        "AgentSmith-HUB MCP Server",
		"version":     "0.1.2",
		"description": "Model Context Protocol server for AgentSmith-HUB Security Data Pipe Platform providing comprehensive access to security management tools",
		"author":      "AgentSmith-HUB Team",
		"license":     "MIT",
		"homepage":    "https://github.com/your-org/AgentSmith-HUB",
		"transport": map[string]interface{}{
			"type": "http",
			"host": host,
			"path": "/mcp",
		},
		"capabilities": map[string]interface{}{
			"resources": map[string]interface{}{
				"description": "Access to project configurations, inputs, outputs, rulesets, and plugins",
				"supported":   true,
			},
			"tools": map[string]interface{}{
				"description": "Complete API access to all AgentSmith-HUB functionality",
				"count":       len(mcpServer.GetAPIMapper().GetAllAPITools()),
				"supported":   true,
			},
			"prompts": map[string]interface{}{
				"description": "Intelligent prompts for project analysis, debugging, optimization, and plugin development",
				"count":       6,
				"supported":   true,
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
		"installation": map[string]interface{}{
			"endpoint":    fmt.Sprintf("%s://%s/mcp", protocol, host),
			"auth_header": "token",
			"auth_value":  "your-auth-token-here",
		},
	}

	return c.JSON(http.StatusOK, manifest)
}

// MCP Statistics endpoint
func getMCPStats(c echo.Context) error {
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	stats := map[string]interface{}{
		"server": map[string]interface{}{
			"initialized": mcpInitialized,
			"version":     common.MCPVersion,
			"library":     "mcp-go + custom handlers",
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
		"tools": map[string]interface{}{
			"total_tools": len(mcpServer.GetAPIMapper().GetAllAPITools()),
		},
		"sessions": map[string]interface{}{
			"active_count": len(mcpSessions),
		},
		"implementation": GetMCPImplementationStatus(),
	}

	return c.JSON(http.StatusOK, stats)
}

// Health check endpoint
func mcpHealthCheck(c echo.Context) error {
	if err := checkLeaderMode(c); err != nil {
		return err
	}

	health := map[string]interface{}{
		"status":      "healthy",
		"timestamp":   time.Now().Unix(),
		"initialized": mcpInitialized,
		"cluster": map[string]interface{}{
			"is_leader": cluster.IsLeader,
			"node_id":   cluster.NodeID,
		},
	}

	return c.JSON(http.StatusOK, health)
}

// Helper function to extract port from host
func getPortFromHost(host string) int {
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		if port, err := strconv.Atoi(host[idx+1:]); err == nil {
			return port
		}
	}
	return 8080 // Default port
}
