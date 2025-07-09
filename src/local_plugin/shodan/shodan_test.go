package shodan

import (
	"AgentSmith-HUB/common"
	"os"
	"testing"
)

func TestIsValidIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"8.8.8.8", true},          // Valid IPv4
		{"192.168.1.1", true},      // Valid private IPv4
		{"2001:db8::1", true},      // Valid IPv6
		{"::1", true},              // Valid IPv6 loopback
		{"invalid", false},         // Invalid
		{"", false},                // Empty
		{"999.999.999.999", false}, // Invalid IPv4
		{"192.168.1", false},       // Incomplete IPv4
		{"192.168.1.1.1", false},   // Too many octets
		{"google.com", false},      // Domain name
	}

	for _, test := range tests {
		result := isValidIP(test.ip)
		if result != test.expected {
			t.Errorf("isValidIP(%q) = %v, expected %v", test.ip, result, test.expected)
		}
	}
}

func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		ip       string
		expected string
	}{
		{"8.8.8.8", "shodan_cache:8.8.8.8"},
		{"192.168.1.1", "shodan_cache:192.168.1.1"},
		{"2001:db8::1", "shodan_cache:2001:db8::1"},
		{"", "shodan_cache:"},
	}

	for _, test := range tests {
		result := getCacheKey(test.ip)
		if result != test.expected {
			t.Errorf("getCacheKey(%q) = %q, expected %q", test.ip, result, test.expected)
		}
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"this is a very long string", 10, "this is a ..."},
		{"exactly10", 9, "exactly10"},
		{"", 5, ""},
		{"test", 0, "..."},
	}

	for _, test := range tests {
		result := truncateString(test.input, test.maxLen)
		if result != test.expected {
			t.Errorf("truncateString(%q, %d) = %q, expected %q", test.input, test.maxLen, result, test.expected)
		}
	}
}

func TestEvalInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		args     []interface{}
		hasError bool
	}{
		{"No arguments", []interface{}{}, true},
		{"Too many arguments", []interface{}{"8.8.8.8", "key1", "extra"}, true},
		{"Non-string IP argument", []interface{}{123}, true},
		{"Non-string apikey argument", []interface{}{"8.8.8.8", 123}, true},
		{"Empty string", []interface{}{""}, true},
		{"Invalid IP", []interface{}{"invalid"}, false}, // Should return result with error field
		{"Valid IP with API key", []interface{}{"8.8.8.8", "test_key"}, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, success, err := Eval(test.args...)

			if test.hasError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				if success {
					t.Errorf("Expected success=false but got true")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if !success {
					t.Errorf("Expected success=true but got false")
				}

				// For invalid IP, check that result contains error
				if test.name == "Invalid IP" {
					shodanResult, ok := result.(*ShodanResult)
					if !ok {
						t.Errorf("Expected ShodanResult but got %T", result)
					} else if shodanResult.Error == "" {
						t.Errorf("Expected error in result but got none")
					}
				}
			}
		})
	}
}

func TestEvalWithAPIKeyParameter(t *testing.T) {
	// Test with API key as parameter
	result, success, err := Eval("8.8.8.8", "test_api_key")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !success {
		t.Errorf("Expected success=true but got false")
	}

	shodanResult, ok := result.(*ShodanResult)
	if !ok {
		t.Errorf("Expected ShodanResult but got %T", result)
		return
	}

	// Should get some kind of result (even if API call fails due to invalid key)
	if shodanResult.IP != "8.8.8.8" {
		t.Errorf("Expected IP to be preserved in result")
	}
}

func TestEvalWithoutAPIKey(t *testing.T) {
	// Save original API key
	originalAPIKey := os.Getenv("SHODAN_API_KEY")

	// Remove API key for test
	os.Unsetenv("SHODAN_API_KEY")

	// Test with valid IP but no API key
	result, success, err := Eval("8.8.8.8")

	// Restore original API key
	if originalAPIKey != "" {
		os.Setenv("SHODAN_API_KEY", originalAPIKey)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !success {
		t.Errorf("Expected success=true but got false")
	}

	shodanResult, ok := result.(*ShodanResult)
	if !ok {
		t.Errorf("Expected ShodanResult but got %T", result)
		return
	}

	if shodanResult.Error != "Shodan API key not configured" {
		t.Errorf("Expected API key error but got: %s", shodanResult.Error)
	}
}

func TestEvalWithMockCache(t *testing.T) {
	// Skip if Redis is not available
	if err := common.RedisPing(); err != nil {
		t.Skip("Redis not available, skipping cache test")
	}

	ip := "8.8.8.8"

	// Clear any existing cache
	cacheKey := getCacheKey(ip)
	common.RedisDel(cacheKey)

	// Create a mock result
	mockResult := &ShodanResult{
		IP:         ip,
		TotalPorts: 3,
		Ports:      []int{53, 443, 853},
		ISP:        "Google LLC",
		Org:        "Google Public DNS",
		Location: &LocationInfo{
			Country:     "United States",
			CountryCode: "US",
		},
		Cached: false,
	}

	// Set cache
	setCachedResult(ip, mockResult)

	// Test retrieval
	result, success, err := Eval(ip)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !success {
		t.Errorf("Expected success=true but got false")
	}

	shodanResult, ok := result.(*ShodanResult)
	if !ok {
		t.Errorf("Expected ShodanResult but got %T", result)
		return
	}

	if !shodanResult.Cached {
		t.Errorf("Expected cached=true but got false")
	}

	if shodanResult.IP != ip {
		t.Errorf("Expected IP %s but got %s", ip, shodanResult.IP)
	}

	if shodanResult.TotalPorts != 3 {
		t.Errorf("Expected total_ports=3 but got %d", shodanResult.TotalPorts)
	}

	if shodanResult.ISP != "Google LLC" {
		t.Errorf("Expected ISP 'Google LLC' but got %s", shodanResult.ISP)
	}

	// Clean up
	common.RedisDel(cacheKey)
}

func TestEvalWithPrivateIP(t *testing.T) {
	// Test with private IP address
	result, success, err := Eval("192.168.1.1", "test_key")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !success {
		t.Errorf("Expected success=true but got false")
	}

	shodanResult, ok := result.(*ShodanResult)
	if !ok {
		t.Errorf("Expected ShodanResult but got %T", result)
		return
	}

	// Private IPs usually not found in Shodan
	if shodanResult.IP != "192.168.1.1" {
		t.Errorf("Expected IP to be preserved in result")
	}
}

func TestEvalWithIPv6(t *testing.T) {
	// Test with IPv6 address
	result, success, err := Eval("2001:4860:4860::8888", "test_key")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !success {
		t.Errorf("Expected success=true but got false")
	}

	shodanResult, ok := result.(*ShodanResult)
	if !ok {
		t.Errorf("Expected ShodanResult but got %T", result)
		return
	}

	if shodanResult.IP != "2001:4860:4860::8888" {
		t.Errorf("Expected IPv6 to be preserved in result")
	}
}
