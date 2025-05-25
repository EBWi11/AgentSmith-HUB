package common

import (
	"context"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
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
	KafkaSASLNone        KafkaSASLType = "none"
	KafkaSASLPlain       KafkaSASLType = "plain"
	KafkaSASLSCRAMSHA256 KafkaSASLType = "scram-sha256"
	KafkaSASLSCRAMSHA512 KafkaSASLType = "scram-sha512"
	KafkaSASLGSSAPI      KafkaSASLType = "gssapi"
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
	BatchSize    int
	BatchTimeout time.Duration
}

// NewKafkaProducer creates a new high-performance Kafka producer with compression and SASL support.
func NewKafkaProducer(brokers []string, topic string, compression KafkaCompressionType, saslCfg *KafkaSASLConfig, msgChan chan map[string]interface{}) (*KafkaProducer, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
		kgo.RecordPartitioner(kgo.RoundRobinPartitioner()),
		kgo.ProducerBatchMaxBytes(1024 * 1024), // 1MB
		kgo.ProducerLinger(100 * time.Millisecond),
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

	prod := &KafkaProducer{
		Client:       cl,
		MsgChan:      msgChan,
		Topic:        topic,
		BatchSize:    1000,
		BatchTimeout: 100 * time.Millisecond,
	}
	go prod.run()
	return prod, nil
}

// run reads from MsgChan, serializes map to JSON, and produces to Kafka asynchronously.
func (p *KafkaProducer) run() {
	batch := make([]*kgo.Record, 0, p.BatchSize)
	ticker := time.NewTicker(p.BatchTimeout)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-p.MsgChan:
			if !ok {
				// Channel closed, send remaining batch
				if len(batch) > 0 {
					p.sendBatch(batch)
				}
				return
			}

			value, err := sonic.Marshal(msg)
			if err != nil {
				continue // skip invalid message
			}

			batch = append(batch, &kgo.Record{
				Topic: p.Topic,
				Value: value,
			})

			if len(batch) >= p.BatchSize {
				p.sendBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				p.sendBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

// sendBatch sends a batch of records to Kafka
func (p *KafkaProducer) sendBatch(batch []*kgo.Record) {
	if len(batch) == 0 {
		return
	}

	// Use a buffered channel to avoid blocking
	done := make(chan error, 1)
	p.Client.Produce(context.Background(), batch[0], func(r *kgo.Record, err error) {
		done <- err
	})

	// Wait for the first record to be acknowledged
	if err := <-done; err != nil {
		// Log error but continue processing
		fmt.Printf("Failed to send batch to Kafka: %v\n", err)
	}
}

// Close shuts down the producer.
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

// run polls Kafka, deserializes JSON to map, and sends messages to MsgChan.
func (c *KafkaConsumer) run() {
	for {
		select {
		case <-c.stopChan:
			return
		default:
			fetches := c.Client.PollFetches(context.Background())
			if errs := fetches.Errors(); len(errs) > 0 {
				continue // skip errored fetches
			}
			fetches.EachRecord(func(rec *kgo.Record) {
				var m map[string]interface{}
				if err := sonic.Unmarshal(rec.Value, &m); err == nil {
					c.MsgChan <- m
				}
			})
			// manual commit for batch performance
			c.Client.CommitUncommittedOffsets(context.Background())
		}
	}
}

// Close shuts down the consumer.
func (c *KafkaConsumer) Close() {
	close(c.stopChan)
	c.Client.Close()
}
