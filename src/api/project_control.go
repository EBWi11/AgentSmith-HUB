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
		cluster.GlobalInstructionManager.PublishProjectStart(projectID)
	case "stop":
		cluster.GlobalInstructionManager.PublishProjectStop(projectID)
	case "restart":
		cluster.GlobalInstructionManager.PublishProjectRestart(projectID)
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

	// Get project
	p := project.GlobalProject.Projects[req.ProjectID]
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Project not found",
		})
	}

	// API-side persistence: Save project states in Redis
	// proj_states: User intention (what user wants the project to be)
	if err := common.SetProjectUserIntention(req.ProjectID, true); err != nil {
		logger.Warn("Failed to persist project user intention to Redis (proj_states)", "project", req.ProjectID, "error", err)
	}

	// Check if project is already running, starting, or stopping
	if p.Status == common.StatusRunning {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is already running",
		})
	}

	if p.Status == common.StatusStarting {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is currently starting, please wait",
		})
	}

	if p.Status == common.StatusStopping {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is currently stopping, please wait",
		})
	}

	// Sync operation to follower nodes FIRST - ensure cluster consistency regardless of local result
	syncProjectOperationToFollowers(req.ProjectID, "start")

	// Start the project
	if err := p.Start(); err != nil {
		// Record failed operation
		RecordProjectOperation(OpTypeProjectStart, req.ProjectID, "failed", err.Error(), nil)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to start project: %v", err),
		})
	}

	// proj_real: Actual runtime state (what the project actually is)
	if err := common.SetProjectRealState(common.Config.LocalIP, req.ProjectID, string(p.Status)); err != nil {
		logger.Warn("Failed to persist project actual state to Redis (proj_real)", "project", req.ProjectID, "error", err)
	}

	// Record successful operation
	RecordProjectOperation(OpTypeProjectStart, req.ProjectID, "success", "", nil)

	// Save project last updated time separately
	if err := common.SetProjectStateTimestamp(common.Config.LocalIP, req.ProjectID, *p.StatusChangedAt); err != nil {
		logger.Warn("Failed to persist project last updated time to Redis", "project", req.ProjectID, "error", err)
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Project started successfully"})
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
	if p.Status != common.StatusRunning {
		if p.Status == common.StatusStarting {
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "Project is currently starting, cannot stop",
			})
		}
		if p.Status == common.StatusStopping {
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "Project is already stopping",
			})
		}
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is not running",
		})
	}

	// Sync operation to follower nodes FIRST - ensure cluster consistency regardless of local result
	syncProjectOperationToFollowers(req.ProjectID, "stop")

	// Stop the project
	if err := p.Stop(); err != nil {
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

	// proj_real: Actual runtime state (what the project actually is)
	if err := common.SetProjectRealState(common.Config.LocalIP, req.ProjectID, string(p.Status)); err != nil {
		logger.Warn("Failed to update project actual state to Redis (proj_real)", "project", req.ProjectID, "error", err)
	}

	// Record successful operation
	RecordProjectOperation(OpTypeProjectStop, req.ProjectID, "success", "", nil)

	// Save project last updated time (stop time)
	if err := common.SetProjectStateTimestamp(common.Config.LocalIP, req.ProjectID, *p.StatusChangedAt); err != nil {
		logger.Warn("Failed to persist project last updated time to Redis", "project", req.ProjectID, "error", err)
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
	common.GlobalMu.RLock()
	p := project.GlobalProject.Projects[req.ProjectID]
	common.GlobalMu.RUnlock()

	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Project not found",
		})
	}

	// Error projects cannot be restarted, they must be started instead
	if p.Status != common.StatusRunning {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is not running, please wait.",
		})
	}

	// Sync operation to follower nodes FIRST - ensure cluster consistency regardless of local result
	syncProjectOperationToFollowers(req.ProjectID, "restart")

	err := p.Restart()
	if err != nil {
		logger.Error("Failed to restart project after component change", "project_id", req.ProjectID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to restart project: %v", err),
		})
	}

	if err := cluster.GlobalInstructionManager.PublishProjectRestart(req.ProjectID); err != nil {
		logger.Error("Failed to publish project restart instructions", "affected_projects", req.ProjectID, "error", err)
	}

	// Save project last updated time (restart time) - only update timestamp, not desired state
	if err := common.SetProjectStateTimestamp(common.Config.LocalIP, req.ProjectID, *p.StatusChangedAt); err != nil {
		logger.Warn("Failed to persist project restart time to Redis", "project", req.ProjectID, "error", err)
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
