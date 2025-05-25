package output

import (
	"AgentSmith-HUB/common"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// OutputType defines the type of output target.
type OutputType string

const (
	OutputTypeKafka         OutputType = "kafka"
	OutputTypeAliyunSLS     OutputType = "aliyun_sls"
	OutputTypeElasticsearch OutputType = "elasticsearch"
)

// KafkaCompressionType and KafkaSASLType should be consistent with input.go
type KafkaCompressionType string
type KafkaSASLType string

const (
	KafkaCompressionNone   KafkaCompressionType = "none"
	KafkaCompressionSnappy KafkaCompressionType = "snappy"
	KafkaCompressionGzip   KafkaCompressionType = "gzip"
	KafkaCompressionLz4    KafkaCompressionType = "lz4"
	KafkaCompressionZstd   KafkaCompressionType = "zstd"

	KafkaSASLNone  KafkaSASLType = "none"
	KafkaSASLPlain KafkaSASLType = "plain"
	KafkaSASLSCRAM KafkaSASLType = "scram"
)

// OutputConfig is the YAML config for an output.
type OutputConfig struct {
	Name          string                     `yaml:"name"`
	Id            string                     `yaml:"id"`
	Type          OutputType                 `yaml:"type"`
	Kafka         *KafkaOutputConfig         `yaml:"kafka,omitempty"`
	AliyunSLS     *AliyunSLSOutputConfig     `yaml:"aliyun_sls,omitempty"`
	Elasticsearch *ElasticsearchOutputConfig `yaml:"elasticsearch,omitempty"`
}

// KafkaOutputConfig holds Kafka-specific config.
type KafkaOutputConfig struct {
	Brokers     []string             `yaml:"brokers"`
	Topic       string               `yaml:"topic"`
	Compression KafkaCompressionType `yaml:"compression,omitempty"`
	SASL        *KafkaSASLConfig     `yaml:"sasl,omitempty"`
}

// KafkaSASLConfig holds Kafka SASL authentication config.
type KafkaSASLConfig struct {
	Enable    bool          `yaml:"enable"`
	Mechanism KafkaSASLType `yaml:"mechanism"`
	Username  string        `yaml:"username"`
	Password  string        `yaml:"password"`
}

// AliyunSLSOutputConfig holds Aliyun SLS-specific config.
type AliyunSLSOutputConfig struct {
	Endpoint        string `yaml:"endpoint"`
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
	Project         string `yaml:"project"`
	Logstore        string `yaml:"logstore"`
	Topic           string `yaml:"topic"`
	Source          string `yaml:"source"`
}

// ElasticsearchOutputConfig holds Elasticsearch-specific config.
type ElasticsearchOutputConfig struct {
	Addresses []string      `yaml:"addresses"`
	Index     string        `yaml:"index"`
	BatchSize int           `yaml:"batch_size"`
	FlushDur  time.Duration `yaml:"flush_dur"`
}

// Output is the runtime output instance.
type Output struct {
	Name     string
	Id       string
	Type     OutputType
	UpStream []*chan map[string]interface{}

	// runtime
	kafkaProducer *common.KafkaProducer
	slsProducer   func() // placeholder for SLS producer, can be extended
	esProducer    *common.ElasticsearchProducer
	stopChan      chan struct{}
	wg            sync.WaitGroup

	// config cache
	kafkaCfg         *KafkaOutputConfig
	aliyunSLSCfg     *AliyunSLSOutputConfig
	elasticsearchCfg *ElasticsearchOutputConfig

	// metrics
	produceTotal uint64
	produceQPS   uint64
	metricStop   chan struct{}
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
func NewOutput(cfg *OutputConfig, upstream []*chan map[string]interface{}) (*Output, error) {
	out := &Output{
		Name:             cfg.Name,
		Id:               cfg.Id,
		Type:             cfg.Type,
		UpStream:         upstream,
		kafkaCfg:         cfg.Kafka,
		aliyunSLSCfg:     cfg.AliyunSLS,
		elasticsearchCfg: cfg.Elasticsearch,
	}
	return out, nil
}

// Start launches the output producer and pulls data from upstream.
func (out *Output) Start() error {
	out.metricStop = make(chan struct{})
	go out.metricLoop()

	switch out.Type {
	case OutputTypeKafka:
		if out.kafkaProducer != nil {
			return fmt.Errorf("kafka producer already started")
		}
		if out.kafkaCfg == nil {
			return fmt.Errorf("kafka config missing")
		}
		msgChan := make(chan map[string]interface{}, 1024)
		prod, err := common.NewKafkaProducer(
			out.kafkaCfg.Brokers,
			out.kafkaCfg.Topic,
			msgChan,
		)
		if err != nil {
			return err
		}
		out.kafkaProducer = prod
		out.wg.Add(1)
		go func() {
			defer out.wg.Done()
			for {
				select {
				case <-out.stopChan:
					close(msgChan)
					return
				default:
					for _, up := range out.UpStream {
						select {
						case msg := <-*up:
							msgChan <- msg
							atomic.AddUint64(&out.produceTotal, 1)
						default:
							// no data, continue
						}
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	case OutputTypeElasticsearch:
		if out.elasticsearchCfg == nil {
			return fmt.Errorf("elasticsearch config missing")
		}
		msgChan := make(chan map[string]interface{}, 1024)
		prod, err := common.NewElasticsearchProducer(
			out.elasticsearchCfg.Addresses,
			out.elasticsearchCfg.Index,
			msgChan,
			out.elasticsearchCfg.BatchSize,
			out.elasticsearchCfg.FlushDur,
		)
		if err != nil {
			return err
		}
		out.esProducer = prod
		out.wg.Add(1)
		go func() {
			defer out.wg.Done()
			for {
				select {
				case <-out.stopChan:
					close(msgChan)
					return
				default:
					for _, up := range out.UpStream {
						select {
						case msg := <-*up:
							msgChan <- msg
							atomic.AddUint64(&out.produceTotal, 1)
						default:
						}
					}
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	case OutputTypeAliyunSLS:
		// TODO: Implement SLS producer integration, similar to above
		return fmt.Errorf("aliyun_sls output not implemented yet")
	default:
		return fmt.Errorf("unsupported output type: %s", out.Type)
	}
	return nil
}

// Stop stops the output producer and waits for all routines to finish.
func (out *Output) Stop() error {
	if out.stopChan != nil {
		close(out.stopChan)
	}
	if out.metricStop != nil {
		close(out.metricStop)
	}
	if out.kafkaProducer != nil {
		out.kafkaProducer.Close()
	}
	if out.esProducer != nil {
		out.esProducer.Close()
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
