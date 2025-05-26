package input

import (
	"AgentSmith-HUB/common"
	"fmt"
	"os"
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
	Name      string `yaml:"name"`
	Id        string
	Type      InputType             `yaml:"type"`
	Kafka     *KafkaInputConfig     `yaml:"kafka,omitempty"`
	AliyunSLS *AliyunSLSInputConfig `yaml:"aliyun_sls,omitempty"`
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

// Input is the runtime input instance.
type Input struct {
	Name       string
	Id         string
	Type       InputType
	DownStream []*chan map[string]interface{}

	// runtime
	kafkaConsumer *common.KafkaConsumer
	slsConsumer   *common.AliyunSLSConsumer
	wg            sync.WaitGroup

	// config cache
	kafkaCfg     *KafkaInputConfig
	aliyunSLSCfg *AliyunSLSInputConfig

	// metrics
	consumeTotal uint64
	consumeQPS   uint64
	metricStop   chan struct{}
}

// LoadInputConfig loads input config from a YAML file.
func LoadInputConfig(path string) (*InputConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg InputConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// NewInput creates an Input from config and downstreams.
func NewInput(path string, id string) (*Input, error) {
	cfg, err := LoadInputConfig(path)
	if err != nil {
		return nil, err
	}

	in := &Input{
		Name:         cfg.Name,
		Id:           id,
		Type:         cfg.Type,
		DownStream:   make([]*chan map[string]interface{}, 0),
		kafkaCfg:     cfg.Kafka,
		aliyunSLSCfg: cfg.AliyunSLS,
	}
	return in, nil
}

// Start launches the input consumer and pushes data to downstream.
func (in *Input) Start() error {
	// Start metric goroutine
	in.metricStop = make(chan struct{})
	go in.metricLoop()

	switch in.Type {
	case InputTypeKafka:
		if in.kafkaConsumer != nil {
			return fmt.Errorf("kafka consumer already started")
		}
		if in.kafkaCfg == nil {
			return fmt.Errorf("kafka config missing")
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
			return err
		}
		in.kafkaConsumer = cons
		in.wg.Add(1)
		go func() {
			defer in.wg.Done()
			for msg := range msgChan {
				for _, down := range in.DownStream {
					*down <- msg
					atomic.AddUint64(&in.consumeTotal, 1)
				}
			}
		}()
	case InputTypeAliyunSLS:
		if in.slsConsumer != nil {
			return fmt.Errorf("sls consumer already started")
		}
		if in.aliyunSLSCfg == nil {
			return fmt.Errorf("aliyun_sls config missing")
		}

		msgChan := make(chan map[string]interface{}, 1024)
		consumerWorker := common.NewAliyunSLSConsumer(in.aliyunSLSCfg.Endpoint, in.aliyunSLSCfg.AccessKeyID, in.aliyunSLSCfg.AccessKeySecret, in.aliyunSLSCfg.Project, in.aliyunSLSCfg.Logstore, in.aliyunSLSCfg.ConsumerGroupName, in.aliyunSLSCfg.ConsumerName, in.aliyunSLSCfg.CursorPosition, in.aliyunSLSCfg.CursorStartTime, in.aliyunSLSCfg.Query, msgChan)

		in.slsConsumer = consumerWorker
		consumerWorker.Start()
		in.wg.Add(1)
		go func() {
			defer in.wg.Done()
			for msg := range msgChan {
				for _, down := range in.DownStream {
					*down <- msg
					atomic.AddUint64(&in.consumeTotal, 1)
				}
			}
		}()
	default:
		return fmt.Errorf("unsupported input type: %s", in.Type)
	}
	return nil
}

// Stop stops the input consumer and waits for all routines to finish.
func (in *Input) Stop() error {
	switch in.Type {
	case InputTypeKafka:
		if in.kafkaConsumer != nil {
			in.kafkaConsumer.Close()
			in.kafkaConsumer = nil
		}
	case InputTypeAliyunSLS:
		if in.slsConsumer != nil {
			in.slsConsumer.Close()
			in.slsConsumer = nil
		}
	}
	if in.metricStop != nil {
		close(in.metricStop)
	}
	in.wg.Wait()
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
