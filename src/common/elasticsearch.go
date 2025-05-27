package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
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

// flush 批量写入ES
func (p *ElasticsearchProducer) flush(batch []map[string]interface{}) {
	p.sendBatch(batch)
}

// Close 关闭producer
func (p *ElasticsearchProducer) Close() {
	close(p.MsgChan)
}

// ElasticsearchConsumer 高性能ES批量消费
type ElasticsearchConsumer struct {
	Client   *elasticsearch.Client
	Index    string
	MsgChan  chan map[string]interface{}
	PageSize int
	Scroll   time.Duration
	stopChan chan struct{}
}

// NewElasticsearchConsumer 创建ES Consumer
func NewElasticsearchConsumer(addresses []string, index string, msgChan chan map[string]interface{}, pageSize int, scroll time.Duration) (*ElasticsearchConsumer, error) {
	cfg := elasticsearch.Config{
		Addresses: addresses,
	}
	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	cons := &ElasticsearchConsumer{
		Client:   client,
		Index:    index,
		MsgChan:  msgChan,
		PageSize: pageSize,
		Scroll:   scroll,
		stopChan: make(chan struct{}),
	}
	go cons.run()
	return cons, nil
}

func (c *ElasticsearchConsumer) run() {
	scrollID := ""
	for {
		select {
		case <-c.stopChan:
			return
		default:
			var res *esapi.Response
			var err error
			if scrollID == "" {
				query := map[string]interface{}{
					"size": c.PageSize,
					"query": map[string]interface{}{
						"match_all": map[string]interface{}{},
					},
				}
				var buf bytes.Buffer
				_ = json.NewEncoder(&buf).Encode(query)
				res, err = c.Client.Search(
					c.Client.Search.WithContext(context.Background()),
					c.Client.Search.WithIndex(c.Index),
					c.Client.Search.WithBody(&buf),
					c.Client.Search.WithScroll(c.Scroll),
				)
			} else {
				res, err = c.Client.Scroll(
					c.Client.Scroll.WithContext(context.Background()),
					c.Client.Scroll.WithScrollID(scrollID),
					c.Client.Scroll.WithScroll(c.Scroll),
				)
			}
			if err != nil {
				time.Sleep(time.Second)
				continue
			}
			if res == nil || res.Body == nil {
				time.Sleep(time.Second)
				continue
			}
			var r struct {
				ScrollID string `json:"_scroll_id"`
				Hits     struct {
					Hits []struct {
						Source map[string]interface{} `json:"_source"`
					} `json:"hits"`
				} `json:"hits"`
			}
			if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
				_ = res.Body.Close()
				time.Sleep(time.Second)
				continue
			}
			_ = res.Body.Close()
			scrollID = r.ScrollID
			if len(r.Hits.Hits) == 0 {
				time.Sleep(time.Second)
				continue
			}
			for _, hit := range r.Hits.Hits {
				c.MsgChan <- hit.Source
			}
		}
	}
}

func (c *ElasticsearchConsumer) Close() {
	close(c.stopChan)
}
