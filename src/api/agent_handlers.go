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
	"gopkg.in/yaml.v3"
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
		localStatus := project.GetAggregatedAgentStatus(id)
		clusterStatus := project.GetClusterAggregatedAgentStatus(id)
		agentData := map[string]interface{}{
			"id":                   id,
			"hasTemp":              hasTemp,
			"raw":                  rawConfig,
			"status":               string(clusterStatus),
			"local_status":         string(localStatus),
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
		"id":           id,
		"raw":          rawConfig,
		"hasTemp":      false,
		"status":       string(project.GetClusterAggregatedAgentStatus(id)),
		"local_status": string(project.GetAggregatedAgentStatus(id)),
		"model":        a.Config.Model,
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

// updateAgentMemoryNotes updates agent memory_notes on the leader.
// JSON body:
//   - notes (required): full memory body when mode=replace, or fragment to append when mode=append
//   - mode (optional): "replace" (default, same semantics as generate-from-log) or "append" (legacy)
//   - log_id, source_comment_ids (optional): attach result to agent log comments
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
		Mode             string   `json:"mode"`
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

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "replace"
	}
	if mode != "replace" && mode != "append" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": `mode must be "replace" or "append"`,
		})
	}

	unlock := acquireAgentMemoryWriteLock(id)
	defer unlock()

	raw, err := loadAgentYAMLForMemoryUpdates(id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
	}

	var cfg agent.AgentConfig
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "failed to parse agent config: " + err.Error(),
		})
	}

	notes := strings.TrimSpace(req.Notes)
	switch mode {
	case "replace":
		cfg.MemoryNotes = agent.MemoryNotesFromSummaryText(notes)
	case "append":
		extra := agent.MemoryNotesFromSummaryText(notes)
		cfg.MemoryNotes = append(cfg.MemoryNotes, extra...)
	}

	updated, err := yaml.Marshal(&cfg)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to marshal updated agent config: " + err.Error(),
		})
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
	affectedProjects, err := reloadComponentUnified(reloadReq)
	if err != nil {
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
	refreshAffectedProjectsForComponentChange("agent", id, affectedProjects, "memory_notes", true)

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
		"message": "memory_notes updated and auto-committed",
	})
}

// generateAgentMemoryFromLog merges user comments with existing memory_notes (full replace:
// review, compress, optimize; comments win on conflict), optionally using this log's
// input/output/trace as background, then auto-commits to the target agent.
func generateAgentMemoryFromLog(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "agent id is required"})
	}
	if !common.IsCurrentNodeLeader() {
		return c.JSON(http.StatusConflict, map[string]string{"error": "memory generation must be executed on leader node"})
	}

	var req struct {
		LogID   string `json:"log_id"`
		Comment string `json:"comment"`
		Tag     string `json:"tag"`
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

	locked, err := common.AgentLogMemoryCommitted(logID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to check log lock: " + err.Error()})
	}
	if locked {
		return c.JSON(http.StatusConflict, map[string]string{"error": "memory has already been applied for this log"})
	}

	if strings.TrimSpace(req.Comment) != "" {
		author := c.Request().Header.Get("X-User")
		if author == "" {
			author = "anonymous"
		}
		if err := common.AppendAgentLogComment(logID, common.AgentLogComment{
			Type:      "user_comment",
			Author:    author,
			Comment:   strings.TrimSpace(req.Comment),
			Tag:       strings.TrimSpace(req.Tag),
			CreatedAt: time.Now(),
		}); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to append comment: " + err.Error()})
		}
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

	// Phase 1 — snapshot memory under lock (do not hold lock during LLM).
	unlockSnap := acquireAgentMemoryWriteLock(id)
	rawSnap, err := loadAgentYAMLForMemoryUpdates(id)
	if err != nil {
		unlockSnap()
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	var cfgSnap agent.AgentConfig
	if err := yaml.Unmarshal([]byte(rawSnap), &cfgSnap); err != nil {
		unlockSnap()
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to parse agent config: " + err.Error()})
	}
	memSnap := cfgSnap.MemoryNotes
	sysSnap := cfgSnap.SystemPrompt
	unlockSnap()

	runCtx := agent.BuildMemoryRunContext(found)
	summary, err := agent.GenerateMemorySummary("", id, sysSnap, memSnap.String(), userComments, runCtx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate memory summary: " + err.Error()})
	}

	// Phase 2 — commit if memory_notes unchanged since snapshot; otherwise 409 (caller may retry).
	unlockCommit := acquireAgentMemoryWriteLock(id)
	defer unlockCommit()

	rawNow, err := loadAgentYAMLForMemoryUpdates(id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	var cfgNow agent.AgentConfig
	if err := yaml.Unmarshal([]byte(rawNow), &cfgNow); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to parse agent config: " + err.Error()})
	}
	if cfgNow.MemoryNotes.String() != memSnap.String() {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "agent memory_notes was updated concurrently; refresh and retry",
		})
	}

	cfgNow.MemoryNotes = agent.MemoryNotesFromSummaryText(summary)

	updated, err := yaml.Marshal(&cfgNow)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to marshal updated agent config: " + err.Error()})
	}
	updatedStr := string(updated)
	project.SetAgentNew(id, updatedStr)
	reloadReq := &ComponentReloadRequest{
		Type:        "agent",
		ID:          id,
		NewContent:  updatedStr,
		OldContent:  rawNow,
		Source:      SourceChangePush,
		SkipVerify:  false,
		WriteToFile: true,
	}
	affectedProjects, err := reloadComponentUnified(reloadReq)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to commit generated memory: " + err.Error()})
	}
	refreshAffectedProjectsForComponentChange("agent", id, affectedProjects, "memory_notes", true)

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
