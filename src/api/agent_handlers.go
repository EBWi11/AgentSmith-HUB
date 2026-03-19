package api

import (
	"AgentSmith-HUB/agent"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/project"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v2"
)

const NewAgentData = `model: gpt-4o-mini
temperature: 0.1
max_tokens: 256

system_prompt: |
  You are a security analyst. For each event, output ONLY a JSON object with your analysis fields.
  The output fields will be merged into the original event automatically.
  Do NOT repeat the original event fields in your output.

  Example output:
  {"threat_level": "high", "threat_analysis": "Suspicious outbound connection to known C2 IP"}

# Reference skill component IDs here
skills: []

tools: []

max_rounds: 1
timeout: 30s
`

func getAgents(c echo.Context) error {
	agents := make([]map[string]interface{}, 0)
	processedIDs := make(map[string]bool)

	project.ForEachAgent(func(id string, a *agent.Agent) bool {
		tempRaw, hasTemp := project.GetAgentNew(id)

		rawConfig := a.RawConfig
		if hasTemp {
			rawConfig = tempRaw
		}

		dailyCalls, dailyAvgMs := project.GetAggregatedAgentDailyStats(id)
		aggregatedStatus := project.GetAggregatedAgentStatus(id)
		agentData := map[string]interface{}{
			"id":                   id,
			"hasTemp":              hasTemp,
			"raw":                  rawConfig,
			"status":               string(aggregatedStatus),
			"model":                a.Config.Model,
			"process_total":        a.GetProcessTotal(),
			"avg_latency_ms":       a.GetAvgLatencyMs(),
			"daily_call_count":     dailyCalls,
			"daily_avg_latency_ms": dailyAvgMs,
			"skills":               a.Config.Skills,
		}

		if a.Status == common.StatusError && a.Err != nil {
			agentData["errorMessage"] = a.Err.Error()
		}
		if a.Path != "" {
			agentData["path"] = a.Path
		}

		agents = append(agents, agentData)
		processedIDs[id] = true
		return true
	})

	allAgentsNew := project.GetAllAgentsNew()
	for id, tempRaw := range allAgentsNew {
		if !processedIDs[id] {
			agentData := map[string]interface{}{
				"id":      id,
				"hasTemp": true,
				"raw":     tempRaw,
			}
			agents = append(agents, agentData)
		}
	}

	return c.JSON(http.StatusOK, agents)
}

func getAgentDetail(c echo.Context) error {
	id := c.Param("id")

	if raw, ok := project.GetAgentNew(id); ok {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":      id,
			"raw":     raw,
			"hasTemp": true,
		})
	}

	a, exists := project.GetAgent(id)
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "agent not found"})
	}

	rawConfig, _ := common.GetRawConfig("agent", id)
	if rawConfig == "" {
		rawConfig = a.RawConfig
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":      id,
		"raw":     rawConfig,
		"hasTemp": false,
		"status":  string(project.GetAggregatedAgentStatus(id)),
		"model":   a.Config.Model,
	})
}

func createAgent(c echo.Context) error {
	return createComponent("agent", c)
}

func updateAgent(c echo.Context) error {
	return updateComponent("agent", c)
}

func deleteAgentHandler(c echo.Context) error {
	return deleteComponent("agent", c)
}

func cancelAgentUpgrade(c echo.Context) error {
	id := c.Param("id")
	project.DeleteAgentNew(id)

	tempPath, tempExists := GetComponentPath("agent", id, true)
	if tempExists {
		_ = os.Remove(tempPath)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Agent upgrade cancelled for %s", id),
	})
}

// updateAgentMemoryNotes appends summarized memory notes to an agent config.
// It expects a JSON body with:
// { "notes": "...", "log_id": "...", "source_comment_ids": ["..."] }.
func updateAgentMemoryNotes(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "agent id is required",
		})
	}

	var req struct {
		Notes            string   `json:"notes"`
		LogID            string   `json:"log_id"`
		SourceCommentIDs []string `json:"source_comment_ids"`
	}
	if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Notes) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "notes is required",
		})
	}

	if !common.IsCurrentNodeLeader() {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "memory update must be executed on leader node",
		})
	}

	if strings.TrimSpace(req.LogID) != "" {
		found, err := common.FindAgentLogByID(id, strings.TrimSpace(req.LogID))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to validate log ownership: " + err.Error(),
			})
		}
		if found == nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "log_id does not belong to this agent",
			})
		}
	}

	// Resolve agent raw config (prefer temp if exists).
	var raw string
	tempPath, tempExists := GetComponentPath("agent", id, true)
	if tempExists {
		if content, err := ReadComponent(tempPath); err == nil {
			raw = content
		}
	}
	if raw == "" {
		if v, ok := project.GetAgentNew(id); ok {
			raw = v
		} else if a, exists := project.GetAgent(id); exists {
			raw = a.RawConfig
		}
	}
	if raw == "" {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "agent not found: " + id,
		})
	}

	var cfg agent.AgentConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "failed to parse agent config: " + err.Error(),
		})
	}

	notes := strings.TrimSpace(req.Notes)
	if cfg.MemoryNotes != "" {
		cfg.MemoryNotes = strings.TrimSpace(cfg.MemoryNotes) + "\n" + notes
	} else {
		cfg.MemoryNotes = notes
	}

	updated, err := yaml.Marshal(&cfg)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to marshal updated agent config: " + err.Error(),
		})
	}

	updatedStr := string(updated)

	// Auto-commit: write pending content to memory and apply immediately,
	// so the change is persisted to main config and runtime is refreshed.
	project.SetAgentNew(id, updatedStr)
	reloadReq := &ComponentReloadRequest{
		Type:        "agent",
		ID:          id,
		NewContent:  updatedStr,
		OldContent:  raw,
		Source:      SourceChangePush,
		SkipVerify:  false,
		WriteToFile: true,
	}
	if _, err := reloadComponentUnified(reloadReq); err != nil {
		if strings.TrimSpace(req.LogID) != "" {
			_ = common.AppendAgentLogComment(strings.TrimSpace(req.LogID), common.AgentLogComment{
				Type:             "memory_summary",
				Author:           "system",
				Comment:          notes,
				Tag:              "memory",
				Status:           "failed",
				LinkedCommentIDs: req.SourceCommentIDs,
				CreatedAt:        time.Now(),
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to auto-commit memory notes: " + err.Error(),
		})
	}

	// If log_id is provided, append a generated memory summary to the same Agent Log comments.
	if strings.TrimSpace(req.LogID) != "" {
		_ = common.AppendAgentLogComment(strings.TrimSpace(req.LogID), common.AgentLogComment{
			Type:             "memory_summary",
			Author:           "system",
			Comment:          notes,
			Tag:              "memory",
			Status:           "committed",
			LinkedCommentIDs: req.SourceCommentIDs,
			CreatedAt:        time.Now(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"message": "memory_notes appended and auto-committed",
	})
}

// generateAgentMemoryFromLog builds memory notes from user comments on one log,
// then auto-commits to the target agent.
func generateAgentMemoryFromLog(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "agent id is required"})
	}
	if !common.IsCurrentNodeLeader() {
		return c.JSON(http.StatusConflict, map[string]string{"error": "memory generation must be executed on leader node"})
	}

	var req struct {
		LogID string `json:"log_id"`
	}
	if err := c.Bind(&req); err != nil || strings.TrimSpace(req.LogID) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "log_id is required"})
	}
	logID := strings.TrimSpace(req.LogID)

	found, err := common.FindAgentLogByID(id, logID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to validate log ownership: " + err.Error()})
	}
	if found == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "log_id does not belong to this agent"})
	}

	comments, err := common.GetAgentLogComments(logID, 100)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load comments: " + err.Error()})
	}
	userComments := make([]common.AgentLogComment, 0, len(comments))
	sourceIDs := make([]string, 0, len(comments))
	for i, cm := range comments {
		if cm.Type == "" || cm.Type == "user_comment" {
			userComments = append(userComments, cm)
			sourceIDs = append(sourceIDs, fmt.Sprintf("%d", i))
		}
	}
	if len(userComments) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no user comments found for this log"})
	}

	// Load current config to include existing memory in summarization context.
	var raw string
	if v, ok := project.GetAgentNew(id); ok {
		raw = v
	} else if a, exists := project.GetAgent(id); exists {
		raw = a.RawConfig
	}
	if raw == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "agent not found: " + id})
	}

	var cfg agent.AgentConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to parse agent config: " + err.Error()})
	}

	summary, err := agent.GenerateMemorySummary("", id, cfg.MemoryNotes, userComments)
	if err != nil {
		_ = common.AppendAgentLogComment(logID, common.AgentLogComment{
			Type:      "memory_summary",
			Author:    "system",
			Comment:   "Memory generation failed: " + err.Error(),
			Tag:       "memory",
			Status:    "failed",
			CreatedAt: time.Now(),
		})
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate memory summary: " + err.Error()})
	}

	// Reuse update endpoint logic by appending and committing directly here.
	cfg.MemoryNotes = strings.TrimSpace(cfg.MemoryNotes)
	if cfg.MemoryNotes != "" {
		cfg.MemoryNotes += "\n" + strings.TrimSpace(summary)
	} else {
		cfg.MemoryNotes = strings.TrimSpace(summary)
	}
	updated, err := yaml.Marshal(&cfg)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to marshal updated agent config: " + err.Error()})
	}
	updatedStr := string(updated)
	project.SetAgentNew(id, updatedStr)
	reloadReq := &ComponentReloadRequest{
		Type:        "agent",
		ID:          id,
		NewContent:  updatedStr,
		OldContent:  raw,
		Source:      SourceChangePush,
		SkipVerify:  false,
		WriteToFile: true,
	}
	if _, err := reloadComponentUnified(reloadReq); err != nil {
		_ = common.AppendAgentLogComment(logID, common.AgentLogComment{
			Type:      "memory_summary",
			Author:    "system",
			Comment:   summary,
			Tag:       "memory",
			Status:    "failed",
			CreatedAt: time.Now(),
		})
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to commit generated memory: " + err.Error()})
	}

	if err := common.AppendAgentLogComment(logID, common.AgentLogComment{
		Type:             "memory_summary",
		Author:           "system",
		Comment:          summary,
		Tag:              "memory",
		Status:           "committed",
		LinkedCommentIDs: sourceIDs,
		CreatedAt:        time.Now(),
	}); err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status":  "ok",
			"summary": summary,
			"warning": "memory committed but failed to append summary comment",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"summary": summary,
	})
}
