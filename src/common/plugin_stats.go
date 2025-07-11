package common

import (
	"fmt"
	"time"
)

// RecordPluginInvoke increments daily success/failure counter for a plugin.
// success = true => increment success, else failure.
// Now includes nodeID to prevent data collision across cluster nodes.
func RecordPluginInvoke(pluginName string, success bool) {
	// Require Redis client to be initialized.
	if GetRedisClient() == nil {
		return
	}

	// Get current node ID (use LocalIP as node identifier)
	nodeID := Config.LocalIP
	if nodeID == "" {
		// Fallback to "unknown" if node ID is not available
		nodeID = "unknown"
	}

	date := time.Now().Format("2006-01-02")
	status := "success"
	if !success {
		status = "failure"
	}

	// Updated key format: plugin_stats:{date}:{nodeID}:{pluginName}:{status}
	key := fmt.Sprintf("plugin_stats:%s:%s:%s:%s", date, nodeID, pluginName, status)
	_, _ = RedisIncr(key)

	// Ensure the key expires (data retention). Keep 90 days for historical lookup.
	// Expire is refreshed on each increment so counters for today persist.
	_ = RedisExpire(key, 90*24*60*60)
}
