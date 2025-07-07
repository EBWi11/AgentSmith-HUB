package api

import (
	"AgentSmith-HUB/common"
	"net/http"
	"strconv"
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

	// Updated pattern: plugin_stats:date:nodeID:pluginName:status
	pattern := "plugin_stats:" + date + ":*:*:*"
	keys, err := common.RedisKeys(pattern)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	type stat struct {
		Success uint64
		Failure uint64
	}

	if byNode {
		// Return results grouped by node
		nodeStats := make(map[string]map[string]*stat)

		for _, k := range keys {
			parts := strings.Split(k, ":")
			if len(parts) != 5 {
				continue // Skip invalid keys
			}
			keyNodeID := parts[2]
			plugin := parts[3]
			status := parts[4]

			// Apply node filter
			if nodeID != "" && nodeID != "all" && keyNodeID != nodeID {
				continue
			}

			// Apply plugin filter
			if filterPlugin != "" && plugin != filterPlugin {
				continue
			}

			cntStr, _ := common.RedisGet(k)
			cnt, _ := strconv.ParseUint(cntStr, 10, 64)

			// Initialize node map if not exists
			if _, exists := nodeStats[keyNodeID]; !exists {
				nodeStats[keyNodeID] = make(map[string]*stat)
			}

			// Initialize plugin stat if not exists
			if _, exists := nodeStats[keyNodeID][plugin]; !exists {
				nodeStats[keyNodeID][plugin] = &stat{}
			}

			// Add to counter
			if status == "success" {
				nodeStats[keyNodeID][plugin].Success += cnt
			} else if status == "failure" {
				nodeStats[keyNodeID][plugin].Failure += cnt
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

		for _, k := range keys {
			parts := strings.Split(k, ":")
			if len(parts) != 5 {
				continue // Skip invalid keys
			}
			keyNodeID := parts[2]
			plugin := parts[3]
			status := parts[4]

			// Apply node filter
			if nodeID != "" && nodeID != "all" && keyNodeID != nodeID {
				continue
			}

			// Apply plugin filter
			if filterPlugin != "" && plugin != filterPlugin {
				continue
			}

			cntStr, _ := common.RedisGet(k)
			cnt, _ := strconv.ParseUint(cntStr, 10, 64)

			// Initialize plugin stat if not exists
			s := stats[plugin]
			if s == nil {
				s = &stat{}
				stats[plugin] = s
			}

			// Aggregate across all nodes
			if status == "success" {
				s.Success += cnt
			} else if status == "failure" {
				s.Failure += cnt
			}
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"date":  date,
			"stats": stats,
		})
	}
}
