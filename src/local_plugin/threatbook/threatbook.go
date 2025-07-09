package threatbook

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// ThreatBookResponse represents the structure of ThreatBook API response
type ThreatBookResponse struct {
	ResponseCode int    `json:"response_code"`
	VerboseMsg   string `json:"verbose_msg"`
	Data         struct {
		// IP查询相关字段
		IP       string `json:"ip,omitempty"`
		Location struct {
			Country     string  `json:"country"`
			Province    string  `json:"province"`
			City        string  `json:"city"`
			Longitude   float64 `json:"lng"`
			Latitude    float64 `json:"lat"`
			CountryCode string  `json:"country_code"`
		} `json:"location,omitempty"`
		ASN struct {
			Number int    `json:"number"`
			Info   string `json:"info"`
			Rank   string `json:"rank"`
		} `json:"asn,omitempty"`

		// 域名查询相关字段
		Domain string `json:"domain,omitempty"`

		// 文件哈希查询相关字段
		MD5    string `json:"md5,omitempty"`
		SHA1   string `json:"sha1,omitempty"`
		SHA256 string `json:"sha256,omitempty"`

		// 通用威胁情报字段
		Judgments   []string               `json:"judgments,omitempty"`
		ThreatTypes []string               `json:"threat_types,omitempty"`
		Confidence  string                 `json:"confidence,omitempty"`
		Severity    string                 `json:"severity,omitempty"`
		Tags        []string               `json:"tags_classes,omitempty"`
		UpdateTime  string                 `json:"update_time,omitempty"`
		IntelTypes  []string               `json:"intel_types,omitempty"`
		Summary     map[string]interface{} `json:"summary,omitempty"`

		// 详细信息
		IntelligenceInfo map[string]interface{} `json:"intelligence,omitempty"`
		Context          map[string]interface{} `json:"context,omitempty"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

// ThreatBookResult represents the processed result for AgentSmith-HUB
type ThreatBookResult struct {
	QueryValue   string `json:"query_value"`
	QueryType    string `json:"query_type"`
	ResponseCode int    `json:"response_code"`

	// 威胁情报核心信息
	IsMalicious bool     `json:"is_malicious"`
	ThreatTypes []string `json:"threat_types,omitempty"`
	Confidence  string   `json:"confidence,omitempty"`
	Severity    string   `json:"severity,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	IntelTypes  []string `json:"intel_types,omitempty"`

	// 地理位置信息（IP查询）
	Location *LocationInfo `json:"location,omitempty"`
	ASN      *ASNInfo      `json:"asn,omitempty"`

	// 哈希信息（文件查询）
	FileHashes *FileHashInfo `json:"file_hashes,omitempty"`

	// 其他信息
	UpdateTime   string                 `json:"update_time,omitempty"`
	Summary      map[string]interface{} `json:"summary,omitempty"`
	Intelligence map[string]interface{} `json:"intelligence,omitempty"`
	Context      map[string]interface{} `json:"context,omitempty"`

	Cached bool   `json:"cached"`
	Error  string `json:"error,omitempty"`
}

// LocationInfo represents geographical information
type LocationInfo struct {
	Country     string  `json:"country,omitempty"`
	Province    string  `json:"province,omitempty"`
	City        string  `json:"city,omitempty"`
	CountryCode string  `json:"country_code,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
	Latitude    float64 `json:"latitude,omitempty"`
}

// ASNInfo represents ASN information
type ASNInfo struct {
	Number int    `json:"number,omitempty"`
	Info   string `json:"info,omitempty"`
	Rank   string `json:"rank,omitempty"`
}

// FileHashInfo represents file hash information
type FileHashInfo struct {
	MD5    string `json:"md5,omitempty"`
	SHA1   string `json:"sha1,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

const (
	// Cache settings
	threatBookCachePrefix = "threatbook_cache:"
	threatBookCacheTTL    = 12 * time.Hour // Cache for 12 hours

	// API settings
	threatBookAPIBaseURL = "https://api.threatbook.cn/v3"
	threatBookAPITimeout = 30 * time.Second
)

// Query types
const (
	QueryTypeIP     = "ip"
	QueryTypeDomain = "domain"
	QueryTypeFile   = "file"
	QueryTypeURL    = "url"
)

// isValidQueryType checks if the query type is supported
func isValidQueryType(queryType string) bool {
	validTypes := []string{QueryTypeIP, QueryTypeDomain, QueryTypeFile, QueryTypeURL}
	for _, validType := range validTypes {
		if queryType == validType {
			return true
		}
	}
	return false
}

// isValidIP checks if the input is a valid IP address
func isValidIP(ip string) bool {
	return regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`).MatchString(ip) ||
		regexp.MustCompile(`^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$|^::1$|^::$`).MatchString(ip)
}

// isValidDomain checks if the input is a valid domain
func isValidDomain(domain string) bool {
	return regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`).MatchString(domain)
}

// isValidFileHash checks if the input is a valid file hash
func isValidFileHash(hash string) bool {
	hash = strings.ToLower(hash)
	// MD5: 32 hex characters
	if regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(hash) {
		return true
	}
	// SHA1: 40 hex characters
	if regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(hash) {
		return true
	}
	// SHA256: 64 hex characters
	if regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(hash) {
		return true
	}
	return false
}

// isValidURL checks if the input is a valid URL
func isValidURL(urlStr string) bool {
	_, err := url.Parse(urlStr)
	return err == nil && (strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://"))
}

// validateQueryValue validates the query value based on query type
func validateQueryValue(queryValue, queryType string) bool {
	switch queryType {
	case QueryTypeIP:
		return isValidIP(queryValue)
	case QueryTypeDomain:
		return isValidDomain(queryValue)
	case QueryTypeFile:
		return isValidFileHash(queryValue)
	case QueryTypeURL:
		return isValidURL(queryValue)
	default:
		return false
	}
}

// getCacheKey generates a cache key for the given query
func getCacheKey(queryValue, queryType string) string {
	return threatBookCachePrefix + queryType + ":" + strings.ToLower(queryValue)
}

// getCachedResult retrieves cached result from Redis
func getCachedResult(queryValue, queryType string) (*ThreatBookResult, bool) {
	cacheKey := getCacheKey(queryValue, queryType)

	cachedData, err := common.RedisGet(cacheKey)
	if err != nil {
		return nil, false
	}

	var result ThreatBookResult
	if err := json.Unmarshal([]byte(cachedData), &result); err != nil {
		return nil, false
	}

	result.Cached = true
	return &result, true
}

// setCachedResult stores result in Redis cache
func setCachedResult(queryValue, queryType string, result *ThreatBookResult) {
	cacheKey := getCacheKey(queryValue, queryType)

	// Mark as cached before storing
	result.Cached = true

	jsonData, err := json.Marshal(result)
	if err != nil {
		logger.Error("Failed to marshal ThreatBook result for cache", "error", err)
		return
	}

	// Set cache with TTL (convert to seconds)
	if _, err := common.RedisSet(cacheKey, string(jsonData), int(threatBookCacheTTL.Seconds())); err != nil {
		logger.Error("Failed to cache ThreatBook result", "error", err)
	}
}

// getThreatBookAPIKey gets the API key from environment variable
func getThreatBookAPIKey() string {
	apiKey := os.Getenv("THREATBOOK_API_KEY")
	if apiKey == "" {
		logger.Warn("THREATBOOK_API_KEY environment variable not set")
	}
	return apiKey
}

// buildAPIURL builds the appropriate API URL based on query type
func buildAPIURL(queryValue, queryType string) string {
	switch queryType {
	case QueryTypeIP:
		return fmt.Sprintf("%s/scene/ip_reputation", threatBookAPIBaseURL)
	case QueryTypeDomain:
		return fmt.Sprintf("%s/scene/domain_reputation", threatBookAPIBaseURL)
	case QueryTypeFile:
		return fmt.Sprintf("%s/scene/file_reputation", threatBookAPIBaseURL)
	case QueryTypeURL:
		return fmt.Sprintf("%s/scene/url_reputation", threatBookAPIBaseURL)
	default:
		return ""
	}
}

// queryThreatBookAPIWithKey queries the ThreatBook API
func queryThreatBookAPIWithKey(queryValue, queryType, apiKey string) (*ThreatBookResult, error) {
	// Use provided API key, fallback to environment variable
	if apiKey == "" {
		apiKey = getThreatBookAPIKey()
	}

	if apiKey == "" {
		return &ThreatBookResult{
			QueryValue: queryValue,
			QueryType:  queryType,
			Error:      "ThreatBook API key not configured",
		}, nil
	}

	// Build API URL
	apiURL := buildAPIURL(queryValue, queryType)
	if apiURL == "" {
		return &ThreatBookResult{
			QueryValue: queryValue,
			QueryType:  queryType,
			Error:      "Unsupported query type",
		}, nil
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: threatBookAPITimeout,
	}

	// Prepare request data
	data := url.Values{}
	data.Set("apikey", apiKey)
	data.Set("resource", queryValue)

	// Create request
	req, err := http.NewRequest("POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "AgentSmith-HUB/1.0")

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query ThreatBook API: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle HTTP errors
	if resp.StatusCode != 200 {
		return &ThreatBookResult{
			QueryValue: queryValue,
			QueryType:  queryType,
			Error:      fmt.Sprintf("ThreatBook API error: HTTP %d", resp.StatusCode),
		}, nil
	}

	// Parse JSON response
	var tbResp ThreatBookResponse
	if err := json.Unmarshal(body, &tbResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Handle API errors
	if tbResp.ResponseCode != 0 {
		return &ThreatBookResult{
			QueryValue:   queryValue,
			QueryType:    queryType,
			ResponseCode: tbResp.ResponseCode,
			Error:        fmt.Sprintf("ThreatBook API error: %s", tbResp.VerboseMsg),
		}, nil
	}

	// Process successful response
	result := &ThreatBookResult{
		QueryValue:   queryValue,
		QueryType:    queryType,
		ResponseCode: tbResp.ResponseCode,
		ThreatTypes:  tbResp.Data.ThreatTypes,
		Confidence:   tbResp.Data.Confidence,
		Severity:     tbResp.Data.Severity,
		Tags:         tbResp.Data.Tags,
		IntelTypes:   tbResp.Data.IntelTypes,
		UpdateTime:   tbResp.Data.UpdateTime,
		Summary:      tbResp.Data.Summary,
		Intelligence: tbResp.Data.IntelligenceInfo,
		Context:      tbResp.Data.Context,
		Cached:       false,
	}

	// Determine if malicious based on judgments and threat types
	result.IsMalicious = len(tbResp.Data.Judgments) > 0 || len(tbResp.Data.ThreatTypes) > 0

	// Add type-specific information
	switch queryType {
	case QueryTypeIP:
		if tbResp.Data.Location.Country != "" {
			result.Location = &LocationInfo{
				Country:     tbResp.Data.Location.Country,
				Province:    tbResp.Data.Location.Province,
				City:        tbResp.Data.Location.City,
				CountryCode: tbResp.Data.Location.CountryCode,
				Longitude:   tbResp.Data.Location.Longitude,
				Latitude:    tbResp.Data.Location.Latitude,
			}
		}
		if tbResp.Data.ASN.Number != 0 {
			result.ASN = &ASNInfo{
				Number: tbResp.Data.ASN.Number,
				Info:   tbResp.Data.ASN.Info,
				Rank:   tbResp.Data.ASN.Rank,
			}
		}
	case QueryTypeFile:
		if tbResp.Data.MD5 != "" || tbResp.Data.SHA1 != "" || tbResp.Data.SHA256 != "" {
			result.FileHashes = &FileHashInfo{
				MD5:    tbResp.Data.MD5,
				SHA1:   tbResp.Data.SHA1,
				SHA256: tbResp.Data.SHA256,
			}
		}
	}

	return result, nil
}

// Eval performs ThreatBook threat intelligence lookup with caching
// Args: queryValue string, queryType string, apiKey string (optional)
// Returns: ThreatBookResult object with threat intelligence information
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, false, fmt.Errorf("threatBook requires 2-3 arguments: queryValue string, queryType string, apiKey string (optional)")
	}

	queryValue, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("first argument (queryValue) must be a string")
	}

	queryType, ok := args[1].(string)
	if !ok {
		return nil, false, fmt.Errorf("second argument (queryType) must be a string")
	}

	// Optional API key parameter
	var apiKey string
	if len(args) == 3 {
		apiKeyArg, ok := args[2].(string)
		if !ok {
			return nil, false, fmt.Errorf("third argument (apiKey) must be a string")
		}
		apiKey = strings.TrimSpace(apiKeyArg)
	}

	// Clean and validate inputs
	queryValue = strings.TrimSpace(queryValue)
	queryType = strings.TrimSpace(strings.ToLower(queryType))

	if queryValue == "" {
		return nil, false, fmt.Errorf("query value cannot be empty")
	}

	if !isValidQueryType(queryType) {
		return &ThreatBookResult{
			QueryValue: queryValue,
			QueryType:  queryType,
			Error:      fmt.Sprintf("Invalid query type. Supported types: %s", strings.Join([]string{QueryTypeIP, QueryTypeDomain, QueryTypeFile, QueryTypeURL}, ", ")),
		}, true, nil
	}

	if !validateQueryValue(queryValue, queryType) {
		return &ThreatBookResult{
			QueryValue: queryValue,
			QueryType:  queryType,
			Error:      fmt.Sprintf("Invalid %s format", queryType),
		}, true, nil
	}

	// Normalize query value
	if queryType == QueryTypeFile {
		queryValue = strings.ToLower(queryValue)
	}

	// Check cache first
	if cachedResult, found := getCachedResult(queryValue, queryType); found {
		return cachedResult, true, nil
	}

	// Query ThreatBook API
	result, err := queryThreatBookAPIWithKey(queryValue, queryType, apiKey)
	if err != nil {
		return &ThreatBookResult{
			QueryValue: queryValue,
			QueryType:  queryType,
			Error:      fmt.Sprintf("Query failed: %s", err.Error()),
		}, true, nil
	}

	// Cache successful results
	if result.Error == "" || strings.Contains(result.Error, "not found") {
		setCachedResult(queryValue, queryType, result)
	}

	return result, true, nil
}
