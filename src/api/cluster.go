package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/project"
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

func handleHeartbeat(c echo.Context) error {
	var payload struct {
		NodeID    string `json:"node_id"`
		NodeAddr  string `json:"node_addr"`
		Timestamp string `json:"timestamp"`
	}

	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid heartbeat payload: %v", err),
		})
	}

	cm := cluster.ClusterInstance
	if cm == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "cluster manager not initialized",
		})
	}

	// Check if this is a new node and register it
	cm.Mu.RLock()
	_, nodeExists := cm.Nodes[payload.NodeID]
	cm.Mu.RUnlock()

	if !nodeExists {
		logger.Info("Registering new follower node", "node_id", payload.NodeID, "node_addr", payload.NodeAddr)
		cm.RegisterNode(payload.NodeID, payload.NodeAddr)
	}

	// Update node heartbeat
	cm.UpdateNodeHeartbeat(payload.NodeID)

	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func getClusterStatus(c echo.Context) error {
	cm := cluster.ClusterInstance
	if cm == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "cluster manager not initialized",
		})
	}

	return c.JSON(http.StatusOK, cm.GetClusterStatus())
}

func tokenCheck(c echo.Context) error {
	token := c.Request().Header.Get("token")
	if token == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "missing token",
		})
	}

	if token == common.Config.Token {
		return c.JSON(http.StatusOK, map[string]string{
			"status": "Authentication successful",
		})
	} else {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"status": "Authentication failed",
		})
	}
}

func leaderConfig(c echo.Context) error {
	if cluster.IsLeader {
		return c.JSON(http.StatusOK, map[string]string{
			"redis":          common.Config.Redis,
			"redis_password": common.Config.RedisPassword,
		})
	} else {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "no leader",
		})
	}
}

func downloadConfig(c echo.Context) error {
	configRoot := common.Config.ConfigRoot
	if configRoot == "" {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "config root not set",
		})
	}

	// Create a zip file in memory
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Walk through the config directory
	err := filepath.Walk(configRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Create a new file in the zip
		relPath, err := filepath.Rel(configRoot, path)
		if err != nil {
			return err
		}

		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		// Read and write file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		_, err = writer.Write(content)
		return err
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to create zip: %v", err),
		})
	}

	// Close the zip writer
	if err := zipWriter.Close(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to close zip: %v", err),
		})
	}

	// Calculate zip sha256
	hash := sha256.New()
	_, err = hash.Write(buf.Bytes())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to calculate sha256: %v", err),
		})
	}
	zipSha256 := fmt.Sprintf("%x", hash.Sum(nil))

	// Set response headers
	c.Response().Header().Set(echo.HeaderContentType, "application/zip")
	c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename=config.zip")
	c.Response().Header().Set("X-Config-Sha256", zipSha256)

	// Send the zip file
	return c.Blob(http.StatusOK, "application/zip", buf.Bytes())
}

func getCluster(c echo.Context) error {
	data, _ := json.Marshal(cluster.ClusterInstance)
	return c.String(http.StatusOK, string(data))
}

func handleComponentSync(c echo.Context) error {
	var request struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		Raw       string `json:"raw"`
		IsRunning bool   `json:"is_running,omitempty"` // Add running status for projects
	}

	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Handle deletion requests
	if strings.HasSuffix(request.Type, "_delete") {
		componentType := strings.TrimSuffix(request.Type, "_delete")

		// Stop and remove component instances on follower
		p := project.GlobalProject

		switch componentType {
		case "ruleset":
			// Lock only for memory operations
			common.GlobalMu.Lock()
			delete(common.AllRulesetsRawConfig, request.ID)
			if ruleset, exists := p.Rulesets[request.ID]; exists {
				delete(p.Rulesets, request.ID)
				common.GlobalMu.Unlock()

				// Check if any projects are using this ruleset before stopping
				projectsUsingRuleset := project.UsageCounter.CountProjectsUsingRuleset(request.ID)
				if projectsUsingRuleset == 0 {
					logger.Info("Stopping deleted ruleset component on follower", "id", request.ID)
					if err := ruleset.Stop(); err != nil {
						logger.Error("Failed to stop deleted ruleset on follower", "id", request.ID, "error", err)
					}
				} else {
					logger.Warn("Cannot stop deleted ruleset - still in use by projects on follower", "id", request.ID, "projects_using", projectsUsingRuleset)
				}
			} else {
				common.GlobalMu.Unlock()
			}
		case "input":
			common.GlobalMu.Lock()
			delete(common.AllInputsRawConfig, request.ID)
			if input, exists := p.Inputs[request.ID]; exists {
				delete(p.Inputs, request.ID)
				common.GlobalMu.Unlock()

				// Check if any projects are using this input before stopping
				projectsUsingInput := project.UsageCounter.CountProjectsUsingInput(request.ID)
				if projectsUsingInput == 0 {
					logger.Info("Stopping deleted input component on follower", "id", request.ID)
					if err := input.Stop(); err != nil {
						logger.Error("Failed to stop deleted input on follower", "id", request.ID, "error", err)
					}
				} else {
					logger.Warn("Cannot stop deleted input - still in use by projects on follower", "id", request.ID, "projects_using", projectsUsingInput)
				}
			} else {
				common.GlobalMu.Unlock()
			}
		case "output":
			common.GlobalMu.Lock()
			delete(common.AllOutputsRawConfig, request.ID)
			if output, exists := p.Outputs[request.ID]; exists {
				delete(p.Outputs, request.ID)
				common.GlobalMu.Unlock()

				// Check if any projects are using this output before stopping
				projectsUsingOutput := project.UsageCounter.CountProjectsUsingOutput(request.ID)
				if projectsUsingOutput == 0 {
					logger.Info("Stopping deleted output component on follower", "id", request.ID)
					if err := output.Stop(); err != nil {
						logger.Error("Failed to stop deleted output on follower", "id", request.ID, "error", err)
					}
				} else {
					logger.Warn("Cannot stop deleted output - still in use by projects on follower", "id", request.ID, "projects_using", projectsUsingOutput)
				}
			} else {
				common.GlobalMu.Unlock()
			}
		case "project":
			common.GlobalMu.Lock()
			delete(common.AllProjectRawConfig, request.ID)
			if proj, exists := p.Projects[request.ID]; exists {
				delete(p.Projects, request.ID)
				common.GlobalMu.Unlock()

				// Always stop projects when deleted
				logger.Info("Stopping deleted project on follower", "id", request.ID)
				if err := proj.Stop(); err != nil {
					logger.Error("Failed to stop deleted project on follower", "id", request.ID, "error", err)
				}
			} else {
				common.GlobalMu.Unlock()
			}
		default:
			// Lock only for memory operations
			common.GlobalMu.Lock()
			// Clean up any remaining config references
			delete(common.AllRulesetsRawConfig, request.ID)
			delete(common.AllInputsRawConfig, request.ID)
			delete(common.AllOutputsRawConfig, request.ID)
			delete(common.AllProjectRawConfig, request.ID)
			common.GlobalMu.Unlock()
		}

		return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
	}

	p := project.GlobalProject

	// Handle regular updates
	switch request.Type {
	case "ruleset":
		// Update config map with lock
		common.GlobalMu.Lock()
		common.AllRulesetsRawConfig[request.ID] = request.Raw
		rs, exists := p.Rulesets[request.ID]
		common.GlobalMu.Unlock()

		// Update running ruleset if it exists (without holding global lock)
		if exists {
			if updatedRuleset, err := rs.HotUpdate(request.Raw, request.ID); err != nil {
				logger.Error("failed to hot update ruleset on follower", "error", err)
			} else {
				// Lock only for updating the map
				common.GlobalMu.Lock()
				p.Rulesets[request.ID] = updatedRuleset
				common.GlobalMu.Unlock()

				// Restart affected projects on follower (rulesets support hot reload, but projects may need restart)
				affectedProjects := project.GetAffectedProjects("ruleset", request.ID)
				restartAffectedProjectsOnFollower(affectedProjects)
			}
		}
	case "input":
		// Update config map with lock
		common.GlobalMu.Lock()
		common.AllInputsRawConfig[request.ID] = request.Raw
		in, exists := p.Inputs[request.ID]
		common.GlobalMu.Unlock()

		// Update running input if it exists (respecting shared component architecture)
		if exists {
			// Count how many running projects are using this input (using centralized counter)
			projectsUsingInput := project.UsageCounter.CountProjectsUsingInput(request.ID)

			// Only stop input if no running projects are using it
			if projectsUsingInput == 0 {
				logger.Info("Stopping old input component on follower for reload", "id", request.ID, "projects_using", projectsUsingInput)
				if err := in.Stop(); err != nil {
					logger.Error("failed to stop input on follower", "error", err)
				}
			} else {
				logger.Info("Input component still in use on follower, skipping stop during reload", "id", request.ID, "projects_using", projectsUsingInput)
			}

			if newInput, err := input.NewInput("", request.Raw, request.ID); err != nil {
				logger.Error("failed to create new input on follower", "error", err)
			} else {
				// Lock only for updating the map
				common.GlobalMu.Lock()
				p.Inputs[request.ID] = newInput
				common.GlobalMu.Unlock()

				// Only start if no projects are currently using it (they will start it when needed)
				if projectsUsingInput == 0 {
					if err := newInput.Start(); err != nil {
						logger.Error("failed to start new input on follower", "error", err)
					}
				} else {
					logger.Info("Input component in use, will be started by projects on follower", "id", request.ID)
				}

				// Restart affected projects on follower
				affectedProjects := project.GetAffectedProjects("input", request.ID)
				restartAffectedProjectsOnFollower(affectedProjects)
			}
		}
	case "output":
		// Update config map with lock
		common.GlobalMu.Lock()
		common.AllOutputsRawConfig[request.ID] = request.Raw
		out, exists := p.Outputs[request.ID]
		common.GlobalMu.Unlock()

		// Update running output if it exists (respecting shared component architecture)
		if exists {
			// Count how many running projects are using this output (using centralized counter)
			projectsUsingOutput := project.UsageCounter.CountProjectsUsingOutput(request.ID)

			// Only stop output if no running projects are using it
			if projectsUsingOutput == 0 {
				logger.Info("Stopping old output component on follower for reload", "id", request.ID, "projects_using", projectsUsingOutput)
				if err := out.Stop(); err != nil {
					logger.Error("failed to stop output on follower", "error", err)
				}
			} else {
				logger.Info("Output component still in use on follower, skipping stop during reload", "id", request.ID, "projects_using", projectsUsingOutput)
			}

			if newOutput, err := output.NewOutput("", request.Raw, request.ID); err != nil {
				logger.Error("failed to create new output on follower", "error", err)
			} else {
				// Lock only for updating the map
				common.GlobalMu.Lock()
				p.Outputs[request.ID] = newOutput
				common.GlobalMu.Unlock()

				// Only start if no projects are currently using it (they will start it when needed)
				if projectsUsingOutput == 0 {
					if err := newOutput.Start(); err != nil {
						logger.Error("failed to start new output on follower", "error", err)
					}
				} else {
					logger.Info("Output component in use, will be started by projects on follower", "id", request.ID)
				}

				// Restart affected projects on follower
				affectedProjects := project.GetAffectedProjects("output", request.ID)
				restartAffectedProjectsOnFollower(affectedProjects)
			}
		}
	case "project":
		// Update config map with lock
		common.GlobalMu.Lock()
		common.AllProjectRawConfig[request.ID] = request.Raw
		proj, exists := p.Projects[request.ID]
		common.GlobalMu.Unlock()

		// Handle project lifecycle on follower
		if exists {
			// Stop the old project if it's running
			if proj.Status == project.ProjectStatusRunning {
				if err := proj.Stop(); err != nil {
					logger.Error("failed to stop project on follower", "id", request.ID, "error", err)
				}
			}

			// Create new project instance
			if newProject, err := project.NewProject("", request.Raw, request.ID); err != nil {
				logger.Error("failed to create new project on follower", "id", request.ID, "error", err)
			} else {
				// Lock only for updating the map
				common.GlobalMu.Lock()
				p.Projects[request.ID] = newProject
				common.GlobalMu.Unlock()

				// Start project based on leader's running status
				if request.IsRunning {
					logger.Info("Starting project on follower based on leader status", "id", request.ID)
					if err := newProject.Start(); err != nil {
						logger.Error("Failed to start project on follower", "id", request.ID, "error", err)
					}
				} else {
					logger.Info("Project not running on leader, keeping stopped on follower", "id", request.ID)
				}
			}
		} else {
			// New project, create it and start if needed
			if newProject, err := project.NewProject("", request.Raw, request.ID); err != nil {
				logger.Error("failed to create new project on follower", "id", request.ID, "error", err)
			} else {
				common.GlobalMu.Lock()
				p.Projects[request.ID] = newProject
				common.GlobalMu.Unlock()

				// Start project based on leader's running status
				if request.IsRunning {
					logger.Info("Starting new project on follower based on leader status", "id", request.ID)
					if err := newProject.Start(); err != nil {
						logger.Error("Failed to start new project on follower", "id", request.ID, "error", err)
					}
				} else {
					logger.Info("Created new project on follower (not running on leader)", "id", request.ID)
				}
			}
		}
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported component type"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "synced"})
}

func syncToFollowers(method, path string, body []byte) {
	cm := cluster.ClusterInstance
	if cm == nil {
		logger.Warn("Cluster manager not initialized, skipping follower sync")
		return
	}

	// Get follower nodes with read lock
	cm.Mu.RLock()
	followers := make([]*cluster.NodeInfo, 0)
	for _, node := range cm.Nodes {
		if node.Status == cluster.NodeStatusFollower && node.IsHealthy && node.Address != cm.SelfAddress {
			followers = append(followers, node)
		}
	}
	cm.Mu.RUnlock()

	if len(followers) == 0 {
		logger.Info("No healthy follower nodes found, skipping sync")
		return
	}

	// Sync to each follower with proper error handling
	var wg sync.WaitGroup
	for _, node := range followers {
		wg.Add(1)
		go func(node *cluster.NodeInfo) {
			defer wg.Done()

			url := "http://" + node.Address + path
			success := false

			for i := 0; i < 3; i++ {
				req, err := http.NewRequest(method, url, bytes.NewReader(body))
				if err != nil {
					logger.Error("Failed to create sync request", "node", node.Address, "error", err)
					break
				}

				req.Header.Set("token", common.Config.Token)
				if len(body) > 0 {
					req.Header.Set("Content-Type", "application/json")
				}

				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Do(req)
				if err == nil && resp.StatusCode < 300 {
					_ = resp.Body.Close()
					success = true
					break
				}

				if resp != nil {
					_ = resp.Body.Close()
				}

				logger.Warn("Sync attempt failed", "node", node.Address, "attempt", i+1, "error", err)
				time.Sleep(time.Duration(i+1) * time.Second)
			}

			if success {
				logger.Info("Successfully synced to follower", "node", node.Address, "path", path)
			} else {
				logger.Error("Failed to sync to follower after all retries", "node", node.Address, "path", path)
			}
		}(node)
	}

	// Wait for all sync operations to complete
	wg.Wait()
}

// restartAffectedProjectsOnFollower restarts projects affected by component changes on follower nodes
// Uses the same individual restart approach as leader to respect component sharing
func restartAffectedProjectsOnFollower(affectedProjects []string) {
	if len(affectedProjects) == 0 {
		return
	}

	logger.Info("Restarting affected projects on follower", "count", len(affectedProjects))

	// Use unified restart function for better maintainability and consistency
	restartedCount, err := project.RestartProjectsSafely(affectedProjects)
	if err != nil {
		logger.Error("Error during affected projects restart on follower", "error", err)
	}

	logger.Info("Batch restart completed on follower", "total_affected", len(affectedProjects), "restarted", restartedCount)
}

func handleProjectStatusSync(c echo.Context) error {
	var request struct {
		ProjectID string `json:"project_id"`
		Action    string `json:"action"`
	}

	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Get project
	common.GlobalMu.RLock()
	proj, exists := project.GlobalProject.Projects[request.ProjectID]
	common.GlobalMu.RUnlock()

	if !exists {
		logger.Warn("Project not found on follower for status sync", "project_id", request.ProjectID)
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	// Execute the action
	switch request.Action {
	case "start":
		if proj.Status != project.ProjectStatusRunning {
			logger.Info("Starting project on follower based on leader command", "project_id", request.ProjectID)
			if err := proj.Start(); err != nil {
				logger.Error("Failed to start project on follower", "project_id", request.ProjectID, "error", err)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start project"})
			}
		}
	case "stop":
		if proj.Status == project.ProjectStatusRunning {
			logger.Info("Stopping project on follower based on leader command", "project_id", request.ProjectID)
			if err := proj.Stop(); err != nil {
				logger.Error("Failed to stop project on follower", "project_id", request.ProjectID, "error", err)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to stop project"})
			}
		}
	case "restart":
		logger.Info("Restarting project on follower based on leader command", "project_id", request.ProjectID)
		if proj.Status == project.ProjectStatusRunning {
			if err := proj.Stop(); err != nil {
				logger.Error("Failed to stop project during restart on follower", "project_id", request.ProjectID, "error", err)
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to stop project during restart"})
			}
		}
		if err := proj.Start(); err != nil {
			logger.Error("Failed to start project during restart on follower", "project_id", request.ProjectID, "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start project during restart"})
		}
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported action"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

// handleQPSSync handles QPS data synchronization from followers
func handleQPSSync(c echo.Context) error {
	// Only accept QPS data on leader nodes
	if !cluster.IsLeader {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "QPS data can only be sent to leader nodes",
		})
	}

	var qpsDataList []common.QPSMetrics
	if err := c.Bind(&qpsDataList); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid QPS data format: %v", err),
		})
	}

	// Add each QPS metric to the global QPS manager
	if common.GlobalQPSManager != nil {
		for _, qpsData := range qpsDataList {
			common.GlobalQPSManager.AddQPSData(&qpsData)
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":           "success",
		"received_metrics": len(qpsDataList),
		"timestamp":        time.Now(),
	})
}

// getQPSData returns QPS data for query
func getQPSData(c echo.Context) error {
	// Only provide QPS data from leader nodes
	if !cluster.IsLeader {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "QPS data is only available from leader nodes",
		})
	}

	if common.GlobalQPSManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "QPS manager not initialized",
		})
	}

	projectID := c.QueryParam("project_id")
	nodeID := c.QueryParam("node_id")
	componentID := c.QueryParam("component_id")
	componentType := c.QueryParam("component_type")
	aggregated := c.QueryParam("aggregated") == "true"

	var result interface{}

	if aggregated && projectID != "" {
		// Return aggregated QPS data for a project
		result = common.GlobalQPSManager.GetAggregatedQPS(projectID)
	} else if projectID != "" && nodeID == "" {
		// Return all components in a project
		result = common.GlobalQPSManager.GetProjectQPS(projectID)
	} else if nodeID != "" && projectID != "" && componentID != "" && componentType != "" {
		// Return specific component QPS data using legacy method for backward compatibility
		result = common.GlobalQPSManager.GetComponentQPSLegacy(nodeID, projectID, componentID, componentType)
	} else if nodeID == "" && projectID == "" {
		// Return all QPS data
		result = common.GlobalQPSManager.GetAllQPS()
	} else {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid query parameters. Use 'project_id' for project data, or specify 'node_id', 'project_id', 'component_id', and 'component_type' for specific component data",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":      result,
		"timestamp": time.Now(),
		"stats":     common.GlobalQPSManager.GetStats(),
	})
}

// getQPSStats returns QPS manager statistics
func getQPSStats(c echo.Context) error {
	// Only provide QPS stats from leader nodes
	if !cluster.IsLeader {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "QPS statistics are only available from leader nodes",
		})
	}

	if common.GlobalQPSManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "QPS manager not initialized",
		})
	}

	stats := common.GlobalQPSManager.GetStats()
	return c.JSON(http.StatusOK, stats)
}

// getHourlyMessages returns real message counts for the past hour
func getHourlyMessages(c echo.Context) error {
	// Only provide message data from leader nodes
	if !cluster.IsLeader {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Message data is only available from leader nodes",
		})
	}

	if common.GlobalQPSManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "QPS manager not initialized",
		})
	}

	projectID := c.QueryParam("project_id")
	nodeID := c.QueryParam("node_id")
	aggregated := c.QueryParam("aggregated") == "true"
	byNode := c.QueryParam("by_node") == "true"

	var result interface{}

	if byNode {
		if nodeID != "" {
			// Return message counts for a specific node
			result = common.GlobalQPSManager.GetNodeHourlyMessages(nodeID)
		} else {
			// Return message counts for all nodes
			result = common.GlobalQPSManager.GetAllNodeHourlyMessages()
		}
	} else if aggregated {
		// Return aggregated message counts across all projects and nodes
		result = common.GlobalQPSManager.GetAggregatedHourlyMessages()
	} else {
		// Return message counts for a specific project (or all if projectID is empty)
		result = common.GlobalQPSManager.GetHourlyMessageCounts(projectID)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":              result,
		"timestamp":         time.Now(),
		"period":            "past_hour",
		"cache_updated_at":  common.GlobalQPSManager.GetCacheUpdateTime(),
		"cache_update_note": "Message counts are calculated every minute to optimize performance",
	})
}

// getSystemMetrics returns current and historical system metrics for this node
func getSystemMetrics(c echo.Context) error {
	if common.GlobalSystemMonitor == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "System monitor not initialized",
		})
	}

	// Parse query parameters
	sinceParam := c.QueryParam("since")
	currentOnly := c.QueryParam("current") == "true"

	if currentOnly {
		// Return only current metrics
		current := common.GlobalSystemMonitor.GetCurrentMetrics()
		if current == nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "No system metrics available",
			})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"current":   current,
			"timestamp": time.Now(),
		})
	}

	var historical []common.SystemDataPoint
	if sinceParam != "" {
		// Parse since timestamp
		since, err := time.Parse(time.RFC3339, sinceParam)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("Invalid since parameter format: %v", err),
			})
		}
		historical = common.GlobalSystemMonitor.GetHistoricalMetrics(since)
	} else {
		// Return all historical data
		historical = common.GlobalSystemMonitor.GetAllMetrics()
	}

	current := common.GlobalSystemMonitor.GetCurrentMetrics()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"current":    current,
		"historical": historical,
		"timestamp":  time.Now(),
		"stats":      common.GlobalSystemMonitor.GetStats(),
	})
}

// getSystemStats returns system monitor statistics
func getSystemStats(c echo.Context) error {
	if common.GlobalSystemMonitor == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "System monitor not initialized",
		})
	}

	stats := common.GlobalSystemMonitor.GetStats()
	return c.JSON(http.StatusOK, stats)
}

// handleMetricsSync handles combined QPS and system metrics synchronization from followers
func handleMetricsSync(c echo.Context) error {
	// Only accept metrics data on leader nodes
	if !cluster.IsLeader {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Metrics data can only be sent to leader nodes",
		})
	}

	var payload struct {
		QPSData       []common.QPSMetrics   `json:"qps_data"`
		SystemMetrics *common.SystemMetrics `json:"system_metrics"`
	}

	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid metrics data format: %v", err),
		})
	}

	var processedQPS, processedSystem int

	// Process QPS data
	if common.GlobalQPSManager != nil && len(payload.QPSData) > 0 {
		for _, qpsData := range payload.QPSData {
			common.GlobalQPSManager.AddQPSData(&qpsData)
			processedQPS++
		}
	}

	// Process system metrics
	if common.GlobalClusterSystemManager != nil && payload.SystemMetrics != nil {
		common.GlobalClusterSystemManager.AddSystemMetrics(payload.SystemMetrics)
		processedSystem = 1
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":           "success",
		"processed_qps":    processedQPS,
		"processed_system": processedSystem,
		"timestamp":        time.Now(),
	})
}

// getClusterSystemMetrics returns system metrics for all cluster nodes
func getClusterSystemMetrics(c echo.Context) error {
	// Only provide cluster system metrics from leader nodes
	if !cluster.IsLeader {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Cluster system metrics are only available from leader nodes",
		})
	}

	if common.GlobalClusterSystemManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Cluster system manager not initialized",
		})
	}

	nodeID := c.QueryParam("node_id")
	aggregated := c.QueryParam("aggregated") == "true"

	if aggregated {
		// Return aggregated metrics across all nodes
		aggregatedMetrics := common.GlobalClusterSystemManager.GetAggregatedMetrics()
		return c.JSON(http.StatusOK, aggregatedMetrics)
	} else if nodeID != "" {
		// Return metrics for specific node
		metrics := common.GlobalClusterSystemManager.GetNodeMetrics(nodeID)
		if metrics == nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": fmt.Sprintf("No metrics found for node: %s", nodeID),
			})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{
			"node_id":   nodeID,
			"metrics":   metrics,
			"timestamp": time.Now(),
		})
	} else {
		// Return metrics for all nodes
		allMetrics := common.GlobalClusterSystemManager.GetAllMetrics()
		return c.JSON(http.StatusOK, map[string]interface{}{
			"metrics":   allMetrics,
			"timestamp": time.Now(),
			"stats":     common.GlobalClusterSystemManager.GetStats(),
		})
	}
}

// getClusterSystemStats returns cluster system manager statistics
func getClusterSystemStats(c echo.Context) error {
	// Only provide cluster system stats from leader nodes
	if !cluster.IsLeader {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Cluster system statistics are only available from leader nodes",
		})
	}

	if common.GlobalClusterSystemManager == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Cluster system manager not initialized",
		})
	}

	stats := common.GlobalClusterSystemManager.GetStats()
	return c.JSON(http.StatusOK, stats)
}
