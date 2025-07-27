package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// HTTPClientConfig holds configuration for HTTP client
type HTTPClientConfig struct {
	BaseURL         string
	Token           string
	Timeout         time.Duration
	MaxIdleConns    int
	MaxConnsPerHost int
	RetryAttempts   int
	RetryDelay      time.Duration
}

// OptimizedHTTPClient provides an optimized HTTP client with connection pooling
type OptimizedHTTPClient struct {
	config *HTTPClientConfig
	client *http.Client
	mu     sync.RWMutex
}

// NewOptimizedHTTPClient creates a new optimized HTTP client
func NewOptimizedHTTPClient(config *HTTPClientConfig) *OptimizedHTTPClient {
	// Set defaults
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = 100
	}
	if config.MaxConnsPerHost == 0 {
		config.MaxConnsPerHost = 10
	}
	if config.RetryAttempts == 0 {
		config.RetryAttempts = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 1 * time.Second
	}

	// Configure transport with connection pooling
	transport := &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxConnsPerHost,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}

	return &OptimizedHTTPClient{
		config: config,
		client: httpClient,
	}
}

// MakeRequest makes an HTTP request with retry logic and proper error handling
func (c *OptimizedHTTPClient) MakeRequest(method, endpoint string, body interface{}, requireAuth bool) ([]byte, error) {
	url := c.config.BaseURL + endpoint

	var reqBody io.Reader
	if body != nil && (method == "POST" || method == "PUT") {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	// Retry logic
	var lastErr error
	for attempt := 0; attempt < c.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(c.config.RetryDelay * time.Duration(attempt))
		}

		result, err := c.doRequest(method, url, reqBody, requireAuth)
		if err == nil {
			return result, nil
		}
		lastErr = err

		// Don't retry on client errors (4xx)
		if httpErr, ok := err.(*HTTPError); ok && httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
			break
		}
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", c.config.RetryAttempts, lastErr)
}

// doRequest performs a single HTTP request
func (c *OptimizedHTTPClient) doRequest(method, url string, body io.Reader, requireAuth bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if requireAuth {
		req.Header.Set("token", c.config.Token)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Message:    string(responseBody),
		}
	}

	return responseBody, nil
}

// HTTPError represents an HTTP error
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// UpdateConfig safely updates client configuration
func (c *OptimizedHTTPClient) UpdateConfig(baseURL, token string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if baseURL != "" {
		c.config.BaseURL = baseURL
	}
	if token != "" {
		c.config.Token = token
	}
}
