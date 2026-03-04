package api

import (
	"AgentSmith-HUB/agent"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/project"
	"fmt"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
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
		agentData := map[string]interface{}{
			"id":                   id,
			"hasTemp":              hasTemp,
			"raw":                  rawConfig,
			"status":               string(a.Status),
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
		"status":  string(a.Status),
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
