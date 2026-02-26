package smith_agent

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
	requestTimeout   = 90 * time.Second
	probeTimeout     = 15 * time.Second
)

type chatRequest struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

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

// callChat performs one OpenAI-compatible chat completion using common.Config (LLMApiKey, LLMBaseURL).
func callChat(systemPrompt, userMessage, model string, maxTokens int, timeout time.Duration) (string, error) {
	if common.Config == nil || strings.TrimSpace(common.Config.LLMApiKey) == "" {
		return "", fmt.Errorf("LLM API key not configured")
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
	if timeout <= 0 {
		timeout = requestTimeout
	}

	messages := []message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}
	body := chatRequest{Model: model, Messages: messages, MaxTokens: maxTokens}
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
	req.Header.Set("User-Agent", "AgentSmith-HUB/smith-agent")

	client := &http.Client{Timeout: timeout}
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

// isLLMConfigurationError returns true for non-retriable model/auth/permission misconfiguration errors.
func isLLMConfigurationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	keywords := []string{
		"not found the model",
		"model not found",
		"permission denied",
		"access denied",
		"forbidden",
		"unauthorized",
		"invalid api key",
		"incorrect api key",
		"does not have access",
		"insufficient permissions",
	}
	for _, kw := range keywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
