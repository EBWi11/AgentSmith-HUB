package input

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// InputType defines the type of input source.
type InputType string

const (
	InputTypeKafka     InputType = "kafka"
	InputTypeAliyunSLS InputType = "aliyun_sls"
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

	// metrics
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
}

func Verify(path string, raw string) error {
	var cfg InputConfig
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			// 从错误信息中提取行号
			if yamlErr, ok := err.(*yaml.TypeError); ok && len(yamlErr.Errors) > 0 {
				errMsg := yamlErr.Errors[0]
				// 尝试提取行号
				lineInfo := ""
				for _, line := range yamlErr.Errors {
					if strings.Contains(line, "line") {
						lineInfo = line
						break
					}
				}
				return fmt.Errorf("YAML parse error: %s (location: %s)", errMsg, lineInfo)
			}
			return fmt.Errorf("YAML parse error: %v", err)
		}
	} else {
		if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
			// 从错误信息中提取行号
			if yamlErr, ok := err.(*yaml.TypeError); ok && len(yamlErr.Errors) > 0 {
				errMsg := yamlErr.Errors[0]
				// 尝试提取行号
				lineInfo := ""
				for _, line := range yamlErr.Errors {
					if strings.Contains(line, "line") {
						lineInfo = line
						break
					}
				}
				return fmt.Errorf("YAML parse error: %s (location: %s)", errMsg, lineInfo)
			}
			return fmt.Errorf("YAML parse error: %v", err)
		}
	}

	// 验证必要字段
	if cfg.Type == "" {
		return fmt.Errorf("missing required field 'type' (line: unknown)")
	}

	// 根据类型验证特定字段
	switch cfg.Type {
	case InputTypeKafka:
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
		// 添加更多AliyunSLS特定字段验证
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
		sampler:      common.GetSampler("input." + id),
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
	case InputTypeKafka:
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

					atomic.AddUint64(&in.consumeTotal, 1)
					atomic.AddUint64(&in.consumeQPS, 1)

					// Sample the message
					if in.sampler != nil {
						in.sampler.Sample(msg, "kafka", in.ProjectNodeSequence)
					}

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
					atomic.AddUint64(&in.consumeQPS, 1)

					// Sample the message
					if in.sampler != nil {
						in.sampler.Sample(msg, "sls", in.ProjectNodeSequence)
					}

					// Forward to downstream
					for _, ch := range in.DownStream {
						*ch <- msg
					}
				}
			}
		}()
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
			atomic.StoreUint64(&in.consumeQPS, cur-lastTotal)
			lastTotal = cur
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
