package output

import (
	"AgentSmith-HUB/common"
	"strings"
	"testing"
	"time"
)

func TestOutputTimingDefaults(t *testing.T) {
	t.Run("kafka linger", func(t *testing.T) {
		cfg := &KafkaOutputConfig{}
		got, err := cfg.resolvedLingerDur()
		if err != nil {
			t.Fatalf("resolvedLingerDur returned error: %v", err)
		}
		if got != defaultKafkaLingerDur {
			t.Fatalf("expected default linger %s, got %s", defaultKafkaLingerDur, got)
		}
	})

	t.Run("elasticsearch defaults", func(t *testing.T) {
		cfg := &ElasticsearchOutputConfig{}
		batchSize, err := cfg.resolvedBatchSize()
		if err != nil {
			t.Fatalf("resolvedBatchSize returned error: %v", err)
		}
		flushDur, err := cfg.resolvedFlushDur()
		if err != nil {
			t.Fatalf("resolvedFlushDur returned error: %v", err)
		}
		if batchSize != defaultElasticsearchBatchSize {
			t.Fatalf("expected default batch size %d, got %d", defaultElasticsearchBatchSize, batchSize)
		}
		if flushDur != defaultElasticsearchFlushDur {
			t.Fatalf("expected default flush duration %s, got %s", defaultElasticsearchFlushDur, flushDur)
		}
	})

	t.Run("clickhouse defaults", func(t *testing.T) {
		cfg := &ClickHouseOutputConfig{}
		batchSize, err := cfg.resolvedBatchSize()
		if err != nil {
			t.Fatalf("resolvedBatchSize returned error: %v", err)
		}
		flushDur, err := cfg.resolvedFlushDur()
		if err != nil {
			t.Fatalf("resolvedFlushDur returned error: %v", err)
		}
		if batchSize != defaultClickHouseBatchSize {
			t.Fatalf("expected default batch size %d, got %d", defaultClickHouseBatchSize, batchSize)
		}
		if flushDur != defaultClickHouseFlushDur {
			t.Fatalf("expected default flush duration %s, got %s", defaultClickHouseFlushDur, flushDur)
		}
	})

	t.Run("aliyun sls defaults", func(t *testing.T) {
		cfg := &AliyunSLSOutputConfig{}
		batchCount, err := cfg.resolvedBatchCount()
		if err != nil {
			t.Fatalf("resolvedBatchCount returned error: %v", err)
		}
		batchSizeBytes, err := cfg.resolvedBatchSizeBytes()
		if err != nil {
			t.Fatalf("resolvedBatchSizeBytes returned error: %v", err)
		}
		lingerDur, err := cfg.resolvedLingerDur()
		if err != nil {
			t.Fatalf("resolvedLingerDur returned error: %v", err)
		}
		if batchCount != defaultAliyunSLSBatchCount {
			t.Fatalf("expected default batch count %d, got %d", defaultAliyunSLSBatchCount, batchCount)
		}
		if batchSizeBytes != defaultAliyunSLSBatchSize {
			t.Fatalf("expected default batch size %d, got %d", defaultAliyunSLSBatchSize, batchSizeBytes)
		}
		if lingerDur != defaultAliyunSLSLingerDur {
			t.Fatalf("expected default linger %s, got %s", defaultAliyunSLSLingerDur, lingerDur)
		}
	})
}

func TestOutputTestCollectionChannelIsSafe(t *testing.T) {
	out := &Output{
		Id:                  "test-output",
		ProjectNodeSequence: "INPUT.demo.OUTPUT.test",
		Type:                OutputTypePrint,
	}

	testChan := make(chan map[string]interface{}, 1)
	out.SetTestCollectionChan(&testChan)
	out.processOutputMessage(map[string]interface{}{"message": "hello"}, true, "unit-test")

	select {
	case got := <-testChan:
		if got["message"] != "hello" {
			t.Fatalf("expected collected message payload, got %#v", got)
		}
		if got["_hub_project_node_sequence"] != out.ProjectNodeSequence {
			t.Fatalf("expected project node sequence to be attached, got %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected output message to be collected")
	}

	out.SetTestCollectionChan(nil)
	out.processOutputMessage(map[string]interface{}{"message": "ignored"}, true, "unit-test")
	select {
	case got := <-testChan:
		t.Fatalf("expected no message after clearing test channel, got %#v", got)
	default:
	}

	close(testChan)
	out.sendToTestCollection(&testChan, map[string]interface{}{"message": "closed"}, "unit-test")
}

func TestVerifyRejectsInvalidOutputTuning(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name: "invalid kafka linger",
			raw: `
type: kafka
kafka:
  brokers: ["localhost:9092"]
  topic: events
  linger_dur: nope
`,
			wantErr: "kafka.linger_dur",
		},
		{
			name: "invalid elasticsearch flush duration",
			raw: `
type: elasticsearch
elasticsearch:
  hosts: ["http://localhost:9200"]
  index: hub-events
  flush_dur: -1s
`,
			wantErr: "elasticsearch.flush_dur",
		},
		{
			name: "invalid clickhouse batch size",
			raw: `
type: clickhouse
clickhouse:
  hosts: ["http://localhost:8123"]
  database: default
  table: events
  batch_size: -1
`,
			wantErr: "clickhouse.batch_size",
		},
		{
			name: "invalid aliyun linger minimum",
			raw: `
type: aliyun_sls
aliyun_sls:
  endpoint: cn-hangzhou.log.aliyuncs.com
  access_key_id: key
  access_key_secret: secret
  project: project-a
  logstore: store-a
  linger_dur: 10ms
`,
			wantErr: "aliyun_sls.linger_dur",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Verify("", tc.raw)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestVerifyAcceptsAliyunSLSOutput(t *testing.T) {
	raw := `
type: aliyun_sls
aliyun_sls:
  endpoint: cn-hangzhou.log.aliyuncs.com
  access_key_id: key
  access_key_secret: secret
  project: project-a
  logstore: store-a
  topic: audit
  source: hub-node
  batch_count: 100
  batch_size_bytes: 2048
  linger_dur: 150ms
`

	if err := Verify("", raw); err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
}

func TestGetPendingMessageCountIncludesAliyunProducerChannel(t *testing.T) {
	upstream := make(chan map[string]interface{}, 2)
	upstream <- map[string]interface{}{"kind": "upstream"}

	producerChan := make(chan map[string]interface{}, 3)
	producerChan <- map[string]interface{}{"kind": "producer-1"}
	producerChan <- map[string]interface{}{"kind": "producer-2"}

	out := &Output{
		Type:     OutputTypeAliyunSLS,
		UpStream: map[string]*chan map[string]interface{}{"in": &upstream},
		aliyunSLSProducer: &common.AliyunSLSProducer{
			MsgChan: producerChan,
		},
	}

	if got := out.GetPendingMessageCount(); got != 3 {
		t.Fatalf("expected 3 pending messages, got %d", got)
	}
}

func TestAliyunResolvedLingerRequiresSDKMinimum(t *testing.T) {
	cfg := &AliyunSLSOutputConfig{LingerDur: (50 * time.Millisecond).String()}
	if _, err := cfg.resolvedLingerDur(); err == nil {
		t.Fatal("expected resolvedLingerDur to reject values below 100ms")
	}
}
