package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func ping(c echo.Context) error {
	return c.String(http.StatusOK, "pong")
}

// getProjects returns a list of all projects
func getProjects(c echo.Context) error {
	p := project.GlobalProject
	result := make([]map[string]interface{}, 0, 0)

	for _, p := range p.Projects {
		result = append(result, map[string]interface{}{
			"id":     p.Id,
			"status": p.Status,
		})
	}

	for id, _ := range p.ProjectsNew {
		result = append(result, map[string]interface{}{
			"id":     id,
			"status": project.ProjectStatusStopped,
		})
	}
	return c.JSON(http.StatusOK, result)
}

// getProject returns details of a specific project
func getProject(c echo.Context) error {
	id := c.Param("id")

	p_raw := project.GetProjectNew(id)
	if p_raw != "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":     id,
			"status": project.ProjectStatusStopped,
			"raw":    p_raw,
		})
	}

	p := project.GetProject(id)
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":     p.Id,
		"status": p.Status,
		"raw":    p.Config.RawConfig,
	})
}

// getRulesets returns a list of all rulesets
func getRulesets(c echo.Context) error {
	p := project.GlobalProject
	rulesets := make([]map[string]interface{}, 0)

	for _, r := range p.Rulesets {
		rulesets = append(rulesets, map[string]interface{}{
			"id": r.RulesetID,
		})
	}

	for id, _ := range p.RulesetsNew {
		rulesets = append(rulesets, map[string]interface{}{
			"id": id,
		})
	}
	return c.JSON(http.StatusOK, rulesets)
}

// getRuleset returns details of a specific ruleset
func getRuleset(c echo.Context) error {
	id := c.Param("id")
	r_raw := project.GetRulesetNew(id)
	if r_raw != "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":  id,
			"raw": r_raw,
		})
	}

	r := project.GetRuleset(id)

	if r != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":  r.RulesetID,
			"raw": r.RawConfig,
		})
	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
}

// getInputs returns a list of all input components
func getInputs(c echo.Context) error {
	p := project.GlobalProject
	inputs := make([]map[string]interface{}, 0)

	for _, in := range p.Inputs {
		inputs = append(inputs, map[string]interface{}{
			"id": in.Id,
		})
	}

	for id, _ := range p.InputsNew {
		inputs = append(inputs, map[string]interface{}{
			"id": id,
		})
	}
	return c.JSON(http.StatusOK, inputs)
}

// getInput returns details of a specific input component
func getInput(c echo.Context) error {
	id := c.Param("id")
	in_raw := project.GlobalProject.InputsNew[id]

	if in_raw != "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":  id,
			"raw": in_raw,
		})
	}

	in := project.GlobalProject.Inputs[id]

	if in != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":  in.Id,
			"raw": in.Config.RawConfig,
		})

	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "input not found"})
}

func getPlugins(c echo.Context) error {
	plugins := make([]map[string]interface{}, 0)

	for _, p := range plugin.Plugins {
		if p.Type == plugin.YAEGI_PLUGIN {
			plugins = append(plugins, map[string]interface{}{
				"name": p.Name,
			})
		}
	}

	for name, _ := range plugin.PluginsNew {
		plugins = append(plugins, map[string]interface{}{
			"name": name,
		})
	}
	return c.JSON(http.StatusOK, plugins)
}

func getPlugin(c echo.Context) error {
	name := c.Param("name")

	p_raw := plugin.PluginsNew[name]
	if p_raw != "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"name": name,
			"raw":  p_raw,
		})
	}

	if p, ok := plugin.Plugins[name]; ok {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"name": p.Name,
			"raw":  string(p.Payload),
		})
	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "plugin not found"})
}

// getOutputs returns a list of all output components
func getOutputs(c echo.Context) error {
	p := project.GlobalProject
	outputs := make([]map[string]interface{}, 0)

	for _, out := range p.Outputs {
		outputs = append(outputs, map[string]interface{}{
			"id": out.Id,
		})
	}

	for id, _ := range p.OutputsNew {
		outputs = append(outputs, map[string]interface{}{
			"id": id,
		})
	}
	return c.JSON(http.StatusOK, outputs)
}

// getOutput returns details of a specific output component
func getOutput(c echo.Context) error {
	id := c.Param("id")
	out_raw := project.GlobalProject.ProjectsNew[id]
	if out_raw != "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":  id,
			"raw": out_raw,
		})
	}

	out := project.GlobalProject.Outputs[id]

	if out != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":  out.Id,
			"raw": out.Config.RawConfig,
		})
	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "output not found"})
}

// getMetrics returns metrics for all projects
func getMetrics(c echo.Context) error {
	p := project.GlobalProject
	metrics := make(map[string]interface{})

	for _, p := range p.Projects {
		projectMetrics := p.GetMetrics()
		metrics[p.Id] = map[string]interface{}{
			"input_qps":  projectMetrics.InputQPS,
			"output_qps": projectMetrics.OutputQPS,
		}
	}
	return c.JSON(http.StatusOK, metrics)
}

// getProjectMetrics returns metrics for a specific project
func getProjectMetrics(c echo.Context) error {
	id := c.Param("id")
	p := project.GetProject(id)
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	metrics := p.GetMetrics()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"input_qps":  metrics.InputQPS,
		"output_qps": metrics.OutputQPS,
	})
}

// getRedisMetrics returns Redis server metrics
func getRedisMetrics(c echo.Context) error {
	metrics, err := common.GetRedisMetrics()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to get Redis metrics: %v", err),
		})
	}
	return c.JSON(http.StatusOK, metrics)
}

// handleHeartbeat handles incoming heartbeat requests from cluster nodes
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

// getClusterStatus returns the current cluster status
func getClusterStatus(c echo.Context) error {
	cm := cluster.ClusterInstance
	if cm == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "cluster manager not initialized",
		})
	}

	return c.JSON(http.StatusOK, cm.GetClusterStatus())
}

// downloadConfig handles downloading the entire config directory
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

	// Get zip sha256
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

// FileChecksum represents a file's checksum information
type FileChecksum struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

type CtrlProjectRequest struct {
	ProjectID string `json:"project_id"`
}

// StartProject starts a project with the given configuration
func StartProject(c echo.Context) error {
	var req CtrlProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Get project
	p := project.GetProject(req.ProjectID)
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

// StopProject stops a running project
func StopProject(c echo.Context) error {
	var req CtrlProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Get project
	p := project.GetProject(req.ProjectID)
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

func getCluster(c echo.Context) error {
	data, _ := json.Marshal(cluster.ClusterInstance)
	return c.String(http.StatusOK, string(data))
}

// handleComponentSync handles component synchronization from leader to follower
func handleComponentSync(c echo.Context) error {
	var request struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Raw  string `json:"raw"`
	}

	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	// Handle deletion requests
	if strings.HasSuffix(request.Type, "_delete") {
		componentType := strings.TrimSuffix(request.Type, "_delete")
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
		return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
	}

	p := project.GlobalProject

	// Handle regular updates
	switch request.Type {
	case "ruleset":
		common.AllRulesetsRawConfig[request.ID] = request.Raw
		// Update running ruleset if it exists
		if rs, ok := p.Rulesets[request.ID]; ok {
			if updatedRuleset, err := rs.HotUpdate(request.Raw, request.ID); err != nil {
				logger.Error("failed to hot update ruleset on follower", "error", err)
			} else {
				p.Rulesets[request.ID] = updatedRuleset
			}
			break
		}
	case "input":
		common.AllInputsRawConfig[request.ID] = request.Raw
		// Update running input if it exists
		if in, ok := p.Inputs[request.ID]; ok {
			if err := in.Stop(); err != nil {
				logger.Error("failed to stop input on follower", "error", err)
			}
			if newInput, err := input.NewInput("", request.Raw, request.ID); err != nil {
				logger.Error("failed to create new input on follower", "error", err)
			} else {
				p.Inputs[request.ID] = newInput
				if err := newInput.Start(); err != nil {
					logger.Error("failed to start new input on follower", "error", err)
				}
			}
			break
		}
	case "output":
		common.AllOutputsRawConfig[request.ID] = request.Raw
		// Update running output if it exists
		if out, ok := p.Outputs[request.ID]; ok {
			if err := out.Stop(); err != nil {
				logger.Error("failed to stop output on follower", "error", err)
			}
			if newOutput, err := output.NewOutput("", request.Raw, request.ID); err != nil {
				logger.Error("failed to create new output on follower", "error", err)
			} else {
				p.Outputs[request.ID] = newOutput
				if err := newOutput.Start(); err != nil {
					logger.Error("failed to start new output on follower", "error", err)
				}
			}
			break
		}
	case "project":
		common.AllProjectRawConfig[request.ID] = request.Raw
		// For projects, we don't automatically start them on followers
		// They should be started explicitly through the start project API
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported component type"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "synced"})
}

// updateRuleset handler function
func updateRuleset(c echo.Context) error {
	id := c.Param("id")
	var requestBody struct {
		Raw string `json:"raw"`
	}

	if err := c.Bind(&requestBody); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if requestBody.Raw == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "raw ruleset content is required"})
	}

	p := project.GlobalProject
	var updatedRuleset *rules_engine.Ruleset
	var err error

	if rs, ok := p.Rulesets[id]; ok {
		updatedRuleset, err = rs.HotUpdate(requestBody.Raw, id)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("failed to update ruleset: %v", err),
			})
		}

		// Update the ruleset in the project and global config
		p.Rulesets[id] = updatedRuleset
		common.GlobalMu.Lock()
		common.AllRulesetsRawConfig[id] = requestBody.Raw
		common.GlobalMu.Unlock()

		// If this is the leader node, notify all followers
		if cluster.IsLeader {
			if err := cluster.NotifyFollowersComponentUpdate("ruleset", id, requestBody.Raw); err != nil {
				logger.Error("failed to notify followers of ruleset update", "error", err)
			}
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":           updatedRuleset.RulesetID,
			"type":         updatedRuleset.Type,
			"is_detection": updatedRuleset.IsDetection,
			"raw":          updatedRuleset.RawConfig,
			"message":      "ruleset updated successfully",
		})
	}

	return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
}

func createComponent(componentType string, c echo.Context) error {
	var request struct {
		ID  string `json:"id"`
		Raw string `json:"raw"`
	}

	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if request.ID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id and raw content are required"})
	}

	filtPath, exist := GetComponentPath(componentType, request.ID, true)
	if exist {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "this file already exists"})
	}

	_, exist = GetComponentPath(componentType, request.ID, false)
	if exist {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "this file already exists"})
	}

	err := WriteComponentFile(filtPath, request.Raw)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	} else {
		switch componentType {
		case "plugin":
			plugin.PluginsNew[request.ID] = NewPluginData
		case "input":
			project.GlobalProject.InputsNew[request.ID] = NewInputData
		case "output":
			project.GlobalProject.OutputsNew[request.ID] = NewOutputData
		case "ruleset":
			project.GlobalProject.RulesetsNew[request.ID] = NewRulesetData
		case "project":
			project.GlobalProject.ProjectsNew[request.ID] = NewProjectData
		}
		return c.JSON(http.StatusCreated, map[string]string{"message": "created successfully"})
	}
}

// Component creation handlers
func createRuleset(c echo.Context) error {
	return createComponent("ruleset", c)
}

func createInput(c echo.Context) error {
	return createComponent("input", c)
}

func createOutput(c echo.Context) error {
	return createComponent("output", c)
}

func createProject(c echo.Context) error {
	return createComponent("project", c)
}

func createPlugin(c echo.Context) error {
	return createComponent("plugin", c)
}

// deleteComponent handles deletion of components
func deleteComponent(componentType string, c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	// Check if component is in use by any project
	projects := project.GlobalProject.Projects
	for _, p := range projects {
		var inUse bool
		switch componentType {
		case "ruleset":
			_, inUse = p.Rulesets[id]
		case "input":
			_, inUse = p.Inputs[id]
		case "output":
			_, inUse = p.Outputs[id]
		}
		if inUse {
			return c.JSON(http.StatusConflict, map[string]string{
				"error": fmt.Sprintf("%s is currently in use by project %s", id, p.Id),
			})
		}
	}

	// If this is the leader node, delete config file and notify followers
	if cluster.IsLeader {
		if err := common.DeleteConfigFile(componentType, id); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("failed to delete configuration: %v", err),
			})
		}

		// Notify followers about deletion
		if err := cluster.NotifyFollowersComponentUpdate(componentType+"_delete", id, ""); err != nil {
			logger.Error("failed to notify followers of component deletion", "error", err)
		}
	}

	// Remove from global configuration
	common.GlobalMu.Lock()
	switch componentType {
	case "ruleset":
		delete(common.AllRulesetsRawConfig, id)
	case "input":
		delete(common.AllInputsRawConfig, id)
	case "output":
		delete(common.AllOutputsRawConfig, id)
	case "project":
		delete(common.AllProjectRawConfig, id)
	}
	common.GlobalMu.Unlock()

	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("%s deleted successfully", componentType),
	})
}

// Component deletion handlers
func deleteRuleset(c echo.Context) error {
	var r *rules_engine.Ruleset
	var ok bool
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	if r, ok = project.GlobalProject.Rulesets[id]; !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id not found"})
	}

	delete(project.GlobalProject.Rulesets, id)

	if cluster.IsLeader {
		go syncToFollowers("DELETE", "/ruleset/"+id, nil)
		_ = os.Remove(r.Path)
	} else {
		delete(common.AllRulesetsRawConfig, id)
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "ruleset deleted successfully"})
}

func deleteInput(c echo.Context) error {
	var i *input.Input
	var ok bool

	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	if i, ok = project.GlobalProject.Inputs[id]; !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id not found"})
	}

	delete(project.GlobalProject.Inputs, id)

	if cluster.IsLeader {
		go syncToFollowers("DELETE", "/input/"+id, nil)
		_ = os.Remove(i.Path)
	} else {
		delete(common.AllInputsRawConfig, id)
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "input deleted successfully"})
}

func deleteOutput(c echo.Context) error {
	var o *output.Output
	var ok bool

	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	if o, ok = project.GlobalProject.Outputs[id]; !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id not found"})
	}

	delete(project.GlobalProject.Outputs, id)

	if cluster.IsLeader {
		go syncToFollowers("DELETE", "/output/"+id, nil)
		_ = os.Remove(o.Path)
	} else {
		delete(common.AllOutputsRawConfig, id)
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "output deleted successfully"})
}

func deleteProject(c echo.Context) error {
	var p *project.Project
	var ok bool

	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	if p, ok = project.GlobalProject.Projects[id]; !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id not found"})
	}

	if cluster.IsLeader {
		_ = os.Remove(p.Config.Path)
		go syncToFollowers("DELETE", "/project/"+id, nil)
	} else {
		delete(common.AllProjectRawConfig, id)
	}

	delete(project.GlobalProject.Projects, id)

	return c.JSON(http.StatusOK, map[string]string{"message": "project deleted successfully"})
}

func deletePlugin(c echo.Context) error {
	var p *plugin.Plugin
	var ok bool

	name := c.Param("name")
	if name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required"})
	}

	if p, ok = plugin.Plugins[name]; !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is not found"})
	}

	if cluster.IsLeader {
		go syncToFollowers("DELETE", "/plugin/"+name, nil)
		_ = os.Remove(p.Path)
	} else {
		delete(common.AllPluginsRawConfig, name)
	}

	delete(plugin.Plugins, name)

	return c.JSON(http.StatusOK, map[string]string{"message": "plugin deleted successfully"})
}

// 通用同步方法：只同步到健康的 follower 节点
func syncToFollowers(method, path string, body []byte) {
	cm := cluster.ClusterInstance
	if cm == nil {
		return
	}
	cm.Mu.Lock()
	defer cm.Mu.Unlock()
	for _, node := range cm.Nodes {
		if node.Status != cluster.NodeStatusFollower || !node.IsHealthy || node.Address == cm.SelfAddress {
			continue
		}
		url := "http://" + node.Address + path
		for i := 0; i < 3; i++ {
			req, _ := http.NewRequest(method, url, bytes.NewReader(body))
			req.Header.Set("token", common.Config.Token)
			if len(body) > 0 {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := http.DefaultClient.Do(req)
			if err == nil && resp.StatusCode < 300 {
				break // 成功
			}
			time.Sleep(2 * time.Second)
		}
	}
}

// Update an existing plugin
func updatePlugin(c echo.Context) error {
	name := c.Param("name")
	var req struct {
		Raw string `json:"raw"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Raw == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "raw is required"})
	}
	p, ok := plugin.Plugins[name]
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "plugin not found"})
	}

	if err := os.WriteFile(p.Path, []byte(req.Raw), 0644); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write plugin file"})
	}

	newP, err := plugin.LoadPlugin(p.Path)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to reload plugin: " + err.Error()})
	}
	plugin.Plugins[name] = newP

	if cluster.IsLeader {
		body, _ := json.Marshal(map[string]string{"id": name, "raw": req.Raw})
		go syncToFollowers("PUT", "/plugin/"+name, body)
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "plugin updated successfully"})
}

// Update an existing input
func updateInput(c echo.Context) error {
	id := c.Param("id")
	var req struct {
		Raw string `json:"raw"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Raw == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "raw is required"})
	}
	in, ok := project.GlobalProject.Inputs[id]
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "input not found"})
	}

	// Write config file
	if cluster.IsLeader {
		if err := common.WriteConfigFile("input", id, req.Raw); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write config file"})
		}
		body, _ := json.Marshal(map[string]string{"id": id, "raw": req.Raw})
		go syncToFollowers("PUT", "/input/"+id, body)
	}

	// Hot reload
	if err := in.Stop(); err != nil {
		logger.Error("failed to stop input", "error", err)
	}
	newIn, err := input.NewInput("", req.Raw, id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid input config: " + err.Error()})
	}
	project.GlobalProject.Inputs[id] = newIn
	common.GlobalMu.Lock()
	common.AllInputsRawConfig[id] = req.Raw
	common.GlobalMu.Unlock()

	return c.JSON(http.StatusOK, map[string]string{"message": "input updated successfully"})
}

// Update an existing output
func updateOutput(c echo.Context) error {
	id := c.Param("id")
	var req struct {
		Raw string `json:"raw"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Raw == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "raw is required"})
	}
	out, ok := project.GlobalProject.Outputs[id]
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "output not found"})
	}

	// Write config file
	if cluster.IsLeader {
		if err := common.WriteConfigFile("output", id, req.Raw); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write config file"})
		}
		body, _ := json.Marshal(map[string]string{"id": id, "raw": req.Raw})
		go syncToFollowers("PUT", "/output/"+id, body)
	}

	// Hot reload
	if err := out.Stop(); err != nil {
		logger.Error("failed to stop output", "error", err)
	}
	newOut, err := output.NewOutput("", req.Raw, id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid output config: " + err.Error()})
	}
	project.GlobalProject.Outputs[id] = newOut
	common.GlobalMu.Lock()
	common.AllOutputsRawConfig[id] = req.Raw
	common.GlobalMu.Unlock()

	return c.JSON(http.StatusOK, map[string]string{"message": "output updated successfully"})
}

// Update an existing project
func updateProject(c echo.Context) error {
	id := c.Param("id")
	var req struct {
		Raw string `json:"raw"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Raw == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "raw is required"})
	}
	_, ok := project.GlobalProject.Projects[id]
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	// Write config file
	if cluster.IsLeader {
		if err := common.WriteConfigFile("project", id, req.Raw); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write config file"})
		}
		body, _ := json.Marshal(map[string]string{"id": id, "raw": req.Raw})
		go syncToFollowers("PUT", "/project/"+id, body)
	}

	// Hot reload
	newPrj, err := project.NewProject("", req.Raw, id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid project config: " + err.Error()})
	}
	project.GlobalProject.Projects[id] = newPrj
	common.GlobalMu.Lock()
	common.AllProjectRawConfig[id] = req.Raw
	common.GlobalMu.Unlock()

	return c.JSON(http.StatusOK, map[string]string{"message": "project updated successfully"})
}

func ServerStart(listener string) error {
	e := echo.New()
	e.HideBanner = true

	// Add CORS middleware with more permissive configuration
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"*"},          // Allow all origins
		AllowHeaders:     []string{"*", "token"}, // Allow all headers and explicitly allow token
		AllowMethods:     []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowCredentials: true,                       // Allow credentials
		ExposeHeaders:    []string{"Content-Length"}, // Expose these headers
		MaxAge:           86400,                      // Cache preflight requests for 24 hours
	}))

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Health check
	e.GET("/ping", ping)

	// Project endpoints
	e.GET("/project", getProjects, TokenAuthMiddleware)
	e.GET("/project/:id", getProject, TokenAuthMiddleware)
	e.POST("/project", createProject, TokenAuthMiddleware)
	e.DELETE("/project/:id", deleteProject, TokenAuthMiddleware)
	e.POST("/project/start", StartProject, TokenAuthMiddleware)
	e.POST("/project/stop", StopProject, TokenAuthMiddleware)
	e.PUT("/project/:id", updateProject, TokenAuthMiddleware)

	// Ruleset endpoints
	e.GET("/ruleset", getRulesets, TokenAuthMiddleware)
	e.GET("/ruleset/:id", getRuleset, TokenAuthMiddleware)
	e.POST("/ruleset", createRuleset, TokenAuthMiddleware)
	e.PUT("/ruleset/:id", updateRuleset, TokenAuthMiddleware)
	e.DELETE("/ruleset/:id", deleteRuleset, TokenAuthMiddleware)

	// Input endpoints
	e.GET("/input", getInputs, TokenAuthMiddleware)
	e.GET("/input/:id", getInput, TokenAuthMiddleware)
	e.POST("/input", createInput, TokenAuthMiddleware)
	e.DELETE("/input/:id", deleteInput, TokenAuthMiddleware)
	e.PUT("/input/:id", updateInput, TokenAuthMiddleware)

	// Output endpoints
	e.GET("/output", getOutputs, TokenAuthMiddleware)
	e.GET("/output/:id", getOutput, TokenAuthMiddleware)
	e.POST("/output", createOutput, TokenAuthMiddleware)
	e.DELETE("/output/:id", deleteOutput, TokenAuthMiddleware)
	e.PUT("/output/:id", updateOutput, TokenAuthMiddleware)

	// Plugin endpoints
	e.GET("/plugin", getPlugins, TokenAuthMiddleware)
	e.GET("/plugin/:name", getPlugin, TokenAuthMiddleware)
	e.POST("/plugin", createPlugin, TokenAuthMiddleware)
	e.PUT("/plugin/:name", updatePlugin, TokenAuthMiddleware)
	e.DELETE("/plugin/:name", deletePlugin, TokenAuthMiddleware)

	// Metrics endpoints
	e.GET("/metrics", getMetrics)
	e.GET("/metrics/:id", getProjectMetrics)

	// Redis monitoring endpoints
	e.GET("/redis/metrics", getRedisMetrics)

	// Cluster endpoints
	e.POST("/cluster/heartbeat", handleHeartbeat, TokenAuthMiddleware)
	e.GET("/cluster/status", getClusterStatus, TokenAuthMiddleware)

	// Config endpoints
	e.GET("/config/download", downloadConfig, TokenAuthMiddleware)

	// HubConfig
	e.GET("/leader_config", leaderConfig, TokenAuthMiddleware)

	// Token check
	e.GET("/token/check", tokenCheck)

	// Component sync endpoint
	e.POST("/component/sync", handleComponentSync, TokenAuthMiddleware)

	// Hub cluster
	e.GET("/cluster_info", getCluster, TokenAuthMiddleware)

	if err := e.Start(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
