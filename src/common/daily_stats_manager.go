package common

import (
	"AgentSmith-HUB/logger"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
		saveInterval:   10 * time.Second,   // Collect every 10 seconds
		retentionDays:  10,                 // Keep 30 days of data
	}

	go dsm.persistenceLoop()
	return dsm
}

func (dsm *DailyStatsManager) writeToRedisLegacy(statsData *DailyStatsData, increment uint64, expiration int) error {
	key := dsm.generateKey(statsData.Date, statsData.NodeID, statsData.ProjectID, statsData.ProjectNodeSequence)
	counterKey := dsm.redisKeyPrefix + key
	_, err := RedisIncrby(counterKey, int64(increment))
	if err != nil {
		return err
	}

	err = RedisExpire(counterKey, expiration)
	if err != nil {
		return err
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
		}
		result[dailyData.ProjectNodeSequence] = &dailyData
	}

	return result
}

// persistenceLoop periodically collects data from all components and saves to Redis
func (dsm *DailyStatsManager) persistenceLoop() {
	ticker := time.NewTicker(dsm.saveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dsm.stopChan:
			return
		case <-ticker.C:
			dsm.CollectAllComponentsData()
		}
	}
}

func (dsm *DailyStatsManager) CollectAllComponentsData() {
	if statsCollector != nil {
		dsm.ApplyBatchUpdates(GetStatsCollector()())
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

		if err := dsm.writeToRedisLegacy(&data, data.TotalMessages, expiration); err != nil {
			logger.Error("Failed to write statistics increment",
				"component", data.ComponentID,
				"sequence", data.ProjectNodeSequence,
				"increment", data,
				"error", err)
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

		// Aggregate by component type based on ProjectNodeSequence prefix
		sequence := data.ProjectNodeSequence
		switch {
		case strings.HasPrefix(sequence, "INPUT."):
			totalInputMessages += data.TotalMessages
		case strings.HasPrefix(sequence, "OUTPUT."):
			totalOutputMessages += data.TotalMessages
		case strings.HasPrefix(sequence, "RULESET.") || strings.Contains(sequence, ".RULESET."):
			totalRulesetMessages += data.TotalMessages
		case strings.HasPrefix(sequence, "PLUGIN.") && strings.HasSuffix(sequence, ".success"):
			totalPluginSuccess += data.TotalMessages
		case strings.HasPrefix(sequence, "PLUGIN.") && strings.HasSuffix(sequence, ".failure"):
			totalPluginFailures += data.TotalMessages
		}
	}

	return map[string]interface{}{
		"date":                   date,
		"total_input_messages":   totalInputMessages,
		"total_output_messages":  totalOutputMessages,
		"total_ruleset_messages": totalRulesetMessages,
		"total_plugin_success":   totalPluginSuccess,
		"total_plugin_failures":  totalPluginFailures,
		"projects":               projectStats,
		"timestamp":              time.Now(),
	}
}
