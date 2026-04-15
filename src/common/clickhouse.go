package common

import (
	"AgentSmith-HUB/logger"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// ClickHouseAuthConfig represents authentication configuration for ClickHouse
type ClickHouseAuthConfig struct {
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// ClickHouseTLSConfig represents TLS configuration for ClickHouse
type ClickHouseTLSConfig struct {
	Enable             bool   `yaml:"enable"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify,omitempty"`
	CAFile             string `yaml:"ca_file,omitempty"`
	CertFile           string `yaml:"cert_file,omitempty"`
	KeyFile            string `yaml:"key_file,omitempty"`
}

// ClickHouseProducer wraps an HTTP client for batch inserts into ClickHouse
type ClickHouseProducer struct {
	httpClient *http.Client
	hosts      []string // ClickHouse HTTP endpoints (e.g. http://localhost:8123)
	database   string
	table      string
	auth       *ClickHouseAuthConfig
	MsgChan    chan map[string]interface{}
	batchSize  int
	flushDur   time.Duration
	maxRetries int
	retryDelay time.Duration
	stopChan   chan struct{}
	hostIndex  uint64 // for round-robin host selection
}

// buildClickHouseTLSConfig creates a *tls.Config from ClickHouseTLSConfig
func buildClickHouseTLSConfig(cfg *ClickHouseTLSConfig) (*tls.Config, error) {
	if cfg == nil || !cfg.Enable {
		return nil, nil
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}

	// Load CA certificate if provided
	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file %s: %w", cfg.CAFile, err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", cfg.CAFile)
		}
		tlsCfg.RootCAs = caCertPool
	}

	// Load client certificate and key if provided (mutual TLS)
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

// buildClickHouseHTTPClient creates an HTTP client with optional TLS
func buildClickHouseHTTPClient(tlsCfg *ClickHouseTLSConfig) (*http.Client, error) {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	if tlsCfg != nil && tlsCfg.Enable {
		tc, err := buildClickHouseTLSConfig(tlsCfg)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tc
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

// NewClickHouseProducer creates a new ClickHouse producer that batch-inserts via the HTTP interface
func NewClickHouseProducer(
	hosts []string,
	database string,
	table string,
	msgChan chan map[string]interface{},
	batchSize int,
	flushDur time.Duration,
	auth *ClickHouseAuthConfig,
	tlsCfg *ClickHouseTLSConfig,
) (*ClickHouseProducer, error) {
	httpClient, err := buildClickHouseHTTPClient(tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create ClickHouse HTTP client: %w", err)
	}

	prod := &ClickHouseProducer{
		httpClient: httpClient,
		hosts:      hosts,
		database:   database,
		table:      table,
		auth:       auth,
		MsgChan:    msgChan,
		batchSize:  batchSize,
		flushDur:   flushDur,
		maxRetries: 3,
		retryDelay: 1 * time.Second,
		stopChan:   make(chan struct{}),
	}

	go prod.run()
	return prod, nil
}

// nextHost returns the next host in round-robin fashion
func (p *ClickHouseProducer) nextHost() string {
	if len(p.hosts) == 1 {
		return p.hosts[0]
	}
	idx := atomic.AddUint64(&p.hostIndex, 1)
	return p.hosts[idx%uint64(len(p.hosts))]
}

// buildInsertURL constructs the ClickHouse HTTP insert URL for the given host
func (p *ClickHouseProducer) buildInsertURL(host string) string {
	// Ensure host has scheme
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}

	query := fmt.Sprintf("INSERT INTO %s.%s FORMAT JSONEachRow", p.database, p.table)
	params := url.Values{}
	params.Set("query", query)

	return host + "/?" + params.Encode()
}

// buildRequest creates an HTTP request with authentication headers
func (p *ClickHouseProducer) buildRequest(ctx context.Context, method, reqURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, err
	}

	// Set authentication via HTTP headers (X-ClickHouse-User / X-ClickHouse-Key)
	if p.auth != nil {
		if p.auth.Username != "" {
			req.Header.Set("X-ClickHouse-User", p.auth.Username)
		}
		if p.auth.Password != "" {
			req.Header.Set("X-ClickHouse-Key", p.auth.Password)
		}
	}

	return req, nil
}

func (p *ClickHouseProducer) run() {
	batch := make([]map[string]interface{}, 0, p.batchSize)
	timer := time.NewTimer(p.flushDur)
	defer timer.Stop()

	for {
		select {
		case <-p.stopChan:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			// Don't flush remaining batch during shutdown to avoid blocking
			return
		case msg, ok := <-p.MsgChan:
			if !ok {
				// Channel is closed, check if stop signal was received
				select {
				case <-p.stopChan:
					return
				default:
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

// sendBatch sends a batch of documents to ClickHouse via HTTP with retry logic
func (p *ClickHouseProducer) sendBatch(batch []map[string]interface{}) {
	if len(batch) == 0 {
		return
	}

	// Encode batch as JSONEachRow (newline-delimited JSON)
	var buf bytes.Buffer
	for _, doc := range batch {
		if err := json.NewEncoder(&buf).Encode(doc); err != nil {
			logger.Error("ClickHouse failed to encode document", "error", err)
			continue
		}
	}

	for i := 0; i <= p.maxRetries; i++ {
		// Check stop signal before each attempt
		select {
		case <-p.stopChan:
			return
		default:
		}

		host := p.nextHost()
		reqURL := p.buildInsertURL(host)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		req, err := p.buildRequest(ctx, http.MethodPost, reqURL, bytes.NewReader(buf.Bytes()))
		if err != nil {
			cancel()
			logger.Error("ClickHouse failed to build request", "error", err)
			return
		}

		resp, err := p.httpClient.Do(req)
		cancel()

		if err != nil {
			if i == p.maxRetries {
				logger.Error("ClickHouse failed to send batch after retries", "retries", p.maxRetries, "error", err)
				return
			}
			select {
			case <-p.stopChan:
				return
			case <-time.After(p.retryDelay):
			}
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if i == p.maxRetries {
				logger.Error("ClickHouse returned error after retries", "retries", p.maxRetries, "status", resp.StatusCode, "response", string(body))
				return
			}
			select {
			case <-p.stopChan:
				return
			case <-time.After(p.retryDelay):
			}
			continue
		}

		// Success
		return
	}
}

// flush batch writes to ClickHouse
func (p *ClickHouseProducer) flush(batch []map[string]interface{}) {
	p.sendBatch(batch)
}

// Close closes the producer
// Note: We don't close MsgChan here because it's owned by the caller
func (p *ClickHouseProducer) Close() {
	if p.stopChan != nil {
		close(p.stopChan)
	}
}

// --- Connection test utilities ---

// buildTestHTTPClient creates a temporary HTTP client for connection testing
func buildTestHTTPClient(tlsCfg *ClickHouseTLSConfig) (*http.Client, error) {
	return buildClickHouseHTTPClient(tlsCfg)
}

// doClickHouseQuery executes a simple query against a ClickHouse host and returns the response body
func doClickHouseQuery(client *http.Client, host string, query string, auth *ClickHouseAuthConfig) ([]byte, int, error) {
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}

	params := url.Values{}
	params.Set("query", query)

	reqURL := host + "/?" + params.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	if auth != nil {
		if auth.Username != "" {
			req.Header.Set("X-ClickHouse-User", auth.Username)
		}
		if auth.Password != "" {
			req.Header.Set("X-ClickHouse-Key", auth.Password)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, resp.StatusCode, nil
}

// TestClickHouseConnection tests connectivity to a ClickHouse instance
func TestClickHouseConnection(hosts []string, auth *ClickHouseAuthConfig, tlsCfg *ClickHouseTLSConfig) error {
	client, err := buildTestHTTPClient(tlsCfg)
	if err != nil {
		return fmt.Errorf("failed to create test HTTP client: %w", err)
	}

	// Try the first host with a simple SELECT 1 query
	host := hosts[0]
	_, statusCode, err := doClickHouseQuery(client, host, "SELECT 1", auth)
	if err != nil {
		return fmt.Errorf("failed to connect to ClickHouse at %s: %w", host, err)
	}

	if statusCode != http.StatusOK {
		return fmt.Errorf("ClickHouse returned unexpected status code %d", statusCode)
	}

	return nil
}

// TestClickHouseTableExists checks if a table exists in ClickHouse
func TestClickHouseTableExists(hosts []string, database string, table string, auth *ClickHouseAuthConfig, tlsCfg *ClickHouseTLSConfig) (bool, error) {
	client, err := buildTestHTTPClient(tlsCfg)
	if err != nil {
		return false, fmt.Errorf("failed to create test HTTP client: %w", err)
	}

	host := hosts[0]
	query := fmt.Sprintf("EXISTS TABLE %s.%s", database, table)
	body, statusCode, err := doClickHouseQuery(client, host, query, auth)
	if err != nil {
		return false, fmt.Errorf("failed to check table existence: %w", err)
	}

	if statusCode != http.StatusOK {
		return false, fmt.Errorf("ClickHouse returned status %d when checking table", statusCode)
	}

	// ClickHouse returns "1\n" if table exists, "0\n" if not
	result := strings.TrimSpace(string(body))
	return result == "1", nil
}

// GetClickHouseServerInfo returns basic server information from ClickHouse
func GetClickHouseServerInfo(hosts []string, auth *ClickHouseAuthConfig, tlsCfg *ClickHouseTLSConfig) (map[string]interface{}, error) {
	client, err := buildTestHTTPClient(tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create test HTTP client: %w", err)
	}

	host := hosts[0]

	// Get version
	versionBody, statusCode, err := doClickHouseQuery(client, host, "SELECT version()", auth)
	if err != nil {
		return nil, fmt.Errorf("failed to get ClickHouse version: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("ClickHouse returned status %d", statusCode)
	}

	info := map[string]interface{}{
		"version": strings.TrimSpace(string(versionBody)),
		"host":    host,
	}

	// Get uptime (best-effort, don't fail if unavailable)
	uptimeBody, statusCode, err := doClickHouseQuery(client, host, "SELECT uptime()", auth)
	if err == nil && statusCode == http.StatusOK {
		info["uptime_seconds"] = strings.TrimSpace(string(uptimeBody))
	}

	return info, nil
}
