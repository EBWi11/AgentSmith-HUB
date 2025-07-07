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
// Optional query params: date (YYYY-MM-DD), plugin (filter by plugin name)
func GetPluginStats(c echo.Context) error {
	date := c.QueryParam("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	filterPlugin := c.QueryParam("plugin")

	// pattern: plugin_stats:date:*:*
	pattern := "plugin_stats:" + date + ":*:*"
	keys, err := common.RedisKeys(pattern)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	type stat struct {
		Success uint64
		Failure uint64
	}
	stats := make(map[string]*stat)
	for _, k := range keys {
		parts := strings.Split(k, ":")
		if len(parts) != 4 {
			continue
		}
		plugin := parts[2]
		status := parts[3]
		if filterPlugin != "" && plugin != filterPlugin {
			continue
		}
		cntStr, _ := common.RedisGet(k)
		cnt, _ := strconv.ParseUint(cntStr, 10, 64)
		s := stats[plugin]
		if s == nil {
			s = &stat{}
			stats[plugin] = s
		}
		if status == "success" {
			s.Success += cnt
		} else {
			s.Failure += cnt
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"date":  date,
		"stats": stats,
	})
}
