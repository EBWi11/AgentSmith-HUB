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

// generateKey creates a unique key for daily statistics
func (dsm *DailyStatsManager) generateKey(date, nodeID, projectNodeSequence string) string {
	return fmt.Sprintf("%s_%s_%s", date, nodeID, projectNodeSequence)
}

// UpdateDailyStats updates daily statistics for a component
func (dsm *DailyStatsManager) UpdateDailyStats(nodeID, projectID, componentID, componentType, projectNodeSequence string, totalMessages uint64) {
	now := time.Now()
	date := now.Format("2006-01-02")
	key := dsm.generateKey(date, nodeID, projectNodeSequence)

	dsm.mutex.Lock()
	defer dsm.mutex.Unlock()

	if existing, exists := dsm.data[key]; exists {
		// Update existing data
		existing.TotalMessages = totalMessages
		existing.LastUpdate = now
	} else {
		// Create new data
		dsm.data[key] = &DailyStatsData{
			NodeID:              nodeID,
			ProjectID:           projectID,
			ComponentID:         componentID,
			ComponentType:       componentType,
			ProjectNodeSequence: projectNodeSequence,
			Date:                date,
			TotalMessages:       totalMessages,
			LastUpdate:          now,
		}
	}
}

// GetDailyStats returns daily statistics for a specific date and optional filters
func (dsm *DailyStatsManager) GetDailyStats(date, projectID, nodeID string) map[string]*DailyStatsData {
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
		internalKey := dsm.generateKey(statsData.Date, statsData.NodeID, statsData.ProjectNodeSequence)
		dsm.data[internalKey] = &statsData
		loadedCount++
	}

	logger.Info("Loaded daily statistics from Redis", "count", loadedCount)
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

	if savedCount > 0 {
		logger.Debug("Saved daily statistics to Redis", "count", savedCount)
	}
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
func (dsm *DailyStatsManager) GetAggregatedDailyStats(date string) map[string]interface{} {
	dsm.mutex.RLock()
	defer dsm.mutex.RUnlock()

	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	projectStats := make(map[string]map[string]uint64) // projectID -> componentType -> totalMessages
	totalInputMessages := uint64(0)
	totalOutputMessages := uint64(0)
	totalRulesetMessages := uint64(0)

	for _, data := range dsm.data {
		if data.Date != date {
			continue
		}

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
			totalRulesetMessages += data.TotalMessages
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
