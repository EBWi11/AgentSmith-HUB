package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/project"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// ProjectStatusSyncRequest represents a project status sync request to followers
type ProjectStatusSyncRequest struct {
	ProjectID string `json:"project_id"`
	Action    string `json:"action"` // "start", "stop", "restart"
}

// syncProjectStatusToFollowers syncs project status changes to follower nodes
func syncProjectStatusToFollowers(projectID string, action string) {
	// Only sync if this is the leader
	if !cluster.IsLeader {
		return
	}

	cm := cluster.ClusterInstance
	if cm == nil {
		return
	}

	// Get follower nodes
	cm.Mu.RLock()
	followers := make([]*cluster.NodeInfo, 0)
	for _, node := range cm.Nodes {
		if node.Status == cluster.NodeStatusFollower && node.IsHealthy && node.Address != cm.SelfAddress {
			followers = append(followers, node)
		}
	}
	cm.Mu.RUnlock()

	if len(followers) == 0 {
		return
	}

	// Prepare sync data
	syncData := ProjectStatusSyncRequest{
		ProjectID: projectID,
		Action:    action,
	}

	jsonData, err := json.Marshal(syncData)
	if err != nil {
		return
	}

	// Sync to each follower
	for _, node := range followers {
		go func(node *cluster.NodeInfo) {
			url := fmt.Sprintf("http://%s/project-status-sync", node.Address)
			req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
			if err != nil {
				return
			}

			req.Header.Set("token", common.Config.Token)
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err == nil && resp != nil {
				_ = resp.Body.Close()
			}
		}(node)
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

	// Sync status to followers
	go syncProjectStatusToFollowers(req.ProjectID, "start")

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

	// Sync status to followers
	go syncProjectStatusToFollowers(req.ProjectID, "stop")

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

	// Sync status to followers
	go syncProjectStatusToFollowers(req.ProjectID, "restart")

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
