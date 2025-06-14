package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/local_plugin"
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

	for id := range p.ProjectsNew {
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

	p_raw, ok := project.GlobalProject.ProjectsNew[id]
	if ok {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":     id,
			"status": project.ProjectStatusStopped,
			"raw":    p_raw,
		})
	}

	p := project.GlobalProject.Projects[id]
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

	for id := range p.RulesetsNew {
		rulesets = append(rulesets, map[string]interface{}{
			"id": id,
		})
	}
	return c.JSON(http.StatusOK, rulesets)
}

// getRuleset returns details of a specific ruleset
func getRuleset(c echo.Context) error {
	id := c.Param("id")

	r_raw, ok := project.GlobalProject.RulesetsNew[id]
	if ok {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":  id,
			"raw": r_raw,
		})
	}

	r := project.GlobalProject.Rulesets[id]

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

	for id := range p.InputsNew {
		inputs = append(inputs, map[string]interface{}{
			"id": id,
		})
	}
	return c.JSON(http.StatusOK, inputs)
}

// getInput returns details of a specific input component
func getInput(c echo.Context) error {
	id := c.Param("id")
	in_raw, ok := project.GlobalProject.InputsNew[id]
	if ok {
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

	for name := range plugin.PluginsNew {
		plugins = append(plugins, map[string]interface{}{
			"name": name,
		})
	}
	return c.JSON(http.StatusOK, plugins)
}

func getPlugin(c echo.Context) error {
	name := c.Param("name")

	// 首先检查是否有临时文件
	p_raw, ok := plugin.PluginsNew[name]
	if ok {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"name": name,
			"raw":  p_raw,
		})
	}

	// 如果没有临时文件，检查正式文件
	if p, ok := plugin.Plugins[name]; ok {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"name": p.Name,
			"raw":  string(p.Payload),
		})
	}

	// 如果内存中没有，尝试从文件系统直接读取
	tempPath, tempExists := GetComponentPath("plugin", name, true)
	if tempExists {
		content, err := ReadComponent(tempPath)
		if err == nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"name": name,
				"raw":  content,
			})
		}
	}

	formalPath, formalExists := GetComponentPath("plugin", name, false)
	if formalExists {
		content, err := ReadComponent(formalPath)
		if err == nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"name": name,
				"raw":  content,
			})
		}
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

	for id := range p.OutputsNew {
		outputs = append(outputs, map[string]interface{}{
			"id": id,
		})
	}
	return c.JSON(http.StatusOK, outputs)
}

// getOutput returns details of a specific output component
func getOutput(c echo.Context) error {
	id := c.Param("id")
	out_raw, ok := project.GlobalProject.ProjectsNew[id]
	if ok {
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
	p := project.GlobalProject.Projects[id]
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

// StopProject stops a running project
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

	// 增强对ID的验证
	if request.ID == "" || strings.TrimSpace(request.ID) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id cannot be empty"})
	}

	// 规范化ID，去除首尾空格
	request.ID = strings.TrimSpace(request.ID)

	filtPath, exist := GetComponentPath(componentType, request.ID, true)
	if exist {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "this file already exists"})
	}

	_, exist = GetComponentPath(componentType, request.ID, false)
	if exist {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "this file already exists"})
	}

	switch componentType {
	case "plugin":
		request.Raw = NewPluginData
	case "input":
		request.Raw = NewInputData
	case "output":
		request.Raw = NewOutputData
	case "ruleset":
		request.Raw = NewRulesetData
	case "project":
		request.Raw = NewProjectData
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

	// 检查组件是否存在
	var componentExists bool
	var componentPath string
	var tempPath string
	var globalMapToUpdate map[string]string

	// 检查是否存在临时文件或正式文件
	tempPath, tempExists := GetComponentPath(componentType, id, true)         // .new 文件
	componentPath, formalExists := GetComponentPath(componentType, id, false) // 正式文件

	// 根据组件类型获取相应的全局映射
	switch componentType {
	case "ruleset":
		_, componentExists = project.GlobalProject.Rulesets[id]
		globalMapToUpdate = common.AllRulesetsRawConfig
	case "input":
		_, componentExists = project.GlobalProject.Inputs[id]
		globalMapToUpdate = common.AllInputsRawConfig
	case "output":
		_, componentExists = project.GlobalProject.Outputs[id]
		globalMapToUpdate = common.AllOutputsRawConfig
	case "project":
		_, componentExists = project.GlobalProject.Projects[id]
		globalMapToUpdate = common.AllProjectRawConfig
	case "plugin":
		_, componentExists = plugin.Plugins[id]
		globalMapToUpdate = common.AllPluginsRawConfig
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unknown component type"})
	}

	// 如果是正式组件（非临时文件），检查是否在使用中
	if componentExists {
		// 检查组件是否被任何项目使用
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

		// 从全局映射中删除组件
		switch componentType {
		case "ruleset":
			delete(project.GlobalProject.Rulesets, id)
		case "input":
			delete(project.GlobalProject.Inputs, id)
		case "output":
			delete(project.GlobalProject.Outputs, id)
		case "project":
			delete(project.GlobalProject.Projects, id)
		case "plugin":
			delete(plugin.Plugins, id)
		}
	}

	// 如果是leader节点，删除文件并通知follower
	if cluster.IsLeader {
		// 删除临时文件（如果存在）
		if tempExists {
			if err := os.Remove(tempPath); err != nil {
				logger.Error("failed to delete temp file", "path", tempPath, "error", err)
			}
		}

		// 删除正式文件（如果存在）
		if formalExists {
			if err := os.Remove(componentPath); err != nil {
				logger.Error("failed to delete component file", "path", componentPath, "error", err)
			}
			// 只有删除正式文件时才通知follower
			go syncToFollowers("DELETE", "/"+componentType+"/"+id, nil)
		}
	} else {
		// 如果是follower节点，只需要从内存中删除配置
		common.GlobalMu.Lock()
		delete(globalMapToUpdate, id)
		common.GlobalMu.Unlock()
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("%s deleted successfully", componentType),
	})
}

// Component deletion handlers
func deleteRuleset(c echo.Context) error {
	return deleteComponent("ruleset", c)
}

func deleteInput(c echo.Context) error {
	return deleteComponent("input", c)
}

func deleteOutput(c echo.Context) error {
	return deleteComponent("output", c)
}

func deleteProject(c echo.Context) error {
	return deleteComponent("project", c)
}

func deletePlugin(c echo.Context) error {
	// Plugins use name instead of id as parameter
	name := c.Param("name")
	if name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required"})
	}

	// Set name parameter as id parameter to reuse deleteComponent function
	c.SetParamNames("id")
	c.SetParamValues(name)

	return deleteComponent("plugin", c)
}

// Generic sync method: only sync to healthy follower nodes
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
				break // Success
			}
			time.Sleep(2 * time.Second)
		}
	}
}

// updateRuleset handler function
func updateRuleset(c echo.Context) error {
	return updateComponent("ruleset", c)
}

// Update an existing plugin
func updatePlugin(c echo.Context) error {
	return updateComponent("plugin", c)
}

// Update an existing input
func updateInput(c echo.Context) error {
	return updateComponent("input", c)
}

// Update an existing output
func updateOutput(c echo.Context) error {
	return updateComponent("output", c)
}

// Update an existing project
func updateProject(c echo.Context) error {
	return updateComponent("project", c)
}

func updateComponent(componentType string, c echo.Context) error {
	id := c.Param("id")
	var req struct {
		Raw string `json:"raw"`
	}

	// Parse request body
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
	}

	// 先检查是否存在临时文件
	componentPath, exist := GetComponentPath(componentType, id, true)
	if !exist {
		// 如果临时文件不存在，检查是否存在正式文件
		componentPath, exist = GetComponentPath(componentType, id, false)
		if !exist {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "component config not found"})
		}

		// 如果只有正式文件，创建临时文件
		tempPath, _ := GetComponentPath(componentType, id, true)
		componentPath = tempPath
	}

	err := WriteComponentFile(componentPath, req.Raw)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write config file: " + err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "component updated successfully"})
}

// Verify component configuration
func verifyComponent(c echo.Context) error {
	componentType := c.Param("type")
	id := c.Param("id")
	var req struct {
		Raw string `json:"raw"`
	}

	// Parse request body
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
	}

	// 如果请求中没有提供raw内容，尝试从临时文件或正式文件中读取
	if req.Raw == "" {
		// 先检查临时文件
		tempPath, tempExists := GetComponentPath(componentType, id, true)
		if tempExists {
			content, err := ReadComponent(tempPath)
			if err == nil {
				req.Raw = content
			}
		}

		// 如果临时文件不存在或读取失败，检查正式文件
		if req.Raw == "" {
			formalPath, formalExists := GetComponentPath(componentType, id, false)
			if formalExists {
				content, err := ReadComponent(formalPath)
				if err == nil {
					req.Raw = content
				}
			}
		}
	}

	var err error
	switch componentType {
	case "input":
		err = input.Verify("", req.Raw)
	case "output":
		err = output.Verify("", req.Raw)
	case "ruleset":
		err = rules_engine.Verify("", req.Raw)
	case "project":
		err = project.Verify("", req.Raw)
	case "plugin":
		err = plugin.Verify("", req.Raw, id)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported component type"})
	}

	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"valid": true,
	})
}

// connectCheck checks the connection status of an input or output component's client
func connectCheck(c echo.Context) error {
	componentType := c.Param("type")
	id := c.Param("id")

	// Check if component type is valid
	if componentType != "inputs" && componentType != "outputs" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid component type. Must be 'inputs' or 'outputs'",
		})
	}

	// Check input component client connection
	if componentType == "inputs" {
		_, existsNew := project.GlobalProject.InputsNew[id]
		inputComp := project.GlobalProject.Inputs[id]

		if !existsNew && inputComp == nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Input component not found",
			})
		}

		// Initialize result
		result := map[string]interface{}{
			"status":  "success",
			"message": "Connection check successful",
			"details": map[string]interface{}{
				"client_type":       "",
				"connection_status": "unknown",
				"connection_info":   map[string]interface{}{},
				"connection_errors": []map[string]interface{}{},
			},
		}

		// If the input is in pending changes, we can't check its connection
		if existsNew {
			result["status"] = "warning"
			result["message"] = "Component has pending changes, cannot check connection"
			return c.JSON(http.StatusOK, result)
		}

		// Check connection based on input type
		switch inputComp.Type {
		case input.InputTypeKafka:
			result["details"].(map[string]interface{})["client_type"] = "kafka"

			// Check if Kafka consumer is initialized
			if inputComp.Config != nil && inputComp.Config.Kafka != nil {
				connectionInfo := map[string]interface{}{
					"brokers": inputComp.Config.Kafka.Brokers,
					"group":   inputComp.Config.Kafka.Group,
					"topic":   inputComp.Config.Kafka.Topic,
				}
				result["details"].(map[string]interface{})["connection_info"] = connectionInfo

				// Check if consumer is running
				if inputComp.GetConsumeQPS() > 0 {
					result["details"].(map[string]interface{})["connection_status"] = "active"
					result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
						"consume_qps":   inputComp.GetConsumeQPS(),
						"consume_total": inputComp.GetConsumeTotal(),
					}
				} else {
					// Consumer exists but no messages being processed
					if inputComp.GetConsumeTotal() > 0 {
						result["status"] = "warning"
						result["message"] = "Connection established but no recent activity"
						result["details"].(map[string]interface{})["connection_status"] = "idle"
						result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
							"consume_total": inputComp.GetConsumeTotal(),
						}
					} else {
						// No messages processed yet
						result["status"] = "warning"
						result["message"] = "Connection established but no messages processed"
						result["details"].(map[string]interface{})["connection_status"] = "connected"
					}
				}
			} else {
				result["status"] = "error"
				result["message"] = "Kafka configuration missing"
				result["details"].(map[string]interface{})["connection_status"] = "not_configured"
				result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
					{"message": "Kafka configuration is incomplete or missing", "severity": "error"},
				}
			}

		case input.InputTypeAliyunSLS:
			result["details"].(map[string]interface{})["client_type"] = "aliyun_sls"

			// Check if SLS consumer is initialized
			if inputComp.Config != nil && inputComp.Config.AliyunSLS != nil {
				connectionInfo := map[string]interface{}{
					"endpoint":       inputComp.Config.AliyunSLS.Endpoint,
					"project":        inputComp.Config.AliyunSLS.Project,
					"logstore":       inputComp.Config.AliyunSLS.Logstore,
					"consumer_group": inputComp.Config.AliyunSLS.ConsumerGroupName,
				}
				result["details"].(map[string]interface{})["connection_info"] = connectionInfo

				// Check if consumer is running
				if inputComp.GetConsumeQPS() > 0 {
					result["details"].(map[string]interface{})["connection_status"] = "active"
					result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
						"consume_qps":   inputComp.GetConsumeQPS(),
						"consume_total": inputComp.GetConsumeTotal(),
					}
				} else {
					// Consumer exists but no messages being processed
					if inputComp.GetConsumeTotal() > 0 {
						result["status"] = "warning"
						result["message"] = "Connection established but no recent activity"
						result["details"].(map[string]interface{})["connection_status"] = "idle"
						result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
							"consume_total": inputComp.GetConsumeTotal(),
						}
					} else {
						// No messages processed yet
						result["status"] = "warning"
						result["message"] = "Connection established but no messages processed"
						result["details"].(map[string]interface{})["connection_status"] = "connected"
					}
				}
			} else {
				result["status"] = "error"
				result["message"] = "Aliyun SLS configuration missing"
				result["details"].(map[string]interface{})["connection_status"] = "not_configured"
				result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
					{"message": "Aliyun SLS configuration is incomplete or missing", "severity": "error"},
				}
			}
		default:
			result["status"] = "error"
			result["message"] = "Unsupported input type"
			result["details"].(map[string]interface{})["client_type"] = string(inputComp.Type)
			result["details"].(map[string]interface{})["connection_status"] = "unsupported"
		}

		return c.JSON(http.StatusOK, result)
	} else if componentType == "outputs" {
		_, existsNew := project.GlobalProject.OutputsNew[id]
		outputComp := project.GlobalProject.Outputs[id]

		if !existsNew && outputComp == nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Output component not found",
			})
		}

		// Initialize result
		result := map[string]interface{}{
			"status":  "success",
			"message": "Connection check successful",
			"details": map[string]interface{}{
				"client_type":       "",
				"connection_status": "unknown",
				"connection_info":   map[string]interface{}{},
				"connection_errors": []map[string]interface{}{},
			},
		}

		// If the output is in pending changes, we can't check its connection
		if existsNew {
			result["status"] = "warning"
			result["message"] = "Component has pending changes, cannot check connection"
			return c.JSON(http.StatusOK, result)
		}

		// Check connection based on output type
		switch outputComp.Type {
		case output.OutputTypeKafka:
			result["details"].(map[string]interface{})["client_type"] = "kafka"

			// Check if Kafka producer is initialized
			if outputComp.Config != nil && outputComp.Config.Kafka != nil {
				connectionInfo := map[string]interface{}{
					"brokers": outputComp.Config.Kafka.Brokers,
					"topic":   outputComp.Config.Kafka.Topic,
				}
				result["details"].(map[string]interface{})["connection_info"] = connectionInfo

				// Check if producer is running
				if outputComp.GetProduceQPS() > 0 {
					result["details"].(map[string]interface{})["connection_status"] = "active"
					result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
						"produce_qps":   outputComp.GetProduceQPS(),
						"produce_total": outputComp.GetProduceTotal(),
					}
				} else {
					// Producer exists but no messages being sent
					if outputComp.GetProduceTotal() > 0 {
						result["status"] = "warning"
						result["message"] = "Connection established but no recent activity"
						result["details"].(map[string]interface{})["connection_status"] = "idle"
						result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
							"produce_total": outputComp.GetProduceTotal(),
						}
					} else {
						// No messages sent yet
						result["status"] = "warning"
						result["message"] = "Connection established but no messages sent"
						result["details"].(map[string]interface{})["connection_status"] = "connected"
					}
				}
			} else {
				result["status"] = "error"
				result["message"] = "Kafka configuration missing"
				result["details"].(map[string]interface{})["connection_status"] = "not_configured"
				result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
					{"message": "Kafka configuration is incomplete or missing", "severity": "error"},
				}
			}

		case output.OutputTypeElasticsearch:
			result["details"].(map[string]interface{})["client_type"] = "elasticsearch"

			// Check if Elasticsearch producer is initialized
			if outputComp.Config != nil && outputComp.Config.Elasticsearch != nil {
				connectionInfo := map[string]interface{}{
					"hosts": outputComp.Config.Elasticsearch.Hosts,
					"index": outputComp.Config.Elasticsearch.Index,
				}
				result["details"].(map[string]interface{})["connection_info"] = connectionInfo

				// Check if producer is running
				if outputComp.GetProduceQPS() > 0 {
					result["details"].(map[string]interface{})["connection_status"] = "active"
					result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
						"produce_qps":   outputComp.GetProduceQPS(),
						"produce_total": outputComp.GetProduceTotal(),
					}
				} else {
					// Producer exists but no messages being sent
					if outputComp.GetProduceTotal() > 0 {
						result["status"] = "warning"
						result["message"] = "Connection established but no recent activity"
						result["details"].(map[string]interface{})["connection_status"] = "idle"
						result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
							"produce_total": outputComp.GetProduceTotal(),
						}
					} else {
						// No messages sent yet
						result["status"] = "warning"
						result["message"] = "Connection established but no messages sent"
						result["details"].(map[string]interface{})["connection_status"] = "connected"
					}
				}
			} else {
				result["status"] = "error"
				result["message"] = "Elasticsearch configuration missing"
				result["details"].(map[string]interface{})["connection_status"] = "not_configured"
				result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
					{"message": "Elasticsearch configuration is incomplete or missing", "severity": "error"},
				}
			}

		case output.OutputTypeAliyunSLS:
			result["details"].(map[string]interface{})["client_type"] = "aliyun_sls"

			// Check if SLS producer is initialized
			if outputComp.Config != nil && outputComp.Config.AliyunSLS != nil {
				connectionInfo := map[string]interface{}{
					"endpoint": outputComp.Config.AliyunSLS.Endpoint,
					"project":  outputComp.Config.AliyunSLS.Project,
					"logstore": outputComp.Config.AliyunSLS.Logstore,
				}
				result["details"].(map[string]interface{})["connection_info"] = connectionInfo

				// Check if producer is running
				if outputComp.GetProduceQPS() > 0 {
					result["details"].(map[string]interface{})["connection_status"] = "active"
					result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
						"produce_qps":   outputComp.GetProduceQPS(),
						"produce_total": outputComp.GetProduceTotal(),
					}
				} else {
					// Producer exists but no messages being sent
					if outputComp.GetProduceTotal() > 0 {
						result["status"] = "warning"
						result["message"] = "Connection established but no recent activity"
						result["details"].(map[string]interface{})["connection_status"] = "idle"
						result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
							"produce_total": outputComp.GetProduceTotal(),
						}
					} else {
						// No messages sent yet
						result["status"] = "warning"
						result["message"] = "Connection established but no messages sent"
						result["details"].(map[string]interface{})["connection_status"] = "connected"
					}
				}
			} else {
				result["status"] = "error"
				result["message"] = "Aliyun SLS configuration missing"
				result["details"].(map[string]interface{})["connection_status"] = "not_configured"
				result["details"].(map[string]interface{})["connection_errors"] = []map[string]interface{}{
					{"message": "Aliyun SLS configuration is incomplete or missing", "severity": "error"},
				}
			}

		case output.OutputTypePrint:
			result["details"].(map[string]interface{})["client_type"] = "print"
			result["details"].(map[string]interface{})["connection_status"] = "always_connected"
			result["details"].(map[string]interface{})["connection_info"] = map[string]interface{}{
				"type": "console_output",
			}

			// Check if producer is running
			if outputComp.GetProduceQPS() > 0 {
				result["details"].(map[string]interface{})["metrics"] = map[string]interface{}{
					"produce_qps":   outputComp.GetProduceQPS(),
					"produce_total": outputComp.GetProduceTotal(),
				}
			}

		default:
			result["status"] = "error"
			result["message"] = "Unsupported output type"
			result["details"].(map[string]interface{})["client_type"] = string(outputComp.Type)
			result["details"].(map[string]interface{})["connection_status"] = "unsupported"
		}

		return c.JSON(http.StatusOK, result)
	}

	return c.JSON(http.StatusInternalServerError, map[string]string{
		"error": "Unknown error occurred",
	})
}

// testPlugin tests a plugin with provided arguments
func testPlugin(c echo.Context) error {
	name := c.Param("name")

	// Parse request body
	var req struct {
		Args []interface{} `json:"args"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
			"result":  nil,
		})
	}

	// Check if plugin exists
	p, ok := plugin.Plugins[name]
	if !ok {
		_, existsNew := plugin.PluginsNew[name]
		if existsNew {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success": false,
				"error":   "Plugin has pending changes, cannot test",
				"result":  nil,
			})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Plugin not found: " + name,
			"result":  nil,
		})
	}

	// Determine plugin type and execute
	var result interface{}
	var success bool
	var errMsg string

	switch p.Type {
	case plugin.LOCAL_PLUGIN:
		// Check if it's a boolean result plugin
		if f, ok := local_plugin.LocalPluginBoolRes[name]; ok {
			// 确保参数类型正确
			if len(req.Args) > 0 {
				for i, arg := range req.Args {
					if arg == nil {
						req.Args[i] = ""
					}
				}
			}

			boolResult, err := f(req.Args...)
			result = boolResult
			success = err == nil
			if err != nil {
				errMsg = fmt.Sprintf("Plugin execution failed: %v", err)
			}
		} else if f, ok := local_plugin.LocalPluginInterfaceAndBoolRes[name]; ok {
			// It's an interface result plugin
			interfaceResult, boolResult, err := f(req.Args...)
			result = map[string]interface{}{
				"result":  interfaceResult,
				"success": boolResult,
			}
			success = err == nil
			if err != nil {
				errMsg = fmt.Sprintf("Plugin execution failed: %v", err)
			}
		} else {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success": false,
				"error":   "Plugin exists but function not found",
				"result":  nil,
			})
		}
	case plugin.YAEGI_PLUGIN:
		// For Yaegi plugins, we need to determine the return type
		// We need to catch panics that might occur during plugin execution
		defer func() {
			if r := recover(); r != nil {
				success = false
				errMsg = fmt.Sprintf("Plugin execution panicked: %v", r)
				result = nil
			}
		}()

		// 检查参数类型
		for i, arg := range req.Args {
			if arg == nil {
				req.Args[i] = "" // 将nil转换为空字符串
			} else {
				// 尝试将参数转换为字符串
				switch v := arg.(type) {
				case float64:
					// JSON解析可能将数字解析为float64
					if v == float64(int(v)) {
						// 如果是整数，转换为整数字符串
						req.Args[i] = fmt.Sprintf("%d", int(v))
					} else {
						req.Args[i] = fmt.Sprintf("%g", v)
					}
				case bool:
					req.Args[i] = fmt.Sprintf("%t", v)
				case map[string]interface{}, []interface{}:
					jsonBytes, err := json.Marshal(v)
					if err != nil {
						req.Args[i] = fmt.Sprintf("%v", v)
					} else {
						req.Args[i] = string(jsonBytes)
					}
				case string:
					// 已经是字符串，不需要转换
				default:
					req.Args[i] = fmt.Sprintf("%v", v)
				}
			}
		}

		// 执行插件
		boolResult := p.FuncEvalCheckNode(req.Args...)
		result = boolResult
		success = true

		// Note: For Yaegi plugins, errors are logged but not returned
	default:
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Unknown plugin type",
			"result":  nil,
		})
	}

	// Return the result
	if errMsg != "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   errMsg,
			"result":  result,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": success,
		"result":  result,
	})
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

	// Verify component configuration
	e.POST("/verify/:type/:id", verifyComponent, TokenAuthMiddleware)

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

	// Get all pending changes
	e.GET("/pending-changes", GetPendingChanges, TokenAuthMiddleware)

	// Apply all pending changes
	e.POST("/apply-changes", ApplyPendingChanges, TokenAuthMiddleware)

	// Apply a single pending change
	e.POST("/apply-single-change", ApplySingleChange, TokenAuthMiddleware)

	// Restart all projects
	e.POST("/restart-all-projects", RestartAllProjects, TokenAuthMiddleware)

	// Create temporary file for editing
	e.POST("/temp-file/:type/:id", CreateTempFile, TokenAuthMiddleware)

	// Connect check
	e.GET("/connect-check/:type/:id", connectCheck, TokenAuthMiddleware)

	// Test plugin
	e.POST("/test-plugin/:name", testPlugin, TokenAuthMiddleware)

	if err := e.Start(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
