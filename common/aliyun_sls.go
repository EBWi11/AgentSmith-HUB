package common

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/aliyun/aliyun-log-go-sdk/producer"

	sls "github.com/aliyun/aliyun-log-go-sdk"
	consumerLibrary "github.com/aliyun/aliyun-log-go-sdk/consumer"
	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
)

// README :
// This is a very simple example of pulling data from your logstore and printing it for consumption.

func SlsConsumer() {
	option := consumerLibrary.LogHubConfig{
		Endpoint:          "",
		AccessKeyID:       "",
		AccessKeySecret:   "",
		Project:           "",
		Logstore:          "",
		ConsumerGroupName: "",
		ConsumerName:      "",
		// This options is used for initialization, will be ignored once consumer group is created and each shard has been started to be consumed.
		// Could be "begin", "end", "specific time format in time stamp", it's log receiving time.
		CursorPosition: consumerLibrary.END_CURSOR,
	}

	consumerWorker := consumerLibrary.InitConsumerWorkerWithCheckpointTracker(option, process)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	consumerWorker.Start()
	if _, ok := <-ch; ok {
		level.Info(consumerWorker.Logger).Log("msg", "get stop signal, start to stop consumer worker", "consumer worker name", option.ConsumerName)
		consumerWorker.StopAndWait()
	}
}

func SlsProducer() {
	{
		producerConfig := producer.GetDefaultProducerConfig()
		producerConfig.Endpoint = os.Getenv("Endpoint")
		producerConfig.AccessKeyID = os.Getenv("AccessKeyID")
		producerConfig.AccessKeySecret = os.Getenv("AccessKeySecret")
		// if you want to use log context, set generate pack id true
		producerConfig.GeneratePackId = true
		producerConfig.LogTags = []*sls.LogTag{
			&sls.LogTag{
				Key:   proto.String("tag_1"),
				Value: proto.String("value_1"),
			},
			&sls.LogTag{
				Key:   proto.String("tag_2"),
				Value: proto.String("value_2"),
			},
		}
		producerInstance, err := producer.NewProducer(producerConfig)
		if err != nil {
			panic(err)
		}
		ch := make(chan os.Signal)
		signal.Notify(ch, os.Kill, os.Interrupt)
		producerInstance.Start()
		var m sync.WaitGroup
		for i := 0; i < 10; i++ {
			m.Add(1)
			go func() {
				defer m.Done()
				for i := 0; i < 1000; i++ {
					// GenerateLog  is producer's function for generating SLS format logs
					// GenerateLog has low performance, and native Log interface is the best choice for high performance.
					log := producer.GenerateLog(uint32(time.Now().Unix()), map[string]string{"content": "test", "content2": fmt.Sprintf("%v", i)})
					err := producerInstance.SendLog("log-project", "log-store", "topic", "127.0.0.1", log)
					if err != nil {
						fmt.Println(err)
					}
				}
			}()
		}
		m.Wait()
		fmt.Println("Send completion")
		if _, ok := <-ch; ok {
			fmt.Println("Get the shutdown signal and start to shut down")
			producerInstance.Close(60000)
		}
	}
}

// Fill in your consumption logic here, and be careful not to change the parameters of the function and the return value,
// otherwise you will report errors.
func process(shardId int, logGroupList *sls.LogGroupList, checkpointTracker consumerLibrary.CheckPointTracker) (string, error) {
	fmt.Println(shardId, logGroupList)
	checkpointTracker.SaveCheckPoint(false)
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
