package api

import (
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/local_plugin"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"encoding/json"
	"fmt"
	"net/http"
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

	// Check if plugin exists
	p, ok := plugin.Plugins[id]
	if !ok {
		_, existsNew := plugin.PluginsNew[id]
		if existsNew {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success": false,
				"error":   "Plugin has pending changes, cannot test",
				"result":  nil,
			})
		}
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Plugin not found: " + id,
			"result":  nil,
		})
	}

	// 将输入数据转换为字符串参数
	// 插件通常接受JSON字符串作为输入
	jsonData, err := json.Marshal(req.Data)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   "Failed to serialize input data: " + err.Error(),
			"result":  nil,
		})
	}

	// 创建参数数组，只传入一个JSON字符串参数
	args := []interface{}{string(jsonData)}

	// Determine plugin type and execute
	var result interface{}
	var success bool
	var errMsg string

	switch p.Type {
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

		// 执行插件
		boolResult := p.FuncEvalCheckNode(args...)
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
			// 提取插件描述（如果有的话）
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
	// 尝试查找注释中的描述
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

	// 如果没有找到合适的注释，返回默认描述
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

	// Check if there's a temporary file first
	tempPath, tempExists := GetComponentPath("project", id, true)
	if tempExists {
		content, err := ReadComponent(tempPath)
		if err == nil {
			projectContent = content
			isTemp = true
		}
	}

	// If no temp file, check formal file
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

	// Create a temporary project for testing
	tempProject, err := project.NewProject("", projectContent, "temp_test_"+id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse project: " + err.Error(),
			"result":  nil,
		})
	}

	// Check if the specified input exists in the project
	if _, exists := tempProject.Inputs[inputNodeName]; !exists {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input node not found in project: " + inputNodeName,
			"result":  nil,
		})
	}

	// Create a map to collect o results
	outputResults := make(map[string][]map[string]interface{})
	outputChannels := make(map[string]chan map[string]interface{})

	// Create channels to capture o data
	for outputName, o := range tempProject.Outputs {
		// Create a channel for each o
		outputChan := make(chan map[string]interface{}, 10)
		outputChannels[outputName] = outputChan

		// Replace the o's upstream channel with our test channel
		for _, upChan := range o.UpStream {
			// Save the original channel reference
			originalChan := *upChan

			// Create a new goroutine to forward messages and capture them
			go func(outName string, origChan chan map[string]interface{}, testChan chan map[string]interface{}) {
				for msg := range testChan {
					// Forward to original channel
					origChan <- msg

					// Make a copy for our results
					msgCopy := make(map[string]interface{})
					for k, v := range msg {
						msgCopy[k] = v
					}

					// Add metadata
					msgCopy["_HUB_OUTPUT_NAME"] = outName
					msgCopy["_HUB_TIMESTAMP"] = time.Now().UnixNano() / int64(time.Millisecond)

					// Send to our results channel
					outputChan <- msgCopy
				}
			}(outputName, originalChan, *upChan)
		}
	}

	// Start the project
	err = tempProject.Start()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "Failed to start project: " + err.Error(),
			"result":  nil,
		})
	}

	// Find the input node's downstream channels
	inputNode := tempProject.Inputs[inputNodeName]
	if len(inputNode.DownStream) == 0 {
		tempProject.Stop()
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Input node has no downstream connections",
			"result":  nil,
		})
	}

	// Send test data to all downstream channels of the input
	for _, downChan := range inputNode.DownStream {
		*downChan <- req.Data
	}

	// Wait a bit to collect results
	time.Sleep(1000 * time.Millisecond)

	// Collect results from o channels
	for outputName, outputChan := range outputChannels {
		// Collect all available messages
		results := []map[string]interface{}{}
		for {
			select {
			case msg := <-outputChan:
				results = append(results, msg)
			default:
				// No more messages
				outputResults[outputName] = results
				goto nextOutput
			}
		}
	nextOutput:
	}

	// Stop the project
	err = tempProject.Stop()
	if err != nil {
		logger.Warn("Failed to stop temporary project: %v", err)
	}

	// Get project structure for visualization
	projectStructure, err := getProjectStructure(tempProject)
	if err != nil {
		logger.Warn("Failed to get project structure: %v", err)
	}

	// Return the results
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":   true,
		"isTemp":    isTemp,
		"outputs":   outputResults,
		"structure": projectStructure,
		"inputNode": req.InputNode,
	})
}

func getProjectInputs(c echo.Context) error {
	id := c.Param("id")

	// 检查项目是否存在
	var projectContent string
	var isTemp bool

	// 首先检查是否有临时文件
	tempPath, tempExists := GetComponentPath("project", id, true)
	if tempExists {
		content, err := ReadComponent(tempPath)
		if err == nil {
			projectContent = content
			isTemp = true
		}
	}

	// 如果没有临时文件，检查正式文件
	if projectContent == "" {
		formalPath, formalExists := GetComponentPath("project", id, false)
		if !formalExists {
			// 检查项目是否存在于内存中
			proj := project.GlobalProject.Projects[id]
			if proj == nil {
				// 检查项目是否存在于新项目中
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

	// 创建临时项目以解析配置
	tempProject, err := project.NewProject("", projectContent, "temp_list_"+id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse project: " + err.Error(),
		})
	}

	// 收集输入节点信息
	inputs := []map[string]string{}
	for name, i := range tempProject.Inputs {
		inputs = append(inputs, map[string]string{
			"id":   "input." + name,
			"name": name,
			"type": string(i.Type),
		})
	}

	// 返回结果
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"isTemp":  isTemp,
		"inputs":  inputs,
	})
}

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
