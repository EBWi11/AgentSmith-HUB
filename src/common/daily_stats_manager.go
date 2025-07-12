package common

import (
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DailyStatsData represents daily statistics for a component
type DailyStatsData struct {
	NodeID              string    `json:"node_id"`
	ProjectID           string    `json:"project_id"`
	ComponentID         string    `json:"component_id"`
	ComponentType       string    `json:"component_type"`
	ProjectNodeSequence string    `json:"project_node_sequence"`
	Date                string    `json:"date"`           // Format: 2006-01-02
	TotalMessages       uint64    `json:"total_messages"` // Total messages for this date
	LastUpdate          time.Time `json:"last_update"`
}

// DailyStatsManager manages daily message statistics with Redis persistence
type DailyStatsManager struct {
	data     map[string]*DailyStatsData // Key: "date_nodeID_projectNodeSequence"
	mutex    sync.RWMutex
	stopChan chan struct{}
	saveWg   sync.WaitGroup

	// Redis settings
	redisEnabled   bool
	redisKeyPrefix string
	saveInterval   time.Duration
	retentionDays  int
}

// NewDailyStatsManager creates a new daily statistics manager instance
func NewDailyStatsManager() *DailyStatsManager {
	dsm := &DailyStatsManager{
		data:           make(map[string]*DailyStatsData),
		stopChan:       make(chan struct{}),
		redisEnabled:   true,               // Default: enabled
		redisKeyPrefix: "hub:daily_stats:", // Redis key prefix
		saveInterval:   5 * time.Minute,    // Save every 5 minutes
		retentionDays:  30,                 // Keep 30 days of data
	}

	// Load existing data from Redis
	dsm.loadFromRedis()

	// Start persistence goroutine
	if dsm.redisEnabled {
		dsm.saveWg.Add(1)
		go dsm.persistenceLoop()
	}

	return dsm
}

// generateKey creates a unique key for daily statistics. We must include projectID so that
// multiple projects共享同一个 ProjectNodeSequence 时不会互相覆盖。
func (dsm *DailyStatsManager) generateKey(date, nodeID, projectID, projectNodeSequence string) string {
	return fmt.Sprintf("%s_%s_%s_%s", date, nodeID, projectID, projectNodeSequence)
}

// UpdateDailyStats updates daily statistics for a component
func (dsm *DailyStatsManager) UpdateDailyStats(nodeID, projectID, componentID, componentType, projectNodeSequence string, totalMessages uint64) {
	now := time.Now()
	date := now.Format("2006-01-02")
	key := dsm.generateKey(date, nodeID, projectID, projectNodeSequence)

	dsm.mutex.Lock()
	defer dsm.mutex.Unlock()

	var statsData *DailyStatsData
	if existing, exists := dsm.data[key]; exists {
		// Fix: Always use the maximum value to avoid duplicate counting
		// When component restarts, the component's counter is set to the previous total
		// So we should always take the maximum value, not add them together
		if totalMessages > existing.TotalMessages {
			// Normal monotonic increase – just overwrite
			existing.TotalMessages = totalMessages
			logger.Debug("Daily stats updated (normal increase)",
				"component", componentID,
				"sequence", projectNodeSequence,
				"old_total", existing.TotalMessages,
				"new_total", totalMessages)
		} else if totalMessages == existing.TotalMessages {
			// No change in total messages, just update timestamp
			logger.Debug("Daily stats updated (no change)",
				"component", componentID,
				"sequence", projectNodeSequence,
				"total", totalMessages)
		} else {
			// totalMessages < existing.TotalMessages
			// This could happen if:
			// 1. Component counter overflow (very unlikely with uint64)
			// 2. Component was forcibly restarted and counter reset to 0
			// 3. Time synchronization issues
			// 4. Data corruption

			// For safety, we keep the existing higher value to avoid data loss
			// But log a warning for investigation
			logger.Warn("Daily stats counter decreased - possible component restart or data issue",
				"component", componentID,
				"sequence", projectNodeSequence,
				"previous_total", existing.TotalMessages,
				"current_total", totalMessages,
				"keeping_previous", true)

			// Keep the existing value unchanged
			// existing.TotalMessages remains unchanged
		}
		existing.LastUpdate = now
		statsData = existing
	} else {
		// Create new data
		statsData = &DailyStatsData{
			NodeID:              nodeID,
			ProjectID:           projectID,
			ComponentID:         componentID,
			ComponentType:       componentType,
			ProjectNodeSequence: projectNodeSequence,
			Date:                date,
			TotalMessages:       totalMessages,
			LastUpdate:          now,
		}
		dsm.data[key] = statsData
		logger.Debug("Created new daily stats entry",
			"component", componentID,
			"sequence", projectNodeSequence,
			"total", totalMessages)
	}

	// Immediately save this specific record to Redis for real-time data availability
	if dsm.redisEnabled {
		go dsm.saveToRedisSingle(key, statsData)
	}
}

// GetDailyStats returns daily statistics for a specific date and optional filters
// This method now reads directly from Redis to get real-time cluster data
func (dsm *DailyStatsManager) GetDailyStats(date, projectID, nodeID string) map[string]*DailyStatsData {
	if !dsm.redisEnabled {
		// Fallback to memory data if Redis is disabled
		return dsm.getMemoryStats(date, projectID, nodeID)
	}

	// Read directly from Redis to get real-time cluster data
	result := make(map[string]*DailyStatsData)

	// Use default date if not specified
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	// Build Redis key pattern based on filters
	var pattern string
	if nodeID != "" && projectID != "" {
		// Most specific: date_nodeID_projectID_*
		pattern = fmt.Sprintf("%s%s_%s_%s_*", dsm.redisKeyPrefix, date, nodeID, projectID)
	} else if nodeID != "" {
		// Node specific: date_nodeID_*
		pattern = fmt.Sprintf("%s%s_%s_*", dsm.redisKeyPrefix, date, nodeID)
	} else if projectID != "" {
		// Project specific: date_*_projectID_*
		pattern = fmt.Sprintf("%s%s_*_%s_*", dsm.redisKeyPrefix, date, projectID)
	} else {
		// All data for the date: date_*
		pattern = fmt.Sprintf("%s%s_*", dsm.redisKeyPrefix, date)
	}

	keys, err := RedisKeys(pattern)
	if err != nil {
		logger.Error("Failed to get daily stats keys from Redis", "pattern", pattern, "error", err)
		// Fallback to memory data
		return dsm.getMemoryStats(date, projectID, nodeID)
	}

	for _, key := range keys {
		jsonData, err := RedisGet(key)
		if err != nil {
			logger.Error("Failed to get daily stats from Redis", "key", key, "error", err)
			continue
		}

		var statsData DailyStatsData
		if err := json.Unmarshal([]byte(jsonData), &statsData); err != nil {
			logger.Error("Failed to unmarshal daily stats from Redis", "key", key, "error", err)
			continue
		}

		// Apply additional filters
		if date != "" && statsData.Date != date {
			continue
		}
		if projectID != "" && statsData.ProjectID != projectID {
			continue
		}
		if nodeID != "" && statsData.NodeID != nodeID {
			continue
		}

		// Generate internal key for result map
		internalKey := dsm.generateKey(statsData.Date, statsData.NodeID, statsData.ProjectID, statsData.ProjectNodeSequence)

		// Create a copy to prevent external modification
		result[internalKey] = &DailyStatsData{
			NodeID:              statsData.NodeID,
			ProjectID:           statsData.ProjectID,
			ComponentID:         statsData.ComponentID,
			ComponentType:       statsData.ComponentType,
			ProjectNodeSequence: statsData.ProjectNodeSequence,
			Date:                statsData.Date,
			TotalMessages:       statsData.TotalMessages,
			LastUpdate:          statsData.LastUpdate,
		}
	}

	return result
}

// getMemoryStats returns daily statistics from memory (fallback method)
func (dsm *DailyStatsManager) getMemoryStats(date, projectID, nodeID string) map[string]*DailyStatsData {
	dsm.mutex.RLock()
	defer dsm.mutex.RUnlock()

	result := make(map[string]*DailyStatsData)

	for key, data := range dsm.data {
		// Filter by date
		if date != "" && data.Date != date {
			continue
		}

		// Filter by project ID
		if projectID != "" && data.ProjectID != projectID {
			continue
		}

		// Filter by node ID
		if nodeID != "" && data.NodeID != nodeID {
			continue
		}

		// Create a copy to prevent external modification
		result[key] = &DailyStatsData{
			NodeID:              data.NodeID,
			ProjectID:           data.ProjectID,
			ComponentID:         data.ComponentID,
			ComponentType:       data.ComponentType,
			ProjectNodeSequence: data.ProjectNodeSequence,
			Date:                data.Date,
			TotalMessages:       data.TotalMessages,
			LastUpdate:          data.LastUpdate,
		}
	}

	return result
}

// loadFromRedis loads existing daily statistics from Redis
func (dsm *DailyStatsManager) loadFromRedis() {
	if !dsm.redisEnabled {
		return
	}

	logger.Info("Loading daily statistics from Redis")

	pattern := dsm.redisKeyPrefix + "*"
	keys, err := RedisKeys(pattern)
	if err != nil {
		logger.Error("Failed to get daily stats keys from Redis", "error", err)
		return
	}

	loadedCount := 0
	cutoffDate := time.Now().AddDate(0, 0, -dsm.retentionDays).Format("2006-01-02")

	for _, key := range keys {
		jsonData, err := RedisGet(key)
		if err != nil {
			logger.Error("Failed to get daily stats from Redis", "key", key, "error", err)
			continue
		}

		var statsData DailyStatsData
		if err := json.Unmarshal([]byte(jsonData), &statsData); err != nil {
			logger.Error("Failed to unmarshal daily stats from Redis", "key", key, "error", err)
			continue
		}

		// Skip old data
		if statsData.Date < cutoffDate {
			continue
		}

		// Generate internal key
		internalKey := dsm.generateKey(statsData.Date, statsData.NodeID, statsData.ProjectID, statsData.ProjectNodeSequence)
		dsm.data[internalKey] = &statsData
		loadedCount++
	}

	logger.Info("Loaded daily statistics from Redis", "count", loadedCount)
}

// saveToRedisSingle saves a single statistics record to Redis immediately
func (dsm *DailyStatsManager) saveToRedisSingle(key string, statsData *DailyStatsData) {
	redisKey := dsm.redisKeyPrefix + key
	expiration := int((time.Duration(dsm.retentionDays) * 24 * time.Hour).Seconds())

	jsonData, err := json.Marshal(statsData)
	if err != nil {
		logger.Error("Failed to marshal daily stats for immediate Redis save", "key", key, "error", err)
		return
	}

	if _, err := RedisSet(redisKey, string(jsonData), expiration); err != nil {
		logger.Error("Failed to immediately save daily stats to Redis", "key", redisKey, "error", err)
		return
	}
}

// saveToRedis saves current daily statistics to Redis
func (dsm *DailyStatsManager) saveToRedis() {
	if !dsm.redisEnabled {
		return
	}

	dsm.mutex.RLock()
	defer dsm.mutex.RUnlock()

	savedCount := 0
	expiration := int((time.Duration(dsm.retentionDays) * 24 * time.Hour).Seconds())

	for key, statsData := range dsm.data {
		redisKey := dsm.redisKeyPrefix + key

		jsonData, err := json.Marshal(statsData)
		if err != nil {
			logger.Error("Failed to marshal daily stats for Redis", "key", key, "error", err)
			continue
		}

		if _, err := RedisSet(redisKey, string(jsonData), expiration); err != nil {
			logger.Error("Failed to save daily stats to Redis", "key", redisKey, "error", err)
			continue
		}

		savedCount++
	}

	// Remove debug log to reduce log volume
	// logger.Debug("Saved daily statistics to Redis", "count", savedCount)
}

// persistenceLoop periodically saves data to Redis
func (dsm *DailyStatsManager) persistenceLoop() {
	defer dsm.saveWg.Done()

	ticker := time.NewTicker(dsm.saveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dsm.stopChan:
			// Final save before shutdown
			logger.Info("Performing final daily stats save to Redis before shutdown")
			dsm.saveToRedis()
			return
		case <-ticker.C:
			dsm.saveToRedis()
		}
	}
}

// Stop stops the daily statistics manager
func (dsm *DailyStatsManager) Stop() {
	close(dsm.stopChan)
	if dsm.redisEnabled {
		dsm.saveWg.Wait()
	}
}

// Global daily statistics manager instance
var GlobalDailyStatsManager *DailyStatsManager

// InitDailyStatsManager initializes the global daily statistics manager
func InitDailyStatsManager() {
	if GlobalDailyStatsManager == nil {
		GlobalDailyStatsManager = NewDailyStatsManager()
		logger.Info("Daily statistics manager initialized")
	}
}

// StopDailyStatsManager stops the global daily statistics manager
func StopDailyStatsManager() {
	if GlobalDailyStatsManager != nil {
		GlobalDailyStatsManager.Stop()
		GlobalDailyStatsManager = nil
		logger.Info("Daily statistics manager stopped")
	}
}

// GetAggregatedDailyStats returns aggregated statistics for a date
// This method now reads directly from Redis to get real-time cluster data
func (dsm *DailyStatsManager) GetAggregatedDailyStats(date string) map[string]interface{} {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	// Get all data for the date from Redis (real-time cluster data)
	allData := dsm.GetDailyStats(date, "", "")

	projectStats := make(map[string]map[string]uint64) // projectID -> componentType -> totalMessages
	totalInputMessages := uint64(0)
	totalOutputMessages := uint64(0)
	totalRulesetMessages := uint64(0)

	for _, data := range allData {
		if _, exists := projectStats[data.ProjectID]; !exists {
			projectStats[data.ProjectID] = make(map[string]uint64)
		}

		projectStats[data.ProjectID][data.ComponentType] += data.TotalMessages

		// Aggregate by component type
		parts := strings.Split(data.ProjectNodeSequence, ".")
		switch data.ComponentType {
		case "input":
			if len(parts) == 2 && strings.ToUpper(parts[0]) == "INPUT" && parts[1] == data.ComponentID {
				totalInputMessages += data.TotalMessages
			}
		case "output":
			if len(parts) >= 2 && strings.ToUpper(parts[len(parts)-2]) == "OUTPUT" && parts[len(parts)-1] == data.ComponentID {
				totalOutputMessages += data.TotalMessages
			}
		case "ruleset":
			// Only count ruleset's own processing (not downstream flow)
			// This represents the actual data volume processed by this specific ruleset
			for i := 0; i < len(parts)-1; i++ {
				if strings.ToUpper(parts[i]) == "RULESET" {
					// Only count if this is the RULESET's own ProjectNodeSequence
					// Avoid counting downstream components like "INPUT.api_sec.RULESET.test.OUTPUT.print_demo"

					// Check if there are more components after this RULESET in the sequence
					hasDownstream := (i + 2) < len(parts)

					if !hasDownstream {
						// This is the RULESET's own ProjectNodeSequence (ends with RULESET.componentId)
						totalRulesetMessages += data.TotalMessages
					}
					// If hasDownstream is true, this means it's a downstream component's sequence
					// that happens to contain this RULESET in its path - we don't count it

					break
				}
			}
		}
	}

	return map[string]interface{}{
		"date":                   date,
		"total_messages":         totalInputMessages + totalOutputMessages + totalRulesetMessages,
		"total_input_messages":   totalInputMessages,
		"total_output_messages":  totalOutputMessages,
		"total_ruleset_messages": totalRulesetMessages,
		"project_breakdown":      projectStats,
	}
}

// GetStats returns statistics about the daily stats manager
func (dsm *DailyStatsManager) GetStats() map[string]interface{} {
	dsm.mutex.RLock()
	defer dsm.mutex.RUnlock()

	totalRecords := len(dsm.data)
	dateCount := make(map[string]bool)
	nodeCount := make(map[string]bool)
	projectCount := make(map[string]bool)

	for _, data := range dsm.data {
		dateCount[data.Date] = true
		nodeCount[data.NodeID] = true
		projectCount[data.ProjectID] = true
	}

	return map[string]interface{}{
		"total_records":   totalRecords,
		"unique_dates":    len(dateCount),
		"unique_nodes":    len(nodeCount),
		"unique_projects": len(projectCount),
		"redis_enabled":   dsm.redisEnabled,
		"retention_days":  dsm.retentionDays,
		"save_interval":   dsm.saveInterval.String(),
	}
}
