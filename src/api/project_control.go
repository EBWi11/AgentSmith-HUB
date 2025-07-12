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

	// Check if project is already running, starting, or stopping
	if p.Status == project.ProjectStatusRunning {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is already running",
		})
	}

	if p.Status == project.ProjectStatusStarting {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is currently starting, please wait",
		})
	}

	if p.Status == project.ProjectStatusStopping {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is currently stopping, please wait",
		})
	}

	// Start the project
	if err := p.Start(); err != nil {
		// Record failed operation
		RecordProjectOperation(OpTypeProjectStart, req.ProjectID, "failed", err.Error(), nil)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to start project: %v", err),
		})
	}

	// API-side persistence: Save project as "running" in Redis
	hashKey := "cluster:proj_states:" + common.Config.LocalIP
	if err := common.RedisHSet(hashKey, req.ProjectID, "running"); err != nil {
		logger.Warn("Failed to persist project start state to Redis", "project", req.ProjectID, "error", err)
	}

	// Record successful operation
	RecordProjectOperation(OpTypeProjectStart, req.ProjectID, "success", "", nil)

	// Save project last updated time separately
	if err := common.SetProjectStateTimestamp(common.Config.LocalIP, req.ProjectID, *p.StatusChangedAt); err != nil {
		logger.Warn("Failed to persist project last updated time to Redis", "project", req.ProjectID, "error", err)
	}

	// Sync operation to follower nodes
	syncProjectOperationToFollowers(req.ProjectID, "start")

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
	if p.Status != project.ProjectStatusRunning {
		if p.Status == project.ProjectStatusStarting {
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "Project is currently starting, cannot stop",
			})
		}
		if p.Status == project.ProjectStatusStopping {
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "Project is already stopping",
			})
		}
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is not running",
		})
	}

	// Stop the project
	if err := p.Stop(); err != nil {
		// Record failed operation
		RecordProjectOperation(OpTypeProjectStop, req.ProjectID, "failed", err.Error(), nil)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to stop project: %v", err),
		})
	}

	// API-side persistence: Remove project from Redis (user wants it stopped)
	hashKey := "cluster:proj_states:" + common.Config.LocalIP
	if err := common.RedisHDel(hashKey, req.ProjectID); err != nil {
		logger.Warn("Failed to remove project state from Redis", "project", req.ProjectID, "error", err)
	}

	// Record successful operation
	RecordProjectOperation(OpTypeProjectStop, req.ProjectID, "success", "", nil)

	// Save project last updated time (stop time)
	if err := common.SetProjectStateTimestamp(common.Config.LocalIP, req.ProjectID, *p.StatusChangedAt); err != nil {
		logger.Warn("Failed to persist project last updated time to Redis", "project", req.ProjectID, "error", err)
	}

	// Sync operation to follower nodes
	syncProjectOperationToFollowers(req.ProjectID, "stop")

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

	// Error projects cannot be restarted, they must be started instead
	if p.Status == project.ProjectStatusError {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Error projects cannot be restarted. Please use start instead.",
		})
	}

	// Check if project is starting or stopping
	if p.Status == project.ProjectStatusStarting {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is currently starting, please wait",
		})
	}

	if p.Status == project.ProjectStatusStopping {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is currently stopping, please wait",
		})
	}

	// Use RestartProjectsSafely to avoid duplicate operation history records
	restartedCount, err := project.RestartProjectsSafely([]string{req.ProjectID}, "user_action")
	if err != nil {
		// Note: RestartProjectsSafely already records failed operations, so we don't record again
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to restart project: %v", err),
		})
	}

	if restartedCount == 0 {
		// Project was not restarted (probably not running)
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is not in a restartable state",
		})
	}

	// Note: Operation history is already recorded in RestartProjectsSafely
	// So we don't need to record it again here

	// Save project last updated time (restart time) - only update timestamp, not desired state
	if err := common.SetProjectStateTimestamp(common.Config.LocalIP, req.ProjectID, *p.StatusChangedAt); err != nil {
		logger.Warn("Failed to persist project restart time to Redis", "project", req.ProjectID, "error", err)
	}

	// Sync operation to follower nodes
	syncProjectOperationToFollowers(req.ProjectID, "restart")

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
