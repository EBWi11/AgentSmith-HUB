package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/project"
	"fmt"
	"net/http"

	"AgentSmith-HUB/common"

	"github.com/labstack/echo/v4"
)

// ProjectStatusSyncRequest represents a project status sync request to followers
type ProjectStatusSyncRequest struct {
	ProjectID string `json:"project_id"`
	Action    string `json:"action"` // "start", "stop", "restart"
}

// syncProjectOperationToFollowers syncs project operation to all follower nodes
func syncProjectOperationToFollowers(projectID, action string) {
	// This function is now handled by the instruction system
	// Just publish the project instruction
	switch action {
	case "start":
		err := cluster.GlobalInstructionManager.PublishProjectStart(projectID)
		if err != nil {
			logger.Error("Failed to publish project start", "project", projectID, "err", err)
		}
	case "stop":
		err := cluster.GlobalInstructionManager.PublishProjectStop(projectID)
		if err != nil {
			logger.Error("Failed to publish project stop", "project", projectID, "err", err)
		}
	case "restart":
		err := cluster.GlobalInstructionManager.PublishProjectRestart(projectID)
		if err != nil {
			logger.Error("Failed to publish project restart", "project", projectID, "err", err)
		}
	default:
		logger.Warn("Unknown project action", "action", action, "project", projectID)
	}
}

func StartProject(c echo.Context) error {
	var req CtrlProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Get project using safe accessor
	p, exists := project.GetProject(req.ProjectID)
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Project not found",
		})
	}

	// API-side persistence: Save project states in Redis
	// proj_states: User intention (what user wants the project to be)
	if err := common.SetProjectUserIntention(req.ProjectID, true); err != nil {
		logger.Warn("Failed to persist project user intention to Redis (proj_states)", "project", req.ProjectID, "error", err)
	}

	// Sync operation to follower nodes FIRST - ensure cluster consistency regardless of local result
	syncProjectOperationToFollowers(req.ProjectID, "start")

	// Start the project
	if err := p.Start(true); err != nil {
		// Record failed operation
		RecordProjectOperation(OpTypeProjectStart, req.ProjectID, "failed", err.Error(), nil)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to start project: %v", err),
		})
	}

	// Record successful operation
	RecordProjectOperation(OpTypeProjectStart, req.ProjectID, "success", "", nil)

	return c.JSON(http.StatusOK, map[string]string{"message": "Project started successfully"})
}

func StopProject(c echo.Context) error {
	var req CtrlProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Get project using safe accessor
	p, exists := project.GetProject(req.ProjectID)
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Project not found",
		})
	}

	// Sync operation to follower nodes FIRST - ensure cluster consistency regardless of local result
	syncProjectOperationToFollowers(req.ProjectID, "stop")

	// Stop the project
	if err := p.Stop(true); err != nil {
		// Record failed operation
		RecordProjectOperation(OpTypeProjectStop, req.ProjectID, "failed", err.Error(), nil)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to stop project: %v", err),
		})
	}

	// API-side persistence: Update project states in Redis
	// proj_states: User intention (user wants project to be stopped)
	if err := common.SetProjectUserIntention(req.ProjectID, false); err != nil {
		logger.Warn("Failed to update project user intention to Redis (proj_states)", "project", req.ProjectID, "error", err)
	}

	// Record successful operation
	RecordProjectOperation(OpTypeProjectStop, req.ProjectID, "success", "", nil)

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

	// Get project using safe accessor
	p, exists := project.GetProject(req.ProjectID)
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Project not found",
		})
	}

	// Sync operation to follower nodes FIRST - ensure cluster consistency regardless of local result
	syncProjectOperationToFollowers(req.ProjectID, "restart")

	err := p.Restart(true, "api")
	if err != nil {
		logger.Error("Failed to restart project after component change", "project_id", req.ProjectID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to restart project: %v", err),
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
	p, exists := project.GetProject(id)
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	var errorMessage string
	if p.Err != nil {
		errorMessage = p.Err.Error()
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"project_id": id,
		"status":     string(p.Status),
		"error":      errorMessage,
	})
}
