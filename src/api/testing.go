package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/local_plugin"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// testRulesetInternal is the unified internal function for testing rulesets
func testRulesetInternal(rulesetContent string, testData map[string]interface{}, timeoutSeconds int, isTemp bool) (map[string]interface{}, error) {
	// Create a temporary ruleset for testing
	tempRuleset, err := rules_engine.NewRuleset("", rulesetContent, "temp_test_"+fmt.Sprintf("%d", time.Now().UnixNano()))
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Failed to parse ruleset: " + err.Error(),
			"results": []interface{}{},
		}, nil
	}

	// Create channels for testing
	inputCh := make(chan map[string]interface{}, 100)
	outputCh := make(chan map[string]interface{}, 100)

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
		return map[string]interface{}{
			"success": false,
			"error":   "Failed to start ruleset: " + err.Error(),
			"results": []interface{}{},
		}, nil
	}

	// Send the test data
	inputCh <- testData

	// Collect results with timeout
	results := make([]map[string]interface{}, 0)
	timeout := time.After(time.Duration(timeoutSeconds) * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var timedOut bool
	lastResultTime := time.Now()

	for {
		select {
		case result, ok := <-outputCh:
			if !ok {
				// Channel closed, we're done
				timedOut = false
				goto done
			}
			results = append(results, result)
			lastResultTime = time.Now()
		case <-ticker.C:
			// Different timeout conditions based on timeout duration
			if timeoutSeconds <= 5 {
				// For short timeouts (like testRulesetContent), check for recent results
				if len(inputCh) == 0 && time.Since(lastResultTime) > 500*time.Millisecond {
					timedOut = false
					goto done
				}
			} else {
				// For longer timeouts (like testRuleset), check task count
				if len(inputCh) == 0 && tempRuleset.GetRunningTaskCount() == 0 {
					timedOut = false
					goto done
				}
			}
		case <-timeout:
			// Timeout occurred
			timedOut = true
			logger.Warn("Ruleset test timed out after %d seconds", timeoutSeconds)
			goto done
		}
	}
done:

	// Stop the ruleset
	err = tempRuleset.Stop()
	if err != nil {
		logger.Warn("Failed to stop temporary ruleset: %v", err)
	}

	// Explicitly set to nil to help GC
	tempRuleset = nil

	// Build response
	response := map[string]interface{}{
		"success": true,
		"results": results,
		"timeout": timedOut,
	}

	// Add isTemp field if specified
	if isTemp {
		response["isTemp"] = true
	}

	// Add timeout warning if needed
	if timedOut {
		response["warning"] = fmt.Sprintf("Test timed out after %d seconds. Results may be incomplete.", timeoutSeconds)
	}

	return response, nil
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
				isTemp = true
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

	// Use the unified internal function with 30-second timeout
	response, err := testRulesetInternal(rulesetContent, req.Data, 30, isTemp)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, response)
	}

	return c.JSON(http.StatusOK, response)
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

	// Use the unified internal function with 2-second timeout
	response, err := testRulesetInternal(req.Content, req.Data, 2, false)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, response)
	}

	return c.JSON(http.StatusOK, response)
}

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

	if !existsInMemory && !existsInTemp {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Plugin not found: " + id,
			"result":  nil,
		})
	}

	var pluginToTest *plugin.Plugin
	var isTemporary bool

	if existsInMemory {
		// Use existing plugin
		pluginToTest = p
		isTemporary = false
	} else {
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
	}

	// Process parameter values, try to convert to appropriate types
	args := make([]interface{}, 0)
	for _, value := range req.Data {
		// Convert to appropriate types
		if str, ok := value.(string); ok {
			args = append(args, str)
		} else {
			// Convert other types to string
			args = append(args, fmt.Sprintf("%v", value))
		}
	}

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

		// Execute plugin (panic is already handled inside plugin functions)
		boolResult, err := pluginToTest.FuncEvalCheckNode(args...)
		result = boolResult
		if err != nil {
			success = false
			errMsg = fmt.Sprintf("Plugin execution failed: %v", err)
		} else {
			success = true
		}
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
	inputCh := make(chan map[string]interface{}, 100)
	tempOutput.UpStream["_testing"] = &inputCh

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

	// Wait for processing with timeout
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var timedOut bool

	for {
		select {
		case <-ticker.C:
			currentChannelLen := len(inputCh)
			// Check if upstream data is 0 or channel length hasn't changed for 5 consecutive checks (500ms)
			if currentChannelLen == 0 {
				timedOut = false
				time.Sleep(500 * time.Millisecond)
				goto done
			}
		case <-timeout:
			// Timeout occurred
			timedOut = true
			goto done
		}
	}
done:

	// Get metrics
	produceTotal := tempOutput.GetProduceTotal()

	// Stop the output
	err = tempOutput.Stop()
	if err != nil {
		logger.Warn("Failed to stop temporary output: %v", err)
	}

	// Return the results with timeout information
	response := map[string]interface{}{
		"success": true,
		"isTemp":  isTemp,
		"metrics": map[string]interface{}{
			"produceTotal": produceTotal,
		},
		"outputType": string(tempOutput.Type),
		"timeout":    timedOut,
	}

	if timedOut {
		response["warning"] = "Test timed out after 30 seconds. Results may be incomplete."
	}

	return c.JSON(http.StatusOK, response)
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

	// Create temporary project for testing (based on PNS logic)
	testProjectId := fmt.Sprintf("test_%s_%d", id, time.Now().UnixNano())
	tempProject, err := project.NewProject("", projectContent, testProjectId, true) // testing=true
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse project: " + err.Error(),
			"result":  nil,
		})
	}

	// Ensure cleanup on exit
	defer func() {
		// Stop the project (this will handle rulesets and other components)
		if stopErr := tempProject.Stop(); stopErr != nil {
			logger.Warn("Failed to stop temporary project: %v", stopErr)
		}

		// Clean up from global project
		common.GlobalMu.Lock()
		delete(project.GlobalProject.Projects, testProjectId)
		common.GlobalMu.Unlock()

		logger.Info("Test project cleanup completed", "project", testProjectId)
	}()

	// Find the input node in flow nodes and check if it exists
	var inputPNS string
	var inputExists bool
	for _, node := range tempProject.FlowNodes {
		if node.FromType == "INPUT" && node.FromID == inputNodeName {
			inputPNS = node.FromPNS
			inputExists = true
			break
		}
	}

	if !inputExists {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input node not found in project: " + inputNodeName,
			"result":  nil,
		})
	}

	// Start the project to initialize PNS components
	err = tempProject.Start()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to start project: " + err.Error(),
			"result":  nil,
		})
	}

	// Create collection channels for outputs in testing mode
	outputChannels := make(map[string]chan map[string]interface{})
	for pns, outputComp := range tempProject.Outputs {
		testChan := make(chan map[string]interface{}, 100)
		outputChannels[pns] = testChan

		// Set TestCollectionChan to collect output data without sending to external systems
		outputComp.TestCollectionChan = &testChan
		logger.Info("Set test collection channel for output", "output", outputComp.Id, "pns", pns, "project", testProjectId)
	}

	// Find the test input component and inject test data through it
	var testInput *input.Input
	for pns, inputComp := range tempProject.Inputs {
		if pns == inputPNS {
			testInput = inputComp
			break
		}
	}

	if testInput == nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Test input component not found. Please check project configuration.",
			"result":  nil,
		})
	}

	if len(testInput.DownStream) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input node has no downstream connections. Please check project configuration.",
			"result":  nil,
		})
	}

	// Use input component's ProcessTestData method to inject test data
	// This ensures proper data flow through the input component's normal processing
	logger.Info("Injecting test data through input component", "input", inputNodeName, "pns", inputPNS, "downstream_count", len(testInput.DownStream))
	testInput.ProcessTestData(req.Data)

	// Wait for processing with timeout
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var timedOut bool
	outputResults := make(map[string][]map[string]interface{})

	// Initialize output results
	for pns := range outputChannels {
		outputResults[pns] = []map[string]interface{}{}
	}

	for {
		select {
		case <-ticker.C:
			// Collect any available results
			for pns, outputChan := range outputChannels {
				for {
					select {
					case msg := <-outputChan:
						outputResults[pns] = append(outputResults[pns], msg)
						logger.Info("Collected message from output", "pns", pns, "message_count", len(outputResults[pns]))
					default:
						goto nextOutput
					}
				}
			nextOutput:
			}

			// Check if all channels are empty and no running tasks
			allChannelsEmpty := true
			allTasksComplete := true

			for pns := range tempProject.MsgChannels {
				if len(*tempProject.MsgChannels[pns]) > 0 {
					allChannelsEmpty = false
					break
				}
			}

			for _, rs := range tempProject.Rulesets {
				if rs.GetRunningTaskCount() > 0 {
					allTasksComplete = false
					break
				}
			}

			if allChannelsEmpty && allTasksComplete {
				timedOut = false
				goto done
			}

		case <-timeout:
			timedOut = true
			goto done
		}
	}
done:

	// Return the results with timeout information
	response := map[string]interface{}{
		"success":   true,
		"isTemp":    isTemp,
		"outputs":   outputResults,
		"inputNode": req.InputNode,
		"timeout":   timedOut,
	}

	if timedOut {
		response["warning"] = "Test timed out after 30 seconds. Results may be incomplete."
	}

	return c.JSON(http.StatusOK, response)
}

func getProjectInputs(c echo.Context) error {
	id := c.Param("id")

	// Check if project exists
	var projectContent string
	var proj *project.Project
	var isTemp bool
	var ok bool

	if proj, ok = project.GlobalProject.Projects[id]; ok {
		projectContent = proj.Config.Content
	} else {
		if projectContent, ok = project.GlobalProject.ProjectsNew[id]; !ok {
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"success": false,
				"error":   "Project not found: " + id,
			})
		}
	}

	// Create temporary project to parse configuration (test version, no real component initialization)
	// Generate unique test project ID to avoid conflicts
	testProjectId := fmt.Sprintf("test_inputs_%s_%d", id, time.Now().UnixNano())
	tempProject, err := project.NewProject("", projectContent, testProjectId, true) // testing=true
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

func connectCheck(c echo.Context) error {
	componentType := c.Param("type")
	id := c.Param("id")

	// Normalize component type (accept both singular and plural forms)
	normalizedType := componentType
	if componentType == "input" {
		normalizedType = "inputs"
	} else if componentType == "output" {
		normalizedType = "outputs"
	} else {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid component type. Must be 'input', 'inputs', 'output', or 'outputs'",
		})
	}

	// Check if this is a POST request with custom configuration
	if c.Request().Method == "POST" {
		return connectCheckWithConfig(c, normalizedType, id)
	}

	// Check input component client connection
	if normalizedType == "inputs" {
		tempContent, existsNew := project.GlobalProject.InputsNew[id]
		inputComp := project.GlobalProject.Inputs[id]

		if !existsNew && inputComp == nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Input component not found",
			})
		}

		var connectivityResult map[string]interface{}
		var isTemp bool

		// If the input has pending changes, use the temporary configuration for testing
		if existsNew {
			// Create a temporary input instance for testing
			tempInput, err := input.NewInput("", tempContent, "temp_test_"+id)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]interface{}{
					"status":  "error",
					"message": "Failed to create temporary input for testing: " + err.Error(),
					"details": map[string]interface{}{
						"client_type":       "",
						"connection_status": "configuration_error",
						"connection_info":   map[string]interface{}{},
						"connection_errors": []map[string]interface{}{
							{"message": "Failed to parse pending configuration: " + err.Error(), "severity": "error"},
						},
					},
				})
			}

			// Test connectivity using the temporary input
			connectivityResult = tempInput.CheckConnectivity()
			isTemp = true

			// Clean up the temporary input (stop it if it was started)
			if stopErr := tempInput.Stop(); stopErr != nil {
				logger.Warn("Failed to stop temporary input after connectivity test", "id", id, "error", stopErr)
			}
		} else {
			// Use the existing input component
			connectivityResult = inputComp.CheckConnectivity()
			isTemp = false
		}

		// Add metadata to indicate if this was tested with pending changes
		if connectivityResult == nil {
			connectivityResult = map[string]interface{}{
				"status":  "error",
				"message": "Connection check failed",
				"details": map[string]interface{}{
					"client_type":       "",
					"connection_status": "unknown",
					"connection_info":   map[string]interface{}{},
					"connection_errors": []map[string]interface{}{
						{"message": "Unknown error during connectivity check", "severity": "error"},
					},
				},
			}
		}

		// Add indicator for temporary configuration testing
		connectivityResult["isTemp"] = isTemp
		if isTemp {
			// Enhance the message to indicate this was tested with pending changes
			if originalMessage, ok := connectivityResult["message"].(string); ok {
				connectivityResult["message"] = originalMessage + " (tested with pending changes)"
			}
		}

		return c.JSON(http.StatusOK, connectivityResult)
	} else if normalizedType == "outputs" {
		tempContent, existsNew := project.GlobalProject.OutputsNew[id]
		outputComp := project.GlobalProject.Outputs[id]

		if !existsNew && outputComp == nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Output component not found",
			})
		}

		var connectivityResult map[string]interface{}
		var isTemp bool

		// If the output has pending changes, use the temporary configuration for testing
		if existsNew {
			// Create a temporary output instance for testing
			tempOutput, err := output.NewOutput("", tempContent, "temp_test_"+id)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]interface{}{
					"status":  "error",
					"message": "Failed to create temporary output for testing: " + err.Error(),
					"details": map[string]interface{}{
						"client_type":       "",
						"connection_status": "configuration_error",
						"connection_info":   map[string]interface{}{},
						"connection_errors": []map[string]interface{}{
							{"message": "Failed to parse pending configuration: " + err.Error(), "severity": "error"},
						},
					},
				})
			}

			// Test connectivity using the temporary output
			connectivityResult = tempOutput.CheckConnectivity()
			isTemp = true

			// Clean up the temporary output (stop it if it was started)
			if stopErr := tempOutput.Stop(); stopErr != nil {
				logger.Warn("Failed to stop temporary output after connectivity test", "id", id, "error", stopErr)
			}
		} else {
			// Use the existing output component
			connectivityResult = outputComp.CheckConnectivity()
			isTemp = false
		}

		// Add metadata to indicate if this was tested with pending changes
		if connectivityResult == nil {
			connectivityResult = map[string]interface{}{
				"status":  "error",
				"message": "Connection check failed",
				"details": map[string]interface{}{
					"client_type":       "",
					"connection_status": "unknown",
					"connection_info":   map[string]interface{}{},
					"connection_errors": []map[string]interface{}{
						{"message": "Unknown error during connectivity check", "severity": "error"},
					},
				},
			}
		}

		// Add indicator for temporary configuration testing
		connectivityResult["isTemp"] = isTemp
		if isTemp {
			// Enhance the message to indicate this was tested with pending changes
			if originalMessage, ok := connectivityResult["message"].(string); ok {
				connectivityResult["message"] = originalMessage + " (tested with pending changes)"
			}
		}

		return c.JSON(http.StatusOK, connectivityResult)
	}

	return c.JSON(http.StatusInternalServerError, map[string]string{
		"error": "Unknown error occurred",
	})
}

func testPluginContent(c echo.Context) error {
	var req struct {
		Content string                 `json:"content"`
		Data    map[string]interface{} `json:"data"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
			"result":  nil,
		})
	}

	if req.Content == "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Plugin content is required",
			"result":  nil,
		})
	}

	// Check if args data is provided
	if req.Data == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Args data is required",
			"result":  nil,
		})
	}

	// Create a temporary plugin for testing
	tempPluginId := "temp_test_content"
	err := plugin.NewPlugin("", req.Content, tempPluginId, plugin.YAEGI_PLUGIN)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Failed to create plugin: " + err.Error(),
			"result":  nil,
		})
	}

	// Get the created plugin from the global registry
	tempPlugin, exists := plugin.Plugins[tempPluginId]
	if !exists {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Failed to retrieve created plugin",
			"result":  nil,
		})
	}

	// Clean up the temporary plugin on exit
	defer delete(plugin.Plugins, tempPluginId)

	// Load the plugin
	err = tempPlugin.YaegiLoad()
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Failed to load plugin: " + err.Error(),
			"result":  nil,
		})
	}

	// Process parameter values, try to convert to appropriate types
	args := make([]interface{}, 0)
	for _, value := range req.Data {
		// Convert to appropriate types
		if str, ok := value.(string); ok {
			args = append(args, str)
		} else {
			// Convert other types to string
			args = append(args, fmt.Sprintf("%v", value))
		}
	}

	// Execute plugin (panic is already handled inside plugin functions)
	var result interface{}
	var success bool
	var errMsg string

	// Execute plugin
	boolResult, err := tempPlugin.FuncEvalCheckNode(args...)
	result = boolResult
	if err != nil {
		success = false
		errMsg = fmt.Sprintf("Plugin execution failed: %v", err)
	} else {
		success = true
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

func testProjectContent(c echo.Context) error {
	inputNode := c.Param("inputNode")

	var req struct {
		Content string                 `json:"content"`
		Data    map[string]interface{} `json:"data"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
			"outputs": map[string][]map[string]interface{}{},
		})
	}

	if req.Content == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Project content is required",
			"outputs": map[string][]map[string]interface{}{},
		})
	}

	if req.Data == nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input data is required",
			"outputs": map[string][]map[string]interface{}{},
		})
	}

	if inputNode == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input node is required",
			"outputs": map[string][]map[string]interface{}{},
		})
	}

	// Create a temporary project for testing
	tempProjectId := fmt.Sprintf("temp_test_content_%d", time.Now().UnixNano())
	tempProject, err := project.NewProject("", req.Content, tempProjectId, true) // testing=true
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to create project: " + err.Error(),
			"outputs": map[string][]map[string]interface{}{},
		})
	}

	// Ensure cleanup on exit
	defer func() {
		if stopErr := tempProject.Stop(); stopErr != nil {
			logger.Warn("Failed to stop temporary project: %v", stopErr)
		}
		// Clean up from global project
		common.GlobalMu.Lock()
		delete(project.GlobalProject.Projects, tempProjectId)
		common.GlobalMu.Unlock()
	}()

	// Check if the specified input exists in the project
	if _, exists := tempProject.Inputs[inputNode]; !exists {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Input node '%s' not found in project", inputNode),
			"outputs": map[string][]map[string]interface{}{},
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
		outputComp.TestCollectionChan = &testChan

		logger.Info("Created test collection channel for output", "output", outputName, "project", tempProjectId)
	}

	// Start the project (will automatically use test mode since TestCollectionChan is set)
	err = tempProject.Start()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to start project: " + err.Error(),
			"outputs": map[string][]map[string]interface{}{},
		})
	}

	// Find the input node and verify it has downstream connections
	inputNodeInstance := tempProject.Inputs[inputNode]
	if len(inputNodeInstance.DownStream) == 0 {
		logger.Warn("Input node has no downstream connections", "input", inputNode, "project", tempProjectId)

		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input node has no downstream connections. Please check project configuration.",
			"outputs": map[string][]map[string]interface{}{},
		})
	}

	// Send test data to all downstream channels of the input
	logger.Info("Sending test data through input processing", "input", inputNode, "downstream_count", len(inputNodeInstance.DownStream))

	// Process test data through the input component's normal data flow
	// This ensures proper counting, sampling, and field addition
	inputNodeInstance.ProcessTestData(req.Data)

	// Wait for data to flow through the system and be collected
	time.Sleep(500 * time.Millisecond)

	// Collect results from output channels with timeout
	collectTimeout := time.After(1000 * time.Millisecond)
	for outputName, testChan := range outputChannels {
		outputResults[outputName] = []map[string]interface{}{}

		// Collect messages from this output channel
		for {
			select {
			case result, ok := <-testChan:
				if !ok {
					// Channel is closed
					goto nextOutput
				}
				outputResults[outputName] = append(outputResults[outputName], result)
			case <-collectTimeout:
				// Timeout reached
				goto nextOutput
			case <-time.After(100 * time.Millisecond):
				// No more messages after 100ms, assume we're done with this output
				goto nextOutput
			}
		}
	nextOutput:
	}

	// Return the results
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"outputs": outputResults,
		"isTemp":  true,
	})
}

func getProjectComponents(c echo.Context) error {
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
	testProjectId := fmt.Sprintf("test_components_%s_%d", id, time.Now().UnixNano())
	tempProject, err := project.NewProject("", projectContent, testProjectId, true) // testing=true
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse project: " + err.Error(),
		})
	}

	// Collect component information
	inputs := []map[string]string{}
	for name := range tempProject.Inputs {
		inputs = append(inputs, map[string]string{
			"id":   name,
			"name": name,
			"type": "input",
		})
	}

	outputs := []map[string]string{}
	for name := range tempProject.Outputs {
		outputs = append(outputs, map[string]string{
			"id":   name,
			"name": name,
			"type": "output",
		})
	}

	rulesets := []map[string]string{}
	for name := range tempProject.Rulesets {
		rulesets = append(rulesets, map[string]string{
			"id":   name,
			"name": name,
			"type": "ruleset",
		})
	}

	// Calculate total component count
	totalComponents := len(inputs) + len(outputs) + len(rulesets)

	// Sort component lists by name
	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i]["name"] < inputs[j]["name"]
	})
	sort.Slice(outputs, func(i, j int) bool {
		return outputs[i]["name"] < outputs[j]["name"]
	})
	sort.Slice(rulesets, func(i, j int) bool {
		return rulesets[i]["name"] < rulesets[j]["name"]
	})

	// Return result
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":         true,
		"isTemp":          isTemp,
		"inputs":          inputs,
		"outputs":         outputs,
		"rulesets":        rulesets,
		"totalComponents": totalComponents,
		"componentCounts": map[string]int{
			"inputs":   len(inputs),
			"outputs":  len(outputs),
			"rulesets": len(rulesets),
		},
	})
}

// connectCheckWithConfig performs connectivity check using custom configuration
func connectCheckWithConfig(c echo.Context, normalizedType, id string) error {
	// Parse request body to get configuration
	var req struct {
		Raw string `json:"raw"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body: " + err.Error(),
		})
	}

	if req.Raw == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Configuration content is required",
		})
	}

	// Generate unique test ID to avoid conflicts
	testId := fmt.Sprintf("temp_connect_test_%s_%d", id, time.Now().UnixNano())

	if normalizedType == "inputs" {
		// Create a temporary input instance for testing
		tempInput, err := input.NewInput("", req.Raw, testId)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"status":  "error",
				"message": "Failed to create temporary input for testing: " + err.Error(),
				"details": map[string]interface{}{
					"client_type":       "",
					"connection_status": "configuration_error",
					"connection_info":   map[string]interface{}{},
					"connection_errors": []map[string]interface{}{
						{"message": "Failed to parse configuration: " + err.Error(), "severity": "error"},
					},
				},
			})
		}

		// Test connectivity using the temporary input
		connectivityResult := tempInput.CheckConnectivity()

		// Clean up the temporary input (stop it if it was started)
		if stopErr := tempInput.Stop(); stopErr != nil {
			logger.Warn("Failed to stop temporary input after connectivity test", "id", testId, "error", stopErr)
		}

		// Add metadata to indicate this was tested with custom configuration
		if connectivityResult == nil {
			connectivityResult = map[string]interface{}{
				"status":  "error",
				"message": "Connection check failed",
				"details": map[string]interface{}{
					"client_type":       "",
					"connection_status": "unknown",
					"connection_info":   map[string]interface{}{},
					"connection_errors": []map[string]interface{}{
						{"message": "Unknown error during connectivity check", "severity": "error"},
					},
				},
			}
		}

		// Add indicator for custom configuration testing
		connectivityResult["isTemp"] = true
		if originalMessage, ok := connectivityResult["message"].(string); ok {
			connectivityResult["message"] = originalMessage + " (tested with custom configuration)"
		}

		return c.JSON(http.StatusOK, connectivityResult)

	} else if normalizedType == "outputs" {
		// Create a temporary output instance for testing
		tempOutput, err := output.NewOutput("", req.Raw, testId)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"status":  "error",
				"message": "Failed to create temporary output for testing: " + err.Error(),
				"details": map[string]interface{}{
					"client_type":       "",
					"connection_status": "configuration_error",
					"connection_info":   map[string]interface{}{},
					"connection_errors": []map[string]interface{}{
						{"message": "Failed to parse configuration: " + err.Error(), "severity": "error"},
					},
				},
			})
		}

		// Test connectivity using the temporary output
		connectivityResult := tempOutput.CheckConnectivity()

		// Clean up the temporary output (stop it if it was started)
		if stopErr := tempOutput.Stop(); stopErr != nil {
			logger.Warn("Failed to stop temporary output after connectivity test", "id", testId, "error", stopErr)
		}

		// Add metadata to indicate this was tested with custom configuration
		if connectivityResult == nil {
			connectivityResult = map[string]interface{}{
				"status":  "error",
				"message": "Connection check failed",
				"details": map[string]interface{}{
					"client_type":       "",
					"connection_status": "unknown",
					"connection_info":   map[string]interface{}{},
					"connection_errors": []map[string]interface{}{
						{"message": "Unknown error during connectivity check", "severity": "error"},
					},
				},
			}
		}

		// Add indicator for custom configuration testing
		connectivityResult["isTemp"] = true
		if originalMessage, ok := connectivityResult["message"].(string); ok {
			connectivityResult["message"] = originalMessage + " (tested with custom configuration)"
		}

		return c.JSON(http.StatusOK, connectivityResult)
	}

	return c.JSON(http.StatusInternalServerError, map[string]string{
		"error": "Unknown error occurred",
	})
}

// getProjectComponentSequences returns project node sequences for each component in the project
func getProjectComponentSequences(c echo.Context) error {
	id := c.Param("id")

	// Check if project exists and get project content
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

	// Create temporary project to parse configuration
	testProjectId := fmt.Sprintf("test_sequences_%s_%d", id, time.Now().UnixNano())
	tempProject, err := project.NewProject("", projectContent, testProjectId, true) // testing=true
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse project: " + err.Error(),
		})
	}

	// Extract the data flow graph from parsed FlowNodes
	flowGraph := make(map[string]interface{})

	// Build flow nodes information
	nodes := make([]map[string]interface{}, 0)
	edges := make([]map[string]interface{}, 0)

	for _, node := range tempProject.FlowNodes {
		// Add nodes
		fromNode := map[string]interface{}{
			"id":   node.FromPNS,
			"type": node.FromType,
			"name": node.FromID,
		}
		toNode := map[string]interface{}{
			"id":   node.ToPNS,
			"type": node.ToType,
			"name": node.ToID,
		}

		// Add edge
		edge := map[string]interface{}{
			"from":    node.FromPNS,
			"to":      node.ToPNS,
			"content": node.Content,
		}

		nodes = append(nodes, fromNode, toNode)
		edges = append(edges, edge)
	}

	flowGraph["nodes"] = nodes
	flowGraph["edges"] = edges

	// Build component sequences using the same logic as loadComponentsFromGlobal
	componentSequences := make(map[string]string)
	hasUpstream := make(map[string]bool)
	for _, tos := range flowGraph {
		for _, to := range tos {
			hasUpstream[to] = true
		}
	}

	// Build ProjectNodeSequence recursively
	var buildSequence func(component string, visited map[string]bool) string
	buildSequence = func(component string, visited map[string]bool) string {
		if visited[component] {
			return component // Break cycle
		}
		if seq, exists := componentSequences[component]; exists {
			return seq
		}
		visited[component] = true
		defer delete(visited, component)

		var upstreamComponent string
		for from, tos := range flowGraph {
			for _, to := range tos {
				if to == component {
					upstreamComponent = from
					break
				}
			}
			if upstreamComponent != "" {
				break
			}
		}

		var sequence string
		if upstreamComponent == "" {
			sequence = component
		} else {
			upstreamSequence := buildSequence(upstreamComponent, visited)
			sequence = upstreamSequence + "." + component
		}
		componentSequences[component] = sequence
		return sequence
	}

	// Build sequences for all components
	for from := range flowGraph {
		buildSequence(from, make(map[string]bool))
	}
	for _, tos := range flowGraph {
		for _, to := range tos {
			buildSequence(to, make(map[string]bool))
		}
	}

	// Group components by type and collect their sequences
	result := map[string]map[string][]string{
		"input":   make(map[string][]string),
		"output":  make(map[string][]string),
		"ruleset": make(map[string][]string),
	}

	// Process each component and its sequence
	for component, sequence := range componentSequences {
		parts := strings.Split(component, ".")
		if len(parts) != 2 {
			continue
		}

		componentType := strings.ToLower(parts[0])
		componentId := parts[1]

		switch componentType {
		case "input":
			if result["input"][componentId] == nil {
				result["input"][componentId] = []string{}
			}
			result["input"][componentId] = append(result["input"][componentId], sequence)
		case "output":
			if result["output"][componentId] == nil {
				result["output"][componentId] = []string{}
			}
			result["output"][componentId] = append(result["output"][componentId], sequence)
		case "ruleset":
			if result["ruleset"][componentId] == nil {
				result["ruleset"][componentId] = []string{}
			}
			result["ruleset"][componentId] = append(result["ruleset"][componentId], sequence)
		}
	}

	// Remove duplicates and sort sequences for each component
	for componentType := range result {
		for componentId := range result[componentType] {
			sequences := result[componentType][componentId]
			// Remove duplicates
			uniqueSequences := make(map[string]bool)
			for _, seq := range sequences {
				uniqueSequences[seq] = true
			}
			// Convert back to slice and sort
			result[componentType][componentId] = []string{}
			for seq := range uniqueSequences {
				result[componentType][componentId] = append(result[componentType][componentId], seq)
			}
			sort.Strings(result[componentType][componentId])
		}
	}

	// Return the result
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"isTemp":  isTemp,
		"data":    result,
	})
}
