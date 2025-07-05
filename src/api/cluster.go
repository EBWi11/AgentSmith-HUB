package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"
)

func getClusterStatus(c echo.Context) error {
	cm := cluster.ClusterInstance
	if cm == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "cluster manager not initialized",
		})
	}

	return c.JSON(http.StatusOK, cm.GetClusterStatus())
}

func getClusterProjectStates(c echo.Context) error {
	cm := cluster.ClusterInstance
	if cm == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "cluster manager not initialized",
		})
	}

	// Allow all nodes to provide cluster project states for read-only access
	// Non-leader nodes will return what they know about the cluster state

	cm.Mu.RLock()
	nodeProjectStates := make(map[string][]cluster.ProjectStatus)
	for nodeID, projectStates := range cm.NodeProjectStates {
		nodeProjectStates[nodeID] = projectStates
	}
	clusterStatus := cm.GetClusterStatus()
	cm.Mu.RUnlock()

	// Combine cluster status and project states
	response := map[string]interface{}{
		"cluster_status": clusterStatus,
		"project_states": nodeProjectStates,
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
	if cluster.IsLeader {
		return c.JSON(http.StatusOK, map[string]string{
			"redis":          common.Config.Redis,
			"redis_password": common.Config.RedisPassword,
		})
	} else {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "no leader",
		})
	}
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
	data, _ := json.Marshal(cluster.ClusterInstance)
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
	if !cluster.IsLeader {
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
	if !cluster.IsLeader {
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
