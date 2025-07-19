package api

import (
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
	"gopkg.in/yaml.v3"
)

// Helper function to return appropriate error format for ruleset APIs
func rulesetErrorResponse(isContentMode bool, success bool, errorMsg string) map[string]interface{} {
	if isContentMode {
		// /test-ruleset-content format
		return map[string]interface{}{
			"success": success,
			"error":   errorMsg,
			"results": []interface{}{},
		}
	} else {
		// /test-ruleset format (original)
		return map[string]interface{}{
			"success": success,
			"error":   errorMsg,
			"result":  nil,
		}
	}
}

func testRuleset(c echo.Context) error {
	id := c.Param("id") // May be empty for /test-ruleset-content endpoint

	// Parse request body
	var req struct {
		Data    map[string]interface{} `json:"data"`
		Content string                 `json:"content,omitempty"` // Optional content for direct testing
	}

	if err := c.Bind(&req); err != nil {
		isContentMode := req.Content != ""
		return c.JSON(http.StatusBadRequest, rulesetErrorResponse(isContentMode, false, "Invalid request body: "+err.Error()))
	}

	// Check if input data is provided
	if req.Data == nil {
		isContentMode := req.Content != ""
		return c.JSON(http.StatusBadRequest, rulesetErrorResponse(isContentMode, false, "Input data is required"))
	}

	var rulesetContent string
	var isTemp bool

	// If content is provided directly, use it (for /test-ruleset-content endpoint)
	if req.Content != "" {
		rulesetContent = req.Content
		isTemp = false // Direct content is not considered temporary
	} else if id != "" {
		// Original logic to find ruleset by ID (for /test-ruleset/:id endpoint)
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
				r, exists := project.GetRuleset(id)
				if !exists {
					// Check if ruleset exists in new rulesets
					content, ok := project.GetRulesetNew(id)
					if !ok {
						return c.JSON(http.StatusNotFound, rulesetErrorResponse(false, false, "Ruleset not found: "+id))
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
						"results": []interface{}{},
					})
				}
				rulesetContent = content
			}
		}
	} else {
		// Neither content nor id provided
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Either ruleset ID or content must be provided",
			"results": []interface{}{},
		})
	}

	// Create a temporary ruleset for testing
	tempRuleset, err := rules_engine.NewRuleset("", rulesetContent, "temp_test_"+fmt.Sprintf("%d", time.Now().UnixNano()))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse ruleset: " + err.Error(),
			"results": []interface{}{},
		})
	}

	// Create channels for testing
	inputCh := make(chan map[string]interface{}, 100)
	outputCh := make(chan map[string]interface{}, 100)

	// Ensure channels are properly cleaned up on function exit
	defer func() {
		// Close input channel safely (we control this channel)
		select {
		default:
			close(inputCh)
		}
		// Note: outputCh will be closed by ruleset.Stop(), we don't close it here
	}()

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

	// Ensure ruleset cleanup on function exit
	defer func() {
		if stopErr := tempRuleset.Stop(); stopErr != nil {
			logger.Warn("Failed to stop temporary ruleset: %v", stopErr)
		}
		// Explicitly set to nil to help GC
		tempRuleset = nil
	}()

	// Send the test data
	inputCh <- req.Data

	// Collect results with timeout
	results := make([]map[string]interface{}, 0)
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var timedOut bool

	for {
		select {
		case result, ok := <-outputCh:
			if !ok {
				// Channel closed, we're done
				timedOut = false
				goto done
			}
			results = append(results, result)
		case <-ticker.C:
			// Use task count for timeout detection
			if len(inputCh) == 0 && tempRuleset.GetRunningTaskCount() == 0 {
				timedOut = false
				goto done
			}
		case <-timeout:
			// Timeout occurred
			timedOut = true
			logger.Warn("Ruleset test timed out after 30 seconds")
			goto done
		}
	}
done:

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
		response["warning"] = "Test timed out after 30 seconds. Results may be incomplete."
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
		Data    map[string]interface{} `json:"data"`
		Content string                 `json:"content,omitempty"` // Optional content for direct testing
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
			"result":  nil,
		})
	}

	var pluginToTest *plugin.Plugin
	var isTemporary bool
	var tempPluginId string

	// If content is provided directly, use it (for /test-plugin-content endpoint)
	if req.Content != "" {
		// Create a temporary plugin for testing (isolated from global registry)
		tempPluginId = fmt.Sprintf("temp_test_content_%d", time.Now().UnixNano())
		tempPlugin, err := plugin.NewTestPlugin("", req.Content, tempPluginId, plugin.YAEGI_PLUGIN)
		if err != nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success": false,
				"error":   "Failed to create plugin: " + err.Error(),
				"result":  nil,
			})
		}

		pluginToTest = tempPlugin
		isTemporary = true

		// No cleanup needed since plugin was never added to global registry
	} else if id != "" {
		// Original logic to find plugin by ID (for /test-plugin/:id endpoint)
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

		// Prioritize temporary version for testing if it exists
		if existsInTemp {
			// Use temporary version for testing (prioritize temporary version)
			tempPluginName := id + "_test_temp"
			tempPlugin, err := plugin.NewTestPlugin("", tempContent, tempPluginName, plugin.YAEGI_PLUGIN)
			if err != nil {
				return c.JSON(http.StatusOK, map[string]interface{}{
					"success": false,
					"error":   fmt.Sprintf("Plugin compilation failed: %v", err),
					"result":  nil,
				})
			}

			pluginToTest = tempPlugin
			isTemporary = true
		} else if existsInMemory {
			// Use existing plugin only if no temporary version exists
			pluginToTest = p
			isTemporary = false
		}
	} else {
		// Neither content nor id provided
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Either plugin ID or content must be provided",
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
			out, exists := project.GetOutput(id)
			if !exists {
				// Check if output exists in new outputs
				content, ok := project.GetOutputNew(id)
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

	// Ensure input channel is properly closed on function exit
	defer func() {
		// Close input channel safely to prevent leaks
		select {
		default:
			close(inputCh)
		}
	}()

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

// Helper function to return appropriate error format based on call mode
func projectErrorResponse(isContentMode bool, httpStatus int, success bool, errorMsg string) map[string]interface{} {
	if isContentMode {
		// /test-project-content format
		return map[string]interface{}{
			"success": success,
			"error":   errorMsg,
			"outputs": map[string][]map[string]interface{}{},
		}
	} else {
		// /test-project format
		return map[string]interface{}{
			"success": success,
			"error":   errorMsg,
			"result":  nil,
		}
	}
}

func testProject(c echo.Context) error {
	// Smart parameter detection: check both URL parameter positions
	id := c.Param("id")                      // For /test-project/:id
	inputNodeFromURL := c.Param("inputNode") // For /test-project-content/:inputNode

	// Parse request body
	var req struct {
		InputNode string                 `json:"input_node,omitempty"` // For /test-project/:id
		Content   string                 `json:"content,omitempty"`    // For /test-project-content/:inputNode
		Data      map[string]interface{} `json:"data"`
	}

	if err := c.Bind(&req); err != nil {
		isContentMode := inputNodeFromURL != ""
		return c.JSON(http.StatusBadRequest, projectErrorResponse(isContentMode, http.StatusBadRequest, false, "Invalid request body: "+err.Error()))
	}

	// Check if input data is provided
	if req.Data == nil {
		isContentMode := inputNodeFromURL != ""
		return c.JSON(http.StatusBadRequest, projectErrorResponse(isContentMode, http.StatusBadRequest, false, "Input data is required"))
	}

	// Determine call mode and extract parameters
	var inputNodeName string
	var projectContent string
	var isTemp bool
	var isContentMode bool

	if inputNodeFromURL != "" && req.Content != "" {
		// /test-project-content/:inputNode mode
		inputNodeName = inputNodeFromURL
		projectContent = req.Content
		isTemp = true
		isContentMode = true
	} else if id != "" && req.InputNode != "" {
		// /test-project/:id mode
		// Parse input node
		nodeParts := strings.Split(req.InputNode, ".")
		if len(nodeParts) != 2 || strings.ToLower(nodeParts[0]) != "input" {
			return c.JSON(http.StatusBadRequest, projectErrorResponse(false, http.StatusBadRequest, false, "Invalid input node format. Expected 'input.name'"))
		}
		inputNodeName = nodeParts[1]
		isContentMode = false

		// Find project content by ID
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
				proj, exists := project.GetProject(id)
				if !exists {
					// Check if project exists in new projects
					content, ok := project.GetProjectNew(id)
					if !ok {
						return c.JSON(http.StatusNotFound, map[string]interface{}{
							"success": false,
							"error":   "Project not found: " + id,
							"outputs": map[string][]map[string]interface{}{},
						})
					}
					projectContent = content
					isTemp = true
				} else {
					projectContent = proj.Config.RawConfig
				}
			} else {
				content, err := ReadComponent(formalPath)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]interface{}{
						"success": false,
						"error":   "Failed to read project: " + err.Error(),
						"outputs": map[string][]map[string]interface{}{},
					})
				}
				projectContent = content
			}
		}
	} else {
		// Invalid parameters
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Either (project ID + input_node) or (inputNode URL param + content) must be provided",
			"outputs": map[string][]map[string]interface{}{},
		})
	}

	// Create temporary project for testing (isolated from GlobalProject)
	testProjectId := fmt.Sprintf("test_%s_%d", id, time.Now().UnixNano())
	tempProject, err := project.NewProject("", projectContent, testProjectId, true) // testing=true keeps it isolated
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse project: " + err.Error(),
			"result":  nil,
		})
	}

	// Ensure cleanup on exit - test projects are isolated so just stop and clean up directly
	defer func() {
		// First, clear TestCollectionChan references to prevent access to closed channels
		for _, outputComp := range tempProject.Outputs {
			if outputComp.TestCollectionChan != nil {
				outputComp.TestCollectionChan = nil
			}
		}

		// Stop the project (this will handle component cleanup)
		if stopErr := tempProject.Stop(true); stopErr != nil {
			logger.Warn("Failed to stop test project: %v", stopErr)
		}

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
	err = tempProject.Start(true)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to start project: " + err.Error(),
			"result":  nil,
		})
	}

	// Create collection channels for outputs in testing mode
	outputChannels := make(map[string]chan map[string]interface{})
	outputIdToPNS := make(map[string]string) // Map output ID to PNS for lookup
	for pns, outputComp := range tempProject.Outputs {
		testChan := make(chan map[string]interface{}, 100)
		outputChannels[outputComp.Id] = testChan // Use output ID as key instead of PNS
		outputIdToPNS[outputComp.Id] = pns

		// Set TestCollectionChan to collect output data without sending to external systems
		outputComp.TestCollectionChan = &testChan
		logger.Info("Set test collection channel for output", "output", outputComp.Id, "pns", pns, "project", testProjectId)
	}

	// Ensure output channels are properly closed on function exit
	defer func() {
		// Close all output test channels to prevent leaks
		for outputId, testChan := range outputChannels {
			select {
			default:
				close(testChan)
				logger.Debug("Closed test channel for output", "outputId", outputId, "project", testProjectId)
			}
		}
	}()

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

	// Wait for processing with timeout (strategy depends on call mode)
	var timedOut bool
	outputResults := make(map[string][]map[string]interface{})

	// Initialize output results
	for outputId := range outputChannels {
		outputResults[outputId] = []map[string]interface{}{}
	}

	if isContentMode {
		// Simple strategy for content mode (like original testProjectContent)
		time.Sleep(500 * time.Millisecond)

		// Collect results from output channels with timeout
		collectTimeout := time.After(1000 * time.Millisecond)
		for outputId, testChan := range outputChannels {
			// Collect messages from this output channel
			for {
				select {
				case result, ok := <-testChan:
					if !ok {
						// Channel is closed
						goto nextOutputContent
					}
					outputResults[outputId] = append(outputResults[outputId], result)
				case <-collectTimeout:
					// Timeout reached
					goto nextOutputContent
				case <-time.After(100 * time.Millisecond):
					// No more messages after 100ms, assume we're done with this output
					goto nextOutputContent
				}
			}
		nextOutputContent:
		}
		timedOut = false
	} else {
		// Complex strategy for ID mode (original testProject logic)
		timeout := time.After(30 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Collect any available results
				for outputId, outputChan := range outputChannels {
					for {
						select {
						case msg := <-outputChan:
							outputResults[outputId] = append(outputResults[outputId], msg)
							logger.Info("Collected message from output", "outputId", outputId, "message_count", len(outputResults[outputId]))
						default:
							goto nextOutputID
						}
					}
				nextOutputID:
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
	}
done:

	// Return the results with appropriate format based on call mode
	response := map[string]interface{}{
		"success": true,
		"outputs": outputResults,
	}

	// Add fields based on call mode
	if isContentMode {
		// /test-project-content format: minimal response
		response["isTemp"] = true
	} else {
		// /test-project format: full response with legacy fields
		response["isTemp"] = isTemp
		response["inputNode"] = req.InputNode
		response["timeout"] = timedOut

		if timedOut {
			response["warning"] = "Test timed out after 30 seconds. Results may be incomplete."
		}
	}

	return c.JSON(http.StatusOK, response)
}

func getProjectInputs(c echo.Context) error {
	id := c.Param("id")

	// Try to get from existing running project first
	if existingProj, exists := project.GetProject(id); exists {
		// Project is already loaded in memory, extract directly
		inputs := extractInputsFromProject(existingProj)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"isTemp":  false,
			"inputs":  inputs,
		})
	}

	// Check if project exists in pending changes or file system
	var projectContent string
	var isTemp bool

	if tempContent, hasTemp := project.GetProjectNew(id); hasTemp {
		projectContent = tempContent
		isTemp = true
	} else {
		// Try to read from file system
		formalPath, formalExists := GetComponentPath("project", id, false)
		if !formalExists {
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"success": false,
				"error":   "Project not found: " + id,
			})
		}

		content, err := ReadComponent(formalPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "Failed to read project: " + err.Error(),
			})
		}
		projectContent = content
		isTemp = false
	}

	// Parse configuration directly without creating a temporary project
	inputs, err := parseProjectInputsDirect(projectContent)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse project: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"isTemp":  isTemp,
		"inputs":  inputs,
	})
}

// extractInputsFromProject extracts input information from an existing project
func extractInputsFromProject(proj *project.Project) []map[string]string {
	inputs := []map[string]string{}
	inputNames := make(map[string]bool)

	// Extract unique input names from FlowNodes
	for _, node := range proj.FlowNodes {
		if node.FromType == "INPUT" {
			if !inputNames[node.FromID] {
				inputNames[node.FromID] = true
				inputs = append(inputs, map[string]string{
					"id":   "input." + node.FromID,
					"name": node.FromID,
					"type": "virtual", // Virtual input node for testing
				})
			}
		}
	}

	// Sort input node list by name
	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i]["name"] < inputs[j]["name"]
	})

	return inputs
}

// parseProjectInputsDirect parses project configuration and extracts input information without creating a project instance
func parseProjectInputsDirect(projectContent string) ([]map[string]string, error) {
	// Parse the YAML content directly
	var cfg project.ProjectConfig
	if err := yaml.Unmarshal([]byte(projectContent), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if strings.TrimSpace(cfg.Content) == "" {
		return nil, fmt.Errorf("project content cannot be empty")
	}

	// Parse content to extract input information
	lines := strings.Split(cfg.Content, "\n")
	inputNames := make(map[string]bool)

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse arrow format: ->
		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line format at line %d: %s", lineNum+1, line)
		}

		from := strings.TrimSpace(parts[0])

		// Parse from node type
		fromType, fromID := parseNodeDirect(from)

		if fromType == "" {
			return nil, fmt.Errorf("invalid node format at line %d: %s", lineNum+1, from)
		}

		// Collect input names
		if fromType == "INPUT" {
			inputNames[fromID] = true
		}
	}

	// Convert to result format
	inputs := []map[string]string{}
	for name := range inputNames {
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

	return inputs, nil
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
		tempContent, existsNew := project.GetInputNew(id)
		inputComp, exists := project.GetInput(id)

		if !existsNew && !exists {
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
		tempContent, existsNew := project.GetOutputNew(id)
		outputComp, exists := project.GetOutput(id)

		if !existsNew && !exists {
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

func getProjectComponents(c echo.Context) error {
	id := c.Param("id")

	// Try to get from existing running project first
	if existingProj, exists := project.GetProject(id); exists {
		// Project is already loaded in memory, extract directly
		result := extractComponentsFromProject(existingProj)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success":         true,
			"isTemp":          false,
			"totalComponents": result["totalComponents"],
			"componentCounts": result["componentCounts"],
			"components":      result["components"],
		})
	}

	// Check if project exists in pending changes
	var projectContent string
	var isTemp bool

	if tempContent, hasTemp := project.GetProjectNew(id); hasTemp {
		projectContent = tempContent
		isTemp = true
	} else {
		// Try to read from file system
		formalPath, formalExists := GetComponentPath("project", id, false)
		if !formalExists {
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"success": false,
				"error":   "Project not found: " + id,
			})
		}

		content, err := ReadComponent(formalPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "Failed to read project: " + err.Error(),
			})
		}
		projectContent = content
		isTemp = false
	}

	// Parse configuration directly without creating a temporary project
	result, err := parseProjectComponentsDirect(projectContent)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse project: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":         true,
		"isTemp":          isTemp,
		"totalComponents": result["totalComponents"],
		"componentCounts": result["componentCounts"],
		"components":      result["components"],
	})
}

// extractComponentsFromProject extracts component information from an existing project
func extractComponentsFromProject(proj *project.Project) map[string]interface{} {
	componentCounts := map[string]int{
		"inputs":   0,
		"outputs":  0,
		"rulesets": 0,
	}

	components := map[string][]string{
		"inputs":   []string{},
		"outputs":  []string{},
		"rulesets": []string{},
	}

	// Extract unique component names from FlowNodes
	inputNames := make(map[string]bool)
	outputNames := make(map[string]bool)
	rulesetNames := make(map[string]bool)

	for _, node := range proj.FlowNodes {
		if node.FromType == "INPUT" {
			inputNames[node.FromID] = true
		}
		if node.FromType == "OUTPUT" {
			outputNames[node.FromID] = true
		}
		if node.FromType == "RULESET" {
			rulesetNames[node.FromID] = true
		}

		if node.ToType == "INPUT" {
			inputNames[node.ToID] = true
		}
		if node.ToType == "OUTPUT" {
			outputNames[node.ToID] = true
		}
		if node.ToType == "RULESET" {
			rulesetNames[node.ToID] = true
		}
	}

	// Convert to sorted slices
	for name := range inputNames {
		components["inputs"] = append(components["inputs"], name)
	}
	for name := range outputNames {
		components["outputs"] = append(components["outputs"], name)
	}
	for name := range rulesetNames {
		components["rulesets"] = append(components["rulesets"], name)
	}

	// Sort component lists
	sort.Strings(components["inputs"])
	sort.Strings(components["outputs"])
	sort.Strings(components["rulesets"])

	// Update counts
	componentCounts["inputs"] = len(components["inputs"])
	componentCounts["outputs"] = len(components["outputs"])
	componentCounts["rulesets"] = len(components["rulesets"])

	totalComponents := componentCounts["inputs"] + componentCounts["outputs"] + componentCounts["rulesets"]

	return map[string]interface{}{
		"totalComponents": totalComponents,
		"componentCounts": componentCounts,
		"components":      components,
	}
}

// parseProjectComponentsDirect parses project configuration and extracts component information without creating a project instance
func parseProjectComponentsDirect(projectContent string) (map[string]interface{}, error) {
	// Parse the YAML content directly
	var cfg project.ProjectConfig
	if err := yaml.Unmarshal([]byte(projectContent), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if strings.TrimSpace(cfg.Content) == "" {
		return nil, fmt.Errorf("project content cannot be empty")
	}

	// Parse content to extract component information
	lines := strings.Split(cfg.Content, "\n")

	inputNames := make(map[string]bool)
	outputNames := make(map[string]bool)
	rulesetNames := make(map[string]bool)

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse arrow format: ->
		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line format at line %d: %s", lineNum+1, line)
		}

		from := strings.TrimSpace(parts[0])
		to := strings.TrimSpace(parts[1])

		// Parse node types
		fromType, fromID := parseNodeDirect(from)
		toType, toID := parseNodeDirect(to)

		if fromType == "" || toType == "" {
			return nil, fmt.Errorf("invalid node format at line %d: %s -> %s", lineNum+1, from, to)
		}

		// Collect component names
		switch fromType {
		case "INPUT":
			inputNames[fromID] = true
		case "OUTPUT":
			outputNames[fromID] = true
		case "RULESET":
			rulesetNames[fromID] = true
		}

		switch toType {
		case "INPUT":
			inputNames[toID] = true
		case "OUTPUT":
			outputNames[toID] = true
		case "RULESET":
			rulesetNames[toID] = true
		}
	}

	// Convert to sorted slices
	components := map[string][]string{
		"inputs":   []string{},
		"outputs":  []string{},
		"rulesets": []string{},
	}

	for name := range inputNames {
		components["inputs"] = append(components["inputs"], name)
	}
	for name := range outputNames {
		components["outputs"] = append(components["outputs"], name)
	}
	for name := range rulesetNames {
		components["rulesets"] = append(components["rulesets"], name)
	}

	// Sort component lists
	sort.Strings(components["inputs"])
	sort.Strings(components["outputs"])
	sort.Strings(components["rulesets"])

	// Calculate counts
	componentCounts := map[string]int{
		"inputs":   len(components["inputs"]),
		"outputs":  len(components["outputs"]),
		"rulesets": len(components["rulesets"]),
	}

	totalComponents := componentCounts["inputs"] + componentCounts["outputs"] + componentCounts["rulesets"]

	return map[string]interface{}{
		"totalComponents": totalComponents,
		"componentCounts": componentCounts,
		"components":      components,
	}, nil
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
// Optimized: directly extracts PNS from existing projects or parses configuration without creating temporary projects
func getProjectComponentSequences(c echo.Context) error {
	id := c.Param("id")

	// Try to get from existing running project first
	if existingProj, exists := project.GetProject(id); exists {
		// Project is already loaded in memory, extract directly from FlowNodes
		result := extractSequencesFromProject(existingProj)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"isTemp":  false,
			"data":    result,
		})
	}

	// Check if project exists in pending changes
	var projectContent string
	var isTemp bool

	if tempContent, hasTemp := project.GetProjectNew(id); hasTemp {
		projectContent = tempContent
		isTemp = true
	} else {
		// Try to read from file system
		formalPath, formalExists := GetComponentPath("project", id, false)
		if !formalExists {
			return c.JSON(http.StatusNotFound, map[string]interface{}{
				"success": false,
				"error":   "Project not found: " + id,
			})
		}

		content, err := ReadComponent(formalPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"error":   "Failed to read project: " + err.Error(),
			})
		}
		projectContent = content
		isTemp = false
	}

	// Parse configuration directly without creating a temporary project
	result, err := parseProjectSequencesDirect(projectContent)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse project: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"isTemp":  isTemp,
		"data":    result,
	})
}

// extractSequencesFromProject extracts sequence information from an existing project
func extractSequencesFromProject(proj *project.Project) map[string]map[string][]string {
	result := map[string]map[string][]string{
		"input":   make(map[string][]string),
		"output":  make(map[string][]string),
		"ruleset": make(map[string][]string),
	}

	// Collect all PNS sequences from FlowNodes
	allSequences := make(map[string]map[string]map[string]bool) // componentType -> componentId -> set of sequences

	// Process each FlowNode to extract PNS information
	for _, node := range proj.FlowNodes {
		// Extract FROM component sequences
		fromType := strings.ToLower(node.FromType)
		fromId := node.FromID
		fromPNS := node.FromPNS

		// Remove TEST_ prefix if present (testing mode)
		if strings.HasPrefix(fromPNS, "TEST_") {
			fromPNS = strings.TrimPrefix(fromPNS, "TEST_"+proj.Id+"_")
		}

		if fromType == "input" || fromType == "output" || fromType == "ruleset" {
			if allSequences[fromType] == nil {
				allSequences[fromType] = make(map[string]map[string]bool)
			}
			if allSequences[fromType][fromId] == nil {
				allSequences[fromType][fromId] = make(map[string]bool)
			}
			allSequences[fromType][fromId][fromPNS] = true
		}

		// Extract TO component sequences
		toType := strings.ToLower(node.ToType)
		toId := node.ToID
		toPNS := node.ToPNS

		// Remove TEST_ prefix if present (testing mode)
		if strings.HasPrefix(toPNS, "TEST_") {
			toPNS = strings.TrimPrefix(toPNS, "TEST_"+proj.Id+"_")
		}

		if toType == "input" || toType == "output" || toType == "ruleset" {
			if allSequences[toType] == nil {
				allSequences[toType] = make(map[string]map[string]bool)
			}
			if allSequences[toType][toId] == nil {
				allSequences[toType][toId] = make(map[string]bool)
			}
			allSequences[toType][toId][toPNS] = true
		}
	}

	// Convert sets to sorted slices for each component
	for componentType, components := range allSequences {
		for componentId, sequences := range components {
			// Convert set to sorted slice
			sequenceList := make([]string, 0, len(sequences))
			for seq := range sequences {
				sequenceList = append(sequenceList, seq)
			}
			sort.Strings(sequenceList)
			result[componentType][componentId] = sequenceList
		}
	}

	return result
}

// parseProjectSequencesDirect parses project configuration and extracts sequences without creating a project instance
func parseProjectSequencesDirect(projectContent string) (map[string]map[string][]string, error) {
	// Parse the YAML content directly
	var cfg project.ProjectConfig
	if err := yaml.Unmarshal([]byte(projectContent), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if strings.TrimSpace(cfg.Content) == "" {
		return nil, fmt.Errorf("project content cannot be empty")
	}

	// Parse content to build flow graph
	lines := strings.Split(cfg.Content, "\n")
	var flowNodes []project.FlowNode

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse arrow format: ->
		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line format at line %d: %s", lineNum+1, line)
		}

		from := strings.TrimSpace(parts[0])
		to := strings.TrimSpace(parts[1])

		// Parse node types
		fromType, fromID := parseNodeDirect(from)
		toType, toID := parseNodeDirect(to)

		if fromType == "" || toType == "" {
			return nil, fmt.Errorf("invalid node format at line %d: %s -> %s", lineNum+1, from, to)
		}

		// Validate flow rules
		if toType == "INPUT" {
			return nil, fmt.Errorf("INPUT node %q cannot be a destination at line %d", to, lineNum+1)
		}

		if fromType == "OUTPUT" {
			return nil, fmt.Errorf("OUTPUT node %q cannot be a source at line %d", from, lineNum+1)
		}

		tmpNode := project.FlowNode{
			FromType: fromType,
			FromID:   fromID,
			ToID:     toID,
			ToType:   toType,
			Content:  line,
		}

		flowNodes = append(flowNodes, tmpNode)
	}

	// Build PNS for each node
	buildPNSDirect(flowNodes)

	// Extract sequences using the same logic as extractSequencesFromProject
	result := map[string]map[string][]string{
		"input":   make(map[string][]string),
		"output":  make(map[string][]string),
		"ruleset": make(map[string][]string),
	}

	allSequences := make(map[string]map[string]map[string]bool)

	for _, node := range flowNodes {
		// Process FROM component
		fromType := strings.ToLower(node.FromType)
		fromId := node.FromID
		fromPNS := node.FromPNS

		if fromType == "input" || fromType == "output" || fromType == "ruleset" {
			if allSequences[fromType] == nil {
				allSequences[fromType] = make(map[string]map[string]bool)
			}
			if allSequences[fromType][fromId] == nil {
				allSequences[fromType][fromId] = make(map[string]bool)
			}
			allSequences[fromType][fromId][fromPNS] = true
		}

		// Process TO component
		toType := strings.ToLower(node.ToType)
		toId := node.ToID
		toPNS := node.ToPNS

		if toType == "input" || toType == "output" || toType == "ruleset" {
			if allSequences[toType] == nil {
				allSequences[toType] = make(map[string]map[string]bool)
			}
			if allSequences[toType][toId] == nil {
				allSequences[toType][toId] = make(map[string]bool)
			}
			allSequences[toType][toId][toPNS] = true
		}
	}

	// Convert to final format
	for componentType, components := range allSequences {
		for componentId, sequences := range components {
			sequenceList := make([]string, 0, len(sequences))
			for seq := range sequences {
				sequenceList = append(sequenceList, seq)
			}
			sort.Strings(sequenceList)
			result[componentType][componentId] = sequenceList
		}
	}

	return result, nil
}

// parseNodeDirect splits "TYPE.name" into ("TYPE", "name")
func parseNodeDirect(s string) (string, string) {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.ToUpper(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
}

// buildPNSDirect builds ProjectNodeSequence for flow nodes
func buildPNSDirect(flowNodes []project.FlowNode) {
	// Build ProjectNodeSequence recursively for a specific component
	var buildSequence func(component string, visited map[string]bool) string
	buildSequence = func(component string, visited map[string]bool) string {
		// Break cycle detection
		if visited[component] {
			return component
		}
		visited[component] = true
		defer delete(visited, component)

		// Find upstream component for this component using flow nodes
		var upstreamComponent string
		for _, conn := range flowNodes {
			toKey := conn.ToType + "." + conn.ToID
			if toKey == component {
				upstreamComponent = conn.FromType + "." + conn.FromID
				break
			}
		}

		var sequence string
		if upstreamComponent == "" {
			// This is a source component (no upstream)
			sequence = component
		} else {
			// Build sequence by prepending upstream sequence
			upstreamSequence := buildSequence(upstreamComponent, visited)
			sequence = upstreamSequence + "." + component
		}

		return sequence
	}

	// Process each connection and set PNS values
	for i := range flowNodes {
		// For FROM component: build sequence independently
		fromKey := flowNodes[i].FromType + "." + flowNodes[i].FromID
		fromSequence := buildSequence(fromKey, make(map[string]bool))

		// For TO component: build sequence based on FROM component in THIS connection
		toKey := flowNodes[i].ToType + "." + flowNodes[i].ToID
		toSequence := fromSequence + "." + toKey

		flowNodes[i].FromPNS = fromSequence
		flowNodes[i].ToPNS = toSequence
	}
}
