package input

import (
	"testing"
)

func TestGrokParsing(t *testing.T) {
	// Test configuration with grok pattern
	config := `
type: kafka
kafka:
  brokers:
    - "localhost:9092"
  group: "test-group"
  topic: "test-topic"
grok_pattern: "%{IP:client} %{WORD:method} %{URIPATHPARAM:request} %{NUMBER:bytes} %{NUMBER:duration}"
`

	// Create input with grok configuration
	input, err := NewInput("", config, "test-input")
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Test data with message field
	testData := map[string]interface{}{
		"message":   "192.168.1.1 GET /api/users 200 150",
		"timestamp": "2024-01-01T12:00:00Z",
	}

	// Parse with grok
	result := input.parseWithGrok(testData)

	// Check if grok parsing worked
	if result["client"] != "192.168.1.1" {
		t.Errorf("Expected client=192.168.1.1, got %v", result["client"])
	}
	if result["method"] != "GET" {
		t.Errorf("Expected method=GET, got %v", result["method"])
	}
	if result["request"] != "/api/users" {
		t.Errorf("Expected request=/api/users, got %v", result["request"])
	}
	if result["bytes"] != "200" {
		t.Errorf("Expected bytes=200, got %v", result["bytes"])
	}
	if result["duration"] != "150" {
		t.Errorf("Expected duration=150, got %v", result["duration"])
	}

	// Check that original fields are preserved
	if result["timestamp"] != "2024-01-01T12:00:00Z" {
		t.Errorf("Expected timestamp to be preserved, got %v", result["timestamp"])
	}
}

func TestGrokWithoutConfig(t *testing.T) {
	// Test configuration without grok
	config := `
type: kafka
kafka:
  brokers:
    - "localhost:9092"
  group: "test-group"
  topic: "test-topic"
`

	// Create input without grok configuration
	input, err := NewInput("", config, "test-input")
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Test data
	testData := map[string]interface{}{
		"message":   "192.168.1.1 GET /api/users 200 150",
		"timestamp": "2024-01-01T12:00:00Z",
	}

	// Parse with grok (should return original data unchanged)
	result := input.parseWithGrok(testData)

	// Check that data is unchanged
	if result["message"] != "192.168.1.1 GET /api/users 200 150" {
		t.Errorf("Expected message to be unchanged, got %v", result["message"])
	}
	if result["timestamp"] != "2024-01-01T12:00:00Z" {
		t.Errorf("Expected timestamp to be unchanged, got %v", result["timestamp"])
	}
}

func TestGrokWithDifferentMessageFields(t *testing.T) {
	// Test configuration with grok pattern
	config := `
type: kafka
kafka:
  brokers:
    - "localhost:9092"
  group: "test-group"
  topic: "test-topic"
grok_pattern: "%{IP:client} %{WORD:method} %{URIPATHPARAM:request}"
`

	// Create input with grok configuration
	input, err := NewInput("", config, "test-input")
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Test with "msg" field instead of "message"
	testData := map[string]interface{}{
		"msg":       "192.168.1.1 POST /api/login",
		"timestamp": "2024-01-01T12:00:00Z",
	}

	// Parse with grok
	result := input.parseWithGrok(testData)

	// Check if grok parsing worked
	if result["client"] != "192.168.1.1" {
		t.Errorf("Expected client=192.168.1.1, got %v", result["client"])
	}
	if result["method"] != "POST" {
		t.Errorf("Expected method=POST, got %v", result["method"])
	}
	if result["request"] != "/api/login" {
		t.Errorf("Expected request=/api/login, got %v", result["request"])
	}
}

func TestGrokWithDirectRegex(t *testing.T) {
	// Test configuration with direct regex pattern
	config := `
type: kafka
kafka:
  brokers:
    - "localhost:9092"
  group: "test-group"
  topic: "test-topic"
grok_pattern: "(?<timestamp>\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z) (?<client_ip>\\d+\\.\\d+\\.\\d+\\.\\d+) (?<method>GET|POST|PUT|DELETE) (?<path>/[a-zA-Z0-9/_-]*)"
`

	// Create input with direct regex pattern
	input, err := NewInput("", config, "test-input")
	if err != nil {
		t.Fatalf("Failed to create input: %v", err)
	}

	// Test data with custom format
	testData := map[string]interface{}{
		"message":     "2024-01-01T12:00:00Z 192.168.1.100 POST /api/users",
		"extra_field": "should_be_preserved",
	}

	// Parse with grok
	result := input.parseWithGrok(testData)

	// Check if direct regex parsing worked
	if result["timestamp"] != "2024-01-01T12:00:00Z" {
		t.Errorf("Expected timestamp=2024-01-01T12:00:00Z, got %v", result["timestamp"])
	}
	if result["client_ip"] != "192.168.1.100" {
		t.Errorf("Expected client_ip=192.168.1.100, got %v", result["client_ip"])
	}
	if result["method"] != "POST" {
		t.Errorf("Expected method=POST, got %v", result["method"])
	}
	if result["path"] != "/api/users" {
		t.Errorf("Expected path=/api/users, got %v", result["path"])
	}

	// Check that original fields are preserved
	if result["extra_field"] != "should_be_preserved" {
		t.Errorf("Expected extra_field to be preserved, got %v", result["extra_field"])
	}
}
