package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

// ElasticsearchProducer wraps the Elasticsearch client with a channel-based interface
type ElasticsearchProducer struct {
	Client     *elasticsearch.Client
	MsgChan    chan map[string]interface{}
	Index      string
	batchSize  int
	flushDur   time.Duration
	maxRetries int
	retryDelay time.Duration
}

// NewElasticsearchProducer creates a new Elasticsearch producer
func NewElasticsearchProducer(hosts []string, index string, msgChan chan map[string]interface{}, batchSize int, flushDur time.Duration) (*ElasticsearchProducer, error) {
	cfg := elasticsearch.Config{
		Addresses:     hosts,
		MaxRetries:    3,
		RetryOnStatus: []int{502, 503, 504, 429},
	}

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create ES client: %v", err)
	}

	prod := &ElasticsearchProducer{
		Client:     client,
		MsgChan:    msgChan,
		Index:      index,
		batchSize:  batchSize,
		flushDur:   flushDur,
		maxRetries: 3,
		retryDelay: 1 * time.Second,
	}

	go prod.run()
	return prod, nil
}

func (p *ElasticsearchProducer) run() {
	batch := make([]map[string]interface{}, 0, p.batchSize)
	timer := time.NewTimer(p.flushDur)
	defer timer.Stop()

	for {
		select {
		case msg, ok := <-p.MsgChan:
			if !ok {
				p.flush(batch)
				return
			}
			batch = append(batch, msg)
			if len(batch) >= p.batchSize {
				p.flush(batch)
				batch = batch[:0]
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(p.flushDur)
			}
		case <-timer.C:
			if len(batch) > 0 {
				p.flush(batch)
				batch = batch[:0]
			}
			timer.Reset(p.flushDur)
		}
	}
}

// sendBatch sends a batch of documents to Elasticsearch with retry logic
func (p *ElasticsearchProducer) sendBatch(batch []map[string]interface{}) {
	if len(batch) == 0 {
		return
	}

	var buf bytes.Buffer
	for _, doc := range batch {
		// Add index action
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": p.Index,
			},
		}
		if err := json.NewEncoder(&buf).Encode(meta); err != nil {
			fmt.Printf("Failed to encode meta: %v\n", err)
			continue
		}
		// Add document
		if err := json.NewEncoder(&buf).Encode(doc); err != nil {
			fmt.Printf("Failed to encode document: %v\n", err)
			continue
		}
	}

	// Try to send with retries
	for i := 0; i <= p.maxRetries; i++ {
		res, err := p.Client.Bulk(bytes.NewReader(buf.Bytes()))
		if err != nil {
			if i == p.maxRetries {
				fmt.Printf("Failed to send batch to ES after %d retries: %v\n", p.maxRetries, err)
				return
			}
			time.Sleep(p.retryDelay)
			continue
		}
		defer res.Body.Close()

		if res.IsError() {
			if i == p.maxRetries {
				fmt.Printf("ES returned error after %d retries: %s\n", p.maxRetries, res.String())
				return
			}
			time.Sleep(p.retryDelay)
			continue
		}

		// Success
		return
	}
}

// flush batch writes to ES
func (p *ElasticsearchProducer) flush(batch []map[string]interface{}) {
	p.sendBatch(batch)
}

// Close closes the producer
// Note: We don't close MsgChan here because it's owned by the caller
func (p *ElasticsearchProducer) Close() {
	// The channel will be closed by the owner (output component)
	// We just need to ensure any pending operations are completed
}
