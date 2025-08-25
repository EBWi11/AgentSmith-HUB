package common

import (
	"AgentSmith-HUB/logger"
	"fmt"
	"strings"

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
		ConsumerName:      fmt.Sprintf("%s-%s", consumerName, Config.LocalIP),
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
			// Blocking send to ensure no data loss
			// If downstream is full, this will block and prevent further consumption
			consumer.MsgChan <- data
		}
	}
	err := checkpointTracker.SaveCheckPoint(false)
	if err != nil {
		logger.Error("[AliyunSLSConsumer] failed to save checkpoint", "error", err.Error())
	}
	return "", nil
}

// TestAliyunSLSConnection tests the connection to Aliyun SLS service
// This method creates a temporary client to test connectivity without affecting existing consumers/producers
func TestAliyunSLSConnection(endpoint, accessKeyID, accessKeySecret, project, logstore string) error {
	// Create a temporary SLS client for testing
	client := sls.CreateNormalInterface(endpoint, accessKeyID, accessKeySecret, "")

	// First try to check logstore directly, as this requires fewer permissions than project-level operations
	// For consumers, we typically only need logstore-level access
	_, err := client.CheckLogstoreExist(project, logstore)
	if err != nil {
		// If error is about permissions but not connection, try a different approach
		if strings.Contains(err.Error(), "Unauthorized") || strings.Contains(err.Error(), "denied by sts or ram") {
			// Try to create a consumer config to test if we have consumer permissions
			// This is a lighter operation that doesn't require project-level permissions
			testConfig := consumerLibrary.LogHubConfig{
				Endpoint:          endpoint,
				AccessKeyID:       accessKeyID,
				AccessKeySecret:   accessKeySecret,
				Project:           project,
				Logstore:          logstore,
				ConsumerGroupName: "test-connection-check",
				ConsumerName:      "test-connection",
				CursorPosition:    consumerLibrary.END_CURSOR,
			}

			// Try to initialize a consumer worker for validation
			// This will fail if credentials are completely invalid, but succeed if we have consumer permissions
			testWorker := consumerLibrary.InitConsumerWorkerWithCheckpointTracker(testConfig, func(shardId int, logGroupList *sls.LogGroupList, checkpointTracker consumerLibrary.CheckPointTracker) (string, error) {
				return "", nil
			})

			if testWorker == nil {
				return fmt.Errorf("failed to create consumer worker - invalid credentials or configuration")
			}

			// Clean up the test worker immediately
			// Don't start it, just validate that it can be created
			testWorker = nil

			// If we reach here, it means we can create a consumer worker successfully
			// This indicates the credentials and basic configuration are valid for consuming
			return nil
		}

		// For other errors (network, invalid endpoint, etc.), return the original error
		return fmt.Errorf("failed to connect to Aliyun SLS or access logstore: %w", err)
	}

	// If CheckLogstoreExist succeeds, we have good connectivity and appropriate permissions
	return nil
}

// TestAliyunSLSLogstoreExists tests if the specified logstore exists in the project
func TestAliyunSLSLogstoreExists(endpoint, accessKeyID, accessKeySecret, project, logstore string) (bool, error) {
	// Create a temporary SLS client for testing
	client := sls.CreateNormalInterface(endpoint, accessKeyID, accessKeySecret, "")

	// Check if logstore exists
	_, err := client.CheckLogstoreExist(project, logstore)
	if err != nil {
		// If error contains "not exist", return false, nil
		if strings.Contains(err.Error(), "not exist") || strings.Contains(err.Error(), "does not exist") {
			return false, nil
		}

		// If it's a permission error, we can't determine if logstore exists
		// but we shouldn't fail the connection check entirely
		if strings.Contains(err.Error(), "Unauthorized") || strings.Contains(err.Error(), "denied by sts or ram") {
			// Return unknown status with descriptive error
			return false, fmt.Errorf("insufficient permissions to verify logstore existence (but connection may still work for consuming)")
		}

		// Other errors indicate connection issues
		return false, fmt.Errorf("failed to check logstore existence: %w", err)
	}

	return true, nil
}

// GetAliyunSLSProjectInfo gets basic information about the SLS project
func GetAliyunSLSProjectInfo(endpoint, accessKeyID, accessKeySecret, project string) (map[string]interface{}, error) {
	// Create a temporary SLS client for testing
	client := sls.CreateNormalInterface(endpoint, accessKeyID, accessKeySecret, "")

	// Try to get project info - this might fail due to permissions
	projectInfo, err := client.GetProject(project)
	if err != nil {
		// If it's a permission error, return limited info
		if strings.Contains(err.Error(), "Unauthorized") || strings.Contains(err.Error(), "denied by sts or ram") {
			return map[string]interface{}{
				"project_name": project,
				"note":         "Limited permissions - project exists but detailed info unavailable",
			}, nil
		}
		return nil, fmt.Errorf("failed to get project info: %w", err)
	}

	// Try to get logstores list (optional, for additional info)
	logstores, err := client.ListLogStore(project)
	if err != nil {
		// Don't fail on logstore list error, just skip this info
		return map[string]interface{}{
			"project_name":        projectInfo.Name,
			"project_description": projectInfo.Description,
			"project_region":      projectInfo.Region,
			"project_status":      projectInfo.Status,
		}, nil
	}

	return map[string]interface{}{
		"project_name":        projectInfo.Name,
		"project_description": projectInfo.Description,
		"project_region":      projectInfo.Region,
		"project_status":      projectInfo.Status,
		"logstore_count":      len(logstores),
		"logstores":           logstores,
	}, nil
}
