package project

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/rules_engine"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

var GlobalProject *GlobalProjectInfo

// projectCommandHandler implements cluster.ProjectCommandHandler interface
type projectCommandHandler struct{}

// ExecuteCommand implements the ProjectCommandHandler interface
func (h *projectCommandHandler) ExecuteCommand(projectID, action string) error {
	proj, exists := GlobalProject.Projects[projectID]
	if !exists {
		return fmt.Errorf("project not found: %s", projectID)
	}

	// Get node ID from common config instead of cluster package
	nodeID := common.Config.LocalIP

	switch action {
	case "start":
		if proj.Status == ProjectStatusRunning {
			logger.Info("Project already running", "project_id", projectID)
			return nil
		}
		err := proj.Start()
		if err != nil {
			// Record operation failure
			common.RecordProjectOperation(common.OpTypeProjectStart, projectID, "failed", err.Error(), map[string]interface{}{
				"triggered_by": "cluster_command",
				"node_id":      nodeID,
			})
			return fmt.Errorf("failed to start project: %w", err)
		}
		// Record operation success
		common.RecordProjectOperation(common.OpTypeProjectStart, projectID, "success", "", map[string]interface{}{
			"triggered_by": "cluster_command",
			"node_id":      nodeID,
		})
		logger.Info("Project started successfully via cluster command", "project_id", projectID)
		return nil

	case "stop":
		if proj.Status == ProjectStatusStopped {
			logger.Info("Project already stopped", "project_id", projectID)
			return nil
		}
		err := proj.Stop()
		if err != nil {
			// Record operation failure
			common.RecordProjectOperation(common.OpTypeProjectStop, projectID, "failed", err.Error(), map[string]interface{}{
				"triggered_by": "cluster_command",
				"node_id":      nodeID,
			})
			return fmt.Errorf("failed to stop project: %w", err)
		}
		// Record operation success
		common.RecordProjectOperation(common.OpTypeProjectStop, projectID, "success", "", map[string]interface{}{
			"triggered_by": "cluster_command",
			"node_id":      nodeID,
		})
		logger.Info("Project stopped successfully via cluster command", "project_id", projectID)
		return nil

	case "restart":
		// First stop if running
		if proj.Status == ProjectStatusRunning {
			err := proj.Stop()
			if err != nil {
				common.RecordProjectOperation(common.OpTypeProjectRestart, projectID, "failed", fmt.Sprintf("Failed to stop: %v", err), map[string]interface{}{
					"triggered_by": "cluster_command",
					"node_id":      nodeID,
				})
				return fmt.Errorf("failed to stop project for restart: %w", err)
			}
		}

		// Then start
		err := proj.Start()
		if err != nil {
			common.RecordProjectOperation(common.OpTypeProjectRestart, projectID, "failed", fmt.Sprintf("Failed to start: %v", err), map[string]interface{}{
				"triggered_by": "cluster_command",
				"node_id":      nodeID,
			})
			return fmt.Errorf("failed to start project for restart: %w", err)
		}
		// Record operation success
		common.RecordProjectOperation(common.OpTypeProjectRestart, projectID, "success", "", map[string]interface{}{
			"triggered_by": "cluster_command",
			"node_id":      nodeID,
		})
		logger.Info("Project restarted successfully via cluster command", "project_id", projectID)
		return nil

	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

// GetProjectCommandHandler returns the project command handler for registration
func GetProjectCommandHandler() interface{} {
	return &projectCommandHandler{}
}

func init() {
	GlobalProject = &GlobalProjectInfo{}
	GlobalProject.Projects = make(map[string]*Project)
	GlobalProject.Inputs = make(map[string]*input.Input)
	GlobalProject.Outputs = make(map[string]*output.Output)
	GlobalProject.Rulesets = make(map[string]*rules_engine.Ruleset)

	GlobalProject.ProjectsNew = make(map[string]string)
	GlobalProject.InputsNew = make(map[string]string)
	GlobalProject.OutputsNew = make(map[string]string)
	GlobalProject.RulesetsNew = make(map[string]string)

	GlobalProject.msgChans = make(map[string]chan map[string]interface{})
	GlobalProject.msgChansCounter = make(map[string]*atomic.Int64)

	// Mapping between logical edge ("FROM->TO") and its channelId
	GlobalProject.edgeChanIds = make(map[string]string)
}

func Verify(path string, raw string) error {
	var err error
	var cfg ProjectConfig
	var p *Project

	// Use common file reading function
	data, err := common.ReadContentFromPathOrRaw(path, raw)
	if err != nil {
		return fmt.Errorf("failed to read project configuration: %w", err)
	}

	if path != "" {
		cfg.RawConfig = string(data)
	} else {
		cfg.RawConfig = raw
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		// Enhanced error parsing to extract accurate line numbers
		errString := err.Error()

		// Handle different types of YAML errors
		if yamlErr, ok := err.(*yaml.TypeError); ok && len(yamlErr.Errors) > 0 {
			// Type errors with multiple error messages
			errMsg := yamlErr.Errors[0]
			lineInfo := ""
			for _, line := range yamlErr.Errors {
				if strings.Contains(line, "line") {
					lineInfo = line
					break
				}
			}
			return fmt.Errorf("failed to parse project configuration: %s (location: %s)", errMsg, lineInfo)
		} else {
			// General YAML parsing errors - extract line number from error string
			// Common patterns: "yaml: line 10:", "at line 10", "line 10:"
			linePattern := `(?i)(?:yaml: |at )?line (\d+)`
			if match := regexp.MustCompile(linePattern).FindStringSubmatch(errString); len(match) > 1 {
				lineNum := match[1]
				return fmt.Errorf("YAML parse error: yaml-line %s: %s", lineNum, errString)
			}
			// If no line number found, return the error as-is but with consistent format
			return fmt.Errorf("YAML parse error: %s", errString)
		}
	}

	if strings.TrimSpace(cfg.Content) == "" {
		return fmt.Errorf("project content cannot be empty in configuration file")
	}

	p = &Project{
		Id:     cfg.Id,
		Status: ProjectStatusStopped,
		Config: &cfg,
	}

	_, err = p.parseContent()
	if err != nil {
		return fmt.Errorf("failed to parse project content: %v", err)
	}

	return nil
}

// NewProject creates a new project instance from a configuration file
// pp: Path to the project configuration file
func NewProject(path string, raw string, id string) (*Project, error) {
	var cfg ProjectConfig
	var data []byte
	var err error

	err = Verify(path, raw)
	if err != nil {
		return nil, fmt.Errorf("project config verify error: %s %s", id, err.Error())
	}

	if path != "" {
		data, _ = os.ReadFile(path)
		cfg.RawConfig = string(data)
		cfg.Path = path
	} else {
		cfg.RawConfig = raw
		data = []byte(raw)
	}
	cfg.Id = id

	_ = yaml.Unmarshal(data, &cfg)

	now := time.Now()
	p := &Project{
		Id:              cfg.Id,
		Status:          ProjectStatusStopped, // Default to stopped status, will be started by StartAllProject
		StatusChangedAt: &now,
		Config:          &cfg,
		Inputs:          make(map[string]*input.Input),
		Outputs:         make(map[string]*output.Output),
		Rulesets:        make(map[string]*rules_engine.Ruleset),
		MsgChannels:     make([]string, 0),
		stopChan:        make(chan struct{}),
		metrics: &ProjectMetrics{
			InputQPS:  make(map[string]uint64),
			OutputQPS: make(map[string]uint64),
		},
	}

	// Initialize components
	if err := p.initComponents(); err != nil {
		now := time.Now()
		p.Status = ProjectStatusError
		p.StatusChangedAt = &now
		p.Err = err

		return p, fmt.Errorf("failed to initialize project components: %w", err)
	}

	// Note: Project status is now properly loaded from .project_status file in main.go
	// No need to override it here - preserve the loaded status
	logger.Info("Project created with loaded status", "id", p.Id, "status", p.Status)

	return p, nil
}

// NewProjectForTesting creates a new project instance specifically for testing
// This version creates completely independent component instances (except inputs) to avoid affecting the live environment
func NewProjectForTesting(path string, raw string, id string) (*Project, error) {
	var cfg ProjectConfig
	var data []byte
	var err error

	err = Verify(path, raw)
	if err != nil {
		return nil, fmt.Errorf("project config verify error: %s %s", id, err.Error())
	}

	if path != "" {
		data, _ = os.ReadFile(path)
		cfg.RawConfig = string(data)
		cfg.Path = path
	} else {
		cfg.RawConfig = raw
		data = []byte(raw)
	}
	cfg.Id = id

	_ = yaml.Unmarshal(data, &cfg)

	now := time.Now()
	p := &Project{
		Id:              cfg.Id,
		Status:          ProjectStatusStopped, // Start as stopped for testing
		StatusChangedAt: &now,
		Config:          &cfg,
		Inputs:          make(map[string]*input.Input),
		Outputs:         make(map[string]*output.Output),
		Rulesets:        make(map[string]*rules_engine.Ruleset),
		MsgChannels:     make([]string, 0),
		stopChan:        make(chan struct{}),
		metrics: &ProjectMetrics{
			InputQPS:  make(map[string]uint64),
			OutputQPS: make(map[string]uint64),
		},
	}

	// Initialize components with independent instances for testing
	if err := p.initComponentsForTesting(); err != nil {
		now := time.Now()
		p.Status = ProjectStatusError
		p.StatusChangedAt = &now
		p.Err = err
		return p, fmt.Errorf("failed to initialize test project components: %w", err)
	}

	return p, nil
}

// loadComponents loads and initializes all project components
// inputNames: List of input component IDs
// outputNames: List of output component IDs
// rulesetNames: List of ruleset IDs
func (p *Project) loadComponents(inputNames []string, outputNames []string, rulesetNames []string) error {
	for _, v := range inputNames {
		// Check if formal component exists
		if _, ok := GlobalProject.Inputs[v]; !ok {
			// Check if it's a temporary component, temporary components should not be referenced
			if _, tempExists := GlobalProject.InputsNew[v]; tempExists {
				return fmt.Errorf("cannot reference temporary input component '%s', please save it first", v)
			}
			return fmt.Errorf("conn't find input %s", v)
		}
	}

	for _, v := range outputNames {
		// Check if formal components exist
		if _, ok := GlobalProject.Outputs[v]; !ok {
			// Check if it's a temporary component, temporary components should not be referenced
			if _, tempExists := GlobalProject.OutputsNew[v]; tempExists {
				return fmt.Errorf("cannot reference temporary output component '%s', please save it first", v)
			}
			return fmt.Errorf("conn't find output %s", v)
		}
	}

	for _, v := range rulesetNames {
		// Check if formal components exist
		if _, ok := GlobalProject.Rulesets[v]; !ok {
			// Check if it's a temporary component, temporary components should not be referenced
			if _, tempExists := GlobalProject.RulesetsNew[v]; tempExists {
				return fmt.Errorf("cannot reference temporary ruleset component '%s', please save it first", v)
			}
			return fmt.Errorf("conn't find ruleset %s", v)
		}
	}
	return nil
}

// initComponents initializes all project components and their connections
func (p *Project) initComponents() error {
	// Parse project content to build the data flow graph
	flowGraph, err := p.parseContent()
	if err != nil {
		return fmt.Errorf("failed to parse project content: %v", err)
	}

	// Collect all input/output/ruleset names from flowGraph
	inputNames := []string{}
	outputNames := []string{}
	rulesetNames := []string{}

	nameExists := func(list []string, name string) bool {
		for _, n := range list {
			if n == name {
				return true
			}
		}
		return false
	}

	for from, tos := range flowGraph {
		fromParts := strings.Split(from, ".")
		if len(fromParts) == 2 {
			switch strings.ToUpper(fromParts[0]) {
			case "INPUT":
				if !nameExists(inputNames, fromParts[1]) {
					inputNames = append(inputNames, fromParts[1])
				}
			case "OUTPUT":
				if !nameExists(outputNames, fromParts[1]) {
					outputNames = append(outputNames, fromParts[1])
				}
			case "RULESET":
				if !nameExists(rulesetNames, fromParts[1]) {
					rulesetNames = append(rulesetNames, fromParts[1])
				}
			}
		}

		for _, to := range tos {
			toParts := strings.Split(to, ".")
			if len(toParts) == 2 {
				switch strings.ToUpper(toParts[0]) {
				case "INPUT":
					if !nameExists(inputNames, toParts[1]) {
						inputNames = append(inputNames, toParts[1])
					}
				case "OUTPUT":
					if !nameExists(outputNames, toParts[1]) {
						outputNames = append(outputNames, toParts[1])
					}
				case "RULESET":
					if !nameExists(rulesetNames, toParts[1]) {
						rulesetNames = append(rulesetNames, toParts[1])
					}
				}
			}
		}
	}

	// load input/output/ruleset
	err = p.loadComponents(inputNames, outputNames, rulesetNames)
	if err != nil {
		return err
	}

	// Actually assign components to the project
	for _, name := range inputNames {
		if i, ok := GlobalProject.Inputs[name]; ok {
			p.Inputs[name] = i
		}
	}

	for _, name := range outputNames {
		if o, ok := GlobalProject.Outputs[name]; ok {
			p.Outputs[name] = o
		}
	}

	for _, name := range rulesetNames {
		if ruleset, ok := GlobalProject.Rulesets[name]; ok {
			p.Rulesets[name] = ruleset
		}
	}

	// Create channel connections
	return p.createChannelConnections(flowGraph)
}

// initComponentsForTesting initializes all project components for testing with independent instances
// This creates new component instances to avoid affecting the live environment
func (p *Project) initComponentsForTesting() error {
	// Parse project content to build the data flow graph
	flowGraph, err := p.parseContent()
	if err != nil {
		return fmt.Errorf("failed to parse project content: %v", err)
	}

	// Collect all input/output/ruleset names from flowGraph
	var inputNames []string
	var outputNames []string
	var rulesetNames []string

	nameExists := func(list []string, name string) bool {
		for _, n := range list {
			if n == name {
				return true
			}
		}
		return false
	}

	for from, tos := range flowGraph {
		fromParts := strings.Split(from, ".")
		if len(fromParts) == 2 {
			switch strings.ToUpper(fromParts[0]) {
			case "INPUT":
				if !nameExists(inputNames, fromParts[1]) {
					inputNames = append(inputNames, fromParts[1])
				}
			case "OUTPUT":
				if !nameExists(outputNames, fromParts[1]) {
					outputNames = append(outputNames, fromParts[1])
				}
			case "RULESET":
				if !nameExists(rulesetNames, fromParts[1]) {
					rulesetNames = append(rulesetNames, fromParts[1])
				}
			}
		}

		for _, to := range tos {
			toParts := strings.Split(to, ".")
			if len(toParts) == 2 {
				switch strings.ToUpper(toParts[0]) {
				case "INPUT":
					if !nameExists(inputNames, toParts[1]) {
						inputNames = append(inputNames, toParts[1])
					}
				case "OUTPUT":
					if !nameExists(outputNames, toParts[1]) {
						outputNames = append(outputNames, toParts[1])
					}
				case "RULESET":
					if !nameExists(rulesetNames, toParts[1]) {
						rulesetNames = append(rulesetNames, toParts[1])
					}
				}
			}
		}
	}

	// For testing, we don't need to initialize actual input components
	// Just validate that the referenced inputs exist in the system for configuration validation
	for _, v := range inputNames {
		if _, ok := GlobalProject.Inputs[v]; !ok {
			return fmt.Errorf("input %s referenced in project flow but not found in system", v)
		}
	}

	// Check if outputs exist (formal or temp configs)
	for _, v := range outputNames {
		if _, ok := GlobalProject.Outputs[v]; !ok {
			// Check if output exists in temp configs
			if _, ok := GlobalProject.OutputsNew[v]; !ok {
				return fmt.Errorf("output %s referenced in project flow but not found", v)
			}
		}
	}

	// Check if rulesets exist (formal or temp configs)
	for _, v := range rulesetNames {
		if _, ok := GlobalProject.Rulesets[v]; !ok {
			// Check if ruleset exists in temp configs
			if _, ok := GlobalProject.RulesetsNew[v]; !ok {
				return fmt.Errorf("ruleset %s referenced in project flow but not found", v)
			}
		}
	}

	// For testing, create virtual input nodes (just placeholders for flow graph validation)
	// We don't need actual input component instances - users will provide test data directly
	for _, name := range inputNames {
		// Create a completely isolated input placeholder for testing
		testInputId := fmt.Sprintf("test_%s_%s_%d", p.Id, name, time.Now().UnixNano())
		testInput := &input.Input{
			Id:                  testInputId,
			DownStream:          make([]*chan map[string]interface{}, 0),
			ProjectNodeSequence: fmt.Sprintf("TEST.%s.%s", p.Id, name),
		}
		p.Inputs[name] = testInput
	}

	// Create independent output instances for testing
	for _, name := range outputNames {
		var outputConfig string
		var err error

		// Check if there's a temp config first
		if tempConfig, ok := GlobalProject.OutputsNew[name]; ok {
			outputConfig = tempConfig
		} else if existingOutput, ok := GlobalProject.Outputs[name]; ok {
			outputConfig = existingOutput.Config.RawConfig
		} else {
			return fmt.Errorf("output %s not found", name)
		}

		// Create a completely isolated output instance for testing
		// Use unique ID with timestamp to avoid any conflicts
		testOutputId := fmt.Sprintf("test_%s_%s_%d", p.Id, name, time.Now().UnixNano())
		testOutput, err := output.NewOutput("", outputConfig, testOutputId)
		if err != nil {
			return fmt.Errorf("failed to create test output %s: %v", name, err)
		}

		// Mark this as a test instance to prevent it from affecting global state
		testOutput.ProjectNodeSequence = fmt.Sprintf("TEST.%s.%s", p.Id, name)

		// Disable sampler for test instances to avoid affecting global sampling state
		testOutput.SetTestMode()

		p.Outputs[name] = testOutput
	}

	// Create independent ruleset instances for testing
	for _, name := range rulesetNames {
		var rulesetConfig string
		var err error

		// Check if there's a temp config first
		if tempConfig, ok := GlobalProject.RulesetsNew[name]; ok {
			rulesetConfig = tempConfig
		} else if existingRuleset, ok := GlobalProject.Rulesets[name]; ok {
			rulesetConfig = existingRuleset.RawConfig
		} else {
			return fmt.Errorf("ruleset %s not found", name)
		}

		// Create a completely isolated ruleset instance for testing
		// Use unique ID with timestamp to avoid any conflicts
		testRulesetId := fmt.Sprintf("test_%s_%s_%d", p.Id, name, time.Now().UnixNano())
		testRuleset, err := rules_engine.NewRuleset("", rulesetConfig, testRulesetId)
		if err != nil {
			return fmt.Errorf("failed to create test ruleset %s: %v", name, err)
		}

		// Mark this as a test instance to prevent it from affecting global state
		testRuleset.ProjectNodeSequence = fmt.Sprintf("TEST.%s.%s", p.Id, name)

		// Disable sampler for test instances to avoid affecting global sampling state
		testRuleset.SetTestMode()

		p.Rulesets[name] = testRuleset
	}

	// Connect components according to the flow graph
	for from, tos := range flowGraph {
		fromParts := strings.Split(from, ".")
		fromType := fromParts[0]
		fromId := fromParts[1]

		for _, to := range tos {
			toParts := strings.Split(to, ".")
			toType := toParts[0]
			toId := toParts[1]

			// Create a channel for this connection
			msgChan := make(chan map[string]interface{}, 1024)

			// Connect based on component types
			switch fromType {
			case "INPUT":
				if in, ok := p.Inputs[fromId]; ok {
					in.DownStream = append(in.DownStream, &msgChan)
				}
			case "RULESET":
				if rs, ok := p.Rulesets[fromId]; ok {
					rs.DownStream[to] = &msgChan
				}
			}

			switch toType {
			case "RULESET":
				if rs, ok := p.Rulesets[toId]; ok {
					rs.UpStream[from] = &msgChan
				}
			case "OUTPUT":
				if out, ok := p.Outputs[toId]; ok {
					out.UpStream = append(out.UpStream, &msgChan)
				}
			}
		}
	}

	return nil
}

// parseContent parses the project content to build the data flow graph
func (p *Project) parseContent() (map[string][]string, error) {
	flowGraph := make(map[string][]string)
	lines := strings.Split(p.Config.Content, "\n")
	edgeSet := make(map[string]struct{}) // Used to detect duplicate flows

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Only support standard arrow format: ->
		parts := strings.Split(line, "->")

		if len(parts) != 2 {
			// Check for invalid arrow-like patterns and provide specific error messages
			if strings.Contains(line, "→") {
				return nil, fmt.Errorf("invalid arrow format at line %d: use '->' instead of '→' in %q", lineNum+1, line)
			} else if strings.Contains(line, "—>") {
				return nil, fmt.Errorf("invalid arrow format at line %d: use '->' instead of '—>' in %q", lineNum+1, line)
			} else if strings.Contains(line, "-->") {
				return nil, fmt.Errorf("invalid arrow format at line %d: use '->' instead of '-->' in %q", lineNum+1, line)
			} else if strings.Contains(line, "=>") {
				return nil, fmt.Errorf("invalid arrow format at line %d: use '->' instead of '=>' in %q", lineNum+1, line)
			} else if strings.Contains(line, "—") || strings.Contains(line, "–") || strings.Contains(line, "―") {
				return nil, fmt.Errorf("invalid arrow format at line %d: use '->' instead of dash characters in %q", lineNum+1, line)
			}
			return nil, fmt.Errorf("invalid line format at line %d: missing or invalid arrow operator in %q (use '->')", lineNum+1, line)
		}

		from := strings.TrimSpace(parts[0])
		to := strings.TrimSpace(parts[1])

		// Validate node types
		fromType, _ := parseNode(from)
		toType, _ := parseNode(to)

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

		// Check for duplicate flows
		edgeKey := from + "->" + to
		if _, exists := edgeSet[edgeKey]; exists {
			return nil, fmt.Errorf("duplicate data flow detected at line %d: %s", lineNum+1, edgeKey)
		}
		edgeSet[edgeKey] = struct{}{}

		// Add to flow graph
		flowGraph[from] = append(flowGraph[from], to)
	}

	// Check if all referenced components exist
	if err := p.validateComponentExistence(flowGraph); err != nil {
		return nil, err
	}

	return flowGraph, nil
}

// parseNode splits "TYPE.name" into ("TYPE", "name")
func parseNode(s string) (string, string) {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.ToUpper(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
}

// validateComponentExistence checks if all referenced components exist in the system
func (p *Project) validateComponentExistence(flowGraph map[string][]string) error {
	// Parse content again to get line numbers
	lines := strings.Split(p.Config.Content, "\n")

	// Build a map of component -> line numbers where they appear
	componentLineMap := make(map[string][]int)

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Find all components mentioned in this line
		parts := strings.Split(line, "->")
		if len(parts) == 2 {
			from := strings.TrimSpace(parts[0])
			to := strings.TrimSpace(parts[1])

			// Record line numbers for both components
			componentLineMap[from] = append(componentLineMap[from], lineNum+1)
			componentLineMap[to] = append(componentLineMap[to], lineNum+1)
		}
	}

	// Collect all input/output/ruleset names from flowGraph
	inputNames := make(map[string]bool)
	outputNames := make(map[string]bool)
	rulesetNames := make(map[string]bool)

	for from, tos := range flowGraph {
		fromParts := strings.Split(from, ".")
		if len(fromParts) == 2 {
			switch strings.ToUpper(fromParts[0]) {
			case "INPUT":
				inputNames[fromParts[1]] = true
			case "OUTPUT":
				outputNames[fromParts[1]] = true
			case "RULESET":
				rulesetNames[fromParts[1]] = true
			}
		}

		for _, to := range tos {
			toParts := strings.Split(to, ".")
			if len(toParts) == 2 {
				switch strings.ToUpper(toParts[0]) {
				case "INPUT":
					inputNames[toParts[1]] = true
				case "OUTPUT":
					outputNames[toParts[1]] = true
				case "RULESET":
					rulesetNames[toParts[1]] = true
				}
			}
		}
	}

	// Check if input components exist
	for inputName := range inputNames {
		if _, ok := GlobalProject.Inputs[inputName]; !ok {
			// Check if it's a temporary component, temporary components should not be referenced
			if _, tempExists := GlobalProject.InputsNew[inputName]; tempExists {
				// Find the line number where this component appears
				componentKey := "INPUT." + inputName
				if lineNumbers, exists := componentLineMap[componentKey]; exists && len(lineNumbers) > 0 {
					return fmt.Errorf("cannot reference temporary input component '%s' at line %d, please save it first", inputName, lineNumbers[0])
				}
				return fmt.Errorf("cannot reference temporary input component '%s', please save it first", inputName)
			}
			// Find the line number where this component appears
			componentKey := "INPUT." + inputName
			if lineNumbers, exists := componentLineMap[componentKey]; exists && len(lineNumbers) > 0 {
				return fmt.Errorf("input component '%s' not found at line %d", inputName, lineNumbers[0])
			}
			return fmt.Errorf("input component '%s' not found", inputName)
		}
	}

	// Check if output components exist
	for outputName := range outputNames {
		if _, ok := GlobalProject.Outputs[outputName]; !ok {
			// Check if it's a temporary component, temporary components should not be referenced
			if _, tempExists := GlobalProject.OutputsNew[outputName]; tempExists {
				// Find the line number where this component appears
				componentKey := "OUTPUT." + outputName
				if lineNumbers, exists := componentLineMap[componentKey]; exists && len(lineNumbers) > 0 {
					return fmt.Errorf("cannot reference temporary output component '%s' at line %d, please save it first", outputName, lineNumbers[0])
				}
				return fmt.Errorf("cannot reference temporary output component '%s', please save it first", outputName)
			}
			// Find the line number where this component appears
			componentKey := "OUTPUT." + outputName
			if lineNumbers, exists := componentLineMap[componentKey]; exists && len(lineNumbers) > 0 {
				return fmt.Errorf("output component '%s' not found at line %d", outputName, lineNumbers[0])
			}
			return fmt.Errorf("output component '%s' not found", outputName)
		}
	}

	// Check if ruleset components exist
	for rulesetName := range rulesetNames {
		if _, ok := GlobalProject.Rulesets[rulesetName]; !ok {
			// Check if it's a temporary component, temporary components should not be referenced
			if _, tempExists := GlobalProject.RulesetsNew[rulesetName]; tempExists {
				// Find the line number where this component appears
				componentKey := "RULESET." + rulesetName
				if lineNumbers, exists := componentLineMap[componentKey]; exists && len(lineNumbers) > 0 {
					return fmt.Errorf("cannot reference temporary ruleset component '%s' at line %d, please save it first", rulesetName, lineNumbers[0])
				}
				return fmt.Errorf("cannot reference temporary ruleset component '%s', please save it first", rulesetName)
			}
			// Find the line number where this component appears
			componentKey := "RULESET." + rulesetName
			if lineNumbers, exists := componentLineMap[componentKey]; exists && len(lineNumbers) > 0 {
				return fmt.Errorf("ruleset component '%s' not found at line %d", rulesetName, lineNumbers[0])
			}
			return fmt.Errorf("ruleset component '%s' not found", rulesetName)
		}
	}

	return nil
}

// Start starts the project and manages shared components safely
func (p *Project) Start() error {
	// Check if this is test mode (any output has TestCollectionChan set)
	isTestMode := false
	for _, out := range p.Outputs {
		if out.TestCollectionChan != nil {
			isTestMode = true
			break
		}
	}

	// In test mode, bypass status checks but in production mode, enforce them
	if !isTestMode {
		if p.Status == ProjectStatusRunning {
			return fmt.Errorf("project is already running %s", p.Id)
		}
		if p.Status == ProjectStatusStarting {
			return fmt.Errorf("project is currently starting, please wait %s", p.Id)
		}
		if p.Status == ProjectStatusStopping {
			return fmt.Errorf("project is currently stopping, please wait %s", p.Id)
		}
		if p.Status == ProjectStatusError {
			return fmt.Errorf("project is error %s %s", p.Id, p.Err.Error())
		}
	} else {
		// Force set status to stopped to allow starting in test mode
		p.Status = ProjectStatusStopped
		logger.Info("Starting project in test mode (bypassing status checks)", "id", p.Id)
	}

	// Set status to starting immediately to prevent duplicate operations
	if !isTestMode {
		now := time.Now()
		p.Status = ProjectStatusStarting
		p.StatusChangedAt = &now
		logger.Info("Project status set to starting", "id", p.Id)

		// No persistence for transient 'starting' state
	}

	// Initialize project control channels
	p.stopChan = make(chan struct{})

	// Parse project content to get component flow
	flowGraph, err := p.parseContent()
	if err != nil {
		now := time.Now()
		p.Status = ProjectStatusError
		p.StatusChangedAt = &now
		p.Err = err
		return fmt.Errorf("failed to parse project content: %v", err)
	}

	// Load components from global registry
	err = p.loadComponentsFromGlobal(flowGraph)
	if err != nil {
		now := time.Now()
		p.Status = ProjectStatusError
		p.StatusChangedAt = &now
		p.Err = err
		return fmt.Errorf("failed to load components: %v", err)
	}

	// Create fresh channel connections
	err = p.createChannelConnections(flowGraph)
	if err != nil {
		now := time.Now()
		p.Status = ProjectStatusError
		p.StatusChangedAt = &now
		p.Err = err
		return fmt.Errorf("failed to create channel connections: %v", err)
	}

	// Use centralized component usage counter for better performance and code maintainability

	// Start inputs first
	for _, in := range p.Inputs {
		if isTestMode {
			// In test mode, directly start all inputs
			logger.Info("Starting input in test mode", "project", p.Id, "input", in.Id)
			if err := in.Start(); err != nil {
				errorMsg := fmt.Errorf("failed to start input %s at %s: %v", in.Id, time.Now().Format("2006-01-02 15:04:05"), err)
				logger.Error("Input component startup failed", "project", p.Id, "input", in.Id, "error", err, "time", time.Now().Format("2006-01-02 15:04:05"))

				// Clean up already started components
				p.cleanupComponentsOnStartupFailure()

				now := time.Now()
				p.Status = ProjectStatusError
				p.StatusChangedAt = &now
				p.Err = errorMsg
				return errorMsg
			}
		} else {
			// In production mode, check component sharing
			runningCount := UsageCounter.CountProjectsUsingInput(in.Id, p.Id)
			if runningCount == 0 {
				// No other project is using this input - start it
				// Reduce log verbosity: only log first input start or errors
				if err := in.Start(); err != nil {
					errorMsg := fmt.Errorf("failed to start shared input %s at %s: %v", in.Id, time.Now().Format("2006-01-02 15:04:05"), err)
					logger.Error("Shared input component startup failed", "project", p.Id, "input", in.Id, "error", err, "time", time.Now().Format("2006-01-02 15:04:05"))

					// Clean up already started components
					p.cleanupComponentsOnStartupFailure()

					now := time.Now()
					p.Status = ProjectStatusError
					p.StatusChangedAt = &now
					p.Err = errorMsg
					return errorMsg
				}
			}
			// Remove verbose reuse log to reduce log volume
			// logger.Info("Reusing already running input component", "project", p.Id, "input", in.Id, "running_projects", runningCount)
		}
	}

	// Start rulesets after inputs
	for _, rs := range p.Rulesets {
		if isTestMode {
			// In test mode, directly start all rulesets
			logger.Info("Starting ruleset in test mode", "project", p.Id, "ruleset", rs.RulesetID)
			if err := rs.Start(); err != nil {
				errorMsg := fmt.Errorf("failed to start ruleset %s at %s: %v", rs.RulesetID, time.Now().Format("2006-01-02 15:04:05"), err)
				logger.Error("Ruleset component startup failed", "project", p.Id, "ruleset", rs.RulesetID, "error", err, "time", time.Now().Format("2006-01-02 15:04:05"))

				// Clean up already started components
				p.cleanupComponentsOnStartupFailure()

				now := time.Now()
				p.Status = ProjectStatusError
				p.StatusChangedAt = &now
				p.Err = errorMsg
				return errorMsg
			}
		} else {
			// In production mode, check component sharing
			runningCount := UsageCounter.CountProjectsUsingRulesetInstance(rs.RulesetID, rs.ProjectNodeSequence, p.Id)
			if runningCount == 0 {
				// No other project is using this ruleset instance - start it
				// Reduce log verbosity: only log errors
				if err := rs.Start(); err != nil {
					errorMsg := fmt.Errorf("failed to start ruleset %s at %s: %v", rs.RulesetID, time.Now().Format("2006-01-02 15:04:05"), err)
					logger.Error("Ruleset instance startup failed", "project", p.Id, "ruleset", rs.RulesetID, "error", err, "time", time.Now().Format("2006-01-02 15:04:05"))

					// Clean up already started components
					p.cleanupComponentsOnStartupFailure()

					now := time.Now()
					p.Status = ProjectStatusError
					p.StatusChangedAt = &now
					p.Err = errorMsg
					return errorMsg
				}
			}
		}
	}

	// Start outputs last (will automatically use test mode if TestCollectionChan is set)
	for _, out := range p.Outputs {
		if isTestMode {
			// In test mode, directly start all outputs (they will automatically detect test mode)
			logger.Info("Starting output in test mode", "project", p.Id, "output", out.Id)
			if err := out.Start(); err != nil {
				errorMsg := fmt.Errorf("failed to start output %s at %s: %v", out.Id, time.Now().Format("2006-01-02 15:04:05"), err)
				logger.Error("Output component startup failed", "project", p.Id, "output", out.Id, "error", err, "time", time.Now().Format("2006-01-02 15:04:05"))

				// Clean up already started components
				p.cleanupComponentsOnStartupFailure()

				now := time.Now()
				p.Status = ProjectStatusError
				p.StatusChangedAt = &now
				p.Err = errorMsg
				return errorMsg
			}
		} else {
			// In production mode, check component sharing
			runningCount := UsageCounter.CountProjectsUsingOutputInstance(out.Id, out.ProjectNodeSequence, p.Id)
			if runningCount == 0 {
				// No other project is using this output instance - start it
				// Reduce log verbosity: only log errors
				if err := out.Start(); err != nil {
					errorMsg := fmt.Errorf("failed to start output %s at %s: %v", out.Id, time.Now().Format("2006-01-02 15:04:05"), err)
					logger.Error("Output instance startup failed", "project", p.Id, "output", out.Id, "error", err, "time", time.Now().Format("2006-01-02 15:04:05"))

					// Clean up already started components
					p.cleanupComponentsOnStartupFailure()

					now := time.Now()
					p.Status = ProjectStatusError
					p.StatusChangedAt = &now
					p.Err = errorMsg
					return errorMsg
				}
			}
			// Remove verbose reuse log to reduce log volume
			// logger.Info("Reusing already running output instance", "project", p.Id, "output", out.Id, "sequence", out.ProjectNodeSequence, "running_projects", runningCount)
		}
	}

	// Start metrics collection
	p.metricsStop = make(chan struct{})
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.collectMetrics()
	}()

	now := time.Now()
	p.Status = ProjectStatusRunning
	p.StatusChangedAt = &now

	// Save the running status to file (skip in test mode)
	if !isTestMode {
		err = p.SaveProjectStatus()
		if err != nil {
			logger.Warn("Failed to save project status", "id", p.Id, "error", err)
		}
	}

	if isTestMode {
		logger.Info("Project started successfully in test mode", "project", p.Id)
	} else {
		logger.Info("Project started successfully with shared components", "project", p.Id)
	}

	// After the project is successfully started, recalculate dependencies synchronously
	AnalyzeProjectDependencies()

	return nil
}

// cleanupComponentsOnStartupFailure cleans up components when project startup fails
// This does NOT change the project status - that should be handled by the caller
func (p *Project) cleanupComponentsOnStartupFailure() {
	// Stop outputs that were started
	for _, out := range p.Outputs {
		// Check if this output instance is used by other projects
		otherProjectsUsing := UsageCounter.CountProjectsUsingOutputInstance(out.Id, out.ProjectNodeSequence, p.Id)
		if otherProjectsUsing == 0 {
			logger.Info("Stopping output instance during startup failure cleanup", "project", p.Id, "output", out.Id)
			_ = out.Stop()
		}
	}

	// Stop rulesets that were started
	for _, rs := range p.Rulesets {
		// Check if this ruleset instance is used by other projects
		otherProjectsUsing := UsageCounter.CountProjectsUsingRulesetInstance(rs.RulesetID, rs.ProjectNodeSequence, p.Id)
		if otherProjectsUsing == 0 {
			logger.Info("Stopping ruleset instance during startup failure cleanup", "project", p.Id, "ruleset", rs.RulesetID)
			_ = rs.Stop()
		}
	}

	// Stop inputs that were started
	for _, in := range p.Inputs {
		// Check if this input is used by other projects
		otherProjectsUsing := UsageCounter.CountProjectsUsingInput(in.Id, p.Id)
		if otherProjectsUsing == 0 {
			_ = in.Stop()
		}
	}

	// Clean up channels
	for _, channelId := range p.MsgChannels {
		newCnt := decrementChannelRef(channelId)
		if newCnt == 0 {
			GlobalProject.EdgeMapMu.RLock()
			ch, exists := GlobalProject.msgChans[channelId]
			GlobalProject.EdgeMapMu.RUnlock()
			if exists {
				closedCh := ch
				// Remove from maps under write lock
				GlobalProject.EdgeMapMu.Lock()
				delete(GlobalProject.msgChans, channelId)
				delete(GlobalProject.msgChansCounter, channelId)
				GlobalProject.EdgeMapMu.Unlock()

				removeEdgeChanId(channelId, closedCh)
				close(closedCh)
			}
		}
	}

	// Clear component references
	p.Inputs = make(map[string]*input.Input)
	p.Outputs = make(map[string]*output.Output)
	p.Rulesets = make(map[string]*rules_engine.Ruleset)
	p.MsgChannels = []string{}

	// Reduce log verbosity: only log critical cleanup completion
	// logger.Info("Finished cleaning up components due to startup failure", "project", p.Id)
}

// stopComponents is an internal function that performs the actual component stopping
// This is used by the public Stop() method and sets the status to stopped
func (p *Project) stopComponents() error {
	logger.Info("Stopping project components", "project", p.Id)

	// Use centralized component usage counter for better performance and code maintainability

	// Step 1: Stop inputs first to prevent new data (only if not used by other projects)
	// This is critical for fast shutdown - we want to stop data sources immediately
	logger.Info("Step 1: Rapidly stopping inputs to prevent new data", "project", p.Id, "count", len(p.Inputs))
	for _, in := range p.Inputs {
		otherProjectsUsing := UsageCounter.CountProjectsUsingInput(in.Id, p.Id)
		if otherProjectsUsing == 0 {
			logger.Info("Rapidly stopping input component", "project", p.Id, "input", in.Id, "other_projects_using", otherProjectsUsing)
			startTime := time.Now()
			if err := in.Stop(); err != nil {
				logger.Error("Failed to stop input", "project", p.Id, "input", in.Id, "error", err)
				// Continue with other inputs instead of failing immediately
			} else {
				logger.Info("Rapidly stopped input", "project", p.Id, "input", in.Id, "duration", time.Since(startTime))
			}
		} else {
			logger.Info("Input component still used by other projects, skipping stop", "project", p.Id, "input", in.Id, "other_projects_using", otherProjectsUsing)
		}
	}

	// Step 2: Wait for data to drain through the pipeline
	logger.Info("Step 2: Waiting for data to drain through pipeline", "project", p.Id)
	p.waitForDataDrain()

	// Step 3: Stop rulesets (only if not used by other projects)
	logger.Info("Step 3: Stopping rulesets", "project", p.Id, "count", len(p.Rulesets))
	for _, rs := range p.Rulesets {
		otherProjectsUsing := UsageCounter.CountProjectsUsingRulesetInstance(rs.RulesetID, rs.ProjectNodeSequence, p.Id)
		if otherProjectsUsing == 0 {
			logger.Info("Stopping ruleset instance", "project", p.Id, "ruleset", rs.RulesetID, "sequence", rs.ProjectNodeSequence, "other_projects_using", otherProjectsUsing)
			startTime := time.Now()
			if err := rs.Stop(); err != nil {
				logger.Error("Failed to stop ruleset", "project", p.Id, "ruleset", rs.RulesetID, "error", err)
			} else {
				logger.Info("Stopped ruleset", "project", p.Id, "ruleset", rs.RulesetID, "duration", time.Since(startTime))
			}
		} else {
			logger.Info("Ruleset instance still used by other projects, skipping stop", "project", p.Id, "ruleset", rs.RulesetID, "sequence", rs.ProjectNodeSequence, "other_projects_using", otherProjectsUsing)
		}
	}

	// Step 4: Stop outputs last (only if not used by other projects)
	logger.Info("Step 4: Stopping outputs", "project", p.Id, "count", len(p.Outputs))
	for _, out := range p.Outputs {
		otherProjectsUsing := UsageCounter.CountProjectsUsingOutputInstance(out.Id, out.ProjectNodeSequence, p.Id)
		if otherProjectsUsing == 0 {
			logger.Info("Stopping output instance", "project", p.Id, "output", out.Id, "sequence", out.ProjectNodeSequence, "other_projects_using", otherProjectsUsing)
			startTime := time.Now()
			if err := out.Stop(); err != nil {
				logger.Error("Failed to stop output", "project", p.Id, "output", out.Id, "error", err)
				// Continue with other outputs instead of failing immediately
			} else {
				logger.Info("Stopped output", "project", p.Id, "output", out.Id, "duration", time.Since(startTime))
			}
		} else {
			logger.Info("Output instance still used by other projects, skipping stop", "project", p.Id, "output", out.Id, "sequence", out.ProjectNodeSequence, "other_projects_using", otherProjectsUsing)
		}
	}

	// Step 5: Stop metrics collection
	if p.metricsStop != nil {
		close(p.metricsStop)
		p.metricsStop = nil
	}

	// Step 6: Wait for all project goroutines to finish
	waitDone := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		logger.Info("All project goroutines finished", "project", p.Id)
	case <-time.After(30 * time.Second):
		logger.Warn("Timeout waiting for project goroutines to finish", "project", p.Id)
	}

	// Step 7: Clean up channels
	logger.Info("Step 7: Cleaning up channels", "project", p.Id, "channel_count", len(p.MsgChannels))
	for _, channelId := range p.MsgChannels {
		newCnt := decrementChannelRef(channelId)
		if newCnt == 0 {
			GlobalProject.EdgeMapMu.RLock()
			ch, exists := GlobalProject.msgChans[channelId]
			GlobalProject.EdgeMapMu.RUnlock()
			if exists {
				closedCh := ch
				// Remove from maps under write lock
				GlobalProject.EdgeMapMu.Lock()
				delete(GlobalProject.msgChans, channelId)
				delete(GlobalProject.msgChansCounter, channelId)
				GlobalProject.EdgeMapMu.Unlock()

				removeEdgeChanId(channelId, closedCh)
				close(closedCh)
			}
		}
	}
	p.MsgChannels = []string{}

	// Step 8: Clear component references
	logger.Info("Step 8: Clearing component references", "project", p.Id)
	p.Inputs = make(map[string]*input.Input)
	p.Outputs = make(map[string]*output.Output)
	p.Rulesets = make(map[string]*rules_engine.Ruleset)

	// Step 9: Close project channels after all goroutines are done
	if p.stopChan != nil {
		close(p.stopChan)
		p.stopChan = nil
	}

	// Set status to stopped and save (only if not shutdown stop)
	now := time.Now()
	p.Status = ProjectStatusStopped
	p.StatusChangedAt = &now

	// Only save status if this is not a shutdown stop (preserve user intention)
	if !p.isShutdownStop {
		err := p.SaveProjectStatus()
		if err != nil {
			logger.Warn("Failed to save project status", "id", p.Id, "error", err)
		}
	} else {
		logger.Info("Skipping status save during shutdown to preserve user intention", "project", p.Id)
	}

	logger.Info("Finished stopping project components", "project", p.Id)
	return nil
}

// SetShutdownMode sets the shutdown flag to control whether status should be saved
func (p *Project) SetShutdownMode(isShutdown bool) {
	p.isShutdownStop = isShutdown
}

// Stop stops the project and all its components in proper order
func (p *Project) Stop() error {
	if p.Status != ProjectStatusRunning && p.Status != ProjectStatusStarting {
		if p.Status == ProjectStatusStopping {
			return fmt.Errorf("project is already stopping %s", p.Id)
		}
		return fmt.Errorf("project is not running %s", p.Id)
	}

	// Set status to stopping immediately to prevent duplicate operations
	now := time.Now()
	p.Status = ProjectStatusStopping
	p.StatusChangedAt = &now
	logger.Info("Project status set to stopping", "id", p.Id)

	// No persistence for transient 'stopping' state

	// Check if project is in error state
	if p.Err != nil {
		logger.Warn("Stopping project with errors", "id", p.Id, "error", p.Err)
	}

	// Overall timeout for the entire stop process
	overallTimeout := time.After(2 * time.Minute) // 2 minute overall timeout
	stopCompleted := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic during project stop", "project", p.Id, "panic", r)
				stopCompleted <- fmt.Errorf("panic during stop: %v", r)
			}
		}()

		// Use the internal stopComponents function
		err := p.stopComponents()
		stopCompleted <- err
	}()

	select {
	case err := <-stopCompleted:
		if err != nil {
			logger.Error("Project stop completed with error", "project", p.Id, "error", err)
			return err
		}
		logger.Info("Project stopped successfully", "project", p.Id)
		return nil
	case <-overallTimeout:
		logger.Error("Project stop timeout exceeded", "project", p.Id)
		now := time.Now()
		p.Status = ProjectStatusError
		p.StatusChangedAt = &now

		// Skip persisting error status

		return fmt.Errorf("project stop timeout exceeded for %s", p.Id)
	}
}

// collectMetrics collects runtime metrics
func (p *Project) collectMetrics() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.metricsStop:
			return
		case <-ticker.C:
			p.metrics.mu.Lock()
			// Update input metrics
			for id, in := range p.Inputs {
				p.metrics.InputQPS[id] = in.GetConsumeQPS()
			}

			// Update output metrics
			for id, out := range p.Outputs {
				p.metrics.OutputQPS[id] = out.GetProduceQPS()
			}

			p.metrics.mu.Unlock()
		}
	}
}

// GetMetrics returns the current project metrics
func (p *Project) GetMetrics() *ProjectMetrics {
	p.metrics.mu.RLock()
	defer p.metrics.mu.RUnlock()
	return p.metrics
}

func AnalyzeProjectDependencies() {
	// Use dedicated project lock to prevent race conditions
	GlobalProject.ProjectMu.RLock()

	// Create a local copy of projects to avoid holding the lock during analysis
	projects := make(map[string]*Project)
	for id, p := range GlobalProject.Projects {
		projects[id] = p
	}

	// Release the lock early since we have a local copy
	GlobalProject.ProjectMu.RUnlock()

	// Clear all project dependencies
	for _, p := range projects {
		p.DependsOn = []string{}
		p.DependedBy = []string{}
		p.SharedInputs = []string{}
		p.SharedOutputs = []string{}
		p.SharedRulesets = []string{}
	}

	// Build component instance usage mapping - now distinguished by ProjectNodeSequence
	instanceUsage := make(map[string][]string) // ProjectNodeSequence -> list of project IDs using it

	// Analyze component instances used by each project
	for projectID, p := range projects {
		// Record input component instance usage
		for _, i := range p.Inputs {
			sequence := i.ProjectNodeSequence
			if sequence != "" {
				instanceUsage[sequence] = append(instanceUsage[sequence], projectID)
			}
		}

		// Record output component instance usage
		for _, o := range p.Outputs {
			sequence := o.ProjectNodeSequence
			if sequence != "" {
				instanceUsage[sequence] = append(instanceUsage[sequence], projectID)
			}
		}

		// Record ruleset instance usage
		for _, r := range p.Rulesets {
			sequence := r.ProjectNodeSequence
			if sequence != "" {
				instanceUsage[sequence] = append(instanceUsage[sequence], projectID)
			}
		}
	}

	// Update real shared component information (only components with the same ProjectNodeSequence are shared)
	for sequence, projectList := range instanceUsage {
		if len(projectList) > 1 {
			// This is a truly shared component instance
			parts := strings.Split(sequence, ".")
			if len(parts) >= 2 {
				componentType := strings.ToLower(parts[len(parts)-2])
				componentID := parts[len(parts)-1]

				for _, projectID := range projectList {
					if p, exists := projects[projectID]; exists {
						switch componentType {
						case "input":
							p.SharedInputs = append(p.SharedInputs, componentID)
						case "output":
							p.SharedOutputs = append(p.SharedOutputs, componentID)
						case "ruleset":
							p.SharedRulesets = append(p.SharedRulesets, componentID)
						}
					}
				}
			}
		}
	}

	// Analyze dependencies between projects
	for projectID, p := range projects {
		// Parse project configuration to get data flow with error handling
		flowGraph, err := p.parseContent()
		if err != nil {
			logger.Error("Failed to parse project content", "id", projectID, "error", err)
			continue
		}

		// Analyze inter-project dependencies in data flow
		for fromNode, toNodes := range flowGraph {
			fromType, fromID := parseNode(fromNode)

			// Check if there are cross-project dependencies
			for _, toNode := range toNodes {
				toType, toID := parseNode(toNode)

				// If source node is output of one project and target node is input of another project, there is inter-project dependency
				if fromType == "OUTPUT" && toType == "INPUT" {
					// Find projects that own these components
					var fromProjectID, toProjectID string

					// Find project that owns the source output
					for pid, proj := range projects {
						if _, exists := proj.Outputs[fromID]; exists {
							fromProjectID = pid
							break
						}
					}

					// Find project that owns the target input
					for pid, proj := range projects {
						if _, exists := proj.Inputs[toID]; exists {
							toProjectID = pid
							break
						}
					}

					// If two different projects are found, there is inter-project dependency
					if fromProjectID != "" && toProjectID != "" && fromProjectID != toProjectID {
						// Update dependency relationship
						if toProj, exists := projects[toProjectID]; exists {
							toProj.DependsOn = append(toProj.DependsOn, fromProjectID)
						}
						if fromProj, exists := projects[fromProjectID]; exists {
							fromProj.DependedBy = append(fromProj.DependedBy, toProjectID)
						}
					}
				}
			}
		}
	}

	// Record dependency relationship information
	for projectID, p := range projects {
		if len(p.DependsOn) > 0 || len(p.DependedBy) > 0 ||
			len(p.SharedInputs) > 0 || len(p.SharedOutputs) > 0 || len(p.SharedRulesets) > 0 {
			logger.Info("Project dependencies analyzed",
				"id", projectID,
				"depends_on", p.DependsOn,
				"depended_by", p.DependedBy,
				"shared_inputs", p.SharedInputs,
				"shared_outputs", p.SharedOutputs,
				"shared_rulesets", p.SharedRulesets,
			)
		}
	}
}

// GetAffectedProjects returns the list of project IDs affected by component changes
func GetAffectedProjects(componentType string, componentID string) []string {
	affectedProjects := make(map[string]struct{})

	switch componentType {
	case "input":
		// Find all projects using this input
		for projectID, p := range GlobalProject.Projects {
			if _, exists := p.Inputs[componentID]; exists {
				affectedProjects[projectID] = struct{}{}
			}
		}
	case "output":
		// Find all projects using this output
		for projectID, p := range GlobalProject.Projects {
			if _, exists := p.Outputs[componentID]; exists {
				affectedProjects[projectID] = struct{}{}
			}
		}
	case "ruleset":
		// Find all projects using this ruleset
		for projectID, p := range GlobalProject.Projects {
			if _, exists := p.Rulesets[componentID]; exists {
				affectedProjects[projectID] = struct{}{}
			}
		}
	case "project":
		// The project itself is affected
		affectedProjects[componentID] = struct{}{}

		// Find other projects that depend on this project
		if p, exists := GlobalProject.Projects[componentID]; exists {
			for _, depID := range p.DependedBy {
				affectedProjects[depID] = struct{}{}
			}
		}
	}

	// Convert to string slice
	result := make([]string, 0, len(affectedProjects))
	for projectID := range affectedProjects {
		result = append(result, projectID)
	}

	return result
}

// SaveProjectStatus saves the current status of a project to a file
// This method saves the "user intention" status, not the actual runtime status
func (p *Project) SaveProjectStatus() error {
	// Only persist durable states (running, stopped). Skip starting, stopping, error.
	if p.Status != ProjectStatusRunning && p.Status != ProjectStatusStopped {
		return nil
	}
	// Skip test projects (IDs starting with "test_")
	if strings.HasPrefix(p.Id, "test_") {
		return nil
	}
	statusFile := common.GetConfigPath(".project_status")

	// Read existing statuses
	projectStatuses := make(map[string]string)

	// Check if the file exists
	if _, err := os.Stat(statusFile); os.IsNotExist(err) {
		// Create the file if it doesn't exist
		f, err := os.Create(statusFile)
		if err != nil {
			return fmt.Errorf("failed to create status file: %w", err)
		}
		_ = f.Close()
	} else {
		// Read the status file if it exists
		data, err := os.ReadFile(statusFile)
		if err == nil {
			// Parse the content
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}

				parts := strings.Split(line, ":")
				if len(parts) == 2 {
					projectStatuses[parts[0]] = parts[1]
				}
			}
		}
	}

	// Update the status for this project
	projectStatuses[p.Id] = string(p.Status)

	// Create or open the status file
	f, err := os.OpenFile(statusFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open status file: %w", err)
	}
	defer f.Close()

	// Write all project statuses to the file
	for id, status := range projectStatuses {
		_, err = fmt.Fprintf(f, "%s:%s\n", id, status)
		if err != nil {
			return fmt.Errorf("failed to write project status: %w", err)
		}
	}

	// Write to Redis for cluster-wide visibility
	updateProjectStatusRedis(common.Config.LocalIP, p.Id, p.Status)

	return nil
}

// LoadProjectStatus loads the project status from a file
func (p *Project) LoadProjectStatus() (ProjectStatus, error) {
	statusFile := common.GetConfigPath(".project_status")

	// Check if the file exists
	if _, err := os.Stat(statusFile); os.IsNotExist(err) {
		// File doesn't exist, create an empty one
		f, err := os.Create(statusFile)
		if err != nil {
			return ProjectStatusStopped, fmt.Errorf("failed to create status file: %w", err)
		}
		_ = f.Close()
		return ProjectStatusStopped, nil
	}

	// Read the status file
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return ProjectStatusStopped, fmt.Errorf("failed to read status file: %w", err)
	}

	// Parse the content
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		projectID := parts[0]
		status := parts[1]

		// If this is the project we're looking for
		if projectID == p.Id {
			return ProjectStatus(status), nil
		}
	}

	// Project not found in the status file
	return ProjectStatusStopped, nil
}

// StopForTesting stops the project quickly for testing purposes and ensures complete cleanup
func (p *Project) StopForTesting() error {
	logger.Info("Stopping and destroying test project", "project", p.Id)

	// Stop metrics collection first
	if p.metricsStop != nil {
		close(p.metricsStop)
		p.metricsStop = nil
	}

	// Stop components quickly without waiting for channel drainage
	// Note: Test components are completely isolated, so stopping them won't affect production
	for _, in := range p.Inputs {
		// Test inputs are virtual, just clear their downstream connections
		in.DownStream = []*chan map[string]interface{}{}
		logger.Debug("Cleared test input", "project", p.Id, "input", in.Id)
	}

	for _, rs := range p.Rulesets {
		// Use the quick stop method for rulesets in testing
		if err := rs.StopForTesting(); err != nil {
			logger.Warn("Failed to stop test ruleset", "project", p.Id, "ruleset", rs.RulesetID, "error", err)
		}
		logger.Debug("Stopped test ruleset", "project", p.Id, "ruleset", rs.RulesetID)
	}

	for _, out := range p.Outputs {
		// Use the quick stop method for outputs in testing
		if err := out.StopForTesting(); err != nil {
			logger.Warn("Failed to stop test output", "project", p.Id, "output", out.Id, "error", err)
		}
		logger.Debug("Stopped test output", "project", p.Id, "output", out.Id)
	}

	// Wait for any remaining goroutines to finish with short timeout
	waitDone := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		logger.Debug("All test project goroutines finished", "project", p.Id)
	case <-time.After(2 * time.Second): // Very short timeout for test cleanup
		logger.Warn("Timeout waiting for test project goroutines, proceeding with cleanup", "project", p.Id)
	}

	// Complete cleanup: destroy all test instances to prevent any memory leaks
	p.destroyTestInstances()

	p.Status = ProjectStatusStopped
	logger.Info("Test project completely destroyed", "project", p.Id)
	return nil
}

// destroyTestInstances completely destroys all test component instances
func (p *Project) destroyTestInstances() {
	logger.Debug("Destroying test instances", "project", p.Id)

	// Close and clear all channels first
	for _, in := range p.Inputs {
		// Close all downstream channels safely
		for _, ch := range in.DownStream {
			if ch != nil {
				// Close channel safely by checking if it's already closed
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Channel already closed, ignore the panic
						}
					}()
					close(*ch)
				}()
			}
		}
		in.DownStream = nil
	}

	for _, rs := range p.Rulesets {
		// Clear upstream and downstream connections safely
		for _, ch := range rs.UpStream {
			if ch != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Channel already closed, ignore the panic
						}
					}()
					close(*ch)
				}()
			}
		}
		for _, ch := range rs.DownStream {
			if ch != nil {
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Channel already closed, ignore the panic
						}
					}()
					close(*ch)
				}()
			}
		}
		rs.UpStream = nil
		rs.DownStream = nil
	}

	for _, out := range p.Outputs {
		// Clear upstream connections and test collection channel
		out.UpStream = nil
		out.TestCollectionChan = nil
	}

	// Clear all component references to allow garbage collection
	p.Inputs = make(map[string]*input.Input)
	p.Outputs = make(map[string]*output.Output)
	p.Rulesets = make(map[string]*rules_engine.Ruleset)
	p.MsgChannels = []string{}

	// Clear project channels safely
	if p.stopChan != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Channel already closed, ignore the panic
				}
			}()
			close(p.stopChan)
		}()
		p.stopChan = nil
	}

	logger.Debug("Test instances destroyed", "project", p.Id)
}

// ParseContentForVisualization parses the project content for visualization purposes
// This is a public wrapper around parseContent for use in API visualization
func (p *Project) ParseContentForVisualization() (map[string][]string, error) {
	return p.parseContent()
}

// waitForDataDrain waits for data to drain through the pipeline after inputs are stopped
func (p *Project) waitForDataDrain() {
	logger.Info("Waiting for data to drain through pipeline", "project", p.Id)

	drainTimeout := time.After(30 * time.Second) // 30 second timeout for data drain
	checkInterval := 100 * time.Millisecond
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-drainTimeout:
			logger.Warn("Data drain timeout reached, proceeding with shutdown", "project", p.Id)
			return
		case <-ticker.C:
			allEmpty := true
			totalMessages := 0

			// Check all channels for remaining messages
			for _, channelId := range p.MsgChannels {
				if ch, exists := GlobalProject.msgChans[channelId]; exists {
					chLen := len(ch)
					if chLen > 0 {
						allEmpty = false
						totalMessages += chLen
					}
				}
			}

			if allEmpty {
				logger.Info("All channels drained, proceeding with shutdown", "project", p.Id)
				return
			}

			// Log progress every 5 seconds
			if time.Now().UnixNano()%(5*int64(time.Second)) < int64(checkInterval) {
				logger.Info("Waiting for channels to drain", "project", p.Id, "remaining_messages", totalMessages)
			}
		}
	}
}

// loadComponentsFromGlobal loads component references from global registry based on flow graph
func (p *Project) loadComponentsFromGlobal(flowGraph map[string][]string) error {
	logger.Info("Loading components from global registry", "project", p.Id)

	// Build ProjectNodeSequence mapping first
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

	// Collect component names
	inputNames := []string{}
	outputNames := []string{}
	rulesetNames := []string{}

	nameExists := func(list []string, name string) bool {
		for _, n := range list {
			if n == name {
				return true
			}
		}
		return false
	}

	for from, tos := range flowGraph {
		fromParts := strings.Split(from, ".")
		if len(fromParts) == 2 {
			switch strings.ToUpper(fromParts[0]) {
			case "INPUT":
				if !nameExists(inputNames, fromParts[1]) {
					inputNames = append(inputNames, fromParts[1])
				}
			case "OUTPUT":
				if !nameExists(outputNames, fromParts[1]) {
					outputNames = append(outputNames, fromParts[1])
				}
			case "RULESET":
				if !nameExists(rulesetNames, fromParts[1]) {
					rulesetNames = append(rulesetNames, fromParts[1])
				}
			}
		}

		for _, to := range tos {
			toParts := strings.Split(to, ".")
			if len(toParts) == 2 {
				switch strings.ToUpper(toParts[0]) {
				case "INPUT":
					if !nameExists(inputNames, toParts[1]) {
						inputNames = append(inputNames, toParts[1])
					}
				case "OUTPUT":
					if !nameExists(outputNames, toParts[1]) {
						outputNames = append(outputNames, toParts[1])
					}
				case "RULESET":
					if !nameExists(rulesetNames, toParts[1]) {
						rulesetNames = append(rulesetNames, toParts[1])
					}
				}
			}
		}
	}

	// Clear existing component references
	p.Inputs = make(map[string]*input.Input)
	p.Outputs = make(map[string]*output.Output)
	p.Rulesets = make(map[string]*rules_engine.Ruleset)

	// Load input components from global registry (inputs can be shared safely)
	for _, name := range inputNames {
		componentKey := "INPUT." + name
		expectedSequence := componentSequences[componentKey]
		if expectedSequence == "" {
			expectedSequence = componentKey
		}

		if globalInput, ok := GlobalProject.Inputs[name]; ok {
			// Set ProjectNodeSequence if not already set or differs from expected
			if globalInput.ProjectNodeSequence == "" {
				globalInput.ProjectNodeSequence = expectedSequence
			}

			p.Inputs[name] = globalInput
			// Ensure owner project list includes current project ID
			if globalInput.OwnerProjects == nil {
				globalInput.OwnerProjects = []string{p.Id}
			} else {
				found := false
				for _, pid := range globalInput.OwnerProjects {
					if pid == p.Id {
						found = true
						break
					}
				}
				if !found {
					globalInput.OwnerProjects = append(globalInput.OwnerProjects, p.Id)
				}
			}
		} else {
			return fmt.Errorf("input component %s not found in global registry", name)
		}
	}

	// Load output components with proper instance management
	for _, name := range outputNames {
		componentKey := "OUTPUT." + name
		expectedSequence := componentSequences[componentKey]
		if expectedSequence == "" {
			expectedSequence = componentKey
		}

		// Check if there's already an output instance with this exact ProjectNodeSequence
		var foundOutput *output.Output
		for _, existingProject := range GlobalProject.Projects {
			if existingOutput, exists := existingProject.Outputs[name]; exists {
				if existingOutput.ProjectNodeSequence == expectedSequence {
					foundOutput = existingOutput
					break
				}
			}
		}

		if foundOutput != nil {
			// Found existing instance with same ProjectNodeSequence, can share
			p.Outputs[name] = foundOutput
			logger.Info("Reusing existing output instance", "project", p.Id, "output", name, "sequence", expectedSequence)
			// add owner
			if foundOutput.OwnerProjects == nil {
				foundOutput.OwnerProjects = []string{p.Id}
			} else {
				found := false
				for _, pid := range foundOutput.OwnerProjects {
					if pid == p.Id {
						found = true
						break
					}
				}
				if !found {
					foundOutput.OwnerProjects = append(foundOutput.OwnerProjects, p.Id)
				}
			}
		} else {
			// Need to create a new instance from global template
			if globalOutput, ok := GlobalProject.Outputs[name]; ok {
				// Create a copy of the global output for this project's specific sequence
				newOutput, err := output.NewFromExisting(globalOutput, expectedSequence)
				if err != nil {
					return fmt.Errorf("failed to create output instance for %s: %v", name, err)
				}
				newOutput.OwnerProjects = []string{p.Id}
				p.Outputs[name] = newOutput
				logger.Info("Created new output instance", "project", p.Id, "output", name, "sequence", expectedSequence)
			} else {
				return fmt.Errorf("output component %s not found in global registry", name)
			}
		}
	}

	// Load ruleset components with proper instance management
	for _, name := range rulesetNames {
		componentKey := "RULESET." + name
		expectedSequence := componentSequences[componentKey]
		if expectedSequence == "" {
			expectedSequence = componentKey
		}

		// Check if there's already a ruleset instance with this exact ProjectNodeSequence
		var foundRuleset *rules_engine.Ruleset
		for _, existingProject := range GlobalProject.Projects {
			if existingRuleset, exists := existingProject.Rulesets[name]; exists {
				if existingRuleset.ProjectNodeSequence == expectedSequence {
					foundRuleset = existingRuleset
					break
				}
			}
		}

		if foundRuleset != nil {
			// Found existing instance with same ProjectNodeSequence, can share
			p.Rulesets[name] = foundRuleset
			logger.Info("Reusing existing ruleset instance", "project", p.Id, "ruleset", name, "sequence", expectedSequence)
			if foundRuleset.OwnerProjects == nil {
				foundRuleset.OwnerProjects = []string{p.Id}
			} else {
				f := false
				for _, pid := range foundRuleset.OwnerProjects {
					if pid == p.Id {
						f = true
						break
					}
				}
				if !f {
					foundRuleset.OwnerProjects = append(foundRuleset.OwnerProjects, p.Id)
				}
			}
		} else {
			// Need to create a new instance from global template
			if globalRuleset, ok := GlobalProject.Rulesets[name]; ok {
				// Create a copy of the global ruleset for this project's specific sequence
				newRuleset, err := rules_engine.NewFromExisting(globalRuleset, expectedSequence)
				if err != nil {
					return fmt.Errorf("failed to create ruleset instance for %s: %v", name, err)
				}
				newRuleset.OwnerProjects = []string{p.Id}
				p.Rulesets[name] = newRuleset
				logger.Info("Created new ruleset instance", "project", p.Id, "ruleset", name, "sequence", expectedSequence)
			} else {
				return fmt.Errorf("ruleset component %s not found in global registry", name)
			}
		}
	}

	logger.Info("Components loaded from global registry", "project", p.Id, "inputs", len(inputNames), "outputs", len(outputNames), "rulesets", len(rulesetNames))
	return nil
}

// createChannelConnections creates fresh channel connections between components
func (p *Project) createChannelConnections(flowGraph map[string][]string) error {
	logger.Info("Creating channel connections", "project", p.Id)

	// Helper to check if a slice already contains a channel (compare by underlying channel value)
	containsChan := func(list []*chan map[string]interface{}, ch chan map[string]interface{}) bool {
		for _, p := range list {
			if p != nil && *p == ch {
				return true
			}
		}
		return false
	}

	// Helper to check if slice of strings contains a value
	containsStr := func(list []string, target string) bool {
		for _, s := range list {
			if s == target {
				return true
			}
		}
		return false
	}

	for from, tos := range flowGraph {
		fromParts := strings.Split(from, ".")
		fromType := fromParts[0]
		fromId := fromParts[1]

		for _, to := range tos {
			toParts := strings.Split(to, ".")
			toType := toParts[0]
			toId := toParts[1]

			edgeKey := fmt.Sprintf("%s->%s", from, to)
			var msgChan chan map[string]interface{}
			var channelId string

			// Use single lock to avoid deadlock - check and create in one operation
			GlobalProject.EdgeMapMu.Lock()

			var ptr *chan map[string]interface{}

			// Check if channel already exists
			if cid, exists := GlobalProject.edgeChanIds[edgeKey]; exists {
				channelId = cid
				msgChan = GlobalProject.msgChans[channelId]
				ptr = &msgChan
				if cntPtr, ok := GlobalProject.msgChansCounter[channelId]; ok {
					cntPtr.Add(1)
				}
				logger.Debug("Reusing existing channel connection", "project", p.Id, "edge", edgeKey, "channel", channelId)
			} else {
				// Create new channel
				channelId = fmt.Sprintf("%s_%s_%s_%s", p.Id, from, to, time.Now().Format("20060102150405"))
				msgChan = make(chan map[string]interface{}, 1024)
				GlobalProject.msgChans[channelId] = msgChan
				ptr = &msgChan
				cnt := &atomic.Int64{}
				cnt.Store(1)
				GlobalProject.msgChansCounter[channelId] = cnt
				GlobalProject.edgeChanIds[edgeKey] = channelId
				logger.Info("Created new channel connection", "project", p.Id, "from", from, "to", to, "channel", channelId)
			}

			GlobalProject.EdgeMapMu.Unlock()

			// Record that this project uses this channelId (avoid duplicates)
			if !containsStr(p.MsgChannels, channelId) {
				p.MsgChannels = append(p.MsgChannels, channelId)
			}

			// Connect components based on types while avoiding duplicate pointer insertion
			switch fromType {
			case "INPUT":
				if in, ok := p.Inputs[fromId]; ok {
					if !containsChan(in.DownStream, msgChan) {
						in.DownStream = append(in.DownStream, &msgChan)
					}
				}
			case "RULESET":
				if rs, ok := p.Rulesets[fromId]; ok {
					rs.DownStream[to] = ptr
				}
			}

			switch toType {
			case "RULESET":
				if rs, ok := p.Rulesets[toId]; ok {
					rs.UpStream[from] = ptr
				}
			case "OUTPUT":
				if out, ok := p.Outputs[toId]; ok {
					if !containsChan(out.UpStream, msgChan) {
						out.UpStream = append(out.UpStream, &msgChan)
					}
				}
			}
		}
	}

	// Log the final ProjectNodeSequence for each component (already set in loadComponentsFromGlobal)
	for _, in := range p.Inputs {
		logger.Info("Input component sequence", "project", p.Id, "input", in.Id, "sequence", in.ProjectNodeSequence)
	}
	for _, out := range p.Outputs {
		logger.Info("Output component sequence", "project", p.Id, "output", out.Id, "sequence", out.ProjectNodeSequence)
	}
	for _, rs := range p.Rulesets {
		logger.Info("Ruleset component sequence", "project", p.Id, "ruleset", rs.RulesetID, "sequence", rs.ProjectNodeSequence)
	}

	return nil
}

// RestartProjectsSafely restarts multiple projects with proper shared component handling
// Returns the number of successfully restarted projects
// trigger: "user_action" for direct user restarts, "component_change" for component-triggered restarts
func RestartProjectsSafely(projectIDs []string, trigger string) (int, error) {
	if len(projectIDs) == 0 {
		return 0, nil
	}

	logger.Info("Starting batch project restart", "count", len(projectIDs), "trigger", trigger)
	restartedCount := 0

	// Sort project IDs to ensure consistent restart order
	sort.Strings(projectIDs)

	// Restart each project individually to respect component sharing
	// The Project.Stop() and Project.Start() methods handle shared components correctly
	for _, projectID := range projectIDs {
		common.GlobalMu.RLock()
		proj, exists := GlobalProject.Projects[projectID]
		common.GlobalMu.RUnlock()

		if !exists {
			logger.Error("Project not found for restart", "id", projectID)
			continue
		}

		if proj.Status == ProjectStatusRunning {
			logger.Info("Restarting project", "id", projectID)
			startTime := time.Now()

			// Stop the project (respects shared components)
			if err := proj.Stop(); err != nil {
				logger.Error("Failed to stop project during restart", "id", projectID, "error", err)
				// Record failed restart operation to Operations History
				// Note: All nodes record to Redis with TTL
				common.RecordProjectOperation(common.OpTypeProjectRestart, projectID, "failed", fmt.Sprintf("Failed to stop: %v", err), map[string]interface{}{
					"trigger": trigger,
					"phase":   "stop",
				})
				continue // Skip starting if stop failed
			}
			logger.Info("Stopped project during restart", "id", projectID)

			// Start the project (respects shared components)
			if err := proj.Start(); err != nil {
				logger.Error("Failed to start project during restart", "id", projectID, "error", err)
				// Record failed restart operation to Operations History
				// Note: All nodes record to Redis with TTL
				common.RecordProjectOperation(common.OpTypeProjectRestart, projectID, "failed", fmt.Sprintf("Failed to start: %v", err), map[string]interface{}{
					"trigger": trigger,
					"phase":   "start",
				})
			} else {
				restartedCount++
				logger.Info("Successfully restarted project", "id", projectID, "duration", time.Since(startTime))
				// Record successful restart operation to Operations History
				// Note: All nodes record to Redis with TTL
				common.RecordProjectOperation(common.OpTypeProjectRestart, projectID, "success", "", map[string]interface{}{
					"duration_ms": time.Since(startTime).Milliseconds(),
					"trigger":     trigger,
				})
			}
		} else if proj.Status == ProjectStatusStarting {
			logger.Info("Skipping project restart (currently starting)", "id", projectID, "status", proj.Status)
		} else if proj.Status == ProjectStatusStopping {
			logger.Info("Skipping project restart (currently stopping)", "id", projectID, "status", proj.Status)
		} else {
			logger.Info("Skipping project restart (not running)", "id", projectID, "status", proj.Status)
		}
	}

	logger.Info("Batch project restart completed", "total_affected", len(projectIDs), "restarted", restartedCount)
	return restartedCount, nil
}

// GetQPSDataForNode collects QPS data from all running projects and components for this node
func GetQPSDataForNode(nodeID string) []common.QPSMetrics {
	var qpsMetrics []common.QPSMetrics
	now := time.Now()

	// Lock to read project data safely
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	// Collect data from all projects
	for projectID, proj := range GlobalProject.Projects {
		if proj.Status != ProjectStatusRunning {
			continue // Skip non-running projects
		}

		// Collect input QPS data
		for inputID, i := range proj.Inputs {
			qps := i.GetConsumeQPS()
			total := i.GetConsumeTotal()
			// Note: Using in-memory total directly since daily_stats system handles Redis persistence
			qpsMetrics = append(qpsMetrics, common.QPSMetrics{
				NodeID:              nodeID,
				ProjectID:           projectID,
				ComponentID:         inputID,
				ComponentType:       "input",
				ProjectNodeSequence: i.ProjectNodeSequence,
				QPS:                 qps,
				TotalMessages:       total,
				Timestamp:           now,
			})
		}

		// Collect output QPS data
		for outputID, o := range proj.Outputs {
			qps := o.GetProduceQPS()
			total := o.GetProduceTotal()
			// Note: Using in-memory total directly since daily_stats system handles Redis persistence
			qpsMetrics = append(qpsMetrics, common.QPSMetrics{
				NodeID:              nodeID,
				ProjectID:           projectID,
				ComponentID:         outputID,
				ComponentType:       "output",
				ProjectNodeSequence: o.ProjectNodeSequence,
				QPS:                 qps,
				TotalMessages:       total,
				Timestamp:           now,
			})
		}

		// Collect ruleset QPS data - now with real processing statistics
		for rulesetID, ruleset := range proj.Rulesets {
			qps := ruleset.GetProcessQPS() // Get real processing QPS
			total := ruleset.GetProcessTotal()
			// Note: Using in-memory total directly since daily_stats system handles Redis persistence
			qpsMetrics = append(qpsMetrics, common.QPSMetrics{
				NodeID:              nodeID,
				ProjectID:           projectID,
				ComponentID:         rulesetID,
				ComponentType:       "ruleset",
				ProjectNodeSequence: ruleset.ProjectNodeSequence,
				QPS:                 qps,   // Real QPS instead of 0
				TotalMessages:       total, // Real total messages instead of 0
				Timestamp:           now,
			})
		}
	}

	return qpsMetrics
}

// clearChannelReferences iterates all projects and removes pointers to the given channel from
// every component's UpStream / DownStream slices or maps. It must be called with
// GlobalProject.ProjectMu write-locked.
func clearChannelReferences(closedCh chan map[string]interface{}) {
	for _, proj := range GlobalProject.Projects {
		// Inputs downstream slice
		for _, in := range proj.Inputs {
			filtered := make([]*chan map[string]interface{}, 0, len(in.DownStream))
			for _, ptr := range in.DownStream {
				if ptr != nil && *ptr == closedCh {
					continue
				}
				filtered = append(filtered, ptr)
			}
			in.DownStream = filtered
		}

		// Rulesets upstream / downstream maps
		for _, rs := range proj.Rulesets {
			for k, ptr := range rs.UpStream {
				if ptr != nil && *ptr == closedCh {
					delete(rs.UpStream, k)
				}
			}
			for k, ptr := range rs.DownStream {
				if ptr != nil && *ptr == closedCh {
					delete(rs.DownStream, k)
				}
			}
		}

		// Outputs upstream slice
		for _, out := range proj.Outputs {
			filtered := make([]*chan map[string]interface{}, 0, len(out.UpStream))
			for _, ptr := range out.UpStream {
				if ptr != nil && *ptr == closedCh {
					continue
				}
				filtered = append(filtered, ptr)
			}
			out.UpStream = filtered
		}
	}
}

// removeEdgeChanId deletes mappings that reference the given channelId and removes
// all component references to the underlying channel. closedCh must be the channel
// instance corresponding to channelId.
func removeEdgeChanId(channelId string, closedCh chan map[string]interface{}) {
	// Remove edge->channelId mapping in a thread-safe manner
	GlobalProject.EdgeMapMu.Lock()
	for edge, cid := range GlobalProject.edgeChanIds {
		if cid == channelId {
			delete(GlobalProject.edgeChanIds, edge)
		}
	}
	GlobalProject.EdgeMapMu.Unlock()

	// Remove component pointers
	GlobalProject.ProjectMu.Lock()
	defer GlobalProject.ProjectMu.Unlock()
	clearChannelReferences(closedCh)
}

// decrementChannelRef decrements the reference counter and returns the new value (int64).
// If the channelId does not exist, it returns 0.
func decrementChannelRef(channelId string) int64 {
	GlobalProject.EdgeMapMu.RLock()
	cntPtr, ok := GlobalProject.msgChansCounter[channelId]
	GlobalProject.EdgeMapMu.RUnlock()
	if !ok {
		return 0
	}
	newVal := cntPtr.Add(-1)
	return newVal
}

// updateProjectStatusRedis writes status to Redis hash and publishes event with error handling
func updateProjectStatusRedis(nodeID, projectID string, status ProjectStatus) {
	if common.GetRedisClient() == nil {
		logger.Warn("Redis client not available, cannot update project status", "node_id", nodeID, "project_id", projectID)
		return
	}

	// Only sync stable states (running, stopped). Skip transient states (starting, stopping, error).
	if status != ProjectStatusRunning && status != ProjectStatusStopped {
		logger.Debug("Skipping Redis sync for transient project status", "node_id", nodeID, "project_id", projectID, "status", status)
		return
	}

	// Ensure we use the correct node ID for consistency
	actualNodeID := nodeID
	if actualNodeID == "" {
		actualNodeID = common.Config.LocalIP
	}

	// Update project status in Redis hash with retry
	hashKey := "cluster:proj_states:" + actualNodeID
	for attempt := 0; attempt < 3; attempt++ {
		if err := common.RedisHSet(hashKey, projectID, string(status)); err != nil {
			logger.Error("Failed to update project status in Redis", "node_id", actualNodeID, "project_id", projectID, "attempt", attempt+1, "error", err)
			if attempt == 2 {
				logger.Error("Failed to update project status after 3 attempts", "node_id", actualNodeID, "project_id", projectID)
				return
			}
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond) // Exponential backoff
			continue
		}
		break
	}

	// Publish project status event with retry and timestamp
	now := time.Now()
	evt := map[string]interface{}{
		"node_id":           actualNodeID,
		"project_id":        projectID,
		"status":            string(status),
		"status_changed_at": now.Format(time.RFC3339),
	}
	data, err := json.Marshal(evt)
	if err != nil {
		logger.Error("Failed to marshal project status event", "node_id", actualNodeID, "project_id", projectID, "error", err)
		return
	}

	for attempt := 0; attempt < 3; attempt++ {
		if err := common.RedisPublish("cluster:proj_status", string(data)); err != nil {
			logger.Error("Failed to publish project status event", "node_id", actualNodeID, "project_id", projectID, "attempt", attempt+1, "error", err)
			if attempt == 2 {
				logger.Error("Failed to publish project status after 3 attempts", "node_id", actualNodeID, "project_id", projectID)
				return
			}
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond) // Exponential backoff
			continue
		}
		break
	}

	logger.Debug("Project status updated successfully", "node_id", actualNodeID, "project_id", projectID, "status", status, "timestamp", now.Format(time.RFC3339))
}
