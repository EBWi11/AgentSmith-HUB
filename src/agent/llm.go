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
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
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
// tokenLimitParam: "max_tokens" | "max_completion_tokens" | "auto" (see resolveTokenLimitField).
// reasoningMode:
//   - "disabled": never send provider-specific reasoning params
//   - "enabled" : always send reasoning params for supported models
//   - "auto"    : enable reasoning based on model name heuristics
func callChatWithTools(model string, messages []Message, tools []ToolDefinition,
	maxTokens int, temperature float64, tokenLimitParam string, reasoningMode string, reasoningBudgetTokens int, ctx ...context.Context) (*ChatResult, error) {

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
		reqBody[resolveTokenLimitField(model, tokenLimitParam)] = maxTokens
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

// resolveTokenLimitField picks the chat-completions field that carries the token limit.
// Prefer explicit agent YAML (token_limit_param); "auto" falls back to model-name heuristics.
func resolveTokenLimitField(model, param string) string {
	switch strings.ToLower(strings.TrimSpace(param)) {
	case "max_completion_tokens":
		return "max_completion_tokens"
	case "auto":
		return tokenLimitFieldForModel(model)
	default:
		return "max_tokens"
	}
}

// tokenLimitFieldForModel is used only when token_limit_param is "auto".
func tokenLimitFieldForModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	switch {
	case strings.HasPrefix(m, "o1"), strings.HasPrefix(m, "o3"), strings.HasPrefix(m, "o4"):
		return "max_completion_tokens"
	case strings.HasPrefix(m, "gpt-5"), strings.HasPrefix(m, "gpt-4.1"):
		return "max_completion_tokens"
	default:
		return "max_tokens"
	}
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

const (
	memoryRunContextMaxField   = 12000
	memorySystemPromptMaxField = 32000
)

func truncateMemoryContextField(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= memoryRunContextMaxField {
		return s
	}
	return s[:memoryRunContextMaxField] + fmt.Sprintf("\n... (truncated, %d bytes total)", len(s))
}

// MemoryRunContext is optional background from one agent log run. It must not
// override existing_memory or user comments; it only helps interpret feedback.
type MemoryRunContext struct {
	Error     string `json:"error,omitempty"`
	RawInput  string `json:"raw_input,omitempty"`
	RawOutput string `json:"raw_output,omitempty"`
	Trace     string `json:"trace,omitempty"`
}

// BuildMemoryRunContext returns truncated run context for memory regeneration, or nil if empty.
func BuildMemoryRunContext(entry *common.AgentLogEntry) *MemoryRunContext {
	if entry == nil {
		return nil
	}
	ctx := &MemoryRunContext{
		Error:     truncateMemoryContextField(entry.Error),
		RawInput:  truncateMemoryContextField(entry.RawInput),
		RawOutput: truncateMemoryContextField(entry.RawOutput),
		Trace:     truncateMemoryContextField(entry.Trace),
	}
	if ctx.Error == "" && ctx.RawInput == "" && ctx.RawOutput == "" && ctx.Trace == "" {
		return nil
	}
	return ctx
}

func truncateSystemPromptForMemory(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= memorySystemPromptMaxField {
		return s
	}
	return s[:memorySystemPromptMaxField] + fmt.Sprintf("\n... (truncated, %d bytes total)", len(s))
}

// GenerateMemorySummary performs a full review of existing agent memory_notes together
// with user comments on a log run, and returns the complete replacement text for
// memory_notes (plain text, not JSON). On conflicts, newer user comments win.
// systemPrompt is the agent's base system prompt (read-only context: what this agent does).
// runCtx is optional background (input/output/trace) and must not be treated as
// the only source of truth. Model defaults to HubConfig.LLMModel when empty.
func GenerateMemorySummary(model string, agentID string, systemPrompt string, existingMemory string, comments []common.AgentLogComment, runCtx *MemoryRunContext) (string, error) {
	if len(comments) == 0 {
		return "", fmt.Errorf("no comments to summarize")
	}

	commentsJSON, _ := json.Marshal(comments)
	userInput := map[string]interface{}{
		"agent_id":        agentID,
		"existing_memory": strings.TrimSpace(existingMemory),
		"comments":        json.RawMessage(commentsJSON),
	}
	if sp := truncateSystemPromptForMemory(systemPrompt); sp != "" {
		userInput["system_prompt"] = sp
	}
	if runCtx != nil {
		userInput["run_context"] = runCtx
	}
	userRaw, _ := json.Marshal(userInput)

	system := `You are AgentSmith-HUB memory builder. Your job is a full pass over the agent's Memory Notes field (existing_memory in the JSON), not a single incremental bullet.

Inputs in the user message JSON:
- system_prompt (optional): the agent's configured base system prompt — read-only context so you know this agent's role, output shape, and domain. Do NOT copy it into memory_notes verbatim; do NOT edit or replace it; use it only to align memory bullets with what the agent is actually supposed to do.
- existing_memory: current persisted guidance (may be empty).
- comments: human feedback for THIS review cycle (authoritative when they conflict with older material).
- run_context (optional): snippets from this agent run (input, output, trace, error). Use only to understand what the comments refer to. Do NOT replace or ignore existing_memory based on run_context alone.

	Requirements:
1) Merge existing_memory with the themes in comments: deduplicate by *intent*, compress redundant lines, and keep durable guidance (patterns, FP/TP heuristics, prompt/style tweaks, confidence rules).
2) If anything in existing_memory conflicts with comments, follow the comments and drop or rewrite the old line.
3) Output structure: produce a concise list (bullets or numbered items). Do not repeat similar thresholds; if two items overlap, merge them. Prefer fewer, higher-signal items over verbose restatement.
4) Default stance: treat scan/attack as NOT authorized unless explicit, verifiable evidence indicates authorization; if evidence is uncertain/insufficient, keep the NOT authorized stance.
5) Whitelist scoping: when authorizing is justified, narrow the allowed scope to specific stable identifiers/field combinations; avoid broad catch-all exceptions.
6) Prefer stable rules over one-off volatile details (exact IPs, ephemeral IDs) unless the comments explicitly require them.
7) Produce the COMPLETE new memory_notes body as plain text (no JSON, no markdown fences). This output replaces the entire memory_notes field — do not assume it will be appended.
8) Stay within reasonable length: compress aggressively while preserving distinct rules.`

	messages := []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: string(userRaw)},
	}

	resp, err := callChatWithTools(model, messages, nil, 4096, 0, "auto", "disabled", 0)
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(resp.Content)
	if out == "" {
		return "", fmt.Errorf("empty memory summary from model")
	}
	return out, nil
}
