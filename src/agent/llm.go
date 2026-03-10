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

// Message is an OpenAI chat message supporting tool roles.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type chatRequestWithTools struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
}

type chatResponseWithTools struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
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
	Content   string
	ToolCalls []ToolCall
	Role      string
}

// AssistantMessage converts the ChatResult back into a Message for the conversation.
func (r *ChatResult) AssistantMessage() Message {
	return Message{
		Role:      "assistant",
		Content:   r.Content,
		ToolCalls: r.ToolCalls,
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

func callChatWithTools(model string, messages []Message, tools []ToolDefinition,
	maxTokens int, temperature float64, ctx ...context.Context) (*ChatResult, error) {

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
		Model:       model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
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
		Content:   strings.TrimSpace(choice.Message.Content),
		ToolCalls: choice.Message.ToolCalls,
		Role:      choice.Message.Role,
	}, nil
}
