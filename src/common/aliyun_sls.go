package common

import (
	"fmt"

	sls "github.com/aliyun/aliyun-log-go-sdk"
	consumerLibrary "github.com/aliyun/aliyun-log-go-sdk/consumer"
)

type AliyunSLSConsumer struct {
	LoghubConfig   consumerLibrary.LogHubConfig
	MsgChan        chan map[string]interface{}
	ConsumerWorker *consumerLibrary.ConsumerWorker
}

func NewAliyunSLSConsumer(endpoint, accessKeyID, accessKeySecret, project, logstore, consumerGroupName, consumerName, cursorPosition string, cursorStartTime int64, query string, msgChan chan map[string]interface{}) (*AliyunSLSConsumer, error) {
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

	slsConsumer.MsgChan = msgChan

	// Start the consumer to validate configuration
	slsConsumer.ConsumerWorker = consumerLibrary.InitConsumerWorkerWithCheckpointTracker(slsConsumer.LoghubConfig, slsConsumer.aliyunSLSConsumerProcess)
	if slsConsumer.ConsumerWorker == nil {
		return nil, fmt.Errorf("failed to initialize SLS consumer worker")
	}

	return slsConsumer, nil
}

func (consumer *AliyunSLSConsumer) Start() {
	consumer.ConsumerWorker.Start()
}

func (consumer *AliyunSLSConsumer) Close() error {
	if consumer.ConsumerWorker == nil {
		return fmt.Errorf("consumer worker is not initialized")
	}
	consumer.ConsumerWorker.StopAndWait()
	return nil
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
