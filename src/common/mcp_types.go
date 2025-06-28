package common

import (
	"encoding/json"
	"time"
)

// MCP Protocol Version - updated to actual latest stable version
const MCPVersion = "2025-03-26"

// MCP Methods
const (
	MCPInitialize    = "initialize"
	MCPListResources = "resources/list"
	MCPReadResource  = "resources/read"
	MCPListTools     = "tools/list"
	MCPCallTool      = "tools/call"
	MCPListPrompts   = "prompts/list"
	MCPGetPrompt     = "prompts/get"
	// New methods in 2025-06-18
	MCPElicit   = "elicit"
	MCPComplete = "completion/complete"
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

type MCPClientCapabilities struct {
	Sampling    *MCPSamplingCapability    `json:"sampling,omitempty"`
	Roots       *MCPRootsCapability       `json:"roots,omitempty"`
	Elicitation *MCPElicitationCapability `json:"elicitation,omitempty"`
}

type MCPServerCapabilities struct {
	Resources *MCPResourceCapability `json:"resources,omitempty"`
	Tools     *MCPToolsCapability    `json:"tools,omitempty"`
	Prompts   *MCPPromptsCapability  `json:"prompts,omitempty"`
	Logging   *MCPLoggingCapability  `json:"logging,omitempty"`
}

type MCPResourceCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

type MCPToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type MCPPromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type MCPSamplingCapability struct{}
type MCPRootsCapability struct{}
type MCPElicitationCapability struct{}
type MCPLoggingCapability struct{}

type MCPImplementationInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// --- Resources ---

type MCPResource struct {
	URI          string                 `json:"uri"`
	Name         string                 `json:"name"`
	Title        string                 `json:"title,omitempty"` // Human-friendly display name (2025-06-18)
	Description  string                 `json:"description,omitempty"`
	MimeType     string                 `json:"mimeType,omitempty"`
	LastModified *time.Time             `json:"lastModified,omitempty"` // Added for smart caching
	Size         int64                  `json:"size,omitempty"`         // Added for resource assessment
	Annotations  map[string]string      `json:"annotations,omitempty"`
	Meta         map[string]interface{} `json:"_meta,omitempty"` // Metadata field (2025-06-18)
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

// MCPToolAnnotations provide additional metadata about a tool's behavior
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

type MCPListToolsResult struct {
	Tools []MCPTool `json:"tools"`
}

type MCPCallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
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

// --- Prompts ---

type MCPPrompt struct {
	Name        string                 `json:"name"`
	Title       string                 `json:"title,omitempty"` // Human-friendly display name (2025-06-18)
	Description string                 `json:"description,omitempty"`
	Arguments   []MCPPromptArg         `json:"arguments,omitempty"`
	Template    string                 `json:"template,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"` // Metadata field (2025-06-18)
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
	Description string                 `json:"description,omitempty"`
	Messages    []MCPPromptMessage     `json:"messages"`
	Meta        map[string]interface{} `json:"_meta,omitempty"` // Metadata field (2025-06-18)
}

// --- Elicitation (2025-06-18) ---

type MCPElicitParams struct {
	Prompt      string                 `json:"prompt"`
	MaxTokens   int                    `json:"maxTokens,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
}

type MCPElicitResult struct {
	Response string                 `json:"response"`
	Meta     map[string]interface{} `json:"_meta,omitempty"`
}

// --- Completion (2025-06-18) ---

type MCPCompletionRequest struct {
	Ref     MCPCompletionRef       `json:"ref"`
	Context map[string]interface{} `json:"context,omitempty"` // Previously-resolved variables
	Meta    map[string]interface{} `json:"_meta,omitempty"`
}

type MCPCompletionRef struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type MCPCompletionResult struct {
	Completion MCPCompletion          `json:"completion"`
	Meta       map[string]interface{} `json:"_meta,omitempty"`
}

type MCPCompletion struct {
	Values []string               `json:"values"`
	Total  int                    `json:"total,omitempty"`
	Meta   map[string]interface{} `json:"_meta,omitempty"`
}

// --- Sampling ---

type MCPSamplingMessage struct {
	Role    string                 `json:"role"`
	Content MCPSamplingContent     `json:"content"`
	Meta    map[string]interface{} `json:"_meta,omitempty"`
}

type MCPSamplingContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MCPCreateMessageRequest struct {
	Messages       []MCPSamplingMessage   `json:"messages"`
	SystemPrompt   string                 `json:"systemPrompt,omitempty"`
	IncludeContext string                 `json:"includeContext,omitempty"`
	Temperature    float64                `json:"temperature,omitempty"`
	MaxTokens      int                    `json:"maxTokens,omitempty"`
	StopSequences  []string               `json:"stopSequences,omitempty"`
	Meta           map[string]interface{} `json:"_meta,omitempty"`
}

type MCPCreateMessageResult struct {
	Role       string                 `json:"role"`
	Content    MCPSamplingContent     `json:"content"`
	Model      string                 `json:"model"`
	StopReason string                 `json:"stopReason,omitempty"`
	Meta       map[string]interface{} `json:"_meta,omitempty"`
}

// --- Roots ---

type MCPListRootsResult struct {
	Roots []MCPRoot              `json:"roots"`
	Meta  map[string]interface{} `json:"_meta,omitempty"`
}

type MCPRoot struct {
	URI  string                 `json:"uri"`
	Name string                 `json:"name,omitempty"`
	Meta map[string]interface{} `json:"_meta,omitempty"`
}

// --- Logging ---

type MCPLoggingLevel string

const (
	MCPLogDebug     MCPLoggingLevel = "debug"
	MCPLogInfo      MCPLoggingLevel = "info"
	MCPLogNotice    MCPLoggingLevel = "notice"
	MCPLogWarning   MCPLoggingLevel = "warning"
	MCPLogError     MCPLoggingLevel = "error"
	MCPLogCritical  MCPLoggingLevel = "critical"
	MCPLogAlert     MCPLoggingLevel = "alert"
	MCPLogEmergency MCPLoggingLevel = "emergency"
)

type MCPLogEntry struct {
	Level  MCPLoggingLevel        `json:"level"`
	Data   interface{}            `json:"data,omitempty"`
	Logger string                 `json:"logger,omitempty"`
	Meta   map[string]interface{} `json:"_meta,omitempty"`
}
