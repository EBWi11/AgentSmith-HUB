package output

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// OutputType defines the type of output destination.
type OutputType string

const (
	OutputTypeKafka         OutputType = "kafka"
	OutputTypeKafkaAzure    OutputType = "kafka_azure"
	OutputTypeKafkaAWS      OutputType = "kafka_aws"
	OutputTypeElasticsearch OutputType = "elasticsearch"
	OutputTypeAliyunSLS     OutputType = "aliyun_sls"
	OutputTypePrint         OutputType = "print"
)

// OutputConfig is the YAML config for an output.
type OutputConfig struct {
	Id            string
	Type          OutputType                 `yaml:"type"`
	Kafka         *KafkaOutputConfig         `yaml:"kafka,omitempty"`
	Elasticsearch *ElasticsearchOutputConfig `yaml:"elasticsearch,omitempty"`
	AliyunSLS     *AliyunSLSOutputConfig     `yaml:"aliyun_sls,omitempty"`
	RawConfig     string
}

// KafkaOutputConfig holds Kafka-specific config.
type KafkaOutputConfig struct {
	Brokers     []string                    `yaml:"brokers"`
	Topic       string                      `yaml:"topic"`
	Compression common.KafkaCompressionType `yaml:"compression,omitempty"`
	SASL        *common.KafkaSASLConfig     `yaml:"sasl,omitempty"`
	TLS         *common.KafkaTLSConfig      `yaml:"tls,omitempty"`
	Key         string                      `yaml:"key"`
}

// ElasticsearchOutputConfig holds Elasticsearch-specific config.
type ElasticsearchOutputConfig struct {
	Hosts     []string                        `yaml:"hosts"`
	Index     string                          `yaml:"index"`
	BatchSize int                             `yaml:"batch_size,omitempty"`
	FlushDur  string                          `yaml:"flush_dur,omitempty"`
	Auth      *common.ElasticsearchAuthConfig `yaml:"auth,omitempty"`
}

// AliyunSLSOutputConfig holds Aliyun SLS-specific config.
type AliyunSLSOutputConfig struct {
	Endpoint        string `yaml:"endpoint"`
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
	Project         string `yaml:"project"`
	Logstore        string `yaml:"logstore"`
}

// Output is the runtime output instance.
type Output struct {
	Status              common.Status
	StatusChangedAt     *time.Time `json:"status_changed_at,omitempty"`
	Err                 error      `json:"-"`
	Id                  string     `json:"Id"`
	Path                string
	ProjectNodeSequence string
	Type                OutputType
	UpStream            map[string]*chan map[string]interface{}

	// runtime
	kafkaProducer         *common.KafkaProducer
	elasticsearchProducer *common.ElasticsearchProducer
	wg                    sync.WaitGroup

	// config cache
	kafkaCfg         *KafkaOutputConfig
	elasticsearchCfg *ElasticsearchOutputConfig
	aliyunSLSCfg     *AliyunSLSOutputConfig

	// metrics - only total count is needed now
	produceTotal      uint64 // cumulative production total
	lastReportedTotal uint64 // For calculating increments in 10-second intervals

	// sampler
	sampler *common.Sampler

	// for print output
	printStop chan struct{}

	// for testing
	TestCollectionChan *chan map[string]interface{}

	// raw config
	Config *OutputConfig

	// OwnerProjects field removed - project usage is now calculated dynamically
}

func Verify(path string, raw string) error {
	var cfg OutputConfig

	// Use common file reading function
	data, err := common.ReadContentFromPathOrRaw(path, raw)
	if err != nil {
		return fmt.Errorf("failed to read output configuration: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		errString := err.Error()
		if yamlErr, ok := err.(*yaml.TypeError); ok && len(yamlErr.Errors) > 0 {
			errMsg := yamlErr.Errors[0]
			lineInfo := ""
			for _, line := range yamlErr.Errors {
				if strings.Contains(line, "line") {
					lineInfo = line
					break
				}
			}
			return fmt.Errorf("failed to parse output configuration: %s (location: %s)", errMsg, lineInfo)
		} else {
			// Use regex to extract line number from general YAML errors
			linePattern := `(?i)(?:yaml: |at )?line (\d+)[:]*\s*(.*)`
			if match := regexp.MustCompile(linePattern).FindStringSubmatch(errString); len(match) > 2 {
				lineNum := match[1]
				errorDesc := strings.TrimSpace(match[2])
				if errorDesc == "" {
					errorDesc = errString
				}
				return fmt.Errorf("YAML parse error: yaml-line %s: %s", lineNum, errorDesc)
			}
			return fmt.Errorf("YAML parse error: %s", errString)
		}
	}

	// Validate required fields
	if cfg.Type == "" {
		return fmt.Errorf("missing required field 'type' (line: unknown)")
	}

	// Validate type-specific fields
	switch cfg.Type {
	case OutputTypeKafka, OutputTypeKafkaAzure, OutputTypeKafkaAWS:
		if cfg.Kafka == nil {
			return fmt.Errorf("missing required field 'kafka' for kafka output (line: unknown)")
		}
		if len(cfg.Kafka.Brokers) == 0 {
			return fmt.Errorf("missing required field 'kafka.brokers' for kafka output (line: unknown)")
		}
		if cfg.Kafka.Topic == "" {
			return fmt.Errorf("missing required field 'kafka.topic' for kafka output (line: unknown)")
		}
	case OutputTypeElasticsearch:
		if cfg.Elasticsearch == nil {
			return fmt.Errorf("missing required field 'elasticsearch' for elasticsearch output (line: unknown)")
		}
		if len(cfg.Elasticsearch.Hosts) == 0 {
			return fmt.Errorf("missing required field 'elasticsearch.hosts' for elasticsearch output (line: unknown)")
		}
		if cfg.Elasticsearch.Index == "" {
			return fmt.Errorf("missing required field 'elasticsearch.index' for elasticsearch output (line: unknown)")
		}
	case OutputTypeAliyunSLS:
		if cfg.AliyunSLS == nil {
			return fmt.Errorf("missing required field 'aliyun_sls' for aliyunSLS output (line: unknown)")
		}
		// Add more AliyunSLS specific field validation
	case OutputTypePrint:
		// Print output doesn't require external connectivity
	default:
		return fmt.Errorf("unsupported output type: %s (line: unknown)", cfg.Type)
	}

	return nil
}

// NewOutput creates an Output from config and upstreams.
func NewOutput(path string, raw string, id string) (*Output, error) {
	var cfg OutputConfig

	err := Verify(path, raw)
	if err != nil {
		return nil, fmt.Errorf("output verify error: %s %s", id, err.Error())
	}

	if path != "" {
		data, _ := os.ReadFile(path)
		_ = yaml.Unmarshal(data, &cfg)
		cfg.RawConfig = string(data)
	} else {
		_ = yaml.Unmarshal([]byte(raw), &cfg)
		cfg.RawConfig = raw
	}

	out := &Output{
		Id:               id,
		Path:             path,
		Type:             cfg.Type,
		UpStream:         make(map[string]*chan map[string]interface{}, 0),
		kafkaCfg:         cfg.Kafka,
		elasticsearchCfg: cfg.Elasticsearch,
		aliyunSLSCfg:     cfg.AliyunSLS,
		Config:           &cfg,
		sampler:          nil, // Will be set below based on cluster role
		Status:           common.StatusStopped,
	}

	// Only create sampler on leader node for performance
	if common.IsLeader {
		out.sampler = common.GetSampler("output." + id)
	}
	return out, nil
}

// SetStatus sets the output status and error information
func (out *Output) SetStatus(status common.Status, err error) {
	if err != nil {
		out.Err = err
		logger.Error("Output status changed with error", "output", out.Id, "status", status, "error", err)
	}
	out.Status = status
	t := time.Now()
	out.StatusChangedAt = &t
}

// cleanup performs cleanup when normal stop fails or panic occurs
func (out *Output) cleanup() {
	// Close print stop channel if it exists and not already closed
	if out.printStop != nil {
		select {
		case <-out.printStop:
			// Already closed
		default:
			close(out.printStop)
		}
		out.printStop = nil
	}

	// Stop producers
	if out.kafkaProducer != nil {
		out.kafkaProducer.Close()
		out.kafkaProducer = nil
	}

	if out.elasticsearchProducer != nil {
		out.elasticsearchProducer.Close()
		out.elasticsearchProducer = nil
	}

	// Reset atomic counter
	atomic.StoreUint64(&out.produceTotal, 0)
	atomic.StoreUint64(&out.lastReportedTotal, 0)

	// Clear test collection channel
	out.TestCollectionChan = nil

	// Clear component channel connections to prevent leaks
	out.UpStream = make(map[string]*chan map[string]interface{})
}

// enhanceMessageWithProjectNodeSequence adds ProjectNodeSequence and output metadata to the message
func (out *Output) enhanceMessageWithProjectNodeSequence(msg map[string]interface{}) map[string]interface{} {
	// Create a copy of the original message to avoid modifying the original
	enhancedMsg := make(map[string]interface{})
	for k, v := range msg {
		enhancedMsg[k] = v
	}

	// Add ProjectNodeSequence information
	enhancedMsg["_hub_project_node_sequence"] = out.ProjectNodeSequence
	enhancedMsg["_hub_output_timestamp"] = time.Now().UTC().Format(time.RFC3339)

	return enhancedMsg
}

// StartForTesting starts the output component in testing mode
// In testing mode, completely ignore output type and only send data to TestCollectionChan
func (out *Output) StartForTesting() error {
	if out.Status != common.StatusStopped {
		return fmt.Errorf("output %s is not stopped", out.Id)
	}

	out.ResetProduceTotal()
	out.SetStatus(common.StatusStarting, nil)

	// Start single goroutine to read from UpStream and send to TestCollectionChan only
	out.wg.Add(1)
	go func() {
		defer out.wg.Done()

		for {
			// Non-blocking check for messages from any upstream channel
			processed := false
			for _, up := range out.UpStream {
				select {
				case msg := <-*up:
					atomic.AddUint64(&out.produceTotal, 1)

					// Skip sampling in testing mode (handled by SetTestMode)
					if out.sampler != nil {
						out.sampler.Sample(msg, out.ProjectNodeSequence)
					}

					// Enhance message with ProjectNodeSequence information
					enhancedMsg := out.enhanceMessageWithProjectNodeSequence(msg)

					if out.TestCollectionChan != nil {
						*out.TestCollectionChan <- enhancedMsg
					}

					processed = true
				default:
				}
			}
			if !processed {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	out.SetStatus(common.StatusRunning, nil)
	return nil
}

// Start initializes and starts the output component based on its type
// Returns an error if the component is already running or if initialization fails
// If TestCollectionChan is set, messages will be duplicated to that chan for testing purposes,
// but the original output type will still be used so that real external side-effects can be observed.
func (out *Output) Start() error {
	// Add panic recovery for critical state changes
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Panic during output start", "output", out.Id, "panic", r)
			// Ensure cleanup and proper status setting on panic
			out.cleanup()
			out.SetStatus(common.StatusError, fmt.Errorf("panic during start: %v", r))
		}
	}()

	// Allow restart from stopped state or from error state
	if out.Status != common.StatusStopped && out.Status != common.StatusError {
		return fmt.Errorf("output %s is not stopped (status: %s)", out.Id, out.Status)
	}

	// Clear error state when restarting
	out.Err = nil
	out.ResetProduceTotal()
	out.SetStatus(common.StatusStarting, nil)
	// Perform connectivity check first before starting (skip for print type as it doesn't need external connectivity)
	if out.Type != OutputTypePrint {
		connectivityResult := out.CheckConnectivity()
		if status, ok := connectivityResult["status"].(string); ok && status == "error" {
			out.SetStatus(common.StatusError, fmt.Errorf("output connectivity check failed: %v", connectivityResult["message"]))
			return fmt.Errorf("output connectivity check failed: %v", connectivityResult["message"])
		}
		logger.Info("Output connectivity verified", "output", out.Id, "type", out.Type)
	}

	// Determine if we need to duplicate data for testing
	hasTestCollector := out.TestCollectionChan != nil

	effectiveType := out.Type

	switch effectiveType {
	case OutputTypeKafka, OutputTypeKafkaAzure, OutputTypeKafkaAWS:
		if out.kafkaProducer != nil {
			out.SetStatus(common.StatusError, fmt.Errorf("kafka producer already running for output %s", out.Id))
			return fmt.Errorf("kafka producer already running for output %s", out.Id)
		}
		if out.kafkaCfg == nil {
			out.SetStatus(common.StatusError, fmt.Errorf("kafka configuration missing for output %s", out.Id))
			return fmt.Errorf("kafka configuration missing for output %s", out.Id)
		}

		msgChan := make(chan map[string]interface{}, 1024)
		producer, err := common.NewKafkaProducer(
			out.kafkaCfg.Brokers,
			out.kafkaCfg.Topic,
			out.kafkaCfg.Compression,
			out.kafkaCfg.SASL,
			msgChan,
			out.kafkaCfg.Key,
			out.kafkaCfg.TLS,
		)
		if err != nil {
			out.SetStatus(common.StatusError, fmt.Errorf("failed to create kafka producer for output %s: %v", out.Id, err))
			return fmt.Errorf("failed to create kafka producer for output %s: %v", out.Id, err)
		}
		out.kafkaProducer = producer

		// Start goroutine to read from UpStream and send enhanced messages to msgChan for Kafka producer
		out.wg.Add(1)
		go func() {
			defer out.wg.Done()
			defer close(msgChan) // Close msgChan when UpStream processing is done

			for {
				// Non-blocking check for messages from any upstream channel
				processed := false
				for _, up := range out.UpStream {
					select {
					case msg, ok := <-*up:
						if !ok {
							// Channel is closed, skip this channel
							continue
						}

						// Always count/sample; duplication handled below
						// Count immediately at upstream read to ensure all messages are counted
						atomic.AddUint64(&out.produceTotal, 1)

						// Sample the message
						if out.sampler != nil {
							out.sampler.Sample(msg, out.ProjectNodeSequence)
						}

						// Enhance message with ProjectNodeSequence information before sending
						enhancedMsg := out.enhanceMessageWithProjectNodeSequence(msg)

						// Duplicate to TestCollectionChan if present (non-blocking)
						if hasTestCollector {
							select {
							case *out.TestCollectionChan <- enhancedMsg:
							default:
								logger.Warn("Test collection channel full, dropping message", "id", out.Id, "type", "kafka")
							}
						}

						// Send enhanced message to msgChan for Kafka producer
						msgChan <- enhancedMsg
						processed = true
					default:
						// No message available from this channel, continue to next
					}
				}
				// If no messages were processed, sleep briefly to avoid busy waiting
				if !processed {
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()

	case OutputTypeElasticsearch:
		if out.elasticsearchProducer != nil {
			out.SetStatus(common.StatusError, fmt.Errorf("elasticsearch producer already running for output %s", out.Id))
			return fmt.Errorf("elasticsearch producer already running for output %s", out.Id)
		}
		if out.elasticsearchCfg == nil {
			out.SetStatus(common.StatusError, fmt.Errorf("elasticsearch configuration missing for output %s", out.Id))
			return fmt.Errorf("elasticsearch configuration missing for output %s", out.Id)
		}

		msgChan := make(chan map[string]interface{}, 1024)
		batchSize := 100
		if out.elasticsearchCfg.BatchSize > 0 {
			batchSize = out.elasticsearchCfg.BatchSize
		}
		flushDur := 3 * time.Second
		if out.elasticsearchCfg.FlushDur != "" {
			if d, err := time.ParseDuration(out.elasticsearchCfg.FlushDur); err == nil {
				flushDur = d
			}
		}
		producer, err := common.NewElasticsearchProducer(
			out.elasticsearchCfg.Hosts,
			out.elasticsearchCfg.Index,
			msgChan,
			batchSize,
			flushDur,
			out.elasticsearchCfg.Auth,
		)
		if err != nil {
			out.SetStatus(common.StatusError, fmt.Errorf("failed to create elasticsearch producer for output %s: %v", out.Id, err))
			return fmt.Errorf("failed to create elasticsearch producer for output %s: %v", out.Id, err)
		}
		out.elasticsearchProducer = producer

		// Start goroutine to read from UpStream and send enhanced messages to msgChan for Elasticsearch producer
		out.wg.Add(1)
		go func() {
			defer out.wg.Done()
			defer close(msgChan) // Close msgChan when UpStream processing is done

			for {
				// Non-blocking check for messages from any upstream channel
				processed := false
				for _, up := range out.UpStream {
					select {
					case msg, ok := <-*up:
						if !ok {
							// Channel is closed, skip this channel
							continue
						}

						// Always count/sample; duplication handled separately
						// Count immediately at upstream read to ensure all messages are counted
						atomic.AddUint64(&out.produceTotal, 1)

						// Sample the message
						if out.sampler != nil {
							out.sampler.Sample(msg, out.ProjectNodeSequence)
						}

						// Enhance message with ProjectNodeSequence information before sending
						enhancedMsg := out.enhanceMessageWithProjectNodeSequence(msg)

						if hasTestCollector {
							select {
							case *out.TestCollectionChan <- enhancedMsg:
							default:
								logger.Warn("Test collection channel full, dropping message", "id", out.Id, "type", "elasticsearch")
							}
						}

						// Send enhanced message to msgChan for Elasticsearch producer
						msgChan <- enhancedMsg
						processed = true
					default:
						// No message available from this channel, continue to next
					}
				}
				// If no messages were processed, sleep briefly to avoid busy waiting
				if !processed {
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()

	case OutputTypePrint:
		out.printStop = make(chan struct{})
		out.wg.Add(1)
		go func() {
			defer out.wg.Done()
			for {
				select {
				case <-out.printStop:
					return
				default:
					// Non-blocking check for messages from any upstream channel
					processed := false
					for _, up := range out.UpStream {
						select {
						case msg, ok := <-*up:
							if !ok {
								// Channel is closed, skip this channel
								continue
							}
							// Always count/sample.
							// Count immediately at upstream read to ensure all messages are counted
							atomic.AddUint64(&out.produceTotal, 1)

							// Sample the message
							if out.sampler != nil {
								out.sampler.Sample(msg, out.ProjectNodeSequence)
							}

							// Duplicate to TestCollectionChan if present
							if hasTestCollector {
								msgWithId := out.enhanceMessageWithProjectNodeSequence(msg)
								select {
								case *out.TestCollectionChan <- msgWithId:
								default:
									logger.Warn("Test collection channel full, dropping message", "id", out.Id, "type", "print")
								}
							}

							// Enhance message with ProjectNodeSequence information for actual output
							enhancedMsg := out.enhanceMessageWithProjectNodeSequence(msg)
							data, _ := json.Marshal(enhancedMsg)
							logger.Info("[Print Output]", "data", string(data))
							processed = true
						default:
							// No message available from this channel, continue to next
						}
					}
					// If no messages were processed, sleep briefly to avoid busy waiting
					if !processed {
						time.Sleep(10 * time.Millisecond)
					}
				}
			}
		}()

	case OutputTypeAliyunSLS:
		out.SetStatus(common.StatusError, fmt.Errorf("aliyun SLS output not implemented yet"))
		return fmt.Errorf("aliyun SLS output not implemented yet")
	}

	out.SetStatus(common.StatusRunning, nil)
	return nil
}

// Stop stops the output producer and waits for all routines to finish.
// It waits until all upstream channels are empty and all pending data is written.
func (out *Output) Stop() error {
	if out.Status != common.StatusRunning && out.Status != common.StatusError {
		return fmt.Errorf("output %s is not running", out.Id)
	}
	out.SetStatus(common.StatusStopping, nil)

	// Overall timeout for output stop
	overallTimeout := time.After(30 * time.Second) // Reduced from 60s to 30s
	stopCompleted := make(chan struct{})

	go func() {
		defer close(stopCompleted)

		// Wait for all upstream channels to be empty before closing producers
		logger.Info("Waiting for upstream channels to empty", "output", out.Id)
		upstreamTimeout := time.After(10 * time.Second) // 10 second timeout for upstream
		waitCount := 0

	waitUpstream:
		for {
			select {
			case <-upstreamTimeout:
				logger.Warn("Timeout waiting for upstream channels, forcing shutdown", "output", out.Id)
				break waitUpstream
			default:
				allEmpty := true
				totalMessages := 0
				for i, up := range out.UpStream {
					chLen := len(*up)
					if chLen > 0 {
						allEmpty = false
						totalMessages += chLen
						if waitCount%20 == 0 { // Log every second (20 * 50ms)
							logger.Info("Output upstream channel not empty", "output", out.Id, "channel", i, "length", chLen)
						}
					}
				}
				if allEmpty {
					logger.Info("All output upstream channels empty", "output", out.Id)
					break waitUpstream
				}
				waitCount++
				if waitCount%20 == 0 { // Log every second (20 * 50ms)
					logger.Info("Still waiting for output upstream channels", "output", out.Id, "total_messages", totalMessages, "wait_cycles", waitCount)
				}
				time.Sleep(50 * time.Millisecond)
			}
		}

		// Brief wait for internal channels to drain
		logger.Info("Waiting for output internal channels to empty", "output", out.Id)
		time.Sleep(1 * time.Second)

		// Metrics stop removed
	}()

	select {
	case <-stopCompleted:
		logger.Info("Output channels drained successfully", "output", out.Id)
	case <-overallTimeout:
		logger.Warn("Output stop timeout exceeded, forcing cleanup", "output", out.Id)
	}

	// Wait for goroutines to finish with timeout
	logger.Info("Waiting for output goroutines to finish", "id", out.Id)
	waitDone := make(chan struct{})
	go func() {
		out.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		logger.Info("Output stopped gracefully", "id", out.Id)
	case <-time.After(5 * time.Second):
		logger.Warn("Timeout waiting for output goroutines, forcing cleanup", "id", out.Id)
	}

	// Use cleanup to ensure all resources are properly released
	out.cleanup()

	out.SetStatus(common.StatusStopped, nil)
	return nil
}

// GetProduceTotal returns the total produced count.
func (out *Output) GetProduceTotal() uint64 {
	return atomic.LoadUint64(&out.produceTotal)
}

// ResetProduceTotal resets the total produced count to zero.
// This should only be called during component cleanup or forced restart.
func (out *Output) ResetProduceTotal() uint64 {
	atomic.StoreUint64(&out.lastReportedTotal, 0)
	return atomic.SwapUint64(&out.produceTotal, 0)
}

// GetIncrementAndUpdate returns the increment since last call and updates the baseline.
// This method is thread-safe and designed for 10-second statistics collection.
func (out *Output) GetIncrementAndUpdate() uint64 {
	current := atomic.LoadUint64(&out.produceTotal)
	last := atomic.SwapUint64(&out.lastReportedTotal, current)

	// Since counters are monotonically increasing and we don't consider overflow
	// (uint64 is large enough), current should always be >= last
	return current - last
}

// StopForTesting stops the output quickly for testing purposes without waiting for channel drainage
func (out *Output) StopForTesting() error {
	logger.Info("Quick stopping test output", "id", out.Id, "type", out.Type)

	// Clear test collection channel
	out.TestCollectionChan = nil

	// In testing mode, there are no special channels or producers to close
	// Just wait for the single goroutine to finish

	// Wait for goroutines with very short timeout
	waitDone := make(chan struct{})
	go func() {
		out.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		logger.Info("Test output stopped successfully", "id", out.Id)
	case <-time.After(1 * time.Second): // Very short timeout for testing
		logger.Warn("Timeout waiting for test output goroutines, proceeding anyway", "id", out.Id)
	}

	// Reset atomic counter for testing cleanup
	previousTotal := atomic.LoadUint64(&out.produceTotal)
	atomic.StoreUint64(&out.produceTotal, 0)
	atomic.StoreUint64(&out.lastReportedTotal, 0)
	logger.Debug("Reset atomic counter for test output component", "output", out.Id, "previous_total", previousTotal)

	// Clear component channel connections to prevent leaks
	out.UpStream = make(map[string]*chan map[string]interface{})

	out.SetStatus(common.StatusStopped, nil)
	return nil
}

// CheckConnectivity performs a real connectivity test for the output component
// This method tests actual connection to external systems (Kafka, ES, etc.)
func (out *Output) CheckConnectivity() map[string]interface{} {
	result := map[string]interface{}{
		"status":  "success",
		"message": "Connection check successful",
		"details": map[string]interface{}{
			"client_type":         string(out.Type),
			"connection_status":   "unknown",
			"connection_info":     map[string]interface{}{},
			"connection_errors":   []map[string]interface{}{},
			"connection_warnings": []map[string]interface{}{},
		},
	}

	switch out.Type {
	case OutputTypeKafka, OutputTypeKafkaAzure, OutputTypeKafkaAWS:
		if out.kafkaCfg == nil {
			result["status"] = "error"
			result["message"] = "Kafka configuration missing"
			result["details"].(map[string]interface{})["connection_status"] = "not_configured"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": "Kafka configuration is incomplete or missing", "severity": "error"},
			}
			return result
		}

		// Set connection info
		connectionInfo := map[string]interface{}{
			"brokers": out.kafkaCfg.Brokers,
			"topic":   out.kafkaCfg.Topic,
		}
		result["details"].(map[string]interface{})["connection_info"] = connectionInfo

		// Test actual connectivity to Kafka brokers
		err := common.TestKafkaConnection(out.kafkaCfg.Brokers, out.kafkaCfg.SASL, out.kafkaCfg.TLS)
		if err != nil {
			result["status"] = "error"
			result["message"] = "Failed to connect to Kafka brokers"
			result["details"].(map[string]interface{})["connection_status"] = "connection_failed"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": err.Error(), "severity": "error"},
			}
			return result
		}

		// Test if topic exists
		topicExists, err := common.TestKafkaTopicExists(out.kafkaCfg.Brokers, out.kafkaCfg.Topic, out.kafkaCfg.SASL, out.kafkaCfg.TLS)
		if err != nil {
			result["status"] = "warning"
			result["message"] = "Connected to Kafka but failed to verify topic"
			result["details"].(map[string]interface{})["connection_status"] = "connected_topic_unknown"
			result["details"].(map[string]interface{})["connection_warnings"] = []map[string]interface{}{
				{"message": fmt.Sprintf("Could not verify topic existence: %v", err), "severity": "warning"},
			}
		} else if !topicExists {
			result["status"] = "warning"
			result["message"] = "Connected to Kafka but topic does not exist"
			result["details"].(map[string]interface{})["connection_status"] = "connected_topic_missing"
			result["details"].(map[string]interface{})["connection_warnings"] = []map[string]interface{}{
				{"message": fmt.Sprintf("Topic '%s' does not exist", out.kafkaCfg.Topic), "severity": "warning"},
			}
		} else {
			result["details"].(map[string]interface{})["connection_status"] = "connected"
			result["message"] = "Successfully connected to Kafka and verified topic"
		}

		// Add producer metrics if available
		if out.kafkaProducer != nil {
			result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
				"produce_total":   out.GetProduceTotal(),
				"producer_active": true,
			}
		} else {
			result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
				"producer_active": false,
			}
		}

	case OutputTypeElasticsearch:
		if out.elasticsearchCfg == nil {
			result["status"] = "error"
			result["message"] = "Elasticsearch configuration missing"
			result["details"].(map[string]interface{})["connection_status"] = "not_configured"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": "Elasticsearch configuration is incomplete or missing", "severity": "error"},
			}
			return result
		}

		// Set connection info
		connectionInfo := map[string]interface{}{
			"hosts": out.elasticsearchCfg.Hosts,
			"index": out.elasticsearchCfg.Index,
		}
		result["details"].(map[string]interface{})["connection_info"] = connectionInfo

		// Test actual connectivity to Elasticsearch cluster
		err := common.TestElasticsearchConnection(out.elasticsearchCfg.Hosts, out.elasticsearchCfg.Auth)
		if err != nil {
			result["status"] = "error"
			result["message"] = "Failed to connect to Elasticsearch cluster"
			result["details"].(map[string]interface{})["connection_status"] = "connection_failed"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": err.Error(), "severity": "error"},
			}
			return result
		}

		// Test if index exists (this is optional for ES as indices can be auto-created)
		indexExists, err := common.TestElasticsearchIndexExists(out.elasticsearchCfg.Hosts, out.elasticsearchCfg.Index, out.elasticsearchCfg.Auth)
		if err != nil {
			result["status"] = "warning"
			result["message"] = "Connected to Elasticsearch but failed to verify index"
			result["details"].(map[string]interface{})["connection_status"] = "connected_index_unknown"
			result["details"].(map[string]interface{})["connection_warnings"] = []map[string]interface{}{
				{"message": fmt.Sprintf("Could not verify index existence: %v", err), "severity": "warning"},
			}
		} else if !indexExists {
			result["status"] = "success" // This is OK for ES as indices can be auto-created
			result["message"] = "Connected to Elasticsearch (index will be auto-created)"
			result["details"].(map[string]interface{})["connection_status"] = "connected_index_will_be_created"
			result["details"].(map[string]interface{})["connection_warnings"] = []map[string]interface{}{
				{"message": fmt.Sprintf("Index '%s' does not exist but will be auto-created", out.elasticsearchCfg.Index), "severity": "info"},
			}
		} else {
			result["details"].(map[string]interface{})["connection_status"] = "connected"
			result["message"] = "Successfully connected to Elasticsearch and verified index"
		}

		// Get cluster info for additional details
		clusterInfo, err := common.GetElasticsearchClusterInfo(out.elasticsearchCfg.Hosts, out.elasticsearchCfg.Auth)
		if err == nil {
			result["details"].(map[string]interface{})["cluster_info"] = clusterInfo
		}

		// Add producer metrics if available
		if out.elasticsearchProducer != nil {
			result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
				"produce_total":   out.GetProduceTotal(),
				"producer_active": true,
				"batch_size":      out.elasticsearchCfg.BatchSize,
			}
		} else {
			result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
				"producer_active": false,
			}
		}

	case OutputTypePrint:
		// Print output doesn't require external connectivity testing
		result["status"] = "success"
		result["message"] = "Print output is ready (no external connection required)"
		result["details"].(map[string]interface{})["connection_status"] = "not_applicable"
		result["details"].(map[string]interface{})["connection_info"] = map[string]interface{}{
			"type":        "console_output",
			"description": "Print output writes directly to console and doesn't require external connectivity",
		}
		result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{}
		result["details"].(map[string]interface{})["connection_warnings"] = []map[string]interface{}{
			{"message": "Connection check is not applicable for print output type", "severity": "info"},
		}
		return result

	case OutputTypeAliyunSLS:
		if out.aliyunSLSCfg == nil {
			result["status"] = "error"
			result["message"] = "Aliyun SLS configuration missing"
			result["details"].(map[string]interface{})["connection_status"] = "not_configured"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": "Aliyun SLS configuration is incomplete or missing", "severity": "error"},
			}
			return result
		}

		// Set connection info (without sensitive credentials)
		connectionInfo := map[string]interface{}{
			"endpoint": out.aliyunSLSCfg.Endpoint,
			"project":  out.aliyunSLSCfg.Project,
			"logstore": out.aliyunSLSCfg.Logstore,
		}
		result["details"].(map[string]interface{})["connection_info"] = connectionInfo

		// Test actual connectivity to Aliyun SLS
		err := common.TestAliyunSLSConnection(
			out.aliyunSLSCfg.Endpoint,
			out.aliyunSLSCfg.AccessKeyID,
			out.aliyunSLSCfg.AccessKeySecret,
			out.aliyunSLSCfg.Project,
			out.aliyunSLSCfg.Logstore,
		)
		if err != nil {
			result["status"] = "error"
			result["message"] = "Failed to connect to Aliyun SLS"
			result["details"].(map[string]interface{})["connection_status"] = "connection_failed"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": err.Error(), "severity": "error"},
			}
			return result
		}

		// Test if logstore exists
		logstoreExists, err := common.TestAliyunSLSLogstoreExists(
			out.aliyunSLSCfg.Endpoint,
			out.aliyunSLSCfg.AccessKeyID,
			out.aliyunSLSCfg.AccessKeySecret,
			out.aliyunSLSCfg.Project,
			out.aliyunSLSCfg.Logstore,
		)
		if err != nil {
			result["status"] = "warning"
			result["message"] = "Connected to Aliyun SLS but failed to verify logstore"
			result["details"].(map[string]interface{})["connection_status"] = "connected_logstore_unknown"
			result["details"].(map[string]interface{})["connection_warnings"] = []map[string]interface{}{
				{"message": fmt.Sprintf("Could not verify logstore existence: %v", err), "severity": "warning"},
			}
		} else if !logstoreExists {
			result["status"] = "error"
			result["message"] = "Connected to Aliyun SLS but logstore does not exist"
			result["details"].(map[string]interface{})["connection_status"] = "connected_logstore_missing"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": fmt.Sprintf("Logstore '%s' does not exist in project '%s'", out.aliyunSLSCfg.Logstore, out.aliyunSLSCfg.Project), "severity": "error"},
			}
			return result
		} else {
			result["details"].(map[string]interface{})["connection_status"] = "connected"
			result["message"] = "Successfully connected to Aliyun SLS and verified logstore"
		}

		// Get project info for additional details
		projectInfo, err := common.GetAliyunSLSProjectInfo(
			out.aliyunSLSCfg.Endpoint,
			out.aliyunSLSCfg.AccessKeyID,
			out.aliyunSLSCfg.AccessKeySecret,
			out.aliyunSLSCfg.Project,
		)
		if err == nil {
			result["details"].(map[string]interface{})["project_info"] = projectInfo
		}

		// Add metrics if available (note: AliyunSLS output is not fully implemented yet)
		result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
			"produce_total":   out.GetProduceTotal(),
			"producer_active": false, // AliyunSLS output producer not implemented yet
			"note":            "AliyunSLS output producer implementation is pending",
		}

	default:
		result["status"] = "error"
		result["message"] = "Unsupported output type"
		result["details"].(map[string]interface{})["connection_status"] = "unsupported"
	}

	return result
}

// NewFromExisting creates a new Output instance from an existing one with a different ProjectNodeSequence
// This is used when multiple projects use the same output component but with different data flow sequences
func NewFromExisting(existing *Output, newProjectNodeSequence string) (*Output, error) {
	if existing == nil {
		return nil, fmt.Errorf("existing output is nil")
	}

	// Verify the existing configuration before creating new instance
	err := Verify(existing.Path, existing.Config.RawConfig)
	if err != nil {
		return nil, fmt.Errorf("output verify error for existing config: %s %s", existing.Id, err.Error())
	}

	// Create a new Output instance with the same configuration but different ProjectNodeSequence
	newOutput := &Output{
		Id:                  existing.Id,
		Path:                existing.Path,
		ProjectNodeSequence: newProjectNodeSequence, // Set the new sequence
		Type:                existing.Type,
		UpStream:            make(map[string]*chan map[string]interface{}, 0),
		kafkaCfg:            existing.kafkaCfg,
		elasticsearchCfg:    existing.elasticsearchCfg,
		aliyunSLSCfg:        existing.aliyunSLSCfg,
		Config:              existing.Config,
		Status:              common.StatusStopped, // Initialize status to stopped
		TestCollectionChan:  nil,                  // Reset for new instance
	}

	// Only create sampler on leader node for performance
	if common.IsLeader {
		newOutput.sampler = common.GetSampler("output." + existing.Id)
	}

	return newOutput, nil
}

// SetTestMode configures the output for test mode by disabling sampling and other global state interactions
func (out *Output) SetTestMode() {
	out.sampler = nil // Disable sampling for test instances
}

// GetPendingMessageCount returns the total number of pending messages in all channels
// This includes upstream channels and internal producer channels
func (out *Output) GetPendingMessageCount() int {
	pendingCount := 0

	// Check upstream channels
	for _, upCh := range out.UpStream {
		if upCh != nil {
			pendingCount += len(*upCh)
		}
	}

	// Check internal producer channels based on output type
	switch out.Type {
	case OutputTypeKafka, OutputTypeKafkaAzure, OutputTypeKafkaAWS:
		if out.kafkaProducer != nil && out.kafkaProducer.MsgChan != nil {
			pendingCount += len(out.kafkaProducer.MsgChan)
		}
	case OutputTypeElasticsearch:
		if out.elasticsearchProducer != nil && out.elasticsearchProducer.MsgChan != nil {
			pendingCount += len(out.elasticsearchProducer.MsgChan)
		}
	}

	return pendingCount
}
