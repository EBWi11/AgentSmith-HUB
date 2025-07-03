package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"AgentSmith-HUB/logger"
)

// QPSDataProvider is a function type that provides QPS data for collection
type QPSDataProvider func() []QPSMetrics

// SystemDataProvider is a function type that provides system metrics for collection
type SystemDataProvider func() *SystemMetrics

// QPSCollector collects QPS data from local components and sends to leader
type QPSCollector struct {
	nodeID         string
	leaderAddr     string
	reportInterval time.Duration
	stopChan       chan struct{}
	wg             sync.WaitGroup
	dataProvider   QPSDataProvider
	systemProvider SystemDataProvider
}

// NewQPSCollector creates a new QPS collector instance
func NewQPSCollector(nodeID, leaderAddr string, dataProvider QPSDataProvider, systemProvider SystemDataProvider) *QPSCollector {
	return &QPSCollector{
		nodeID:         nodeID,
		leaderAddr:     leaderAddr,
		reportInterval: 10 * time.Second, // Report every 10 seconds
		stopChan:       make(chan struct{}),
		dataProvider:   dataProvider,
		systemProvider: systemProvider,
	}
}

// Start starts the QPS collection and reporting loop
func (qc *QPSCollector) Start() {
	logger.Info("Starting QPS collector", "node_id", qc.nodeID, "leader_addr", qc.leaderAddr)

	qc.wg.Add(1)
	go qc.collectAndReportLoop()
}

// Stop stops the QPS collector
func (qc *QPSCollector) Stop() {
	logger.Info("Stopping QPS collector", "node_id", qc.nodeID)

	close(qc.stopChan)
	qc.wg.Wait()

	logger.Info("QPS collector stopped")
}

// collectAndReportLoop is the main loop for collecting and reporting QPS data
func (qc *QPSCollector) collectAndReportLoop() {
	defer qc.wg.Done()

	ticker := time.NewTicker(qc.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-qc.stopChan:
			return
		case <-ticker.C:
			qc.collectAndReport()
		}
	}
}

// collectAndReport collects current QPS data and system metrics, then sends to leader
func (qc *QPSCollector) collectAndReport() {
	// Collect QPS data using the provided data provider function
	var qpsData []QPSMetrics
	if qc.dataProvider != nil {
		qpsData = qc.dataProvider()
	}

	// Collect system metrics using the provided system provider function
	var systemMetrics *SystemMetrics
	if qc.systemProvider != nil {
		systemMetrics = qc.systemProvider()
	}

	// Prepare combined data payload
	payload := map[string]interface{}{
		"qps_data": qpsData,
	}

	if systemMetrics != nil {
		payload["system_metrics"] = systemMetrics
	}

	// Send data to leader
	if err := qc.sendDataToLeader(payload); err != nil {
		logger.Error("Failed to send data to leader", "error", err, "node_id", qc.nodeID)
	}
	// Remove debug log to reduce log volume
	// logger.Debug("Successfully sent data to leader",
	// 	"node_id", qc.nodeID,
	// 	"qps_metrics_count", len(qpsData),
	// 	"has_system_metrics", systemMetrics != nil)
}

// sendDataToLeader sends combined QPS and system data to the leader node
func (qc *QPSCollector) sendDataToLeader(payload map[string]interface{}) error {
	if qc.leaderAddr == "" {
		return fmt.Errorf("leader address not set")
	}

	// Marshal data to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Send POST request to leader (use combined endpoint) - ensure proper URL format
	var url string
	if strings.HasPrefix(qc.leaderAddr, "http://") || strings.HasPrefix(qc.leaderAddr, "https://") {
		url = fmt.Sprintf("%s/metrics-sync", qc.leaderAddr)
	} else {
		url = fmt.Sprintf("http://%s/metrics-sync", qc.leaderAddr)
	}

	// Create request with authentication token
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers including authentication token
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("token", Config.Token)

	// Send request with timeout
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("leader returned status code: %d", resp.StatusCode)
	}

	return nil
}

// UpdateLeaderAddr updates the leader address
func (qc *QPSCollector) UpdateLeaderAddr(leaderAddr string) {
	qc.leaderAddr = leaderAddr
	logger.Info("Updated QPS collector leader address", "node_id", qc.nodeID, "leader_addr", leaderAddr)
}

// Global QPS collector instance (only on followers)
var GlobalQPSCollector *QPSCollector

// InitQPSCollector initializes the global QPS collector (only call on followers)
func InitQPSCollector(nodeID, leaderAddr string, dataProvider QPSDataProvider, systemProvider SystemDataProvider) {
	if GlobalQPSCollector == nil {
		GlobalQPSCollector = NewQPSCollector(nodeID, leaderAddr, dataProvider, systemProvider)
		GlobalQPSCollector.Start()
		logger.Info("QPS collector initialized and started", "node_id", nodeID)
	}
}

// StopQPSCollector stops the global QPS collector
func StopQPSCollector() {
	if GlobalQPSCollector != nil {
		GlobalQPSCollector.Stop()
		GlobalQPSCollector = nil
	}
}

// UpdateQPSCollectorLeader updates the leader address for QPS collector
func UpdateQPSCollectorLeader(leaderAddr string) {
	if GlobalQPSCollector != nil {
		GlobalQPSCollector.UpdateLeaderAddr(leaderAddr)
	}
}
