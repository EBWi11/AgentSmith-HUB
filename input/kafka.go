package input

import (
	"context"
	"github.com/bytedance/sonic"
	"github.com/twmb/franz-go/pkg/kgo"
)

// KafkaProducer wraps a franz-go producer with a channel-based interface.
type KafkaProducer struct {
	Client  *kgo.Client
	MsgChan chan map[string]interface{}
	Topic   string
}

// NewKafkaProducer creates a new high-performance Kafka producer.
func NewKafkaProducer(brokers []string, topic string, msgChan chan map[string]interface{}) (*KafkaProducer, error) {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()), // high perf
		kgo.RecordPartitioner(kgo.RoundRobinPartitioner()),
	)
	if err != nil {
		return nil, err
	}
	prod := &KafkaProducer{
		Client:  cl,
		MsgChan: msgChan,
		Topic:   topic,
	}
	go prod.run()
	return prod, nil
}

// run reads from MsgChan, serializes map to JSON, and produces to Kafka asynchronously.
func (p *KafkaProducer) run() {
	for msg := range p.MsgChan {
		value, err := sonic.Marshal(msg)
		if err != nil {
			continue // skip invalid message
		}
		rec := &kgo.Record{
			Topic: p.Topic,
			Value: value,
		}
		p.Client.Produce(context.Background(), rec, nil)
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

// NewKafkaConsumer creates a new high-performance Kafka consumer.
func NewKafkaConsumer(brokers []string, group, topic string, msgChan chan map[string]interface{}) (*KafkaConsumer, error) {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(), // manual commit for perf
	)
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
