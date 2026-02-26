package llm_call

import (
	"AgentSmith-HUB/common"
	"bytes"
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
	defaultMaxTokens = 2048
	requestTimeout   = 60 * time.Second
)

// chatRequest is OpenAI-compatible chat completions request body
type chatRequest struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the minimal structure we need from OpenAI chat completions response
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			Role    string `json:"role"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// callLLM performs a single chat completion using apiKey and baseURL from common.Config
func callLLM(systemPrompt, userMessage, model string, maxTokens int) (string, error) {
	if common.Config == nil || strings.TrimSpace(common.Config.LLMApiKey) == "" {
		return "", fmt.Errorf("LLM API key not configured: set llm_api_key in config.yaml")
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

	messages := []message{
		{Role: "system", Content: systemPrompt},
	}
	if userMessage != "" {
		messages = append(messages, message{Role: "user", Content: userMessage})
	} else {
		// Single-shot with system only: send one user message so model has something to respond to
		messages = append(messages, message{Role: "user", Content: "Proceed according to the system instructions and respond."})
	}

	body := chatRequest{
		Model:     model,
		Messages:  messages,
		MaxTokens: maxTokens,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := baseURL + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "AgentSmith-HUB/1.0")

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if chatResp.Error != nil {
		return "", fmt.Errorf("LLM API error: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM API returned no choices")
	}
	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// Eval performs a single LLM call with system prompt as parameter.
// apiKey and baseURL are read from config (llm_api_key, llm_base_url); plugin is only registered when llm_api_key is set.
// Args: systemPrompt string (required), userMessage string (optional), model string (optional), maxTokens int (optional).
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) < 1 {
		return nil, false, fmt.Errorf("llmCall requires at least 1 argument: systemPrompt string")
	}
	systemPrompt, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("first argument (systemPrompt) must be a string")
	}
	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == "" {
		return nil, false, fmt.Errorf("systemPrompt cannot be empty")
	}

	userMessage := ""
	if len(args) >= 2 {
		if u, ok := args[1].(string); ok {
			userMessage = strings.TrimSpace(u)
		}
	}
	model := ""
	if len(args) >= 3 {
		if m, ok := args[2].(string); ok {
			model = strings.TrimSpace(m)
		}
	}
	maxTokens := 0
	if len(args) >= 4 {
		switch v := args[3].(type) {
		case int:
			maxTokens = v
		case int64:
			maxTokens = int(v)
		case float64:
			maxTokens = int(v)
		}
	}

	reply, err := callLLM(systemPrompt, userMessage, model, maxTokens)
	if err != nil {
		return nil, false, err
	}
	return reply, true, nil
}
