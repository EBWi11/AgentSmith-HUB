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
  group: test`

const NewOutputData = `type: kafka
kafka:
  brokers:
    - "192.168.27.130:9092"
  topic: "kafka_output_demo"`

const NewRulesetData = `<root author="name">
    <rule id="rule_id" name="name">
        <check type="REGEX" field="exe">testcases</check>
        
        <threshold group_by="src_ip" range="1m" value="100" />
        
        <check type="INCL" field="exe" logic="OR" delimiter="|">abc|edf</check>
        <check type="PLUGIN" field="exe">isPrivateIP("127.0.0.1")</check>
        
        <append field="abc">123</append>
        <del>exe,argv</del>
        
        <checklist condition="a or b">
            <check id="a" type="EQU" field="status">200</check>
            <check id="b" type="INCL" field="path">favicon.ico</check>
        </checklist>
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
				project.SetInputNew(id, content)
			case "output":
				project.SetOutputNew(id, content)
			case "ruleset":
				project.SetRulesetNew(id, content)
			case "project":
				project.SetProjectNew(id, content)
			case "plugin":
				plugin.SetPluginNew(id, content)
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
	result := make([]map[string]interface{}, 0)

	// Create a map to track processed IDs
	processedIDs := make(map[string]bool)

	// Use safe accessor to iterate over projects
	project.ForEachProject(func(projId string, proj *project.Project) bool {
		// Check if there is a temporary file
		tempRaw, hasTemp := project.GetProjectNew(proj.Id)

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
		if proj.Status == common.StatusError && proj.Err != nil {
			projectData["error"] = proj.Err.Error()
			projectData["errorMessage"] = proj.Err.Error() // Add for consistency with other components
		}

		result = append(result, projectData)
		processedIDs[proj.Id] = true
		return true
	})

	// Add components that only exist in temporary files
	allProjectsNew := project.GetAllProjectsNew()
	for id, tempRaw := range allProjectsNew {
		if !processedIDs[id] {
			projectData := map[string]interface{}{
				"id":      id,
				"status":  common.StatusStopped,
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

	p_raw, ok := project.GetProjectNew(id)
	if ok {
		tempPath, _ := GetComponentPath("project", id, true)
		// Get sample data for this project (for MCP interface optimization)
		sampleData, dataSource, err := getSampleDataForProject(id)
		response := map[string]interface{}{
			"id":     id,
			"status": common.StatusStopped,
			"raw":    p_raw,
			"path":   tempPath,
		}
		if err == nil && len(sampleData) > 0 {
			response["sample_data"] = sampleData
			response["data_source"] = dataSource
		}
		return c.JSON(http.StatusOK, response)
	}

	p, exists := project.GetProject(id)
	if !exists {
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
	rulesets := make([]map[string]interface{}, 0)

	// Create a map to track processed IDs
	processedIDs := make(map[string]bool)

	// Helper function to find which projects use a ruleset
	findProjectsUsingRuleset := func(rulesetId string) []string {
		projects := make([]string, 0)
		project.ForEachProject(func(projId string, proj *project.Project) bool {
			if _, exists := proj.Rulesets[rulesetId]; exists {
				projects = append(projects, proj.Id)
			}
			return true
		})
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
				// Convert line to lowercase for case-insensitive matching
				lowerLine := strings.ToLower(line)
				if strings.Contains(lowerLine, `type="detection"`) || strings.Contains(lowerLine, `type='detection'`) {
					return "detection"
				} else if strings.Contains(lowerLine, `type="exclude"`) || strings.Contains(lowerLine, `type='exclude'`) {
					return "exclude"
				}
			}
		}
		return "detection" // default
	}

	// Use safe accessor to iterate over rulesets
	project.ForEachRuleset(func(rulesetId string, r *rules_engine.Ruleset) bool {
		// Check if there is a temporary file
		tempRaw, hasTemp := project.GetRulesetNew(r.RulesetID)

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
			"status":           string(r.Status),
		}

		// Include error information if component has errors
		if r.Status == common.StatusError && r.Err != nil {
			rulesetData["errorMessage"] = r.Err.Error()
		}

		// Include path information if available
		if r.Path != "" {
			rulesetData["path"] = r.Path
		}

		rulesets = append(rulesets, rulesetData)
		processedIDs[r.RulesetID] = true
		return true
	})

	// Add components that only exist in temporary files
	allRulesetsNew := project.GetAllRulesetsNew()
	for id, tempRaw := range allRulesetsNew {
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

	r_raw, ok := project.GetRulesetNew(id)
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

	r, exists := project.GetRuleset(id)

	if exists {
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
	inputs := make([]map[string]interface{}, 0)

	// Create a map to track processed IDs
	processedIDs := make(map[string]bool)

	// Helper function to find which projects use an input
	findProjectsUsingInput := func(inputId string) []string {
		projects := make([]string, 0)
		project.ForEachProject(func(projId string, proj *project.Project) bool {
			if _, exists := proj.Inputs[inputId]; exists {
				projects = append(projects, proj.Id)
			}
			return true
		})
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

	// Use safe accessor to iterate over inputs
	project.ForEachInput(func(inputId string, in *input.Input) bool {
		// Check if there is a temporary file
		tempRaw, hasTemp := project.GetInputNew(in.Id)

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
			"status":           string(in.Status),
		}

		// Include error information if component has errors
		if in.Status == common.StatusError && in.Err != nil {
			inputData["errorMessage"] = in.Err.Error()
		}

		// Include path information if available
		if in.Path != "" {
			inputData["path"] = in.Path
		}

		inputs = append(inputs, inputData)
		processedIDs[in.Id] = true
		return true
	})

	// Add components that only exist in temporary files
	allInputsNew := project.GetAllInputsNew()
	for id, tempRaw := range allInputsNew {
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
	in_raw, ok := project.GetInputNew(id)
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

	in, exists := project.GetInput(id)

	if exists {
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

// getPlugins returns plugin information with configurable detail level
// Query parameters:
//   - detailed: "true" for full information, "false" for simple format (default: false)
//   - include_temp: "true" to include temporary plugins, "false" to exclude (default: true)
//   - include_usage: "true" to include usage analysis, "false" to exclude (default: true)
//   - type: filter by plugin type - "local", "yaegi", "all" (default: all)
//
// Examples:
//   - /plugins?detailed=true - Full plugin management interface
//   - /plugins?detailed=false&include_temp=false&type=yaegi - Simple plugin list for testing
func getPlugins(c echo.Context) error {
	// Get query parameters to control response detail
	detailed := c.QueryParam("detailed") == "true"
	includeTemp := c.QueryParam("include_temp") != "false"   // Default to true
	includeUsage := c.QueryParam("include_usage") != "false" // Default to true
	pluginType := c.QueryParam("type")                       // Filter by plugin type: "local", "yaegi", "all" (default)

	plugins := make([]map[string]interface{}, 0)

	// Create a map to track processed names
	processedNames := make(map[string]bool)

	// Helper function to find which rulesets use a plugin (only if needed)
	findRulesetsUsingPlugin := func(pluginName string) []string {
		if !detailed || !includeUsage {
			return []string{}
		}

		rulesets := make([]string, 0)
		project.ForEachRuleset(func(rulesetId string, r *rules_engine.Ruleset) bool {
			// Check if plugin is used in any rule within this ruleset
			for _, rule := range r.Rules {
				// Check in checklist nodes
				for _, checklist := range rule.ChecklistMap {
					for _, node := range checklist.CheckNodes {
						if node.Type == "PLUGIN" && strings.Contains(node.Value, pluginName+"(") {
							rulesets = append(rulesets, r.RulesetID)
							return true // Continue to next ruleset
						}
					}
				}
				// Check in standalone check nodes
				for _, node := range rule.CheckMap {
					if node.Type == "PLUGIN" && strings.Contains(node.Value, pluginName+"(") {
						rulesets = append(rulesets, r.RulesetID)
						return true // Continue to next ruleset
					}
				}
				// Check in append elements
				for _, appendElem := range rule.AppendsMap {
					if appendElem.Type == "PLUGIN" && strings.Contains(appendElem.Value, pluginName+"(") {
						rulesets = append(rulesets, r.RulesetID)
						return true // Continue to next ruleset
					}
				}
				// Check in plugin elements
				for _, pluginElem := range rule.PluginMap {
					if strings.Contains(pluginElem.Value, pluginName+"(") {
						rulesets = append(rulesets, r.RulesetID)
						return true // Continue to next ruleset
					}
				}
			}
			return true // Continue to next ruleset
		})
		return rulesets
	}

	// Helper function to extract plugin description from code
	extractPluginDescription := func(code string) string {
		if !detailed {
			return ""
		}

		// Try to find description in comments
		lines := strings.Split(code, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "//") {
				desc := strings.TrimSpace(strings.TrimPrefix(line, "//"))
				if desc != "" && !strings.HasPrefix(desc, "Package") && !strings.HasPrefix(desc, "import") {
					return desc
				}
			}
		}

		// If no suitable comment is found, return default description
		return "Plugin function"
	}

	for _, p := range plugin.Plugins {
		// Filter by plugin type if specified
		var currentPluginType string
		if p.Type == plugin.LOCAL_PLUGIN {
			currentPluginType = "local"
		} else if p.Type == plugin.YAEGI_PLUGIN {
			currentPluginType = "yaegi"
		} else {
			currentPluginType = "unknown"
		}

		// Skip if type filter is specified and doesn't match
		if pluginType != "" && pluginType != "all" && pluginType != currentPluginType {
			continue
		}

		// Check if there is a temporary file
		tempRaw, hasTemp := plugin.PluginsNew[p.Name]

		// Skip temporary plugins if not requested
		if !includeTemp && hasTemp {
			continue
		}

		// Get raw configuration (prioritize temp if exists)
		rawConfig := string(p.Payload)
		if hasTemp {
			rawConfig = tempRaw
		}

		// Create plugin data based on detail level
		var pluginData map[string]interface{}

		if detailed {
			// Find rulesets using this plugin
			usedByRulesets := findRulesetsUsingPlugin(p.Name)

			pluginData = map[string]interface{}{
				"id":               p.Name,            // Use id field for consistency with other components
				"name":             p.Name,            // Keep name for backward compatibility
				"type":             currentPluginType, // Convert to string type for frontend differentiation
				"hasTemp":          hasTemp,
				"raw":              rawConfig,
				"returnType":       p.ReturnType, // Include return type for filtering
				"parameters":       p.Parameters, // Include parameter information
				"used_by_rulesets": usedByRulesets,
				"ruleset_count":    len(usedByRulesets),
				"status":           string(p.Status), // Add status for error handling
			}

			// Include error information if plugin has errors
			if p.Status == common.StatusError && p.Err != nil {
				pluginData["errorMessage"] = p.Err.Error()
			}

			// Include path information if available
			if p.Path != "" {
				pluginData["path"] = p.Path
			}
		} else {
			// Simple format for basic usage
			pluginData = map[string]interface{}{
				"name":        p.Name,
				"description": extractPluginDescription(rawConfig),
			}
		}

		plugins = append(plugins, pluginData)
		processedNames[p.Name] = true
	}

	// Add plugins that only exist in temporary files (only if including temp and detailed)
	if includeTemp && detailed {
		for name, content := range plugin.PluginsNew {
			if !processedNames[name] {
				// Try to determine return type for temporary plugins
				returnType := "unknown"
				parameters := []plugin.PluginParameter{}
				if content != "" {
					// Create a temporary plugin instance to get return type
					tempPlugin := &plugin.Plugin{
						Name:       name,
						Payload:    []byte(content),
						Type:       plugin.YAEGI_PLUGIN,
						IsTestMode: true, // Set test mode to avoid statistics recording
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
			// Get description from LocalPluginDesc
			desc := local_plugin.LocalPluginDesc[id]
			if desc == "" {
				desc = "Built-in plugin (source unavailable)"
			}
			// For built-in plugins, only show description, not source code
			rawContent = fmt.Sprintf("// Built-in Plugin: %s\n// %s\n", id, desc)
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
	outputs := make([]map[string]interface{}, 0)

	// Create a map to track processed IDs
	processedIDs := make(map[string]bool)

	// Helper function to find which projects use an output
	findProjectsUsingOutput := func(outputId string) []string {
		projects := make([]string, 0)
		project.ForEachProject(func(projId string, proj *project.Project) bool {
			if _, exists := proj.Outputs[outputId]; exists {
				projects = append(projects, proj.Id)
			}
			return true
		})
		return projects
	}

	// Use safe accessor to iterate over outputs
	project.ForEachOutput(func(outputId string, out *output.Output) bool {
		// Check if there is a temporary file
		tempRaw, hasTemp := project.GetOutputNew(out.Id)

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
			"status":           string(out.Status),
		}

		// Include error information if component has errors
		if out.Status == common.StatusError && out.Err != nil {
			outputData["errorMessage"] = out.Err.Error()
		}

		// Include path information if available
		if out.Path != "" {
			outputData["path"] = out.Path
		}

		outputs = append(outputs, outputData)
		processedIDs[out.Id] = true
		return true
	})

	// Add components that only exist in temporary files
	allOutputsNew := project.GetAllOutputsNew()
	for id, rawConfig := range allOutputsNew {
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
	out_raw, ok := project.GetOutputNew(id)
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

	out, exists := project.GetOutput(id)

	if exists {
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

	// Only use default templates if no raw content is provided or content is effectively empty
	if request.Raw == "" || strings.TrimSpace(request.Raw) == "" {
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
	}

	// Write file without lock (file system operations are atomic)
	err := WriteComponentFile(filtPath, request.Raw)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	switch componentType {
	case "plugin":
		plugin.SetPluginNew(request.ID, request.Raw)
	case "input":
		project.SetInputNew(request.ID, request.Raw)
	case "output":
		project.SetOutputNew(request.ID, request.Raw)
	case "ruleset":
		project.SetRulesetNew(request.ID, request.Raw)
	case "project":
		project.SetProjectNew(request.ID, request.Raw)
	}

	// Publish instruction for component creation (NEW - this was missing!)
	if common.IsCurrentNodeLeader() && cluster.GlobalInstructionManager != nil {
		if err := cluster.GlobalInstructionManager.PublishComponentAdd(componentType, request.ID, request.Raw); err != nil {
			logger.Error("Failed to publish component creation instruction", "type", componentType, "id", request.ID, "error", err)
			// Don't fail the request, but log the error
		}
		// Record component creation operation history (for leader visibility)
		common.RecordComponentAdd(componentType, request.ID, request.Raw, "success", "")
	}

	// Create enhanced response with deployment guidance
	componentTypeName := strings.ToTitle(componentType)

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message":      fmt.Sprintf("✅ %s created successfully in temporary file", componentTypeName),
		"component_id": request.ID,
		"status":       "pending",
		"file_type":    "temporary",
		"next_steps": map[string]interface{}{
			"1": "review changes with 'get_pending_changes'",
			"2": "deploy with 'apply_changes'",
			"3": fmt.Sprintf("test %s functionality", componentType),
		},
		"important_note": fmt.Sprintf("⚠️ %s is in temporary file, not yet active", componentType),
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

	var suffix string
	var dir string

	switch componentType {
	case "ruleset":
		suffix = ".xml"
		dir = "ruleset"
	case "input":
		suffix = ".yaml"
		dir = "input"
	case "output":
		suffix = ".yaml"
		dir = "output"
	case "project":
		suffix = ".yaml"
		dir = "project"
	case "plugin":
		suffix = ".go"
		dir = "plugin"
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid component type",
		})
	}

	configRoot := common.Config.ConfigRoot
	componentPath := filepath.Join(configRoot, dir, id+suffix)
	tempPath := filepath.Join(configRoot, dir, id+suffix+".new")

	// Check if files exist
	_, formalErr := os.Stat(componentPath)
	_, tempErr := os.Stat(tempPath)
	formalExists := formalErr == nil
	tempExists := tempErr == nil

	if !formalExists && !tempExists {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("%s not found", componentType),
		})
	}

	// Safely delete component using encapsulated functions
	var affectedProjects []string
	var deletionErr error

	switch componentType {
	case "ruleset":
		affectedProjects, deletionErr = project.SafeDeleteRuleset(id)
	case "input":
		affectedProjects, deletionErr = project.SafeDeleteInput(id)
	case "output":
		affectedProjects, deletionErr = project.SafeDeleteOutput(id)
	case "project":
		affectedProjects, deletionErr = project.SafeDeleteProject(id)
	case "plugin":
		affectedProjects, deletionErr = plugin.SafeDeletePlugin(id)
	default:
		deletionErr = fmt.Errorf("unsupported component type: %s", componentType)
	}

	if deletionErr != nil {
		// Record failed deletion operation
		RecordComponentDelete(componentType, id, "failed", deletionErr.Error(), []string{})
		return c.JSON(http.StatusConflict, map[string]string{
			"error": deletionErr.Error(),
		})
	}

	// Get the affected projects from the safe delete operation for cluster notification

	// If it's leader node, delete files and notify followers
	if common.IsCurrentNodeLeader() {
		// Use affected projects from safe delete operation (already calculated)

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
		}

		// Publish deletion instruction regardless of which files existed
		// This ensures followers are notified of the deletion
		if componentType == "project" {
			// Delete project config from Redis
			if err := common.DeleteProjectConfig(id); err != nil {
				logger.Warn("Failed to delete project config from Redis", "project", id, "error", err)
			}

			// Publish project deletion instruction
			if err := cluster.GlobalInstructionManager.PublishComponentDelete("project", id, []string{id}); err != nil {
				logger.Error("Failed to publish project deletion instruction", "project", id, "error", err)
			}
		} else {
			// For other components, publish deletion instruction with affected projects
			if err := cluster.GlobalInstructionManager.PublishComponentDelete(componentType, id, affectedProjects); err != nil {
				logger.Error("Failed to publish component deletion instruction", "type", componentType, "id", id, "error", err)
			}

			// Restart affected projects
			if len(affectedProjects) > 0 {
				if err := cluster.GlobalInstructionManager.PublishProjectsRestart(affectedProjects, "component_deleted"); err != nil {
					logger.Error("Failed to publish project restart instructions", "affected_projects", affectedProjects, "error", err)
				}
			}
		}

		// Record successful deletion operation
		RecordComponentDelete(componentType, id, "success", "", affectedProjects)
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
	id := c.Param("id")

	// API-side persistence: Remove project from Redis when deleted
	hashKey := "cluster:proj_states:" + common.Config.LocalIP
	if err := common.RedisHDel(hashKey, id); err != nil {
		logger.Warn("Failed to remove project state from Redis during deletion", "project", id, "error", err)
	}

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

	// First check memory (like getProject and getPlugin functions)
	var originalContent string
	var err error
	var tempPath string
	var tempExists bool

	// Check memory first, then fall back to file system
	switch componentType {
	case "plugin":
		// Check if plugin exists in memory
		if p, ok := plugin.Plugins[id]; ok {
			originalContent = string(p.Payload)
		} else if p_raw, ok := plugin.PluginsNew[id]; ok {
			originalContent = p_raw
		} else {
			// Fall back to file system check
			formalPath, formalExists := GetComponentPath(componentType, id, false)
			tempPath, tempExists = GetComponentPath(componentType, id, true)

			if formalExists {
				originalContent, err = ReadComponent(formalPath)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read original file: " + err.Error()})
				}
			} else if tempExists {
				originalContent, err = ReadComponent(tempPath)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read temporary file: " + err.Error()})
				}
			} else {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "component config not found"})
			}
		}
	case "project":
		// Check if project exists in memory
		if p, exists := project.GetProject(id); exists {
			originalContent = p.Config.RawConfig
		} else if p_raw, ok := project.GetProjectNew(id); ok {
			originalContent = p_raw
		} else {
			// Fall back to file system check
			formalPath, formalExists := GetComponentPath(componentType, id, false)
			tempPath, tempExists = GetComponentPath(componentType, id, true)

			if formalExists {
				originalContent, err = ReadComponent(formalPath)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read original file: " + err.Error()})
				}
			} else if tempExists {
				originalContent, err = ReadComponent(tempPath)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read temporary file: " + err.Error()})
				}
			} else {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "component config not found"})
			}
		}
	case "input":
		// Check if input exists in memory
		if i, exists := project.GetInput(id); exists {
			originalContent = i.Config.RawConfig
		} else if i_raw, ok := project.GetInputNew(id); ok {
			originalContent = i_raw
		} else {
			// Fall back to file system check
			formalPath, formalExists := GetComponentPath(componentType, id, false)
			tempPath, tempExists = GetComponentPath(componentType, id, true)

			if formalExists {
				originalContent, err = ReadComponent(formalPath)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read original file: " + err.Error()})
				}
			} else if tempExists {
				originalContent, err = ReadComponent(tempPath)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read temporary file: " + err.Error()})
				}
			} else {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "component config not found"})
			}
		}
	case "output":
		// Check if output exists in memory
		if o, exists := project.GetOutput(id); exists {
			originalContent = o.Config.RawConfig
		} else if o_raw, ok := project.GetOutputNew(id); ok {
			originalContent = o_raw
		} else {
			// Fall back to file system check
			formalPath, formalExists := GetComponentPath(componentType, id, false)
			tempPath, tempExists = GetComponentPath(componentType, id, true)

			if formalExists {
				originalContent, err = ReadComponent(formalPath)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read original file: " + err.Error()})
				}
			} else if tempExists {
				originalContent, err = ReadComponent(tempPath)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read temporary file: " + err.Error()})
				}
			} else {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "component config not found"})
			}
		}
	case "ruleset":
		// Check if ruleset exists in memory
		if r, exists := project.GetRuleset(id); exists {
			originalContent = r.RawConfig
		} else if r_raw, ok := project.GetRulesetNew(id); ok {
			originalContent = r_raw
		} else {
			// Fall back to file system check
			formalPath, formalExists := GetComponentPath(componentType, id, false)
			tempPath, tempExists = GetComponentPath(componentType, id, true)

			if formalExists {
				originalContent, err = ReadComponent(formalPath)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read original file: " + err.Error()})
				}
			} else if tempExists {
				originalContent, err = ReadComponent(tempPath)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read temporary file: " + err.Error()})
				}
			} else {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "component config not found"})
			}
		}
	default:
		return c.JSON(http.StatusNotFound, map[string]string{"error": "component config not found"})
	}

	// Compare content with original file
	newContent := strings.TrimSpace(req.Raw)
	originalContentTrimmed := strings.TrimSpace(originalContent)

	if newContent == originalContentTrimmed {
		return c.JSON(http.StatusOK, map[string]string{"message": "content identical to original file, no changes needed"})
	}

	// Content is different, create or update temporary file
	tempPath, _ = GetComponentPath(componentType, id, true)
	err = WriteComponentFile(tempPath, req.Raw)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to write config file: " + err.Error()})
	}

	switch componentType {
	case "input":
		project.SetInputNew(id, req.Raw)
	case "output":
		project.SetOutputNew(id, req.Raw)
	case "ruleset":
		project.SetRulesetNew(id, req.Raw)
	case "project":
		project.SetProjectNew(id, req.Raw)
	case "plugin":
		plugin.SetPluginNew(id, req.Raw)
	}

	// Record component update operation history (for leader visibility)
	if common.IsCurrentNodeLeader() {
		common.RecordComponentUpdate(componentType, id, req.Raw, "success", "")
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
			"1": "review changes with 'get_pending_changes'",
			"2": "deploy with 'apply_changes'",
			"3": fmt.Sprintf("test updated %s functionality", componentType),
		},
		"important_note": fmt.Sprintf("⚠️ %s is in temporary file, not yet active", componentType),
		"helpful_commands": []string{
			"get_pending_changes - View all changes waiting for deployment",
			"apply_changes - Deploy all pending changes to production",
			fmt.Sprintf("verify_component - Validate %s configuration", componentType),
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
	project.DeleteProjectNew(id)

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
	project.DeleteRulesetNew(id)

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
	project.DeleteInputNew(id)

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
	project.DeleteOutputNew(id)

	// Delete temp file if exists (without holding lock)
	tempPath, tempExists := GetComponentPath("output", id, true)
	if tempExists {
		_ = os.Remove(tempPath)
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "output upgrade cancelled"})
}

func cancelPluginUpgrade(c echo.Context) error {
	id := c.Param("id")

	// Use safe accessor for memory operations
	plugin.DeletePluginNew(id)

	// Delete temp file if exists
	tempPath, tempExists := GetComponentPath("plugin", id, true)
	if tempExists {
		_ = os.Remove(tempPath)
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "plugin upgrade cancelled"})
}

// GetSamplerData retrieves sample data from project components
func GetSamplerData(c echo.Context) error {
	// Only leader nodes collect sample data for performance reasons
	if !common.IsCurrentNodeLeader() {
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
		_, componentExists = project.GetInput(componentId)
	case "output":
		_, componentExists = project.GetOutput(componentId)
	case "ruleset":
		_, componentExists = project.GetRuleset(componentId)
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
		logger.Info("Component not found for sample data request, returning empty data",
			"componentType", componentType,
			"componentId", componentId,
			"nodeSequence", nodeSequence,
			"message", "This is normal for new components that haven't been saved yet")

		// Return empty data instead of 404 error for new components
		response := map[string]interface{}{
			componentName: map[string][]interface{}{},
		}
		return c.JSON(http.StatusOK, response)
	}

	// Collect samples from all samplers that contain this component in their flow path
	result := make(map[string][]interface{})

	// Get potential sampler names based on component types and IDs
	samplerNames := []string{}
	project.ForEachInput(func(inputId string, _ *input.Input) bool {
		samplerNames = append(samplerNames, "input."+inputId)
		return true
	})
	project.ForEachRuleset(func(rulesetId string, _ *rules_engine.Ruleset) bool {
		samplerNames = append(samplerNames, "ruleset."+rulesetId)
		return true
	})
	project.ForEachOutput(func(outputId string, _ *output.Output) bool {
		samplerNames = append(samplerNames, "output."+outputId)
		return true
	})

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
				if strings.HasSuffix(strings.ToLower(projectNodeSequence), strings.ToLower(nodeSequence)) {
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
	if !common.IsCurrentNodeLeader() {
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

	nodeSequence := strings.ToLower(fmt.Sprintf("ruleset.%s", componentId))
	var allSampleData []map[string]interface{}

	// Get potential sampler names based on component types and IDs
	samplerNames := []string{}
	project.ForEachInput(func(inputId string, _ *input.Input) bool {
		samplerNames = append(samplerNames, "input."+inputId)
		return true
	})
	project.ForEachRuleset(func(rulesetId string, _ *rules_engine.Ruleset) bool {
		samplerNames = append(samplerNames, "ruleset."+rulesetId)
		return true
	})
	project.ForEachOutput(func(outputId string, _ *output.Output) bool {
		samplerNames = append(samplerNames, "output."+outputId)
		return true
	})

	// Collect sample data from all potential sources
	for _, samplerName := range samplerNames {
		sampler := common.GetSampler(samplerName)
		if sampler != nil {
			samples := sampler.GetSamples()
			for projectNodeSequence, sampleDataList := range samples {
				// Match samples that flow into this ruleset
				if strings.HasSuffix(strings.ToLower(projectNodeSequence), strings.ToLower(nodeSequence)) {
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
			Name:       id,
			Payload:    []byte(tempContent),
			Type:       plugin.YAEGI_PLUGIN,
			IsTestMode: true, // Set test mode to avoid statistics recording
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
			componentMap = project.GetAllInputsNew()
		case "output":
			componentMap = project.GetAllOutputsNew()
		case "ruleset":
			componentMap = project.GetAllRulesetsNew()
		case "project":
			componentMap = project.GetAllProjectsNew()
		case "plugin":
			componentMap = plugin.PluginsNew
		}
	} else {
		// For formal files, we need to read from the actual component instances
		componentMap = make(map[string]string)
		switch componentType {
		case "input":
			project.ForEachInput(func(id string, comp *input.Input) bool {
				componentMap[comp.Id] = comp.Config.RawConfig
				return true
			})
		case "output":
			project.ForEachOutput(func(id string, comp *output.Output) bool {
				componentMap[comp.Id] = comp.Config.RawConfig
				return true
			})
		case "ruleset":
			project.ForEachRuleset(func(id string, comp *rules_engine.Ruleset) bool {
				componentMap[comp.RulesetID] = comp.RawConfig
				return true
			})
		case "project":
			project.ForEachProject(func(id string, comp *project.Project) bool {
				componentMap[comp.Id] = comp.Config.RawConfig
				return true
			})
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
	if tempRaw, ok := project.GetRulesetNew(rulesetId); ok {
		currentRawConfig = tempRaw
		isTemp = true
	} else if r, exists := project.GetRuleset(rulesetId); exists {
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
		// Use normal Stop method - even temporary rulesets should process data gracefully
		if err := tempRuleset.Stop(); err != nil {
			logger.Warn("Failed to stop temporary ruleset", "error", err)
		}
		tempRuleset = nil
	}

	// Save to temp file
	tempPath, _ := GetComponentPath("ruleset", rulesetId, true)
	err = WriteComponentFile(tempPath, updatedXML)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save updated ruleset: " + err.Error()})
	}

	// Update memory
	project.SetRulesetNew(rulesetId, updatedXML)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":  "✅ Rule deleted successfully from temporary file",
		"rule_id":  ruleId,
		"was_temp": isTemp,
		"status":   "pending",
		"next_steps": map[string]interface{}{
			"1": "review changes with 'get_pending_changes'",
			"2": "deploy with 'apply_changes'",
			"3": "test ruleset functionality",
		},
		"important_note": "⚠️ Rule deletion is in temporary file, not yet active",
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
			"error":      "missing ruleset ID",
			"suggestion": "use 'get_rulesets' to find available rulesets",
		})
	}

	if request.RuleRaw == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":      "missing rule content",
			"suggestion": "provide complete rule XML in 'rule_raw' field",
		})
	}

	// Validate the rule XML syntax first
	ruleId, ruleName, err := validateAndExtractRuleId(request.RuleRaw)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":      err.Error(),
			"suggestion": "use 'get_ruleset_syntax_guide' for syntax reference",
		})
	}

	// Get current ruleset content (prioritize temp file if exists)
	var currentRawConfig string
	var isTemp bool

	// Check temp file first
	if tempRaw, ok := project.GetRulesetNew(rulesetId); ok {
		currentRawConfig = tempRaw
		isTemp = true
	} else if r, exists := project.GetRuleset(rulesetId); exists {
		currentRawConfig = r.RawConfig
		isTemp = false
	} else {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
	}

	// Check if rule ID already exists in current ruleset
	if ruleExistsInXML(currentRawConfig, ruleId) {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":       fmt.Sprintf("rule ID '%s' already exists", ruleId),
			"conflict_id": ruleId,
			"suggestion":  fmt.Sprintf("try: %s_v2, %s_enhanced, or add timestamp", ruleId, ruleId),
		})
	}

	// Ensure the rule contains an <append field="desc"> element with the rule's descriptive name
	processedRuleRaw := addDescAppendToRule(request.RuleRaw, ruleName)

	// Create a temporary ruleset with the new (possibly augmented) rule to do complete validation
	updatedXML, err := addRuleToXML(currentRawConfig, processedRuleRaw)
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
			"error":      err.Error(),
			"suggestion": "check plugin references, field names, and syntax",
		})
	}

	// Clean up the temporary ruleset (it was only for validation)
	if tempRuleset != nil {
		// Use normal Stop method - even temporary rulesets should process data gracefully
		if err := tempRuleset.Stop(); err != nil {
			logger.Warn("Failed to stop temporary ruleset", "error", err)
		}
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
	project.SetRulesetNew(rulesetId, updatedXML)

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
	// Use regex to find the exact rule with the specified ID
	// This pattern matches: <rule id="exact_id" or <rule id="exact_id" followed by space/other attributes
	rulePattern := fmt.Sprintf(`<rule\s+[^>]*id\s*=\s*"%s"[^>]*>`, regexp.QuoteMeta(ruleId))
	ruleRegex := regexp.MustCompile(rulePattern)

	lines := strings.Split(xmlContent, "\n")
	var result []string
	skipMode := false
	found := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Check if this line contains a rule with the target ID using regex
		if ruleRegex.MatchString(line) {
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
	// Use regex to find the exact rule with the specified ID
	rulePattern := fmt.Sprintf(`<rule\s+[^>]*id\s*=\s*"%s"[^>]*>`, regexp.QuoteMeta(ruleId))
	ruleRegex := regexp.MustCompile(rulePattern)

	lines := strings.Split(xmlContent, "\n")
	for _, line := range lines {
		if ruleRegex.MatchString(line) {
			return true
		}
	}
	return false
}

// validateAndExtractRuleId validates rule XML syntax and returns (ruleID, ruleName)
func validateAndExtractRuleId(ruleRaw string) (string, string, error) {
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
		return "", "", fmt.Errorf("XML syntax error: %v", err)
	}

	if len(tempRuleset.Rules) != 1 {
		return "", "", fmt.Errorf("expected exactly one rule, got %d", len(tempRuleset.Rules))
	}

	ruleId := strings.TrimSpace(tempRuleset.Rules[0].ID)
	ruleName := strings.TrimSpace(tempRuleset.Rules[0].Name)

	if ruleId == "" {
		return "", "", fmt.Errorf("missing rule ID")
	}

	return ruleId, ruleName, nil
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

// addDescAppendToRule ensures the given rule XML contains an <append field="desc"> element.
// If missing, it appends one (static value = ruleName) right before the closing </rule> tag.
// The function returns the possibly-modified rule XML.
func addDescAppendToRule(ruleRaw, ruleName string) string {
	// Quick check – already has desc append?
	lowered := strings.ToLower(ruleRaw)
	if strings.Contains(lowered, "field=\"desc\"") {
		return ruleRaw // nothing to do
	}

	// Escape XML special chars in ruleName
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	safeRuleName := replacer.Replace(ruleName)

	// Construct append snippet (4-space indent by default)
	appendSnippet := fmt.Sprintf("    <append field=\"desc\">%s</append>\n", safeRuleName)

	// Insert before closing </rule>
	closingTag := "</rule>"
	idx := strings.LastIndex(ruleRaw, closingTag)
	if idx == -1 {
		// malformed? fallback to original
		return ruleRaw
	}

	return ruleRaw[:idx] + appendSnippet + ruleRaw[idx:]
}

// GetBatchPluginParameters returns parameter information for multiple plugins
func GetBatchPluginParameters(c echo.Context) error {
	idsParam := c.QueryParam("ids")
	if idsParam == "" {
		// Return all plugins if no specific IDs requested
		result := make(map[string]interface{})

		// Get parameters from loaded plugins only (no temporary plugins)
		for id, p := range plugin.Plugins {
			result[id] = map[string]interface{}{
				"parameters": p.Parameters,
				"returnType": p.ReturnType,
			}
		}

		return c.JSON(http.StatusOK, result)
	}

	// Parse comma-separated IDs
	ids := strings.Split(idsParam, ",")
	result := make(map[string]interface{})

	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		// Check if plugin exists in memory (loaded plugins only)
		if p, exists := plugin.Plugins[id]; exists {
			result[id] = map[string]interface{}{
				"parameters": p.Parameters,
				"returnType": p.ReturnType,
			}
		} else {
			// Plugin not found in loaded plugins
			result[id] = map[string]interface{}{
				"error": "Plugin not found",
			}
		}
	}

	return c.JSON(http.StatusOK, result)
}

// GetBatchRulesetFields returns field information for multiple rulesets
func GetBatchRulesetFields(c echo.Context) error {
	idsParam := c.QueryParam("ids")
	if idsParam == "" {
		// Return all rulesets if no specific IDs requested
		result := make(map[string]interface{})

		// Get field keys from all loaded rulesets
		project.ForEachRuleset(func(id string, _ *rules_engine.Ruleset) bool {
			fields := GetRulesetFieldsForID(id)
			result[id] = fields
			return true
		})

		return c.JSON(http.StatusOK, result)
	}

	// Parse comma-separated IDs
	ids := strings.Split(idsParam, ",")
	result := make(map[string]interface{})

	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		// Check if ruleset exists
		if _, exists := project.GetRuleset(id); exists {
			fields := GetRulesetFieldsForID(id)
			result[id] = fields
		} else {
			// Ruleset not found
			result[id] = map[string]interface{}{
				"error": "Ruleset not found",
			}
		}
	}

	return c.JSON(http.StatusOK, result)
}

// GetRulesetFieldsForID is a helper function to get field keys for a specific ruleset
func GetRulesetFieldsForID(id string) map[string]interface{} {
	// Get sample data for this ruleset
	fieldSet := make(map[string]bool)

	// Try to get sampler data for this ruleset
	samplerName := "ruleset." + id
	sampler := common.GetSampler(samplerName)
	if sampler != nil {
		samples := sampler.GetSamples()
		// Extract field keys from all samples
		for _, sampleList := range samples {
			for _, sample := range sampleList {
				if sample.Data != nil {
					if dataMap, ok := sample.Data.(map[string]interface{}); ok {
						extractKeysFromMap(dataMap, "", fieldSet)
					}
				}
			}
		}
	}

	// Convert to sorted slice
	fieldKeys := make([]string, 0, len(fieldSet))
	for field := range fieldSet {
		fieldKeys = append(fieldKeys, field)
	}
	sort.Strings(fieldKeys)

	return map[string]interface{}{
		"fieldKeys":   fieldKeys,
		"sampleCount": len(fieldKeys),
	}
}

// getPluginUsage returns which rulesets are using a specific plugin
func getPluginUsage(c echo.Context) error {
	pluginID := c.Param("id")
	if pluginID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "plugin ID is required"})
	}

	// Check if plugin exists using safe accessor
	content := getExistingPluginContent(pluginID)
	if content == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "plugin not found"})
	}

	usage := analyzePluginUsage(pluginID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"plugin_id": pluginID,
		"usage":     usage,
	})
}

// PluginUsageInfo represents plugin usage information
type PluginUsageInfo struct {
	UsedByRulesets []RulesetUsageInfo `json:"used_by_rulesets"`
	UsedByProjects []ProjectUsageInfo `json:"used_by_projects"`
	TotalUsage     int                `json:"total_usage"`
}

// RulesetUsageInfo represents how a ruleset uses a plugin
type RulesetUsageInfo struct {
	RulesetID  string   `json:"ruleset_id"`
	UsageCount int      `json:"usage_count"`
	UsageTypes []string `json:"usage_types"` // "checknode", "append"
	RuleIDs    []string `json:"rule_ids"`    // which rules use this plugin
}

// ProjectUsageInfo represents how a project uses a plugin (through rulesets)
type ProjectUsageInfo struct {
	ProjectID     string   `json:"project_id"`
	ProjectStatus string   `json:"project_status"`
	RulesetIDs    []string `json:"ruleset_ids"` // which rulesets in this project use the plugin
}

// analyzePluginUsage analyzes how a plugin is used across rulesets and projects
func analyzePluginUsage(pluginID string) PluginUsageInfo {
	usage := PluginUsageInfo{
		UsedByRulesets: []RulesetUsageInfo{},
		UsedByProjects: []ProjectUsageInfo{},
		TotalUsage:     0,
	}

	// Use safe accessors instead of long-term locking
	// Analyze ruleset usage
	rulesetUsageMap := make(map[string]*RulesetUsageInfo)

	project.ForEachRuleset(func(rulesetID string, ruleset *rules_engine.Ruleset) bool {
		if ruleset == nil || ruleset.RawConfig == "" {
			return true
		}

		rulesetUsage := analyzePluginUsageInRuleset(pluginID, rulesetID, ruleset.RawConfig)
		if rulesetUsage.UsageCount > 0 {
			rulesetUsageMap[rulesetID] = &rulesetUsage
			usage.UsedByRulesets = append(usage.UsedByRulesets, rulesetUsage)
			usage.TotalUsage += rulesetUsage.UsageCount
		}
		return true
	})

	// Analyze project usage (which projects use rulesets that use this plugin)
	projectUsageMap := make(map[string]*ProjectUsageInfo)

	project.ForEachProject(func(projectID string, proj *project.Project) bool {
		if proj == nil {
			return true
		}

		projectUsage := ProjectUsageInfo{
			ProjectID:     projectID,
			ProjectStatus: string(proj.Status),
			RulesetIDs:    []string{},
		}

		// Check which rulesets in this project use the plugin
		for _, rulesetComponent := range proj.Rulesets {
			actualRulesetID := rulesetComponent.RulesetID
			if _, usesPlugin := rulesetUsageMap[actualRulesetID]; usesPlugin {
				projectUsage.RulesetIDs = append(projectUsage.RulesetIDs, actualRulesetID)
			}
		}

		if len(projectUsage.RulesetIDs) > 0 {
			projectUsageMap[projectID] = &projectUsage
			usage.UsedByProjects = append(usage.UsedByProjects, projectUsage)
		}
		return true
	})

	return usage
}

// analyzePluginUsageInRuleset analyzes how a plugin is used in a specific ruleset
func analyzePluginUsageInRuleset(pluginID, rulesetID, rulesetContent string) RulesetUsageInfo {
	usage := RulesetUsageInfo{
		RulesetID:  rulesetID,
		UsageCount: 0,
		UsageTypes: []string{},
		RuleIDs:    []string{},
	}

	// Parse the ruleset XML to find plugin usage
	// Look for plugin usage patterns in the content

	// Pattern 1: Plugin used in checknode - <node type="PLUGIN" field="...">pluginName(...)</node>
	checknodePattern := `<node[^>]*type="PLUGIN"[^>]*>([^<]*` + pluginID + `[^<]*)</node>`
	if matches := regexp.MustCompile(checknodePattern).FindAllString(rulesetContent, -1); len(matches) > 0 {
		usage.UsageCount += len(matches)
		usage.UsageTypes = append(usage.UsageTypes, "checknode")
	}

	// Pattern 2: Plugin used in append - <append ... type="PLUGIN">pluginName(...)</append>
	appendPattern := `<append[^>]*type="PLUGIN"[^>]*>([^<]*` + pluginID + `[^<]*)</append>`
	if matches := regexp.MustCompile(appendPattern).FindAllString(rulesetContent, -1); len(matches) > 0 {
		usage.UsageCount += len(matches)
		if !containsString(usage.UsageTypes, "append") {
			usage.UsageTypes = append(usage.UsageTypes, "append")
		}
	}

	// Pattern 3: Plugin used in condition - <plugin>pluginName(...)</plugin>
	conditionPattern := `<plugin[^>]*>([^<]*` + pluginID + `[^<]*)</plugin>`
	if matches := regexp.MustCompile(conditionPattern).FindAllString(rulesetContent, -1); len(matches) > 0 {
		usage.UsageCount += len(matches)
		if !containsString(usage.UsageTypes, "condition") {
			usage.UsageTypes = append(usage.UsageTypes, "condition")
		}
	}

	// Extract rule IDs that use this plugin (simplified approach)
	if usage.UsageCount > 0 {
		// Look for rule IDs in the vicinity of plugin usage
		ruleIDPattern := `<rule[^>]*id="([^"]+)"[^>]*>`
		ruleMatches := regexp.MustCompile(ruleIDPattern).FindAllStringSubmatch(rulesetContent, -1)
		for _, match := range ruleMatches {
			if len(match) > 1 {
				ruleID := match[1]
				// Check if this rule contains the plugin (simplified check)
				ruleStart := strings.Index(rulesetContent, match[0])
				ruleEnd := strings.Index(rulesetContent[ruleStart:], "</rule>")
				if ruleEnd > 0 {
					ruleContent := rulesetContent[ruleStart : ruleStart+ruleEnd]
					if strings.Contains(ruleContent, pluginID) {
						usage.RuleIDs = append(usage.RuleIDs, ruleID)
					}
				}
			}
		}
	}

	return usage
}

// containsString checks if a string slice contains a specific string
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ComponentOperations has been moved to common package
// API handlers will use common.GlobalComponentOperations.CreateComponentDirect() etc.
