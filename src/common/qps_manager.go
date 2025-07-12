package common

import (
	"AgentSmith-HUB/logger"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// QPSMetrics represents QPS metrics for a single component
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

	// Redis Pub/Sub
	pubsub  *redis.PubSub
	stopSub chan struct{}
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
		cacheWg:            sync.WaitGroup{},
		stopSub:            make(chan struct{}),
	}

	// Load existing data from Redis via Daily Stats Manager
	qm.loadFromRedis()

	// Start cleanup goroutine to remove old data
	qm.cleanupWg.Add(1)
	go qm.cleanupLoop()

	// Start cache update goroutine
	qm.cacheWg.Add(1)
	go qm.cacheUpdateLoop()

	// Start Redis subscriber for follower metrics
	qm.startRedisSubscriber()

	return qm
}

// loadFromRedis loads historical data from Redis via Daily Stats Manager
func (qm *QPSManager) loadFromRedis() {
	if GlobalDailyStatsManager == nil {
		return
	}

	// Get today's data from Daily Stats Manager
	today := time.Now().Format("2006-01-02")
	dailyStats := GlobalDailyStatsManager.GetDailyStats(today, "", "")

	loadedComponents := 0
	for _, statsData := range dailyStats {
		// Create component data based on daily stats
		key := qm.generateKey(statsData.NodeID, statsData.ProjectID, statsData.ProjectNodeSequence)

		// Only load if we don't already have this component
		if _, exists := qm.data[key]; !exists {
			componentData := &ComponentQPSData{
				NodeID:              statsData.NodeID,
				ProjectID:           statsData.ProjectID,
				ComponentID:         statsData.ComponentID,
				ComponentType:       statsData.ComponentType,
				ProjectNodeSequence: statsData.ProjectNodeSequence,
				DataPoints:          make([]QPSDataPoint, 0),
				LastUpdate:          statsData.LastUpdate,
				CurrentTotal:        statsData.TotalMessages,
			}

			// Add a single data point representing the current state
			if statsData.TotalMessages > 0 {
				dataPoint := QPSDataPoint{
					QPS:           0, // We don't have QPS data from daily stats
					TotalMessages: statsData.TotalMessages,
					Timestamp:     statsData.LastUpdate,
				}
				componentData.DataPoints = append(componentData.DataPoints, dataPoint)
			}

			qm.data[key] = componentData
			loadedComponents++
		}
	}

	if loadedComponents > 0 {
		logger.Info("QPS Manager loaded data from Redis", "components", loadedComponents)
	}
}

// generateKey creates unique map key; include projectID to avoid collisions when multiple projects
// share the same ProjectNodeSequence (e.g., shared INPUT).
func (qm *QPSManager) generateKey(nodeID, projectID, projectNodeSequence string) string {
	return fmt.Sprintf("%s_%s_%s", nodeID, projectID, projectNodeSequence)
}

// AddQPSData adds or updates QPS data for a component
func (qm *QPSManager) AddQPSData(metrics *QPSMetrics) {
	if metrics == nil {
		return
	}

	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	key := qm.generateKey(metrics.NodeID, metrics.ProjectID, metrics.ProjectNodeSequence)

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

	// Update daily statistics in Redis
	if GlobalDailyStatsManager != nil {
		GlobalDailyStatsManager.UpdateDailyStats(
			metrics.NodeID,
			metrics.ProjectID,
			metrics.ComponentID,
			metrics.ComponentType,
			metrics.ProjectNodeSequence,
			metrics.TotalMessages,
		)
	}

	// Keep only data from the last hour (3600 seconds) and ensure sorted order
	cutoffTime := time.Now().Add(-time.Hour)
	var validPoints []QPSDataPoint
	for _, point := range componentData.DataPoints {
		if point.Timestamp.After(cutoffTime) {
			validPoints = append(validPoints, point)
		}
	}

	// Sort data points by timestamp to ensure correct ordering
	sort.Slice(validPoints, func(i, j int) bool {
		return validPoints[i].Timestamp.Before(validPoints[j].Timestamp)
	})

	componentData.DataPoints = validPoints
}

// GetComponentQPS returns QPS data for a specific component by ProjectNodeSequence
func (qm *QPSManager) GetComponentQPS(nodeID, projectNodeSequence string) *ComponentQPSData {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	// Search for matching component data since we don't have projectID in this call
	// This is needed because the key format includes projectID: nodeID_projectID_projectNodeSequence
	for _, data := range qm.data {
		if data.NodeID == nodeID && data.ProjectNodeSequence == projectNodeSequence {
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

// cleanupLoop periodically removes old data (older than 1 hour in memory)
func (qm *QPSManager) cleanupLoop() {
	defer qm.cleanupWg.Done()

	ticker := time.NewTicker(5 * time.Minute) // Cleanup every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-qm.stopChan:
			return
		case <-ticker.C:
			qm.cleanup() // Clean memory data
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
	// Calculate message counts for all projects - simple and clear
	allMessages := qm.calculateDailyMessageCounts("")
	aggregatedMessages := qm.calculateAggregatedDailyMessages()
	nodeMessages := qm.calculateNodeDailyMessages()

	// Update cache with write lock
	qm.cacheMutex.Lock()
	qm.cachedMessages = allMessages
	qm.cachedAggregated = aggregatedMessages
	qm.cachedNodeMessages = nodeMessages
	qm.cacheUpdateTime = time.Now()
	qm.cacheMutex.Unlock()
}

// Stop stops the QPS manager and all goroutines
func (qm *QPSManager) Stop() {
	close(qm.stopChan)
	qm.cleanupWg.Wait()
	qm.cacheWg.Wait()

	if qm.stopSub != nil {
		close(qm.stopSub)
	}
}

// calculateDailyMessageCounts calculates daily message counts (simplified)
func (qm *QPSManager) calculateDailyMessageCounts(projectID string) map[string]interface{} {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	// Group by ProjectNodeSequence + NodeID to avoid data loss from multiple nodes
	// Key format: "projectNodeSequence_nodeID" to distinguish same sequence on different nodes
	sequenceGroups := make(map[string]*ComponentQPSData)
	sequenceTypes := make(map[string]string)

	for _, data := range qm.data {
		if projectID == "" || data.ProjectID == projectID {
			// Include nodeID in the grouping key to avoid overwriting data from different nodes
			groupKey := fmt.Sprintf("%s_%s", data.ProjectNodeSequence, data.NodeID)
			sequenceTypes[groupKey] = data.ComponentType
			sequenceGroups[groupKey] = data
		}
	}

	// Now aggregate results by ProjectNodeSequence (combine all nodes for same sequence)
	sequenceAggregated := make(map[string]map[string]interface{})

	for groupKey, componentData := range sequenceGroups {
		// Extract ProjectNodeSequence from groupKey (remove the nodeID suffix)
		parts := strings.Split(groupKey, "_")
		sequenceKey := strings.Join(parts[:len(parts)-1], "_") // Everything before the last underscore

		// Use real current total messages - this is the actual cumulative count
		totalMessages := componentData.CurrentTotal

		// Simple approach: use the real total messages for today (MSG/D)
		dailyMessages := totalMessages

		// Aggregate by sequence (sum across all nodes for the same sequence)
		if _, exists := sequenceAggregated[sequenceKey]; !exists {
			sequenceAggregated[sequenceKey] = map[string]interface{}{
				"component_type":        sequenceTypes[groupKey],
				"project_node_sequence": sequenceKey,
				"daily_messages":        uint64(0), // Real MSG/D - simple and clear
				"node_count":            0,
			}
		}

		agg := sequenceAggregated[sequenceKey]
		agg["daily_messages"] = agg["daily_messages"].(uint64) + dailyMessages // Sum daily messages across nodes
		agg["node_count"] = agg["node_count"].(int) + 1
	}

	result := make(map[string]interface{})
	for sequenceKey, aggregatedData := range sequenceAggregated {
		result[sequenceKey] = aggregatedData
	}

	return result
}

// calculateAggregatedDailyMessages calculates aggregated message counts across all nodes (simplified)
func (qm *QPSManager) calculateAggregatedDailyMessages() map[string]interface{} {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	// Simple aggregation by project and component type
	projectStats := make(map[string]map[string]uint64) // projectID -> componentType -> dailyMessages
	totalInputMessages := uint64(0)
	totalOutputMessages := uint64(0)
	totalRulesetMessages := uint64(0)

	for _, data := range qm.data {
		if _, exists := projectStats[data.ProjectID]; !exists {
			projectStats[data.ProjectID] = make(map[string]uint64)
		}

		// Simple: use current total as daily message count
		dailyMessages := data.CurrentTotal
		projectStats[data.ProjectID][data.ComponentType] += dailyMessages

		// Global totals by component type
		switch data.ComponentType {
		case "input":
			totalInputMessages += dailyMessages
		case "output":
			totalOutputMessages += dailyMessages
		case "ruleset":
			totalRulesetMessages += dailyMessages
		}
	}

	return map[string]interface{}{
		"total_messages":         totalInputMessages + totalOutputMessages + totalRulesetMessages,
		"total_input_messages":   totalInputMessages,
		"total_output_messages":  totalOutputMessages,
		"total_ruleset_messages": totalRulesetMessages,
		"project_breakdown":      projectStats,
	}
}

// GetDailyMessageCounts returns cached daily message counts
func (qm *QPSManager) GetDailyMessageCounts(projectID string) map[string]interface{} {
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

// GetAggregatedDailyMessages returns cached aggregated message counts across all nodes
func (qm *QPSManager) GetAggregatedDailyMessages() map[string]interface{} {
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

// calculateNodeDailyMessages calculates message counts by node (simplified)
func (qm *QPSManager) calculateNodeDailyMessages() map[string]interface{} {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	nodeStats := make(map[string]map[string]uint64) // nodeID -> componentType -> dailyMessages

	for _, data := range qm.data {
		if _, exists := nodeStats[data.NodeID]; !exists {
			nodeStats[data.NodeID] = make(map[string]uint64)
		}

		// Simple: use current total as daily message count
		dailyMessages := data.CurrentTotal
		nodeStats[data.NodeID][data.ComponentType] += dailyMessages
	}

	// Convert to final format
	result := make(map[string]interface{})
	for nodeID, stats := range nodeStats {
		inputMessages := stats["input"]
		outputMessages := stats["output"]
		rulesetMessages := stats["ruleset"]

		result[nodeID] = map[string]interface{}{
			"input_messages":   inputMessages,
			"output_messages":  outputMessages,
			"ruleset_messages": rulesetMessages,
			"total_messages":   inputMessages + outputMessages + rulesetMessages,
		}
	}

	return result
}

// GetNodeDailyMessages returns cached message counts for a specific node
func (qm *QPSManager) GetNodeDailyMessages(nodeID string) map[string]interface{} {
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

// GetAllNodeDailyMessages returns cached message counts for all nodes
func (qm *QPSManager) GetAllNodeDailyMessages() map[string]interface{} {
	qm.cacheMutex.RLock()
	defer qm.cacheMutex.RUnlock()

	return qm.cachedNodeMessages
}

// GetStats returns statistics about the QPS manager (legacy method)
func (qm *QPSManager) GetStats() map[string]interface{} {
	return qm.GetQPSStats()
}

// Global QPS manager instance (used by all nodes)
var GlobalQPSManager *QPSManager

// InitQPSManager initializes the global QPS manager (call on all nodes)
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

// GetQPSStats returns statistics about QPS data storage
func (qm *QPSManager) GetQPSStats() map[string]interface{} {
	qm.mutex.RLock()
	defer qm.mutex.RUnlock()

	totalComponents := len(qm.data)
	totalDataPoints := 0
	nodeCount := make(map[string]bool)
	projectCount := make(map[string]bool)
	oldestTimestamp := time.Now()
	newestTimestamp := time.Time{}

	for _, data := range qm.data {
		totalDataPoints += len(data.DataPoints)
		nodeCount[data.NodeID] = true
		projectCount[data.ProjectID] = true

		// Find oldest and newest timestamps
		for _, point := range data.DataPoints {
			if point.Timestamp.Before(oldestTimestamp) {
				oldestTimestamp = point.Timestamp
			}
			if point.Timestamp.After(newestTimestamp) {
				newestTimestamp = point.Timestamp
			}
		}
	}

	// Check if Daily Stats Manager is available for Redis persistence
	redisEnabled := GlobalDailyStatsManager != nil
	var redisRetentionDays int
	var redisSaveInterval string

	if redisEnabled {
		dailyStatsInfo := GlobalDailyStatsManager.GetStats()
		if retentionDays, ok := dailyStatsInfo["retention_days"].(int); ok {
			redisRetentionDays = retentionDays
		}
		if saveInterval, ok := dailyStatsInfo["save_interval"].(string); ok {
			redisSaveInterval = saveInterval
		}
	}

	stats := map[string]interface{}{
		"total_components":      totalComponents,
		"total_data_points":     totalDataPoints,
		"unique_nodes":          len(nodeCount),
		"unique_projects":       len(projectCount),
		"memory_data_retention": "1 hour",
		"redis_persistence":     redisEnabled,
		"redis_data_retention":  fmt.Sprintf("%d days", redisRetentionDays),
		"redis_save_interval":   redisSaveInterval,
	}

	if totalDataPoints > 0 {
		stats["oldest_data"] = oldestTimestamp
		stats["newest_data"] = newestTimestamp
		stats["data_span"] = newestTimestamp.Sub(oldestTimestamp).String()
	}

	return stats
}

func (qm *QPSManager) startRedisSubscriber() {
	// Remove complex Redis pub/sub logic
	// Each node only manages its own data now
	return
}
