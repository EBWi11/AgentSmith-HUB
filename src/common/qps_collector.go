package common

import (
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
	reportInterval time.Duration
	stopChan       chan struct{}
	wg             sync.WaitGroup
	dataProvider   QPSDataProvider
	systemProvider SystemDataProvider
}

// NewQPSCollector creates a new QPS collector instance
func NewQPSCollector(nodeID string, dataProvider QPSDataProvider, systemProvider SystemDataProvider) *QPSCollector {
	return &QPSCollector{
		nodeID:         nodeID,
		reportInterval: 10 * time.Second, // Report every 10 seconds
		stopChan:       make(chan struct{}),
		dataProvider:   dataProvider,
		systemProvider: systemProvider,
	}
}

// Start starts the QPS collection and reporting loop
func (qc *QPSCollector) Start() {
	logger.Info("Starting QPS collector", "node_id", qc.nodeID)

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

// collectAndReport collects current QPS data and system metrics, then processes locally
func (qc *QPSCollector) collectAndReport() {
	// Collect QPS data using the provided data provider function
	var qpsData []QPSMetrics
	if qc.dataProvider != nil {
		qpsData = qc.dataProvider()
	}

	// Process QPS data locally instead of sending to Redis
	if GlobalQPSManager != nil {
		for _, q := range qpsData {
			GlobalQPSManager.AddQPSData(&q)
		}
	}

	// Collect system metrics using the provided system provider function
	var systemMetrics *SystemMetrics
	if qc.systemProvider != nil {
		systemMetrics = qc.systemProvider()
	}

	// Process system metrics locally
	if systemMetrics != nil && GlobalClusterSystemManager != nil {
		GlobalClusterSystemManager.AddSystemMetrics(systemMetrics)
	}
}

// Global QPS collector instance (only on followers)
var GlobalQPSCollector *QPSCollector

// InitQPSCollector initializes the global QPS collector (only call on followers)
func InitQPSCollector(nodeID string, dataProvider QPSDataProvider, systemProvider SystemDataProvider) {
	if GlobalQPSCollector == nil {
		GlobalQPSCollector = NewQPSCollector(nodeID, dataProvider, systemProvider)
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
