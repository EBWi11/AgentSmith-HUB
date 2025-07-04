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
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v2"
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
	"fmt"
	"strings"
)

// Eval is the main function for plugin execution
// For checknode usage: returns (bool, error)
// For other usage: returns (interface{}, bool, error)
func Eval(args ...interface{}) (bool, error) {
	// Input validation - always check arguments first
	if len(args) == 0 {
		return false, errors.New("plugin requires at least one argument")
	}
	
	// Get the first argument (typically data or _$ORIDATA)
	data := args[0]
	
	// Convert to string for processing
	dataStr := fmt.Sprintf("%v", data)
	
	// Example implementation: check if data contains specific pattern
	if strings.Contains(dataStr, "something") {
		return true, nil
	}
	
	return false, nil
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

const NewRulesetData = `<root author="name">
    <rule id="rule_id">
        <filter field="key">vaule</filter>
        <checklist>
            <node type="REGEX" field="exe">testcases</node>
            <node type="INCL" field="exe" logic="OR" delimiter="|">abc|edf</node>
			<node type="PLUGIN" field="exe">plugin_name(_$ORIDATA)</node>
        </checklist>
        <append field="abc">123</append>
        <del>exe,argv</del>
		<plugin>plugin_name(_$ORIDATA, "test", field1)</plugin>
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
	result := make([]map[string]interface{}, 0)

	// Create a map to track processed IDs
	processedIDs := make(map[string]bool)

	for _, proj := range p.Projects {
		// Check if there is a temporary file
		tempRaw, hasTemp := p.ProjectsNew[proj.Id]

		// Get component lists
		inputList := make([]string, 0, len(proj.Inputs))
		for inputId := range proj.Inputs {
			inputList = append(inputList, inputId)
		}

		outputList := make([]string, 0, len(proj.Outputs))
		for outputId := range proj.Outputs {
			outputList = append(outputList, outputId)
		}

		rulesetList := make([]string, 0, len(proj.Rulesets))
		for rulesetId := range proj.Rulesets {
			rulesetList = append(rulesetList, rulesetId)
		}

		// Get raw configuration (prioritize temp if exists)
		rawConfig := proj.Config.RawConfig
		if hasTemp {
			rawConfig = tempRaw
		}

		projectData := map[string]interface{}{
			"id":                proj.Id,
			"status":            proj.Status,
			"hasTemp":           hasTemp,
			"raw":               rawConfig,
			"status_changed_at": proj.StatusChangedAt,
			"components": map[string]interface{}{
				"inputs":   inputList,
				"outputs":  outputList,
				"rulesets": rulesetList,
			},
			"component_counts": map[string]int{
				"inputs":   len(inputList),
				"outputs":  len(outputList),
				"rulesets": len(rulesetList),
				"total":    len(inputList) + len(outputList) + len(rulesetList),
			},
		}

		// Include path information
		if proj.Config != nil && proj.Config.Path != "" {
			projectData["path"] = proj.Config.Path
		}

		// Include error message if project status is error
		if proj.Status == project.ProjectStatusError && proj.Err != nil {
			projectData["error"] = proj.Err.Error()
		}

		result = append(result, projectData)
		processedIDs[proj.Id] = true
	}

	// Add components that only exist in temporary files
	for id, tempRaw := range p.ProjectsNew {
		if !processedIDs[id] {
			projectData := map[string]interface{}{
				"id":      id,
				"status":  project.ProjectStatusStopped,
				"hasTemp": true,
				"raw":     tempRaw,
				"components": map[string]interface{}{
					"inputs":   []string{},
					"outputs":  []string{},
					"rulesets": []string{},
				},
				"component_counts": map[string]int{
					"inputs":   0,
					"outputs":  0,
					"rulesets": 0,
					"total":    0,
				},
			}
			result = append(result, projectData)
		}
	}
	return c.JSON(http.StatusOK, result)
}

func getProject(c echo.Context) error {
	id := c.Param("id")

	p_raw, ok := project.GlobalProject.ProjectsNew[id]
	if ok {
		tempPath, _ := GetComponentPath("project", id, true)
		// Get sample data for this project (for MCP interface optimization)
		sampleData, dataSource, err := getSampleDataForProject(id)
		response := map[string]interface{}{
			"id":     id,
			"status": project.ProjectStatusStopped,
			"raw":    p_raw,
			"path":   tempPath,
		}
		if err == nil && len(sampleData) > 0 {
			response["sample_data"] = sampleData
			response["data_source"] = dataSource
		}
		return c.JSON(http.StatusOK, response)
	}

	p := project.GlobalProject.Projects[id]
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	formalPath, _ := GetComponentPath("project", id, false)
	// Get sample data for this project (for MCP interface optimization)
	sampleData, dataSource, err := getSampleDataForProject(id)
	response := map[string]interface{}{
		"id":                p.Id,
		"status":            p.Status,
		"raw":               p.Config.RawConfig,
		"path":              formalPath,
		"status_changed_at": p.StatusChangedAt,
	}
	if err == nil && len(sampleData) > 0 {
		response["sample_data"] = sampleData
		response["data_source"] = dataSource
	}
	return c.JSON(http.StatusOK, response)
}

func getRulesets(c echo.Context) error {
	p := project.GlobalProject
	rulesets := make([]map[string]interface{}, 0)

	// Create a map to track processed IDs
	processedIDs := make(map[string]bool)

	// Helper function to find which projects use a ruleset
	findProjectsUsingRuleset := func(rulesetId string) []string {
		projects := make([]string, 0)
		for _, proj := range p.Projects {
			if _, exists := proj.Rulesets[rulesetId]; exists {
				projects = append(projects, proj.Id)
			}
		}
		return projects
	}

	// Helper function to count rules in XML content
	countRulesInXML := func(xmlContent string) int {
		if xmlContent == "" {
			return 0
		}
		lines := strings.Split(xmlContent, "\n")
		count := 0
		for _, line := range lines {
			if strings.Contains(line, "<rule") && strings.Contains(line, "id=") {
				count++
			}
		}
		return count
	}

	// Helper function to extract ruleset type from XML
	extractRulesetType := func(xmlContent string) string {
		if xmlContent == "" {
			return "unknown"
		}
		lines := strings.Split(xmlContent, "\n")
		for _, line := range lines {
			if strings.Contains(line, "<root") && strings.Contains(line, "type=") {
				if strings.Contains(line, `type="detection"`) || strings.Contains(line, `type='detection'`) {
					return "detection"
				} else if strings.Contains(line, `type="whitelist"`) || strings.Contains(line, `type='whitelist'`) {
					return "whitelist"
				}
			}
		}
		return "detection" // default
	}

	for _, r := range p.Rulesets {
		// Check if there is a temporary file
		tempRaw, hasTemp := p.RulesetsNew[r.RulesetID]

		// Get raw configuration (prioritize temp if exists)
		rawConfig := r.RawConfig
		if hasTemp {
			rawConfig = tempRaw
		}

		// Get projects using this ruleset
		usedByProjects := findProjectsUsingRuleset(r.RulesetID)

		// Count rules and extract type
		ruleCount := countRulesInXML(rawConfig)
		rulesetType := extractRulesetType(rawConfig)

		rulesetData := map[string]interface{}{
			"id":               r.RulesetID,
			"hasTemp":          hasTemp,
			"raw":              rawConfig,
			"type":             rulesetType,
			"rule_count":       ruleCount,
			"used_by_projects": usedByProjects,
			"project_count":    len(usedByProjects),
		}

		// Include path information if available
		if r.Path != "" {
			rulesetData["path"] = r.Path
		}

		rulesets = append(rulesets, rulesetData)
		processedIDs[r.RulesetID] = true
	}

	// Add components that only exist in temporary files
	for id, tempRaw := range p.RulesetsNew {
		if !processedIDs[id] {
			// Count rules and extract type from temp content
			ruleCount := countRulesInXML(tempRaw)
			rulesetType := extractRulesetType(tempRaw)

			rulesetData := map[string]interface{}{
				"id":               id,
				"hasTemp":          true,
				"raw":              tempRaw,
				"type":             rulesetType,
				"rule_count":       ruleCount,
				"used_by_projects": []string{}, // No projects use temp rulesets
				"project_count":    0,
			}
			rulesets = append(rulesets, rulesetData)
		}
	}
	return c.JSON(http.StatusOK, rulesets)
}

func getRuleset(c echo.Context) error {
	id := c.Param("id")

	r_raw, ok := project.GlobalProject.RulesetsNew[id]
	if ok {
		tempPath, _ := GetComponentPath("ruleset", id, true)
		// Get sample data for this ruleset (for MCP interface optimization)
		sampleData, dataSource, err := getSampleDataForRuleset(id)
		response := map[string]interface{}{
			"id":   id,
			"raw":  r_raw,
			"path": tempPath,
		}
		if err == nil && len(sampleData) > 0 {
			response["sample_data"] = sampleData
			response["data_source"] = dataSource
		}
		return c.JSON(http.StatusOK, response)
	}

	r := project.GlobalProject.Rulesets[id]

	if r != nil {
		formalPath, _ := GetComponentPath("ruleset", id, false)
		// Get sample data for this ruleset (for MCP interface optimization)
		sampleData, dataSource, err := getSampleDataForRuleset(id)
		response := map[string]interface{}{
			"id":   r.RulesetID,
			"raw":  r.RawConfig,
			"path": formalPath,
		}
		if err == nil && len(sampleData) > 0 {
			response["sample_data"] = sampleData
			response["data_source"] = dataSource
		}
		return c.JSON(http.StatusOK, response)
	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
}

func getInputs(c echo.Context) error {
	p := project.GlobalProject
	inputs := make([]map[string]interface{}, 0)

	// Create a map to track processed IDs
	processedIDs := make(map[string]bool)

	// Helper function to find which projects use an input
	findProjectsUsingInput := func(inputId string) []string {
		projects := make([]string, 0)
		for _, proj := range p.Projects {
			if _, exists := proj.Inputs[inputId]; exists {
				projects = append(projects, proj.Id)
			}
		}
		return projects
	}

	// Helper function to extract input type from YAML content
	extractInputType := func(yamlContent string) string {
		if yamlContent == "" {
			return "unknown"
		}
		lines := strings.Split(yamlContent, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "type:") {
				parts := strings.Split(trimmed, ":")
				if len(parts) >= 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
		return "unknown"
	}

	for _, in := range p.Inputs {
		// Check if there is a temporary file
		tempRaw, hasTemp := p.InputsNew[in.Id]

		// Get raw configuration (prioritize temp if exists)
		rawConfig := in.Config.RawConfig
		if hasTemp {
			rawConfig = tempRaw
		}

		// Get projects using this input
		usedByProjects := findProjectsUsingInput(in.Id)

		// Extract input type
		inputType := extractInputType(rawConfig)

		inputData := map[string]interface{}{
			"id":               in.Id,
			"hasTemp":          hasTemp,
			"raw":              rawConfig,
			"type":             inputType,
			"used_by_projects": usedByProjects,
			"project_count":    len(usedByProjects),
		}

		// Include path information if available
		if in.Path != "" {
			inputData["path"] = in.Path
		}

		inputs = append(inputs, inputData)
		processedIDs[in.Id] = true
	}

	// Add components that only exist in temporary files
	for id, tempRaw := range p.InputsNew {
		if !processedIDs[id] {
			// Extract type from temp content
			inputType := extractInputType(tempRaw)

			inputData := map[string]interface{}{
				"id":               id,
				"hasTemp":          true,
				"raw":              tempRaw,
				"type":             inputType,
				"used_by_projects": []string{}, // No projects use temp inputs
				"project_count":    0,
			}
			inputs = append(inputs, inputData)
		}
	}
	return c.JSON(http.StatusOK, inputs)
}

// parseOutputType extracts the type field from output YAML configuration
func parseOutputType(rawConfig string) string {
	var config struct {
		Type string `yaml:"type"`
	}

	if err := yaml.Unmarshal([]byte(rawConfig), &config); err != nil {
		return ""
	}

	return config.Type
}

func getInput(c echo.Context) error {
	id := c.Param("id")
	in_raw, ok := project.GlobalProject.InputsNew[id]
	if ok {
		tempPath, _ := GetComponentPath("input", id, true)
		// Get sample data for this input (for MCP interface optimization)
		sampleData, dataSource, err := getSampleDataForInput(id)
		response := map[string]interface{}{
			"id":   id,
			"raw":  in_raw,
			"path": tempPath,
		}
		if err == nil && len(sampleData) > 0 {
			response["sample_data"] = sampleData
			response["data_source"] = dataSource
		}
		return c.JSON(http.StatusOK, response)
	}

	in := project.GlobalProject.Inputs[id]

	if in != nil {
		formalPath, _ := GetComponentPath("input", id, false)
		// Get sample data for this input (for MCP interface optimization)
		sampleData, dataSource, err := getSampleDataForInput(id)
		response := map[string]interface{}{
			"id":   in.Id,
			"raw":  in.Config.RawConfig,
			"path": formalPath,
		}
		if err == nil && len(sampleData) > 0 {
			response["sample_data"] = sampleData
			response["data_source"] = dataSource
		}
		return c.JSON(http.StatusOK, response)
	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "input not found"})
}

func getPlugins(c echo.Context) error {
	plugins := make([]map[string]interface{}, 0)

	// Create a map to track processed names
	processedNames := make(map[string]bool)

	// Helper function to find which rulesets use a plugin
	findRulesetsUsingPlugin := func(pluginName string) []string {
		rulesets := make([]string, 0)
		p := project.GlobalProject
		for _, r := range p.Rulesets {
			// Check if plugin is used in any rule within this ruleset
			for _, rule := range r.Rules {
				// Check in checklist nodes
				for _, node := range rule.Checklist.CheckNodes {
					if node.Type == "PLUGIN" && strings.Contains(node.Value, pluginName+"(") {
						rulesets = append(rulesets, r.RulesetID)
						goto nextRuleset
					}
				}
				// Check in append elements
				for _, appendElem := range rule.Appends {
					if appendElem.Type == "PLUGIN" && strings.Contains(appendElem.Value, pluginName+"(") {
						rulesets = append(rulesets, r.RulesetID)
						goto nextRuleset
					}
				}
				// Check in plugin elements
				for _, pluginElem := range rule.Plugins {
					if strings.Contains(pluginElem.Value, pluginName+"(") {
						rulesets = append(rulesets, r.RulesetID)
						goto nextRuleset
					}
				}
			}
		nextRuleset:
		}
		return rulesets
	}

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
		tempRaw, hasTemp := plugin.PluginsNew[p.Name]

		// Get raw configuration (prioritize temp if exists)
		rawConfig := string(p.Payload)
		if hasTemp {
			rawConfig = tempRaw
		}

		// Find rulesets using this plugin
		usedByRulesets := findRulesetsUsingPlugin(p.Name)

		pluginData := map[string]interface{}{
			"id":               p.Name,     // Use id field for consistency with other components
			"name":             p.Name,     // Keep name for backward compatibility
			"type":             pluginType, // Convert to string type for frontend differentiation
			"hasTemp":          hasTemp,
			"raw":              rawConfig,
			"returnType":       p.ReturnType, // Include return type for filtering
			"parameters":       p.Parameters, // Include parameter information
			"used_by_rulesets": usedByRulesets,
			"ruleset_count":    len(usedByRulesets),
		}

		plugins = append(plugins, pluginData)
		processedNames[p.Name] = true
	}

	// Add plugins that only exist in temporary files
	for name, content := range plugin.PluginsNew {
		if !processedNames[name] {
			// Try to determine return type for temporary plugins
			returnType := "unknown"
			parameters := []plugin.PluginParameter{}
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
					parameters = tempPlugin.Parameters
				}
			}

			pluginData := map[string]interface{}{
				"id":               name,  // Use id field for consistency with other components
				"name":             name,  // Keep name for backward compatibility
				"type":             "new", // Mark as new plugin
				"hasTemp":          true,
				"raw":              content,
				"returnType":       returnType, // Include return type for filtering
				"parameters":       parameters, // Include parameter information
				"used_by_rulesets": []string{}, // No rulesets use temp plugins
				"ruleset_count":    0,
			}
			plugins = append(plugins, pluginData)
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

	// Helper function to find which projects use an output
	findProjectsUsingOutput := func(outputId string) []string {
		projects := make([]string, 0)
		for _, proj := range p.Projects {
			if _, exists := proj.Outputs[outputId]; exists {
				projects = append(projects, proj.Id)
			}
		}
		return projects
	}

	for _, out := range p.Outputs {
		// Check if there is a temporary file
		tempRaw, hasTemp := p.OutputsNew[out.Id]

		// Get raw configuration (prioritize temp if exists)
		rawConfig := out.Config.RawConfig
		if hasTemp {
			rawConfig = tempRaw
		}

		// Get projects using this output
		usedByProjects := findProjectsUsingOutput(out.Id)

		// Extract output type
		outputType := string(out.Type)
		if hasTemp {
			// Parse type from temp content if available
			if parsedType := parseOutputType(tempRaw); parsedType != "" {
				outputType = parsedType
			}
		}

		outputData := map[string]interface{}{
			"id":               out.Id,
			"hasTemp":          hasTemp,
			"raw":              rawConfig,
			"type":             outputType,
			"used_by_projects": usedByProjects,
			"project_count":    len(usedByProjects),
		}

		// Include path information if available
		if out.Path != "" {
			outputData["path"] = out.Path
		}

		outputs = append(outputs, outputData)
		processedIDs[out.Id] = true
	}

	// Add components that only exist in temporary files
	for id, rawConfig := range p.OutputsNew {
		if !processedIDs[id] {
			// Parse the temporary file to get type information
			outputType := "unknown"
			if rawConfig != "" {
				if parsedType := parseOutputType(rawConfig); parsedType != "" {
					outputType = parsedType
				}
			}

			outputData := map[string]interface{}{
				"id":               id,
				"hasTemp":          true,
				"raw":              rawConfig,
				"type":             outputType,
				"used_by_projects": []string{}, // No projects use temp outputs
				"project_count":    0,
			}
			outputs = append(outputs, outputData)
		}
	}
	return c.JSON(http.StatusOK, outputs)
}

func getOutput(c echo.Context) error {
	id := c.Param("id")
	out_raw, ok := project.GlobalProject.OutputsNew[id]
	if ok {
		tempPath, _ := GetComponentPath("output", id, true)
		// Parse type from temporary file content
		outputType := parseOutputType(out_raw)
		// Get sample data for this output (for MCP interface optimization)
		sampleData, dataSource, err := getSampleDataForOutput(id)
		response := map[string]interface{}{
			"id":   id,
			"raw":  out_raw,
			"path": tempPath,
			"type": outputType,
		}
		if err == nil && len(sampleData) > 0 {
			response["sample_data"] = sampleData
			response["data_source"] = dataSource
		}
		return c.JSON(http.StatusOK, response)
	}

	out := project.GlobalProject.Outputs[id]

	if out != nil {
		formalPath, _ := GetComponentPath("output", id, false)
		// Get sample data for this output (for MCP interface optimization)
		sampleData, dataSource, err := getSampleDataForOutput(id)
		response := map[string]interface{}{
			"id":   out.Id,
			"raw":  out.Config.RawConfig,
			"path": formalPath,
			"type": string(out.Type), // Include output type
		}
		if err == nil && len(sampleData) > 0 {
			response["sample_data"] = sampleData
			response["data_source"] = dataSource
		}
		return c.JSON(http.StatusOK, response)
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

	// Create enhanced response with deployment guidance
	componentTypeName := strings.ToTitle(componentType)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message":      fmt.Sprintf("✅ %s created successfully in temporary file", componentTypeName),
		"component_id": request.ID,
		"status":       "pending",
		"file_type":    "temporary",
		"next_steps": map[string]interface{}{
			"1": "Review changes: Use 'get_pending_changes' to see all components awaiting deployment",
			"2": "Deploy component: Use 'apply_changes' to activate the component in production",
			"3": fmt.Sprintf("Test component: Use appropriate test tools to verify %s functionality", componentType),
		},
		"important_note": fmt.Sprintf("⚠️ This %s is currently in a TEMPORARY file and is NOT ACTIVE in production yet. You must apply changes to activate it.", componentType),
		"helpful_commands": []string{
			"get_pending_changes - View all changes waiting for deployment",
			"apply_changes - Deploy all pending changes to production",
			fmt.Sprintf("verify_component - Validate %s configuration", componentType),
		},
		"deployment_required": true,
	})
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

	// Create enhanced response with deployment guidance
	componentTypeName := strings.ToTitle(componentType)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":      fmt.Sprintf("✅ %s updated successfully in temporary file", componentTypeName),
		"component_id": id,
		"status":       "pending",
		"file_type":    "temporary",
		"changes":      "saved to temporary file",
		"next_steps": map[string]interface{}{
			"1": "Review changes: Use 'get_pending_changes' to see all modifications awaiting deployment",
			"2": "Deploy changes: Use 'apply_changes' to activate the updated component in production",
			"3": fmt.Sprintf("Test changes: Use appropriate test tools to verify updated %s functionality", componentType),
		},
		"important_note": fmt.Sprintf("⚠️ Your %s update is in a TEMPORARY file and is NOT YET ACTIVE in production. The original version is still running until you apply changes.", componentType),
		"helpful_commands": []string{
			"get_pending_changes - View all changes waiting for deployment",
			"apply_changes - Deploy all pending changes to production",
			fmt.Sprintf("verify_component - Validate updated %s configuration", componentType),
		},
		"deployment_required": true,
	})
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

		// Extract line number from error message
		errorMsg := err.Error()
		lineNumber := 0

		// Parse different error formats:
		// 1. "YAML parse error: yaml-line X: ..." format
		if match := regexp.MustCompile(`yaml-line\s+(\d+)`).FindStringSubmatch(errorMsg); len(match) > 1 {
			if num, parseErr := strconv.Atoi(match[1]); parseErr == nil {
				lineNumber = num
			}
		} else if match := regexp.MustCompile(`(\d+):(\d+):`).FindStringSubmatch(errorMsg); len(match) > 1 {
			// 2. Plugin format: "failed to parse plugin code: 7:1: expected declaration, found asdsad"
			if num, parseErr := strconv.Atoi(match[1]); parseErr == nil {
				lineNumber = num
			}
		} else if match := regexp.MustCompile(`at line (\d+)`).FindStringSubmatch(errorMsg); len(match) > 1 {
			// 3. Project format: "... not found at line 2"
			if num, parseErr := strconv.Atoi(match[1]); parseErr == nil {
				lineNumber = num
			}
		}

		return &rules_engine.ValidationResult{
			IsValid: false,
			Errors: []rules_engine.ValidationError{
				{
					Line:    lineNumber,
					Message: errorMsg,
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
	// Only leader nodes collect sample data for performance reasons
	if !cluster.IsLeader {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"message": "Sample data collection is only available on leader node",
			"data":    map[string]interface{}{},
		})
	}

	componentName := c.QueryParam("name")               // e.g., "input", "output", "ruleset"
	nodeSequence := c.QueryParam("projectNodeSequence") // e.g., "INPUT.api_sec.RULESET.test" or "ruleset.test" (legacy)

	logger.Info("GetSamplerData request", "componentName", componentName, "nodeSequence", nodeSequence)

	if componentName == "" || nodeSequence == "" {
		logger.Error("Missing required parameters for GetSamplerData", "componentName", componentName, "nodeSequence", nodeSequence)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Missing required parameters: name and projectNodeSequence",
		})
	}

	// Enhanced parsing to handle both old and new ProjectNodeSequence formats
	var componentType, componentId string

	// Check if this is a full ProjectNodeSequence (new format) or simple type.id (legacy format)
	if strings.Contains(nodeSequence, ".") {
		parts := strings.Split(nodeSequence, ".")

		// For full ProjectNodeSequence like "INPUT.api_sec.RULESET.test.OUTPUT.print_demo"
		// Extract the component info based on the requested componentName
		normalizedName := strings.ToUpper(componentName) // Convert to uppercase for matching

		// Find the position of the requested component type in the sequence
		for i, part := range parts {
			if strings.ToUpper(part) == normalizedName {
				componentType = strings.ToLower(part) // Use lowercase for consistency
				if i+1 < len(parts) {
					componentId = parts[i+1]
				}
				break
			}
		}

		// If not found in full sequence, try legacy format (type.id)
		if componentType == "" && len(parts) == 2 {
			componentType = strings.ToLower(parts[0])
			componentId = parts[1]
		}
	} else {
		// Single component name without dots - assume it's the ID and use componentName as type
		componentType = strings.ToLower(componentName)
		componentId = nodeSequence
	}

	if componentType == "" || componentId == "" {
		logger.Error("Failed to parse component info from nodeSequence",
			"nodeSequence", nodeSequence,
			"componentName", componentName,
			"parsedType", componentType,
			"parsedId", componentId)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Failed to parse component information from projectNodeSequence",
		})
	}

	logger.Info("Parsed component info", "componentType", componentType, "componentId", componentId)

	// Check if the component exists - support case insensitive
	componentExists := false
	normalizedType := strings.ToLower(componentType) // Normalize to lowercase for processing
	switch normalizedType {
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
		logger.Error("Unsupported component type in GetSamplerData",
			"componentType", componentType,
			"normalizedType", normalizedType,
			"componentId", componentId,
			"nodeSequence", nodeSequence,
			"supportedTypes", []string{"input", "output", "ruleset"})
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Unsupported component type: '%s'. Supported types: input, output, ruleset", componentType),
		})
	}

	if !componentExists {
		logger.Warn("Component not found for sample data request",
			"componentType", componentType,
			"componentId", componentId,
			"nodeSequence", nodeSequence)
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
	totalSamples := 0
	for _, samplerName := range samplerNames {
		sampler := common.GetSampler(samplerName)
		if sampler != nil {
			samples := sampler.GetSamples()
			for projectNodeSequence, sampleData := range samples {
				// Enhanced matching logic to handle both legacy and new ProjectNodeSequence formats
				// Support both "RULESET.test" (legacy) and "INPUT.api_sec.RULESET.test" (new format)
				matched := false

				// Method 1: Use suffix matching to get the component's own sample data
				// This ensures we get the data AT this component, not data that has passed through it
				// For example: "input.skyguard" should match "INPUT.skyguard" but NOT "INPUT.skyguard.RULESET.test"
				if strings.HasSuffix(projectNodeSequence, nodeSequence) {
					matched = true
				}

				// Method 2: Component position matching (for new ProjectNodeSequence format)
				if !matched {
					// Parse the requested nodeSequence to extract component type and ID
					parts := strings.Split(nodeSequence, ".")
					if len(parts) == 2 {
						requestedType := strings.ToUpper(parts[0])
						requestedID := parts[1]

						// Check if the ProjectNodeSequence contains this component in the right position
						sequenceParts := strings.Split(projectNodeSequence, ".")
						for i := 0; i < len(sequenceParts)-1; i++ {
							if strings.ToUpper(sequenceParts[i]) == requestedType && sequenceParts[i+1] == requestedID {
								matched = true
								break
							}
						}
					}
				}

				if matched {
					logger.Info("Found matching sample data",
						"projectNodeSequence", projectNodeSequence,
						"nodeSequence", nodeSequence,
						"sampleCount", len(sampleData))

					// Convert SampleData to interface{} for JSON response
					convertedSamples := make([]interface{}, len(sampleData))
					for i, sample := range sampleData {
						convertedSamples[i] = map[string]interface{}{
							"data":                  sample.Data,
							"timestamp":             sample.Timestamp.Format(time.RFC3339),
							"project_node_sequence": sample.ProjectNodeSequence,
						}
					}
					result[projectNodeSequence] = convertedSamples
					totalSamples += len(sampleData)
				}
			}
		}
	}

	// Improve empty data handling: return success response regardless of whether there is data
	if totalSamples == 0 {
		logger.Info("No sample data found for component",
			"componentType", componentType,
			"componentId", componentId,
			"nodeSequence", nodeSequence,
			"message", "This is normal if the component hasn't processed any data yet")
	}

	// Initialize response structure
	response := map[string]interface{}{
		componentName: result,
	}

	logger.Info("GetSamplerData response ready",
		"componentName", componentName,
		"componentId", componentId,
		"totalFlowPaths", len(result),
		"totalSamples", totalSamples)

	return c.JSON(http.StatusOK, response)
}

// GetRulesetFields extracts field keys from sample data for intelligent completion in ruleset editing
func GetRulesetFields(c echo.Context) error {
	// Only leader nodes collect sample data for performance reasons
	if !cluster.IsLeader {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"componentId": c.Param("id"),
			"fieldKeys":   []string{},
			"sampleCount": 0,
			"message":     "Sample data collection is only available on leader node",
		})
	}

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

// SearchResult represents a single search match
type SearchResult struct {
	ComponentType string `json:"component_type"`
	ComponentID   string `json:"component_id"`
	FileName      string `json:"file_name"`
	FilePath      string `json:"file_path"`
	LineNumber    int    `json:"line_number"`
	LineContent   string `json:"line_content"`
	IsTemporary   bool   `json:"is_temporary"`
}

// SearchResponse represents the search API response
type SearchResponse struct {
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
	Total   int            `json:"total"`
}

// searchComponentsConfig handles the search API endpoint
func searchComponentsConfig(c echo.Context) error {
	query := c.QueryParam("q")
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "query parameter 'q' is required",
		})
	}

	// Component types to search
	componentTypes := []string{"input", "output", "ruleset", "project", "plugin"}
	var allResults []SearchResult

	for _, componentType := range componentTypes {
		// Search formal files
		results := searchInComponentType(componentType, query, false)
		allResults = append(allResults, results...)

		// Search temporary files
		tempResults := searchInComponentType(componentType, query, true)
		allResults = append(allResults, tempResults...)
	}

	// Sort results by component type, then by component ID, then by line number
	sort.Slice(allResults, func(i, j int) bool {
		if allResults[i].ComponentType != allResults[j].ComponentType {
			return allResults[i].ComponentType < allResults[j].ComponentType
		}
		if allResults[i].ComponentID != allResults[j].ComponentID {
			return allResults[i].ComponentID < allResults[j].ComponentID
		}
		return allResults[i].LineNumber < allResults[j].LineNumber
	})

	response := SearchResponse{
		Query:   query,
		Results: allResults,
		Total:   len(allResults),
	}

	return c.JSON(http.StatusOK, response)
}

// searchInComponentType searches within a specific component type
func searchInComponentType(componentType, query string, isTemporary bool) []SearchResult {
	var results []SearchResult
	var componentMap map[string]string

	// Get component content map based on type and temporary status
	if isTemporary {
		switch componentType {
		case "input":
			componentMap = project.GlobalProject.InputsNew
		case "output":
			componentMap = project.GlobalProject.OutputsNew
		case "ruleset":
			componentMap = project.GlobalProject.RulesetsNew
		case "project":
			componentMap = project.GlobalProject.ProjectsNew
		case "plugin":
			componentMap = plugin.PluginsNew
		}
	} else {
		// For formal files, we need to read from the actual component instances
		componentMap = make(map[string]string)
		switch componentType {
		case "input":
			for _, comp := range project.GlobalProject.Inputs {
				componentMap[comp.Id] = comp.Config.RawConfig
			}
		case "output":
			for _, comp := range project.GlobalProject.Outputs {
				componentMap[comp.Id] = comp.Config.RawConfig
			}
		case "ruleset":
			for _, comp := range project.GlobalProject.Rulesets {
				componentMap[comp.RulesetID] = comp.RawConfig
			}
		case "project":
			for _, comp := range project.GlobalProject.Projects {
				componentMap[comp.Id] = comp.Config.RawConfig
			}
		case "plugin":
			for _, comp := range plugin.Plugins {
				if comp.Type == plugin.YAEGI_PLUGIN {
					componentMap[comp.Name] = string(comp.Payload)
				} else if comp.Type == plugin.LOCAL_PLUGIN {
					// Try to read local plugin source
					if source, err := readLocalPluginSource(comp.Name); err == nil {
						componentMap[comp.Name] = source
					}
				}
			}
		}
	}

	// Search within each component's content
	for componentID, content := range componentMap {
		matches := searchInContent(content, query)
		for _, match := range matches {
			filePath, _ := GetComponentPath(componentType, componentID, isTemporary)
			fileName := filepath.Base(filePath)

			result := SearchResult{
				ComponentType: componentType,
				ComponentID:   componentID,
				FileName:      fileName,
				FilePath:      filePath,
				LineNumber:    match.LineNumber,
				LineContent:   match.LineContent,
				IsTemporary:   isTemporary,
			}
			results = append(results, result)
		}
	}

	return results
}

// ContentMatch represents a match within content
type ContentMatch struct {
	LineNumber  int
	LineContent string
}

// searchInContent searches for query within content and returns matches
func searchInContent(content, query string) []ContentMatch {
	var matches []ContentMatch

	if content == "" || query == "" {
		return matches
	}

	lines := strings.Split(content, "\n")
	queryLower := strings.ToLower(query)

	for lineNum, line := range lines {
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, queryLower) {
			matches = append(matches, ContentMatch{
				LineNumber:  lineNum + 1, // 1-based line numbers
				LineContent: strings.TrimSpace(line),
			})
		}
	}

	return matches
}

// deleteRulesetRule deletes a specific rule from a ruleset
func deleteRulesetRule(c echo.Context) error {
	rulesetId := c.Param("id")
	ruleId := c.Param("ruleId")

	if rulesetId == "" || ruleId == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ruleset id and rule id are required"})
	}

	// Get current ruleset content (prioritize temp file if exists)
	var currentRawConfig string
	var isTemp bool

	// Check temp file first
	if tempRaw, ok := project.GlobalProject.RulesetsNew[rulesetId]; ok {
		currentRawConfig = tempRaw
		isTemp = true
	} else if r := project.GlobalProject.Rulesets[rulesetId]; r != nil {
		currentRawConfig = r.RawConfig
		isTemp = false
	} else {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
	}

	// Note: We don't validate the current ruleset here since we're removing a rule
	// The important validation is after removal to ensure the result is valid

	// Remove the specified rule from XML
	updatedXML, err := removeRuleFromXML(currentRawConfig, ruleId)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Do complete ruleset validation after rule removal
	// This ensures the remaining ruleset is still valid and functional
	tempRuleset, err := rules_engine.NewRuleset("", updatedXML, "temp_validation_delete_"+rulesetId)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error":   "ruleset validation failed after rule deletion",
			"details": err.Error(),
		})
	}

	// Clean up the temporary ruleset (it was only for validation)
	if tempRuleset != nil {
		tempRuleset = nil
	}

	// Save to temp file
	tempPath, _ := GetComponentPath("ruleset", rulesetId, true)
	err = WriteComponentFile(tempPath, updatedXML)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save updated ruleset: " + err.Error()})
	}

	// Update memory
	common.GlobalMu.Lock()
	project.GlobalProject.RulesetsNew[rulesetId] = updatedXML
	common.GlobalMu.Unlock()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":  "✅ Rule deleted successfully from temporary file",
		"rule_id":  ruleId,
		"was_temp": isTemp,
		"status":   "pending",
		"next_steps": map[string]interface{}{
			"1": "Review changes: Use 'get_pending_changes' to see all modifications awaiting deployment",
			"2": "Deploy changes: Use 'apply_changes' to activate the rule deletion in production",
			"3": "Test ruleset: Use 'test_ruleset' to verify the ruleset works correctly without this rule",
		},
		"important_note": "⚠️ The rule deletion is saved in a TEMPORARY file and is NOT YET ACTIVE in production. The rule is still active until you apply changes.",
		"helpful_commands": []string{
			"get_pending_changes - View all changes waiting for deployment",
			"apply_changes - Deploy all pending changes to production",
			"test_ruleset - Test the ruleset after rule deletion",
		},
		"deployment_required": true,
	})
}

// addRulesetRule adds a new rule to a ruleset
func addRulesetRule(c echo.Context) error {
	rulesetId := c.Param("id")

	var request struct {
		RuleRaw string `json:"rule_raw"`
	}

	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if rulesetId == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":            "❌ REQUEST VALIDATION FAILED: Missing ruleset ID.\n\n🔧 HOW TO FIX: Specify the ruleset ID where you want to add the rule.\n\n📋 To find available rulesets, use 'get_rulesets'.",
			"helpful_commands": []string{"get_rulesets - View all available rulesets"},
		})
	}

	if request.RuleRaw == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "❌ REQUEST VALIDATION FAILED: Missing rule content.\n\n🔧 HOW TO FIX: Provide the complete rule XML in the 'rule_raw' field.\n\n📝 REQUIRED FORMAT:\n<rule id=\"unique_id\" name=\"Detailed description\">\n    <filter field=\"field_name\">value</filter>\n    <checklist>\n        <node type=\"NODE_TYPE\" field=\"field_name\">value</node>\n    </checklist>\n</rule>",
			"helpful_commands": []string{
				"get_rule_templates - View example rules",
				"get_ruleset_syntax_guide - Learn proper syntax",
			},
		})
	}

	// Validate the rule XML syntax first
	ruleId, err := validateAndExtractRuleId(request.RuleRaw)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
			"helpful_commands": []string{
				"get_ruleset_syntax_guide - Learn proper rule syntax and best practices",
				"get_rule_templates - View example rules for common patterns",
				"get_ruleset_templates - See complete ruleset examples",
			},
			"suggestion": "Review the syntax guide and templates to understand the correct rule format",
		})
	}

	// Get current ruleset content (prioritize temp file if exists)
	var currentRawConfig string
	var isTemp bool

	// Check temp file first
	if tempRaw, ok := project.GlobalProject.RulesetsNew[rulesetId]; ok {
		currentRawConfig = tempRaw
		isTemp = true
	} else if r := project.GlobalProject.Rulesets[rulesetId]; r != nil {
		currentRawConfig = r.RawConfig
		isTemp = false
	} else {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
	}

	// Check if rule ID already exists in current ruleset
	if ruleExistsInXML(currentRawConfig, ruleId) {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":       fmt.Sprintf("❌ RULE ADDITION FAILED: Rule ID conflict detected.\n\n🔧 HOW TO FIX: A rule with ID '%s' already exists in this ruleset. Choose a different, unique ID.\n\n💡 SUGGESTED ALTERNATIVES:\n- %s_v2\n- %s_enhanced\n- %s_updated\n- Add date/time: %s_2024\n\n📋 To view existing rules, use 'get_ruleset' to see all current rule IDs.", ruleId, ruleId, ruleId, ruleId, ruleId),
			"conflict_id": ruleId,
			"suggestion":  "Use a unique rule ID that doesn't exist in the ruleset",
			"helpful_commands": []string{
				"get_ruleset - View all existing rules and their IDs",
				"search_components - Search for existing rule patterns",
			},
		})
	}

	// Create a temporary ruleset with the new rule to do complete validation
	updatedXML, err := addRuleToXML(currentRawConfig, request.RuleRaw)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Do complete ruleset validation including rule complexity check
	// This will validate:
	// - XML syntax and structure
	// - Rule logic and conditions
	// - Plugin references and parameters
	// - Threshold configurations
	// - Checklist nodes and conditions
	// - All rule dependencies and constraints
	tempRuleset, err := rules_engine.NewRuleset("", updatedXML, "temp_validation_"+rulesetId)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":            fmt.Sprintf("❌ RULE VALIDATION FAILED: Advanced validation check failed.\n\n🔧 VALIDATION ERROR DETAILS:\n%s\n\n📋 COMMON ISSUES TO CHECK:\n- Plugin references: Ensure all referenced plugins exist\n- Field names: Verify field names match your data schema\n- Node types: Use valid node types (EQU, INCL, REGEX, etc.)\n- Threshold syntax: Check group_by fields and time ranges\n- Logic operators: Verify condition expressions use correct syntax\n\n💡 DEBUGGING STEPS:\n1. Use 'get_ruleset_syntax_guide' for syntax reference\n2. Use 'get_samplers_data' to verify available fields\n3. Test individual components with simpler rules first", err.Error()),
			"validation_stage": "complete_ruleset_check",
			"helpful_commands": []string{
				"get_ruleset_syntax_guide - Complete syntax reference and best practices",
				"get_samplers_data - View available data fields and formats",
				"get_rule_templates - See working examples of different rule types",
				"test_ruleset_content - Test your rule with sample data",
			},
			"suggestion": "Review the detailed error above and consult the syntax guide for proper formatting",
		})
	}

	// Clean up the temporary ruleset (it was only for validation)
	if tempRuleset != nil {
		// The NewRuleset call above already did full validation, no need to start/stop
		tempRuleset = nil
	}

	// If we reach here, the rule is completely valid
	// Now save to temp file and update memory using existing logic
	tempPath, _ := GetComponentPath("ruleset", rulesetId, true)
	err = WriteComponentFile(tempPath, updatedXML)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save updated ruleset: " + err.Error()})
	}

	// Update memory
	common.GlobalMu.Lock()
	project.GlobalProject.RulesetsNew[rulesetId] = updatedXML
	common.GlobalMu.Unlock()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":  "✅ Rule added successfully to temporary file",
		"rule_id":  ruleId,
		"was_temp": isTemp,
		"status":   "pending",
		"next_steps": map[string]interface{}{
			"1": "Check pending changes: Use 'get_pending_changes' to see all changes awaiting deployment",
			"2": "Apply changes: Use 'apply_changes' to deploy the rule to production environment",
			"3": "Test rule: Use 'test_ruleset' with sample data to validate rule behavior",
		},
		"important_note": "⚠️ This rule is currently in a temporary file and is NOT ACTIVE in production yet. You must apply changes to activate it.",
		"helpful_commands": []string{
			"get_pending_changes - View all changes waiting for deployment",
			"apply_changes - Deploy all pending changes to production",
			"test_ruleset - Test the ruleset with sample data",
		},
	})
}

// Helper functions for XML manipulation

// removeRuleFromXML removes a rule with the specified ID from the XML
func removeRuleFromXML(xmlContent, ruleId string) (string, error) {
	// Find and remove the rule element
	lines := strings.Split(xmlContent, "\n")
	var result []string
	skipMode := false
	found := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Check if this line starts a rule with the target ID
		if strings.Contains(line, "<rule") && strings.Contains(line, fmt.Sprintf(`id="%s"`, ruleId)) {
			found = true
			// Check if it's a self-closing tag
			if strings.Contains(line, "/>") {
				// Self-closing tag, skip this line only
				continue
			} else {
				// Multi-line rule, enter skip mode
				skipMode = true
				continue
			}
		}

		// If in skip mode, skip lines until we find the closing tag
		if skipMode {
			if strings.Contains(line, "</rule>") {
				skipMode = false
			}
			continue
		}

		// Add the line to result if not skipping
		result = append(result, line)
	}

	if !found {
		return "", fmt.Errorf("rule with id '%s' not found", ruleId)
	}

	return strings.Join(result, "\n"), nil
}

// ruleExistsInXML checks if a rule with the specified ID exists in the XML
func ruleExistsInXML(xmlContent, ruleId string) bool {
	lines := strings.Split(xmlContent, "\n")
	for _, line := range lines {
		if strings.Contains(line, "<rule") && strings.Contains(line, fmt.Sprintf(`id="%s"`, ruleId)) {
			return true
		}
	}
	return false
}

// validateAndExtractRuleId validates rule XML and extracts the rule ID
func validateAndExtractRuleId(ruleRaw string) (string, error) {
	// Create a temporary ruleset XML with just this rule to validate it
	tempXML := fmt.Sprintf(`<root>
	%s
</root>`, ruleRaw)

	// Parse to check XML syntax
	var tempRuleset struct {
		Rules []struct {
			ID   string `xml:"id,attr"`
			Name string `xml:"name,attr"`
		} `xml:"rule"`
	}

	if err := xml.Unmarshal([]byte(tempXML), &tempRuleset); err != nil {
		return "", fmt.Errorf("❌ RULE VALIDATION FAILED: XML syntax error.\n\n🔧 HOW TO FIX: Check your XML structure for common issues:\n- Missing closing tags\n- Unescaped special characters (< > & \" ')\n- Incorrect attribute quotes\n- Malformed tag structure\n\n🐛 XML ERROR: %v\n\n📝 VALID EXAMPLE:\n<rule id=\"example_rule\" name=\"Detailed description here\">\n    <filter field=\"event_type\">security</filter>\n    <checklist>\n        <node type=\"INCL\" field=\"command\">suspicious_command</node>\n    </checklist>\n</rule>\n\n💡 Use an XML validator to check your syntax before submitting!", err)
	}

	if len(tempRuleset.Rules) != 1 {
		return "", fmt.Errorf("❌ RULE VALIDATION FAILED: Expected exactly one rule, got %d.\n\n🔧 HOW TO FIX: When adding a single rule, provide exactly one <rule> element.\n\n📝 CORRECT FORMAT:\n<rule id=\"unique_id\" name=\"Descriptive name\">\n    <!-- rule content -->\n</rule>\n\n❌ AVOID: Multiple <rule> elements or no <rule> elements in a single addition.", len(tempRuleset.Rules))
	}

	ruleId := strings.TrimSpace(tempRuleset.Rules[0].ID)
	if ruleId == "" {
		return "", fmt.Errorf("❌ RULE VALIDATION FAILED: Missing rule ID.\n\n🔧 HOW TO FIX: Add a unique 'id' attribute to your rule.\n\n📝 CORRECT FORMAT:\n<rule id=\"unique_descriptive_id\" name=\"Detailed description\">\n\n💡 GOOD ID EXAMPLES:\n- \"detect_ssh_brute_force\"\n- \"monitor_privilege_escalation\"\n- \"alert_sql_injection_attempts\"\n\n⚠️ The ID must be unique within the ruleset and descriptive of the rule's purpose.")
	}

	// Validate rule name (description) for LLM compliance
	ruleName := strings.TrimSpace(tempRuleset.Rules[0].Name)
	if ruleName == "" {
		return "", fmt.Errorf("❌ RULE VALIDATION FAILED: Missing 'name' attribute. \n\n🔧 HOW TO FIX: Add a detailed 'name' attribute to your rule that explains what it detects and why it's important.\n\n📝 EXAMPLE: <rule id=\"%s\" name=\"Detect suspicious bash reverse shell execution patterns from compromised accounts\">\n\n💡 The 'name' should be descriptive enough for team members to understand the rule's purpose without reading the technical details.", ruleId)
	}

	// Check for meaningful description (not just simple words)
	if len(ruleName) < 10 {
		return "", fmt.Errorf("❌ RULE VALIDATION FAILED: Rule name too short.\n\n🔧 HOW TO FIX: The 'name' attribute should contain a comprehensive description (at least 10 characters). Current name: '%s'\n\n📝 GOOD EXAMPLES:\n- \"Detect SQL injection attempts in web application logs\"\n- \"Monitor for suspicious PowerShell execution patterns\"\n- \"Alert on unusual network connections from endpoints\"\n\n💡 Make it descriptive so others understand what the rule does!", ruleName)
	}

	// Check for common non-descriptive patterns
	lowercaseName := strings.ToLower(ruleName)
	nonDescriptivePatterns := []string{
		"test", "rule", "check", "detect", "monitor", "alert", "filter",
		"validation", "basic", "simple", "default", "example", "demo",
	}

	for _, pattern := range nonDescriptivePatterns {
		if lowercaseName == pattern || lowercaseName == pattern+" rule" {
			return "", fmt.Errorf("❌ RULE VALIDATION FAILED: Generic rule name detected.\n\n🔧 HOW TO FIX: Instead of using generic terms like '%s', provide a specific description of what the rule detects.\n\n📝 GOOD EXAMPLES:\n- Instead of 'test rule' → 'Test for unauthorized SSH key installation attempts'\n- Instead of 'detection' → 'Detect cryptocurrency mining processes on endpoints'\n- Instead of 'alert' → 'Alert on failed authentication patterns indicating brute force attacks'\n\n💡 Be specific about the security threat or behavior being monitored!", ruleName)
		}
	}

	// Check for reasonable content (should contain some explanation words)
	meaningfulWords := []string{"detect", "monitor", "identify", "prevent", "block", "alert", "track", "analyze", "security", "threat", "suspicious", "malicious", "unauthorized", "anomaly", "intrusion", "attack", "vulnerability", "compliance", "policy", "behavior", "pattern", "activity", "event", "access", "authentication", "authorization", "login", "failure", "success", "error", "warning", "critical", "high", "risk", "dangerous", "forbidden", "restricted", "allowed", "permitted", "denied", "blocked", "filtered", "whitelist", "blacklist", "system", "network", "endpoint", "server", "client", "user", "admin", "privilege", "escalation", "injection", "overflow", "exploit", "malware", "virus", "ransomware", "phishing", "fraud", "breach", "incident", "response"}

	hasmeaningfulContent := false
	for _, word := range meaningfulWords {
		if strings.Contains(lowercaseName, word) {
			hasmeaningfulContent = true
			break
		}
	}

	// Also check for common describing words like "when", "if", "for", "that", "which", etc.
	describingWords := []string{"when", "if", "for", "that", "which", "where", "how", "what", "why", "during", "after", "before", "while", "upon", "attempts", "tries", "executes", "performs", "contains", "includes", "matches", "exceeds", "violates", "indicates", "suggests", "shows", "reveals", "from", "to", "in", "on", "at", "with", "by", "through", "via", "using", "based", "related", "associated", "connected", "linked", "involving", "regarding", "concerning", "about"}

	if !hasmeaningfulContent {
		for _, word := range describingWords {
			if strings.Contains(lowercaseName, word) {
				hasmeaningfulContent = true
				break
			}
		}
	}

	if !hasmeaningfulContent {
		return "", fmt.Errorf("❌ RULE VALIDATION FAILED: Rule name lacks meaningful security context.\n\n🔧 HOW TO FIX: Your rule name '%s' should explain the specific security purpose and detection logic.\n\n📝 IMPROVED EXAMPLES:\n- Add security keywords: 'intrusion', 'malicious', 'unauthorized', 'suspicious'\n- Specify what you're detecting: 'failed logins', 'code injection', 'privilege escalation'\n- Include the context: 'from external IPs', 'on critical systems', 'during off-hours'\n\n💡 GOOD PATTERN: 'Detect [specific threat] when [condition] indicates [security concern]'\n\n🎯 EXAMPLE: 'Detect reverse shell connections when bash processes contain suspicious command line patterns indicating potential compromise'", ruleName)
	}

	return ruleId, nil
}

// addRuleToXML adds a new rule to the XML before the closing </root> tag
func addRuleToXML(xmlContent, ruleRaw string) (string, error) {
	// Find the position of </root> and insert the rule before it
	lines := strings.Split(xmlContent, "\n")
	var result []string
	inserted := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// If we find the closing root tag, insert the rule before it
		if strings.Contains(line, "</root>") && !inserted {
			// Add the rule with proper indentation
			ruleLines := strings.Split(ruleRaw, "\n")
			for _, ruleLine := range ruleLines {
				if strings.TrimSpace(ruleLine) != "" {
					result = append(result, "    "+ruleLine)
				}
			}
			inserted = true
		}

		result = append(result, line)
	}

	if !inserted {
		return "", fmt.Errorf("could not find closing </root> tag to insert rule")
	}

	return strings.Join(result, "\n"), nil
}
