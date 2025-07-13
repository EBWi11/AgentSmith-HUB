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

	// Component restart detection
	lastSeenCounters map[string]uint64 // Key: "nodeID_projectNodeSequence" -> last seen counter
	restartDetection bool              // Enable restart detection

	// Async Redis write channel
	redisWriteChan chan *DailyStatsData
	redisWriteWg   sync.WaitGroup
}

// NewDailyStatsManager creates a new daily statistics manager instance
func NewDailyStatsManager() *DailyStatsManager {
	dsm := &DailyStatsManager{
		data:             make(map[string]*DailyStatsData),
		stopChan:         make(chan struct{}),
		redisEnabled:     true,               // Default: enabled
		redisKeyPrefix:   "hub:daily_stats:", // Redis key prefix
		saveInterval:     10 * time.Second,   // Save every 10 seconds
		retentionDays:    30,                 // Keep 30 days of data
		lastSeenCounters: make(map[string]uint64),
		restartDetection: true,                             // Enable restart detection
		redisWriteChan:   make(chan *DailyStatsData, 1000), // Buffered channel for async writes
	}

	// Load existing data from Redis
	dsm.loadFromRedis()

	// Start Redis writer goroutine
	if dsm.redisEnabled {
		dsm.redisWriteWg.Add(1)
		go dsm.redisWriterLoop()
	}

	// Start persistence goroutine
	if dsm.redisEnabled {
		dsm.saveWg.Add(1)
		go dsm.persistenceLoop()
	}

	return dsm
}

// redisWriterLoop handles async Redis writes to avoid blocking the main operations
func (dsm *DailyStatsManager) redisWriterLoop() {
	defer dsm.redisWriteWg.Done()

	expiration := int((time.Duration(dsm.retentionDays) * 24 * time.Hour).Seconds())

	for {
		select {
		case <-dsm.stopChan:
			// Drain remaining writes before stopping
			for {
				select {
				case statsData := <-dsm.redisWriteChan:
					dsm.writeToRedis(statsData, expiration)
				default:
					return
				}
			}
		case statsData := <-dsm.redisWriteChan:
			dsm.writeToRedis(statsData, expiration)
		}
	}
}

// writeToRedis performs the actual Redis write operation
func (dsm *DailyStatsManager) writeToRedis(statsData *DailyStatsData, expiration int) {
	key := dsm.generateKey(statsData.Date, statsData.NodeID, statsData.ProjectID, statsData.ProjectNodeSequence)
	redisKey := dsm.redisKeyPrefix + key

	jsonData, err := json.Marshal(statsData)
	if err != nil {
		logger.Error("Failed to marshal daily stats for Redis write", "key", key, "error", err)
		return
	}

	if _, err := RedisSet(redisKey, string(jsonData), expiration); err != nil {
		logger.Error("Failed to write daily stats to Redis", "key", redisKey, "error", err)
	}
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

			// Update to new value but skip Redis write to avoid negative diff calculation
			// This allows recovery on next update cycle
			previousTotal := existing.TotalMessages
			existing.TotalMessages = totalMessages
			logger.Warn("Daily stats counter decreased - possible overflow or restart, updating value but skipping Redis write",
				"component", componentID,
				"sequence", projectNodeSequence,
				"previous_total", previousTotal,
				"current_total", totalMessages,
				"action", "skip_redis_write")

			// Skip Redis write by returning early
			existing.LastUpdate = now
			return
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

	// Async Redis write: send data to Redis writer (no lock held during Redis IO)
	// No copy needed since statsData points to stable memory and Redis writer only reads
	if dsm.redisEnabled {
		select {
		case dsm.redisWriteChan <- statsData:
			// Successfully queued for async write
		default:
			// Channel full, skip this write (will be saved in next periodic save)
			logger.Warn("Redis write channel full, skipping immediate write", "component", componentID)
		}
	}
}

// GetDailyStats returns daily statistics directly from Redis (as user suggested)
func (dsm *DailyStatsManager) GetDailyStats(date, projectID, nodeID string) map[string]*DailyStatsData {
	// Always read from Redis for real-time cluster data
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
		return result
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

	dsm.mutex.Lock()
	defer dsm.mutex.Unlock()

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

// saveToRedis saves current daily statistics to Redis (periodic bulk save)
func (dsm *DailyStatsManager) saveToRedis() {
	if !dsm.redisEnabled {
		return
	}

	// Step 1: Copy data while holding lock (very fast)
	dsm.mutex.RLock()
	dataCopy := make(map[string]*DailyStatsData, len(dsm.data))
	for k, v := range dsm.data {
		dataCopy[k] = &DailyStatsData{
			NodeID:              v.NodeID,
			ProjectID:           v.ProjectID,
			ComponentID:         v.ComponentID,
			ComponentType:       v.ComponentType,
			ProjectNodeSequence: v.ProjectNodeSequence,
			Date:                v.Date,
			TotalMessages:       v.TotalMessages,
			LastUpdate:          v.LastUpdate,
		}
	}
	dsm.mutex.RUnlock()

	// Step 2: Release lock and do Redis operations (network IO) without holding lock
	savedCount := 0
	expiration := int((time.Duration(dsm.retentionDays) * 24 * time.Hour).Seconds())

	for key, statsData := range dataCopy {
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

	logger.Debug("Saved daily statistics to Redis", "count", savedCount)
}

// persistenceLoop periodically collects data from all components and saves to Redis
func (dsm *DailyStatsManager) persistenceLoop() {
	defer dsm.saveWg.Done()

	ticker := time.NewTicker(dsm.saveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dsm.stopChan:
			// Final collection and save before shutdown
			logger.Info("Performing final daily stats collection and save to Redis before shutdown")
			dsm.collectAllComponentsData()
			dsm.saveToRedis()
			return
		case <-ticker.C:
			// Collect latest data from all components, then save to Redis
			dsm.collectAllComponentsData()
			dsm.saveToRedis()
		}
	}
}

// collectAllComponentsData collects current statistics from all running components
func (dsm *DailyStatsManager) collectAllComponentsData() {
	if statsCollector == nil {
		return
	}

	// Get current node ID
	nodeID := GetNodeID()

	// Collect component data with minimal lock time
	components := statsCollector()

	// Batch update statistics
	dsm.applyBatchUpdates(nodeID, components)
}

// applyBatchUpdates applies component statistics updates in batch to improve performance
func (dsm *DailyStatsManager) applyBatchUpdates(nodeID string, components []ComponentStatsData) {
	if len(components) == 0 {
		return
	}

	now := time.Now()
	date := now.Format("2006-01-02")

	// Step 1: Process updates while holding lock (fast, in-memory operations only)
	dsm.mutex.Lock()
	var updatesToWrite []*DailyStatsData

	for _, component := range components {
		key := dsm.generateKey(date, nodeID, component.ProjectID, component.ProjectNodeSequence)
		counterKey := fmt.Sprintf("%s_%s", nodeID, component.ProjectNodeSequence)

		var statsData *DailyStatsData
		var shouldWriteToRedis bool

		if existing, exists := dsm.data[key]; exists {
			// Check if this is a component restart
			if dsm.restartDetection {
				lastCounter, hasLastCounter := dsm.lastSeenCounters[counterKey]

				if component.TotalMessages < existing.TotalMessages {
					// Possible restart detected
					if hasLastCounter && component.TotalMessages < lastCounter {
						// Confirmed restart: counter went backwards
						increment := component.TotalMessages // Messages processed since restart
						existing.TotalMessages = existing.TotalMessages + increment

						logger.Info("Component restart detected, adding increment",
							"component", component.ComponentID,
							"sequence", component.ProjectNodeSequence,
							"previous_total", existing.TotalMessages-increment,
							"restart_increment", increment,
							"new_total", existing.TotalMessages)
						shouldWriteToRedis = true
					} else {
						// Possible overflow or restart - update value but skip Redis write
						previousTotal := existing.TotalMessages
						existing.TotalMessages = component.TotalMessages
						logger.Warn("Counter decreased without confirmed restart - possible overflow, updating value but skipping Redis write",
							"component", component.ComponentID,
							"sequence", component.ProjectNodeSequence,
							"previous_total", previousTotal,
							"current_counter", component.TotalMessages,
							"action", "skip_redis_write")
						shouldWriteToRedis = false // Skip Redis write
					}
					existing.LastUpdate = now
					statsData = existing
				} else if component.TotalMessages > existing.TotalMessages {
					// Normal increase - data changed, need to write to Redis
					existing.TotalMessages = component.TotalMessages
					existing.LastUpdate = now
					statsData = existing
					shouldWriteToRedis = true
				} else {
					// No change in count, just update timestamp
					existing.LastUpdate = now
					statsData = existing
					// No need to write to Redis for timestamp-only updates
				}

				// Update last seen counter
				dsm.lastSeenCounters[counterKey] = component.TotalMessages
			} else {
				// Restart detection disabled, use simple logic
				if component.TotalMessages >= existing.TotalMessages {
					if component.TotalMessages > existing.TotalMessages {
						shouldWriteToRedis = true
					}
					existing.TotalMessages = component.TotalMessages
				} else {
					// Counter decreased - possible overflow, update value but skip Redis write
					previousTotal := existing.TotalMessages
					existing.TotalMessages = component.TotalMessages
					logger.Warn("Counter decreased with restart detection disabled - possible overflow, updating value but skipping Redis write",
						"component", component.ComponentID,
						"sequence", component.ProjectNodeSequence,
						"previous_total", previousTotal,
						"current_counter", component.TotalMessages,
						"action", "skip_redis_write")
					shouldWriteToRedis = false // Skip Redis write
				}
				existing.LastUpdate = now
				statsData = existing
			}
		} else {
			// Create new entry
			statsData = &DailyStatsData{
				NodeID:              nodeID,
				ProjectID:           component.ProjectID,
				ComponentID:         component.ComponentID,
				ComponentType:       component.ComponentType,
				ProjectNodeSequence: component.ProjectNodeSequence,
				Date:                date,
				TotalMessages:       component.TotalMessages,
				LastUpdate:          now,
			}
			dsm.data[key] = statsData
			shouldWriteToRedis = true // New entry always needs to be written

			// Initialize counter tracking
			if dsm.restartDetection {
				dsm.lastSeenCounters[counterKey] = component.TotalMessages
			}
		}

		// Only queue for Redis write if data actually changed
		if shouldWriteToRedis {
			updatesToWrite = append(updatesToWrite, statsData)
		}
	}

	dsm.mutex.Unlock()

	// Step 2: Queue for async Redis writes (no lock held during network IO)
	// No copy needed - statsData points to stable memory that won't be modified
	// until the next update cycle, and Redis writer only reads the data
	if dsm.redisEnabled && len(updatesToWrite) > 0 {
		for _, statsData := range updatesToWrite {
			select {
			case dsm.redisWriteChan <- statsData:
				// Successfully queued
			default:
				// Channel full, skip (will be saved in next periodic save)
				logger.Warn("Redis write channel full, skipping write for component",
					"component", statsData.ComponentID)
			}
		}
	}
}

// ComponentStatsData represents statistics for a single component
type ComponentStatsData struct {
	ProjectID           string
	ComponentID         string
	ComponentType       string
	ProjectNodeSequence string
	TotalMessages       uint64
}

// StatsCollectorFunc is a function type for collecting component statistics
type StatsCollectorFunc func() []ComponentStatsData

// statsCollector is a global callback function set by the project package
var statsCollector StatsCollectorFunc

// SetStatsCollector sets the callback function for collecting component statistics
func SetStatsCollector(collector StatsCollectorFunc) {
	statsCollector = collector
}

// Stop stops the daily statistics manager
func (dsm *DailyStatsManager) Stop() {
	close(dsm.stopChan)

	// Wait for Redis writer to finish
	if dsm.redisEnabled {
		dsm.redisWriteWg.Wait()
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

// GetAggregatedDailyStats returns aggregated statistics for a date (read from Redis)
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
		if len(parts) >= 2 {
			componentTypeFromSequence := strings.ToLower(parts[len(parts)-2])
			switch componentTypeFromSequence {
			case "input":
				totalInputMessages += data.TotalMessages
			case "output":
				totalOutputMessages += data.TotalMessages
			case "ruleset":
				totalRulesetMessages += data.TotalMessages
			}
		}
	}

	return map[string]interface{}{
		"date":                   date,
		"total_input_messages":   totalInputMessages,
		"total_output_messages":  totalOutputMessages,
		"total_ruleset_messages": totalRulesetMessages,
		"projects":               projectStats,
		"timestamp":              time.Now(),
	}
}

// GetStats returns general statistics about the daily stats manager
func (dsm *DailyStatsManager) GetStats() map[string]interface{} {
	dsm.mutex.RLock()
	defer dsm.mutex.RUnlock()

	totalEntries := len(dsm.data)
	totalCounters := len(dsm.lastSeenCounters)

	// Group by date
	dateGroups := make(map[string]int)
	for _, statsData := range dsm.data {
		dateGroups[statsData.Date]++
	}

	return map[string]interface{}{
		"total_entries":       totalEntries,
		"total_counters":      totalCounters,
		"retention_days":      dsm.retentionDays,
		"save_interval":       dsm.saveInterval.String(),
		"restart_detection":   dsm.restartDetection,
		"redis_enabled":       dsm.redisEnabled,
		"entries_by_date":     dateGroups,
		"redis_key_prefix":    dsm.redisKeyPrefix,
		"redis_write_pending": len(dsm.redisWriteChan),
	}
}
