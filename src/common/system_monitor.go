package common

import (
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"AgentSmith-HUB/logger"
)

// SystemMetrics represents system resource metrics at a point in time
type SystemMetrics struct {
	NodeID         string    `json:"node_id"`
	CPUPercent     float64   `json:"cpu_percent"`     // CPU usage percentage
	MemoryUsedMB   float64   `json:"memory_used_mb"`  // Memory usage in MB
	MemoryPercent  float64   `json:"memory_percent"`  // Memory usage percentage
	GoroutineCount int       `json:"goroutine_count"` // Number of goroutines
	Timestamp      time.Time `json:"timestamp"`
}

// SystemDataPoint represents a single system metrics measurement
type SystemDataPoint struct {
	CPUPercent     float64   `json:"cpu_percent"`
	MemoryUsedMB   float64   `json:"memory_used_mb"`
	MemoryPercent  float64   `json:"memory_percent"`
	GoroutineCount int       `json:"goroutine_count"`
	Timestamp      time.Time `json:"timestamp"`
}

// SystemMonitor monitors and stores system resource usage
type SystemMonitor struct {
	nodeID          string
	dataPoints      []SystemDataPoint
	mutex           sync.RWMutex
	stopChan        chan struct{}
	collectInterval time.Duration
	cleanupWg       sync.WaitGroup

	// For real CPU calculation
	prevCPUTime  time.Duration
	prevWallTime time.Time
	firstMeasure bool
}

// NewSystemMonitor creates a new system monitor instance
func NewSystemMonitor(nodeID string) *SystemMonitor {
	sm := &SystemMonitor{
		nodeID:          nodeID,
		dataPoints:      make([]SystemDataPoint, 0),
		stopChan:        make(chan struct{}),
		collectInterval: 30 * time.Second, // Collect every 30 seconds
		firstMeasure:    true,
	}

	// Start collection and cleanup goroutines
	sm.cleanupWg.Add(2)
	go sm.collectLoop()
	go sm.cleanupLoop()

	return sm
}

// Start starts the system monitor (already started in NewSystemMonitor)
func (sm *SystemMonitor) Start() {
	logger.Info("System monitor started", "node_id", sm.nodeID, "collect_interval", sm.collectInterval)
}

// Stop stops the system monitor
func (sm *SystemMonitor) Stop() {
	logger.Info("Stopping system monitor", "node_id", sm.nodeID)

	close(sm.stopChan)
	sm.cleanupWg.Wait()

	logger.Info("System monitor stopped")
}

// collectLoop periodically collects system metrics
func (sm *SystemMonitor) collectLoop() {
	defer sm.cleanupWg.Done()

	// Initial collection
	sm.collectMetrics()

	ticker := time.NewTicker(sm.collectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopChan:
			return
		case <-ticker.C:
			sm.collectMetrics()
		}
	}
}

// collectMetrics collects current system metrics
func (sm *SystemMonitor) collectMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	now := time.Now()

	// Calculate CPU usage (simplified approach)
	cpuPercent := sm.calculateCPUPercent()

	// Memory metrics - use RSS for accurate process memory usage
	memoryUsedMB := getProcessRSSMB()

	// Get system total memory for accurate percentage calculation
	systemMemoryMB := getSystemMemoryMB()

	// Calculate memory percentage relative to system total memory
	var memoryPercent float64
	if systemMemoryMB > 0 {
		memoryPercent = (memoryUsedMB / systemMemoryMB) * 100
	} else {
		// Fallback to runtime-based calculation if system memory detection fails
		memoryPercent = float64(memStats.Alloc) / float64(memStats.Sys) * 100
	}

	if memoryPercent > 100 {
		memoryPercent = 100
	}

	// Goroutine count
	goroutineCount := runtime.NumGoroutine()

	dataPoint := SystemDataPoint{
		CPUPercent:     cpuPercent,
		MemoryUsedMB:   memoryUsedMB,
		MemoryPercent:  memoryPercent,
		GoroutineCount: goroutineCount,
		Timestamp:      now,
	}

	sm.mutex.Lock()
	sm.dataPoints = append(sm.dataPoints, dataPoint)
	sm.mutex.Unlock()
}

// calculateCPUPercent calculates CPU usage percentage for the current process using real CPU time
func (sm *SystemMonitor) calculateCPUPercent() float64 {
	now := time.Now()
	cpuTime := getCurrentProcessCPUTime()

	if sm.firstMeasure || sm.prevWallTime.IsZero() {
		// First measurement, can't calculate percentage yet
		sm.prevCPUTime = cpuTime
		sm.prevWallTime = now
		sm.firstMeasure = false
		return 0.1 // Return minimal value for first measurement
	}

	// Calculate time differences
	wallTimeDiff := now.Sub(sm.prevWallTime)
	cpuTimeDiff := cpuTime - sm.prevCPUTime

	// Update for next calculation
	sm.prevCPUTime = cpuTime
	sm.prevWallTime = now

	if wallTimeDiff <= 0 {
		return 0.1
	}

	// Calculate CPU percentage relative to system total CPU
	// cpuTimeDiff is the CPU time consumed by the process
	// wallTimeDiff is the wall clock time elapsed
	// cpuTimeDiff / wallTimeDiff gives CPU utilization
	// Dividing by numCPU normalizes to 0-100% range
	cpuPercent := float64(cpuTimeDiff) / float64(wallTimeDiff) * 100

	// Normalize to per-core usage (0-100%)
	// This represents the true percentage of a single core used
	// For example, 100% means using one full core, 50% means using half a core
	numCPU := getCPULimit()
	if numCPU > 0 {
		cpuPercent = cpuPercent / numCPU
	}

	// Cap at 100% (a process can't use more than 100% of a single core's time)
	if cpuPercent > 100 {
		cpuPercent = 100
	}

	// Ensure minimum value for running process
	if cpuPercent < 0.01 {
		cpuPercent = 0.01
	}

	return cpuPercent
}

// getCurrentProcessCPUTime gets the current process CPU time (user + system time)
func getCurrentProcessCPUTime() time.Duration {
	var usage syscall.Rusage
	err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
	if err != nil {
		// Fallback to a minimal value if syscall fails
		return time.Duration(0)
	}

	// Convert timeval to duration (user time + system time)
	userTime := time.Duration(usage.Utime.Sec)*time.Second + time.Duration(usage.Utime.Usec)*time.Microsecond
	sysTime := time.Duration(usage.Stime.Sec)*time.Second + time.Duration(usage.Stime.Usec)*time.Microsecond

	return userTime + sysTime
}

// getProcessRSSMB gets the current process RSS (Resident Set Size) memory in MB
func getProcessRSSMB() float64 {
	// For Linux, read from /proc/self/status
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/self/status"); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "VmRSS:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						if kb, err := strconv.ParseFloat(fields[1], 64); err == nil {
							return kb / 1024 // Convert KB to MB
						}
					}
					break
				}
			}
		}
	}

	// For macOS and other Unix systems, use ps command to get RSS
	if runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" {
		cmd := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid()))
		output, err := cmd.Output()
		if err == nil {
			if rssKB, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64); err == nil {
				return rssKB / 1024 // Convert KB to MB
			}
		}
	}

	// Fallback to runtime memStats.Alloc if we can't get RSS
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return float64(memStats.Alloc) / 1024 / 1024
}
func getCPULimit() float64 {
	// Cgroup v2
	cgroupV2Path := "/sys/fs/cgroup/cpu.max"
	if _, err := os.Stat(cgroupV2Path); err == nil {
		if data, err := os.ReadFile(cgroupV2Path); err == nil {
			str := strings.TrimSpace(string(data))
			if str != "max" {
				fields := strings.Fields(str)
				if len(fields) >= 2 {
					if quota, err := strconv.ParseFloat(fields[0], 64); err == nil {
						if period, err := strconv.ParseFloat(fields[1], 64); err == nil {
							return quota / period
						}
					}
				}
			}
		}
	}

	// Cgroup v1
	quotaPath := "/sys/fs/cgroup/cpu/cpu.cfs_quota_us"
	periodPath := "/sys/fs/cgroup/cpu/cpu.cfs_period_us"

	if _, err := os.Stat(quotaPath); err == nil {
		if quotaData, err := os.ReadFile(quotaPath); err == nil {
			if periodData, err := os.ReadFile(periodPath); err == nil {
				if quota, err := strconv.ParseFloat(strings.TrimSpace(string(quotaData)), 64); err == nil {
					if period, err := strconv.ParseFloat(strings.TrimSpace(string(periodData)), 64); err == nil {
						if quota > 0 {
							return quota / period
						}
					}
				}
			}
		}
	}
	return float64(runtime.NumCPU())
}

// getSystemMemoryMB attempts to get system total memory in MB
func getSystemMemoryMB() float64 {
	for _, p := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		if res, err := os.ReadFile(p); err == nil {
			res := strings.TrimSpace(string(res))
			if res != "max" {
				if limit, err := strconv.ParseUint(res, 10, 64); err == nil && limit < math.MaxInt64/2 {
					return float64(limit) / 1024 / 1024
				}
			}
		}

	}
	// Try to read from /proc/meminfo on Linux
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseFloat(fields[1], 64); err == nil {
						return kb / 1024 // Convert KB to MB
					}
				}
				break
			}
		}
	}

	// For macOS, use sysctl to get actual system memory
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("sysctl", "-n", "hw.memsize")
		output, err := cmd.Output()
		if err == nil {
			if memsizeBytes, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64); err == nil {
				return memsizeBytes / 1024 / 1024
			}
		}
		// Fallback to conservative estimate if sysctl fails
		return 16.0 * 1024
	}

	// For other systems or as general fallback
	// Use a more reasonable estimation based on typical modern systems
	return 8.0 * 1024 // Assume at least 8GB for modern systems
}

// cleanupLoop periodically removes old data (older than 1 hour)
func (sm *SystemMonitor) cleanupLoop() {
	defer sm.cleanupWg.Done()

	ticker := time.NewTicker(5 * time.Minute) // Cleanup every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopChan:
			return
		case <-ticker.C:
			sm.cleanup()
		}
	}
}

// cleanup removes data older than 1 hour
func (sm *SystemMonitor) cleanup() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	cutoffTime := time.Now().Add(-time.Hour)

	var validPoints []SystemDataPoint
	for _, point := range sm.dataPoints {
		if point.Timestamp.After(cutoffTime) {
			validPoints = append(validPoints, point)
		}
	}

	oldCount := len(sm.dataPoints)
	sm.dataPoints = validPoints

	if oldCount != len(sm.dataPoints) {
		logger.Debug("System metrics cleanup completed",
			"node_id", sm.nodeID,
			"removed_points", oldCount-len(sm.dataPoints),
			"remaining_points", len(sm.dataPoints))
	}
}

// GetCurrentMetrics returns the latest system metrics
func (sm *SystemMonitor) GetCurrentMetrics() *SystemMetrics {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	if len(sm.dataPoints) == 0 {
		return nil
	}

	latest := sm.dataPoints[len(sm.dataPoints)-1]
	return &SystemMetrics{
		NodeID:         sm.nodeID,
		CPUPercent:     latest.CPUPercent,
		MemoryUsedMB:   latest.MemoryUsedMB,
		MemoryPercent:  latest.MemoryPercent,
		GoroutineCount: latest.GoroutineCount,
		Timestamp:      latest.Timestamp,
	}
}

// GetHistoricalMetrics returns historical system metrics within the specified time range
func (sm *SystemMonitor) GetHistoricalMetrics(since time.Time) []SystemDataPoint {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	var result []SystemDataPoint
	for _, point := range sm.dataPoints {
		if point.Timestamp.After(since) {
			result = append(result, point)
		}
	}

	return result
}

// GetAllMetrics returns all stored system metrics
func (sm *SystemMonitor) GetAllMetrics() []SystemDataPoint {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	// Return a copy to prevent external modification
	result := make([]SystemDataPoint, len(sm.dataPoints))
	copy(result, sm.dataPoints)
	return result
}

// GetStats returns statistics about the system monitor
func (sm *SystemMonitor) GetStats() map[string]interface{} {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	var totalCPU, totalMemory float64
	minMemory, maxMemory := float64(0), float64(0)
	minCPU, maxCPU := float64(0), float64(0)

	if len(sm.dataPoints) > 0 {
		minMemory = sm.dataPoints[0].MemoryUsedMB
		maxMemory = sm.dataPoints[0].MemoryUsedMB
		minCPU = sm.dataPoints[0].CPUPercent
		maxCPU = sm.dataPoints[0].CPUPercent

		for _, point := range sm.dataPoints {
			totalCPU += point.CPUPercent
			totalMemory += point.MemoryUsedMB

			if point.MemoryUsedMB < minMemory {
				minMemory = point.MemoryUsedMB
			}
			if point.MemoryUsedMB > maxMemory {
				maxMemory = point.MemoryUsedMB
			}
			if point.CPUPercent < minCPU {
				minCPU = point.CPUPercent
			}
			if point.CPUPercent > maxCPU {
				maxCPU = point.CPUPercent
			}
		}
	}

	stats := map[string]interface{}{
		"node_id":           sm.nodeID,
		"total_data_points": len(sm.dataPoints),
		"data_retention":    "1 hour",
		"collect_interval":  sm.collectInterval.String(),
	}

	if len(sm.dataPoints) > 0 {
		current := sm.dataPoints[len(sm.dataPoints)-1]
		stats["current_cpu_percent"] = current.CPUPercent
		stats["current_memory_mb"] = current.MemoryUsedMB
		stats["current_memory_percent"] = current.MemoryPercent
		stats["current_goroutines"] = current.GoroutineCount

		stats["avg_cpu_percent"] = totalCPU / float64(len(sm.dataPoints))
		stats["avg_memory_mb"] = totalMemory / float64(len(sm.dataPoints))
		stats["min_memory_mb"] = minMemory
		stats["max_memory_mb"] = maxMemory
		stats["min_cpu_percent"] = minCPU
		stats["max_cpu_percent"] = maxCPU

		// Time range
		stats["oldest_data"] = sm.dataPoints[0].Timestamp
		stats["latest_data"] = current.Timestamp
	}

	return stats
}

// Global system monitor instance
var GlobalSystemMonitor *SystemMonitor

// InitSystemMonitor initializes the global system monitor
func InitSystemMonitor(nodeID string) {
	if GlobalSystemMonitor == nil {
		GlobalSystemMonitor = NewSystemMonitor(nodeID)
		GlobalSystemMonitor.Start()
		logger.Info("System monitor initialized", "node_id", nodeID)
	}
}

// ClusterSystemManager manages system metrics from all cluster nodes (only on leader)
type ClusterSystemManager struct {
	// Key format: "nodeID"
	data      map[string]*SystemMetrics
	mutex     sync.RWMutex
	stopChan  chan struct{}
	cleanupWg sync.WaitGroup
}

// NewClusterSystemManager creates a new cluster system manager instance
func NewClusterSystemManager() *ClusterSystemManager {
	csm := &ClusterSystemManager{
		data:     make(map[string]*SystemMetrics),
		stopChan: make(chan struct{}),
	}

	// Start cleanup goroutine to remove old data
	csm.cleanupWg.Add(1)
	go csm.cleanupLoop()

	// Note: System metrics are now received via heartbeat mechanism
	// See HeartbeatManager.listenHeartbeats() for follower metrics handling

	return csm
}

// AddSystemMetrics adds or updates system metrics for a node
func (csm *ClusterSystemManager) AddSystemMetrics(metrics *SystemMetrics) {
	if metrics == nil {
		return
	}

	csm.mutex.Lock()
	defer csm.mutex.Unlock()

	// Store the metrics with current timestamp
	metrics.Timestamp = time.Now()
	csm.data[metrics.NodeID] = metrics
}

// GetNodeMetrics returns system metrics for a specific node
func (csm *ClusterSystemManager) GetNodeMetrics(nodeID string) *SystemMetrics {
	csm.mutex.RLock()
	defer csm.mutex.RUnlock()

	if metrics, exists := csm.data[nodeID]; exists {
		// Return a copy to prevent external modification
		return &SystemMetrics{
			NodeID:         metrics.NodeID,
			CPUPercent:     metrics.CPUPercent,
			MemoryUsedMB:   metrics.MemoryUsedMB,
			MemoryPercent:  metrics.MemoryPercent,
			GoroutineCount: metrics.GoroutineCount,
			Timestamp:      metrics.Timestamp,
		}
	}

	return nil
}

// GetAllMetrics returns system metrics for all nodes
func (csm *ClusterSystemManager) GetAllMetrics() map[string]*SystemMetrics {
	csm.mutex.RLock()
	defer csm.mutex.RUnlock()

	result := make(map[string]*SystemMetrics)
	for nodeID, metrics := range csm.data {
		// Return copies to prevent external modification
		result[nodeID] = &SystemMetrics{
			NodeID:         metrics.NodeID,
			CPUPercent:     metrics.CPUPercent,
			MemoryUsedMB:   metrics.MemoryUsedMB,
			MemoryPercent:  metrics.MemoryPercent,
			GoroutineCount: metrics.GoroutineCount,
			Timestamp:      metrics.Timestamp,
		}
	}

	return result
}

// cleanupLoop periodically removes old data (older than 5 minutes)
func (csm *ClusterSystemManager) cleanupLoop() {
	defer csm.cleanupWg.Done()

	ticker := time.NewTicker(2 * time.Minute) // Cleanup every 2 minutes
	defer ticker.Stop()

	for {
		select {
		case <-csm.stopChan:
			return
		case <-ticker.C:
			csm.cleanup()
		}
	}
}

// cleanup removes data older than 5 minutes (nodes that haven't reported recently)
func (csm *ClusterSystemManager) cleanup() {
	csm.mutex.Lock()
	defer csm.mutex.Unlock()

	cutoffTime := time.Now().Add(-5 * time.Minute)

	for nodeID, metrics := range csm.data {
		if metrics.Timestamp.Before(cutoffTime) {
			delete(csm.data, nodeID)
			logger.Debug("Removed stale system metrics for node", "node_id", nodeID)
		}
	}
}

// Stop stops the cluster system manager and cleanup goroutine
func (csm *ClusterSystemManager) Stop() {
	close(csm.stopChan)
	csm.cleanupWg.Wait()
}

// GetAggregatedMetrics returns aggregated system metrics across all nodes
func (csm *ClusterSystemManager) GetAggregatedMetrics() map[string]interface{} {
	csm.mutex.RLock()
	defer csm.mutex.RUnlock()

	totalNodes := len(csm.data)
	var totalCPU, totalMemoryPercent, totalMemoryMB float64
	var totalGoroutines int

	if totalNodes == 0 {
		return map[string]interface{}{
			"avg_cpu_percent":    0,
			"avg_memory_percent": 0,
			"total_memory_mb":    0,
			"total_goroutines":   0,
			"total_nodes":        0,
			"timestamp":          time.Now(),
		}
	}

	for _, metrics := range csm.data {
		totalCPU += metrics.CPUPercent
		totalMemoryPercent += metrics.MemoryPercent
		totalMemoryMB += metrics.MemoryUsedMB
		totalGoroutines += metrics.GoroutineCount
	}

	return map[string]interface{}{
		"avg_cpu_percent":    totalCPU / float64(totalNodes),
		"avg_memory_percent": totalMemoryPercent / float64(totalNodes),
		"total_memory_mb":    totalMemoryMB,
		"total_goroutines":   totalGoroutines,
		"total_nodes":        totalNodes,
		"timestamp":          time.Now(),
	}
}

// GetStats returns statistics about the cluster system manager
func (csm *ClusterSystemManager) GetStats() map[string]interface{} {
	csm.mutex.RLock()
	defer csm.mutex.RUnlock()

	totalNodes := len(csm.data)
	var avgCPU, avgMemory float64

	if totalNodes > 0 {
		for _, metrics := range csm.data {
			avgCPU += metrics.CPUPercent
			avgMemory += metrics.MemoryUsedMB
		}
		avgCPU /= float64(totalNodes)
		avgMemory /= float64(totalNodes)
	}

	return map[string]interface{}{
		"total_nodes":      totalNodes,
		"avg_cpu_percent":  avgCPU,
		"avg_memory_mb":    avgMemory,
		"data_retention":   "5 minutes",
		"cleanup_interval": "2 minutes",
	}
}

// Global cluster system manager instance (only on leader)
var GlobalClusterSystemManager *ClusterSystemManager

// InitClusterSystemManager initializes the global cluster system manager (only call on leader)
func InitClusterSystemManager() {
	if GlobalClusterSystemManager == nil {
		GlobalClusterSystemManager = NewClusterSystemManager()
		logger.Info("Cluster system manager initialized")
	}
}

// StopClusterSystemManager stops the global cluster system manager
func StopClusterSystemManager() {
	if GlobalClusterSystemManager != nil {
		GlobalClusterSystemManager.Stop()
		GlobalClusterSystemManager = nil
	}
}
