package common

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrorLogEntry represents an error log entry for Redis storage
type ErrorLogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Source    string                 `json:"source"` // "hub" or "plugin"
	NodeID    string                 `json:"node_id"`
	Function  string                 `json:"function,omitempty"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// WriteErrorLogToRedis writes an error log entry to Redis
func WriteErrorLogToRedis(entry ErrorLogEntry) error {
	if rdb == nil {
		return fmt.Errorf("Redis client not initialized")
	}

	// Convert entry to JSON
	jsonData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal error log entry: %w", err)
	}

	// Use Redis key pattern: cluster:error_logs:{nodeID}
	key := fmt.Sprintf("cluster:error_logs:%s", entry.NodeID)

	// Use LPUSH to add to the front of the list (newest first)
	// Keep only the last 10000 entries per node
	if err := RedisLPush(key, string(jsonData), 10000); err != nil {
		return fmt.Errorf("failed to push error log to Redis: %w", err)
	}

	// Set TTL for the key to 7 days (7 * 24 * 60 * 60 = 604800 seconds)
	if err := RedisExpire(key, 7*24*60*60); err != nil {
		// Don't fail if TTL setting fails, just log it
		return nil
	}

	return nil
}

// GetErrorLogsFromRedis retrieves error logs from Redis for all nodes or a specific node
func GetErrorLogsFromRedis(nodeID string, limit int, offset int) ([]ErrorLogEntry, error) {
	if rdb == nil {
		return nil, fmt.Errorf("Redis client not initialized")
	}

	var allEntries []ErrorLogEntry

	if nodeID == "" || nodeID == "all" {
		// Get logs from all nodes
		pattern := "cluster:error_logs:*"
		keys, err := RedisKeys(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to get error log keys: %w", err)
		}

		// Collect entries from all nodes with their timestamps for sorting
		type timestampedEntry struct {
			entry     ErrorLogEntry
			timestamp int64
		}
		var timestampedEntries []timestampedEntry

		for _, key := range keys {
			entries, err := getErrorLogsFromKey(key, -1, 0) // Get all entries from each node
			if err != nil {
				continue // Skip this node if error
			}
			for _, entry := range entries {
				timestampedEntries = append(timestampedEntries, timestampedEntry{
					entry:     entry,
					timestamp: entry.Timestamp.Unix(),
				})
			}
		}

		// totalCount = len(timestampedEntries) // Not needed for simple pagination

		// Use efficient sorting algorithm (merge sort via sort.Slice)
		sort.Slice(timestampedEntries, func(i, j int) bool {
			return timestampedEntries[i].timestamp > timestampedEntries[j].timestamp // newest first
		})

		// Apply pagination efficiently
		start := offset
		end := offset + limit
		if start >= len(timestampedEntries) {
			return []ErrorLogEntry{}, nil
		}
		if end > len(timestampedEntries) {
			end = len(timestampedEntries)
		}

		// Extract entries from paginated slice
		for i := start; i < end; i++ {
			allEntries = append(allEntries, timestampedEntries[i].entry)
		}
	} else {
		// Get logs from specific node with direct pagination
		key := fmt.Sprintf("cluster:error_logs:%s", nodeID)
		entries, err := getErrorLogsFromKey(key, limit, offset)
		if err != nil {
			return nil, err
		}
		allEntries = entries
	}

	return allEntries, nil
}

// GetErrorLogsFromRedisWithFilter retrieves error logs with server-side filtering
func GetErrorLogsFromRedisWithFilter(nodeID string, source string, startTime, endTime time.Time, keyword string, limit int, offset int) ([]ErrorLogEntry, int, error) {
	if rdb == nil {
		return nil, 0, fmt.Errorf("Redis client not initialized")
	}

	var allEntries []ErrorLogEntry
	var filteredEntries []ErrorLogEntry

	if nodeID == "" || nodeID == "all" {
		// Get logs from all nodes
		pattern := "cluster:error_logs:*"
		keys, err := RedisKeys(pattern)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to get error log keys: %w", err)
		}

		for _, key := range keys {
			entries, err := getErrorLogsFromKey(key, -1, 0) // Get all entries from each node
			if err != nil {
				continue // Skip this node if error
			}
			allEntries = append(allEntries, entries...)
		}
	} else {
		// Get logs from specific node
		key := fmt.Sprintf("cluster:error_logs:%s", nodeID)
		entries, err := getErrorLogsFromKey(key, -1, 0)
		if err != nil {
			return nil, 0, err
		}
		allEntries = entries
	}

	// Apply filters
	for _, entry := range allEntries {
		// Source filter
		if source != "" && source != "all" && source != entry.Source {
			continue
		}

		// Time range filter
		if !startTime.IsZero() && entry.Timestamp.Before(startTime) {
			continue
		}
		if !endTime.IsZero() && entry.Timestamp.After(endTime) {
			continue
		}

		// Keyword filter
		if keyword != "" {
			keywordLower := strings.ToLower(keyword)
			messageMatch := strings.Contains(strings.ToLower(entry.Message), keywordLower)
			functionMatch := strings.Contains(strings.ToLower(entry.Function), keywordLower)
			fileMatch := strings.Contains(strings.ToLower(entry.File), keywordLower)
			errorMatch := strings.Contains(strings.ToLower(entry.Error), keywordLower)

			if !messageMatch && !functionMatch && !fileMatch && !errorMatch {
				continue
			}
		}

		filteredEntries = append(filteredEntries, entry)
	}

	totalCount := len(filteredEntries)

	// Sort by timestamp (newest first) using efficient algorithm
	sort.Slice(filteredEntries, func(i, j int) bool {
		return filteredEntries[i].Timestamp.After(filteredEntries[j].Timestamp)
	})

	// Apply pagination
	start := offset
	end := offset + limit
	if start >= len(filteredEntries) {
		return []ErrorLogEntry{}, totalCount, nil
	}
	if end > len(filteredEntries) {
		end = len(filteredEntries)
	}

	return filteredEntries[start:end], totalCount, nil
}

// getErrorLogsFromKey retrieves error logs from a specific Redis key
func getErrorLogsFromKey(key string, limit int, offset int) ([]ErrorLogEntry, error) {
	var entries []ErrorLogEntry

	// Calculate Redis range parameters
	start := int64(offset)
	stop := int64(-1) // -1 means get all from start
	if limit > 0 {
		stop = start + int64(limit) - 1
	}

	// Get entries from Redis list
	jsonEntries, err := RedisLRange(key, start, stop)
	if err != nil {
		return nil, fmt.Errorf("failed to get error logs from Redis: %w", err)
	}

	// Parse JSON entries
	for _, jsonEntry := range jsonEntries {
		var entry ErrorLogEntry
		if err := json.Unmarshal([]byte(jsonEntry), &entry); err != nil {
			continue // Skip invalid entries
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetErrorLogStats returns statistics about error logs in Redis
func GetErrorLogStats() (map[string]int, error) {
	if rdb == nil {
		return nil, fmt.Errorf("Redis client not initialized")
	}

	stats := make(map[string]int)
	pattern := "cluster:error_logs:*"
	keys, err := RedisKeys(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to get error log keys: %w", err)
	}

	for _, key := range keys {
		// Extract node ID from key
		nodeID := key[len("cluster:error_logs:"):]

		// Get list length
		length, err := rdb.LLen(ctx, key).Result()
		if err != nil {
			continue
		}

		stats[nodeID] = int(length)
	}

	return stats, nil
}
