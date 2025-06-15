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
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/labstack/echo/v4"
)

// Constants for file extensions
const (
	RULESET_EXT     = ".xml"
	RULESET_EXT_NEW = ".xml.new"

	PLUGIN_EXT     = ".go"
	PLUGIN_EXT_NEW = ".go.new"

	EXT     = ".yaml"
	EXT_NEW = ".yaml.new"
)

// Default templates for new components
const NewPluginData = `package plugin

import (
	"errors"
	"strings"
)

func Eval(data string) (bool, error) {
	if data == "" {
		return false, errors.New("")
	}

	if strings.HasSuffix(data, "something") {
		return true, nil
	} else {
		return false, nil
	}
}`

const NewInputData = `type: kafka
kafka:
  brokers:
    - 127.0.0.1:9092
  topic: test-topic
  group: test
  
#type: aliyun_sls
#aliyun_sls:
#  endpoint: "cn-beijing.log.aliyuncs.com"
#  access_key_id: "xx"
#  access_key_secret: "xx"
#  project: "xx"
#  logstore: "xx"
#  consumer_group_name: "xx"
#  consumer_name: "xx"
#  cursor_position: "BEGIN_CURSOR"
#  query: "xx"`

const NewOutputData = `name: kafka_output_demo
type: kafka
kafka:
  brokers:
    - "192.168.27.130:9092"
  topic: "kafka_output_demo"`

const NewRulesetData = `<root name="test2" type="DETECTION">
    <rule id="reverse_shell_01" name="测试" author="test">
        <filter field="data_type">_$data_type</filter>
        <checklist condition="a and c and d and e">
            <node id="a" type="REGEX" field="exe">testcases</node>
            <node id="c" type="INCL" field="exe" logic="OR" delimiter="|">abc|edf</node>
            <node id="d" type="EQU" field="sessionid">_$sessionid</node>
        </checklist>
        <append field_name="abc">123</append>
        <del>exe,argv</del>
    </rule>
</root>`

const NewProjectData = `content: |
  INPUT.demo -> OUTPUT.demo`

// Utility functions
func GetExt(componentType string, new bool) string {
	if componentType == "ruleset" {
		if new {
			return RULESET_EXT_NEW
		} else {
			return RULESET_EXT
		}
	} else if componentType == "plugin" {
		if new {
			return PLUGIN_EXT_NEW
		} else {
			return PLUGIN_EXT
		}
	} else {
		if new {
			return EXT_NEW
		} else {
			return EXT
		}
	}
}

func GetComponentPath(componentType string, id string, new bool) (string, bool) {
	dirPath := path.Join(common.Config.ConfigRoot, componentType)
	filePath := path.Join(dirPath, id+GetExt(componentType, new))

	//check if dir exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return filePath, false
		}
	}

	_, err := os.Stat(filePath)
	exists := !os.IsNotExist(err)

	return filePath, exists
}

func WriteComponentFile(path string, content string) error {
	// Check if this is a temporary file (.new)
	if strings.HasSuffix(path, ".new") {
		// Extract component type and ID from path
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			componentType := parts[len(parts)-2] // Second to last part is component type
			filename := parts[len(parts)-1]      // Last part is filename

			var id string
			// Determine ID based on file extension
			if strings.HasSuffix(filename, ".xml.new") {
				id = filename[:len(filename)-8] // Remove .xml.new
			} else if strings.HasSuffix(filename, ".go.new") {
				id = filename[:len(filename)-7] // Remove .go.new
			} else if strings.HasSuffix(filename, ".yaml.new") {
				id = filename[:len(filename)-9] // Remove .yaml.new
			}

			// If it's a temporary file, also update the in-memory copy
			switch componentType {
			case "input":
				if project.GlobalProject.InputsNew == nil {
					project.GlobalProject.InputsNew = make(map[string]string)
				}
				project.GlobalProject.InputsNew[id] = content
			case "output":
				if project.GlobalProject.OutputsNew == nil {
					project.GlobalProject.OutputsNew = make(map[string]string)
				}
				project.GlobalProject.OutputsNew[id] = content
			case "ruleset":
				if project.GlobalProject.RulesetsNew == nil {
					project.GlobalProject.RulesetsNew = make(map[string]string)
				}
				project.GlobalProject.RulesetsNew[id] = content
			case "project":
				if project.GlobalProject.ProjectsNew == nil {
					project.GlobalProject.ProjectsNew = make(map[string]string)
				}
				project.GlobalProject.ProjectsNew[id] = content
			case "plugin":
				if plugin.PluginsNew == nil {
					plugin.PluginsNew = make(map[string]string)
				}
				plugin.PluginsNew[id] = content
			}
		}
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	return nil
}

func ReadComponent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	} else {
		return string(data), err
	}
}

// Type definitions
type FileChecksum struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

type CtrlProjectRequest struct {
	ProjectID string `json:"project_id"`
}

func getProjects(c echo.Context) error {
	p := project.GlobalProject
	result := make([]map[string]interface{}, 0, 0)

	// Create a map to track processed IDs
	processedIDs := make(map[string]bool)

	for _, proj := range p.Projects {
		// Check if there is a temporary file
		_, hasTemp := p.ProjectsNew[proj.Id]

		result = append(result, map[string]interface{}{
			"id":      proj.Id,
			"status":  proj.Status,
			"hasTemp": hasTemp,
		})
		processedIDs[proj.Id] = true
	}

	// Add components that only exist in temporary files
	for id := range p.ProjectsNew {
		if !processedIDs[id] {
			result = append(result, map[string]interface{}{
				"id":      id,
				"status":  project.ProjectStatusStopped,
				"hasTemp": true,
			})
		}
	}
	return c.JSON(http.StatusOK, result)
}

func getProject(c echo.Context) error {
	id := c.Param("id")

	p_raw, ok := project.GlobalProject.ProjectsNew[id]
	if ok {
		tempPath, _ := GetComponentPath("project", id, true)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":     id,
			"status": project.ProjectStatusStopped,
			"raw":    p_raw,
			"path":   tempPath,
		})
	}

	p := project.GlobalProject.Projects[id]
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	formalPath, _ := GetComponentPath("project", id, false)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":     p.Id,
		"status": p.Status,
		"raw":    p.Config.RawConfig,
		"path":   formalPath,
	})
}

func getRulesets(c echo.Context) error {
	p := project.GlobalProject
	rulesets := make([]map[string]interface{}, 0)

	// Create a map to track processed IDs
	processedIDs := make(map[string]bool)

	for _, r := range p.Rulesets {
		// Check if there is a temporary file
		_, hasTemp := p.RulesetsNew[r.RulesetID]

		rulesets = append(rulesets, map[string]interface{}{
			"id":      r.RulesetID,
			"hasTemp": hasTemp,
		})
		processedIDs[r.RulesetID] = true
	}

	// Add components that only exist in temporary files
	for id := range p.RulesetsNew {
		if !processedIDs[id] {
			rulesets = append(rulesets, map[string]interface{}{
				"id":      id,
				"hasTemp": true,
			})
		}
	}
	return c.JSON(http.StatusOK, rulesets)
}

func getRuleset(c echo.Context) error {
	id := c.Param("id")

	r_raw, ok := project.GlobalProject.RulesetsNew[id]
	if ok {
		tempPath, _ := GetComponentPath("ruleset", id, true)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":   id,
			"raw":  r_raw,
			"path": tempPath,
		})
	}

	r := project.GlobalProject.Rulesets[id]

	if r != nil {
		formalPath, _ := GetComponentPath("ruleset", id, false)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":   r.RulesetID,
			"raw":  r.RawConfig,
			"path": formalPath,
		})
	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
}

func getInputs(c echo.Context) error {
	p := project.GlobalProject
	inputs := make([]map[string]interface{}, 0)

	// Create a map to track processed IDs
	processedIDs := make(map[string]bool)

	for _, in := range p.Inputs {
		// Check if there is a temporary file
		_, hasTemp := p.InputsNew[in.Id]

		inputs = append(inputs, map[string]interface{}{
			"id":      in.Id,
			"hasTemp": hasTemp,
		})
		processedIDs[in.Id] = true
	}

	// Add components that only exist in temporary files
	for id := range p.InputsNew {
		if !processedIDs[id] {
			inputs = append(inputs, map[string]interface{}{
				"id":      id,
				"hasTemp": true,
			})
		}
	}
	return c.JSON(http.StatusOK, inputs)
}

func getInput(c echo.Context) error {
	id := c.Param("id")
	in_raw, ok := project.GlobalProject.InputsNew[id]
	if ok {
		tempPath, _ := GetComponentPath("input", id, true)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":   id,
			"raw":  in_raw,
			"path": tempPath,
		})
	}

	in := project.GlobalProject.Inputs[id]

	if in != nil {
		formalPath, _ := GetComponentPath("input", id, false)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":   in.Id,
			"raw":  in.Config.RawConfig,
			"path": formalPath,
		})

	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "input not found"})
}

func getPlugins(c echo.Context) error {
	plugins := make([]map[string]interface{}, 0)

	// Create a map to track processed names
	processedNames := make(map[string]bool)

	for _, p := range plugin.Plugins {
		// Show all types of plugins, including local and Yaegi plugins
		var pluginType string
		if p.Type == plugin.LOCAL_PLUGIN {
			pluginType = "local"
		} else if p.Type == plugin.YAEGI_PLUGIN {
			pluginType = "yaegi"
		} else {
			pluginType = "unknown"
		}

		// Check if there is a temporary file
		_, hasTemp := plugin.PluginsNew[p.Name]

		plugins = append(plugins, map[string]interface{}{
			"id":      p.Name,     // Use id field for consistency with other components
			"name":    p.Name,     // Keep name for backward compatibility
			"type":    pluginType, // Convert to string type for frontend differentiation
			"hasTemp": hasTemp,
		})
		processedNames[p.Name] = true
	}

	// Add plugins that only exist in temporary files
	for name := range plugin.PluginsNew {
		if !processedNames[name] {
			plugins = append(plugins, map[string]interface{}{
				"id":      name,  // Use id field for consistency with other components
				"name":    name,  // Keep name for backward compatibility
				"type":    "new", // Mark as new plugin
				"hasTemp": true,
			})
		}
	}
	return c.JSON(http.StatusOK, plugins)
}

func getPlugin(c echo.Context) error {
	// Use :id parameter for consistency with other components
	id := c.Param("id")
	if id == "" {
		// Fallback to :name for backward compatibility
		id = c.Param("name")
	}

	// First check if there is a temporary file
	p_raw, ok := plugin.PluginsNew[id]
	if ok {
		tempPath, _ := GetComponentPath("plugin", id, true)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":   id,
			"name": id, // Keep name for backward compatibility
			"raw":  p_raw,
			"type": "new", // Mark as new plugin
			"path": tempPath,
		})
	}

	// If no temporary file, check formal file
	if p, ok := plugin.Plugins[id]; ok {
		var pluginType string
		var rawContent string

		if p.Type == plugin.LOCAL_PLUGIN {
			pluginType = "local"
			// For local plugins, display explanatory information
			rawContent = fmt.Sprintf(`// Built-in Plugin: %s
// This is a built-in plugin that cannot be viewed or edited.
// Built-in plugins are compiled into the application and provide core functionality.`, id)
		} else if p.Type == plugin.YAEGI_PLUGIN {
			pluginType = "yaegi"
			rawContent = string(p.Payload)
		} else {
			pluginType = "unknown"
			rawContent = string(p.Payload)
		}

		formalPath, _ := GetComponentPath("plugin", id, false)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":   p.Name, // Use id field for consistency
			"name": p.Name, // Keep name for backward compatibility
			"raw":  rawContent,
			"type": pluginType, // Add type information
			"path": formalPath,
		})
	}

	// If not in memory, try to read directly from file system
	tempPath, tempExists := GetComponentPath("plugin", id, true)
	if tempExists {
		content, err := ReadComponent(tempPath)
		if err == nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"id":   id, // Use id field for consistency
				"name": id, // Keep name for backward compatibility
				"raw":  content,
				"type": "yaegi", // Plugins in file system default to yaegi type
				"path": tempPath,
			})
		}
	}

	formalPath, formalExists := GetComponentPath("plugin", id, false)
	if formalExists {
		content, err := ReadComponent(formalPath)
		if err == nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"id":   id, // Use id field for consistency
				"name": id, // Keep name for backward compatibility
				"raw":  content,
				"type": "yaegi", // Plugins in file system default to yaegi type
				"path": formalPath,
			})
		}
	}

	return c.JSON(http.StatusNotFound, map[string]string{"error": "plugin not found"})
}

func getOutputs(c echo.Context) error {
	p := project.GlobalProject
	outputs := make([]map[string]interface{}, 0)

	// 创建一个map来跟踪已处理的ID
	processedIDs := make(map[string]bool)

	for _, out := range p.Outputs {
		// 检查是否有临时文件
		_, hasTemp := p.OutputsNew[out.Id]

		outputs = append(outputs, map[string]interface{}{
			"id":      out.Id,
			"hasTemp": hasTemp,
		})
		processedIDs[out.Id] = true
	}

	// 添加只存在于临时文件中的组件
	for id := range p.OutputsNew {
		if !processedIDs[id] {
			outputs = append(outputs, map[string]interface{}{
				"id":      id,
				"hasTemp": true,
			})
		}
	}
	return c.JSON(http.StatusOK, outputs)
}

func getOutput(c echo.Context) error {
	id := c.Param("id")
	out_raw, ok := project.GlobalProject.OutputsNew[id]
	if ok {
		tempPath, _ := GetComponentPath("output", id, true)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":   id,
			"raw":  out_raw,
			"path": tempPath,
		})
	}

	out := project.GlobalProject.Outputs[id]

	if out != nil {
		formalPath, _ := GetComponentPath("output", id, false)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"id":   out.Id,
			"raw":  out.Config.RawConfig,
			"path": formalPath,
		})
	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "output not found"})
}

func createComponent(componentType string, c echo.Context) error {
	var request struct {
		ID  string `json:"id"`
		Raw string `json:"raw"`
	}

	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Enhanced ID validation
	if request.ID == "" || strings.TrimSpace(request.ID) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id cannot be empty"})
	}

	// Normalize ID by trimming spaces
	request.ID = strings.TrimSpace(request.ID)

	// Check file existence without lock (file system operations are atomic)
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

	// Write file without lock (file system operations are atomic)
	err := WriteComponentFile(filtPath, request.Raw)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Only lock for memory operations
	common.GlobalMu.Lock()
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
	common.GlobalMu.Unlock()

	return c.JSON(http.StatusCreated, map[string]string{"message": "created successfully"})
}

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

func deleteComponent(componentType string, c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id is required"})
	}

	// Check file existence without lock (file system operations are atomic)
	tempPath, tempExists := GetComponentPath(componentType, id, true)         // .new 文件
	componentPath, formalExists := GetComponentPath(componentType, id, false) // 正式文件

	// Lock for all memory operations
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	// Check if component exists
	var componentExists bool
	var globalMapToUpdate map[string]string

	// Get corresponding global mapping based on component type
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

	// If it's a formal component (not temporary file), check if it's in use
	if componentExists {
		// Check if component is used by any project
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

		// Remove component from global mapping
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

	// Delete from memory maps (both temporary and formal)
	switch componentType {
	case "ruleset":
		delete(project.GlobalProject.RulesetsNew, id)
	case "input":
		delete(project.GlobalProject.InputsNew, id)
	case "output":
		delete(project.GlobalProject.OutputsNew, id)
	case "project":
		delete(project.GlobalProject.ProjectsNew, id)
	case "plugin":
		delete(plugin.PluginsNew, id)
	}

	// For follower nodes, also delete from global config maps
	if !cluster.IsLeader {
		delete(globalMapToUpdate, id)
	}

	// Unlock before file operations to avoid holding lock during I/O
	common.GlobalMu.Unlock()

	// If it's leader node, delete files and notify followers
	if cluster.IsLeader {
		// Delete temporary file if exists
		if tempExists {
			if err := os.Remove(tempPath); err != nil {
				logger.Error("failed to delete temp file", "path", tempPath, "error", err)
			}
		}

		// Delete formal file if exists
		if formalExists {
			if err := os.Remove(componentPath); err != nil {
				logger.Error("failed to delete component file", "path", componentPath, "error", err)
			}
			// Only notify followers when deleting formal file
			go syncToFollowers("DELETE", "/"+componentType+"/"+id, nil)
		}
	}

	// Re-acquire lock to ensure consistent return
	common.GlobalMu.Lock()

	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("%s deleted successfully", componentType),
	})
}

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
	return deleteComponent("plugin", c)
}

func updateRuleset(c echo.Context) error {
	return updateComponent("ruleset", c)
}

func updatePlugin(c echo.Context) error {
	return updateComponent("plugin", c)
}

func updateInput(c echo.Context) error {
	return updateComponent("input", c)
}

func updateOutput(c echo.Context) error {
	return updateComponent("output", c)
}

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

	// First check if formal file exists
	formalPath, formalExists := GetComponentPath(componentType, id, false)
	if !formalExists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "component config not found"})
	}

	// Read original file content to compare
	originalContent, err := ReadComponent(formalPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read original file: " + err.Error()})
	}

	// Compare content with original file
	newContent := strings.TrimSpace(req.Raw)
	originalContentTrimmed := strings.TrimSpace(originalContent)

	if newContent == originalContentTrimmed {
		// Content is identical, no need to create temporary file
		// If temporary file exists, delete it
		tempPath, tempExists := GetComponentPath(componentType, id, true)
		if tempExists {
			_ = os.Remove(tempPath)
			// Also remove from memory
			switch componentType {
			case "input":
				delete(project.GlobalProject.InputsNew, id)
			case "output":
				delete(project.GlobalProject.OutputsNew, id)
			case "ruleset":
				delete(project.GlobalProject.RulesetsNew, id)
			case "project":
				delete(project.GlobalProject.ProjectsNew, id)
			case "plugin":
				delete(plugin.PluginsNew, id)
			}
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "content identical to original file, no changes needed"})
	}

	// Content is different, create or update temporary file
	tempPath, _ := GetComponentPath(componentType, id, true)
	err = WriteComponentFile(tempPath, req.Raw)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write config file: " + err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "component updated successfully"})
}

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

	// Normalize component type (convert plural to singular)
	singularType := componentType
	switch componentType {
	case "inputs":
		singularType = "input"
	case "outputs":
		singularType = "output"
	case "rulesets":
		singularType = "ruleset"
	case "projects":
		singularType = "project"
	case "plugins":
		singularType = "plugin"
	}

	// If no raw content provided in request, try to read from temporary or formal files
	if req.Raw == "" {
		// First check temporary file
		tempPath, tempExists := GetComponentPath(singularType, id, true)
		if tempExists {
			content, err := ReadComponent(tempPath)
			if err == nil {
				req.Raw = content
			}
		}

		// If temporary file doesn't exist or read failed, check formal file
		if req.Raw == "" {
			formalPath, formalExists := GetComponentPath(singularType, id, false)
			if formalExists {
				content, err := ReadComponent(formalPath)
				if err == nil {
					req.Raw = content
				}
			}
		}
	}

	var err error
	switch singularType {
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

// Cancel upgrade functions - delete both memory and temp files
func cancelProjectUpgrade(c echo.Context) error {
	id := c.Param("id")

	// Lock for memory operations
	common.GlobalMu.Lock()
	delete(project.GlobalProject.ProjectsNew, id)
	common.GlobalMu.Unlock()

	// Delete temp file if exists (without holding lock)
	tempPath, tempExists := GetComponentPath("project", id, true)
	if tempExists {
		_ = os.Remove(tempPath)
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "project upgrade cancelled"})
}

func cancelRulesetUpgrade(c echo.Context) error {
	id := c.Param("id")

	// Lock for memory operations
	common.GlobalMu.Lock()
	delete(project.GlobalProject.RulesetsNew, id)
	common.GlobalMu.Unlock()

	// Delete temp file if exists (without holding lock)
	tempPath, tempExists := GetComponentPath("ruleset", id, true)
	if tempExists {
		_ = os.Remove(tempPath)
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "ruleset upgrade cancelled"})
}

func cancelInputUpgrade(c echo.Context) error {
	id := c.Param("id")

	// Lock for memory operations
	common.GlobalMu.Lock()
	delete(project.GlobalProject.InputsNew, id)
	common.GlobalMu.Unlock()

	// Delete temp file if exists (without holding lock)
	tempPath, tempExists := GetComponentPath("input", id, true)
	if tempExists {
		_ = os.Remove(tempPath)
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "input upgrade cancelled"})
}

func cancelOutputUpgrade(c echo.Context) error {
	id := c.Param("id")

	// Lock for memory operations
	common.GlobalMu.Lock()
	delete(project.GlobalProject.OutputsNew, id)
	common.GlobalMu.Unlock()

	// Delete temp file if exists (without holding lock)
	tempPath, tempExists := GetComponentPath("output", id, true)
	if tempExists {
		_ = os.Remove(tempPath)
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "output upgrade cancelled"})
}

func cancelPluginUpgrade(c echo.Context) error {
	id := c.Param("id")

	// Lock for memory operations
	common.GlobalMu.Lock()
	delete(plugin.PluginsNew, id)
	common.GlobalMu.Unlock()

	// Delete temp file if exists (without holding lock)
	tempPath, tempExists := GetComponentPath("plugin", id, true)
	if tempExists {
		_ = os.Remove(tempPath)
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "plugin upgrade cancelled"})
}
