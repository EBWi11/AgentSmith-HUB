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
temperature: 0.3
max_tokens: 4096

system_prompt: |
  You are a security analyst. For each batch of events:
  1. Classify the threat level (critical/high/medium/low/info)
  2. Add a "threat_analysis" field with your reasoning
  3. Filter out benign events
  Output a JSON array of enriched events.

# Reference skill component IDs here
skills: []

tools: all

batch:
  size: 10
  timeout: 30s
  max_rounds: 5

distributed:
  mode: independent
  rate_limit_rps: 0
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

		agentData := map[string]interface{}{
			"id":            id,
			"hasTemp":       hasTemp,
			"raw":           rawConfig,
			"status":        string(a.Status),
			"model":         a.Config.Model,
			"process_total": a.GetProcessTotal(),
			"skills":        a.Config.Skills,
			"distributed":   a.Config.Distributed,
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
