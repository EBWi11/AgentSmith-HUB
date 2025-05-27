package common

import (
	"AgentSmith-HUB/logger"
	"context"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// KafkaCompressionType defines supported compression types
type KafkaCompressionType string

const (
	KafkaCompressionNone   KafkaCompressionType = "none"
	KafkaCompressionSnappy KafkaCompressionType = "snappy"
	KafkaCompressionGzip   KafkaCompressionType = "gzip"
	KafkaCompressionLz4    KafkaCompressionType = "lz4"
	KafkaCompressionZstd   KafkaCompressionType = "zstd"
)

// KafkaSASLType defines supported SASL mechanisms
type KafkaSASLType string

const (
	KafkaSASLPlain       KafkaSASLType = "plain"
	KafkaSASLSCRAMSHA256 KafkaSASLType = "scram-sha256"
	KafkaSASLSCRAMSHA512 KafkaSASLType = "scram-sha512"
	KafkaSASLOAuth       KafkaSASLType = "oauth"
)

// KafkaSASLConfig holds SASL authentication configuration
type KafkaSASLConfig struct {
	Enable    bool          `yaml:"enable"`
	Mechanism KafkaSASLType `yaml:"mechanism"`
	Username  string        `yaml:"username"`
	Password  string        `yaml:"password"`
	// For GSSAPI
	Realm              string `yaml:"realm,omitempty"`
	KeyTabPath         string `yaml:"keytab_path,omitempty"`
	KerberosConfigPath string `yaml:"kerberos_config_path,omitempty"`
	ServiceName        string `yaml:"service_name,omitempty"`
	// For OAuth
	TokenURL     string   `yaml:"token_url,omitempty"`
	ClientID     string   `yaml:"client_id,omitempty"`
	ClientSecret string   `yaml:"client_secret,omitempty"`
	Scopes       []string `yaml:"scopes,omitempty"`
}

// KafkaProducer wraps a franz-go producer with a channel-based interface.
type KafkaProducer struct {
	Client       *kgo.Client
	MsgChan      chan map[string]interface{}
	Topic        string
	KeyField     string
	KeyFieldList []string // List of fields to use as keys
	BatchSize    int
	BatchTimeout time.Duration
}

func EnsureTopicExists(cl *kgo.Client, topic string) (bool, error) {
	admin := kadm.NewClient(cl)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metadata, err := admin.ListTopics(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to list topics: %w", err)
	}
	if _, exists := metadata[topic]; exists {
		return true, nil
	}

	return false, fmt.Errorf("don't exist this topic: %w", err)
}

// NewKafkaProducer creates a new high-performance Kafka producer with compression, SASL, and key support.
func NewKafkaProducer(
	brokers []string,
	topic string,
	compression KafkaCompressionType,
	saslCfg *KafkaSASLConfig,
	msgChan chan map[string]interface{},
	keyField string,
) (*KafkaProducer, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
		kgo.RecordPartitioner(kgo.RoundRobinPartitioner()),
		kgo.ProducerBatchMaxBytes(1_000_000),
		kgo.ProducerLinger(50 * time.Millisecond),
	}

	// Add compression if specified
	if compression != KafkaCompressionNone && compression != "" {
		opts = append(opts, kgo.ProducerBatchCompression(getCompression(compression)))
	}

	// Add SASL if enabled
	if saslCfg != nil && saslCfg.Enable {
		mechanism, err := getSASLMechanism(saslCfg)
		if err != nil {
			return nil, err
		}
		if mechanism != nil {
			opts = append(opts, kgo.SASL(mechanism))
		}
	}

	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	prod := &KafkaProducer{
		Client:       cl,
		MsgChan:      msgChan,
		Topic:        topic,
		KeyField:     keyField,
		KeyFieldList: StringToList(keyField),
		BatchSize:    1000,
		BatchTimeout: 100 * time.Millisecond,
	}

	_, err = EnsureTopicExists(cl, topic)
	if err != nil {
		return nil, err
	}

	go prod.run()
	return prod, nil
}

// run processes messages from the input channel and sends them to Kafka
// It handles message serialization and error reporting
func (p *KafkaProducer) run() {
	for msg := range p.MsgChan {
		value, err := sonic.Marshal(msg)
		if err != nil {
			logger.Error("[KafkaProducer] failed to serialize message: %v", err)
			continue // skip invalid message
		}

		rec := &kgo.Record{
			Topic: p.Topic,
			Value: value,
		}

		if p.KeyField != "" {
			if tmp, ok := GetCheckData(msg, p.KeyFieldList); ok {
				rec.Key = []byte(tmp)
			}
		}

		p.Client.Produce(context.Background(), rec, func(r *kgo.Record, err error) {
			if err != nil {
				logger.Error("[KafkaProducer] failed to produce message to topic %s: %v", p.Topic, err)
			}
		})
	}
}

// Close gracefully shuts down the Kafka producer
func (p *KafkaProducer) Close() {
	p.Client.Close()
}

// KafkaConsumer wraps a franz-go consumer with a channel-based interface.
type KafkaConsumer struct {
	Client   *kgo.Client
	MsgChan  chan map[string]interface{}
	stopChan chan struct{}
}

// getCompression returns the appropriate compression option based on the compression type
// compression: The type of compression to use (Snappy, Gzip, Lz4, Zstd)
func getCompression(compression KafkaCompressionType) kgo.CompressionCodec {
	switch compression {
	case KafkaCompressionSnappy:
		return kgo.SnappyCompression()
	case KafkaCompressionGzip:
		return kgo.GzipCompression()
	case KafkaCompressionLz4:
		return kgo.Lz4Compression()
	case KafkaCompressionZstd:
		return kgo.ZstdCompression()
	default:
		return kgo.NoCompression()
	}
}

// getSASLMechanism returns the appropriate SASL mechanism based on the configuration
func getSASLMechanism(cfg *KafkaSASLConfig) (sasl.Mechanism, error) {
	if !cfg.Enable {
		return nil, nil
	}

	switch cfg.Mechanism {
	case KafkaSASLPlain:
		return plain.Plain(func(context.Context) (plain.Auth, error) {
			return plain.Auth{
				User: cfg.Username,
				Pass: cfg.Password,
			}, nil
		}), nil
	case KafkaSASLSCRAMSHA256:
		return scram.Sha256(func(context.Context) (scram.Auth, error) {
			return scram.Auth{
				User: cfg.Username,
				Pass: cfg.Password,
			}, nil
		}), nil
	case KafkaSASLSCRAMSHA512:
		return scram.Sha512(func(context.Context) (scram.Auth, error) {
			return scram.Auth{
				User: cfg.Username,
				Pass: cfg.Password,
			}, nil
		}), nil
	case KafkaSASLOAuth:
		// OAuth is not supported in the current version
		return nil, fmt.Errorf("OAuth mechanism is not supported")
	default:
		return nil, nil
	}
}

// NewKafkaConsumer creates a new high-performance Kafka consumer with compression and SASL support.
func NewKafkaConsumer(brokers []string, group, topic string, compression KafkaCompressionType, saslCfg *KafkaSASLConfig, msgChan chan map[string]interface{}) (*KafkaConsumer, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(), // manual commit for perf
	}

	// Add compression if specified
	if compression != KafkaCompressionNone {
		opts = append(opts, kgo.ProducerBatchCompression(getCompression(compression)))
	}

	// Add SASL if enabled
	if saslCfg != nil && saslCfg.Enable {
		mechanism, err := getSASLMechanism(saslCfg)
		if err != nil {
			return nil, err
		}
		if mechanism != nil {
			opts = append(opts, kgo.SASL(mechanism))
		}
	}

	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	cons := &KafkaConsumer{
		Client:   cl,
		MsgChan:  msgChan,
		stopChan: make(chan struct{}),
	}
	go cons.run()
	return cons, nil
}

// run continuously polls for messages from Kafka and forwards them to the message channel
// It handles message deserialization and error reporting
func (c *KafkaConsumer) run() {
	for {
		select {
		case <-c.stopChan:
			return
		default:
			fetches := c.Client.PollFetches(context.Background())
			if errs := fetches.Errors(); len(errs) > 0 {
				for _, err := range errs {
					logger.Error("[KafkaConsumer] fetch error: %v", err)
				}
				continue // skip errored fetches
			}
			fetches.EachRecord(func(rec *kgo.Record) {
				var m map[string]interface{}
				if err := sonic.Unmarshal(rec.Value, &m); err != nil {
					logger.Error("[KafkaConsumer] failed to deserialize message: %v", err)
					return
				}
				c.MsgChan <- m
			})
			// manual commit for batch performance
			if err := c.Client.CommitUncommittedOffsets(context.Background()); err != nil {
				logger.Error("[KafkaConsumer] failed to commit offsets: %v", err)
			}
		}
	}
}

// Close gracefully shuts down the Kafka consumer
func (c *KafkaConsumer) Close() {
	close(c.stopChan)
	c.Client.Close()
}
