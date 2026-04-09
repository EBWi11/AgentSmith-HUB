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
	OutputTypeClickHouse    OutputType = "clickhouse"
	OutputTypePrint         OutputType = "print"
)

// OutputConfig is the YAML config for an output.
type OutputConfig struct {
	Id            string
	Type          OutputType                 `yaml:"type"`
	Kafka         *KafkaOutputConfig         `yaml:"kafka,omitempty"`
	Elasticsearch *ElasticsearchOutputConfig `yaml:"elasticsearch,omitempty"`
	AliyunSLS     *AliyunSLSOutputConfig     `yaml:"aliyun_sls,omitempty"`
	ClickHouse    *ClickHouseOutputConfig    `yaml:"clickhouse,omitempty"`
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
	Idempotent  *bool                       `yaml:"idempotent,omitempty"`
}

// ElasticsearchOutputConfig holds Elasticsearch-specific config.
type ElasticsearchOutputConfig struct {
	Hosts     []string                        `yaml:"hosts"`
	Index     string                          `yaml:"index"`
	Version   string                          `yaml:"version,omitempty"`    // elasticsearch version: v7, v8, v9 (default auto-detect, fallback v8)
	BatchSize int                             `yaml:"batch_size,omitempty"` // batch size per bulk request
	FlushDur  string                          `yaml:"flush_dur,omitempty"`  // flush interval, e.g. "5s"
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

// ClickHouseOutputConfig holds ClickHouse-specific config.
type ClickHouseOutputConfig struct {
	Hosts     []string                     `yaml:"hosts"`
	Database  string                       `yaml:"database"`
	Table     string                       `yaml:"table"`
	BatchSize int                          `yaml:"batch_size,omitempty"`
	FlushDur  string                       `yaml:"flush_dur,omitempty"`
	Auth      *common.ClickHouseAuthConfig `yaml:"auth,omitempty"`
	TLS       *common.ClickHouseTLSConfig  `yaml:"tls,omitempty"`
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
	clickhouseProducer    *common.ClickHouseProducer
	wg                    sync.WaitGroup

	// config cache
	kafkaCfg         *KafkaOutputConfig
	elasticsearchCfg *ElasticsearchOutputConfig
	aliyunSLSCfg     *AliyunSLSOutputConfig
	clickhouseCfg    *ClickHouseOutputConfig

	// metrics - only total count is needed now
	produceTotal      uint64 // cumulative production total
	lastReportedTotal uint64 // For calculating increments in 10-second intervals

	// sampler
	sampler *common.Sampler

	// for stopping goroutines - unified stop signal for all output types
	stopChan chan struct{}

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
	case OutputTypeClickHouse:
		if cfg.ClickHouse == nil {
			return fmt.Errorf("missing required field 'clickhouse' for clickhouse output (line: unknown)")
		}
		if len(cfg.ClickHouse.Hosts) == 0 {
			return fmt.Errorf("missing required field 'clickhouse.hosts' for clickhouse output (line: unknown)")
		}
		if cfg.ClickHouse.Database == "" {
			return fmt.Errorf("missing required field 'clickhouse.database' for clickhouse output (line: unknown)")
		}
		if cfg.ClickHouse.Table == "" {
			return fmt.Errorf("missing required field 'clickhouse.table' for clickhouse output (line: unknown)")
		}
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
		clickhouseCfg:    cfg.ClickHouse,
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
	// Note: stopChan is already closed in Stop() method, so we don't close it here

	// Stop producers (idempotent operations)
	if out.kafkaProducer != nil {
		out.kafkaProducer.Close()
		out.kafkaProducer = nil
	}

	if out.elasticsearchProducer != nil {
		out.elasticsearchProducer.Close()
		out.elasticsearchProducer = nil
	}

	if out.clickhouseProducer != nil {
		out.clickhouseProducer.Close()
		out.clickhouseProducer = nil
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
	// Create a deep copy of the original message to avoid concurrent map access issues
	enhancedMsg := common.MapDeepCopy(msg)

	// Add ProjectNodeSequence information
	enhancedMsg["_hub_project_node_sequence"] = out.ProjectNodeSequence
	enhancedMsg["_hub_output_timestamp"] = time.Now().UTC().Format(time.RFC3339)

	return enhancedMsg
}

func (out *Output) ensureStopChan() chan struct{} {
	if out.stopChan == nil {
		out.stopChan = make(chan struct{})
	}
	return out.stopChan
}

func (out *Output) processOutputMessage(msg map[string]interface{}, hasTestCollector bool, testCollectorType string) map[string]interface{} {
	atomic.AddUint64(&out.produceTotal, 1)

	if out.sampler != nil {
		out.sampler.Sample(msg, out.ProjectNodeSequence)
	}

	enhancedMsg := out.enhanceMessageWithProjectNodeSequence(msg)

	if hasTestCollector && out.TestCollectionChan != nil {
		select {
		case *out.TestCollectionChan <- enhancedMsg:
		default:
			logger.Error("Test collection channel full, dropping message", "id", out.Id, "type", testCollectorType)
		}
	}

	return enhancedMsg
}

func (out *Output) startProducerBridge(msgChan chan map[string]interface{}, producerType string, hasTestCollector bool) {
	stopCh := out.ensureStopChan()
	var forwarderWG sync.WaitGroup

	for _, up := range out.UpStream {
		forwarderWG.Add(1)
		out.wg.Add(1)
		go func(up *chan map[string]interface{}) {
			defer out.wg.Done()
			defer forwarderWG.Done()

			for {
				select {
				case <-stopCh:
					return
				case msg, ok := <-*up:
					if !ok {
						return
					}

					enhancedMsg := out.processOutputMessage(msg, hasTestCollector, producerType)
					select {
					case <-stopCh:
						return
					case msgChan <- enhancedMsg:
					}
				}
			}
		}(up)
	}

	out.wg.Add(1)
	go func() {
		defer out.wg.Done()
		forwarderWG.Wait()
		close(msgChan)
	}()
}

func (out *Output) startPrintBridge(hasTestCollector bool) {
	stopCh := out.ensureStopChan()
	for _, up := range out.UpStream {
		out.wg.Add(1)
		go func(up *chan map[string]interface{}) {
			defer out.wg.Done()
			for {
				select {
				case <-stopCh:
					return
				case msg, ok := <-*up:
					if !ok {
						return
					}
					enhancedMsg := out.processOutputMessage(msg, hasTestCollector, "print")
					data, _ := json.Marshal(enhancedMsg)
					logger.Info("[Print Output]", "data", string(data))
				}
			}
		}(up)
	}
}

func (out *Output) startTestingBridge() {
	stopCh := out.ensureStopChan()
	for _, up := range out.UpStream {
		out.wg.Add(1)
		go func(up *chan map[string]interface{}) {
			defer out.wg.Done()
			for {
				select {
				case <-stopCh:
					logger.Debug("Testing output goroutine received stop signal", "id", out.Id)
					return
				case msg, ok := <-*up:
					if !ok {
						return
					}

					enhancedMsg := out.processOutputMessage(msg, false, "testing")
					if out.TestCollectionChan != nil {
						select {
						case *out.TestCollectionChan <- enhancedMsg:
						default:
							logger.Error("Test collection channel full, dropping message", "id", out.Id, "type", "testing")
						}
					}
				}
			}
		}(up)
	}
}

// StartForTesting starts the output component in testing mode
// In testing mode, completely ignore output type and only send data to TestCollectionChan
func (out *Output) StartForTesting() error {
	if out.Status != common.StatusStopped {
		return fmt.Errorf("output %s is not stopped", out.Id)
	}

	out.ResetProduceTotal()
	out.SetStatus(common.StatusStarting, nil)

	// Initialize stop channel for testing
	out.stopChan = make(chan struct{})
	out.startTestingBridge()

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

		msgChan := make(chan map[string]interface{}, common.PipelineOutputBuffer)
		producer, err := common.NewKafkaProducer(
			out.kafkaCfg.Brokers,
			out.kafkaCfg.Topic,
			out.kafkaCfg.Compression,
			out.kafkaCfg.SASL,
			msgChan,
			out.kafkaCfg.Key,
			out.kafkaCfg.TLS,
			// default idempotent true if not specified
			(out.kafkaCfg.Idempotent == nil) || (out.kafkaCfg.Idempotent != nil && *out.kafkaCfg.Idempotent),
		)
		if err != nil {
			out.SetStatus(common.StatusError, fmt.Errorf("failed to create kafka producer for output %s: %v", out.Id, err))
			return fmt.Errorf("failed to create kafka producer for output %s: %v", out.Id, err)
		}
		out.kafkaProducer = producer

		out.stopChan = make(chan struct{})
		out.startProducerBridge(msgChan, "kafka", hasTestCollector)

	case OutputTypeElasticsearch:
		if out.elasticsearchProducer != nil {
			out.SetStatus(common.StatusError, fmt.Errorf("elasticsearch producer already running for output %s", out.Id))
			return fmt.Errorf("elasticsearch producer already running for output %s", out.Id)
		}
		if out.elasticsearchCfg == nil {
			out.SetStatus(common.StatusError, fmt.Errorf("elasticsearch configuration missing for output %s", out.Id))
			return fmt.Errorf("elasticsearch configuration missing for output %s", out.Id)
		}

		msgChan := make(chan map[string]interface{}, common.PipelineOutputBuffer)
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
			out.elasticsearchCfg.Version,
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

		out.ensureStopChan()

		upstreamCount := len(out.UpStream)
		logger.Info("Elasticsearch output starting", "output", out.Id, "index", out.elasticsearchCfg.Index, "upstream_count", upstreamCount)
		if upstreamCount == 0 {
			logger.Error("Elasticsearch output has no upstream connections; no data will be written until project connects input/ruleset/agent to this output", "output", out.Id)
		}

		out.startProducerBridge(msgChan, "elasticsearch", hasTestCollector)

	case OutputTypePrint:
		out.ensureStopChan()
		out.startPrintBridge(hasTestCollector)

	case OutputTypeClickHouse:
		if out.clickhouseProducer != nil {
			out.SetStatus(common.StatusError, fmt.Errorf("clickhouse producer already running for output %s", out.Id))
			return fmt.Errorf("clickhouse producer already running for output %s", out.Id)
		}
		if out.clickhouseCfg == nil {
			out.SetStatus(common.StatusError, fmt.Errorf("clickhouse configuration missing for output %s", out.Id))
			return fmt.Errorf("clickhouse configuration missing for output %s", out.Id)
		}

		msgChan := make(chan map[string]interface{}, common.PipelineOutputBuffer)
		batchSize := 1000
		if out.clickhouseCfg.BatchSize > 0 {
			batchSize = out.clickhouseCfg.BatchSize
		}
		flushDur := 3 * time.Second
		if out.clickhouseCfg.FlushDur != "" {
			if d, err := time.ParseDuration(out.clickhouseCfg.FlushDur); err == nil {
				flushDur = d
			}
		}
		producer, err := common.NewClickHouseProducer(
			out.clickhouseCfg.Hosts,
			out.clickhouseCfg.Database,
			out.clickhouseCfg.Table,
			msgChan,
			batchSize,
			flushDur,
			out.clickhouseCfg.Auth,
			out.clickhouseCfg.TLS,
		)
		if err != nil {
			out.SetStatus(common.StatusError, fmt.Errorf("failed to create clickhouse producer for output %s: %v", out.Id, err))
			return fmt.Errorf("failed to create clickhouse producer for output %s: %v", out.Id, err)
		}
		out.clickhouseProducer = producer

		out.ensureStopChan()
		out.startProducerBridge(msgChan, "clickhouse", hasTestCollector)

	case OutputTypeAliyunSLS:
		out.SetStatus(common.StatusError, fmt.Errorf("aliyun SLS output not implemented yet"))
		return fmt.Errorf("aliyun SLS output not implemented yet")
	}

	out.SetStatus(common.StatusRunning, nil)
	return nil
}

// Stop stops the output producer and waits for all routines to finish.
func (out *Output) Stop() error {
	if out.Status != common.StatusRunning && out.Status != common.StatusError {
		// Allow stopping from any state for cleanup purposes, but only do actual work if needed
		if out.Status == common.StatusStopped {
			logger.Debug("Output already stopped, skipping stop operation", "output", out.Id)
			return nil
		}
		// For other states (e.g., StatusStarting), proceed with stop to ensure cleanup
		logger.Debug("Stopping output from non-running state", "output", out.Id, "current_status", out.Status)
	}
	out.SetStatus(common.StatusStopping, nil)
	logger.Info("Starting output stop process", "id", out.Id, "type", out.Type, "current_status", out.Status)

	// Step 1: Signal all output goroutines to stop first
	if out.stopChan != nil {
		logger.Debug("Closing stopChan", "id", out.Id)
		close(out.stopChan)
		out.stopChan = nil
	} else {
		logger.Error("stopChan is nil during stop", "id", out.Id)
	}

	// Step 2: Wait for bridge goroutines to finish with timeout and force cleanup if needed
	logger.Info("Waiting for output goroutines to finish", "id", out.Id)
	waitDone := make(chan struct{})
	go func() {
		out.wg.Wait()
		close(waitDone)
	}()

	var stopError error
	select {
	case <-waitDone:
		logger.Info("Output stopped gracefully", "id", out.Id)
	case <-time.After(10 * time.Second): // Increased timeout to allow for network operations and retries
		logger.Error("Timeout waiting for output goroutines, forcing cleanup", "id", out.Id)

		// Try to get more information about pending messages for debugging
		pendingCount := out.GetPendingMessageCount()
		logger.Error("Output stop timeout details", "id", out.Id, "type", out.Type, "pending_messages", pendingCount)

		stopError = fmt.Errorf("timeout waiting for goroutines to finish")
	}

	// Step 3: Close producers after bridge goroutines have stopped sending into producer channels.
	logger.Info("Stopping output producers", "id", out.Id)
	if out.kafkaProducer != nil {
		logger.Debug("Closing kafka producer", "id", out.Id)
		out.kafkaProducer.Close()
		out.kafkaProducer = nil
	}
	if out.elasticsearchProducer != nil {
		logger.Debug("Closing elasticsearch producer", "id", out.Id)
		out.elasticsearchProducer.Close()
		out.elasticsearchProducer = nil
	}
	if out.clickhouseProducer != nil {
		logger.Debug("Closing clickhouse producer", "id", out.Id)
		out.clickhouseProducer.Close()
		out.clickhouseProducer = nil
	}

	// Step 4: Final cleanup to ensure all resources are properly released
	out.cleanup()

	// Set final status based on whether there were any errors during stop
	if stopError != nil {
		out.SetStatus(common.StatusError, fmt.Errorf("stop operation failed: %w", stopError))
		return stopError
	} else {
		out.SetStatus(common.StatusStopped, nil)
		return nil
	}
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
// This method is thread-safe and designed for statistics collection.
// Uses CAS operation to ensure atomicity.
func (out *Output) GetIncrementAndUpdate() uint64 {
	current := atomic.LoadUint64(&out.produceTotal)
	last := atomic.LoadUint64(&out.lastReportedTotal)

	// Use CAS to atomically update lastReportedTotal
	// If CAS fails, we simply return 0 - one missed stat collection is not critical
	if atomic.CompareAndSwapUint64(&out.lastReportedTotal, last, current) {
		return current - last
	}
	return 0
}

// StopForTesting stops the output quickly for testing purposes without waiting for channel drainage
func (out *Output) StopForTesting() error {
	logger.Info("Quick stopping test output", "id", out.Id, "type", out.Type)

	// Step 1: Signal goroutines to stop by closing stopChan channel
	if out.stopChan != nil {
		close(out.stopChan)
		out.stopChan = nil
	}

	// Step 2: Clear test collection channel
	out.TestCollectionChan = nil

	// Step 3: Wait for goroutines with very short timeout
	waitDone := make(chan struct{})
	go func() {
		out.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		logger.Info("Test output stopped successfully", "id", out.Id)
	case <-time.After(1 * time.Second): // Very short timeout for testing
		logger.Error("Timeout waiting for test output goroutines, proceeding anyway", "id", out.Id)
	}

	// Step 4: Reset atomic counter for testing cleanup
	previousTotal := atomic.LoadUint64(&out.produceTotal)
	atomic.StoreUint64(&out.produceTotal, 0)
	atomic.StoreUint64(&out.lastReportedTotal, 0)
	logger.Debug("Reset atomic counter for test output component", "output", out.Id, "previous_total", previousTotal)

	// Step 5: Clear component channel connections to prevent leaks
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
			"hosts":   out.elasticsearchCfg.Hosts,
			"index":   out.elasticsearchCfg.Index,
			"version": common.NormalizeElasticsearchVersionForDisplay(out.elasticsearchCfg.Version),
		}
		result["details"].(map[string]interface{})["connection_info"] = connectionInfo

		// Test actual connectivity to Elasticsearch cluster (respect configured version)
		err := common.TestElasticsearchConnection(out.elasticsearchCfg.Hosts, out.elasticsearchCfg.Version, out.elasticsearchCfg.Auth)
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
		indexExists, err := common.TestElasticsearchIndexExists(out.elasticsearchCfg.Hosts, out.elasticsearchCfg.Index, out.elasticsearchCfg.Version, out.elasticsearchCfg.Auth)
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
		clusterInfo, err := common.GetElasticsearchClusterInfo(out.elasticsearchCfg.Hosts, out.elasticsearchCfg.Version, out.elasticsearchCfg.Auth)
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

	case OutputTypeClickHouse:
		if out.clickhouseCfg == nil {
			result["status"] = "error"
			result["message"] = "ClickHouse configuration missing"
			result["details"].(map[string]interface{})["connection_status"] = "not_configured"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": "ClickHouse configuration is incomplete or missing", "severity": "error"},
			}
			return result
		}

		// Set connection info
		connectionInfo := map[string]interface{}{
			"hosts":    out.clickhouseCfg.Hosts,
			"database": out.clickhouseCfg.Database,
			"table":    out.clickhouseCfg.Table,
		}
		result["details"].(map[string]interface{})["connection_info"] = connectionInfo

		// Test actual connectivity to ClickHouse
		err := common.TestClickHouseConnection(out.clickhouseCfg.Hosts, out.clickhouseCfg.Auth, out.clickhouseCfg.TLS)
		if err != nil {
			result["status"] = "error"
			result["message"] = "Failed to connect to ClickHouse"
			result["details"].(map[string]interface{})["connection_status"] = "connection_failed"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": err.Error(), "severity": "error"},
			}
			return result
		}

		// Test if table exists
		tableExists, err := common.TestClickHouseTableExists(out.clickhouseCfg.Hosts, out.clickhouseCfg.Database, out.clickhouseCfg.Table, out.clickhouseCfg.Auth, out.clickhouseCfg.TLS)
		if err != nil {
			result["status"] = "warning"
			result["message"] = "Connected to ClickHouse but failed to verify table"
			result["details"].(map[string]interface{})["connection_status"] = "connected_table_unknown"
			result["details"].(map[string]interface{})["connection_warnings"] = []map[string]interface{}{
				{"message": fmt.Sprintf("Could not verify table existence: %v", err), "severity": "warning"},
			}
		} else if !tableExists {
			result["status"] = "error"
			result["message"] = "Connected to ClickHouse but table does not exist"
			result["details"].(map[string]interface{})["connection_status"] = "connected_table_missing"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": fmt.Sprintf("Table '%s.%s' does not exist", out.clickhouseCfg.Database, out.clickhouseCfg.Table), "severity": "error"},
			}
			return result
		} else {
			result["details"].(map[string]interface{})["connection_status"] = "connected"
			result["message"] = "Successfully connected to ClickHouse and verified table"
		}

		// Get server info for additional details
		serverInfo, err := common.GetClickHouseServerInfo(out.clickhouseCfg.Hosts, out.clickhouseCfg.Auth, out.clickhouseCfg.TLS)
		if err == nil {
			result["details"].(map[string]interface{})["server_info"] = serverInfo
		}

		// Add producer metrics if available
		if out.clickhouseProducer != nil {
			result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
				"produce_total":   out.GetProduceTotal(),
				"producer_active": true,
				"batch_size":      out.clickhouseCfg.BatchSize,
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
		clickhouseCfg:       existing.clickhouseCfg,
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
	case OutputTypeClickHouse:
		if out.clickhouseProducer != nil && out.clickhouseProducer.MsgChan != nil {
			pendingCount += len(out.clickhouseProducer.MsgChan)
		}
	}

	return pendingCount
}
