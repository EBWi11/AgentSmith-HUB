package api

import (
	"AgentSmith-HUB/project"
	"net/http"

	"github.com/labstack/echo/v4"
)

func ping(c echo.Context) error {
	return c.String(http.StatusOK, "pong")
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
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"component_type": componentType,
		"component_id":   id,
		"usage":          usage,
		"note":           "Usage shows actual project-specific component instances, not shared component IDs",
	})
}
