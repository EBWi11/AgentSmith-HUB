package common

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// QPSMetrics represents QPS data for a specific component
type QPSMetrics struct {
	NodeID              string    `json:"node_id"`
	ProjectID           string    `json:"project_id"`
	ComponentID         string    `json:"component_id"`
	ComponentType       string    `json:"component_type"`        // "input", "output", "ruleset"
	ProjectNodeSequence string    `json:"project_node_sequence"` // e.g., "input.kafka1.ruleset.filter.output.es1"
	QPS                 uint64    `json:"qps"`
	TotalMessages       uint64    `json:"total_messages"` // Real total message count
	Timestamp           time.Time `json:"timestamp"`
}

// QPSDataPoint represents a single QPS measurement
type QPSDataPoint struct {
	QPS           uint64    `json:"qps"`
	TotalMessages uint64    `json:"total_messages"` // Real total message count at this point
	Timestamp     time.Time `json:"timestamp"`
}

// ComponentQPSData holds time series data for a component
type ComponentQPSData struct {
	NodeID              string         `json:"node_id"`
	ProjectID           string         `json:"project_id"`
	ComponentID         string         `json:"component_id"`
	ComponentType       string         `json:"component_type"`
	ProjectNodeSequence string         `json:"project_node_sequence"`
	DataPoints          []QPSDataPoint `json:"data_points"`
	LastUpdate          time.Time      `json:"last_update"`
	CurrentTotal        uint64         `json:"current_total"` // Current total message count
}

// QPSManager manages QPS data collection and aggregation on leader node
type QPSManager struct {
	// Key format: "nodeID_projectNodeSequence" - Use ProjectNodeSequence as primary dimension
	data      map[string]*ComponentQPSData
	mutex     sync.RWMutex
	stopChan  chan struct{}
	cleanupWg sync.WaitGroup

	// Cached message counts (updated every minute)
	cachedMessages     map[string]interface{}
	cachedAggregated   map[string]interface{}
	cachedNodeMessages map[string]interface{} // nodeID -> message stats
	cacheUpdateTime    time.Time
	cacheMutex         sync.RWMutex
	cacheWg            sync.WaitGroup
}

// NewQPSManager creates a new QPS manager instance
func NewQPSManager() *QPSManager {
	qm := &QPSManager{
		data:               make(map[string]*ComponentQPSData),
		stopChan:           make(chan struct{}),
		cachedMessages:     make(map[string]interface{}),
		cachedAggregated:   make(map[string]interface{}),
		cachedNodeMessages: make(map[string]interface{}),
		cacheUpdateTime:    time.Now(),
	}

	// Start cleanup goroutine to remove old data
	qm.cleanupWg.Add(1)
	go qm.cleanupLoop()

	// Start cache update goroutine
	qm.cacheWg.Add(1)
	go qm.cacheUpdateLoop()

	return qm
}

// generateKey creates a unique key for component QPS data based on ProjectNodeSequence
func (qm *QPSManager) generateKey(nodeID, projectNodeSequence string) string {
	return fmt.Sprintf("%s_%s", nodeID, projectNodeSequence)
}

// AddQPSData adds or updates QPS data for a component
func (qm *QPSManager) AddQPSData(metrics *QPSMetrics) {
	if metrics == nil {
		return
	}

	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	key := qm.generateKey(metrics.NodeID, metrics.ProjectNodeSequence)

	// Get or create component data
	componentData, exists := qm.data[key]
	if !exists {
		componentData = &ComponentQPSData{
			NodeID:              metrics.NodeID,
			ProjectID:           metrics.ProjectID,
			ComponentID:         metrics.ComponentID,
			ComponentType:       metrics.ComponentType,
			ProjectNodeSequence: metrics.ProjectNodeSequence,
			DataPoints:          make([]QPSDataPoint, 0),
		}
		qm.data[key] = componentData
	}

	// Add new data point
	dataPoint := QPSDataPoint{
		QPS:           metrics.QPS,
		TotalMessages: metrics.TotalMessages,
		Timestamp:     metrics.Timestamp,
	}

	componentData.DataPoints = append(componentData.DataPoints, dataPoint)
	componentData.LastUpdate = metrics.Timestamp
	componentData.CurrentTotal = metrics.TotalMessages // Update current total

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

// GetComponentQPS returns QPS data for a specific component by ProjectNodeSequence
func (qm *QPSManager) GetComponentQPS(nodeID, projectNodeSequence string) *ComponentQPSData {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	key := qm.generateKey(nodeID, projectNodeSequence)
	if data, exists := qm.data[key]; exists {
		// Create a copy to avoid race conditions
		result := &ComponentQPSData{
			NodeID:              data.NodeID,
			ProjectID:           data.ProjectID,
			ComponentID:         data.ComponentID,
			ComponentType:       data.ComponentType,
			ProjectNodeSequence: data.ProjectNodeSequence,
			LastUpdate:          data.LastUpdate,
			DataPoints:          make([]QPSDataPoint, len(data.DataPoints)),
		}
		copy(result.DataPoints, data.DataPoints)
		return result
	}
	return nil
}

// GetComponentQPSLegacy returns QPS data for a specific component using legacy parameters
// This method provides backward compatibility for API calls that use the old parameter format
func (qm *QPSManager) GetComponentQPSLegacy(nodeID, projectID, componentID, componentType string) *ComponentQPSData {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	// Search for matching component data by iterating through all entries
	// Since ProjectNodeSequence can be complex (e.g., "input.kafka1.ruleset.filter.output.es1"),
	// we need to find the entry that matches the legacy parameters
	for _, data := range qm.data {
		if data.NodeID == nodeID &&
			data.ProjectID == projectID &&
			data.ComponentID == componentID &&
			data.ComponentType == componentType {
			// Found matching component, create a copy
			result := &ComponentQPSData{
				NodeID:              data.NodeID,
				ProjectID:           data.ProjectID,
				ComponentID:         data.ComponentID,
				ComponentType:       data.ComponentType,
				ProjectNodeSequence: data.ProjectNodeSequence,
				LastUpdate:          data.LastUpdate,
				DataPoints:          make([]QPSDataPoint, len(data.DataPoints)),
			}
			copy(result.DataPoints, data.DataPoints)
			return result
		}
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
				NodeID:              data.NodeID,
				ProjectID:           data.ProjectID,
				ComponentID:         data.ComponentID,
				ComponentType:       data.ComponentType,
				ProjectNodeSequence: data.ProjectNodeSequence,
				LastUpdate:          data.LastUpdate,
				DataPoints:          make([]QPSDataPoint, len(data.DataPoints)),
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
			NodeID:              data.NodeID,
			ProjectID:           data.ProjectID,
			ComponentID:         data.ComponentID,
			ComponentType:       data.ComponentType,
			ProjectNodeSequence: data.ProjectNodeSequence,
			LastUpdate:          data.LastUpdate,
			DataPoints:          make([]QPSDataPoint, len(data.DataPoints)),
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

	// Group by ProjectNodeSequence instead of component only
	sequenceGroups := make(map[string][]QPSDataPoint)
	sequenceTypes := make(map[string]string)

	for _, data := range qm.data {
		if data.ProjectID == projectID {
			sequenceKey := data.ProjectNodeSequence
			sequenceTypes[sequenceKey] = data.ComponentType

			// Add all data points from this node
			if _, exists := sequenceGroups[sequenceKey]; !exists {
				sequenceGroups[sequenceKey] = make([]QPSDataPoint, 0)
			}
			sequenceGroups[sequenceKey] = append(sequenceGroups[sequenceKey], data.DataPoints...)
		}
	}

	// Aggregate QPS by time window (group by minute)
	result := make(map[string]interface{})

	for sequenceKey, dataPoints := range sequenceGroups {
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

		result[sequenceKey] = map[string]interface{}{
			"component_type":        sequenceTypes[sequenceKey],
			"project_node_sequence": sequenceKey,
			"data_points":           aggregatedPoints,
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

// cacheUpdateLoop periodically updates the cached message counts every minute
func (qm *QPSManager) cacheUpdateLoop() {
	defer qm.cacheWg.Done()

	ticker := time.NewTicker(1 * time.Minute) // Update cache every minute
	defer ticker.Stop()

	// Initial cache update
	qm.updateMessageCache()

	for {
		select {
		case <-qm.stopChan:
			return
		case <-ticker.C:
			qm.updateMessageCache()
		}
	}
}

// updateMessageCache updates the cached message counts
func (qm *QPSManager) updateMessageCache() {
	// Calculate message counts for all projects
	allMessages := qm.calculateHourlyMessageCounts("")
	aggregatedMessages := qm.calculateAggregatedHourlyMessages()
	nodeMessages := qm.calculateNodeHourlyMessages()

	// Update cache with write lock
	qm.cacheMutex.Lock()
	qm.cachedMessages = allMessages
	qm.cachedAggregated = aggregatedMessages
	qm.cachedNodeMessages = nodeMessages
	qm.cacheUpdateTime = time.Now()
	qm.cacheMutex.Unlock()
}

// Stop stops the QPS manager and cleanup goroutine
func (qm *QPSManager) Stop() {
	close(qm.stopChan)
	qm.cleanupWg.Wait()
	qm.cacheWg.Wait()
}

// calculateHourlyMessageCounts calculates the real message counts for the past hour (internal method)
func (qm *QPSManager) calculateHourlyMessageCounts(projectID string) map[string]interface{} {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	// Group by ProjectNodeSequence
	sequenceGroups := make(map[string]*ComponentQPSData)
	sequenceTypes := make(map[string]string)

	for _, data := range qm.data {
		if projectID == "" || data.ProjectID == projectID {
			sequenceKey := data.ProjectNodeSequence
			sequenceTypes[sequenceKey] = data.ComponentType

			// Use the latest component data for each sequence
			if existing, exists := sequenceGroups[sequenceKey]; !exists || data.LastUpdate.After(existing.LastUpdate) {
				sequenceGroups[sequenceKey] = data
			}
		}
	}

	result := make(map[string]interface{})

	for sequenceKey, componentData := range sequenceGroups {
		// Use real current total messages - this is the actual cumulative count
		totalMessages := componentData.CurrentTotal

		// Calculate hourly message rate based on recent data points (for rate calculation)
		var hourlyRate uint64 = 0
		if len(componentData.DataPoints) > 1 {
			// Find the oldest and newest data points within the hour
			oldestPoint := componentData.DataPoints[0]
			newestPoint := componentData.DataPoints[len(componentData.DataPoints)-1]

			// Calculate time difference in hours
			timeDiff := newestPoint.Timestamp.Sub(oldestPoint.Timestamp).Hours()
			if timeDiff > 0 {
				messageDiff := newestPoint.TotalMessages - oldestPoint.TotalMessages
				hourlyRate = uint64(float64(messageDiff) / timeDiff)
			}
		}

		result[sequenceKey] = map[string]interface{}{
			"component_type":        sequenceTypes[sequenceKey],
			"project_node_sequence": sequenceKey,
			"total_messages":        totalMessages, // Real cumulative total
			"hourly_rate":           hourlyRate,    // Messages per hour rate
			"data_points_count":     len(componentData.DataPoints),
		}
	}

	return result
}

// calculateAggregatedHourlyMessages calculates aggregated message counts across all nodes (internal method)
func (qm *QPSManager) calculateAggregatedHourlyMessages() map[string]interface{} {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	// Group by project and component type for individual component statistics
	projectStats := make(map[string]map[string]uint64) // projectID -> componentType -> hourlyRate
	totalInputMessages := uint64(0)                    // Total hourly input messages across all projects
	totalOutputMessages := uint64(0)                   // Total hourly output messages across all projects
	totalRulesetMessages := uint64(0)                  // Total hourly ruleset processed messages

	for _, data := range qm.data {
		if _, exists := projectStats[data.ProjectID]; !exists {
			projectStats[data.ProjectID] = make(map[string]uint64)
		}

		// Calculate hourly message rate based on recent data points
		var hourlyRate uint64 = 0
		if len(data.DataPoints) > 1 {
			// Find the oldest and newest data points within the hour
			oldestPoint := data.DataPoints[0]
			newestPoint := data.DataPoints[len(data.DataPoints)-1]

			// Calculate time difference in hours
			timeDiff := newestPoint.Timestamp.Sub(oldestPoint.Timestamp).Hours()
			if timeDiff > 0 {
				messageDiff := newestPoint.TotalMessages - oldestPoint.TotalMessages
				hourlyRate = uint64(float64(messageDiff) / timeDiff)
			}
		}

		// Count hourly rates for individual statistics
		projectStats[data.ProjectID][data.ComponentType] += hourlyRate

		// For global totals, count hourly rates by component type
		parts := strings.Split(data.ProjectNodeSequence, ".")

		switch data.ComponentType {
		case "input":
			// Only count input if this ProjectNodeSequence represents the actual input component
			if len(parts) == 2 && parts[0] == "input" && parts[1] == data.ComponentID {
				totalInputMessages += hourlyRate
			}
		case "output":
			// Only count output if this ProjectNodeSequence represents the final output component
			if len(parts) >= 2 && parts[len(parts)-2] == "output" && parts[len(parts)-1] == data.ComponentID {
				totalOutputMessages += hourlyRate
			}
		case "ruleset":
			// Now count ruleset processing - this shows hourly processing rate
			totalRulesetMessages += hourlyRate
		}
	}

	return map[string]interface{}{
		"total_messages":         totalInputMessages + totalOutputMessages + totalRulesetMessages, // All hourly messages
		"total_input_messages":   totalInputMessages,                                              // Hourly input messages
		"total_output_messages":  totalOutputMessages,                                             // Hourly output messages
		"total_ruleset_messages": totalRulesetMessages,                                            // Hourly ruleset processed messages
		"project_breakdown":      projectStats,                                                    // Includes all component hourly rates
	}
}

// GetHourlyMessageCounts returns cached message counts for the past hour
func (qm *QPSManager) GetHourlyMessageCounts(projectID string) map[string]interface{} {
	qm.cacheMutex.RLock()
	defer qm.cacheMutex.RUnlock()

	if projectID == "" {
		// Return all cached messages
		return qm.cachedMessages
	}

	// Filter cached data for specific project
	// Since the cached data keys are ProjectNodeSequence, we need to look up the original data
	// to get the correct ProjectID mapping
	result := make(map[string]interface{})

	// We need to access the original data to get accurate project filtering
	qm.mutex.RLock()
	for _, data := range qm.data {
		if data.ProjectID == projectID {
			// Check if this ProjectNodeSequence exists in cached messages
			if cachedData, exists := qm.cachedMessages[data.ProjectNodeSequence]; exists {
				result[data.ProjectNodeSequence] = cachedData
			}
		}
	}
	qm.mutex.RUnlock()

	return result
}

// GetAggregatedHourlyMessages returns cached aggregated message counts across all nodes
func (qm *QPSManager) GetAggregatedHourlyMessages() map[string]interface{} {
	qm.cacheMutex.RLock()
	defer qm.cacheMutex.RUnlock()

	return qm.cachedAggregated
}

// GetCacheUpdateTime returns the last cache update time
func (qm *QPSManager) GetCacheUpdateTime() time.Time {
	qm.cacheMutex.RLock()
	defer qm.cacheMutex.RUnlock()

	return qm.cacheUpdateTime
}

// calculateNodeHourlyMessages calculates message counts by node (internal method)
func (qm *QPSManager) calculateNodeHourlyMessages() map[string]interface{} {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	nodeStats := make(map[string]map[string]uint64) // nodeID -> componentType -> hourlyRate

	for _, data := range qm.data {
		if _, exists := nodeStats[data.NodeID]; !exists {
			nodeStats[data.NodeID] = make(map[string]uint64)
		}

		// Calculate hourly message rate based on recent data points
		var hourlyRate uint64 = 0
		if len(data.DataPoints) > 1 {
			// Find the oldest and newest data points within the hour
			oldestPoint := data.DataPoints[0]
			newestPoint := data.DataPoints[len(data.DataPoints)-1]

			// Calculate time difference in hours
			timeDiff := newestPoint.Timestamp.Sub(oldestPoint.Timestamp).Hours()
			if timeDiff > 0 {
				messageDiff := newestPoint.TotalMessages - oldestPoint.TotalMessages
				hourlyRate = uint64(float64(messageDiff) / timeDiff)
			}
		}

		// Count hourly rates for node statistics - shows actual message processing rate
		nodeStats[data.NodeID][data.ComponentType] += hourlyRate
	}

	// Convert to final format
	result := make(map[string]interface{})
	for nodeID, stats := range nodeStats {
		inputMessages := stats["input"]
		outputMessages := stats["output"]
		rulesetMessages := stats["ruleset"] // Hourly ruleset processing rate

		result[nodeID] = map[string]interface{}{
			"input_messages":   inputMessages,
			"output_messages":  outputMessages,
			"ruleset_messages": rulesetMessages,                                  // Hourly ruleset processing rate
			"total_messages":   inputMessages + outputMessages + rulesetMessages, // Total hourly message processing rate
		}
	}

	return result
}

// GetNodeHourlyMessages returns cached message counts for a specific node
func (qm *QPSManager) GetNodeHourlyMessages(nodeID string) map[string]interface{} {
	qm.cacheMutex.RLock()
	defer qm.cacheMutex.RUnlock()

	if nodeData, exists := qm.cachedNodeMessages[nodeID]; exists {
		return nodeData.(map[string]interface{})
	}

	// Return empty result if node not found
	return map[string]interface{}{
		"input_messages":   uint64(0),
		"output_messages":  uint64(0),
		"ruleset_messages": uint64(0),
		"total_messages":   uint64(0),
	}
}

// GetAllNodeHourlyMessages returns cached message counts for all nodes
func (qm *QPSManager) GetAllNodeHourlyMessages() map[string]interface{} {
	qm.cacheMutex.RLock()
	defer qm.cacheMutex.RUnlock()

	return qm.cachedNodeMessages
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
