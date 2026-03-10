package common

import (
	"fmt"
	"strconv"
	"time"
)

// AgentDailyStats represents per-agent daily statistics stored in Redis.
// All values are aggregated across the cluster for a given date.
type AgentDailyStats struct {
	CallCount    uint64  `json:"call_count"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

const agentDailyStatsPrefix = "hub:agent_daily_stats:"
const agentDailyStatsRetentionDays = 10

// IncrementAgentDailyStats increments today's call count and latency sum (ns) for the given agent.
// This is safe to call from any node; Redis aggregates counts across the cluster.
func IncrementAgentDailyStats(agentID string, latencyNs uint64) error {
	if agentID == "" {
		return fmt.Errorf("agentID is empty")
	}

	date := time.Now().Format("2006-01-02")
	countKey := fmt.Sprintf("%s%s:count", agentDailyStatsPrefix, date)
	latencyKey := fmt.Sprintf("%s%s:latency_ns", agentDailyStatsPrefix, date)

	rdb := GetRedisClient()
	if rdb == nil {
		return fmt.Errorf("redis client not initialized")
	}

	ttl := time.Duration(agentDailyStatsRetentionDays) * 24 * time.Hour

	pipe := rdb.TxPipeline()
	pipe.HIncrBy(ctx, countKey, agentID, 1)
	pipe.HIncrBy(ctx, latencyKey, agentID, int64(latencyNs))
	pipe.Expire(ctx, countKey, ttl)
	pipe.Expire(ctx, latencyKey, ttl)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to increment agent daily stats: %w", err)
	}
	return nil
}

// GetAgentDailyStats returns daily stats for a specific agent on a given date (YYYY-MM-DD).
// If date is empty, today's date is used.
func GetAgentDailyStats(date string, agentID string) (AgentDailyStats, error) {
	var result AgentDailyStats
	if agentID == "" {
		return result, fmt.Errorf("agentID is empty")
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	countKey := fmt.Sprintf("%s%s:count", agentDailyStatsPrefix, date)
	latencyKey := fmt.Sprintf("%s%s:latency_ns", agentDailyStatsPrefix, date)

	countStr, err := RedisHGet(countKey, agentID)
	if err != nil {
		return result, fmt.Errorf("failed to get agent daily count: %w", err)
	}
	if countStr == "" {
		// No data for this agent on this date
		return result, nil
	}
	count, err := strconv.ParseUint(countStr, 10, 64)
	if err != nil {
		return result, fmt.Errorf("failed to parse agent daily count: %w", err)
	}

	latencyStr, err := RedisHGet(latencyKey, agentID)
	if err != nil {
		return result, fmt.Errorf("failed to get agent daily latency: %w", err)
	}
	var latencyNs uint64
	if latencyStr != "" {
		latencyNs, err = strconv.ParseUint(latencyStr, 10, 64)
		if err != nil {
			return result, fmt.Errorf("failed to parse agent daily latency: %w", err)
		}
	}

	result.CallCount = count
	if count > 0 && latencyNs > 0 {
		result.AvgLatencyMs = float64(latencyNs) / float64(count) / 1e6
	}
	return result, nil
}

// GetAllAgentsDailyStats returns daily stats for all agents on a given date (YYYY-MM-DD).
// If date is empty, today's date is used.
func GetAllAgentsDailyStats(date string) (map[string]AgentDailyStats, error) {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	countKey := fmt.Sprintf("%s%s:count", agentDailyStatsPrefix, date)
	latencyKey := fmt.Sprintf("%s%s:latency_ns", agentDailyStatsPrefix, date)

	countMap, err := RedisHGetAll(countKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent daily count hash: %w", err)
	}
	latencyMap, err := RedisHGetAll(latencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent daily latency hash: %w", err)
	}

	result := make(map[string]AgentDailyStats, len(countMap))
	for agentID, countStr := range countMap {
		count, err := strconv.ParseUint(countStr, 10, 64)
		if err != nil {
			continue
		}
		stats := AgentDailyStats{CallCount: count}

		if latStr, ok := latencyMap[agentID]; ok && latStr != "" {
			if latencyNs, err := strconv.ParseUint(latStr, 10, 64); err == nil && count > 0 {
				stats.AvgLatencyMs = float64(latencyNs) / float64(count) / 1e6
			}
		}
		result[agentID] = stats
	}
	return result, nil
}
