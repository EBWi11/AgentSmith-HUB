package common

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"AgentSmith-HUB/logger"
	es7 "github.com/elastic/go-elasticsearch/v7"
	es8 "github.com/elastic/go-elasticsearch/v8"
	es9 "github.com/elastic/go-elasticsearch/v9"
)

// ElasticsearchAuthConfig represents authentication configuration for Elasticsearch
type ElasticsearchAuthConfig struct {
	Type     string `yaml:"type"`               // auth type: basic, api_key, bearer
	Username string `yaml:"username,omitempty"` // for basic auth
	Password string `yaml:"password,omitempty"` // for basic auth
	APIKey   string `yaml:"api_key,omitempty"`  // for api_key auth
	Token    string `yaml:"token,omitempty"`    // for bearer token auth
}

// ElasticsearchProducer wraps the Elasticsearch client with a channel-based interface
type ElasticsearchProducer struct {
	Version       string
	Client7       *es7.Client
	Client8       *es8.Client
	Client9       *es9.Client
	MsgChan       chan map[string]interface{}
	Index         string
	IndexTemplate string // Store the original index template for time pattern replacement
	batchSize     int
	flushDur      time.Duration
	maxRetries    int
	retryDelay    time.Duration
	stopChan      chan struct{} // Add stop channel for graceful shutdown
}

// replaceTimePatterns replaces time patterns in index name with actual values
func replaceTimePatterns(indexTemplate string) string {
	now := time.Now()

	// Replace various time patterns
	replacements := map[string]string{
		"{YYYY}":       now.Format("2006"),
		"{YY}":         now.Format("06"),
		"{MM}":         now.Format("01"),
		"{DD}":         now.Format("02"),
		"{HH}":         now.Format("15"),
		"{mm}":         now.Format("04"),
		"{ss}":         now.Format("05"),
		"{YYYY.MM.DD}": now.Format("2006.01.02"),
		"{YYYY-MM-DD}": now.Format("2006-01-02"),
		"{YYYY/MM/DD}": now.Format("2006/01/02"),
		"{YYYY_MM_DD}": now.Format("2006_01_02"),
		"{YYYY.MM}":    now.Format("2006.01"),
		"{YYYY-MM}":    now.Format("2006-01"),
		"{YYYY/MM}":    now.Format("2006/01"),
		"{YYYY_MM}":    now.Format("2006_01"),
	}

	result := indexTemplate
	for pattern, replacement := range replacements {
		result = strings.ReplaceAll(result, pattern, replacement)
	}

	return result
}

// normalizeElasticsearchVersion maps user-provided version string to internal version key.
// Supported values: "7"/"v7", "8"/"v8", otherwise default to "v8".
func normalizeElasticsearchVersion(version string) string {
	v := strings.ToLower(strings.TrimSpace(version))
	switch v {
	case "7", "v7", "es7", "elasticsearch7":
		return "v7"
	case "8", "v8", "es8", "elasticsearch8":
		return "v8"
	case "9", "v9", "es9", "elasticsearch9":
		return "v9"
	default:
		// Default to v8 for broader compatibility when detection is not possible
		return "v8"
	}
}

// NormalizeElasticsearchVersionForDisplay returns a user-friendly version string for UI / logs.
// It always returns one of: "v7", "v8", "v9".
func NormalizeElasticsearchVersionForDisplay(version string) string {
	return normalizeElasticsearchVersion(version)
}

// DetectElasticsearchVersion tries to determine ES major version from the cluster root response.
// It returns "v7", "v8", or "v9" on success.
func DetectElasticsearchVersion(hosts []string, auth *ElasticsearchAuthConfig) (string, error) {
	if len(hosts) == 0 {
		return "", fmt.Errorf("no Elasticsearch hosts configured for version detection")
	}

	baseURL := strings.TrimSpace(hosts[0])
	if baseURL == "" {
		return "", fmt.Errorf("empty Elasticsearch host for version detection")
	}

	// Ensure we hit the root path
	if !strings.HasSuffix(baseURL, "/") {
		baseURL = baseURL + "/"
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create version detection request: %w", err)
	}

	// Configure authentication headers if provided
	if auth != nil {
		switch auth.Type {
		case "basic":
			if auth.Username != "" && auth.Password != "" {
				req.SetBasicAuth(auth.Username, auth.Password)
			}
		case "api_key":
			if auth.APIKey != "" {
				req.Header.Set("Authorization", "ApiKey "+auth.APIKey)
			}
		case "bearer":
			if auth.Token != "" {
				req.Header.Set("Authorization", "Bearer "+auth.Token)
			}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to query Elasticsearch root for version detection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status code during version detection: %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("failed to decode version detection response: %w", err)
	}

	versionField, ok := body["version"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("version field not found in Elasticsearch root response")
	}

	number, ok := versionField["number"].(string)
	if !ok || number == "" {
		return "", fmt.Errorf("version.number field not found or empty in Elasticsearch root response")
	}

	major := strings.SplitN(number, ".", 2)[0]
	switch major {
	case "7":
		return "v7", nil
	case "8":
		return "v8", nil
	case "9":
		return "v9", nil
	default:
		return "", fmt.Errorf("unsupported Elasticsearch major version %s in %q", major, number)
	}
}

// detectOrDefaultElasticsearchVersion returns:
// - normalized explicit version if provided
// - otherwise, tries to detect from cluster root
// - on detection failure, falls back to v8
func detectOrDefaultElasticsearchVersion(hosts []string, explicit string, auth *ElasticsearchAuthConfig) string {
	if strings.TrimSpace(explicit) != "" {
		return normalizeElasticsearchVersion(explicit)
	}
	if v, err := DetectElasticsearchVersion(hosts, auth); err == nil {
		return v
	}
	// Fallback to v8 behavior if detection fails
	return "v8"
}

// NewElasticsearchProducer creates a new Elasticsearch producer
// version: "v7", "v8", or "v9" (empty => auto-detect, fallback v8)
func NewElasticsearchProducer(hosts []string, index string, version string, msgChan chan map[string]interface{}, batchSize int, flushDur time.Duration, auth *ElasticsearchAuthConfig) (*ElasticsearchProducer, error) {
	normVersion := detectOrDefaultElasticsearchVersion(hosts, version, auth)

	// Common TLS + retry config factory
	buildCommonTransport := func() *http.Transport {
		return &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Skip TLS certificate verification
			},
		}
	}

	var (
		client7 *es7.Client
		client8 *es8.Client
		client9 *es9.Client
		err     error
	)

	switch normVersion {
	case "v7":
		cfg := es7.Config{
			Addresses:     hosts,
			MaxRetries:    3,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client7, err = es7.NewClient(cfg)
	case "v8":
		cfg := es8.Config{
			Addresses:     hosts,
			MaxRetries:    3,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client8, err = es8.NewClient(cfg)
	default: // "v9"
		cfg := es9.Config{
			Addresses:     hosts,
			MaxRetries:    3,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client9, err = es9.NewClient(cfg)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create ES client for version %s: %v", normVersion, err)
	}

	// Replace time patterns in index name
	resolvedIndex := replaceTimePatterns(index)

	prod := &ElasticsearchProducer{
		Version:       normVersion,
		Client7:       client7,
		Client8:       client8,
		Client9:       client9,
		MsgChan:       msgChan,
		Index:         resolvedIndex,
		IndexTemplate: index, // Store original template for potential future use
		batchSize:     batchSize,
		flushDur:      flushDur,
		maxRetries:    3,
		retryDelay:    1 * time.Second,
		stopChan:      make(chan struct{}),
	}

	go prod.run()
	return prod, nil
}

func (p *ElasticsearchProducer) run() {
	batch := make([]map[string]interface{}, 0, p.batchSize)
	timer := time.NewTimer(p.flushDur)
	defer timer.Stop()

	for {
		select {
		case <-p.stopChan:
			// Stop timer to prevent any further timer events
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			// Flush any remaining batch before shutdown so that short-lived
			// test runs (e.g. /test-output) still send their data to ES.
			if len(batch) > 0 {
				p.flush(batch)
			}
			return
		case msg, ok := <-p.MsgChan:
			if !ok {
				// Channel is closed, check if we should still flush
				// First check if stop signal was received
				select {
				case <-p.stopChan:
					// Stop signal received, skip flushing and return immediately
					return
				default:
					// No stop signal, flush remaining batch
					if len(batch) > 0 {
						p.flush(batch)
					}
					return
				}
			}
			batch = append(batch, msg)
			if len(batch) >= p.batchSize {
				p.flush(batch)
				batch = batch[:0]
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(p.flushDur)
			}
		case <-timer.C:
			if len(batch) > 0 {
				p.flush(batch)
				batch = batch[:0]
			}
			timer.Reset(p.flushDur)
		}
	}
}

// sendBatch sends a batch of documents to Elasticsearch with retry logic
func (p *ElasticsearchProducer) sendBatch(batch []map[string]interface{}) {
	if len(batch) == 0 {
		return
	}

	var buf bytes.Buffer
	for _, doc := range batch {
		// Add index action
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": p.Index,
			},
		}
		if err := json.NewEncoder(&buf).Encode(meta); err != nil {
			logger.Error("Elasticsearch failed to encode bulk meta", "error", err)
			continue
		}
		// Add document
		if err := json.NewEncoder(&buf).Encode(doc); err != nil {
			logger.Error("Elasticsearch failed to encode document", "error", err)
			continue
		}
	}

	// Try to send with retries and timeout control
	for i := 0; i <= p.maxRetries; i++ {
		// Check if we should stop before each retry
		select {
		case <-p.stopChan:
			// Stop signal received, abort sending
			return
		default:
		}

		// Create context with shorter timeout for faster shutdown (reduced from 5s to 2s)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		// Use context for bulk request based on client version
		var (
			res interface {
				IsError() bool
				String() string
			}
			err error
		)

		switch p.Version {
		case "v7":
			if p.Client7 == nil {
				cancel()
				logger.Error("Elasticsearch v7 client is not initialized")
				return
			}
			r, errBulk := p.Client7.Bulk(bytes.NewReader(buf.Bytes()), p.Client7.Bulk.WithContext(ctx))
			err = errBulk
			res = r
		case "v8":
			if p.Client8 == nil {
				cancel()
				logger.Error("Elasticsearch v8 client is not initialized")
				return
			}
			r, errBulk := p.Client8.Bulk(bytes.NewReader(buf.Bytes()), p.Client8.Bulk.WithContext(ctx))
			err = errBulk
			res = r
		default: // "v9"
			if p.Client9 == nil {
				cancel()
				logger.Error("Elasticsearch v9 client is not initialized")
				return
			}
			r, errBulk := p.Client9.Bulk(bytes.NewReader(buf.Bytes()), p.Client9.Bulk.WithContext(ctx))
			err = errBulk
			res = r
		}

		cancel() // Always cancel context

		if err != nil {
			if i == p.maxRetries {
				logger.Error("Elasticsearch failed to send batch after retries", "retries", p.maxRetries, "error", err)
				return
			}
			// Check stop signal before retry delay
			select {
			case <-p.stopChan:
				return
			case <-time.After(p.retryDelay):
			}
			continue
		}
		if res != nil && res.IsError() {
			if i == p.maxRetries {
				logger.Error("Elasticsearch returned error after retries", "retries", p.maxRetries, "response", res.String())
				return
			}
			// Check stop signal before retry delay
			select {
			case <-p.stopChan:
				return
			case <-time.After(p.retryDelay):
			}
			continue
		}

		// Bulk returns HTTP 200 even when item-level errors occur; check body
		if res != nil {
			bodyStr := res.String()
			var bulkResp struct {
				Errors bool `json:"errors"`
			}
			if json.Unmarshal([]byte(bodyStr), &bulkResp) == nil && bulkResp.Errors {
				logger.Error("Elasticsearch bulk reported item errors (HTTP 200)", "response", bodyStr)
			}
		}
		// Success
		return
	}
}

// flush batch writes to ES
func (p *ElasticsearchProducer) flush(batch []map[string]interface{}) {
	p.sendBatch(batch)
}

// Close closes the producer
// Note: We don't close MsgChan here because it's owned by the caller
func (p *ElasticsearchProducer) Close() {
	// Signal the goroutine to stop
	if p.stopChan != nil {
		close(p.stopChan)
	}
	// The channel will be closed by the owner (output component)
	// We just need to ensure any pending operations are completed
}

// TestElasticsearchConnection tests the connection to Elasticsearch cluster
// This method creates a temporary client to test connectivity without affecting the main producer
// version: "v7", "v8", or "v9" (empty => auto-detect, fallback v8)
func TestElasticsearchConnection(hosts []string, version string, auth *ElasticsearchAuthConfig) error {
	normVersion := detectOrDefaultElasticsearchVersion(hosts, version, auth)

	buildCommonTransport := func() *http.Transport {
		return &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Skip TLS certificate verification
			},
		}
	}

	// Test connection by pinging the cluster
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch normVersion {
	case "v7":
		cfg := es7.Config{
			Addresses:     hosts,
			MaxRetries:    1,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client, err := es7.NewClient(cfg)
		if err != nil {
			return fmt.Errorf("failed to create test client (v7): %w", err)
		}
		res, err := client.Ping(client.Ping.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("failed to ping Elasticsearch cluster (v7): %w", err)
		}
		defer res.Body.Close()
		if res.IsError() {
			return fmt.Errorf("Elasticsearch cluster returned error (v7): %s", res.String())
		}
	case "v8":
		cfg := es8.Config{
			Addresses:     hosts,
			MaxRetries:    1,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client, err := es8.NewClient(cfg)
		if err != nil {
			return fmt.Errorf("failed to create test client (v8): %w", err)
		}
		res, err := client.Ping(client.Ping.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("failed to ping Elasticsearch cluster (v8): %w", err)
		}
		defer res.Body.Close()
		if res.IsError() {
			return fmt.Errorf("Elasticsearch cluster returned error (v8): %s", res.String())
		}
	default: // "v9"
		cfg := es9.Config{
			Addresses:     hosts,
			MaxRetries:    1,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client, err := es9.NewClient(cfg)
		if err != nil {
			return fmt.Errorf("failed to create test client (v9): %w", err)
		}
		res, err := client.Ping(client.Ping.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("failed to ping Elasticsearch cluster (v9): %w", err)
		}
		defer res.Body.Close()
		if res.IsError() {
			return fmt.Errorf("Elasticsearch cluster returned error (v9): %s", res.String())
		}
	}

	return nil
}

// TestElasticsearchIndexExists tests if a specific index exists in Elasticsearch
// version: "v7", "v8", or "v9" (empty => auto-detect, fallback v8)
func TestElasticsearchIndexExists(hosts []string, index string, version string, auth *ElasticsearchAuthConfig) (bool, error) {
	normVersion := detectOrDefaultElasticsearchVersion(hosts, version, auth)

	buildCommonTransport := func() *http.Transport {
		return &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Skip TLS certificate verification
			},
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch normVersion {
	case "v7":
		cfg := es7.Config{
			Addresses:     hosts,
			MaxRetries:    1,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client, err := es7.NewClient(cfg)
		if err != nil {
			return false, fmt.Errorf("failed to create test client (v7): %w", err)
		}
		res, err := client.Indices.Exists([]string{index}, client.Indices.Exists.WithContext(ctx))
		if err != nil {
			return false, fmt.Errorf("failed to check index existence (v7): %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode == 200 {
			return true, nil
		} else if res.StatusCode == 404 {
			return false, nil
		}
		return false, fmt.Errorf("unexpected response when checking index (v7): %s", res.String())
	case "v8":
		cfg := es8.Config{
			Addresses:     hosts,
			MaxRetries:    1,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client, err := es8.NewClient(cfg)
		if err != nil {
			return false, fmt.Errorf("failed to create test client (v8): %w", err)
		}
		res, err := client.Indices.Exists([]string{index}, client.Indices.Exists.WithContext(ctx))
		if err != nil {
			return false, fmt.Errorf("failed to check index existence (v8): %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode == 200 {
			return true, nil
		} else if res.StatusCode == 404 {
			return false, nil
		}
		return false, fmt.Errorf("unexpected response when checking index (v8): %s", res.String())
	default: // "v9"
		cfg := es9.Config{
			Addresses:     hosts,
			MaxRetries:    1,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client, err := es9.NewClient(cfg)
		if err != nil {
			return false, fmt.Errorf("failed to create test client (v9): %w", err)
		}
		res, err := client.Indices.Exists([]string{index}, client.Indices.Exists.WithContext(ctx))
		if err != nil {
			return false, fmt.Errorf("failed to check index existence (v9): %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode == 200 {
			return true, nil
		} else if res.StatusCode == 404 {
			return false, nil
		}
		return false, fmt.Errorf("unexpected response when checking index (v9): %s", res.String())
	}
}

// GetElasticsearchClusterInfo gets basic cluster information
// version: "v7", "v8", or "v9" (empty => auto-detect, fallback v8)
func GetElasticsearchClusterInfo(hosts []string, version string, auth *ElasticsearchAuthConfig) (map[string]interface{}, error) {
	normVersion := detectOrDefaultElasticsearchVersion(hosts, version, auth)

	buildCommonTransport := func() *http.Transport {
		return &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Skip TLS certificate verification
			},
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch normVersion {
	case "v7":
		cfg := es7.Config{
			Addresses:     hosts,
			MaxRetries:    1,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client, err := es7.NewClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create test client (v7): %w", err)
		}
		res, err := client.Info(client.Info.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster info (v7): %w", err)
		}
		defer res.Body.Close()
		if res.IsError() {
			return nil, fmt.Errorf("Elasticsearch cluster returned error (v7): %s", res.String())
		}
		var clusterInfo map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&clusterInfo); err != nil {
			return nil, fmt.Errorf("failed to decode cluster info (v7): %w", err)
		}
		return clusterInfo, nil
	case "v8":
		cfg := es8.Config{
			Addresses:     hosts,
			MaxRetries:    1,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client, err := es8.NewClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create test client (v8): %w", err)
		}
		res, err := client.Info(client.Info.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster info (v8): %w", err)
		}
		defer res.Body.Close()
		if res.IsError() {
			return nil, fmt.Errorf("Elasticsearch cluster returned error (v8): %s", res.String())
		}
		var clusterInfo map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&clusterInfo); err != nil {
			return nil, fmt.Errorf("failed to decode cluster info (v8): %w", err)
		}
		return clusterInfo, nil
	default: // "v9"
		cfg := es9.Config{
			Addresses:     hosts,
			MaxRetries:    1,
			RetryOnStatus: []int{502, 503, 504, 429},
			Transport:     buildCommonTransport(),
		}
		if auth != nil {
			switch auth.Type {
			case "basic":
				if auth.Username != "" && auth.Password != "" {
					cfg.Username = auth.Username
					cfg.Password = auth.Password
				}
			case "api_key":
				if auth.APIKey != "" {
					cfg.APIKey = auth.APIKey
				}
			case "bearer":
				if auth.Token != "" {
					cfg.Header = http.Header{
						"Authorization": []string{"Bearer " + auth.Token},
					}
				}
			}
		}
		client, err := es9.NewClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create test client (v9): %w", err)
		}
		res, err := client.Info(client.Info.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster info (v9): %w", err)
		}
		defer res.Body.Close()
		if res.IsError() {
			return nil, fmt.Errorf("Elasticsearch cluster returned error (v9): %s", res.String())
		}
		var clusterInfo map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&clusterInfo); err != nil {
			return nil, fmt.Errorf("failed to decode cluster info (v9): %w", err)
		}
		return clusterInfo, nil
	}
}
