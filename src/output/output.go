package output

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// OutputType defines the type of output destination.
type OutputType string

const (
	OutputTypeKafka         OutputType = "kafka"
	OutputTypeElasticsearch OutputType = "elasticsearch"
	OutputTypePrint         OutputType = "print"
)

// OutputConfig is the YAML config for an output.
type OutputConfig struct {
	Name          string `yaml:"name"`
	Id            string
	Type          OutputType                 `yaml:"type"`
	Kafka         *KafkaOutputConfig         `yaml:"kafka,omitempty"`
	Elasticsearch *ElasticsearchOutputConfig `yaml:"elasticsearch,omitempty"`
	AliyunSLS     *AliyunSLSOutputConfig     `yaml:"aliyun_sls,omitempty"`
	// No config needed for print
}

// KafkaOutputConfig holds Kafka-specific config.
type KafkaOutputConfig struct {
	Brokers     []string                    `yaml:"brokers"`
	Topic       string                      `yaml:"topic"`
	Compression common.KafkaCompressionType `yaml:"compression,omitempty"`
	SASL        *common.KafkaSASLConfig     `yaml:"sasl,omitempty"`
	Key         string                      `yaml:"key"`
}

// ElasticsearchOutputConfig holds Elasticsearch-specific config.
type ElasticsearchOutputConfig struct {
	Hosts     []string `yaml:"hosts"`
	Index     string   `yaml:"index"`
	BatchSize int      `yaml:"batch_size,omitempty"`
	FlushDur  string   `yaml:"flush_dur,omitempty"`
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
	Name                string
	Id                  string
	ProjectNodeSequence string
	Type                OutputType
	UpStream            []*chan map[string]interface{}

	// runtime
	kafkaProducer         *common.KafkaProducer
	elasticsearchProducer *common.ElasticsearchProducer
	wg                    sync.WaitGroup

	// config cache
	kafkaCfg         *KafkaOutputConfig
	elasticsearchCfg *ElasticsearchOutputConfig
	aliyunSLSCfg     *AliyunSLSOutputConfig

	// metrics
	produceTotal uint64
	produceQPS   uint64
	metricStop   chan struct{}

	// for print output
	printStop chan struct{}
}

// LoadOutputConfig loads output config from a YAML file.
func LoadOutputConfig(path string) (*OutputConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg OutputConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// NewOutput creates an Output from config and upstreams.
func NewOutput(path string, id string) (*Output, error) {
	cfg, err := LoadOutputConfig(path)
	if err != nil {
		return nil, err
	}

	out := &Output{
		Name:             cfg.Name,
		Id:               id,
		Type:             cfg.Type,
		UpStream:         make([]*chan map[string]interface{}, 0),
		kafkaCfg:         cfg.Kafka,
		elasticsearchCfg: cfg.Elasticsearch,
		aliyunSLSCfg:     cfg.AliyunSLS,
	}
	return out, nil
}

// Start initializes and starts the output component based on its type
// Returns an error if the component is already running or if initialization fails
func (out *Output) Start() error {
	// Start metric goroutine
	out.metricStop = make(chan struct{})
	go out.metricLoop()

	switch out.Type {
	case OutputTypeKafka:
		if out.kafkaProducer != nil {
			return fmt.Errorf("kafka producer already running for output %s", out.Id)
		}
		if out.kafkaCfg == nil {
			return fmt.Errorf("kafka configuration missing for output %s", out.Id)
		}
		msgChan := make(chan map[string]interface{}, 1024)
		prod, err := common.NewKafkaProducer(
			out.kafkaCfg.Brokers,
			out.kafkaCfg.Topic,
			out.kafkaCfg.Compression,
			out.kafkaCfg.SASL,
			msgChan,
			out.kafkaCfg.Key,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize kafka producer for output %s: %w", out.Id, err)
		}
		out.kafkaProducer = prod
		out.wg.Add(1)
		go func() {
			defer out.wg.Done()
			for _, up := range out.UpStream {
				go func() {
					for msg := range *up {
						msgChan <- msg
						atomic.AddUint64(&out.produceTotal, 1)
					}
				}()
			}
		}()
	case OutputTypeElasticsearch:
		if out.elasticsearchProducer != nil {
			return fmt.Errorf("elasticsearch producer already running for output %s", out.Id)
		}
		if out.elasticsearchCfg == nil {
			return fmt.Errorf("elasticsearch configuration missing for output %s", out.Id)
		}
		msgChan := make(chan map[string]interface{}, 1024)
		batchSize := 1000
		if out.elasticsearchCfg.BatchSize > 0 {
			batchSize = out.elasticsearchCfg.BatchSize
		}
		flushDur := 5 * time.Second
		if out.elasticsearchCfg.FlushDur != "" {
			if d, err := time.ParseDuration(out.elasticsearchCfg.FlushDur); err == nil {
				flushDur = d
			}
		}
		prod, err := common.NewElasticsearchProducer(
			out.elasticsearchCfg.Hosts,
			out.elasticsearchCfg.Index,
			msgChan,
			batchSize,
			flushDur,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize elasticsearch producer for output %s: %w", out.Id, err)
		}
		out.elasticsearchProducer = prod
		out.wg.Add(1)
		go func() {
			defer out.wg.Done()
			for _, up := range out.UpStream {
				go func() {
					for msg := range *up {
						msgChan <- msg
						atomic.AddUint64(&out.produceTotal, 1)
					}
				}()
			}
		}()
	case OutputTypePrint:
		out.printStop = make(chan struct{})
		out.wg.Add(1)
		go func() {
			defer out.wg.Done()
			for _, up := range out.UpStream {
				go func() {
					for {
						select {
						case <-out.printStop:
							return
						case msg, ok := <-*up:
							if !ok {
								return
							}
							b, err := json.Marshal(msg)
							if err != nil {
								logger.Error("[PRINT OUTPUT] marshal error: %v\n", err)
								continue
							}
							fmt.Println(string(b))
							atomic.AddUint64(&out.produceTotal, 1)
						}
					}
				}()
			}
		}()
	default:
		return fmt.Errorf("unsupported output type %s for output %s", out.Type, out.Id)
	}
	return nil
}

// Stop stops the output producer and waits for all routines to finish.
// It waits until all upstream channels are empty and all pending data is written.
func (out *Output) Stop() error {
	// Wait for all upstream channels to be empty before closing producers
waitUpstream:
	for {
		allEmpty := true
		for _, up := range out.UpStream {
			if len(*up) > 0 {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			break waitUpstream
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for all internal msgChan to be empty (for each output type)
	switch out.Type {
	case OutputTypeKafka:
		if out.kafkaProducer != nil && out.kafkaProducer.MsgChan != nil {
		waitKafkaMsgChan:
			for {
				if len(out.kafkaProducer.MsgChan) == 0 {
					break waitKafkaMsgChan
				}
				time.Sleep(50 * time.Millisecond)
			}
			out.kafkaProducer.Close()
			out.kafkaProducer = nil
		}
	case OutputTypeElasticsearch:
		if out.elasticsearchProducer != nil && out.elasticsearchProducer.MsgChan != nil {
		waitESMsgChan:
			for {
				if len(out.elasticsearchProducer.MsgChan) == 0 {
					break waitESMsgChan
				}
				time.Sleep(50 * time.Millisecond)
			}
			out.elasticsearchProducer.Close()
			out.elasticsearchProducer = nil
		}
	case OutputTypePrint:
		if out.printStop != nil {
			close(out.printStop)
		}
	}
	if out.metricStop != nil {
		close(out.metricStop)
	}
	out.wg.Wait()
	return nil
}

// metricLoop calculates QPS and can be extended for more metrics.
func (out *Output) metricLoop() {
	var lastTotal uint64
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-out.metricStop:
			return
		case <-ticker.C:
			cur := atomic.LoadUint64(&out.produceTotal)
			atomic.StoreUint64(&out.produceQPS, cur-lastTotal)
			lastTotal = cur
		}
	}
}

// GetProduceQPS returns the latest QPS.
func (out *Output) GetProduceQPS() uint64 {
	return atomic.LoadUint64(&out.produceQPS)
}

// GetProduceTotal returns the total produced count.
func (out *Output) GetProduceTotal() uint64 {
	return atomic.LoadUint64(&out.produceTotal)
}
