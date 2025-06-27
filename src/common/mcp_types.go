package common

import (
	"encoding/json"
	"time"
)

// MCP Protocol Version - using widely supported stable version
const MCPVersion = "2024-11-05"

// MCP Methods
const (
	MCPInitialize    = "initialize"
	MCPListResources = "resources/list"
	MCPReadResource  = "resources/read"
	MCPListTools     = "tools/list"
	MCPCallTool      = "tools/call"
	MCPListPrompts   = "prompts/list"
	MCPGetPrompt     = "prompts/get"
)

// MCPMessage is the base structure for all MCP communication.
type MCPMessage struct {
	JSONRpc string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

// MCPError provides a structured error response.
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"` // Detailed error information
}

// --- Initialization ---

type MCPInitializeParams struct {
	ProtocolVersion string                `json:"protocolVersion"`
	Capabilities    MCPClientCapabilities `json:"capabilities,omitempty"`
	ClientInfo      MCPImplementationInfo `json:"clientInfo,omitempty"`
}

type MCPInitializeResult struct {
	ProtocolVersion string                `json:"protocolVersion"`
	Capabilities    MCPServerCapabilities `json:"capabilities"`
	ServerInfo      MCPImplementationInfo `json:"serverInfo"`
}

type MCPClientCapabilities struct{}
type MCPServerCapabilities struct {
	Resources *MCPResourceCapability `json:"resources,omitempty"`
	Tools     *MCPToolsCapability    `json:"tools,omitempty"`
	Prompts   *MCPPromptsCapability  `json:"prompts,omitempty"`
}
type MCPResourceCapability struct{}
type MCPToolsCapability struct{}
type MCPPromptsCapability struct{}

type MCPImplementationInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// --- Resources ---

type MCPResource struct {
	URI          string            `json:"uri"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	MimeType     string            `json:"mimeType,omitempty"`
	LastModified *time.Time        `json:"lastModified,omitempty"` // Added for smart caching
	Size         int64             `json:"size,omitempty"`         // Added for resource assessment
	Annotations  map[string]string `json:"annotations,omitempty"`
}

type MCPListResourcesResult struct {
	Resources []MCPResource `json:"resources"`
}

type MCPReadResourceParams struct {
	URI string `json:"uri"`
}

type MCPResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text,omitempty"`
	Blob     []byte `json:"blob,omitempty"`
}

type MCPReadResourceResult struct {
	Contents []MCPResourceContents `json:"contents"`
}

// --- Tools ---

type MCPTool struct {
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	InputSchema map[string]MCPToolArg `json:"inputSchema,omitempty"` // Enhanced for strong typing
}

// MCPToolArg defines a strongly-typed argument for a tool.
type MCPToolArg struct {
	Type        string `json:"type"` // e.g., "string", "number", "boolean"
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type MCPListToolsResult struct {
	Tools []MCPTool `json:"tools"`
}

type MCPCallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type MCPToolResult struct {
	Content  []MCPToolContent       `json:"content"`
	IsError  bool                   `json:"isError,omitempty"`
	Metadata map[string]interface{} `json:"_meta,omitempty"`
}

// MCPToolContent provides a structured format for tool output.
type MCPToolContent struct {
	Format string `json:"format"` // Only "text" and "image" are supported by MCP specification
	Text   string `json:"text"`
}

// --- Prompts ---

type MCPPrompt struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Arguments   []MCPPromptArg `json:"arguments,omitempty"`
	Template    string         `json:"template,omitempty"`
}

// MCPPromptArg defines a strongly-typed argument for a prompt.
type MCPPromptArg struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // e.g., "string", "number", "boolean"
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type MCPListPromptsResult struct {
	Prompts []MCPPrompt `json:"prompts"`
}

type MCPGetPromptParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type MCPPromptMessage struct {
	Role    string           `json:"role"`
	Content MCPPromptContent `json:"content"`
}

type MCPPromptContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MCPGetPromptResult struct {
	Description string             `json:"description,omitempty"`
	Messages    []MCPPromptMessage `json:"messages"`
}
