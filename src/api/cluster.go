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
		Type string `json:"type"`
		ID   string `json:"id"`
		Raw  string `json:"raw"`
	}

	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Handle deletion requests
	if strings.HasSuffix(request.Type, "_delete") {
		componentType := strings.TrimSuffix(request.Type, "_delete")

		// Lock only for memory operations
		common.GlobalMu.Lock()
		switch componentType {
		case "ruleset":
			delete(common.AllRulesetsRawConfig, request.ID)
		case "input":
			delete(common.AllInputsRawConfig, request.ID)
		case "output":
			delete(common.AllOutputsRawConfig, request.ID)
		case "project":
			delete(common.AllProjectRawConfig, request.ID)
		}
		common.GlobalMu.Unlock()

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

		// Update running input if it exists (without holding global lock)
		if exists {
			if err := in.Stop(); err != nil {
				logger.Error("failed to stop input on follower", "error", err)
			}
			if newInput, err := input.NewInput("", request.Raw, request.ID); err != nil {
				logger.Error("failed to create new input on follower", "error", err)
			} else {
				// Lock only for updating the map
				common.GlobalMu.Lock()
				p.Inputs[request.ID] = newInput
				common.GlobalMu.Unlock()

				if err := newInput.Start(); err != nil {
					logger.Error("failed to start new input on follower", "error", err)
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

		// Update running output if it exists (without holding global lock)
		if exists {
			if err := out.Stop(); err != nil {
				logger.Error("failed to stop output on follower", "error", err)
			}
			if newOutput, err := output.NewOutput("", request.Raw, request.ID); err != nil {
				logger.Error("failed to create new output on follower", "error", err)
			} else {
				// Lock only for updating the map
				common.GlobalMu.Lock()
				p.Outputs[request.ID] = newOutput
				common.GlobalMu.Unlock()

				if err := newOutput.Start(); err != nil {
					logger.Error("failed to start new output on follower", "error", err)
				}

				// Restart affected projects on follower
				affectedProjects := project.GetAffectedProjects("output", request.ID)
				restartAffectedProjectsOnFollower(affectedProjects)
			}
		}
	case "project":
		// Lock only for memory operations
		common.GlobalMu.Lock()
		common.AllProjectRawConfig[request.ID] = request.Raw
		proj, exists := p.Projects[request.ID]
		common.GlobalMu.Unlock()

		// Handle project lifecycle on follower
		if exists {
			wasRunning := (proj.Status == project.ProjectStatusRunning)

			// Stop the old project if it's running
			if wasRunning {
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

				// Restart project if it was previously running
				if wasRunning {
					if err := newProject.Start(); err != nil {
						logger.Error("failed to restart project on follower", "id", request.ID, "error", err)
					} else {
						logger.Info("Successfully restarted project on follower", "id", request.ID)
					}
				}
			}
		} else {
			// New project, just create it but don't start automatically
			if newProject, err := project.NewProject("", request.Raw, request.ID); err != nil {
				logger.Error("failed to create new project on follower", "id", request.ID, "error", err)
			} else {
				common.GlobalMu.Lock()
				p.Projects[request.ID] = newProject
				common.GlobalMu.Unlock()
				logger.Info("Created new project on follower", "id", request.ID)
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
func restartAffectedProjectsOnFollower(affectedProjects []string) {
	if len(affectedProjects) == 0 {
		return
	}

	logger.Info("Restarting affected projects on follower", "count", len(affectedProjects))

	// First stop all affected projects
	for _, projectID := range affectedProjects {
		if proj, exists := project.GlobalProject.Projects[projectID]; exists {
			if proj.Status == project.ProjectStatusRunning {
				logger.Info("Stopping project for restart on follower", "id", projectID)
				if err := proj.Stop(); err != nil {
					logger.Error("Failed to stop project on follower", "id", projectID, "error", err)
				}
			}
		}
	}

	// Then start all affected projects
	for _, projectID := range affectedProjects {
		if proj, exists := project.GlobalProject.Projects[projectID]; exists {
			if proj.Status == project.ProjectStatusStopped {
				logger.Info("Starting project after changes on follower", "id", projectID)
				if err := proj.Start(); err != nil {
					logger.Error("Failed to start project on follower", "id", projectID, "error", err)
				}
			}
		}
	}
}
