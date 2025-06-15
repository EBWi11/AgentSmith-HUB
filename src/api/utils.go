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

	// Simple implementation - check usage based on component type
	switch componentType {
	case "rulesets":
		// Check which projects use this ruleset
		for _, p := range project.GlobalProject.Projects {
			if _, exists := p.Rulesets[id]; exists {
				usage = append(usage, map[string]interface{}{
					"type":   "project",
					"id":     p.Id,
					"name":   p.Id,
					"status": p.Status,
				})
			}
		}
	case "inputs":
		// Check which projects use this input
		for _, p := range project.GlobalProject.Projects {
			if _, exists := p.Inputs[id]; exists {
				usage = append(usage, map[string]interface{}{
					"type":   "project",
					"id":     p.Id,
					"name":   p.Id,
					"status": p.Status,
				})
			}
		}
	case "outputs":
		// Check which projects use this output
		for _, p := range project.GlobalProject.Projects {
			if _, exists := p.Outputs[id]; exists {
				usage = append(usage, map[string]interface{}{
					"type":   "project",
					"id":     p.Id,
					"name":   p.Id,
					"status": p.Status,
				})
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"component_type": componentType,
		"component_id":   id,
		"usage":          usage,
	})
}
