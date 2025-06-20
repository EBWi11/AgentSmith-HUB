package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/local_plugin"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

func testPlugin(c echo.Context) error {
	// Use :id parameter for consistency with other components
	id := c.Param("id")
	if id == "" {
		// Fallback to :name for backward compatibility
		id = c.Param("name")
	}

	// Parse request body
	var req struct {
		Data map[string]interface{} `json:"data"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
			"result":  nil,
		})
	}

	// Check if plugin exists in memory
	p, existsInMemory := plugin.Plugins[id]

	// Check if plugin exists in temporary files
	tempContent, existsInTemp := plugin.PluginsNew[id]

	var pluginToTest *plugin.Plugin
	var isTemporary bool

	if existsInMemory {
		// Use existing plugin
		pluginToTest = p
		isTemporary = false
	} else if existsInTemp {
		// Try to verify the temporary plugin
		err := plugin.Verify("", tempContent, id+"_test_temp")
		if err != nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("Plugin compilation failed: %v", err),
				"result":  nil,
			})
		}

		// Create a temporary plugin instance for testing
		tempPlugin := &plugin.Plugin{
			Name:    id,
			Payload: []byte(tempContent),
			Type:    plugin.YAEGI_PLUGIN,
		}

		pluginToTest = tempPlugin
		isTemporary = true
	} else {
		// Try to load plugin from file system directly
		configRoot := common.Config.ConfigRoot
		pluginPath := filepath.Join(configRoot, "plugin", id+".go")

		if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success": false,
				"error":   "Plugin not found: " + id,
				"result":  nil,
			})
		}

		// Verify the plugin file
		err := plugin.Verify(pluginPath, "", id+"_test_temp")
		if err != nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("Plugin compilation failed: %v", err),
				"result":  nil,
			})
		}

		// Read and create a temporary plugin instance
		content, err := os.ReadFile(pluginPath)
		if err != nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("Failed to read plugin file: %v", err),
				"result":  nil,
			})
		}

		tempPlugin := &plugin.Plugin{
			Name:    id,
			Path:    pluginPath,
			Payload: content,
			Type:    plugin.YAEGI_PLUGIN,
		}

		pluginToTest = tempPlugin
		isTemporary = true
	}

	// Convert input data to string parameter
	// Plugins typically accept JSON string as input
	jsonData, err := json.Marshal(req.Data)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Failed to serialize input data: " + err.Error(),
			"result":  nil,
		})
	}

	// Create parameter array, only passing one JSON string parameter
	args := []interface{}{string(jsonData)}

	// Determine plugin type and execute
	var result interface{}
	var success bool
	var errMsg string

	switch pluginToTest.Type {
	case plugin.LOCAL_PLUGIN:
		// Check if it's a boolean result plugin
		if f, ok := local_plugin.LocalPluginBoolRes[id]; ok {
			boolResult, err := f(args...)
			result = boolResult
			success = err == nil
			if err != nil {
				errMsg = fmt.Sprintf("Plugin execution failed: %v", err)
			}
		} else if f, ok := local_plugin.LocalPluginInterfaceAndBoolRes[id]; ok {
			// It's an interface result plugin
			interfaceResult, boolResult, err := f(args...)
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

		if isTemporary {
			// For temporary plugins, we need to load them first since they're not in the global registry
			err := pluginToTest.YaegiLoad()
			if err != nil {
				return c.JSON(http.StatusOK, map[string]interface{}{
					"success": false,
					"error":   fmt.Sprintf("Failed to load temporary plugin: %v", err),
					"result":  nil,
				})
			}
		}

		// Execute plugin
		boolResult := pluginToTest.FuncEvalCheckNode(args...)
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

func getAvailablePlugins(c echo.Context) error {
	plugins := make([]map[string]interface{}, 0)

	// Only return formal plugins, exclude temporary plugins
	for _, p := range plugin.Plugins {
		if p.Type == plugin.YAEGI_PLUGIN {
			// Extract plugin description (if any)
			description := extractPluginDescription(string(p.Payload))
			plugins = append(plugins, map[string]interface{}{
				"name":        p.Name,
				"description": description,
			})
		}
	}

	return c.JSON(http.StatusOK, plugins)
}

func extractPluginDescription(code string) string {
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

func testRuleset(c echo.Context) error {
	id := c.Param("id")

	// Parse request body
	var req struct {
		Data map[string]interface{} `json:"data"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
			"result":  nil,
		})
	}

	// Check if input data is provided
	if req.Data == nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input data is required",
			"result":  nil,
		})
	}

	// Check if ruleset exists in formal or temporary files
	var rulesetContent string
	var isTemp bool

	// Check if there's a temporary file first
	tempPath, tempExists := GetComponentPath("ruleset", id, true)
	if tempExists {
		content, err := ReadComponent(tempPath)
		if err == nil {
			rulesetContent = content
			isTemp = true
		}
	}

	// If no temp file, check formal file
	if rulesetContent == "" {
		formalPath, formalExists := GetComponentPath("ruleset", id, false)
		if !formalExists {
			// Check if ruleset exists in memory
			r := project.GlobalProject.Rulesets[id]
			if r == nil {
				// Check if ruleset exists in new rulesets
				content, ok := project.GlobalProject.RulesetsNew[id]
				if !ok {
					return c.JSON(http.StatusNotFound, map[string]interface{}{
						"success": false,
						"error":   "Ruleset not found: " + id,
						"result":  nil,
					})
				}
				rulesetContent = content
			} else {
				rulesetContent = r.RawConfig
			}
		} else {
			content, err := ReadComponent(formalPath)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]interface{}{
					"success": false,
					"error":   "Failed to read ruleset: " + err.Error(),
					"result":  nil,
				})
			}
			rulesetContent = content
		}
	}

	// Create a temporary ruleset for testing
	tempRuleset, err := rules_engine.NewRuleset("", rulesetContent, "temp_test_"+id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse ruleset: " + err.Error(),
			"result":  nil,
		})
	}

	// Create channels for testing
	inputCh := make(chan map[string]interface{}, 1)
	outputCh := make(chan map[string]interface{}, 10)

	// Set up the ruleset
	tempRuleset.UpStream = map[string]*chan map[string]interface{}{
		"test": &inputCh,
	}
	tempRuleset.DownStream = map[string]*chan map[string]interface{}{
		"test": &outputCh,
	}

	// Start the ruleset
	err = tempRuleset.Start()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to start ruleset: " + err.Error(),
			"result":  nil,
		})
	}

	// Send the test data
	inputCh <- req.Data

	// Collect results with timeout
	results := make([]map[string]interface{}, 0)
	timeout := time.After(2 * time.Second)
	collectDone := make(chan bool)

	go func() {
		for {
			select {
			case result, ok := <-outputCh:
				if !ok {
					collectDone <- true
					return
				}
				results = append(results, result)
			case <-time.After(500 * time.Millisecond):
				// If no more results after 500ms, assume we're done
				collectDone <- true
				return
			}
		}
	}()

	// Wait for collection to complete or timeout
	select {
	case <-collectDone:
		// Collection completed normally
	case <-timeout:
		// Timeout occurred
	}

	// Stop the ruleset
	err = tempRuleset.Stop()
	if err != nil {
		logger.Warn("Failed to stop temporary ruleset: %v", err)
	}

	// Return the results
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"isTemp":  isTemp,
		"results": results,
	})
}

func testOutput(c echo.Context) error {
	id := c.Param("id")

	// Parse request body
	var req struct {
		Data map[string]interface{} `json:"data"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
			"result":  nil,
		})
	}

	// Check if input data is provided
	if req.Data == nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input data is required",
			"result":  nil,
		})
	}

	// Check if output exists in formal or temporary files
	var outputContent string
	var isTemp bool

	// Check if there's a temporary file first
	tempPath, tempExists := GetComponentPath("output", id, true)
	if tempExists {
		content, err := ReadComponent(tempPath)
		if err == nil {
			outputContent = content
			isTemp = true
		}
	}

	// If no temp file, check formal file
	if outputContent == "" {
		formalPath, formalExists := GetComponentPath("output", id, false)
		if !formalExists {
			// Check if output exists in memory
			out := project.GlobalProject.Outputs[id]
			if out == nil {
				// Check if output exists in new outputs
				content, ok := project.GlobalProject.OutputsNew[id]
				if !ok {
					return c.JSON(http.StatusNotFound, map[string]interface{}{
						"success": false,
						"error":   "Output not found: " + id,
						"result":  nil,
					})
				}
				outputContent = content
			} else {
				outputContent = out.Config.RawConfig
			}
		} else {
			content, err := ReadComponent(formalPath)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]interface{}{
					"success": false,
					"error":   "Failed to read output: " + err.Error(),
					"result":  nil,
				})
			}
			outputContent = content
		}
	}

	// Create a temporary output for testing
	tempOutput, err := output.NewOutput("", outputContent, "temp_test_"+id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse output: " + err.Error(),
			"result":  nil,
		})
	}

	// Create channels for testing
	inputCh := make(chan map[string]interface{}, 1)
	tempOutput.UpStream = append(tempOutput.UpStream, &inputCh)

	// Start the output
	err = tempOutput.Start()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to start output: " + err.Error(),
			"result":  nil,
		})
	}

	// Send the test data
	inputCh <- req.Data

	// Wait a bit to ensure data is processed
	time.Sleep(500 * time.Millisecond)

	// Get metrics
	produceTotal := tempOutput.GetProduceTotal()
	produceQPS := tempOutput.GetProduceQPS()

	// Stop the output
	err = tempOutput.Stop()
	if err != nil {
		logger.Warn("Failed to stop temporary output: %v", err)
	}

	// Return the results
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"isTemp":  isTemp,
		"metrics": map[string]interface{}{
			"produceTotal": produceTotal,
			"produceQPS":   produceQPS,
		},
		"outputType": string(tempOutput.Type),
	})
}

func testProject(c echo.Context) error {
	id := c.Param("id")

	// Parse request body
	var req struct {
		InputNode string                 `json:"input_node"` // Format: "input.name"
		Data      map[string]interface{} `json:"data"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
			"result":  nil,
		})
	}

	// Check if input data and node are provided
	if req.Data == nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input data is required",
			"result":  nil,
		})
	}

	if req.InputNode == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input node is required",
			"result":  nil,
		})
	}

	// Parse input node
	nodeParts := strings.Split(req.InputNode, ".")
	if len(nodeParts) != 2 || strings.ToLower(nodeParts[0]) != "input" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid input node format. Expected 'input.name'",
			"result":  nil,
		})
	}
	inputNodeName := nodeParts[1]

	// Check if project exists
	var projectContent string
	var isTemp bool

	// First check if there is a temporary file
	tempPath, tempExists := GetComponentPath("project", id, true)
	if tempExists {
		content, err := ReadComponent(tempPath)
		if err == nil {
			projectContent = content
			isTemp = true
		}
	}

	// If no temporary file, check formal file
	if projectContent == "" {
		formalPath, formalExists := GetComponentPath("project", id, false)
		if !formalExists {
			// Check if project exists in memory
			proj := project.GlobalProject.Projects[id]
			if proj == nil {
				// Check if project exists in new projects
				content, ok := project.GlobalProject.ProjectsNew[id]
				if !ok {
					return c.JSON(http.StatusNotFound, map[string]interface{}{
						"success": false,
						"error":   "Project not found: " + id,
						"result":  nil,
					})
				}
				projectContent = content
			} else {
				projectContent = proj.Config.RawConfig
			}
		} else {
			content, err := ReadComponent(formalPath)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]interface{}{
					"success": false,
					"error":   "Failed to read project: " + err.Error(),
					"result":  nil,
				})
			}
			projectContent = content
		}
	}

	// Create temporary project to parse configuration (test version, no real component initialization)
	// Generate unique test project ID to avoid conflicts
	testProjectId := fmt.Sprintf("test_%s_%d", id, time.Now().UnixNano())
	tempProject, err := project.NewProjectForTesting("", projectContent, testProjectId)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse project: " + err.Error(),
			"result":  nil,
		})
	}

	// Ensure cleanup on exit
	defer func() {
		if stopErr := tempProject.StopForTesting(); stopErr != nil {
			logger.Warn("Failed to stop temporary project: %v", stopErr)
		}
	}()

	// Check if the specified input exists in the project
	if _, exists := tempProject.Inputs[inputNodeName]; !exists {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input node not found in project: " + inputNodeName,
			"result":  nil,
		})
	}

	// Create a map to collect output results
	outputResults := make(map[string][]map[string]interface{})
	outputChannels := make(map[string]chan map[string]interface{})

	// For each output component, create a test collection channel
	for outputName, outputComp := range tempProject.Outputs {
		// Create a channel for each output to collect test results
		testChan := make(chan map[string]interface{}, 100)
		outputChannels[outputName] = testChan

		// Set the test collection channel for output component
		// Don't replace UpStream - let the output use its original data flow
		// We'll modify the output to also send data to test collection channel
		outputComp.TestCollectionChan = &testChan

		logger.Info("Created test collection channel for output", "output", outputName, "project", testProjectId)
	}

	// Start the project
	err = tempProject.StartForTesting()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to start project: " + err.Error(),
			"result":  nil,
		})
	}

	// Find the input node and verify it has downstream connections
	inputNode := tempProject.Inputs[inputNodeName]
	if len(inputNode.DownStream) == 0 {
		logger.Warn("Input node has no downstream connections", "input", inputNodeName, "project", testProjectId)

		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input node has no downstream connections. Please check project configuration.",
			"result":  nil,
		})
	}

	// Send test data to all downstream channels of the input
	logger.Info("Sending test data to input channels", "input", inputNodeName, "downstream_count", len(inputNode.DownStream))
	for i, downChan := range inputNode.DownStream {
		logger.Info("Sending data to downstream channel", "input", inputNodeName, "channel", i)
		*downChan <- req.Data
	}

	// Wait for data to flow through the system and be collected
	time.Sleep(500 * time.Millisecond)

	// Collect results from output channels with timeout
	collectTimeout := time.After(1000 * time.Millisecond)
	for outputName, outputChan := range outputChannels {
		results := []map[string]interface{}{}

		// Collect messages with timeout
		logger.Info("Collecting results from output channel", "output", outputName)

	collectLoop:
		for {
			select {
			case msg := <-outputChan:
				// Directly use the message without adding test metadata
				results = append(results, msg)
				logger.Info("Collected message from output", "output", outputName, "message_count", len(results))

			case <-collectTimeout:
				logger.Info("Collection timeout reached", "output", outputName, "collected_count", len(results))
				break collectLoop

			default:
				// No more immediate messages, check if we should continue waiting
				if len(results) > 0 {
					// We got some results, wait a bit more for potential additional messages
					time.Sleep(50 * time.Millisecond)
				} else {
					// No results yet, continue waiting
					time.Sleep(10 * time.Millisecond)
				}
			}
		}

		outputResults[outputName] = results
		logger.Info("Final collection result", "output", outputName, "total_messages", len(results))
	}

	// Return the results
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":   true,
		"isTemp":    isTemp,
		"outputs":   outputResults,
		"inputNode": req.InputNode,
	})
}

func getProjectInputs(c echo.Context) error {
	id := c.Param("id")

	// Check if project exists
	var projectContent string
	var isTemp bool

	// First check if there is a temporary file
	tempPath, tempExists := GetComponentPath("project", id, true)
	if tempExists {
		content, err := ReadComponent(tempPath)
		if err == nil {
			projectContent = content
			isTemp = true
		}
	}

	// If no temporary file, check formal file
	if projectContent == "" {
		formalPath, formalExists := GetComponentPath("project", id, false)
		if !formalExists {
			// Check if project exists in memory
			proj := project.GlobalProject.Projects[id]
			if proj == nil {
				// Check if project exists in new projects
				content, ok := project.GlobalProject.ProjectsNew[id]
				if !ok {
					return c.JSON(http.StatusNotFound, map[string]interface{}{
						"success": false,
						"error":   "Project not found: " + id,
					})
				}
				projectContent = content
			} else {
				projectContent = proj.Config.RawConfig
			}
		} else {
			content, err := ReadComponent(formalPath)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]interface{}{
					"success": false,
					"error":   "Failed to read project: " + err.Error(),
				})
			}
			projectContent = content
		}
	}

	// Create temporary project to parse configuration (test version, no real component initialization)
	// Generate unique test project ID to avoid conflicts
	testProjectId := fmt.Sprintf("test_inputs_%s_%d", id, time.Now().UnixNano())
	tempProject, err := project.NewProjectForTesting("", projectContent, testProjectId)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse project: " + err.Error(),
		})
	}

	// Collect input node information (these are virtual input nodes, only for flow chart validation)
	inputs := []map[string]string{}
	for name := range tempProject.Inputs {
		inputs = append(inputs, map[string]string{
			"id":   "input." + name,
			"name": name,
			"type": "virtual", // Virtual input node for testing
		})
	}

	// Sort input node list by name
	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i]["name"] < inputs[j]["name"]
	})

	// Return result
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"isTemp":  isTemp,
		"inputs":  inputs,
	})
}

// createTestProject creates a completely independent project instance for testing
// All components (except inputs) are created as new instances to avoid affecting the live environment
func createTestProject(projectContent string, testProjectId string) (*project.Project, error) {
	// Create the project instance using a special constructor for testing
	tempProject, err := project.NewProjectForTesting("", projectContent, testProjectId)
	if err != nil {
		return nil, fmt.Errorf("failed to parse project: %v", err)
	}

	return tempProject, nil
}

func connectCheck(c echo.Context) error {
	componentType := c.Param("type")
	id := c.Param("id")

	// Normalize component type (accept both singular and plural forms)
	normalizedType := componentType
	if componentType == "input" {
		normalizedType = "inputs"
	} else if componentType == "output" {
		normalizedType = "outputs"
	}

	// Check if component type is valid
	if normalizedType != "inputs" && normalizedType != "outputs" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid component type. Must be 'input', 'inputs', 'output', or 'outputs'",
		})
	}

	// Check input component client connection
	if normalizedType == "inputs" {
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

		// Use the enhanced connectivity check method from the input component
		connectivityResult := inputComp.CheckConnectivity()
		return c.JSON(http.StatusOK, connectivityResult)
	} else if normalizedType == "outputs" {
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

		// Use the enhanced connectivity check method from the output component
		connectivityResult := outputComp.CheckConnectivity()
		return c.JSON(http.StatusOK, connectivityResult)
	}

	return c.JSON(http.StatusInternalServerError, map[string]string{
		"error": "Unknown error occurred",
	})
}

func testRulesetContent(c echo.Context) error {
	var req struct {
		Content string                 `json:"content"`
		Data    map[string]interface{} `json:"data"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
			"results": []interface{}{},
		})
	}

	if req.Content == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Ruleset content is required",
			"results": []interface{}{},
		})
	}

	if req.Data == nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input data is required",
			"results": []interface{}{},
		})
	}

	// Create a temporary ruleset for testing
	tempRuleset, err := rules_engine.NewRuleset("", req.Content, "temp_test_content")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse ruleset: " + err.Error(),
			"results": []interface{}{},
		})
	}

	// Create channels for testing
	inputCh := make(chan map[string]interface{}, 1)
	outputCh := make(chan map[string]interface{}, 10)

	// Set up the ruleset
	tempRuleset.UpStream = map[string]*chan map[string]interface{}{
		"test": &inputCh,
	}
	tempRuleset.DownStream = map[string]*chan map[string]interface{}{
		"test": &outputCh,
	}

	// Start the ruleset
	err = tempRuleset.Start()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to start ruleset: " + err.Error(),
			"results": []interface{}{},
		})
	}

	// Send the test data
	inputCh <- req.Data

	// Collect results with timeout
	results := make([]map[string]interface{}, 0)
	timeout := time.After(2 * time.Second)
	collectDone := make(chan bool)

	go func() {
		for {
			select {
			case result, ok := <-outputCh:
				if !ok {
					collectDone <- true
					return
				}
				results = append(results, result)
			case <-time.After(500 * time.Millisecond):
				// If no more results after 500ms, assume we're done
				collectDone <- true
				return
			}
		}
	}()

	// Wait for collection to complete or timeout
	select {
	case <-collectDone:
		// Collection completed normally
	case <-timeout:
		// Timeout occurred
	}

	// Stop the ruleset
	err = tempRuleset.Stop()
	if err != nil {
		logger.Warn("Failed to stop temporary ruleset: %v", err)
	}

	// Return the results
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"results": results,
	})
}
