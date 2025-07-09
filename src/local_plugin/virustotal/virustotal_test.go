package virustotal

import (
	"AgentSmith-HUB/common"
	"os"
	"testing"
)

func TestIsValidHash(t *testing.T) {
	tests := []struct {
		hash     string
		expected bool
	}{
		{"d41d8cd98f00b204e9800998ecf8427e", true},                                 // MD5
		{"da39a3ee5e6b4b0d3255bfef95601890afd80709", true},                         // SHA1
		{"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", true}, // SHA256
		{"invalid", false}, // Invalid
		{"", false},        // Empty
		{"d41d8cd98f00b204e9800998ecf8427g", false},                                // Invalid character
		{"d41d8cd98f00b204e9800998ecf8427", false},                                 // Too short
		{"d41d8cd98f00b204e9800998ecf8427e1", false},                               // Too long for MD5
		{"da39a3ee5e6b4b0d3255bfef95601890afd8070", false},                         // Too short for SHA1
		{"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b85", false}, // Too short for SHA256
		{"E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855", true}, // Uppercase SHA256
	}

	for _, test := range tests {
		result := isValidHash(test.hash)
		if result != test.expected {
			t.Errorf("isValidHash(%q) = %v, expected %v", test.hash, result, test.expected)
		}
	}
}

func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		hash     string
		expected string
	}{
		{"d41d8cd98f00b204e9800998ecf8427e", "vt_cache:d41d8cd98f00b204e9800998ecf8427e"},
		{"DA39A3EE5E6B4B0D3255BFEF95601890AFD80709", "vt_cache:da39a3ee5e6b4b0d3255bfef95601890afd80709"},
		{"", "vt_cache:"},
	}

	for _, test := range tests {
		result := getCacheKey(test.hash)
		if result != test.expected {
			t.Errorf("getCacheKey(%q) = %q, expected %q", test.hash, result, test.expected)
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
		{"Too many arguments", []interface{}{"hash1", "key1", "extra"}, true},
		{"Non-string hash argument", []interface{}{123}, true},
		{"Non-string apikey argument", []interface{}{"hash1", 123}, true},
		{"Empty string", []interface{}{""}, true},
		{"Invalid hash", []interface{}{"invalid"}, false}, // Should return result with error field
		{"Valid hash with API key", []interface{}{"d41d8cd98f00b204e9800998ecf8427e", "test_key"}, false},
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

				// For invalid hash, check that result contains error
				if test.name == "Invalid hash" {
					vtResult, ok := result.(*VirusTotalResult)
					if !ok {
						t.Errorf("Expected VirusTotalResult but got %T", result)
					} else if vtResult.Error == "" {
						t.Errorf("Expected error in result but got none")
					}
				}
			}
		})
	}
}

func TestEvalWithAPIKeyParameter(t *testing.T) {
	// Test with API key as parameter
	result, success, err := Eval("d41d8cd98f00b204e9800998ecf8427e", "test_api_key")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !success {
		t.Errorf("Expected success=true but got false")
	}

	vtResult, ok := result.(*VirusTotalResult)
	if !ok {
		t.Errorf("Expected VirusTotalResult but got %T", result)
		return
	}

	// Should get some kind of result (even if API call fails due to invalid key)
	if vtResult.Hash != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("Expected hash to be preserved in result")
	}
}

func TestEvalWithoutAPIKey(t *testing.T) {
	// Save original API key
	originalAPIKey := os.Getenv("VIRUSTOTAL_API_KEY")

	// Remove API key for test
	os.Unsetenv("VIRUSTOTAL_API_KEY")

	// Test with valid hash but no API key
	result, success, err := Eval("d41d8cd98f00b204e9800998ecf8427e")

	// Restore original API key
	if originalAPIKey != "" {
		os.Setenv("VIRUSTOTAL_API_KEY", originalAPIKey)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !success {
		t.Errorf("Expected success=true but got false")
	}

	vtResult, ok := result.(*VirusTotalResult)
	if !ok {
		t.Errorf("Expected VirusTotalResult but got %T", result)
		return
	}

	if vtResult.Error != "VirusTotal API key not configured" {
		t.Errorf("Expected API key error but got: %s", vtResult.Error)
	}
}

func TestEvalWithMockCache(t *testing.T) {
	// Skip if Redis is not available
	if err := common.RedisPing(); err != nil {
		t.Skip("Redis not available, skipping cache test")
	}

	hash := "d41d8cd98f00b204e9800998ecf8427e"

	// Clear any existing cache
	cacheKey := getCacheKey(hash)
	common.RedisDel(cacheKey)

	// Create a mock result
	mockResult := &VirusTotalResult{
		Hash:         hash,
		Detections:   1,
		TotalEngines: 10,
		Malicious:    1,
		Cached:       false,
	}

	// Set cache
	setCachedResult(hash, mockResult)

	// Test retrieval
	result, success, err := Eval(hash)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !success {
		t.Errorf("Expected success=true but got false")
	}

	vtResult, ok := result.(*VirusTotalResult)
	if !ok {
		t.Errorf("Expected VirusTotalResult but got %T", result)
		return
	}

	if !vtResult.Cached {
		t.Errorf("Expected cached=true but got false")
	}

	if vtResult.Hash != hash {
		t.Errorf("Expected hash %s but got %s", hash, vtResult.Hash)
	}

	if vtResult.Detections != 1 {
		t.Errorf("Expected detections=1 but got %d", vtResult.Detections)
	}

	// Clean up
	common.RedisDel(cacheKey)
}
