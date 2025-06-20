package api

import (
	"AgentSmith-HUB/project"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

func StartProject(c echo.Context) error {
	var req CtrlProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Get project
	p := project.GlobalProject.Projects[req.ProjectID]
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Project not found",
		})
	}

	// Check if project is already running
	if p.Status == project.ProjectStatusRunning {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is already running",
		})
	}

	// Start the project
	if err := p.Start(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to start project: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Project started successfully",
		"project": map[string]interface{}{
			"id":     p.Id,
			"status": p.Status,
		},
	})
}

func StopProject(c echo.Context) error {
	var req CtrlProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Get project
	p := project.GlobalProject.Projects[req.ProjectID]
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Project not found",
		})
	}

	// Check if project is running
	if p.Status != project.ProjectStatusRunning {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is not running",
		})
	}

	// Stop the project
	if err := p.Stop(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to stop project: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Project stopped successfully",
		"project": map[string]interface{}{
			"id":     p.Id,
			"status": p.Status,
		},
	})
}

func RestartProject(c echo.Context) error {
	var req CtrlProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Get project
	p := project.GlobalProject.Projects[req.ProjectID]
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Project not found",
		})
	}

	// Check if project is running, if so stop it first
	if p.Status == project.ProjectStatusRunning {
		if err := p.Stop(); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("Failed to stop project: %v", err),
			})
		}
	}

	// Start the project
	if err := p.Start(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to start project: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Project restarted successfully",
		"project": map[string]interface{}{
			"id":     p.Id,
			"status": p.Status,
		},
	})
}

func getProjectError(c echo.Context) error {
	id := c.Param("id")
	p := project.GlobalProject.Projects[id]
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"project_id": id,
		"status":     string(p.Status),
		"error":      "", // Simple implementation
	})
}
