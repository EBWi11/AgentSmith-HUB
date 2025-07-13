package common

import (
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"strconv"
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

	// Note: diffCounters removed - components now manage their own increments

	// Async Redis write channel
	redisWriteChan chan *DailyStatsData
	redisWriteWg   sync.WaitGroup
}

// NewDailyStatsManager creates a new daily statistics manager instance
func NewDailyStatsManager() *DailyStatsManager {
	dsm := &DailyStatsManager{
		data:           make(map[string]*DailyStatsData),
		stopChan:       make(chan struct{}),
		redisEnabled:   true,               // Default: enabled
		redisKeyPrefix: "hub:daily_stats:", // Redis key prefix
		saveInterval:   10 * time.Second,   // Save every 10 seconds
		retentionDays:  30,                 // Keep 30 days of data

		redisWriteChan: make(chan *DailyStatsData, 1000), // Buffered channel for async writes
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
	// 使用优化的 Hash 存储方案
	// 这里的增量是 statsData.TotalMessages，因为这是从内存中的累积值
	if err := dsm.writeToRedisOptimized(statsData, statsData.TotalMessages, expiration); err != nil {
		logger.Error("Failed to write daily stats to Redis", "error", err)
	}
}

// writeToRedisWithIncrement performs atomic increment in Redis and updates metadata
func (dsm *DailyStatsManager) writeToRedisWithIncrement(statsData *DailyStatsData, increment uint64, expiration int) error {
	// 使用优化的 Hash 存储方案
	return dsm.writeToRedisOptimized(statsData, increment, expiration)
}

// writeToRedisOptimized 使用 Redis Hash 优化存储，减少 50% 空间
func (dsm *DailyStatsManager) writeToRedisOptimized(statsData *DailyStatsData, increment uint64, expiration int) error {
	// Hash Key: hub:daily_stats:hash:date
	hashKey := fmt.Sprintf("%shash:%s", dsm.redisKeyPrefix, statsData.Date)

	// Field 编码所有必要的元数据
	// 格式：nodeID|projectID|componentType|componentID|sequence
	field := fmt.Sprintf("%s|%s|%s|%s|%s",
		statsData.NodeID,
		statsData.ProjectID,
		statsData.ComponentType,
		statsData.ComponentID,
		statsData.ProjectNodeSequence)

	// 原子递增
	_, err := RedisHIncrBy(hashKey, field, int64(increment))
	if err != nil {
		logger.Error("Failed to increment hash counter in Redis",
			"key", hashKey,
			"field", field,
			"increment", increment,
			"error", err)
		return err
	}

	// 设置过期时间（只需要在第一次设置）
	if err := RedisExpire(hashKey, expiration); err != nil {
		logger.Warn("Failed to set expiration for hash key", "key", hashKey, "error", err)
	}

	logger.Debug("Successfully incremented Redis hash counter",
		"hash_key", hashKey,
		"field", field,
		"increment", increment)

	return nil
}

// writeToRedisLegacy 保留旧的实现用于兼容
func (dsm *DailyStatsManager) writeToRedisLegacy(statsData *DailyStatsData, increment uint64, expiration int) error {
	key := dsm.generateKey(statsData.Date, statsData.NodeID, statsData.ProjectID, statsData.ProjectNodeSequence)

	// Use separate Redis keys for counter and metadata
	counterKey := dsm.redisKeyPrefix + key + ":counter"
	metadataKey := dsm.redisKeyPrefix + key + ":metadata"

	// Atomically increment the counter in Redis
	newTotal, err := RedisIncrby(counterKey, int64(increment))
	if err != nil {
		logger.Error("Failed to increment counter in Redis", "key", counterKey, "increment", increment, "error", err)
		return err
	}

	// Set expiration for counter key
	if err := RedisExpire(counterKey, expiration); err != nil {
		logger.Warn("Failed to set expiration for counter key", "key", counterKey, "error", err)
	}

	// Update metadata with the new total
	metadataStatsData := &DailyStatsData{
		NodeID:              statsData.NodeID,
		ProjectID:           statsData.ProjectID,
		ComponentID:         statsData.ComponentID,
		ComponentType:       statsData.ComponentType,
		ProjectNodeSequence: statsData.ProjectNodeSequence,
		Date:                statsData.Date,
		TotalMessages:       uint64(newTotal), // Use the Redis-returned total
		LastUpdate:          statsData.LastUpdate,
	}

	// Store metadata as JSON
	jsonData, err := json.Marshal(metadataStatsData)
	if err != nil {
		logger.Error("Failed to marshal metadata for Redis write", "key", key, "error", err)
		return err
	}

	if _, err := RedisSet(metadataKey, string(jsonData), expiration); err != nil {
		logger.Error("Failed to write metadata to Redis", "key", metadataKey, "error", err)
		return err
	}

	logger.Debug("Successfully incremented Redis counter and updated metadata",
		"counter_key", counterKey,
		"increment", increment,
		"new_total", newTotal,
		"metadata_key", metadataKey)

	return nil
}

// generateKey creates a unique key for daily statistics. We must include projectID so that
// multiple projects共享同一个 ProjectNodeSequence 时不会互相覆盖。
func (dsm *DailyStatsManager) generateKey(date, nodeID, projectID, projectNodeSequence string) string {
	return fmt.Sprintf("%s_%s_%s_%s", date, nodeID, projectID, projectNodeSequence)
}

// Note: UpdateDailyStats has been removed as it was not being used.
// All statistics collection is now handled through applyBatchUpdates called from persistenceLoop.

// GetDailyStats returns daily statistics directly from Redis (as user suggested)
func (dsm *DailyStatsManager) GetDailyStats(date, projectID, nodeID string) map[string]*DailyStatsData {
	// Always read from Redis for real-time cluster data
	result := make(map[string]*DailyStatsData)

	// Use default date if not specified
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	// 首先尝试读取新的 Hash 格式
	hashData := dsm.getDailyStatsFromHash(date, projectID, nodeID)
	for k, v := range hashData {
		result[k] = v
	}

	// 然后读取旧格式数据（向后兼容）
	legacyData := dsm.getDailyStatsLegacy(date, projectID, nodeID)
	for k, v := range legacyData {
		// 如果新格式中已经有了，不覆盖（新格式优先）
		if _, exists := result[k]; !exists {
			result[k] = v
		}
	}

	return result
}

// getDailyStatsFromHash 从新的 Hash 格式读取数据
func (dsm *DailyStatsManager) getDailyStatsFromHash(date, projectID, nodeID string) map[string]*DailyStatsData {
	result := make(map[string]*DailyStatsData)

	// Hash Key
	hashKey := fmt.Sprintf("%shash:%s", dsm.redisKeyPrefix, date)

	// 获取整个 Hash
	hashData, err := RedisHGetAll(hashKey)
	if err != nil {
		logger.Error("Failed to get hash data from Redis", "key", hashKey, "error", err)
		return result
	}

	for field, countStr := range hashData {
		// 解析 field：nodeID|projectID|componentType|componentID|sequence
		parts := strings.Split(field, "|")
		if len(parts) != 5 {
			logger.Warn("Invalid field format in hash", "field", field)
			continue
		}

		fieldNodeID := parts[0]
		fieldProjectID := parts[1]
		componentType := parts[2]
		componentID := parts[3]
		projectNodeSequence := parts[4]

		// 应用过滤条件
		if nodeID != "" && fieldNodeID != nodeID {
			continue
		}
		if projectID != "" && fieldProjectID != projectID {
			continue
		}

		// 解析计数
		count, err := strconv.ParseUint(countStr, 10, 64)
		if err != nil {
			logger.Warn("Failed to parse count", "field", field, "count", countStr, "error", err)
			continue
		}

		// 生成内部键
		internalKey := dsm.generateKey(date, fieldNodeID, fieldProjectID, projectNodeSequence)

		// 创建 DailyStatsData
		result[internalKey] = &DailyStatsData{
			NodeID:              fieldNodeID,
			ProjectID:           fieldProjectID,
			ComponentID:         componentID,
			ComponentType:       componentType,
			ProjectNodeSequence: projectNodeSequence,
			Date:                date,
			TotalMessages:       count,
			LastUpdate:          time.Now(), // 使用当前时间
		}
	}

	return result
}

// getDailyStatsLegacy 从旧格式读取数据（向后兼容）
func (dsm *DailyStatsManager) getDailyStatsLegacy(date, projectID, nodeID string) map[string]*DailyStatsData {
	result := make(map[string]*DailyStatsData)

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
		// Check if this is a metadata key (new format) or legacy key (old format)
		if strings.HasSuffix(key, ":metadata") {
			// New format: read from metadata key
			jsonData, err := RedisGet(key)
			if err != nil {
				logger.Error("Failed to get daily stats metadata from Redis", "key", key, "error", err)
				continue
			}

			var statsData DailyStatsData
			if err := json.Unmarshal([]byte(jsonData), &statsData); err != nil {
				logger.Error("Failed to unmarshal daily stats metadata from Redis", "key", key, "error", err)
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
		} else if !strings.HasSuffix(key, ":counter") && !strings.Contains(key, ":hash:") {
			// Legacy format: read from old JSON key
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
		// Skip counter keys and hash keys as they are handled separately
	}

	return result
}

func (dsm *DailyStatsManager) loadFromRedis() {
	if !dsm.redisEnabled {
		return
	}

	logger.Info("Loading daily statistics from Redis")

	loadedCount := 0
	cutoffDate := time.Now().AddDate(0, 0, -dsm.retentionDays).Format("2006-01-02")

	dsm.mutex.Lock()
	defer dsm.mutex.Unlock()

	// 首先加载新的 Hash 格式数据
	hashPattern := fmt.Sprintf("%shash:*", dsm.redisKeyPrefix)
	hashKeys, err := RedisKeys(hashPattern)
	if err != nil {
		logger.Error("Failed to get hash keys from Redis", "error", err)
	} else {
		for _, hashKey := range hashKeys {
			// 提取日期
			parts := strings.Split(hashKey, ":")
			if len(parts) < 3 {
				continue
			}
			date := parts[len(parts)-1]

			// 跳过过期数据
			if date < cutoffDate {
				continue
			}

			// 获取 Hash 中的所有数据
			hashData, err := RedisHGetAll(hashKey)
			if err != nil {
				logger.Error("Failed to get hash data from Redis", "key", hashKey, "error", err)
				continue
			}

			for field, countStr := range hashData {
				// 解析 field
				fieldParts := strings.Split(field, "|")
				if len(fieldParts) != 5 {
					continue
				}

				count, err := strconv.ParseUint(countStr, 10, 64)
				if err != nil {
					continue
				}

				statsData := &DailyStatsData{
					NodeID:              fieldParts[0],
					ProjectID:           fieldParts[1],
					ComponentType:       fieldParts[2],
					ComponentID:         fieldParts[3],
					ProjectNodeSequence: fieldParts[4],
					Date:                date,
					TotalMessages:       count,
					LastUpdate:          time.Now(),
				}

				internalKey := dsm.generateKey(date, statsData.NodeID, statsData.ProjectID, statsData.ProjectNodeSequence)
				dsm.data[internalKey] = statsData
				loadedCount++
			}
		}
	}

	// 然后加载旧格式数据（向后兼容）
	pattern := dsm.redisKeyPrefix + "*"
	keys, err := RedisKeys(pattern)
	if err != nil {
		logger.Error("Failed to get daily stats keys from Redis", "error", err)
		return
	}

	for _, key := range keys {
		// 跳过新格式的键
		if strings.Contains(key, ":hash:") || strings.HasSuffix(key, ":counter") || strings.HasSuffix(key, ":metadata") {
			continue
		}

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
		// 如果新格式中已经有了，不覆盖
		if _, exists := dsm.data[internalKey]; !exists {
			dsm.data[internalKey] = &statsData
			loadedCount++
		}
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

	// Step 2: Group by date for batch update
	dataByDate := make(map[string]map[string]*DailyStatsData)
	for _, statsData := range dataCopy {
		if _, exists := dataByDate[statsData.Date]; !exists {
			dataByDate[statsData.Date] = make(map[string]*DailyStatsData)
		}
		dataByDate[statsData.Date][dsm.generateKey(statsData.Date, statsData.NodeID, statsData.ProjectID, statsData.ProjectNodeSequence)] = statsData
	}

	// Step 3: Update Redis using Hash format
	savedCount := 0
	expiration := int((time.Duration(dsm.retentionDays) * 24 * time.Hour).Seconds())

	for date, dayData := range dataByDate {
		hashKey := fmt.Sprintf("%shash:%s", dsm.redisKeyPrefix, date)

		// Update all fields for this date
		for _, statsData := range dayData {
			field := fmt.Sprintf("%s|%s|%s|%s|%s",
				statsData.NodeID,
				statsData.ProjectID,
				statsData.ComponentType,
				statsData.ComponentID,
				statsData.ProjectNodeSequence)

			// 使用 HSet 而不是 HIncrBy，因为这是完整保存
			if err := RedisHSet(hashKey, field, statsData.TotalMessages); err != nil {
				logger.Error("Failed to save daily stats to Redis hash",
					"hash_key", hashKey,
					"field", field,
					"error", err)
				continue
			}
			savedCount++
		}

		// Set expiration for the hash
		if err := RedisExpire(hashKey, expiration); err != nil {
			logger.Warn("Failed to set expiration for hash key", "key", hashKey, "error", err)
		}
	}

	logger.Debug("Saved daily statistics to Redis", "count", savedCount)
}

// persistenceLoop periodically collects data from all components and saves to Redis
func (dsm *DailyStatsManager) persistenceLoop() {
	defer dsm.saveWg.Done()

	ticker := time.NewTicker(dsm.saveInterval)
	defer ticker.Stop()

	// Cleanup ticker for removing old difference counters (once per hour)
	cleanupTicker := time.NewTicker(1 * time.Hour)
	defer cleanupTicker.Stop()

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
		case <-cleanupTicker.C:
			// Note: cleanupOldDiffCounters removed - no longer needed
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

// CollectAllComponentsData is a public wrapper for collectAllComponentsData
func (dsm *DailyStatsManager) CollectAllComponentsData() {
	dsm.collectAllComponentsData()
}

// CollectFinalStatsBeforeComponentStop collects final statistics for a specific component before it stops
func (dsm *DailyStatsManager) CollectFinalStatsBeforeComponentStop(componentType, componentID string) {
	if statsCollector == nil {
		return
	}

	logger.Info("Collecting final statistics before component stop", "type", componentType, "id", componentID)

	// Get current component statistics
	components := statsCollector()

	// Filter for the specific component with meaningful increments
	componentStats := make([]ComponentStatsData, 0)
	for _, component := range components {
		if component.ComponentID == componentID &&
			strings.Contains(strings.ToLower(component.ComponentType), strings.ToLower(componentType)) &&
			component.TotalMessages > 0 {
			componentStats = append(componentStats, component)
		}
	}

	if len(componentStats) > 0 {
		logger.Info("Saving final component statistics before stop",
			"type", componentType,
			"id", componentID,
			"stats_count", len(componentStats))

		// Apply batch updates for these specific component stats
		nodeID := GetNodeID()
		dsm.ApplyBatchUpdatesWithForceWrite(nodeID, componentStats)
	}
}

// applyBatchUpdates applies component statistics updates in batch using increments from components
func (dsm *DailyStatsManager) applyBatchUpdates(nodeID string, components []ComponentStatsData) {
	if len(components) == 0 {
		return
	}

	now := time.Now()
	date := now.Format("2006-01-02")

	// Filter components with meaningful increments
	var validComponents []ComponentStatsData
	for _, component := range components {
		// TotalMessages field now contains the increment (from component's GetIncrementAndUpdate)
		if component.TotalMessages > 0 {
			validComponents = append(validComponents, component)
		}
	}

	if len(validComponents) == 0 {
		return
	}

	// Update Redis daily counters (Layer 3) with increments from components
	dsm.mutex.Lock()
	var updatesToWrite []*DailyStatsData

	for _, component := range validComponents {
		key := dsm.generateKey(date, nodeID, component.ProjectID, component.ProjectNodeSequence)
		increment := component.TotalMessages // This is actually the increment now

		var statsData *DailyStatsData
		if existing, exists := dsm.data[key]; exists {
			// Add increment to existing daily total
			existing.TotalMessages += increment
			existing.LastUpdate = now
			statsData = existing

			logger.Debug("Updated daily total with increment",
				"component", component.ComponentID,
				"sequence", component.ProjectNodeSequence,
				"increment", increment,
				"new_total", existing.TotalMessages)
		} else {
			// Create new daily stats entry
			statsData = &DailyStatsData{
				NodeID:              nodeID,
				ProjectID:           component.ProjectID,
				ComponentID:         component.ComponentID,
				ComponentType:       component.ComponentType,
				ProjectNodeSequence: component.ProjectNodeSequence,
				Date:                date,
				TotalMessages:       increment,
				LastUpdate:          now,
			}
			dsm.data[key] = statsData

			logger.Debug("Created new daily stats entry",
				"component", component.ComponentID,
				"sequence", component.ProjectNodeSequence,
				"initial_total", increment)
		}

		updatesToWrite = append(updatesToWrite, statsData)
	}

	dsm.mutex.Unlock()

	// Queue for async Redis writes (Layer 3 persistence)
	if dsm.redisEnabled && len(updatesToWrite) > 0 {
		for _, statsData := range updatesToWrite {
			select {
			case dsm.redisWriteChan <- statsData:
				// Successfully queued
			default:
				// Channel full, skip (will be saved in next periodic save)
				logger.Warn("Redis write channel full, skipping write", "component", statsData.ComponentID)
			}
		}
	}
}

// ApplyBatchUpdatesWithForceWrite applies component statistics updates with forced synchronous Redis writes
// This is used for final statistics collection before component stops to ensure data is not lost
func (dsm *DailyStatsManager) ApplyBatchUpdatesWithForceWrite(nodeID string, components []ComponentStatsData) {
	if len(components) == 0 {
		return
	}

	now := time.Now()
	date := now.Format("2006-01-02")
	expiration := int((time.Duration(dsm.retentionDays) * 24 * time.Hour).Seconds())

	// Filter components with meaningful increments
	var validComponents []ComponentStatsData
	for _, component := range components {
		// TotalMessages field now contains the increment (from component's GetIncrementAndUpdate)
		if component.TotalMessages > 0 {
			validComponents = append(validComponents, component)
		}
	}

	if len(validComponents) == 0 {
		return
	}

	// Update Redis daily counters (Layer 3) with increments from components
	dsm.mutex.Lock()
	var updatesToWrite []*DailyStatsData

	for _, component := range validComponents {
		key := dsm.generateKey(date, nodeID, component.ProjectID, component.ProjectNodeSequence)
		increment := component.TotalMessages // This is actually the increment now

		var statsData *DailyStatsData
		if existing, exists := dsm.data[key]; exists {
			// Add increment to existing daily total
			existing.TotalMessages += increment
			existing.LastUpdate = now
			statsData = existing

			logger.Debug("Updated daily total with increment (force write)",
				"component", component.ComponentID,
				"sequence", component.ProjectNodeSequence,
				"increment", increment,
				"new_total", existing.TotalMessages)
		} else {
			// Create new daily stats entry
			statsData = &DailyStatsData{
				NodeID:              nodeID,
				ProjectID:           component.ProjectID,
				ComponentID:         component.ComponentID,
				ComponentType:       component.ComponentType,
				ProjectNodeSequence: component.ProjectNodeSequence,
				Date:                date,
				TotalMessages:       increment,
				LastUpdate:          now,
			}
			dsm.data[key] = statsData

			logger.Debug("Created new daily stats entry (force write)",
				"component", component.ComponentID,
				"sequence", component.ProjectNodeSequence,
				"initial_total", increment)
		}

		updatesToWrite = append(updatesToWrite, statsData)
	}

	dsm.mutex.Unlock()

	// Force synchronous Redis writes for final statistics using atomic increments
	if dsm.redisEnabled && len(updatesToWrite) > 0 {
		logger.Info("Force writing final statistics to Redis using atomic increments",
			"count", len(updatesToWrite))

		for i, statsData := range updatesToWrite {
			// Use the corresponding increment value from validComponents
			increment := validComponents[i].TotalMessages
			if err := dsm.writeToRedisWithIncrement(statsData, increment, expiration); err != nil {
				logger.Error("Failed to write final statistics with increment",
					"component", statsData.ComponentID,
					"increment", increment,
					"error", err)
			}
		}

		logger.Info("Final statistics atomic increment write completed")
	}
}

// ComponentStatsData represents statistics for a single component
type ComponentStatsData struct {
	ProjectID           string
	ComponentID         string
	ComponentType       string
	ProjectNodeSequence string
	TotalMessages       uint64 // This is now the increment, not total (should be renamed in future)
}

// StatsCollectorFunc is a function type for collecting component statistics
type StatsCollectorFunc func() []ComponentStatsData

// statsCollector is a global callback function set by the project package
var statsCollector StatsCollectorFunc

// SetStatsCollector sets the callback function for collecting component statistics
func SetStatsCollector(collector StatsCollectorFunc) {
	statsCollector = collector
}

// GetStatsCollector returns the current stats collector function
func GetStatsCollector() StatsCollectorFunc {
	return statsCollector
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

	// Group by date
	dateGroups := make(map[string]int)
	for _, statsData := range dsm.data {
		dateGroups[statsData.Date]++
	}

	return map[string]interface{}{
		"total_entries":             totalEntries,
		"retention_days":            dsm.retentionDays,
		"save_interval":             dsm.saveInterval.String(),
		"component_managed_counter": true, // Indicates components manage their own increments
		"redis_enabled":             dsm.redisEnabled,
		"entries_by_date":           dateGroups,
		"redis_key_prefix":          dsm.redisKeyPrefix,
		"redis_write_pending":       len(dsm.redisWriteChan),
	}
}

// Note: ResetDiffCounter, ResetDiffCounters, and cleanupOldDiffCounters methods removed
// Components now manage their own increments via GetIncrementAndUpdate()
