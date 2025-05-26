package common

import (
	"fmt"
	consumerLibrary "github.com/aliyun/aliyun-log-go-sdk/consumer"
	"time"

	"github.com/aliyun/aliyun-log-go-sdk/producer"

	sls "github.com/aliyun/aliyun-log-go-sdk"
	"github.com/gogo/protobuf/proto"
)

type AliyunSLSConsumer struct {
	LoghubConfig   consumerLibrary.LogHubConfig
	MsgChan        chan map[string]interface{}
	ConsumerWorker *consumerLibrary.ConsumerWorker
}

func NewAliyunSLSConsumer(endpoint, accessKeyID, accessKeySecret, project, logstore, consumerGroupName, consumerName, cursorPosition string, cursorStartTime int64, query string, msgChan chan map[string]interface{}) *AliyunSLSConsumer {
	slsConsumer := &AliyunSLSConsumer{}

	if cursorPosition == "" {
		cursorPosition = consumerLibrary.END_CURSOR
	}

	slsConsumer.LoghubConfig = consumerLibrary.LogHubConfig{
		Endpoint:          endpoint,
		AccessKeyID:       accessKeyID,
		AccessKeySecret:   accessKeySecret,
		Project:           project,
		Logstore:          logstore,
		ConsumerGroupName: consumerGroupName,
		ConsumerName:      consumerName,
		CursorPosition:    cursorPosition,
		Query:             query,
	}

	if cursorPosition == consumerLibrary.SPECIAL_TIMER_CURSOR {
		slsConsumer.LoghubConfig.CursorStartTime = cursorStartTime
	}

	if query != "" {
		slsConsumer.LoghubConfig.Query = query
	}

	slsConsumer.MsgChan = msgChan

	return slsConsumer
}

func (consumer *AliyunSLSConsumer) Start() {
	consumer.ConsumerWorker = consumerLibrary.InitConsumerWorkerWithCheckpointTracker(consumer.LoghubConfig, consumer.aliyunSLSConsumerProcess)
	consumer.ConsumerWorker.Start()
}

func (consumer *AliyunSLSConsumer) Close() {
	consumer.ConsumerWorker.StopAndWait()
}

func (consumer *AliyunSLSConsumer) aliyunSLSConsumerProcess(shardId int, logGroupList *sls.LogGroupList, checkpointTracker consumerLibrary.CheckPointTracker) (string, error) {
	for _, logGroup := range logGroupList.LogGroups {
		for _, log := range logGroup.Logs {
			data := map[string]interface{}{}
			for _, tmp := range log.GetContents() {
				data[tmp.GetKey()] = tmp.GetValue()
			}
			consumer.MsgChan <- data
		}
	}
	_ = checkpointTracker.SaveCheckPoint(false)
	return "", nil
}

// AliyunSLSProducer wraps the Aliyun SLS producer with a channel-based interface
type AliyunSLSProducer struct {
	Producer     *producer.Producer
	MsgChan      chan map[string]interface{}
	Project      string
	Logstore     string
	BatchSize    int
	BatchTimeout time.Duration
}

// NewAliyunSLSProducer creates a new Aliyun SLS producer
func NewAliyunSLSProducer(endpoint, accessKeyID, accessKeySecret, project, logstore string, msgChan chan map[string]interface{}) (*AliyunSLSProducer, error) {
	producerConfig := producer.GetDefaultProducerConfig()
	producerConfig.Endpoint = endpoint
	producerConfig.AccessKeyID = accessKeyID
	producerConfig.AccessKeySecret = accessKeySecret
	producerConfig.GeneratePackId = true
	// Add batch configuration
	producerConfig.MaxBatchSize = 512 * 1024 // 512KB
	producerConfig.MaxReservedAttempts = 11
	producerConfig.NoRetryStatusCodeList = []int{400, 404}

	producerInstance, err := producer.NewProducer(producerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create SLS producer: %v", err)
	}

	prod := &AliyunSLSProducer{
		Producer:     producerInstance,
		MsgChan:      msgChan,
		Project:      project,
		Logstore:     logstore,
		BatchSize:    1000,
		BatchTimeout: 2 * time.Second,
	}

	go prod.run()
	return prod, nil
}

// run reads from MsgChan and sends logs to Aliyun SLS
func (p *AliyunSLSProducer) run() {
	batch := make([]*sls.Log, 0, p.BatchSize)
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

			// Convert map to SLS log
			logContents := make([]*sls.LogContent, 0, len(msg))
			for k, v := range msg {
				logContents = append(logContents, &sls.LogContent{
					Key:   proto.String(k),
					Value: proto.String(fmt.Sprintf("%v", v)),
				})
			}

			log := &sls.Log{
				Time:     proto.Uint32(uint32(time.Now().Unix())),
				Contents: logContents,
			}

			batch = append(batch, log)

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

// sendBatch sends a batch of logs to SLS
func (p *AliyunSLSProducer) sendBatch(batch []*sls.Log) {
	if len(batch) == 0 {
		return
	}

	// Send each log in the batch
	for _, log := range batch {
		err := p.Producer.SendLog(p.Project, p.Logstore, "", "", log)
		if err != nil {
			fmt.Printf("Failed to send log to SLS: %v\n", err)
		}
	}
}

// Close shuts down the producer
func (p *AliyunSLSProducer) Close() {
	if p.Producer != nil {
		p.Producer.Close(60000) // 60 seconds timeout
	}
}
