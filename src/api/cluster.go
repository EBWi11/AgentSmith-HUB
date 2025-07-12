package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

func getClusterStatus(c echo.Context) error {
	status := cluster.GetClusterStatus()
	return c.JSON(http.StatusOK, status)
}

func getClusterProjectStates(c echo.Context) error {
	// Return cluster status with project states
	status := cluster.GetClusterStatus()

	// Get real project states from Redis for all nodes
	projectStates := make(map[string]interface{})

	// Always try to get current node's project states first
	currentNodeID := common.Config.LocalIP
	if currentNodeID != "" {
		// Get actual project states (real runtime status)
		if nodeStates, err := common.GetAllProjectActualStates(currentNodeID); err == nil {
			// Get timestamps for this node
			nodeTimestamps, _ := common.GetAllProjectStateTimestamps(currentNodeID)

			// Convert to array of project state objects
			var projectList []map[string]interface{}
			for projectID, status := range nodeStates {
				projectData := map[string]interface{}{
					"id":     projectID,
					"status": status,
				}

				// Add timestamp if available
				if timestamp, exists := nodeTimestamps[projectID]; exists {
					projectData["status_changed_at"] = timestamp.Format(time.RFC3339)
				}

				projectList = append(projectList, projectData)
			}
			projectStates[currentNodeID] = projectList
		} else {
			// If Redis error, return empty array for this node
			projectStates[currentNodeID] = []map[string]interface{}{}
		}
	}

	// Get all nodes from cluster status for additional nodes
	if nodes, ok := status["nodes"].([]map[string]interface{}); ok {
		for _, node := range nodes {
			if nodeID, ok := node["id"].(string); ok {
				// Skip if already processed current node
				if nodeID == currentNodeID {
					continue
				}

				// Get actual project states for this node (real runtime status)
				if nodeStates, err := common.GetAllProjectActualStates(nodeID); err == nil {
					// Get timestamps for this node
					nodeTimestamps, _ := common.GetAllProjectStateTimestamps(nodeID)

					// Convert to array of project state objects
					var projectList []map[string]interface{}
					for projectID, status := range nodeStates {
						projectData := map[string]interface{}{
							"id":     projectID,
							"status": status,
						}

						// Add timestamp if available
						if timestamp, exists := nodeTimestamps[projectID]; exists {
							projectData["status_changed_at"] = timestamp.Format(time.RFC3339)
						}

						projectList = append(projectList, projectData)
					}
					projectStates[nodeID] = projectList
				} else {
					// If Redis error, return empty array for this node
					projectStates[nodeID] = []map[string]interface{}{}
				}
			}
		}
	}

	response := map[string]interface{}{
		"cluster_status": status,
		"project_states": projectStates,
	}

	return c.JSON(http.StatusOK, response)
}

func tokenCheck(c echo.Context) error {
	token := c.Request().Header.Get("token")
	if token == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "missing token",
		})
	}

	if token == common.Config.Token {
		return c.JSON(http.StatusOK, map[string]string{
			"status": "Authentication successful",
		})
	} else {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"status": "Authentication failed",
		})
	}
}

func leaderConfig(c echo.Context) error {
	if err := common.RequireLeader(); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "no leader",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"redis":          common.Config.Redis,
		"redis_password": common.Config.RedisPassword,
	})
}

func downloadConfig(c echo.Context) error {
	configRoot := common.Config.ConfigRoot
	if configRoot == "" {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "config root not set",
		})
	}

	// Create a zip file in memory
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Walk through the config directory
	err := filepath.Walk(configRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Create a new file in the zip
		relPath, err := filepath.Rel(configRoot, path)
		if err != nil {
			return err
		}

		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		// Read and write file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		_, err = writer.Write(content)
		return err
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to create zip: %v", err),
		})
	}

	// Close the zip writer
	if err := zipWriter.Close(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to close zip: %v", err),
		})
	}

	// Calculate zip sha256
	hash := sha256.New()
	_, err = hash.Write(buf.Bytes())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to calculate sha256: %v", err),
		})
	}
	zipSha256 := fmt.Sprintf("%x", hash.Sum(nil))

	// Set response headers
	c.Response().Header().Set(echo.HeaderContentType, "application/zip")
	c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename=config.zip")
	c.Response().Header().Set("X-Config-Sha256", zipSha256)

	// Send the zip file
	return c.Blob(http.StatusOK, "application/zip", buf.Bytes())
}

func getCluster(c echo.Context) error {
	status := cluster.GetClusterStatus()
	data, _ := json.Marshal(status)
	return c.String(http.StatusOK, string(data))
}

// getQPSData returns QPS data for query
// Each node can provide its own data - no leader restriction needed
func getQPSData(c echo.Context) error {
	if common.GlobalQPSManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "QPS manager not initialized",
		})
	}

	projectID := c.QueryParam("project_id")
	nodeID := c.QueryParam("node_id")
	componentID := c.QueryParam("component_id")
	componentType := c.QueryParam("component_type")
	aggregated := c.QueryParam("aggregated") == "true"

	var result interface{}

	if aggregated && projectID != "" {
		// Return aggregated QPS data for a project
		result = common.GlobalQPSManager.GetAggregatedQPS(projectID)
	} else if projectID != "" && nodeID == "" {
		// Return all components in a project
		result = common.GlobalQPSManager.GetProjectQPS(projectID)
	} else if nodeID != "" && projectID != "" && componentID != "" && componentType != "" {
		// Return specific component QPS data using legacy method for backward compatibility
		result = common.GlobalQPSManager.GetComponentQPSLegacy(nodeID, projectID, componentID, componentType)
	} else if nodeID == "" && projectID == "" {
		// Return all QPS data
		result = common.GlobalQPSManager.GetAllQPS()
	} else {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid query parameters. Use 'project_id' for project data, or specify 'node_id', 'project_id', 'component_id', and 'component_type' for specific component data",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":      result,
		"timestamp": time.Now(),
		"stats":     common.GlobalQPSManager.GetStats(),
	})
}

// getDailyMessages returns real message counts for today (from 00:00)
// Modified to read directly from Redis via Daily Stats Manager
func getDailyMessages(c echo.Context) error {
	if common.GlobalDailyStatsManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Daily statistics manager not initialized",
		})
	}

	projectID := c.QueryParam("project_id")
	nodeID := c.QueryParam("node_id")
	aggregated := c.QueryParam("aggregated") == "true"
	byNode := c.QueryParam("by_node") == "true"

	// Get date parameter, default to today
	date := c.QueryParam("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	var result interface{}

	if byNode {
		if nodeID != "" {
			// Return message counts for a specific node from Redis
			nodeStats := common.GlobalDailyStatsManager.GetDailyStats(date, "", nodeID)
			nodeResult := map[string]uint64{
				"input_messages":   0,
				"output_messages":  0,
				"ruleset_messages": 0,
			}

			for _, statsData := range nodeStats {
				nodeResult[statsData.ComponentType+"_messages"] += statsData.TotalMessages
			}

			nodeResult["total_messages"] = nodeResult["input_messages"] + nodeResult["output_messages"] + nodeResult["ruleset_messages"]
			result = nodeResult
		} else {
			// Return message counts for all nodes from Redis
			allNodeStats := common.GlobalDailyStatsManager.GetDailyStats(date, "", "")
			nodeResults := make(map[string]map[string]uint64)

			for _, statsData := range allNodeStats {
				if _, exists := nodeResults[statsData.NodeID]; !exists {
					nodeResults[statsData.NodeID] = map[string]uint64{
						"input_messages":   0,
						"output_messages":  0,
						"ruleset_messages": 0,
					}
				}
				nodeResults[statsData.NodeID][statsData.ComponentType+"_messages"] += statsData.TotalMessages
			}

			// Calculate totals for each node
			for nodeID, stats := range nodeResults {
				stats["total_messages"] = stats["input_messages"] + stats["output_messages"] + stats["ruleset_messages"]
				nodeResults[nodeID] = stats
			}

			result = nodeResults
		}
	} else if aggregated {
		// Return aggregated message counts directly from Redis
		result = common.GlobalDailyStatsManager.GetAggregatedDailyStats(date)
	} else {
		// Return message counts for a specific project or all projects from Redis
		dailyStats := common.GlobalDailyStatsManager.GetDailyStats(date, projectID, "")

		// Group by ProjectNodeSequence
		sequenceGroups := make(map[string]map[string]interface{})

		for _, statsData := range dailyStats {
			sequenceKey := statsData.ProjectNodeSequence

			if _, exists := sequenceGroups[sequenceKey]; !exists {
				sequenceGroups[sequenceKey] = map[string]interface{}{
					"component_type":        statsData.ComponentType,
					"project_node_sequence": statsData.ProjectNodeSequence,
					"total_messages":        uint64(0),
					"daily_messages":        uint64(0),
				}
			}

			sequenceGroups[sequenceKey]["total_messages"] = statsData.TotalMessages
			sequenceGroups[sequenceKey]["daily_messages"] = statsData.TotalMessages // For daily stats, these are the same
		}

		result = sequenceGroups
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":        result,
		"timestamp":   time.Now(),
		"period":      "today",
		"period_note": "Message counts are from Redis daily statistics",
		"data_source": "redis",
	})
}

// getSystemMetrics returns current and historical system metrics for this node
func getSystemMetrics(c echo.Context) error {
	if common.GlobalSystemMonitor == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "System monitor not initialized",
		})
	}

	// Parse query parameters
	sinceParam := c.QueryParam("since")
	currentOnly := c.QueryParam("current") == "true"

	if currentOnly {
		// Return only current metrics
		current := common.GlobalSystemMonitor.GetCurrentMetrics()
		if current == nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "No system metrics available",
			})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"current":   current,
			"timestamp": time.Now(),
		})
	}

	var historical []common.SystemDataPoint
	if sinceParam != "" {
		// Parse since timestamp
		since, err := time.Parse(time.RFC3339, sinceParam)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("Invalid since parameter format: %v", err),
			})
		}
		historical = common.GlobalSystemMonitor.GetHistoricalMetrics(since)
	} else {
		// Return all historical data
		historical = common.GlobalSystemMonitor.GetAllMetrics()
	}

	current := common.GlobalSystemMonitor.GetCurrentMetrics()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"current":    current,
		"historical": historical,
		"timestamp":  time.Now(),
		"stats":      common.GlobalSystemMonitor.GetStats(),
	})
}

// getSystemStats returns system monitor statistics
func getSystemStats(c echo.Context) error {
	if common.GlobalSystemMonitor == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "System monitor not initialized",
		})
	}

	stats := common.GlobalSystemMonitor.GetStats()
	return c.JSON(http.StatusOK, stats)
}

// getClusterSystemMetrics returns system metrics for all cluster nodes
func getClusterSystemMetrics(c echo.Context) error {
	// Only provide cluster system metrics from leader nodes
	if !common.IsCurrentNodeLeader() {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Cluster system metrics are only available from leader nodes",
		})
	}

	if common.GlobalClusterSystemManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Cluster system manager not initialized",
		})
	}

	nodeID := c.QueryParam("node_id")
	aggregated := c.QueryParam("aggregated") == "true"

	if aggregated {
		// Return aggregated metrics across all nodes
		aggregatedMetrics := common.GlobalClusterSystemManager.GetAggregatedMetrics()
		return c.JSON(http.StatusOK, aggregatedMetrics)
	} else if nodeID != "" {
		// Return metrics for specific node
		metrics := common.GlobalClusterSystemManager.GetNodeMetrics(nodeID)
		if metrics == nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": fmt.Sprintf("No metrics found for node: %s", nodeID),
			})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{
			"node_id":   nodeID,
			"metrics":   metrics,
			"timestamp": time.Now(),
		})
	} else {
		// Return metrics for all nodes
		allMetrics := common.GlobalClusterSystemManager.GetAllMetrics()
		return c.JSON(http.StatusOK, map[string]interface{}{
			"metrics":   allMetrics,
			"timestamp": time.Now(),
			"stats":     common.GlobalClusterSystemManager.GetStats(),
		})
	}
}

// getClusterSystemStats returns cluster system manager statistics
func getClusterSystemStats(c echo.Context) error {
	// Only provide cluster system stats from leader nodes
	if err := common.RequireLeader(); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Cluster system statistics are only available from leader nodes",
		})
	}

	if common.GlobalClusterSystemManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Cluster system manager not initialized",
		})
	}

	stats := common.GlobalClusterSystemManager.GetStats()
	return c.JSON(http.StatusOK, stats)
}

// getClusterHealth returns comprehensive cluster health information
func getClusterHealth(c echo.Context) error {
	if err := common.RequireLeader(); err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Cluster health is only available on leader node",
		})
	}

	health := make(map[string]interface{})

	// Basic cluster info
	status := cluster.GetClusterStatus()
	health["cluster_status"] = status

	// Node health summary
	healthy := 0
	unhealthy := 0
	if nodes, ok := status["nodes"].(map[string]interface{}); ok {
		for _, node := range nodes {
			if nodeInfo, ok := node.(map[string]interface{}); ok {
				if online, ok := nodeInfo["online"].(bool); ok && online {
					healthy++
				} else {
					unhealthy++
				}
			}
		}
	}
	// Add leader as healthy
	healthy++

	health["node_summary"] = map[string]interface{}{
		"total_nodes":     healthy + unhealthy,
		"healthy_nodes":   healthy,
		"unhealthy_nodes": unhealthy,
		"leader_node":     status["node_id"],
	}

	// Redis connectivity
	redisHealthy := false
	if err := common.RedisPing(); err == nil {
		redisHealthy = true
	}
	health["redis_healthy"] = redisHealthy

	// QPS Manager status
	qpsHealthy := false
	if common.GlobalQPSManager != nil {
		qpsStats := common.GlobalQPSManager.GetStats()
		qpsHealthy = true
		health["qps_manager"] = qpsStats
	}
	health["qps_manager_healthy"] = qpsHealthy

	// System Monitor status
	systemHealthy := false
	if common.GlobalSystemMonitor != nil {
		systemStats := common.GlobalSystemMonitor.GetStats()
		systemHealthy = true
		health["system_monitor"] = systemStats
	}
	health["system_monitor_healthy"] = systemHealthy

	// Overall health
	healthyNodeCount := 0
	if nodeSum, ok := health["node_summary"].(map[string]interface{}); ok {
		if hc, ok := nodeSum["healthy_nodes"].(int); ok {
			healthyNodeCount = hc
		}
	}
	overallHealthy := redisHealthy && qpsHealthy && systemHealthy && (healthyNodeCount > 0)
	health["overall_healthy"] = overallHealthy
	health["timestamp"] = time.Now()

	return c.JSON(http.StatusOK, health)
}

// compactInstructions manually triggers instruction compaction (leader only)
func compactInstructions(c echo.Context) error {
	if err := common.RequireLeader(); err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Instruction compaction is only available on leader node",
		})
	}

	if cluster.GlobalInstructionManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Instruction manager not initialized",
		})
	}

	// Get current stats before compaction
	currentVersion := cluster.GlobalInstructionManager.GetCurrentVersion()

	// Perform compaction
	if err := cluster.GlobalInstructionManager.CompactInstructions(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Compaction failed: %v", err),
		})
	}

	// Get new stats after compaction
	newVersion := cluster.GlobalInstructionManager.GetCurrentVersion()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":          "Instruction compaction completed successfully",
		"previous_version": currentVersion,
		"new_version":      newVersion,
		"timestamp":        time.Now(),
	})
}

// getInstructionStats returns instruction statistics
func getInstructionStats(c echo.Context) error {
	if err := common.RequireLeader(); err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Instruction statistics are only available on leader node",
		})
	}

	if cluster.GlobalInstructionManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Instruction manager not initialized",
		})
	}

	currentVersion := cluster.GlobalInstructionManager.GetCurrentVersion()

	// Count existing instructions in Redis
	instructionCount := int64(0)
	if parts := strings.Split(currentVersion, "."); len(parts) == 2 {
		if version, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			instructionCount = version
		}
	}

	// Get active followers
	activeFollowers, err := cluster.GlobalInstructionManager.GetActiveFollowers()
	if err != nil {
		logger.Warn("Failed to get active followers", "error", err)
		activeFollowers = []string{}
	}

	// Calculate if compaction would be triggered
	compactionEnabled := true             // From initialization
	shouldCompact := instructionCount > 0 // Always compact on new instructions

	return c.JSON(http.StatusOK, map[string]interface{}{
		"current_version":     currentVersion,
		"instruction_count":   instructionCount,
		"compaction_enabled":  compactionEnabled,
		"should_compact":      shouldCompact,
		"active_followers":    activeFollowers,
		"followers_executing": len(activeFollowers),
		"can_compact_now":     len(activeFollowers) == 0,
		"compaction_strategy": "every_instruction",
		"timestamp":           time.Now(),
	})
}

// getFollowerExecutionStatus returns the execution status of all followers
func getFollowerExecutionStatus(c echo.Context) error {
	if err := common.RequireLeader(); err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Follower execution status is only available on leader node",
		})
	}

	if cluster.GlobalInstructionManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Instruction manager not initialized",
		})
	}

	// Get active followers
	activeFollowers, err := cluster.GlobalInstructionManager.GetActiveFollowers()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to get active followers: %v", err),
		})
	}

	// Get all known nodes from heartbeat
	clusterStatus := cluster.GetClusterStatus()
	allNodes := []string{}
	if nodes, ok := clusterStatus["nodes"].(map[string]interface{}); ok {
		for nodeID := range nodes {
			allNodes = append(allNodes, nodeID)
		}
	}

	// Build execution status for each node
	nodeStatus := make(map[string]interface{})
	for _, nodeID := range allNodes {
		isExecuting := false
		for _, activeNode := range activeFollowers {
			if activeNode == nodeID {
				isExecuting = true
				break
			}
		}

		nodeStatus[nodeID] = map[string]interface{}{
			"executing": isExecuting,
			"role":      "follower",
		}
	}

	// Add leader status
	if leaderID, ok := clusterStatus["node_id"].(string); ok {
		nodeStatus[leaderID] = map[string]interface{}{
			"executing": false, // Leader doesn't execute instructions
			"role":      "leader",
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"node_status":         nodeStatus,
		"total_nodes":         len(allNodes) + 1, // +1 for leader
		"executing_followers": len(activeFollowers),
		"idle_followers":      len(allNodes) - len(activeFollowers),
		"can_compact":         len(activeFollowers) == 0,
		"timestamp":           time.Now(),
	})
}
