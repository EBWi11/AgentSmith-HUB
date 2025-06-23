package common

import (
	"fmt"
	"sync"
	"time"
)

// QPSMetrics represents QPS data for a specific component
type QPSMetrics struct {
	NodeID        string    `json:"node_id"`
	ProjectID     string    `json:"project_id"`
	ComponentID   string    `json:"component_id"`
	ComponentType string    `json:"component_type"` // "input", "output", "ruleset"
	QPS           uint64    `json:"qps"`
	Timestamp     time.Time `json:"timestamp"`
}

// QPSDataPoint represents a single QPS measurement
type QPSDataPoint struct {
	QPS       uint64    `json:"qps"`
	Timestamp time.Time `json:"timestamp"`
}

// ComponentQPSData holds time series data for a component
type ComponentQPSData struct {
	NodeID        string         `json:"node_id"`
	ProjectID     string         `json:"project_id"`
	ComponentID   string         `json:"component_id"`
	ComponentType string         `json:"component_type"`
	DataPoints    []QPSDataPoint `json:"data_points"`
	LastUpdate    time.Time      `json:"last_update"`
}

// QPSManager manages QPS data collection and aggregation on leader node
type QPSManager struct {
	// Key format: "nodeID_projectID_componentID_componentType"
	data      map[string]*ComponentQPSData
	mutex     sync.RWMutex
	stopChan  chan struct{}
	cleanupWg sync.WaitGroup
}

// NewQPSManager creates a new QPS manager instance
func NewQPSManager() *QPSManager {
	qm := &QPSManager{
		data:     make(map[string]*ComponentQPSData),
		stopChan: make(chan struct{}),
	}

	// Start cleanup goroutine to remove old data
	qm.cleanupWg.Add(1)
	go qm.cleanupLoop()

	return qm
}

// generateKey creates a unique key for component QPS data
func (qm *QPSManager) generateKey(nodeID, projectID, componentID, componentType string) string {
	return fmt.Sprintf("%s_%s_%s_%s", nodeID, projectID, componentID, componentType)
}

// AddQPSData adds or updates QPS data for a component
func (qm *QPSManager) AddQPSData(metrics *QPSMetrics) {
	if metrics == nil {
		return
	}

	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	key := qm.generateKey(metrics.NodeID, metrics.ProjectID, metrics.ComponentID, metrics.ComponentType)

	// Get or create component data
	componentData, exists := qm.data[key]
	if !exists {
		componentData = &ComponentQPSData{
			NodeID:        metrics.NodeID,
			ProjectID:     metrics.ProjectID,
			ComponentID:   metrics.ComponentID,
			ComponentType: metrics.ComponentType,
			DataPoints:    make([]QPSDataPoint, 0),
		}
		qm.data[key] = componentData
	}

	// Add new data point
	dataPoint := QPSDataPoint{
		QPS:       metrics.QPS,
		Timestamp: metrics.Timestamp,
	}

	componentData.DataPoints = append(componentData.DataPoints, dataPoint)
	componentData.LastUpdate = metrics.Timestamp

	// Keep only data from the last hour (3600 seconds)
	cutoffTime := time.Now().Add(-time.Hour)
	var validPoints []QPSDataPoint
	for _, point := range componentData.DataPoints {
		if point.Timestamp.After(cutoffTime) {
			validPoints = append(validPoints, point)
		}
	}
	componentData.DataPoints = validPoints
}

// GetComponentQPS returns QPS data for a specific component
func (qm *QPSManager) GetComponentQPS(nodeID, projectID, componentID, componentType string) *ComponentQPSData {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	key := qm.generateKey(nodeID, projectID, componentID, componentType)
	if data, exists := qm.data[key]; exists {
		// Create a copy to avoid race conditions
		result := &ComponentQPSData{
			NodeID:        data.NodeID,
			ProjectID:     data.ProjectID,
			ComponentID:   data.ComponentID,
			ComponentType: data.ComponentType,
			LastUpdate:    data.LastUpdate,
			DataPoints:    make([]QPSDataPoint, len(data.DataPoints)),
		}
		copy(result.DataPoints, data.DataPoints)
		return result
	}
	return nil
}

// GetProjectQPS returns aggregated QPS data for all components in a project
func (qm *QPSManager) GetProjectQPS(projectID string) map[string]*ComponentQPSData {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	result := make(map[string]*ComponentQPSData)

	for key, data := range qm.data {
		if data.ProjectID == projectID {
			// Create a copy
			copied := &ComponentQPSData{
				NodeID:        data.NodeID,
				ProjectID:     data.ProjectID,
				ComponentID:   data.ComponentID,
				ComponentType: data.ComponentType,
				LastUpdate:    data.LastUpdate,
				DataPoints:    make([]QPSDataPoint, len(data.DataPoints)),
			}
			copy(copied.DataPoints, data.DataPoints)
			result[key] = copied
		}
	}

	return result
}

// GetAllQPS returns all QPS data
func (qm *QPSManager) GetAllQPS() map[string]*ComponentQPSData {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	result := make(map[string]*ComponentQPSData)

	for key, data := range qm.data {
		// Create a copy
		copied := &ComponentQPSData{
			NodeID:        data.NodeID,
			ProjectID:     data.ProjectID,
			ComponentID:   data.ComponentID,
			ComponentType: data.ComponentType,
			LastUpdate:    data.LastUpdate,
			DataPoints:    make([]QPSDataPoint, len(data.DataPoints)),
		}
		copy(copied.DataPoints, data.DataPoints)
		result[key] = copied
	}

	return result
}

// GetAggregatedQPS returns aggregated QPS data across all nodes for each component
func (qm *QPSManager) GetAggregatedQPS(projectID string) map[string]interface{} {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	// Group by component (projectID_componentID_componentType)
	componentGroups := make(map[string][]QPSDataPoint)
	componentTypes := make(map[string]string)

	for _, data := range qm.data {
		if data.ProjectID == projectID {
			componentKey := fmt.Sprintf("%s_%s_%s", data.ProjectID, data.ComponentID, data.ComponentType)
			componentTypes[componentKey] = data.ComponentType

			// Add all data points from this node
			if _, exists := componentGroups[componentKey]; !exists {
				componentGroups[componentKey] = make([]QPSDataPoint, 0)
			}
			componentGroups[componentKey] = append(componentGroups[componentKey], data.DataPoints...)
		}
	}

	// Aggregate QPS by time window (group by minute)
	result := make(map[string]interface{})

	for componentKey, dataPoints := range componentGroups {
		parts := []rune(componentKey)
		if len(parts) < 3 {
			continue
		}

		// Group data points by minute
		minuteGroups := make(map[string][]QPSDataPoint)
		for _, point := range dataPoints {
			minuteKey := point.Timestamp.Truncate(time.Minute).Format("2006-01-02T15:04:05Z")
			if _, exists := minuteGroups[minuteKey]; !exists {
				minuteGroups[minuteKey] = make([]QPSDataPoint, 0)
			}
			minuteGroups[minuteKey] = append(minuteGroups[minuteKey], point)
		}

		// Calculate aggregated QPS for each minute
		aggregatedPoints := make([]map[string]interface{}, 0)
		for minuteKey, points := range minuteGroups {
			totalQPS := uint64(0)
			for _, point := range points {
				totalQPS += point.QPS
			}

			aggregatedPoints = append(aggregatedPoints, map[string]interface{}{
				"timestamp": minuteKey,
				"qps":       totalQPS,
				"nodes":     len(points),
			})
		}

		result[componentKey] = map[string]interface{}{
			"component_type": componentTypes[componentKey],
			"data_points":    aggregatedPoints,
		}
	}

	return result
}

// cleanupLoop periodically removes old data (older than 1 hour)
func (qm *QPSManager) cleanupLoop() {
	defer qm.cleanupWg.Done()

	ticker := time.NewTicker(5 * time.Minute) // Cleanup every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-qm.stopChan:
			return
		case <-ticker.C:
			qm.cleanup()
		}
	}
}

// cleanup removes data older than 1 hour
func (qm *QPSManager) cleanup() {
	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	cutoffTime := time.Now().Add(-time.Hour)

	for key, data := range qm.data {
		// Remove old data points
		var validPoints []QPSDataPoint
		for _, point := range data.DataPoints {
			if point.Timestamp.After(cutoffTime) {
				validPoints = append(validPoints, point)
			}
		}

		if len(validPoints) == 0 {
			// No valid data points, remove the entire component entry
			delete(qm.data, key)
		} else {
			data.DataPoints = validPoints
		}
	}
}

// Stop stops the QPS manager and cleanup goroutine
func (qm *QPSManager) Stop() {
	close(qm.stopChan)
	qm.cleanupWg.Wait()
}

// GetStats returns statistics about the QPS manager
func (qm *QPSManager) GetStats() map[string]interface{} {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	totalComponents := len(qm.data)
	totalDataPoints := 0
	nodeCount := make(map[string]bool)
	projectCount := make(map[string]bool)

	for _, data := range qm.data {
		totalDataPoints += len(data.DataPoints)
		nodeCount[data.NodeID] = true
		projectCount[data.ProjectID] = true
	}

	return map[string]interface{}{
		"total_components":  totalComponents,
		"total_data_points": totalDataPoints,
		"unique_nodes":      len(nodeCount),
		"unique_projects":   len(projectCount),
		"data_retention":    "1 hour",
	}
}

// Global QPS manager instance (only on leader)
var GlobalQPSManager *QPSManager

// InitQPSManager initializes the global QPS manager (only call on leader)
func InitQPSManager() {
	if GlobalQPSManager == nil {
		GlobalQPSManager = NewQPSManager()
	}
}

// StopQPSManager stops the global QPS manager
func StopQPSManager() {
	if GlobalQPSManager != nil {
		GlobalQPSManager.Stop()
		GlobalQPSManager = nil
	}
}
