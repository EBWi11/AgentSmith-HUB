package api

import (
	"AgentSmith-HUB/common"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// GetPluginStats returns success/failure counts for plugins of the given date (default today)
// Optional query params:
// - date (YYYY-MM-DD): filter by date
// - plugin (string): filter by plugin name
// - node_id (string): filter by specific node, "all" for all nodes (default: aggregated across all nodes)
// - by_node (bool): return results grouped by node instead of aggregated
func GetPluginStats(c echo.Context) error {
	date := c.QueryParam("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	filterPlugin := c.QueryParam("plugin")
	nodeID := c.QueryParam("node_id")
	byNode := c.QueryParam("by_node") == "true"

	// Use daily stats system to get plugin statistics
	// Pattern: hub:daily_stats:{date}#{nodeID}#{projectID}#{projectNodeSequence}
	// For plugins: projectNodeSequence is "PLUGIN.{pluginName}.success" or "PLUGIN.{pluginName}.failure"
	if common.GlobalDailyStatsManager == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Daily stats manager not initialized"})
	}

	// Get all daily stats data for the specified date
	allData := common.GlobalDailyStatsManager.GetDailyStats(date, "", nodeID)

	type stat struct {
		Success uint64 `json:"success"`
		Failure uint64 `json:"failure"`
	}

	if byNode {
		// Return results grouped by node
		nodeStats := make(map[string]map[string]*stat)

		for _, data := range allData {
			// Only process plugin statistics
			if data.ComponentType != "plugin_success" && data.ComponentType != "plugin_failure" {
				continue
			}

			// Extract plugin name from ProjectNodeSequence
			// Format: "PLUGIN.{pluginName}.success" or "PLUGIN.{pluginName}.failure"
			parts := strings.Split(data.ProjectNodeSequence, ".")
			if len(parts) != 3 || strings.ToUpper(parts[0]) != "PLUGIN" {
				continue
			}
			plugin := parts[1]
			status := strings.ToLower(parts[2])

			// Apply node filter
			if nodeID != "" && nodeID != "all" && data.NodeID != nodeID {
				continue
			}

			// Apply plugin filter
			if filterPlugin != "" && plugin != filterPlugin {
				continue
			}

			// Initialize node map if not exists
			if _, exists := nodeStats[data.NodeID]; !exists {
				nodeStats[data.NodeID] = make(map[string]*stat)
			}

			// Initialize plugin stat if not exists
			if _, exists := nodeStats[data.NodeID][plugin]; !exists {
				nodeStats[data.NodeID][plugin] = &stat{}
			}

			// Add to counter
			if status == "success" {
				nodeStats[data.NodeID][plugin].Success += data.TotalMessages
			} else if status == "failure" {
				nodeStats[data.NodeID][plugin].Failure += data.TotalMessages
			}
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"date":       date,
			"by_node":    true,
			"node_stats": nodeStats,
		})
	} else {
		// Return aggregated results across all nodes (default behavior)
		stats := make(map[string]*stat)

		for _, data := range allData {
			// Only process plugin statistics
			if data.ComponentType != "plugin_success" && data.ComponentType != "plugin_failure" {
				continue
			}

			// Extract plugin name from ProjectNodeSequence
			// Format: "PLUGIN.{pluginName}.success" or "PLUGIN.{pluginName}.failure"
			parts := strings.Split(data.ProjectNodeSequence, ".")
			if len(parts) != 3 || strings.ToUpper(parts[0]) != "PLUGIN" {
				continue
			}
			plugin := parts[1]
			status := strings.ToLower(parts[2])

			// Apply node filter
			if nodeID != "" && nodeID != "all" && data.NodeID != nodeID {
				continue
			}

			// Apply plugin filter
			if filterPlugin != "" && plugin != filterPlugin {
				continue
			}

			// Initialize plugin stat if not exists
			s := stats[plugin]
			if s == nil {
				s = &stat{}
				stats[plugin] = s
			}

			// Aggregate across all nodes
			if status == "success" {
				s.Success += data.TotalMessages
			} else if status == "failure" {
				s.Failure += data.TotalMessages
			}
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"date":  date,
			"stats": stats,
		})
	}
}
