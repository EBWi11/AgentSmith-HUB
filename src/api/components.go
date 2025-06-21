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
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
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
    <rule id="reverse_shell_01" name="测试">
        <filter field="data_type">_$data_type</filter>
        <checklist condition="a and c and d and e">
            <node id="a" type="REGEX" field="exe">testcases</node>
            <node id="c" type="INCL" field="exe" logic="OR" delimiter="|">abc|edf</node>
            <node id="d" type="EQU" field="sessionid">_$sessionid</node>
        </checklist>
        <append field="abc">123</append>
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
			"id":         p.Name,     // Use id field for consistency with other components
			"name":       p.Name,     // Keep name for backward compatibility
			"type":       pluginType, // Convert to string type for frontend differentiation
			"hasTemp":    hasTemp,
			"returnType": p.ReturnType, // Include return type for filtering
		})
		processedNames[p.Name] = true
	}

	// Add plugins that only exist in temporary files
	for name, content := range plugin.PluginsNew {
		if !processedNames[name] {
			// Try to determine return type for temporary plugins
			returnType := "unknown"
			if content != "" {
				// Create a temporary plugin instance to get return type
				tempPlugin := &plugin.Plugin{
					Name:    name,
					Payload: []byte(content),
					Type:    plugin.YAEGI_PLUGIN,
				}
				// Try to load temporarily to get return type
				if err := tempPlugin.YaegiLoad(); err == nil {
					returnType = tempPlugin.ReturnType
				}
			}

			plugins = append(plugins, map[string]interface{}{
				"id":         name,  // Use id field for consistency with other components
				"name":       name,  // Keep name for backward compatibility
				"type":       "new", // Mark as new plugin
				"hasTemp":    true,
				"returnType": returnType, // Include return type for filtering
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
			// For local plugins, try to read the actual source code
			sourceCode, err := readLocalPluginSource(id)
			if err != nil {
				// Fallback to explanatory text if source cannot be read
				rawContent = fmt.Sprintf(`// Built-in Plugin: %s
// This is a built-in plugin that cannot be viewed or edited.
// Built-in plugins are compiled into the application and provide core functionality.
// Error reading source: %s`, id, err.Error())
			} else {
				// Add header comment to indicate this is a built-in plugin
				rawContent = fmt.Sprintf(`// Built-in Plugin: %s (Read-Only)
// This is a built-in plugin source code for reference only.
// Built-in plugins cannot be modified through the web interface.

%s`, id, sourceCode)
			}
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

	// Create a map to track processed IDs
	processedIDs := make(map[string]bool)

	for _, out := range p.Outputs {
		// Check if there is a temporary file
		_, hasTemp := p.OutputsNew[out.Id]

		outputs = append(outputs, map[string]interface{}{
			"id":      out.Id,
			"hasTemp": hasTemp,
		})
		processedIDs[out.Id] = true
	}

	// Add components that only exist in temporary files
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
	tempPath, tempExists := GetComponentPath(componentType, id, true)         // .new file
	componentPath, formalExists := GetComponentPath(componentType, id, false) // formal file

	// Check if component exists and perform memory operations
	var componentExists bool
	var globalMapToUpdate map[string]string

	common.GlobalMu.Lock()

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
		common.GlobalMu.Unlock()
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
				common.GlobalMu.Unlock()
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

	// Check if formal file exists, if not check if temporary file exists
	formalPath, formalExists := GetComponentPath(componentType, id, false)
	tempPath, tempExists := GetComponentPath(componentType, id, true)

	var originalContent string
	var err error

	if formalExists {
		// Read original formal file content to compare
		originalContent, err = ReadComponent(formalPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read original file: " + err.Error()})
		}
	} else if tempExists {
		// If no formal file but temp file exists, read temp file content
		originalContent, err = ReadComponent(tempPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read temporary file: " + err.Error()})
		}
	} else {
		// Neither formal nor temp file exists
		return c.JSON(http.StatusNotFound, map[string]string{"error": "component config not found"})
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
			// Also remove from memory with lock protection
			common.GlobalMu.Lock()
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
			common.GlobalMu.Unlock()
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "content identical to original file, no changes needed"})
	}

	// Content is different, create or update temporary file
	tempPath, _ = GetComponentPath(componentType, id, true)
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

	// Helper function to convert simple error to ValidationResult format
	createSimpleResult := func(err error) *rules_engine.ValidationResult {
		if err == nil {
			return &rules_engine.ValidationResult{
				IsValid:  true,
				Errors:   []rules_engine.ValidationError{},
				Warnings: []rules_engine.ValidationWarning{},
			}
		}
		return &rules_engine.ValidationResult{
			IsValid: false,
			Errors: []rules_engine.ValidationError{
				{
					Message: err.Error(),
				},
			},
			Warnings: []rules_engine.ValidationWarning{},
		}
	}

	switch singularType {
	case "input":
		err := input.Verify("", req.Raw)
		result := createSimpleResult(err)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"valid":    result.IsValid,
			"errors":   result.Errors,
			"warnings": result.Warnings,
		})
	case "output":
		err := output.Verify("", req.Raw)
		result := createSimpleResult(err)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"valid":    result.IsValid,
			"errors":   result.Errors,
			"warnings": result.Warnings,
		})
	case "ruleset":
		// Use detailed validation for rulesets
		result, err := rules_engine.ValidateWithDetails("", req.Raw)
		if err != nil {
			// If detailed validation fails, fall back to simple error
			result = createSimpleResult(err)
		}
		return c.JSON(http.StatusOK, map[string]interface{}{
			"valid":    result.IsValid,
			"errors":   result.Errors,
			"warnings": result.Warnings,
		})
	case "project":
		err := project.Verify("", req.Raw)
		result := createSimpleResult(err)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"valid":    result.IsValid,
			"errors":   result.Errors,
			"warnings": result.Warnings,
		})
	case "plugin":
		err := plugin.Verify("", req.Raw, id)
		result := createSimpleResult(err)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"valid":    result.IsValid,
			"errors":   result.Errors,
			"warnings": result.Warnings,
		})
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported component type"})
	}
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

// GetSamplerData retrieves sample data from project components
func GetSamplerData(c echo.Context) error {
	componentName := c.QueryParam("name")               // e.g., "input", "output", "ruleset"
	nodeSequence := c.QueryParam("projectNodeSequence") // e.g., "input.123", "ruleset.test"

	logger.Info("GetSamplerData request", "componentName", componentName, "nodeSequence", nodeSequence)

	if componentName == "" || nodeSequence == "" {
		logger.Error("Missing required parameters for GetSamplerData", "componentName", componentName, "nodeSequence", nodeSequence)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Missing required parameters: name and projectNodeSequence",
		})
	}

	// Parse node sequence to get component type and ID
	parts := strings.Split(nodeSequence, ".")
	if len(parts) < 2 {
		logger.Error("Invalid projectNodeSequence format", "nodeSequence", nodeSequence)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid projectNodeSequence format, expected 'type.id'",
		})
	}

	componentType := parts[0]
	componentId := strings.Join(parts[1:], ".")

	logger.Info("Parsed component info", "componentType", componentType, "componentId", componentId)

	// Check if the component exists
	componentExists := false
	switch componentType {
	case "input":
		common.GlobalMu.RLock()
		_, componentExists = project.GlobalProject.Inputs[componentId]
		common.GlobalMu.RUnlock()
	case "output":
		common.GlobalMu.RLock()
		_, componentExists = project.GlobalProject.Outputs[componentId]
		common.GlobalMu.RUnlock()
	case "ruleset":
		common.GlobalMu.RLock()
		_, componentExists = project.GlobalProject.Rulesets[componentId]
		common.GlobalMu.RUnlock()
	default:
		logger.Error("Unsupported component type", "componentType", componentType)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Unsupported component type: " + componentType,
		})
	}

	if !componentExists {
		logger.Error("Component not found", "componentType", componentType, "componentId", componentId)
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("Component '%s' of type '%s' not found", componentId, componentType),
		})
	}

	// Collect samples from all samplers that contain this component in their flow path
	result := make(map[string][]interface{})

	// Get potential sampler names based on component types and IDs
	samplerNames := []string{}
	common.GlobalMu.RLock()
	for inputId := range project.GlobalProject.Inputs {
		samplerNames = append(samplerNames, "input."+inputId)
	}
	for rulesetId := range project.GlobalProject.Rulesets {
		samplerNames = append(samplerNames, "ruleset."+rulesetId)
	}
	for outputId := range project.GlobalProject.Outputs {
		samplerNames = append(samplerNames, "output."+outputId)
	}
	common.GlobalMu.RUnlock()

	// Search through all samplers for flow paths ending with our target component (suffix matching)
	for _, samplerName := range samplerNames {
		sampler := common.GetSampler(samplerName)
		if sampler != nil {
			samples := sampler.GetSamples()
			for projectNodeSequence, sampleData := range samples {
				// IMPORTANT: Use suffix matching instead of contains matching
				// This ensures we get samples for the specific component position in the flow
				// e.g., for "ruleset.test", match "input.123.ruleset.test" but not "ruleset.test.output.print"
				if strings.HasSuffix(projectNodeSequence, nodeSequence) {
					logger.Info("Found matching sample data with suffix",
						"projectNodeSequence", projectNodeSequence,
						"nodeSequence", nodeSequence,
						"sampleCount", len(sampleData))

					// Convert SampleData to interface{} for JSON response
					convertedSamples := make([]interface{}, len(sampleData))
					for i, sample := range sampleData {
						convertedSamples[i] = map[string]interface{}{
							"data":                  sample.Data,
							"timestamp":             sample.Timestamp.Format(time.RFC3339),
							"source":                sample.Source,
							"project_node_sequence": sample.ProjectNodeSequence,
						}
					}
					result[projectNodeSequence] = convertedSamples
				}
			}
		}
	}

	// Initialize response structure
	response := map[string]interface{}{
		componentName: result,
	}

	logger.Info("GetSamplerData response ready",
		"componentName", componentName,
		"componentId", componentId,
		"totalFlowPaths", len(result),
		"totalSamples", func() int {
			total := 0
			for _, samples := range result {
				total += len(samples)
			}
			return total
		}())

	return c.JSON(http.StatusOK, response)
}

// GetRulesetFields extracts field keys from sample data for intelligent completion in ruleset editing
func GetRulesetFields(c echo.Context) error {
	componentId := c.Param("id")
	if componentId == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Component ID is required",
		})
	}

	logger.Info("GetRulesetFields request",
		"componentId", componentId)

	nodeSequence := fmt.Sprintf("ruleset.%s", componentId)
	var allSampleData []map[string]interface{}

	// Get potential sampler names based on component types and IDs
	samplerNames := []string{}
	common.GlobalMu.RLock()
	for inputId := range project.GlobalProject.Inputs {
		samplerNames = append(samplerNames, "input."+inputId)
	}
	for rulesetId := range project.GlobalProject.Rulesets {
		samplerNames = append(samplerNames, "ruleset."+rulesetId)
	}
	for outputId := range project.GlobalProject.Outputs {
		samplerNames = append(samplerNames, "output."+outputId)
	}
	common.GlobalMu.RUnlock()

	// Collect sample data from all potential sources
	for _, samplerName := range samplerNames {
		sampler := common.GetSampler(samplerName)
		if sampler != nil {
			samples := sampler.GetSamples()
			for projectNodeSequence, sampleDataList := range samples {
				// Match samples that flow into this ruleset
				if strings.HasSuffix(projectNodeSequence, nodeSequence) {
					for _, sample := range sampleDataList {
						if sampleMap, ok := sample.Data.(map[string]interface{}); ok {
							allSampleData = append(allSampleData, sampleMap)
						}
					}
				}
			}
		}
	}

	// Extract all possible field keys from the sample data
	fieldKeys := extractFieldKeys(allSampleData)

	response := map[string]interface{}{
		"componentId": componentId,
		"fieldKeys":   fieldKeys,
		"sampleCount": len(allSampleData),
	}

	logger.Info("GetRulesetFields response ready",
		"componentId", componentId,
		"fieldCount", len(fieldKeys),
		"sampleCount", len(allSampleData))

	return c.JSON(http.StatusOK, response)
}

// extractFieldKeys recursively extracts all possible field paths from sample data
func extractFieldKeys(sampleData []map[string]interface{}) []string {
	fieldSet := make(map[string]bool)

	for _, sample := range sampleData {
		extractKeysFromMap(sample, "", fieldSet)
	}

	// Convert set to sorted slice
	var fields []string
	for field := range fieldSet {
		fields = append(fields, field)
	}

	// Sort fields for consistent output
	sort.Strings(fields)

	return fields
}

// extractKeysFromMap recursively extracts keys from a nested map structure
func extractKeysFromMap(data map[string]interface{}, prefix string, fieldSet map[string]bool) {
	for key, value := range data {
		// Build the field path
		var fieldPath string
		if prefix == "" {
			fieldPath = key
		} else {
			fieldPath = prefix + "." + key
		}

		// Add current field path
		fieldSet[fieldPath] = true

		// Process nested structures
		switch v := value.(type) {
		case map[string]interface{}:
			// Nested map - recurse
			extractKeysFromMap(v, fieldPath, fieldSet)
		case []interface{}:
			// Array - check elements
			for i, item := range v {
				indexedPath := fieldPath + ".#_" + strconv.Itoa(i)
				fieldSet[indexedPath] = true

				if itemMap, ok := item.(map[string]interface{}); ok {
					extractKeysFromMap(itemMap, indexedPath, fieldSet)
				}
			}
		case string:
			// Only parse as JSON if it's clearly JSON (starts with { or [)
			if (strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}")) ||
				(strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]")) {
				var jsonData map[string]interface{}
				if err := sonic.Unmarshal([]byte(v), &jsonData); err == nil {
					extractKeysFromMap(jsonData, fieldPath, fieldSet)
				}
			}
			// Only parse as URL query string if it looks like one (contains = and &)
			if strings.Contains(v, "=") && (strings.Contains(v, "&") || strings.Count(v, "=") == 1) {
				if parsed, err := url.ParseQuery(v); err == nil && len(parsed) > 0 {
					queryMap := make(map[string]interface{})
					for qKey, qValues := range parsed {
						queryMap[qKey] = strings.Join(qValues, "")
					}
					extractKeysFromMap(queryMap, fieldPath, fieldSet)
				}
			}
		}
	}
}

// GetPluginParameters returns parameter information for a specific plugin
func GetPluginParameters(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Plugin ID is required",
		})
	}

	// Check if plugin exists in memory
	if p, exists := plugin.Plugins[id]; exists {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success":    true,
			"plugin":     id,
			"parameters": p.Parameters,
			"returnType": p.ReturnType,
		})
	}

	// Check if plugin exists in temporary files
	if tempContent, exists := plugin.PluginsNew[id]; exists {
		// Create a temporary plugin instance to parse parameters
		tempPlugin := &plugin.Plugin{
			Name:    id,
			Payload: []byte(tempContent),
			Type:    plugin.YAEGI_PLUGIN,
		}

		// Try to load the temporary plugin to get parameters
		err := tempPlugin.YaegiLoad()
		if err != nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("Failed to parse temporary plugin: %v", err),
			})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"success":    true,
			"plugin":     id,
			"parameters": tempPlugin.Parameters,
			"returnType": tempPlugin.ReturnType,
		})
	}

	return c.JSON(http.StatusNotFound, map[string]interface{}{
		"success": false,
		"error":   "Plugin not found: " + id,
	})
}

// readLocalPluginSource reads the source code of a built-in plugin
func readLocalPluginSource(pluginName string) (string, error) {
	// Get current working directory
	wd, err := os.Getwd()
	if err != nil {
		logger.Warn("Failed to get working directory", "error", err)
		wd = "."
	}

	// Map plugin names to their source file paths
	var sourcePath string
	switch pluginName {
	case "isLocalIP":
		// Try multiple possible paths
		possiblePaths := []string{
			filepath.Join(wd, "local_plugin", "is_local_ip", "is_local_ip.go"),
			filepath.Join(wd, "src", "local_plugin", "is_local_ip", "is_local_ip.go"),
			"local_plugin/is_local_ip/is_local_ip.go",
			"src/local_plugin/is_local_ip/is_local_ip.go",
		}

		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				sourcePath = path
				break
			}
		}

	case "parseJSON":
		// Try multiple possible paths
		possiblePaths := []string{
			filepath.Join(wd, "local_plugin", "local_plugin.go"),
			filepath.Join(wd, "src", "local_plugin", "local_plugin.go"),
			"local_plugin/local_plugin.go",
			"src/local_plugin/local_plugin.go",
		}

		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				sourcePath = path
				break
			}
		}
	}

	if sourcePath == "" {
		return "", fmt.Errorf("source file not found for plugin: %s", pluginName)
	}

	// Read the source file
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to read source file %s: %w", sourcePath, err)
	}

	// For parseJSON, we need to extract only the relevant function
	if pluginName == "parseJSON" {
		return extractParseJSONFunction(string(content)), nil
	}

	return string(content), nil
}

// extractParseJSONFunction extracts the parseJSON function from local_plugin.go
func extractParseJSONFunction(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inFunction := false
	braceCount := 0

	for _, line := range lines {
		if strings.Contains(line, "func parseJSONData") {
			inFunction = true
			braceCount = 0
		}

		if inFunction {
			result = append(result, line)

			// Count braces to determine function end
			for _, char := range line {
				if char == '{' {
					braceCount++
				} else if char == '}' {
					braceCount--
					if braceCount == 0 {
						inFunction = false
						break
					}
				}
			}
		}
	}

	if len(result) > 0 {
		// Add package declaration and imports for context
		return `package plugin

import (
"encoding/json"
"errors"
)

// parseJSONData parses JSON string and returns parsed data (for testing interface{} return type)
` + strings.Join(result[1:], "\n") // Skip the first line as we already added the function comment
	}

	return content // Fallback to full content if extraction fails
}
