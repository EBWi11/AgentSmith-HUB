package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// ErrorLogEntry represents a single error log entry
type ErrorLogEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Level       string    `json:"level"`
	Message     string    `json:"message"`
	Source      string    `json:"source"`       // "hub" or "plugin"
	NodeID      string    `json:"node_id"`      // cluster node identifier
	NodeAddress string    `json:"node_address"` // cluster node address
	Context     string    `json:"context"`      // additional context from log
	Error       string    `json:"error"`        // error details
	Line        int       `json:"line"`         // line number in log file
}

// ErrorLogFilter represents filter parameters for error logs
type ErrorLogFilter struct {
	Source      string    `json:"source"`     // "hub", "plugin", "agent", or "all"
	LevelFilter string    `json:"level"`      // "error" = only ERROR/FATAL (default); "all" = all levels
	NodeID      string    `json:"node_id"`    // specific node or "all"
	StartTime   time.Time `json:"start_time"` // start time filter
	EndTime     time.Time `json:"end_time"`   // end time filter
	Keyword     string    `json:"keyword"`    // keyword search
	Limit       int       `json:"limit"`      // limit number of results
	Offset      int       `json:"offset"`     // pagination offset
}

// ErrorLogResponse represents the response for error log queries
type ErrorLogResponse struct {
	Logs       []ErrorLogEntry `json:"logs"`
	TotalCount int             `json:"total_count"`
	HasMore    bool            `json:"has_more"`
}

// ClusterErrorLogResponse represents aggregated error logs from cluster
type ClusterErrorLogResponse struct {
	Logs       []ErrorLogEntry     `json:"logs"`
	NodeStats  map[string]NodeStat `json:"node_stats"`
	TotalCount int                 `json:"total_count"`
}

// AgentLogAPIEntry represents one agent log entry for the frontend.
type AgentLogAPIEntry struct {
	Timestamp           time.Time                `json:"timestamp"`
	NodeID              string                   `json:"node_id"`
	AgentID             string                   `json:"agent_id"`
	ID                  string                   `json:"id"`
	ProjectID           string                   `json:"project_id,omitempty"`
	ProjectNodeSequence string                   `json:"project_node_sequence,omitempty"`
	IsTest              bool                     `json:"is_test,omitempty"`
	RawInput            string                   `json:"raw_input,omitempty"`
	RawOutput           string                   `json:"raw_output,omitempty"`
	Trace               string                   `json:"trace,omitempty"`
	Error               string                   `json:"error,omitempty"`
	Comments            []common.AgentLogComment `json:"comments,omitempty"`
}

// AgentLogAPIResponse is the response payload for /agent-logs.
type AgentLogAPIResponse struct {
	Logs       []AgentLogAPIEntry `json:"logs"`
	TotalCount int                `json:"total_count"`
}

const commonAgentLogMaxFetch = 5000

// AgentLogCommentRequest is the payload for creating a new comment.
type AgentLogCommentRequest struct {
	Comment string `json:"comment"`
	Tag     string `json:"tag,omitempty"`
}

// NodeStat represents error statistics for a node
type NodeStat struct {
	NodeID       string `json:"node_id"`
	HubErrors    int    `json:"hub_errors"`
	PluginErrors int    `json:"plugin_errors"`
	TotalErrors  int    `json:"total_errors"`
}

// getUnifiedErrorLogs gets error logs from Redis for all nodes (leader only)
func getUnifiedErrorLogs(filter ErrorLogFilter) ([]ErrorLogEntry, int, error) {
	// Use the new common package function with server-side filtering
	logs, totalCount, err := common.GetErrorLogsFromRedisWithFilter(
		filter.NodeID,
		filter.Source,
		filter.LevelFilter,
		filter.StartTime,
		filter.EndTime,
		filter.Keyword,
		filter.Limit,
		filter.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get error logs from Redis: %w", err)
	}

	// Convert common.ErrorLogEntry to api.ErrorLogEntry
	var apiLogs []ErrorLogEntry
	for _, log := range logs {
		apiLog := ErrorLogEntry{
			Timestamp:   log.Timestamp,
			Level:       log.Level,
			Message:     log.Message,
			Source:      log.Source,
			NodeID:      log.NodeID,
			NodeAddress: log.NodeID, // Use NodeID as address for now
			Error:       log.Error,  // Include error details
			Line:        log.Line,
		}

		// Build context JSON from details; include error message so Raw Context shows it
		contextObj := make(map[string]interface{})
		for k, v := range log.Details {
			contextObj[k] = v
		}
		if log.Error != "" {
			contextObj["error"] = log.Error
		}
		if len(contextObj) > 0 {
			if contextBytes, err := json.Marshal(contextObj); err == nil {
				apiLog.Context = string(contextBytes)
			}
		}

		apiLogs = append(apiLogs, apiLog)
	}

	return apiLogs, totalCount, nil
}

// getErrorLogs handles GET /error-logs - unified endpoint for all nodes
func getErrorLogs(c echo.Context) error {
	var filter ErrorLogFilter

	// Parse query parameters
	filter.Source = c.QueryParam("source")
	filter.NodeID = c.QueryParam("node_id")
	filter.Keyword = c.QueryParam("keyword")
	// level: "error" = only ERROR/FATAL (default for Error Logs); "all" = all levels (for Agent Tools Logs)
	filter.LevelFilter = c.QueryParam("level")
	if filter.LevelFilter == "" {
		filter.LevelFilter = "error"
	}

	// Parse time filters
	if startTime := c.QueryParam("start_time"); startTime != "" {
		if parsed, err := time.Parse(time.RFC3339, startTime); err == nil {
			filter.StartTime = parsed
		}
	}
	if endTime := c.QueryParam("end_time"); endTime != "" {
		if parsed, err := time.Parse(time.RFC3339, endTime); err == nil {
			filter.EndTime = parsed
		}
	}

	// Default to last 1 hour if no time filters provided
	if filter.StartTime.IsZero() && filter.EndTime.IsZero() {
		end := time.Now()
		start := end.Add(-1 * time.Hour)
		filter.StartTime = start
		filter.EndTime = end
	}

	// Parse pagination
	if limit := c.QueryParam("limit"); limit != "" {
		if parsed, err := strconv.Atoi(limit); err == nil && parsed > 0 {
			filter.Limit = parsed
		} else {
			filter.Limit = 100 // Default limit
		}
	} else {
		filter.Limit = 100
	}

	if offset := c.QueryParam("offset"); offset != "" {
		if parsed, err := strconv.Atoi(offset); err == nil && parsed >= 0 {
			filter.Offset = parsed
		}
	}

	// All nodes can access unified logs from Redis
	logs, totalCount, err := getUnifiedErrorLogs(filter)
	if err != nil {
		logger.Error("Failed to get unified error logs", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to read error logs: " + err.Error(),
		})
	}

	response := ErrorLogResponse{
		Logs:       logs,
		TotalCount: totalCount,
		HasMore:    filter.Offset+filter.Limit < totalCount,
	}

	return c.JSON(http.StatusOK, response)
}

// getAgentLogs handles GET /agent-logs - returns recent agent logs stored in Redis.
// Supports optional filtering by node_id, agent, and project.
func getAgentLogs(c echo.Context) error {
	agentID := c.QueryParam("agent")
	project := c.QueryParam("project")
	nodeID := c.QueryParam("node_id")

	// Pagination params (simple, list-per-agent; frontend can aggregate if needed)
	limit := 100
	offset := 0
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := c.QueryParam("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	var rawEntries []string
	if agentID != "" {
		key := fmt.Sprintf("%s%s", "hub:agent_logs:", agentID)
		entries, err := common.RedisLRange(key, 0, int64(commonAgentLogMaxFetch-1))
		if err != nil {
			logger.Error("Failed to read agent logs from Redis", "error", err, "agent", agentID)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to read agent logs: " + err.Error(),
			})
		}
		rawEntries = entries
	} else {
		// aggregate from all agents when filter is not specified
		keys, err := common.RedisKeys("hub:agent_logs:*")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to list agent log keys: " + err.Error(),
			})
		}
		for _, key := range keys {
			entries, err := common.RedisLRange(key, 0, int64(commonAgentLogMaxFetch-1))
			if err != nil {
				continue
			}
			rawEntries = append(rawEntries, entries...)
		}
	}

	var logs []AgentLogAPIEntry
	for _, raw := range rawEntries {
		var entry common.AgentLogEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			continue
		}
		// node_id filter
		if nodeID != "" && nodeID != "all" && entry.NodeID != nodeID {
			continue
		}
		// project filter: match by prefix or exact PNS string
		if project != "" && entry.ProjectNodeSequence != "" {
			if !strings.Contains(entry.ProjectNodeSequence, project) {
				continue
			}
		}

		comments, _ := common.GetAgentLogComments(entry.ID, 50)

		logs = append(logs, AgentLogAPIEntry{
			Timestamp:           entry.Timestamp,
			NodeID:              entry.NodeID,
			AgentID:             entry.AgentID,
			ID:                  entry.ID,
			ProjectID:           entry.ProjectID,
			ProjectNodeSequence: entry.ProjectNodeSequence,
			IsTest:              entry.IsTest,
			RawInput:            entry.RawInput,
			RawOutput:           entry.RawOutput,
			Trace:               entry.Trace,
			Error:               entry.Error,
			Comments:            comments,
		})
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Timestamp.After(logs[j].Timestamp)
	})
	total := len(logs)
	if offset >= total {
		logs = []AgentLogAPIEntry{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		logs = logs[offset:end]
	}

	resp := AgentLogAPIResponse{
		Logs:       logs,
		TotalCount: total,
	}
	return c.JSON(http.StatusOK, resp)
}

// postAgentLogComment handles POST /agent-logs/:agentId/:logId/comments.
func postAgentLogComment(c echo.Context) error {
	agentID := c.Param("agentId")
	logID := c.Param("logId")
	if logID == "" || agentID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "agentId and logId are required",
		})
	}

	var req AgentLogCommentRequest
	if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Comment) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "comment is required",
		})
	}

	found, err := common.FindAgentLogByID(agentID, logID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to validate log ownership: " + err.Error(),
		})
	}
	if found == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "log_id does not belong to this agent",
		})
	}

	locked, err := common.AgentLogMemoryCommitted(logID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to check log comment lock: " + err.Error(),
		})
	}
	if locked {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "memory has already been applied for this log; further comments are not allowed",
		})
	}

	author := c.Request().Header.Get("X-User")
	if author == "" {
		author = "anonymous"
	}

	entry := common.AgentLogComment{
		Type:      "user_comment",
		Author:    author,
		Comment:   strings.TrimSpace(req.Comment),
		Tag:       strings.TrimSpace(req.Tag),
		CreatedAt: time.Now(),
	}

	if err := common.AppendAgentLogComment(logID, entry); err != nil {
		logger.Error("Failed to append agent log comment", "error", err, "log_id", logID)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to write comment: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// getErrorLogNodes handles GET /error-logs/nodes - returns all nodes that have error logs
func getErrorLogNodes(c echo.Context) error {
	// Get all known nodes from Redis (tracked by leader heartbeat)
	nodes, err := common.GetKnownNodes()
	if err != nil {
		logger.Error("Failed to get known nodes", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve known nodes: " + err.Error(),
		})
	}

	response := map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	}

	return c.JSON(http.StatusOK, response)
}

// getClusterErrorLogs - DEPRECATED: Use getErrorLogs instead
// This endpoint is kept for backward compatibility but redirects to the unified endpoint
func getClusterErrorLogs(c echo.Context) error {
	logger.Info("getClusterErrorLogs called - redirecting to unified getErrorLogs endpoint")
	return getErrorLogs(c)
}
