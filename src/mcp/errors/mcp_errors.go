package errors

import (
	"AgentSmith-HUB/common"
	"fmt"
)

// MCPError types for better error handling
type MCPErrorType int

const (
	ErrValidation MCPErrorType = iota
	ErrAPI
	ErrAuth
	ErrNetwork
	ErrInternal
)

// MCPError represents a structured MCP error
type MCPError struct {
	Type        MCPErrorType
	Message     string
	Suggestions []string // Specific actionable suggestions for LLM
	Details     map[string]interface{}
}

func (e MCPError) Error() string {
	return e.Message
}

// ToMCPResult converts MCPError to MCPToolResult
func (e MCPError) ToMCPResult() common.MCPToolResult {
	var prefix string
	switch e.Type {
	case ErrValidation:
		prefix = "❌ Validation Error"
	case ErrAPI:
		prefix = "🔧 API Error"
	case ErrAuth:
		prefix = "🔐 Authentication Error"
	case ErrNetwork:
		prefix = "🌐 Network Error"
	case ErrInternal:
		prefix = "⚠️ Internal Error"
	default:
		prefix = "❌ Error"
	}

	// Build comprehensive error message with suggestions
	errorText := fmt.Sprintf("%s: %s", prefix, e.Message)
	if len(e.Suggestions) > 0 {
		errorText += "\n\n📋 **Suggested Actions:**"
		for i, suggestion := range e.Suggestions {
			errorText += fmt.Sprintf("\n%d. %s", i+1, suggestion)
		}
	}

	return common.MCPToolResult{
		Content: []common.MCPToolContent{{
			Type: "text",
			Text: errorText,
		}},
		IsError: true,
	}
}

// Common error constructors
func NewValidationError(message string, details ...map[string]interface{}) MCPError {
	var det map[string]interface{}
	if len(details) > 0 {
		det = details[0]
	}
	return MCPError{
		Type:        ErrValidation,
		Message:     message,
		Suggestions: []string{}, // Will be populated by specific error functions
		Details:     det,
	}
}

func NewValidationErrorWithSuggestions(message string, suggestions []string, details ...map[string]interface{}) MCPError {
	var det map[string]interface{}
	if len(details) > 0 {
		det = details[0]
	}
	return MCPError{
		Type:        ErrValidation,
		Message:     message,
		Suggestions: suggestions,
		Details:     det,
	}
}

func NewAPIError(message string, statusCode int) MCPError {
	var suggestions []string
	switch statusCode {
	case 400:
		suggestions = []string{
			"Check the request parameters for correct format and required fields",
			"Verify that all required parameters are provided",
			"Use 'get_pending_changes' to check if there are conflicting temporary changes",
		}
	case 401:
		suggestions = []string{
			"Authentication failed - check if the API token is valid",
			"Use 'token_check' tool to verify authentication status",
			"Contact administrator if token appears to be correct",
		}
	case 403:
		suggestions = []string{
			"Access denied - check if you have permissions for this operation",
			"Some operations require admin privileges",
			"Verify the component exists and you have access to it",
		}
	case 404:
		suggestions = []string{
			"The requested resource (ruleset, input, output, etc.) was not found",
			"Use 'get_rulesets', 'get_inputs', 'get_outputs' to list available components",
			"Check if the component ID is spelled correctly",
			"Use 'get_pending_changes' to see if the component exists but is not yet deployed",
		}
	case 409:
		suggestions = []string{
			"Conflict detected - the resource may already exist or be in use",
			"Check for existing components with the same ID",
			"Use 'get_pending_changes' to see if there are conflicting changes",
		}
	case 500:
		suggestions = []string{
			"Internal server error - try the operation again",
			"Check 'get_error_logs' for detailed error information",
			"If the problem persists, contact system administrator",
		}
	default:
		suggestions = []string{
			"Check the API documentation for this endpoint",
			"Verify all required parameters are provided correctly",
			"Use diagnostic tools like 'get_error_logs' for more information",
		}
	}

	return MCPError{
		Type:        ErrAPI,
		Message:     message,
		Suggestions: suggestions,
		Details:     map[string]interface{}{"statusCode": statusCode},
	}
}
