package shodan

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// ShodanResponse represents the structure of Shodan API response
type ShodanResponse struct {
	IP          string   `json:"ip"`
	Hostnames   []string `json:"hostnames"`
	City        string   `json:"city"`
	Region      string   `json:"region_code"`
	Country     string   `json:"country_name"`
	CountryCode string   `json:"country_code"`
	PostalCode  string   `json:"postal_code"`
	Latitude    float64  `json:"latitude"`
	Longitude   float64  `json:"longitude"`
	ISP         string   `json:"isp"`
	ASN         string   `json:"asn"`
	Org         string   `json:"org"`
	OS          string   `json:"os"`
	Tags        []string `json:"tags"`
	Vulns       []string `json:"vulns"`
	LastUpdate  string   `json:"last_update"`
	Ports       []int    `json:"ports"`
	Data        []struct {
		Port      int                    `json:"port"`
		Transport string                 `json:"transport"`
		Protocol  string                 `json:"protocol"`
		Product   string                 `json:"product"`
		Version   string                 `json:"version"`
		Title     string                 `json:"title"`
		HTML      string                 `json:"html"`
		Banner    string                 `json:"data"`
		Timestamp string                 `json:"timestamp"`
		SSL       map[string]interface{} `json:"ssl,omitempty"`
		HTTP      map[string]interface{} `json:"http,omitempty"`
		CPE       []string               `json:"cpe"`
		Opts      map[string]interface{} `json:"opts,omitempty"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

// ShodanResult represents the processed result for AgentSmith-HUB
type ShodanResult struct {
	IP         string        `json:"ip"`
	Hostnames  []string      `json:"hostnames,omitempty"`
	Location   *LocationInfo `json:"location,omitempty"`
	ISP        string        `json:"isp,omitempty"`
	ASN        string        `json:"asn,omitempty"`
	Org        string        `json:"org,omitempty"`
	OS         string        `json:"os,omitempty"`
	Tags       []string      `json:"tags,omitempty"`
	Vulns      []string      `json:"vulns,omitempty"`
	LastUpdate string        `json:"last_update,omitempty"`
	Ports      []int         `json:"ports,omitempty"`
	Services   []ServiceInfo `json:"services,omitempty"`
	TotalPorts int           `json:"total_ports"`
	HasVulns   bool          `json:"has_vulns"`
	Cached     bool          `json:"cached"`
	Error      string        `json:"error,omitempty"`
}

// LocationInfo represents geographical information
type LocationInfo struct {
	City        string  `json:"city,omitempty"`
	Region      string  `json:"region,omitempty"`
	Country     string  `json:"country,omitempty"`
	CountryCode string  `json:"country_code,omitempty"`
	PostalCode  string  `json:"postal_code,omitempty"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
}

// ServiceInfo represents service information on a port
type ServiceInfo struct {
	Port      int      `json:"port"`
	Transport string   `json:"transport,omitempty"`
	Protocol  string   `json:"protocol,omitempty"`
	Product   string   `json:"product,omitempty"`
	Version   string   `json:"version,omitempty"`
	Title     string   `json:"title,omitempty"`
	Banner    string   `json:"banner,omitempty"`
	Timestamp string   `json:"timestamp,omitempty"`
	CPE       []string `json:"cpe,omitempty"`
	HasSSL    bool     `json:"has_ssl"`
	HasHTTP   bool     `json:"has_http"`
}

const (
	// Cache settings
	shodanCachePrefix = "shodan_cache:"
	shodanCacheTTL    = 6 * time.Hour // Cache for 6 hours (Shodan data changes less frequently)

	// API settings
	shodanAPIBaseURL = "https://api.shodan.io/shodan/host"
	shodanAPITimeout = 30 * time.Second
)

// isValidIP checks if the input is a valid IP address
func isValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// getCacheKey generates a cache key for the given IP
func getCacheKey(ip string) string {
	return shodanCachePrefix + ip
}

// getCachedResult retrieves cached result from Redis
func getCachedResult(ip string) (*ShodanResult, bool) {
	cacheKey := getCacheKey(ip)

	cachedData, err := common.RedisGet(cacheKey)
	if err != nil {
		return nil, false
	}

	var result ShodanResult
	if err := json.Unmarshal([]byte(cachedData), &result); err != nil {
		return nil, false
	}

	result.Cached = true
	return &result, true
}

// setCachedResult stores result in Redis cache
func setCachedResult(ip string, result *ShodanResult) {
	cacheKey := getCacheKey(ip)

	// Mark as cached before storing
	result.Cached = true

	jsonData, err := json.Marshal(result)
	if err != nil {
		logger.Error("Failed to marshal Shodan result for cache", "error", err)
		return
	}

	// Set cache with TTL (convert to seconds)
	if _, err := common.RedisSet(cacheKey, string(jsonData), int(shodanCacheTTL.Seconds())); err != nil {
		logger.Error("Failed to cache Shodan result", "error", err)
	}
}

// getShodanAPIKey gets the API key from environment variable
func getShodanAPIKey() string {
	apiKey := os.Getenv("SHODAN_API_KEY")
	if apiKey == "" {
		logger.Warn("SHODAN_API_KEY environment variable not set")
	}
	return apiKey
}

// queryShodanAPIWithKey queries the Shodan API for IP information
func queryShodanAPIWithKey(ip string, apiKey string) (*ShodanResult, error) {
	// Use provided API key, fallback to environment variable
	if apiKey == "" {
		apiKey = getShodanAPIKey()
	}

	if apiKey == "" {
		return &ShodanResult{
			IP:    ip,
			Error: "Shodan API key not configured",
		}, nil
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: shodanAPITimeout,
	}

	// Build API URL
	url := fmt.Sprintf("%s/%s?key=%s", shodanAPIBaseURL, ip, apiKey)

	// Create request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("User-Agent", "AgentSmith-HUB/1.0")

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query Shodan API: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle HTTP errors
	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			return &ShodanResult{
				IP:    ip,
				Error: "IP not found in Shodan database",
			}, nil
		}
		if resp.StatusCode == 401 {
			return &ShodanResult{
				IP:    ip,
				Error: "Invalid Shodan API key",
			}, nil
		}
		return &ShodanResult{
			IP:    ip,
			Error: fmt.Sprintf("Shodan API error: HTTP %d", resp.StatusCode),
		}, nil
	}

	// Parse JSON response
	var shodanResp ShodanResponse
	if err := json.Unmarshal(body, &shodanResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Handle API errors in response
	if shodanResp.Error != "" {
		return &ShodanResult{
			IP:    ip,
			Error: fmt.Sprintf("Shodan API error: %s", shodanResp.Error),
		}, nil
	}

	// Process successful response
	result := &ShodanResult{
		IP:         ip,
		Hostnames:  shodanResp.Hostnames,
		ISP:        shodanResp.ISP,
		ASN:        shodanResp.ASN,
		Org:        shodanResp.Org,
		OS:         shodanResp.OS,
		Tags:       shodanResp.Tags,
		Vulns:      shodanResp.Vulns,
		LastUpdate: shodanResp.LastUpdate,
		Ports:      shodanResp.Ports,
		TotalPorts: len(shodanResp.Ports),
		HasVulns:   len(shodanResp.Vulns) > 0,
		Cached:     false,
	}

	// Add location information
	if shodanResp.City != "" || shodanResp.Country != "" {
		result.Location = &LocationInfo{
			City:        shodanResp.City,
			Region:      shodanResp.Region,
			Country:     shodanResp.Country,
			CountryCode: shodanResp.CountryCode,
			PostalCode:  shodanResp.PostalCode,
			Latitude:    shodanResp.Latitude,
			Longitude:   shodanResp.Longitude,
		}
	}

	// Process service data
	services := make([]ServiceInfo, 0, len(shodanResp.Data))
	for _, data := range shodanResp.Data {
		service := ServiceInfo{
			Port:      data.Port,
			Transport: data.Transport,
			Protocol:  data.Protocol,
			Product:   data.Product,
			Version:   data.Version,
			Title:     data.Title,
			Banner:    truncateString(data.Banner, 500), // Limit banner size
			Timestamp: data.Timestamp,
			CPE:       data.CPE,
			HasSSL:    data.SSL != nil,
			HasHTTP:   data.HTTP != nil,
		}
		services = append(services, service)
	}
	result.Services = services

	return result, nil
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Eval performs Shodan IP lookup with caching
// Args: ip string, apiKey string (optional)
// Returns: ShodanResult object with infrastructure information
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, false, fmt.Errorf("shodan requires 1-2 arguments: ip string, apiKey string (optional)")
	}

	ipStr, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("first argument (ip) must be a string")
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

	// Clean and validate IP
	ipStr = strings.TrimSpace(ipStr)
	if ipStr == "" {
		return nil, false, fmt.Errorf("IP address cannot be empty")
	}

	if !isValidIP(ipStr) {
		return &ShodanResult{
			IP:    ipStr,
			Error: "Invalid IP address format",
		}, true, nil
	}

	// Check cache first
	if cachedResult, found := getCachedResult(ipStr); found {
		return cachedResult, true, nil
	}

	// Query Shodan API
	result, err := queryShodanAPIWithKey(ipStr, apiKey)
	if err != nil {
		return &ShodanResult{
			IP:    ipStr,
			Error: fmt.Sprintf("Query failed: %s", err.Error()),
		}, true, nil
	}

	// Cache successful results (even if IP not found)
	if result.Error == "" || result.Error == "IP not found in Shodan database" {
		setCachedResult(ipStr, result)
	}

	return result, true, nil
}
