package input

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// InputType defines the type of input source.
type InputType string

const (
	InputTypeKafka      InputType = "kafka"
	InputTypeKafkaAzure InputType = "kafka_azure"
	InputTypeKafkaAWS   InputType = "kafka_aws"
	InputTypeAliyunSLS  InputType = "aliyun_sls"
)

// InputConfig is the YAML config for an input.
type InputConfig struct {
	Id        string
	Type      InputType             `yaml:"type"`
	Kafka     *KafkaInputConfig     `yaml:"kafka,omitempty"`
	AliyunSLS *AliyunSLSInputConfig `yaml:"aliyun_sls,omitempty"`
	RawConfig string
}

// KafkaInputConfig holds Kafka-specific config.
type KafkaInputConfig struct {
	Brokers     []string                    `yaml:"brokers"`
	Group       string                      `yaml:"group"`
	Topic       string                      `yaml:"topic"`
	Compression common.KafkaCompressionType `yaml:"compression,omitempty"`
	SASL        *common.KafkaSASLConfig     `yaml:"sasl,omitempty"`
	TLS         *common.KafkaTLSConfig      `yaml:"tls,omitempty"`
}

// AliyunSLSInputConfig holds Aliyun SLS-specific config.
type AliyunSLSInputConfig struct {
	Endpoint          string `yaml:"endpoint"`
	AccessKeyID       string `yaml:"access_key_id"`
	AccessKeySecret   string `yaml:"access_key_secret"`
	Project           string `yaml:"project"`
	Logstore          string `yaml:"logstore"`
	ConsumerGroupName string `yaml:"consumer_group_name"`
	ConsumerName      string `yaml:"consumer_name"`
	CursorPosition    string `yaml:"cursor_position,omitempty"`   // begin, end, or specific timestamp
	CursorStartTime   int64  `yaml:"cursor_start_time,omitempty"` // Unix timestamp in milliseconds
	Query             string `yaml:"query,omitempty"`             // Optional query for filtering logs
}

// Input represents an input component that consumes data from external sources
type Input struct {
	Id                  string `json:"Id"`
	Path                string
	ProjectNodeSequence string
	Type                InputType
	DownStream          []*chan map[string]interface{}

	// runtime
	kafkaConsumer *common.KafkaConsumer
	slsConsumer   *common.AliyunSLSConsumer

	// config cache
	kafkaCfg     *KafkaInputConfig
	aliyunSLSCfg *AliyunSLSInputConfig

	consumeTotal uint64
	consumeQPS   uint64
	metricStop   chan struct{}

	// sampler
	sampler *common.Sampler

	// raw config
	Config *InputConfig

	// goroutine management
	wg       sync.WaitGroup
	stopChan chan struct{}

	// List of project IDs that share this input instance (for per-project metrics)
	OwnerProjects []string `json:"-"`
}

func Verify(path string, raw string) error {
	var cfg InputConfig

	// Use common file reading function
	data, err := common.ReadContentFromPathOrRaw(path, raw)
	if err != nil {
		return fmt.Errorf("failed to read input configuration: %w", err)
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
			return fmt.Errorf("failed to parse input configuration: %s (location: %s)", errMsg, lineInfo)
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
	case InputTypeKafka, InputTypeKafkaAzure, InputTypeKafkaAWS:
		if cfg.Kafka == nil {
			return fmt.Errorf("missing required field 'kafka' for kafka input (line: unknown)")
		}
		if len(cfg.Kafka.Brokers) == 0 {
			return fmt.Errorf("missing required field 'kafka.brokers' for kafka input (line: unknown)")
		}
		if cfg.Kafka.Topic == "" {
			return fmt.Errorf("missing required field 'kafka.topic' for kafka input (line: unknown)")
		}
	case InputTypeAliyunSLS:
		if cfg.AliyunSLS == nil {
			return fmt.Errorf("missing required field 'aliyun_sls' for aliyunSLS input (line: unknown)")
		}
		// Add more AliyunSLS specific field validation
	default:
		return fmt.Errorf("unsupported input type: %s (line: unknown)", cfg.Type)
	}

	return nil
}

// NewInput creates an Input from config and downstreams.
func NewInput(path string, raw string, id string) (*Input, error) {
	var cfg InputConfig

	err := Verify(path, raw)
	if err != nil {
		return nil, fmt.Errorf("input verify error: %s %s", id, err.Error())
	}

	if path != "" {
		data, _ := os.ReadFile(path)
		_ = yaml.Unmarshal(data, &cfg)
		cfg.RawConfig = string(data)
	} else {
		_ = yaml.Unmarshal([]byte(raw), &cfg)
		cfg.RawConfig = raw
	}

	in := &Input{
		Id:           id,
		Path:         path,
		Type:         cfg.Type,
		DownStream:   make([]*chan map[string]interface{}, 0),
		kafkaCfg:     cfg.Kafka,
		aliyunSLSCfg: cfg.AliyunSLS,
		Config:       &cfg,
		sampler:      nil, // Will be set below based on cluster role
	}

	// Only create sampler on leader node for performance
	if cluster.IsLeader {
		in.sampler = common.GetSampler("input." + id)
	}
	return in, nil
}

// Start initializes and starts the input component based on its type
// Returns an error if the component is already running or if initialization fails
func (in *Input) Start() error {
	// Initialize stop channel
	in.stopChan = make(chan struct{})

	// Start metric goroutine
	in.metricStop = make(chan struct{})
	in.wg.Add(1)
	go func() {
		defer in.wg.Done()
		in.metricLoop()
	}()

	switch in.Type {
	case InputTypeKafka, InputTypeKafkaAzure, InputTypeKafkaAWS:
		if in.kafkaConsumer != nil {
			return fmt.Errorf("kafka consumer already running for input %s", in.Id)
		}
		if in.kafkaCfg == nil {
			return fmt.Errorf("kafka configuration missing for input %s", in.Id)
		}
		msgChan := make(chan map[string]interface{}, 1024)
		cons, err := common.NewKafkaConsumer(
			in.kafkaCfg.Brokers,
			in.kafkaCfg.Group,
			in.kafkaCfg.Topic,
			in.kafkaCfg.Compression,
			in.kafkaCfg.SASL,
			in.kafkaCfg.TLS,
			msgChan,
		)
		if err != nil {
			return fmt.Errorf("failed to create kafka consumer for input %s: %v", in.Id, err)
		}
		in.kafkaConsumer = cons

		// Start consumer goroutine with proper management
		in.wg.Add(1)
		go func() {
			defer in.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Panic in kafka consumer goroutine", "input", in.Id, "panic", r)
				}
			}()

			for {
				select {
				case <-in.stopChan:
					logger.Info("Kafka consumer goroutine stopping", "input", in.Id)
					return
				case msg, ok := <-msgChan:
					if !ok {
						logger.Info("Kafka message channel closed", "input", in.Id)
						return
					}

					// Optimization: only increment total count, QPS is calculated by metricLoop
					atomic.AddUint64(&in.consumeTotal, 1)

					// Sample the message
					if in.sampler != nil {
						pid := ""
						if len(in.OwnerProjects) > 0 {
							pid = in.OwnerProjects[0]
						}
						in.sampler.Sample(msg, in.ProjectNodeSequence, pid)
					}

					// Add input ID to message data
					if msg == nil {
						msg = make(map[string]interface{})
					}
					msg["_hub_input"] = in.Id

					// Forward to downstream
					for _, ch := range in.DownStream {
						*ch <- msg
					}
				}
			}
		}()

	case InputTypeAliyunSLS:
		if in.slsConsumer != nil {
			return fmt.Errorf("sls consumer already running for input %s", in.Id)
		}
		if in.aliyunSLSCfg == nil {
			return fmt.Errorf("sls configuration missing for input %s", in.Id)
		}

		msgChan := make(chan map[string]interface{}, 1024)
		cons, err := common.NewAliyunSLSConsumer(
			in.aliyunSLSCfg.Endpoint,
			in.aliyunSLSCfg.AccessKeyID,
			in.aliyunSLSCfg.AccessKeySecret,
			in.aliyunSLSCfg.Project,
			in.aliyunSLSCfg.Logstore,
			in.aliyunSLSCfg.ConsumerGroupName,
			in.aliyunSLSCfg.ConsumerName,
			in.aliyunSLSCfg.CursorPosition,
			in.aliyunSLSCfg.CursorStartTime,
			in.aliyunSLSCfg.Query,
			msgChan,
		)
		if err != nil {
			return fmt.Errorf("failed to create sls consumer for input %s: %v", in.Id, err)
		}
		in.slsConsumer = cons

		cons.Start()

		// Start consumer goroutine with proper management
		in.wg.Add(1)
		go func() {
			defer in.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Panic in sls consumer goroutine", "input", in.Id, "panic", r)
				}
			}()

			for {
				select {
				case <-in.stopChan:
					logger.Info("SLS consumer goroutine stopping", "input", in.Id)
					return
				case msg, ok := <-msgChan:
					if !ok {
						logger.Info("SLS message channel closed", "input", in.Id)
						return
					}

					atomic.AddUint64(&in.consumeTotal, 1)

					// Sample the message
					if in.sampler != nil {
						pid := ""
						if len(in.OwnerProjects) > 0 {
							pid = in.OwnerProjects[0]
						}
						in.sampler.Sample(msg, in.ProjectNodeSequence, pid)
					}

					// Add input ID to message data
					if msg == nil {
						msg = make(map[string]interface{})
					}
					msg["_hub_input"] = in.Id

					// Forward to downstream
					for _, ch := range in.DownStream {
						*ch <- msg
					}
				}
			}
		}()

	default:
		return fmt.Errorf("unsupported input type %s", in.Type)
	}

	return nil
}

// Stop stops the input component and its consumers
func (in *Input) Stop() error {
	logger.Info("Stopping input", "id", in.Id, "type", in.Type)

	// Signal all goroutines to stop
	if in.stopChan != nil {
		close(in.stopChan)
	}

	// First, stop the consumers to prevent new messages
	if in.kafkaConsumer != nil {
		in.kafkaConsumer.Close()
		in.kafkaConsumer = nil
	}

	if in.slsConsumer != nil {
		if err := in.slsConsumer.Close(); err != nil {
			logger.Warn("Failed to close sls consumer", "input", in.Id, "error", err)
		}
		in.slsConsumer = nil
	}

	// Stop metrics collection
	if in.metricStop != nil {
		close(in.metricStop)
		in.metricStop = nil
	}

	// Wait for all goroutines to finish with timeout
	waitDone := make(chan struct{})
	go func() {
		in.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		logger.Info("Input stopped gracefully", "id", in.Id)
	case <-time.After(10 * time.Second): // Reduced timeout for faster shutdown
		logger.Warn("Input stop timeout, some goroutines may still be running", "id", in.Id)
	}

	// Reset stop channel for potential restart
	in.stopChan = nil

	return nil
}

// metricLoop calculates QPS and can be extended for more metrics.
func (in *Input) metricLoop() {
	var lastTotal uint64
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-in.metricStop:
			return
		case <-ticker.C:
			cur := atomic.LoadUint64(&in.consumeTotal)

			var qps uint64
			// Safe handling: if current value is less than last value, set QPS to 0
			if cur < lastTotal {
				logger.Warn("Counter decreased, possibly due to overflow or restart",
					"input", in.Id,
					"lastTotal", lastTotal,
					"currentTotal", cur)
				qps = 0         // Set QPS to 0 to avoid underflow
				lastTotal = cur // Reset lastTotal to current value
			} else {
				qps = cur - lastTotal
				lastTotal = cur
			}

			atomic.StoreUint64(&in.consumeQPS, qps)

			// Persist total messages per project & sequence to Redis
			if in.ProjectNodeSequence != "" {
				if len(in.OwnerProjects) == 0 {
					// Fallback for legacy instances
					key := "msg_total:" + in.ProjectNodeSequence + ":input"
					_, _ = common.RedisIncrby(key, int64(qps))
				} else {
					for _, pid := range in.OwnerProjects {
						key := fmt.Sprintf("msg_total:%s:%s:input", pid, in.ProjectNodeSequence)
						_, _ = common.RedisIncrby(key, int64(qps))
					}
				}
			}
		}
	}
}

// GetConsumeQPS returns the latest QPS.
func (in *Input) GetConsumeQPS() uint64 {
	return atomic.LoadUint64(&in.consumeQPS)
}

// GetConsumeTotal returns the total consumed count.
func (in *Input) GetConsumeTotal() uint64 {
	return atomic.LoadUint64(&in.consumeTotal)
}

// CheckConnectivity performs a real connectivity test for the input component
// This method tests actual connection to external systems (Kafka, SLS, etc.)
func (in *Input) CheckConnectivity() map[string]interface{} {
	result := map[string]interface{}{
		"status":  "success",
		"message": "Connection check successful",
		"details": map[string]interface{}{
			"client_type":         string(in.Type),
			"connection_status":   "unknown",
			"connection_info":     map[string]interface{}{},
			"connection_errors":   []map[string]interface{}{},
			"connection_warnings": []map[string]interface{}{},
		},
	}

	switch in.Type {
	case InputTypeKafka, InputTypeKafkaAzure, InputTypeKafkaAWS:
		if in.kafkaCfg == nil {
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
			"brokers": in.kafkaCfg.Brokers,
			"topic":   in.kafkaCfg.Topic,
			"group":   in.kafkaCfg.Group,
		}
		result["details"].(map[string]interface{})["connection_info"] = connectionInfo

		// Test actual connectivity to Kafka brokers
		err := common.TestKafkaConnection(in.kafkaCfg.Brokers, in.kafkaCfg.SASL, in.kafkaCfg.TLS)
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
		topicExists, err := common.TestKafkaTopicExists(in.kafkaCfg.Brokers, in.kafkaCfg.Topic, in.kafkaCfg.SASL, in.kafkaCfg.TLS)
		if err != nil {
			result["status"] = "warning"
			result["message"] = "Connected to Kafka but failed to verify topic"
			result["details"].(map[string]interface{})["connection_status"] = "connected_topic_unknown"
			result["details"].(map[string]interface{})["connection_warnings"] = []map[string]interface{}{
				{"message": fmt.Sprintf("Could not verify topic existence: %v", err), "severity": "warning"},
			}
		} else if !topicExists {
			result["status"] = "error"
			result["message"] = "Connected to Kafka but topic does not exist"
			result["details"].(map[string]interface{})["connection_status"] = "connected_topic_missing"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": fmt.Sprintf("Topic '%s' does not exist", in.kafkaCfg.Topic), "severity": "error"},
			}
		} else {
			result["details"].(map[string]interface{})["connection_status"] = "connected"
			result["message"] = "Successfully connected to Kafka and verified topic"
		}

		// Add consumer metrics if available
		if in.kafkaConsumer != nil {
			result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
				"consume_qps":     in.GetConsumeQPS(),
				"consume_total":   in.GetConsumeTotal(),
				"consumer_active": true,
			}
		} else {
			result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
				"consumer_active": false,
			}
		}

	case InputTypeAliyunSLS:
		if in.aliyunSLSCfg == nil {
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
			"endpoint":       in.aliyunSLSCfg.Endpoint,
			"project":        in.aliyunSLSCfg.Project,
			"logstore":       in.aliyunSLSCfg.Logstore,
			"consumer_group": in.aliyunSLSCfg.ConsumerGroupName,
		}
		result["details"].(map[string]interface{})["connection_info"] = connectionInfo

		// Test actual connectivity to Aliyun SLS
		err := common.TestAliyunSLSConnection(
			in.aliyunSLSCfg.Endpoint,
			in.aliyunSLSCfg.AccessKeyID,
			in.aliyunSLSCfg.AccessKeySecret,
			in.aliyunSLSCfg.Project,
			in.aliyunSLSCfg.Logstore,
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

		// Connection successful, now test if logstore exists
		logstoreExists, err := common.TestAliyunSLSLogstoreExists(
			in.aliyunSLSCfg.Endpoint,
			in.aliyunSLSCfg.AccessKeyID,
			in.aliyunSLSCfg.AccessKeySecret,
			in.aliyunSLSCfg.Project,
			in.aliyunSLSCfg.Logstore,
		)
		if err != nil {
			// Check if it's a permission issue
			if strings.Contains(err.Error(), "insufficient permissions") {
				result["status"] = "success"
				result["message"] = "Connected to Aliyun SLS (logstore verification limited by permissions)"
				result["details"].(map[string]interface{})["connection_status"] = "connected_limited_permissions"
				result["details"].(map[string]interface{})["connection_warnings"] = []map[string]interface{}{
					{"message": err.Error(), "severity": "info"},
				}
			} else {
				result["status"] = "warning"
				result["message"] = "Connected to Aliyun SLS but failed to verify logstore"
				result["details"].(map[string]interface{})["connection_status"] = "connected_logstore_unknown"
				result["details"].(map[string]interface{})["connection_warnings"] = []map[string]interface{}{
					{"message": fmt.Sprintf("Could not verify logstore existence: %v", err), "severity": "warning"},
				}
			}
		} else if !logstoreExists {
			result["status"] = "error"
			result["message"] = "Connected to Aliyun SLS but logstore does not exist"
			result["details"].(map[string]interface{})["connection_status"] = "connected_logstore_missing"
			result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
				{"message": fmt.Sprintf("Logstore '%s' does not exist in project '%s'", in.aliyunSLSCfg.Logstore, in.aliyunSLSCfg.Project), "severity": "error"},
			}
			return result
		} else {
			result["details"].(map[string]interface{})["connection_status"] = "connected"
			result["message"] = "Successfully connected to Aliyun SLS and verified logstore"
		}

		// Try to get project info for additional details (this might fail due to permissions)
		projectInfo, err := common.GetAliyunSLSProjectInfo(
			in.aliyunSLSCfg.Endpoint,
			in.aliyunSLSCfg.AccessKeyID,
			in.aliyunSLSCfg.AccessKeySecret,
			in.aliyunSLSCfg.Project,
		)
		if err == nil {
			result["details"].(map[string]interface{})["project_info"] = projectInfo
		} else {
			// Don't fail the connection check just because we can't get project info
			// This is likely a permission issue, not a connectivity issue
			if strings.Contains(err.Error(), "Unauthorized") || strings.Contains(err.Error(), "denied by sts or ram") {
				result["details"].(map[string]interface{})["project_info"] = map[string]interface{}{
					"note": "Project info unavailable due to limited permissions",
				}
			}
		}

		// Add consumer metrics if available
		if in.slsConsumer != nil {
			result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
				"consume_qps":     in.GetConsumeQPS(),
				"consume_total":   in.GetConsumeTotal(),
				"consumer_active": true,
			}
		} else {
			result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
				"consumer_active": false,
			}
		}

	default:
		result["status"] = "error"
		result["message"] = "Unsupported input type"
		result["details"].(map[string]interface{})["connection_status"] = "unsupported"
	}

	return result
}
