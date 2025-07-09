package threatbook

import (
	"AgentSmith-HUB/common"
	"os"
	"testing"
)

func TestIsValidQueryType(t *testing.T) {
	tests := []struct {
		queryType string
		expected  bool
	}{
		{"ip", true},
		{"domain", true},
		{"file", true},
		{"url", true},
		{"invalid", false},
		{"", false},
		{"IP", false}, // case sensitive
		{"DOMAIN", false},
	}

	for _, test := range tests {
		result := isValidQueryType(test.queryType)
		if result != test.expected {
			t.Errorf("isValidQueryType(%q) = %v, expected %v", test.queryType, result, test.expected)
		}
	}
}

func TestIsValidIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"192.168.1.1", true},
		{"8.8.8.8", true},
		{"255.255.255.255", true},
		{"0.0.0.0", true},
		{"2001:db8::1", true},
		{"::1", true},
		{"256.1.1.1", false},
		{"192.168.1", false},
		{"invalid", false},
		{"", false},
		{"example.com", false},
	}

	for _, test := range tests {
		result := isValidIP(test.ip)
		if result != test.expected {
			t.Errorf("isValidIP(%q) = %v, expected %v", test.ip, result, test.expected)
		}
	}
}

func TestIsValidDomain(t *testing.T) {
	tests := []struct {
		domain   string
		expected bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"test-domain.org", true},
		{"a.b.c.d.e", true},
		{"192.168.1.1", false},
		{"invalid..domain", false},
		{"-invalid.com", false},
		{"invalid-.com", false},
		{"", false},
		{"toolongdomainnamethatshouldnotbevalidbecauseitexceedsthelimitof63characterspersubdomain.com", false},
	}

	for _, test := range tests {
		result := isValidDomain(test.domain)
		if result != test.expected {
			t.Errorf("isValidDomain(%q) = %v, expected %v", test.domain, result, test.expected)
		}
	}
}

func TestIsValidFileHash(t *testing.T) {
	tests := []struct {
		hash     string
		expected bool
	}{
		{"d41d8cd98f00b204e9800998ecf8427e", true},                                 // MD5
		{"da39a3ee5e6b4b0d3255bfef95601890afd80709", true},                         // SHA1
		{"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", true}, // SHA256
		{"D41D8CD98F00B204E9800998ECF8427E", true},                                 // Uppercase MD5
		{"invalid", false},
		{"", false},
		{"d41d8cd98f00b204e9800998ecf8427g", false},  // Invalid character
		{"d41d8cd98f00b204e9800998ecf8427", false},   // Too short
		{"d41d8cd98f00b204e9800998ecf8427e1", false}, // Too long for MD5
	}

	for _, test := range tests {
		result := isValidFileHash(test.hash)
		if result != test.expected {
			t.Errorf("isValidFileHash(%q) = %v, expected %v", test.hash, result, test.expected)
		}
	}
}

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"https://sub.example.com/path", true},
		{"http://192.168.1.1:8080/path", true},
		{"ftp://example.com", false}, // Only http/https supported
		{"example.com", false},       // Missing protocol
		{"", false},
		{"invalid", false},
	}

	for _, test := range tests {
		result := isValidURL(test.url)
		if result != test.expected {
			t.Errorf("isValidURL(%q) = %v, expected %v", test.url, result, test.expected)
		}
	}
}

func TestValidateQueryValue(t *testing.T) {
	tests := []struct {
		queryValue string
		queryType  string
		expected   bool
	}{
		{"192.168.1.1", "ip", true},
		{"example.com", "domain", true},
		{"d41d8cd98f00b204e9800998ecf8427e", "file", true},
		{"https://example.com", "url", true},
		{"192.168.1.1", "domain", false}, // Wrong type
		{"example.com", "ip", false},     // Wrong type
		{"invalid", "ip", false},
		{"", "ip", false},
	}

	for _, test := range tests {
		result := validateQueryValue(test.queryValue, test.queryType)
		if result != test.expected {
			t.Errorf("validateQueryValue(%q, %q) = %v, expected %v", test.queryValue, test.queryType, result, test.expected)
		}
	}
}

func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		queryValue string
		queryType  string
		expected   string
	}{
		{"192.168.1.1", "ip", "threatbook_cache:ip:192.168.1.1"},
		{"Example.com", "domain", "threatbook_cache:domain:example.com"},
		{"D41D8CD98F00B204E9800998ECF8427E", "file", "threatbook_cache:file:d41d8cd98f00b204e9800998ecf8427e"},
		{"https://Example.com", "url", "threatbook_cache:url:https://example.com"},
	}

	for _, test := range tests {
		result := getCacheKey(test.queryValue, test.queryType)
		if result != test.expected {
			t.Errorf("getCacheKey(%q, %q) = %q, expected %q", test.queryValue, test.queryType, result, test.expected)
		}
	}
}

func TestBuildAPIURL(t *testing.T) {
	tests := []struct {
		queryValue string
		queryType  string
		expected   string
	}{
		{"192.168.1.1", "ip", "https://api.threatbook.cn/v3/scene/ip_reputation"},
		{"example.com", "domain", "https://api.threatbook.cn/v3/scene/domain_reputation"},
		{"hash", "file", "https://api.threatbook.cn/v3/scene/file_reputation"},
		{"https://example.com", "url", "https://api.threatbook.cn/v3/scene/url_reputation"},
		{"anything", "invalid", ""},
	}

	for _, test := range tests {
		result := buildAPIURL(test.queryValue, test.queryType)
		if result != test.expected {
			t.Errorf("buildAPIURL(%q, %q) = %q, expected %q", test.queryValue, test.queryType, result, test.expected)
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
		{"One argument", []interface{}{"192.168.1.1"}, true},
		{"Too many arguments", []interface{}{"192.168.1.1", "ip", "key", "extra"}, true},
		{"Non-string query value", []interface{}{123, "ip"}, true},
		{"Non-string query type", []interface{}{"192.168.1.1", 123}, true},
		{"Non-string API key", []interface{}{"192.168.1.1", "ip", 123}, true},
		{"Empty query value", []interface{}{"", "ip"}, true},
		{"Invalid query type", []interface{}{"192.168.1.1", "invalid"}, false}, // Should return result with error
		{"Invalid query value", []interface{}{"invalid", "ip"}, false},         // Should return result with error
		{"Valid input", []interface{}{"192.168.1.1", "ip"}, false},
		{"Valid input with API key", []interface{}{"192.168.1.1", "ip", "test_key"}, false},
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

				// For invalid inputs, check that result contains error
				if test.name == "Invalid query type" || test.name == "Invalid query value" {
					tbResult, ok := result.(*ThreatBookResult)
					if !ok {
						t.Errorf("Expected ThreatBookResult but got %T", result)
					} else if tbResult.Error == "" {
						t.Errorf("Expected error in result but got none")
					}
				}
			}
		})
	}
}

func TestEvalWithAPIKeyParameter(t *testing.T) {
	// Test with API key as parameter
	result, success, err := Eval("192.168.1.1", "ip", "test_api_key")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !success {
		t.Errorf("Expected success=true but got false")
	}

	tbResult, ok := result.(*ThreatBookResult)
	if !ok {
		t.Errorf("Expected ThreatBookResult but got %T", result)
		return
	}

	// Should get some kind of result (even if API call fails due to invalid key)
	if tbResult.QueryValue != "192.168.1.1" {
		t.Errorf("Expected query value to be preserved in result")
	}

	if tbResult.QueryType != "ip" {
		t.Errorf("Expected query type to be preserved in result")
	}
}

func TestEvalWithoutAPIKey(t *testing.T) {
	// Save original API key
	originalAPIKey := os.Getenv("THREATBOOK_API_KEY")

	// Remove API key for test
	os.Unsetenv("THREATBOOK_API_KEY")

	// Test with valid input but no API key
	result, success, err := Eval("example.com", "domain")

	// Restore original API key
	if originalAPIKey != "" {
		os.Setenv("THREATBOOK_API_KEY", originalAPIKey)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !success {
		t.Errorf("Expected success=true but got false")
	}

	tbResult, ok := result.(*ThreatBookResult)
	if !ok {
		t.Errorf("Expected ThreatBookResult but got %T", result)
		return
	}

	if tbResult.Error != "ThreatBook API key not configured" {
		t.Errorf("Expected API key error but got: %s", tbResult.Error)
	}
}

func TestEvalWithMockCache(t *testing.T) {
	// Skip if Redis is not available
	if err := common.RedisPing(); err != nil {
		t.Skip("Redis not available, skipping cache test")
	}

	queryValue := "example.com"
	queryType := "domain"

	// Clear any existing cache
	cacheKey := getCacheKey(queryValue, queryType)
	common.RedisDel(cacheKey)

	// Create a mock result
	mockResult := &ThreatBookResult{
		QueryValue:   queryValue,
		QueryType:    queryType,
		ResponseCode: 0,
		IsMalicious:  true,
		ThreatTypes:  []string{"malware"},
		Confidence:   "high",
		Cached:       false,
	}

	// Set cache
	setCachedResult(queryValue, queryType, mockResult)

	// Test retrieval
	result, success, err := Eval(queryValue, queryType)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !success {
		t.Errorf("Expected success=true but got false")
	}

	tbResult, ok := result.(*ThreatBookResult)
	if !ok {
		t.Errorf("Expected ThreatBookResult but got %T", result)
		return
	}

	if !tbResult.Cached {
		t.Errorf("Expected cached=true but got false")
	}

	if tbResult.QueryValue != queryValue {
		t.Errorf("Expected query value %s but got %s", queryValue, tbResult.QueryValue)
	}

	if tbResult.QueryType != queryType {
		t.Errorf("Expected query type %s but got %s", queryType, tbResult.QueryType)
	}

	if !tbResult.IsMalicious {
		t.Errorf("Expected is_malicious=true but got false")
	}

	// Clean up
	common.RedisDel(cacheKey)
}

func TestEvalDifferentQueryTypes(t *testing.T) {
	tests := []struct {
		queryValue string
		queryType  string
	}{
		{"8.8.8.8", "ip"},
		{"google.com", "domain"},
		{"d41d8cd98f00b204e9800998ecf8427e", "file"},
		{"https://google.com", "url"},
	}

	for _, test := range tests {
		t.Run(test.queryType, func(t *testing.T) {
			result, success, err := Eval(test.queryValue, test.queryType, "test_key")

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !success {
				t.Errorf("Expected success=true but got false")
			}

			tbResult, ok := result.(*ThreatBookResult)
			if !ok {
				t.Errorf("Expected ThreatBookResult but got %T", result)
				return
			}

			if tbResult.QueryValue != test.queryValue {
				t.Errorf("Expected query value %s but got %s", test.queryValue, tbResult.QueryValue)
			}

			if tbResult.QueryType != test.queryType {
				t.Errorf("Expected query type %s but got %s", test.queryType, tbResult.QueryType)
			}
		})
	}
}

func TestEvalCaseInsensitiveQueryType(t *testing.T) {
	tests := []struct {
		queryType string
		expected  string
	}{
		{"IP", "ip"},
		{"Domain", "domain"},
		{"FILE", "file"},
		{"URL", "url"},
		{"Ip", "ip"},
		{"DOMAIN", "domain"},
	}

	for _, test := range tests {
		result, success, err := Eval("test", test.queryType, "test_key")

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if !success {
			t.Errorf("Expected success=true but got false")
		}

		tbResult, ok := result.(*ThreatBookResult)
		if !ok {
			t.Errorf("Expected ThreatBookResult but got %T", result)
			return
		}

		if tbResult.QueryType != test.expected {
			t.Errorf("Expected query type %s but got %s", test.expected, tbResult.QueryType)
		}
	}
}
