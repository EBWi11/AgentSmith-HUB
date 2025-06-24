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
		// Check which projects use this ruleset (consider ProjectNodeSequence for independent instances)
		for _, p := range project.GlobalProject.Projects {
			if ruleset, exists := p.Rulesets[id]; exists {
				usage = append(usage, map[string]interface{}{
					"type":                  "project",
					"id":                    p.Id,
					"name":                  p.Id,
					"status":                p.Status,
					"project_node_sequence": ruleset.ProjectNodeSequence, // Include sequence for clarity
				})
			}
		}
	case "inputs":
		// Check which projects use this input (inputs are typically shared, so check by ID)
		for _, p := range project.GlobalProject.Projects {
			if input, exists := p.Inputs[id]; exists {
				usage = append(usage, map[string]interface{}{
					"type":                  "project",
					"id":                    p.Id,
					"name":                  p.Id,
					"status":                p.Status,
					"project_node_sequence": input.ProjectNodeSequence, // Include sequence for clarity
				})
			}
		}
	case "outputs":
		// Check which projects use this output (consider ProjectNodeSequence for independent instances)
		for _, p := range project.GlobalProject.Projects {
			if output, exists := p.Outputs[id]; exists {
				usage = append(usage, map[string]interface{}{
					"type":                  "project",
					"id":                    p.Id,
					"name":                  p.Id,
					"status":                p.Status,
					"project_node_sequence": output.ProjectNodeSequence, // Include sequence for clarity
				})
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"component_type": componentType,
		"component_id":   id,
		"usage":          usage,
		"note":           "Usage shows actual project-specific component instances, not shared component IDs",
	})
}
