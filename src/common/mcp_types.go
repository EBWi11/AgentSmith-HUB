package common

import (
	"encoding/json"
)

// MCP Protocol Version - updated to actual latest stable version
const MCPVersion = "2025-03-26"

// MCPMessage is the base structure for all MCP communication.
type MCPMessage struct {
	JSONRpc string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"` // Detailed error information
}

type MCPToolAnnotations struct {
	Title           string `json:"title,omitempty"`           // Human-readable title for the tool
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`    // If true, the tool does not modify its environment
	DestructiveHint *bool  `json:"destructiveHint,omitempty"` // If true, the tool may perform destructive updates
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`  // If true, repeated calls with same args have no additional effect
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`   // If true, tool interacts with external entities
}

type MCPTool struct {
	Name        string                 `json:"name"`
	Title       string                 `json:"title,omitempty"` // Human-friendly display name (2025-06-18)
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]MCPToolArg  `json:"inputSchema,omitempty"` // Enhanced for strong typing
	Annotations *MCPToolAnnotations    `json:"annotations,omitempty"` // Tool behavior hints
	Meta        map[string]interface{} `json:"_meta,omitempty"`       // Metadata field (2025-06-18)
}

// MCPToolArg defines a strongly-typed argument for a tool.
type MCPToolArg struct {
	Type        string `json:"type"` // e.g., "string", "number", "boolean"
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type MCPToolResult struct {
	Content []MCPToolContent       `json:"content"`
	IsError bool                   `json:"isError,omitempty"`
	Meta    map[string]interface{} `json:"_meta,omitempty"`
}

// MCPToolContent provides a structured format for tool output with 2025-06-18 enhancements
type MCPToolContent struct {
	Type     string                 `json:"type"`               // "text", "image", "resource" (2025-06-18)
	Text     string                 `json:"text,omitempty"`     // For text content
	Data     string                 `json:"data,omitempty"`     // For image content (base64)
	MimeType string                 `json:"mimeType,omitempty"` // For image content
	Resource *MCPResourceLink       `json:"resource,omitempty"` // Resource link (2025-06-18)
	Meta     map[string]interface{} `json:"_meta,omitempty"`    // Metadata field (2025-06-18)
}

// MCPResourceLink for linking resources in tool call results (2025-06-18)
type MCPResourceLink struct {
	URI         string                 `json:"uri"`
	Type        string                 `json:"type"` // Type of resource
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
}

type MCPPrompt struct {
	Name         string                 `json:"name"`
	Title        string                 `json:"title,omitempty"` // Human-friendly display name (2025-06-18)
	Description  string                 `json:"description,omitempty"`
	Arguments    []MCPPromptArg         `json:"arguments,omitempty"`
	Template     string                 `json:"template,omitempty"`
	Texts        map[string]string      `json:"texts,omitempty"` // optional per-language text variants
	Placeholders []string               `json:"placeholders,omitempty"`
	Meta         map[string]interface{} `json:"_meta,omitempty"` // Metadata field (2025-06-18)
}

// MCPPromptArg defines a strongly-typed argument for a prompt.
type MCPPromptArg struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // e.g., "string", "number", "boolean"
	Description string `json:"description"`
	Required    bool   `json:"required"`
}
