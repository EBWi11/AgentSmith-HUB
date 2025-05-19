package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// ElasticsearchProducer 高性能ES批量写入
type ElasticsearchProducer struct {
	Client    *elasticsearch.Client
	Index     string
	MsgChan   chan map[string]interface{}
	batchSize int
	flushDur  time.Duration
}

// NewElasticsearchProducer 创建ES Producer
func NewElasticsearchProducer(addresses []string, index string, msgChan chan map[string]interface{}, batchSize int, flushDur time.Duration) (*ElasticsearchProducer, error) {
	cfg := elasticsearch.Config{
		Addresses: addresses,
	}
	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	prod := &ElasticsearchProducer{
		Client:    client,
		Index:     index,
		MsgChan:   msgChan,
		batchSize: batchSize,
		flushDur:  flushDur,
	}
	go prod.run()
	return prod, nil
}

// run 批量消费channel并写入ES
func (p *ElasticsearchProducer) run() {
	batch := make([]map[string]interface{}, 0, p.batchSize)
	timer := time.NewTimer(p.flushDur)
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

// flush 批量写入ES
func (p *ElasticsearchProducer) flush(batch []map[string]interface{}) {
	if len(batch) == 0 {
		return
	}
	var buf bytes.Buffer
	for _, doc := range batch {
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": p.Index,
			},
		}
		metaLine, _ := json.Marshal(meta)
		docLine, _ := json.Marshal(doc)
		buf.Write(metaLine)
		buf.WriteByte('\n')
		buf.Write(docLine)
		buf.WriteByte('\n')
	}
	req := esapi.BulkRequest{
		Body: bytes.NewReader(buf.Bytes()),
	}
	resp, err := req.Do(context.Background(), p.Client)
	if err != nil {
		fmt.Printf("ES bulk error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.IsError() {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("ES bulk response error: %s\n", body)
	}
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

// run 批量拉取ES数据并写入channel
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
				// 初次请求
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
				res.Body.Close()
				time.Sleep(time.Second)
				continue
			}
			res.Body.Close()
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

// Close 关闭consumer
func (c *ElasticsearchConsumer) Close() {
	close(c.stopChan)
}
