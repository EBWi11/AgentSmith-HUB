package api

import (
	"AgentSmith-HUB/agent"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/project"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func ping(c echo.Context) error {
	return c.String(http.StatusOK, "pong")
}

func getFeatures(c echo.Context) error {
	llmAvailable := common.Config != nil && strings.TrimSpace(common.Config.LLMApiKey) != ""
	return c.JSON(http.StatusOK, map[string]interface{}{
		"llm_available": llmAvailable,
	})
}

// GetComponentUsage returns usage information for a component
func GetComponentUsage(c echo.Context) error {
	componentType := c.Param("type")
	id := c.Param("id")

	usage := make([]map[string]interface{}, 0)

	// Enhanced implementation - check usage based on component type and ProjectNodeSequence
	// This ensures we only show projects that are actually using specific component instances
	switch componentType {
	case "rulesets":
		// Check which projects use this ruleset (need to iterate through ProjectNodeSequence keys)
		project.ForEachProject(func(projectID string, p *project.Project) bool {
			for pns, rulesetComponent := range p.Rulesets {
				if rulesetComponent.RulesetID == id {
					usage = append(usage, map[string]interface{}{
						"type":                  "project",
						"id":                    p.Id,
						"name":                  p.Id,
						"status":                p.Status,
						"project_node_sequence": pns, // Use the actual ProjectNodeSequence key
					})
				}
			}
			return true
		})
	case "inputs":
		// Check which projects use this input (need to iterate through ProjectNodeSequence keys)
		project.ForEachProject(func(projectID string, p *project.Project) bool {
			for pns, inputComponent := range p.Inputs {
				if inputComponent.Id == id {
					usage = append(usage, map[string]interface{}{
						"type":                  "project",
						"id":                    p.Id,
						"name":                  p.Id,
						"status":                p.Status,
						"project_node_sequence": pns, // Use the actual ProjectNodeSequence key
					})
				}
			}
			return true
		})
	case "outputs":
		// Check which projects use this output (need to iterate through ProjectNodeSequence keys)
		project.ForEachProject(func(projectID string, p *project.Project) bool {
			for pns, outputComponent := range p.Outputs {
				if outputComponent.Id == id {
					usage = append(usage, map[string]interface{}{
						"type":                  "project",
						"id":                    p.Id,
						"name":                  p.Id,
						"status":                p.Status,
						"project_node_sequence": pns, // Use the actual ProjectNodeSequence key
					})
				}
			}
			return true
		})
	case "agents":
		// Check which projects use this agent (need to iterate through ProjectNodeSequence keys)
		project.ForEachProject(func(projectID string, p *project.Project) bool {
			for pns, agentComponent := range p.GetProjectAgents() {
				if agentComponent != nil && agentComponent.Id == id {
					usage = append(usage, map[string]interface{}{
						"type":                  "project",
						"id":                    p.Id,
						"name":                  p.Id,
						"status":                p.Status,
						"project_node_sequence": pns,
					})
				}
			}
			return true
		})
	case "skills":
		// Skill usage: find agents that reference this skill, then find projects that use those agents.
		// We also include the precise ProjectNodeSequence of the agent node within each project.
		affectedAgentIDs := make(map[string]struct{})
		project.ForEachAgent(func(agentID string, a *agent.Agent) bool {
			if a != nil && a.Config != nil {
				for _, sid := range a.Config.Skills {
					if sid == id {
						affectedAgentIDs[agentID] = struct{}{}
						break
					}
				}
			}
			return true
		})

		if len(affectedAgentIDs) > 0 {
			project.ForEachProject(func(projectID string, p *project.Project) bool {
				for _, node := range p.FlowNodes {
					if node.FromType == "AGENT" {
						if _, ok := affectedAgentIDs[node.FromID]; ok {
							usage = append(usage, map[string]interface{}{
								"type":                  "project",
								"id":                    p.Id,
								"name":                  p.Id,
								"status":                p.Status,
								"project_node_sequence": node.FromPNS,
							})
						}
					}
					if node.ToType == "AGENT" {
						if _, ok := affectedAgentIDs[node.ToID]; ok {
							usage = append(usage, map[string]interface{}{
								"type":                  "project",
								"id":                    p.Id,
								"name":                  p.Id,
								"status":                p.Status,
								"project_node_sequence": node.ToPNS,
							})
						}
					}
				}
				return true
			})
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"component_type": componentType,
		"component_id":   id,
		"usage":          usage,
		"note":           "Usage shows actual project-specific component instances, not shared component IDs",
	})
}
