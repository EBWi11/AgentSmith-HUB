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

// syncProjectOperationToFollowers syncs a successful local project operation to follower nodes.
func syncProjectOperationToFollowers(projectID, action string) error {
	// This function is now handled by the instruction system
	// Just publish the project instruction
	switch action {
	case "start":
		if err := cluster.GlobalInstructionManager.PublishProjectStart(projectID); err != nil {
			logger.Error("Failed to publish project start", "project", projectID, "err", err)
			return err
		}
	case "stop":
		if err := cluster.GlobalInstructionManager.PublishProjectStop(projectID); err != nil {
			logger.Error("Failed to publish project stop", "project", projectID, "err", err)
			return err
		}
	case "restart":
		if err := cluster.GlobalInstructionManager.PublishProjectRestart(projectID); err != nil {
			logger.Error("Failed to publish project restart", "project", projectID, "err", err)
			return err
		}
	default:
		logger.Error("Unknown project action", "action", action, "project", projectID)
		return fmt.Errorf("unknown project action: %s", action)
	}
	return nil
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

	// Start the project locally first; only publish cluster side effects after
	// the leader has actually converged to the requested state.
	if err := p.StartConverged(); err != nil {
		// Record failed operation
		RecordProjectOperation(OpTypeProjectStart, req.ProjectID, "failed", err.Error(), nil)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to start project: %v", err),
		})
	}

	// API-side persistence: Save project states in Redis
	// proj_states: User intention (what user wants the project to be)
	if err := common.SetProjectUserIntention(req.ProjectID, true); err != nil {
		logger.Error("Failed to persist project user intention to Redis (proj_states)", "project", req.ProjectID, "error", err)
	}

	if err := syncProjectOperationToFollowers(req.ProjectID, "start"); err != nil {
		RecordProjectOperation(OpTypeProjectStart, req.ProjectID, "failed", err.Error(), map[string]interface{}{
			"local_status": p.Status,
			"sync_phase":   "followers",
		})
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Project started locally but failed to sync followers: %v", err),
		})
	}

	// Record successful operation
	RecordProjectOperation(OpTypeProjectStart, req.ProjectID, "success", "", nil)

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

	// Get project using safe accessor
	p, exists := project.GetProject(req.ProjectID)
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Project not found",
		})
	}

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
		logger.Error("Failed to update project user intention to Redis (proj_states)", "project", req.ProjectID, "error", err)
	}

	if err := syncProjectOperationToFollowers(req.ProjectID, "stop"); err != nil {
		RecordProjectOperation(OpTypeProjectStop, req.ProjectID, "failed", err.Error(), map[string]interface{}{
			"local_status": p.Status,
			"sync_phase":   "followers",
		})
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Project stopped locally but failed to sync followers: %v", err),
		})
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

	err := p.Restart(true, "api")
	if err != nil {
		logger.Error("Failed to restart project after component change", "project_id", req.ProjectID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to restart project: %v", err),
		})
	}

	if err := syncProjectOperationToFollowers(req.ProjectID, "restart"); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Project restarted locally but failed to sync followers: %v", err),
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
