package common

import (
	"fmt"
	"time"
)

// RecordPluginInvoke increments daily success/failure counter for a plugin.
// success = true => increment success, else failure.
func RecordPluginInvoke(pluginName string, success bool) {
	// Require Redis client to be initialized.
	if GetRedisClient() == nil {
		return
	}
	date := time.Now().Format("2006-01-02")
	status := "success"
	if !success {
		status = "failure"
	}
	key := fmt.Sprintf("plugin_stats:%s:%s:%s", date, pluginName, status)
	_, _ = RedisIncr(key)
}
