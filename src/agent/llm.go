package agent

import (
	"AgentSmith-HUB/common"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "https://api.openai.com/v1"
	defaultModel     = "gpt-4o-mini"
	defaultMaxTokens = 4096
	requestTimeout   = 120 * time.Second
)

// ToolDefinition describes a function the LLM can call (OpenAI-compatible).
type ToolDefinition struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall represents a single tool invocation requested by the LLM.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Message is an OpenAI-compatible chat message supporting tool roles.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`

	// Optional reasoning / thinking fields for providers that support them
	// (e.g. kimi-k2.5, Claude with thinking blocks). These are passed through
	// transparently when present so multi-turn tool calls work correctly.
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ThinkingBlocks   json.RawMessage `json:"thinking_blocks,omitempty"`
}

// chatRequestWithTools is a flexible request map so we can attach
// provider-specific fields (e.g. thinking / reasoning options) when needed.
type chatRequestWithTools map[string]interface{}

type chatResponseWithTools struct {
	Choices []struct {
		Message struct {
			Role       string          `json:"role"`
			Content    string          `json:"content"`
			ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
			// Provider-specific reasoning fields (if returned)
			ReasoningContent string          `json:"reasoning_content,omitempty"`
			ThinkingBlocks   json.RawMessage `json:"thinking_blocks,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// ChatResult is the parsed response from a tool-aware LLM call.
type ChatResult struct {
	Content          string
	ToolCalls        []ToolCall
	Role             string
	ReasoningContent string
	ThinkingBlocks   json.RawMessage
}

// AssistantMessage converts the ChatResult back into a Message for the conversation.
func (r *ChatResult) AssistantMessage() Message {
	return Message{
		Role:             "assistant",
		Content:          r.Content,
		ToolCalls:        r.ToolCalls,
		ReasoningContent: r.ReasoningContent,
		ThinkingBlocks:   r.ThinkingBlocks,
	}
}

// ToolResultMessage creates a tool-result message to feed back to the LLM.
func ToolResultMessage(toolCallID, content string) Message {
	return Message{
		Role:       "tool",
		Content:    content,
		ToolCallID: toolCallID,
	}
}

// callChatWithTools performs a single tool-aware LLM call.
// reasoningMode:
//   - "disabled": never send provider-specific reasoning params
//   - "enabled" : always send reasoning params for supported models
//   - "auto"    : enable reasoning based on model name heuristics
func callChatWithTools(model string, messages []Message, tools []ToolDefinition,
	maxTokens int, temperature float64, reasoningMode string, reasoningBudgetTokens int, ctx ...context.Context) (*ChatResult, error) {

	if common.Config == nil || strings.TrimSpace(common.Config.LLMApiKey) == "" {
		return nil, fmt.Errorf("LLM API key not configured")
	}

	apiKey := strings.TrimSpace(common.Config.LLMApiKey)
	baseURL := strings.TrimSpace(common.Config.LLMBaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	if model == "" {
		model = strings.TrimSpace(common.Config.LLMModel)
		if model == "" {
			model = defaultModel
		}
	}
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	reqBody := chatRequestWithTools{
		"model":    model,
		"messages": messages,
	}
	if maxTokens > 0 {
		reqBody["max_tokens"] = maxTokens
	}
	if temperature > 0 {
		// Only send temperature when explicitly configured; leaving it out
		// lets the backend apply its own sensible default.
		reqBody["temperature"] = temperature
	}
	if len(tools) > 0 {
		reqBody["tools"] = tools
	}

	// Attach provider-specific reasoning / thinking parameters when requested.
	reasoningMode = strings.ToLower(strings.TrimSpace(reasoningMode))
	if shouldEnableKimiThinking(model, reasoningMode) {
		thinking := map[string]interface{}{
			"type": "enabled",
		}
		if reasoningBudgetTokens > 0 {
			thinking["budget_tokens"] = reasoningBudgetTokens
		}
		reqBody["thinking"] = thinking
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := baseURL + "/chat/completions"

	var reqCtx context.Context
	if len(ctx) > 0 && ctx[0] != nil {
		reqCtx = ctx[0]
	} else {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
	}

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("User-Agent", "AgentSmith-HUB/agent")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("LLM request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponseWithTools
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parse response: %w (body: %.200s)", err, string(respBody))
	}
	if chatResp.Error != nil {
		return nil, fmt.Errorf("LLM API error: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM API returned no choices")
	}

	choice := chatResp.Choices[0]
	return &ChatResult{
		Content:          strings.TrimSpace(choice.Message.Content),
		ToolCalls:        choice.Message.ToolCalls,
		Role:             choice.Message.Role,
		ReasoningContent: choice.Message.ReasoningContent,
		ThinkingBlocks:   choice.Message.ThinkingBlocks,
	}, nil
}

// shouldEnableKimiThinking determines whether to attach Kimi-style "thinking" params.
// This is intentionally conservative and only triggers when explicitly requested
// or when reasoningMode is "auto" and the model name matches known patterns.
func shouldEnableKimiThinking(model, reasoningMode string) bool {
	modelLower := strings.ToLower(strings.TrimSpace(model))
	if modelLower == "" {
		return false
	}

	switch reasoningMode {
	case "disabled":
		return false
	case "enabled":
		return true
	case "auto":
		// Heuristic: enable for kimi-k2.5 family by default in auto mode.
		if strings.Contains(modelLower, "kimi-k2.5") {
			return true
		}
	}
	return false
}

// GenerateMemorySummary builds concise memory notes from user comments.
// Model defaults to HubConfig.LLMModel when empty.
func GenerateMemorySummary(model string, agentID string, existingMemory string, comments []common.AgentLogComment) (string, error) {
	if len(comments) == 0 {
		return "", fmt.Errorf("no comments to summarize")
	}

	commentsJSON, _ := json.Marshal(comments)
	userInput := map[string]interface{}{
		"agent_id":        agentID,
		"existing_memory": existingMemory,
		"comments":        json.RawMessage(commentsJSON),
	}
	userRaw, _ := json.Marshal(userInput)

	messages := []Message{
		{
			Role: "system",
			Content: "You are AgentSmith-HUB memory builder. Summarize user comments into 3-6 concise, durable guidance bullets for future decisions. " +
				"Output plain text only (no JSON). Focus on reusable decision patterns, known false positives, and confidence adjustments. " +
				"Avoid specific volatile details like exact IPs/usernames.",
		},
		{
			Role:    "user",
			Content: string(userRaw),
		},
	}

	resp, err := callChatWithTools(model, messages, nil, 512, 0, "disabled", 0)
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(resp.Content)
	if out == "" {
		return "", fmt.Errorf("empty memory summary from model")
	}
	return out, nil
}
