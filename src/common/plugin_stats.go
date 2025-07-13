package common

// RecordPluginInvoke is deprecated and no longer used.
// Plugin statistics are now handled by atomic counters in plugin instances
// and collected every 10 seconds through the Daily Stats Manager system.
// This function is kept for backward compatibility but does nothing.
func RecordPluginInvoke(pluginName string, success bool) {
	// This function is deprecated and no longer used.
	// Plugin statistics are now handled through:
	// 1. Plugin.RecordInvocation() - atomic counters in memory
	// 2. collectAllComponentStats() - periodic collection every 10 seconds
	// 3. Daily Stats Manager - unified Redis write system
}
