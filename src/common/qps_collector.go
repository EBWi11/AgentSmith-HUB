package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"AgentSmith-HUB/logger"
)

// QPSDataProvider is a function type that provides QPS data for collection
type QPSDataProvider func() []QPSMetrics

// QPSCollector collects QPS data from local components and sends to leader
type QPSCollector struct {
	nodeID         string
	leaderAddr     string
	reportInterval time.Duration
	stopChan       chan struct{}
	wg             sync.WaitGroup
	dataProvider   QPSDataProvider
}

// NewQPSCollector creates a new QPS collector instance
func NewQPSCollector(nodeID, leaderAddr string, dataProvider QPSDataProvider) *QPSCollector {
	return &QPSCollector{
		nodeID:         nodeID,
		leaderAddr:     leaderAddr,
		reportInterval: 10 * time.Second, // Report every 10 seconds
		stopChan:       make(chan struct{}),
		dataProvider:   dataProvider,
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

// collectAndReport collects current QPS data and sends to leader
func (qc *QPSCollector) collectAndReport() {
	// Collect QPS data using the provided data provider function
	var qpsData []QPSMetrics
	if qc.dataProvider != nil {
		qpsData = qc.dataProvider()
	}

	if len(qpsData) == 0 {
		// No data to report
		return
	}

	// Send data to leader
	if err := qc.sendQPSDataToLeader(qpsData); err != nil {
		logger.Error("Failed to send QPS data to leader", "error", err, "node_id", qc.nodeID)
	} else {
		logger.Debug("Successfully sent QPS data to leader",
			"node_id", qc.nodeID,
			"metrics_count", len(qpsData))
	}
}

// sendQPSDataToLeader sends QPS data to the leader node
func (qc *QPSCollector) sendQPSDataToLeader(qpsData []QPSMetrics) error {
	if qc.leaderAddr == "" {
		return fmt.Errorf("leader address not set")
	}

	// Marshal QPS data to JSON
	jsonData, err := json.Marshal(qpsData)
	if err != nil {
		return fmt.Errorf("failed to marshal QPS data: %w", err)
	}

	// Send POST request to leader
	url := fmt.Sprintf("http://%s/qps-sync", qc.leaderAddr)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
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
func InitQPSCollector(nodeID, leaderAddr string, dataProvider QPSDataProvider) {
	if GlobalQPSCollector == nil {
		GlobalQPSCollector = NewQPSCollector(nodeID, leaderAddr, dataProvider)
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
