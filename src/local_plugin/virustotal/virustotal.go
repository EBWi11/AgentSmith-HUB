package virustotal

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// VirusTotalResponse represents the structure of VirusTotal API response
type VirusTotalResponse struct {
	Data struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			MD5          string `json:"md5"`
			SHA1         string `json:"sha1"`
			SHA256       string `json:"sha256"`
			LastAnalysis struct {
				Results map[string]struct {
					Category   string `json:"category"`
					Result     string `json:"result"`
					Method     string `json:"method"`
					EngineName string `json:"engine_name"`
				} `json:"results"`
				Stats struct {
					Harmless   int `json:"harmless"`
					Malicious  int `json:"malicious"`
					Suspicious int `json:"suspicious"`
					Undetected int `json:"undetected"`
					Timeout    int `json:"timeout"`
				} `json:"stats"`
			} `json:"last_analysis_stats"`
			Names     []string `json:"names"`
			Size      int64    `json:"size"`
			TypeTag   string   `json:"type_tag"`
			FirstSeen int64    `json:"first_submission_date"`
			LastSeen  int64    `json:"last_submission_date"`
		} `json:"attributes"`
	} `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// VirusTotalResult represents the processed result for AgentSmith-HUB
type VirusTotalResult struct {
	Hash         string            `json:"hash"`
	MD5          string            `json:"md5,omitempty"`
	SHA1         string            `json:"sha1,omitempty"`
	SHA256       string            `json:"sha256,omitempty"`
	Detections   int               `json:"detections"`
	TotalEngines int               `json:"total_engines"`
	Malicious    int               `json:"malicious"`
	Suspicious   int               `json:"suspicious"`
	Harmless     int               `json:"harmless"`
	Undetected   int               `json:"undetected"`
	Timeout      int               `json:"timeout"`
	Names        []string          `json:"names,omitempty"`
	Size         int64             `json:"size,omitempty"`
	TypeTag      string            `json:"type_tag,omitempty"`
	FirstSeen    string            `json:"first_seen,omitempty"`
	LastSeen     string            `json:"last_seen,omitempty"`
	Engines      map[string]string `json:"engines,omitempty"`
	Cached       bool              `json:"cached"`
	Error        string            `json:"error,omitempty"`
}

const (
	// Cache settings
	vtCachePrefix = "vt_cache:"
	vtCacheTTL    = 24 * time.Hour // Cache for 24 hours

	// API settings
	vtAPIBaseURL = "https://www.virustotal.com/api/v3"
	vtAPITimeout = 30 * time.Second
)

// isValidHash checks if the input is a valid hash (MD5, SHA1, or SHA256)
func isValidHash(hash string) bool {
	hash = strings.ToLower(hash)

	// MD5: 32 hex characters
	if matched, _ := regexp.MatchString(`^[a-f0-9]{32}$`, hash); matched {
		return true
	}

	// SHA1: 40 hex characters
	if matched, _ := regexp.MatchString(`^[a-f0-9]{40}$`, hash); matched {
		return true
	}

	// SHA256: 64 hex characters
	if matched, _ := regexp.MatchString(`^[a-f0-9]{64}$`, hash); matched {
		return true
	}

	return false
}

// getCacheKey generates a cache key for the given hash
func getCacheKey(hash string) string {
	return vtCachePrefix + strings.ToLower(hash)
}

// getCachedResult retrieves cached result from Redis
func getCachedResult(hash string) (*VirusTotalResult, bool) {
	cacheKey := getCacheKey(hash)

	cachedData, err := common.RedisGet(cacheKey)
	if err != nil {
		return nil, false
	}

	var result VirusTotalResult
	if err := json.Unmarshal([]byte(cachedData), &result); err != nil {
		return nil, false
	}

	result.Cached = true
	return &result, true
}

// setCachedResult stores result in Redis cache
func setCachedResult(hash string, result *VirusTotalResult) {
	cacheKey := getCacheKey(hash)

	// Mark as cached before storing
	result.Cached = true

	jsonData, err := json.Marshal(result)
	if err != nil {
		logger.Error("Failed to marshal VirusTotal result for cache", "error", err)
		return
	}

	// Set cache with TTL (convert to seconds)
	if _, err := common.RedisSet(cacheKey, string(jsonData), int(vtCacheTTL.Seconds())); err != nil {
		logger.Error("Failed to cache VirusTotal result", "error", err)
	}
}

// getVirusTotalAPIKey gets the API key from environment variable
func getVirusTotalAPIKey() string {
	apiKey := os.Getenv("VIRUSTOTAL_API_KEY")
	if apiKey == "" {
		logger.Warn("VIRUSTOTAL_API_KEY environment variable not set")
	}
	return apiKey
}

// queryVirusTotalAPIWithKey queries the VirusTotal API for file hash information
func queryVirusTotalAPIWithKey(hash string, apiKey string) (*VirusTotalResult, error) {
	// Use provided API key, fallback to environment variable
	if apiKey == "" {
		apiKey = getVirusTotalAPIKey()
	}

	if apiKey == "" {
		return &VirusTotalResult{
			Hash:  hash,
			Error: "VirusTotal API key not configured",
		}, nil
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: vtAPITimeout,
	}

	// Build API URL
	url := fmt.Sprintf("%s/files/%s", vtAPIBaseURL, hash)

	// Create request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("X-Apikey", apiKey)
	req.Header.Set("User-Agent", "AgentSmith-HUB/1.0")

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query VirusTotal API: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse JSON response
	var vtResp VirusTotalResponse
	if err := json.Unmarshal(body, &vtResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Handle API errors
	if vtResp.Error.Code != "" {
		if vtResp.Error.Code == "NotFoundError" {
			return &VirusTotalResult{
				Hash:  hash,
				Error: "Hash not found in VirusTotal database",
			}, nil
		}
		return &VirusTotalResult{
			Hash:  hash,
			Error: fmt.Sprintf("VirusTotal API error: %s", vtResp.Error.Message),
		}, nil
	}

	// Process successful response
	result := &VirusTotalResult{
		Hash:       hash,
		MD5:        vtResp.Data.Attributes.MD5,
		SHA1:       vtResp.Data.Attributes.SHA1,
		SHA256:     vtResp.Data.Attributes.SHA256,
		Malicious:  vtResp.Data.Attributes.LastAnalysis.Stats.Malicious,
		Suspicious: vtResp.Data.Attributes.LastAnalysis.Stats.Suspicious,
		Harmless:   vtResp.Data.Attributes.LastAnalysis.Stats.Harmless,
		Undetected: vtResp.Data.Attributes.LastAnalysis.Stats.Undetected,
		Timeout:    vtResp.Data.Attributes.LastAnalysis.Stats.Timeout,
		Names:      vtResp.Data.Attributes.Names,
		Size:       vtResp.Data.Attributes.Size,
		TypeTag:    vtResp.Data.Attributes.TypeTag,
		Cached:     false,
	}

	// Calculate totals
	result.TotalEngines = result.Malicious + result.Suspicious + result.Harmless + result.Undetected + result.Timeout
	result.Detections = result.Malicious + result.Suspicious

	// Format timestamps
	if vtResp.Data.Attributes.FirstSeen > 0 {
		result.FirstSeen = time.Unix(vtResp.Data.Attributes.FirstSeen, 0).UTC().Format(time.RFC3339)
	}
	if vtResp.Data.Attributes.LastSeen > 0 {
		result.LastSeen = time.Unix(vtResp.Data.Attributes.LastSeen, 0).UTC().Format(time.RFC3339)
	}

	// Extract engine results (limit to positive detections to avoid too much data)
	engines := make(map[string]string)
	for engineName, engineResult := range vtResp.Data.Attributes.LastAnalysis.Results {
		if engineResult.Category == "malicious" || engineResult.Category == "suspicious" {
			engines[engineName] = engineResult.Result
		}
	}
	if len(engines) > 0 {
		result.Engines = engines
	}

	return result, nil
}

// Eval performs VirusTotal hash lookup with caching
// Args: hash string, apiKey string (optional)
// Returns: VirusTotalResult object with detection information
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, false, fmt.Errorf("virusTotal requires 1-2 arguments: hash string, apiKey string (optional)")
	}

	hashStr, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("first argument (hash) must be a string")
	}

	// Optional API key parameter
	var apiKey string
	if len(args) == 2 {
		apiKeyArg, ok := args[1].(string)
		if !ok {
			return nil, false, fmt.Errorf("second argument (apiKey) must be a string")
		}
		apiKey = strings.TrimSpace(apiKeyArg)
	}

	// Clean and validate hash
	hashStr = strings.TrimSpace(hashStr)
	if hashStr == "" {
		return nil, false, fmt.Errorf("hash cannot be empty")
	}

	if !isValidHash(hashStr) {
		return &VirusTotalResult{
			Hash:  hashStr,
			Error: "Invalid hash format (must be MD5, SHA1, or SHA256)",
		}, true, nil
	}

	// Convert to lowercase for consistency
	hashStr = strings.ToLower(hashStr)

	// Check cache first
	if cachedResult, found := getCachedResult(hashStr); found {
		return cachedResult, true, nil
	}

	// Query VirusTotal API
	result, err := queryVirusTotalAPIWithKey(hashStr, apiKey)
	if err != nil {
		return &VirusTotalResult{
			Hash:  hashStr,
			Error: fmt.Sprintf("Query failed: %s", err.Error()),
		}, true, nil
	}

	// Cache successful results (even if hash not found)
	if result.Error == "" || result.Error == "Hash not found in VirusTotal database" {
		setCachedResult(hashStr, result)
	}

	return result, true, nil
}
