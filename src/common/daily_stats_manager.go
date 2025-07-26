package common

import (
	"AgentSmith-HUB/logger"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// ComponentInfo represents a component extracted from ProjectNodeSequence
type ComponentInfo struct {
	Type string // input, output, ruleset, plugin_success, plugin_failure
	ID   string // component identifier
}

// ParseProjectNodeSequence extracts all components from a ProjectNodeSequence
// Examples:
//   - "INPUT.kafka1" -> [{Type: "input", ID: "kafka1"}]
//   - "INPUT.kafka1.RULESET.test.OUTPUT.print" -> [{Type: "input", ID: "kafka1"}, {Type: "ruleset", ID: "test"}, {Type: "output", ID: "print"}]
//   - "PLUGIN.hash_md5.success" -> [{Type: "plugin_success", ID: "hash_md5"}]
func ParseProjectNodeSequence(sequence string) []ComponentInfo {
	if sequence == "" {
		return nil
	}

	var components []ComponentInfo
	parts := strings.Split(sequence, ".")

	for i := 0; i < len(parts)-1; i += 2 {
		componentType := strings.ToLower(parts[i])

		// Handle special cases
		switch componentType {
		case "input", "output", "ruleset":
			if i+1 < len(parts) {
				components = append(components, ComponentInfo{
					Type: componentType,
					ID:   parts[i+1],
				})
			}
		case "plugin":
			// Plugin sequences are like "PLUGIN.plugin_name.success" or "PLUGIN.plugin_name.failure"
			if i+2 < len(parts) {
				pluginID := parts[i+1]
				status := strings.ToLower(parts[i+2])

				if status == "success" {
					components = append(components, ComponentInfo{
						Type: "plugin_success",
						ID:   pluginID,
					})
				} else if status == "failure" {
					components = append(components, ComponentInfo{
						Type: "plugin_failure",
						ID:   pluginID,
					})
				}
				i++ // Skip the status part
			}
		}
	}

	return components
}

// GetComponentTypeFromSequence extracts the component type from the LAST part of ProjectNodeSequence
// Examples:
//   - "INPUT.kafka1" -> "input" (last component type is INPUT)
//   - "INPUT.kafka1.RULESET.test.OUTPUT.print" -> "output" (last component type is OUTPUT)
//   - "PLUGIN.hash_md5.success" -> "plugin_success" (ends with success after PLUGIN)
func GetComponentTypeFromSequence(sequence, fallbackType string) string {
	if sequence == "" {
		return fallbackType
	}

	// Split by dots and scan backwards to find the last component type
	parts := strings.Split(sequence, ".")

	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.ToUpper(parts[i])

		// Check for component type keywords
		switch part {
		case "INPUT":
			return "input"
		case "OUTPUT":
			return "output"
		case "RULESET":
			return "ruleset"
		case "SUCCESS":
			// Plugin success - need to verify there's a PLUGIN earlier
			for j := i - 1; j >= 0; j-- {
				if strings.ToUpper(parts[j]) == "PLUGIN" {
					return "plugin_success"
				}
			}
		case "FAILURE":
			// Plugin failure - need to verify there's a PLUGIN earlier
			for j := i - 1; j >= 0; j-- {
				if strings.ToUpper(parts[j]) == "PLUGIN" {
					return "plugin_failure"
				}
			}
		}
	}

	return fallbackType
}

// DailyStatsData represents daily statistics for a component
type DailyStatsData struct {
	NodeID              string `json:"node_id"`
	ProjectID           string `json:"project_id"`
	ComponentID         string `json:"component_id"`
	ComponentType       string `json:"component_type"`
	ProjectNodeSequence string `json:"project_node_sequence"`
	Date                string `json:"date"`           // Format: 2006-01-02
	TotalMessages       uint64 `json:"total_messages"` // Total messages for this date
}

// DailyStatsManager manages daily message statistics with Redis persistence
type DailyStatsManager struct {
	stopChan       chan struct{}
	redisKeyPrefix string
	saveInterval   time.Duration
	retentionDays  int
}

// NewDailyStatsManager creates a new daily statistics manager instance
func NewDailyStatsManager() *DailyStatsManager {
	dsm := &DailyStatsManager{
		stopChan:       make(chan struct{}),
		redisKeyPrefix: "hub:daily_stats:", // Redis key prefix
		saveInterval:   15 * time.Second,   // Collect every 15 seconds
		retentionDays:  10,                 // Keep 30 days of data
	}

	go dsm.persistenceLoop()
	return dsm
}

func (dsm *DailyStatsManager) writeToRedisLegacy(statsData *DailyStatsData, increment uint64, expiration int) error {
	key := dsm.generateKey(statsData.Date, statsData.NodeID, statsData.ProjectID, statsData.ProjectNodeSequence)
	counterKey := dsm.redisKeyPrefix + key

	// Use Redis transaction to ensure atomicity of INCRBY and EXPIRE operations
	ctx := context.Background()
	pipe := GetRedisClient().Pipeline()
	pipe.IncrBy(ctx, counterKey, int64(increment))
	pipe.Expire(ctx, counterKey, time.Duration(expiration)*time.Second)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute Redis transaction for stats update: %w", err)
	}

	return nil
}

func (dsm *DailyStatsManager) generateKey(date, nodeID, projectID, projectNodeSequence string) string {
	return fmt.Sprintf("%s#%s#%s#%s", date, nodeID, projectID, projectNodeSequence)
}

func (dsm *DailyStatsManager) parserKey(key string) (res DailyStatsData, err error) {
	ss := strings.Split(strings.Replace(key, dsm.redisKeyPrefix, "", 1), "#")
	if len(ss) != 4 {
		return res, fmt.Errorf("invalid key format: %s", key)
	} else {
		msgT, err := RedisGet(key)
		if err != nil {
			return res, err
		}

		res.Date = ss[0]
		res.NodeID = ss[1]
		res.ProjectID = ss[2]
		res.ProjectNodeSequence = ss[3]
		t, id := GetComponentFromSequenceID(res.ProjectNodeSequence)
		res.ComponentType = t
		res.ComponentID = id
		res.TotalMessages, err = strconv.ParseUint(msgT, 10, 64)
		if err != nil {
			return res, err
		}
		return res, nil
	}

}

func (dsm *DailyStatsManager) GetDailyStats(date, projectID, nodeID string) map[string]*DailyStatsData {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	return dsm.getDailyStatsLegacy(date, projectID, nodeID)
}

func (dsm *DailyStatsManager) getDailyStatsLegacy(date, projectID, nodeID string) map[string]*DailyStatsData {
	result := make(map[string]*DailyStatsData)
	var pattern string
	var err error

	if nodeID != "" && projectID != "" {
		pattern = fmt.Sprintf("%s%s#%s#%s#*", dsm.redisKeyPrefix, date, nodeID, projectID)
	} else if nodeID != "" {
		pattern = fmt.Sprintf("%s%s#%s#*", dsm.redisKeyPrefix, date, nodeID)
	} else if projectID != "" {
		pattern = fmt.Sprintf("%s%s#*#%s#*", dsm.redisKeyPrefix, date, projectID)
	} else {
		pattern = fmt.Sprintf("%s%s#*", dsm.redisKeyPrefix, date)
	}

	keys, err := RedisKeys(pattern)
	if err != nil {
		logger.Error("Failed to get daily stats keys from Redis", "pattern", pattern, "error", err)
		return result
	}

	for _, key := range keys {
		dailyData, err := dsm.parserKey(key)
		if err != nil {
			logger.Error("Failed to get daily stats key from Redis", "pattern", pattern, "error", err)
			continue
		}

		// Preserve all node data by using unique keys including NodeID
		// This prevents data overwriting while maintaining node-level information for byNode queries
		// Format: ProjectNodeSequence#NodeID
		uniqueKey := dailyData.ProjectNodeSequence + "#" + dailyData.NodeID
		result[uniqueKey] = &dailyData
	}

	return result
}

// persistenceLoop periodically collects data from all components and saves to Redis
// Improved to handle high concurrency scenarios with better error handling
func (dsm *DailyStatsManager) persistenceLoop() {
	ticker := time.NewTicker(dsm.saveInterval)
	defer ticker.Stop()

	// Track if collection is in progress to prevent overlapping
	var collecting int32

	for {
		select {
		case <-dsm.stopChan:
			return
		case <-ticker.C:
			// Use atomic operation to prevent overlapping collections
			if atomic.CompareAndSwapInt32(&collecting, 0, 1) {
				// Start collection in a goroutine to prevent blocking the ticker
				go func() {
					defer atomic.StoreInt32(&collecting, 0)

					// Add timeout to prevent long-running collections
					done := make(chan struct{})
					go func() {
						dsm.CollectAllComponentsData()
						close(done)
					}()

					select {
					case <-done:
						// Collection completed successfully
					case <-time.After(4 * time.Second): // Timeout at 4 seconds to prevent overlap
						logger.Warn("Statistics collection timed out, may have missed some data")
					}
				}()
			} else {
				// Instead of skipping, wait a bit and try again to avoid data loss
				logger.Debug("Previous statistics collection still in progress, waiting...")
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

func (dsm *DailyStatsManager) CollectAllComponentsData() {
	if statsCollector != nil {
		// 检查是否有运行中的项目，如果没有则跳过收集
		stats := GetStatsCollector()()
		if len(stats) == 0 {
			logger.Debug("No running components found, skipping stats collection")
			return
		}
		dsm.ApplyBatchUpdates(stats)
	}
}

func (dsm *DailyStatsManager) ApplyBatchUpdates(dailyStatsData []DailyStatsData) {
	now := time.Now()
	date := now.Format("2006-01-02")

	expiration := int((time.Duration(dsm.retentionDays) * 24 * time.Hour).Seconds())

	for i := range dailyStatsData {
		data := dailyStatsData[i]
		data.Date = date
		data.NodeID = GetNodeID()

		// Skip writing to Redis if TotalMessages is 0
		if data.TotalMessages == 0 {
			continue
		}

		// Add retry mechanism for Redis writes to prevent data loss
		maxRetries := 3
		for retry := 0; retry < maxRetries; retry++ {
			if err := dsm.writeToRedisLegacy(&data, data.TotalMessages, expiration); err != nil {
				if retry == maxRetries-1 {
					// Final retry failed, log error
					logger.Error("Failed to write statistics increment after retries",
						"component", data.ComponentID,
						"sequence", data.ProjectNodeSequence,
						"increment", data.TotalMessages,
						"retries", retry+1,
						"error", err)
				} else {
					// Retry after a short delay
					logger.Warn("Redis write failed, retrying",
						"component", data.ComponentID,
						"sequence", data.ProjectNodeSequence,
						"retry", retry+1,
						"error", err)
					time.Sleep(100 * time.Millisecond)
				}
			} else {
				// Success, break retry loop
				break
			}
		}
	}
}

// StatsCollectorFunc is a function type for collecting component statistics
type StatsCollectorFunc func() []DailyStatsData

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

func (dsm *DailyStatsManager) Stop() {
	close(dsm.stopChan)
}

var GlobalDailyStatsManager *DailyStatsManager

// InitDailyStatsManager initializes the global daily statistics manager
func InitDailyStatsManager() {
	if GlobalDailyStatsManager == nil {
		GlobalDailyStatsManager = NewDailyStatsManager()
	}
}

// StopDailyStatsManager stops the global daily statistics manager
func StopDailyStatsManager() {
	if GlobalDailyStatsManager != nil {
		GlobalDailyStatsManager.Stop()
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
	totalPluginSuccess := uint64(0)
	totalPluginFailures := uint64(0)

	for _, data := range allData {
		if _, exists := projectStats[data.ProjectID]; !exists {
			projectStats[data.ProjectID] = make(map[string]uint64)
		}

		projectStats[data.ProjectID][data.ComponentType] += data.TotalMessages

		// Extract component type from the last part of the sequence
		// This ensures each record is counted only once for the correct component type
		actualComponentType := GetComponentTypeFromSequence(data.ProjectNodeSequence, data.ComponentType)

		// Count this record for the determined component type
		switch actualComponentType {
		case "input":
			totalInputMessages += data.TotalMessages
		case "output":
			totalOutputMessages += data.TotalMessages
		case "ruleset":
			totalRulesetMessages += data.TotalMessages
		case "plugin_success":
			totalPluginSuccess += data.TotalMessages
		case "plugin_failure":
			totalPluginFailures += data.TotalMessages
		}
	}

	// Build project breakdown using the same component type classification as totals
	projectBreakdown := make(map[string]map[string]uint64) // projectID -> {input, output, ruleset}
	for _, data := range allData {
		if _, exists := projectBreakdown[data.ProjectID]; !exists {
			projectBreakdown[data.ProjectID] = map[string]uint64{
				"input":   0,
				"output":  0,
				"ruleset": 0,
			}
		}

		// Use the same component type classification logic as totals
		actualComponentType := GetComponentTypeFromSequence(data.ProjectNodeSequence, data.ComponentType)
		switch actualComponentType {
		case "input":
			projectBreakdown[data.ProjectID]["input"] += data.TotalMessages
		case "output":
			projectBreakdown[data.ProjectID]["output"] += data.TotalMessages
		case "ruleset":
			projectBreakdown[data.ProjectID]["ruleset"] += data.TotalMessages
			// Note: plugin_success and plugin_failure are not included in project breakdown
		}
	}

	return map[string]interface{}{
		"date":                   date,
		"total_input_messages":   totalInputMessages,
		"total_output_messages":  totalOutputMessages,
		"total_ruleset_messages": totalRulesetMessages,
		"total_plugin_success":   totalPluginSuccess,
		"total_plugin_failures":  totalPluginFailures,
		"project_breakdown":      projectBreakdown, // Changed from "projects" to match frontend expectation
		"timestamp":              time.Now(),
	}
}
