package project

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
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

// collectAllComponentStats collects current statistics from all running components
// All increments are collected atomically at the same moment to ensure consistency
func collectAllComponentStats() []common.DailyStatsData {
	var components []common.DailyStatsData

	// Take a snapshot of running projects to minimize lock time
	var runningProjects []*Project
	GlobalProject.ProjectMu.RLock()
	for _, proj := range GlobalProject.Projects {
		if proj.Status == ProjectStatusRunning {
			runningProjects = append(runningProjects, proj)
		}
	}
	GlobalProject.ProjectMu.RUnlock()

	for _, proj := range runningProjects {
		for _, i := range proj.Inputs {
			increment := i.GetIncrementAndUpdate()
			if increment > 0 {
				components = append(components, common.DailyStatsData{
					ProjectID:           proj.Id,
					ComponentID:         i.Id,
					ComponentType:       "input",
					ProjectNodeSequence: i.ProjectNodeSequence,
					TotalMessages:       increment,
				})
			}
		}

		for _, o := range proj.Outputs {
			increment := o.GetIncrementAndUpdate()
			if increment > 0 {
				components = append(components, common.DailyStatsData{
					ProjectID:           proj.Id,
					ComponentID:         o.Id,
					ComponentType:       "output",
					ProjectNodeSequence: o.ProjectNodeSequence,
					TotalMessages:       increment,
				})
			}
		}

		for _, r := range proj.Rulesets {
			increment := r.GetIncrementAndUpdate()
			if increment > 0 {
				components = append(components, common.DailyStatsData{
					ProjectID:           proj.Id,
					ComponentID:         r.RulesetID,
					ComponentType:       "ruleset",
					ProjectNodeSequence: r.ProjectNodeSequence,
					TotalMessages:       increment,
				})
			}
		}
	}

	for pluginName, p := range plugin.Plugins {
		// Plugin success statistics - use increment method
		successIncrement := p.GetSuccessIncrementAndUpdate()
		components = append(components, common.DailyStatsData{
			ProjectID:           "global", // Plugins are global across all projects
			ComponentID:         pluginName,
			ComponentType:       "plugin_success",
			ProjectNodeSequence: fmt.Sprintf("PLUGIN.%s.success", pluginName),
			TotalMessages:       successIncrement, // Now this is the increment, not total
		})

		// Plugin failure statistics - use increment method
		failureIncrement := p.GetFailureIncrementAndUpdate()
		components = append(components, common.DailyStatsData{
			ProjectID:           "global", // Plugins are global across all projects
			ComponentID:         pluginName,
			ComponentType:       "plugin_failure",
			ProjectNodeSequence: fmt.Sprintf("PLUGIN.%s.failure", pluginName),
			TotalMessages:       failureIncrement, // Now this is the increment, not total
		})
	}

	return components
}

// projectCommandHandler implements cluster.ProjectCommandHandler interface
type projectCommandHandler struct{}

// ExecuteCommand implements the ProjectCommandHandler interface
func (h *projectCommandHandler) ExecuteCommand(projectID, action string) error {
	return h.ExecuteCommandWithOptions(projectID, action, true) // Default: record operations
}

func (h *projectCommandHandler) ExecuteCommandWithOptions(projectID, action string, recordOperation bool) error {
	proj, exists := GlobalProject.Projects[projectID]
	if !exists {
		// Try to create project from global config if it doesn't exist
		logger.Info("Project not found locally, attempting to create from global config", "project_id", projectID)

		// First try to get config from Redis (most reliable source)
		projectConfig, err := common.GetProjectConfig(projectID)
		if err != nil || projectConfig == "" {
			// Fallback to global config map
			common.GlobalMu.RLock()
			projectConfig = common.AllProjectRawConfig[projectID]
			common.GlobalMu.RUnlock()
		}

		if projectConfig != "" {
			// Create project from config
			newProj, err := NewProject("", projectConfig, projectID)
			if err != nil {
				logger.Error("Failed to create project from config", "project_id", projectID, "error", err)
				return fmt.Errorf("failed to create project from config: %w", err)
			}

			// Add to global projects
			GlobalProject.Projects[projectID] = newProj
			proj = newProj
			logger.Info("Successfully created project from config", "project_id", projectID)
		} else {
			logger.Error("Project config not found in Redis or global config", "project_id", projectID)
			return fmt.Errorf("project not found: %s", projectID)
		}
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
			// Record operation failure only if requested
			if recordOperation {
				common.RecordProjectOperation(common.OpTypeProjectStart, projectID, "failed", err.Error(), map[string]interface{}{
					"triggered_by": "cluster_command",
					"node_id":      nodeID,
				})
			}
			return fmt.Errorf("failed to start project: %w", err)
		}
		// Record operation success only if requested
		if recordOperation {
			common.RecordProjectOperation(common.OpTypeProjectStart, projectID, "success", "", map[string]interface{}{
				"triggered_by": "cluster_command",
				"node_id":      nodeID,
			})
		}
		logger.Info("Project started successfully via cluster command", "project_id", projectID)
		return nil

	case "stop":
		if proj.Status == ProjectStatusStopped {
			logger.Info("Project already stopped", "project_id", projectID)
			return nil
		}
		err := proj.Stop()
		if err != nil {
			// Record operation failure only if requested
			if recordOperation {
				common.RecordProjectOperation(common.OpTypeProjectStop, projectID, "failed", err.Error(), map[string]interface{}{
					"triggered_by": "cluster_command",
					"node_id":      nodeID,
				})
			}
			return fmt.Errorf("failed to stop project: %w", err)
		}
		// Record operation success only if requested
		if recordOperation {
			common.RecordProjectOperation(common.OpTypeProjectStop, projectID, "success", "", map[string]interface{}{
				"triggered_by": "cluster_command",
				"node_id":      nodeID,
			})
		}
		logger.Info("Project stopped successfully via cluster command", "project_id", projectID)
		return nil

	case "restart":
		// First stop if running
		if proj.Status == ProjectStatusRunning {
			err := proj.Stop()
			if err != nil {
				if recordOperation {
					common.RecordProjectOperation(common.OpTypeProjectRestart, projectID, "failed", fmt.Sprintf("Failed to stop: %v", err), map[string]interface{}{
						"triggered_by": "cluster_command",
						"node_id":      nodeID,
					})
				}
				return fmt.Errorf("failed to stop project for restart: %w", err)
			}
		}

		// Then start
		err := proj.Start()
		if err != nil {
			if recordOperation {
				common.RecordProjectOperation(common.OpTypeProjectRestart, projectID, "failed", fmt.Sprintf("Failed to start: %v", err), map[string]interface{}{
					"triggered_by": "cluster_command",
					"node_id":      nodeID,
				})
			}
			return fmt.Errorf("failed to start project for restart: %w", err)
		}
		// Record operation success only if requested
		if recordOperation {
			common.RecordProjectOperation(common.OpTypeProjectRestart, projectID, "success", "", map[string]interface{}{
				"triggered_by": "cluster_command",
				"node_id":      nodeID,
			})
		}
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

	common.SetStatsCollector(collectAllComponentStats)
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

	flowGraph, err := p.parseContent()
	if err != nil {
		return fmt.Errorf("failed to parse project content: %v", err)
	}

	// Check for duplicate project content by comparing projectNodeSequences
	if err := checkProjectContentDuplication(cfg.Id, flowGraph); err != nil {
		return err
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
	}

	// Initialize components
	if err := p.initComponents(); err != nil {
		p.setProjectStatus(ProjectStatusError, err)
		return p, fmt.Errorf("failed to initialize project components: %w", err)
	}

	logger.Info("Project created successfully", "id", p.Id, "status", p.Status)

	// Add to global project registry
	GlobalProject.Projects[p.Id] = p

	// Update global project config map for cluster synchronization
	common.GlobalMu.Lock()
	if common.AllProjectRawConfig == nil {
		common.AllProjectRawConfig = make(map[string]string)
	}
	common.AllProjectRawConfig[p.Id] = p.Config.RawConfig
	common.GlobalMu.Unlock()

	// Store project config in Redis for cluster-wide access
	if err := common.StoreProjectConfig(p.Id, p.Config.RawConfig); err != nil {
		logger.Warn("Failed to store project config in Redis", "project", p.Id, "error", err)
	}

	logger.Info("Project created successfully", "project", p.Id)
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
		Testing:         true,
		Id:              cfg.Id,
		Status:          ProjectStatusStopped, // Start as stopped for testing
		StatusChangedAt: &now,
		Config:          &cfg,
		Inputs:          make(map[string]*input.Input),
		Outputs:         make(map[string]*output.Output),
		Rulesets:        make(map[string]*rules_engine.Ruleset),
		MsgChannels:     make([]string, 0),
		stopChan:        make(chan struct{}),
	}

	// Initialize components with independent instances for testing
	if err := p.initComponentsForTesting(); err != nil {
		p.setProjectStatus(ProjectStatusError, err)
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

// Start starts the project and all its components
func (p *Project) Start() error {
	// Check if project was in error state or stopped state - both need force full reload
	wasErrorState := p.Status == ProjectStatusError
	wasStoppedState := p.Status == ProjectStatusStopped
	needsForceReload := wasErrorState || wasStoppedState

	if needsForceReload && !p.Testing {
		if wasErrorState {
			logger.Info("Project was in error state, will force full component reload", "project", p.Id, "previous_error", p.Err)
			p.Err = nil
		} else if wasStoppedState {
			logger.Info("Project was stopped, will force full component reload to restore all references", "project", p.Id)
		}
	}

	if p.Status == ProjectStatusRunning {
		return fmt.Errorf("project is already running %s", p.Id)
	}
	if p.Status == ProjectStatusStarting {
		return fmt.Errorf("project is currently starting, please wait %s", p.Id)
	}
	if p.Status == ProjectStatusStopping {
		return fmt.Errorf("project is currently stopping, please wait %s", p.Id)
	}

	p.setProjectStatus(ProjectStatusStarting, nil)

	// Initialize project control channels
	p.stopChan = make(chan struct{})

	// Parse project content to get component flow
	flowGraph, err := p.parseContent()
	if err != nil {
		p.setProjectStatus(ProjectStatusError, err)
		return fmt.Errorf("failed to parse project content: %v", err)
	}

	// Load components from global registry - force reload if was in error or stopped state
	err = p.loadComponentsFromGlobal(flowGraph, needsForceReload)
	if err != nil {
		p.setProjectStatus(ProjectStatusError, err)
		return fmt.Errorf("failed to load components: %v", err)
	}

	// Create fresh channel connections
	err = p.createChannelConnections(flowGraph)
	if err != nil {
		p.setProjectStatus(ProjectStatusError, err)
		return fmt.Errorf("failed to create channel connections: %v", err)
	}

	// Start inputs first
	for _, in := range p.Inputs {
		if p.Testing {
			//todo
		} else {
			// In production mode, check component sharing
			runningCount := UsageCounter.CountProjectsUsingInput(in.Id, p.Id)
			if runningCount == 0 {
				if err := in.Start(); err != nil {
					errorMsg := fmt.Errorf("failed to start shared input %s: %v", in.Id, err)
					logger.Error("Shared input component startup failed", "project", p.Id, "input", in.Id, "error", err)
					p.cleanupComponentsOnStartupFailure()
					p.setProjectStatus(ProjectStatusError, errorMsg)
					return errorMsg
				}
			} else {
				// Input is shared with other projects, verify connectivity using CheckConnectivity
				logger.Info("Input component shared with other projects, verifying connectivity", "project", p.Id, "input", in.Id, "other_projects_using", runningCount)
				connectivityResult := in.CheckConnectivity()
				if status, ok := connectivityResult["status"].(string); ok && status == "error" {
					errorMsg := fmt.Errorf("shared input %s connectivity check failed: %v", in.Id, connectivityResult["message"])
					logger.Error("Shared input component connectivity failed", "project", p.Id, "input", in.Id, "error", errorMsg)
					p.cleanupComponentsOnStartupFailure()
					p.setProjectStatus(ProjectStatusError, errorMsg)
					return errorMsg
				}
				logger.Info("Shared input component connectivity verified", "project", p.Id, "input", in.Id)
			}
		}
	}

	// Start rulesets after inputs
	for _, rs := range p.Rulesets {
		if p.Testing {
			// In test mode, directly start all rulesets
			logger.Info("Starting ruleset in test mode", "project", p.Id, "ruleset", rs.RulesetID)
			if err := rs.Start(); err != nil {
				errorMsg := fmt.Errorf("failed to start ruleset %s: %v", rs.RulesetID, err)
				logger.Error("Ruleset component startup failed", "project", p.Id, "ruleset", rs.RulesetID, "error", err)
				p.cleanupComponentsOnStartupFailure()
				p.setProjectStatus(ProjectStatusError, errorMsg)
				return errorMsg
			}
		} else {
			runningCount := UsageCounter.CountProjectsUsingRulesetInstance(rs.RulesetID, rs.ProjectNodeSequence, p.Id)
			if runningCount == 0 {
				if err := rs.Start(); err != nil {
					errorMsg := fmt.Errorf("failed to start ruleset %s: %v", rs.RulesetID, err)
					logger.Error("Ruleset instance startup failed", "project", p.Id, "ruleset", rs.RulesetID, "error", err)
					p.cleanupComponentsOnStartupFailure()
					p.setProjectStatus(ProjectStatusError, errorMsg)
					return errorMsg
				}
			}
		}
	}

	// Start outputs last
	for _, out := range p.Outputs {
		if p.Testing {
			logger.Info("Starting output in test mode", "project", p.Id, "output", out.Id)
			if err := out.Start(); err != nil {
				errorMsg := fmt.Errorf("failed to start output %s: %v", out.Id, err)
				logger.Error("Output component startup failed", "project", p.Id, "output", out.Id, "error", err)
				p.cleanupComponentsOnStartupFailure()
				p.setProjectStatus(ProjectStatusError, errorMsg)
				return errorMsg
			}
		} else {
			// In production mode, check component sharing
			runningCount := UsageCounter.CountProjectsUsingOutputInstance(out.Id, out.ProjectNodeSequence, p.Id)
			if runningCount == 0 {
				if err := out.Start(); err != nil {
					errorMsg := fmt.Errorf("failed to start output %s: %v", out.Id, err)
					logger.Error("Output instance startup failed", "project", p.Id, "output", out.Id, "error", err)
					p.cleanupComponentsOnStartupFailure()
					p.setProjectStatus(ProjectStatusError, errorMsg)
					return errorMsg
				}
			} else {
				// Output is shared with other projects, verify connectivity using CheckConnectivity
				logger.Info("Output component shared with other projects, verifying connectivity", "project", p.Id, "output", out.Id, "other_projects_using", runningCount)
				connectivityResult := out.CheckConnectivity()
				if status, ok := connectivityResult["status"].(string); ok && status == "error" {
					errorMsg := fmt.Errorf("shared output %s connectivity check failed: %v", out.Id, connectivityResult["message"])
					logger.Error("Shared output component connectivity failed", "project", p.Id, "output", out.Id, "error", errorMsg)
					p.cleanupComponentsOnStartupFailure()
					p.setProjectStatus(ProjectStatusError, errorMsg)
					return errorMsg
				}
				logger.Info("Shared output component connectivity verified", "project", p.Id, "output", out.Id)
			}
		}
	}

	if p.Testing {
		// In test mode, don't update Redis
		now := time.Now()
		p.Status = ProjectStatusRunning
		p.StatusChangedAt = &now
		logger.Info("Project started successfully in test mode", "project", p.Id)
	} else {
		// In production mode, use unified status update
		p.setProjectStatus(ProjectStatusRunning, nil)
	}

	// After the project is successfully started, recalculate dependencies synchronously
	AnalyzeProjectDependencies()
	return nil
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
	p.setProjectStatus(ProjectStatusStopping, nil)

	// Check if project is in error state
	if p.Err != nil {
		logger.Warn("Stopping project with errors", "id", p.Id, "error", p.Err)
	}

	// Overall timeout for the entire stop process
	overallTimeout := time.After(2 * time.Minute)
	stopCompleted := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic during project stop", "project", p.Id, "panic", r)
				stopCompleted <- fmt.Errorf("panic during stop: %v", r)
			}
		}()

		// Use the internal stopComponents function
		err := p.stopComponentsInternal() // Don't save status to Redis, project methods don't handle persistence
		stopCompleted <- err
	}()

	select {
	case err := <-stopCompleted:
		if err != nil {
			logger.Error("Project stop completed with error", "project", p.Id, "error", err)
			return err
		}
		// Ensure the final 'stopped' status is synced to Redis
		// This is important for cluster visibility, especially for followers
		p.setProjectStatus(ProjectStatusStopped, nil)
		logger.Info("Project stopped successfully", "project", p.Id)
		return nil
	case <-overallTimeout:
		logger.Error("Project stop timeout exceeded, forcing cleanup", "project", p.Id)

		// Force cleanup
		if err := p.forceCleanup(); err != nil {
			logger.Error("Force cleanup failed", "project", p.Id, "error", err)
		}

		p.setProjectStatus(ProjectStatusStopped, nil)

		return fmt.Errorf("project stop timeout exceeded, forced cleanup completed for %s", p.Id)
	}
}

func (p *Project) stopComponentsInternal() error {
	logger.Info("Stopping project components", "project", p.Id)

	// Step 1: Stop inputs first to prevent new data
	logger.Info("Step 1: Stopping inputs to prevent new data", "project", p.Id, "count", len(p.Inputs))
	for inputId, in := range p.Inputs {
		otherProjectsUsing := UsageCounter.CountProjectsUsingInput(in.Id, p.Id)
		if otherProjectsUsing == 0 {
			// No other projects using this input, stop the entire input
			logger.Info("Stopping input component completely", "project", p.Id, "input", in.Id)
			startTime := time.Now()
			if err := in.Stop(); err != nil {
				logger.Error("Failed to stop input", "project", p.Id, "input", in.Id, "error", err)
				// Continue with other inputs instead of failing immediately
			} else {
				logger.Info("Stopped input", "project", p.Id, "input", in.Id, "duration", time.Since(startTime))
			}
		} else {
			// Other projects are using this input, only disconnect channels
			logger.Info("Input shared with other projects, disconnecting channels only",
				"project", p.Id, "input", in.Id, "other_projects_using", otherProjectsUsing)

			// Find and remove channels that belong to this project's flow
			// We need to identify which downstream channels belong to this project
			channelsToRemove := p.findInputDownstreamChannelsForProject(inputId)

			// Remove these channels from the input's downstream
			p.disconnectInputChannels(in, channelsToRemove)

			logger.Info("Disconnected input channels", "project", p.Id, "input", in.Id,
				"channels_removed", len(channelsToRemove))
		}
	}

	// Step 2: Wait for all data to be processed through the entire pipeline
	logger.Info("Step 2: Waiting for data to be fully processed through pipeline", "project", p.Id)
	p.waitForCompleteDataProcessing()

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

	common.GlobalDailyStatsManager.CollectAllComponentsData()

	// Step 4: Stop outputs last (only if not used by other projects)
	logger.Info("Step 4: Stopping outputs", "project", p.Id, "count", len(p.Outputs))
	for _, out := range p.Outputs {
		otherProjectsUsing := UsageCounter.CountProjectsUsingOutputInstance(out.Id, out.ProjectNodeSequence, p.Id)
		if otherProjectsUsing == 0 {
			logger.Info("Stopping output instance", "project", p.Id, "output", out.Id, "sequence", out.ProjectNodeSequence, "other_projects_using", otherProjectsUsing)
			startTime := time.Now()
			if err := out.Stop(); err != nil {
				logger.Error("Failed to stop output", "project", p.Id, "output", out.Id, "error", err)
			} else {
				logger.Info("Stopped output", "project", p.Id, "output", out.Id, "duration", time.Since(startTime))
			}
		} else {
			logger.Info("Output instance still used by other projects, skipping stop", "project", p.Id, "output", out.Id, "sequence", out.ProjectNodeSequence, "other_projects_using", otherProjectsUsing)
		}
	}

	// Step 5: Wait for all project goroutines to finish
	waitDone := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		logger.Info("All project goroutines finished", "project", p.Id)
	case <-time.After(60 * time.Second):
		logger.Warn("Timeout waiting for project goroutines to finish", "project", p.Id)
	}

	// Step 6: Clean up channels
	logger.Info("Step 6: Cleaning up channels", "project", p.Id, "channel_count", len(p.MsgChannels))
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

	// Step 7: Clear component references
	logger.Info("Step 7: Clearing component references", "project", p.Id)
	p.Inputs = make(map[string]*input.Input)
	p.Outputs = make(map[string]*output.Output)
	p.Rulesets = make(map[string]*rules_engine.Ruleset)

	// Step 8: Close project channels after all goroutines are done
	if p.stopChan != nil {
		close(p.stopChan)
		p.stopChan = nil
	}

	p.setProjectStatus(ProjectStatusStopped, nil)
	logger.Info("Finished stopping project components", "project", p.Id)
	return nil
}

// forceCleanup performs aggressive cleanup when normal stop fails
func (p *Project) forceCleanup() error {
	// Force close all channels without waiting
	for _, channelId := range p.MsgChannels {
		GlobalProject.EdgeMapMu.Lock()
		if ch, exists := GlobalProject.msgChans[channelId]; exists {
			// Don't wait for graceful channel closure
			select {
			case <-ch:
				// Channel already closed
			default:
				close(ch)
			}
			delete(GlobalProject.msgChans, channelId)
			delete(GlobalProject.msgChansCounter, channelId)
		}
		GlobalProject.EdgeMapMu.Unlock()
	}

	// Force close project stop channel
	if p.stopChan != nil {
		select {
		case <-p.stopChan:
			// Already closed
		default:
			close(p.stopChan)
		}
		p.stopChan = nil
	}

	common.GlobalDailyStatsManager.CollectAllComponentsData()

	// Clear component references immediately
	p.Inputs = make(map[string]*input.Input)
	p.Outputs = make(map[string]*output.Output)
	p.Rulesets = make(map[string]*rules_engine.Ruleset)
	p.MsgChannels = []string{}

	logger.Warn("Force cleanup completed for project", "project", p.Id)
	return nil
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

// SaveProjectStatus saves project status to Redis for cluster visibility
func (p *Project) SaveProjectStatus() error {
	// Update global project config map for cluster synchronization
	common.GlobalMu.Lock()
	if common.AllProjectRawConfig == nil {
		common.AllProjectRawConfig = make(map[string]string)
	}
	common.AllProjectRawConfig[p.Id] = p.Config.RawConfig
	common.GlobalMu.Unlock()

	// Store project config in Redis for cluster-wide access
	if err := common.StoreProjectConfig(p.Id, p.Config.RawConfig); err != nil {
		logger.Warn("Failed to store project config in Redis", "project", p.Id, "error", err)
	}

	// Write to Redis for cluster-wide visibility
	updateProjectStatusRedis(common.Config.LocalIP, p.Id, p.Status, p.StatusChangedAt)

	return nil
}

// StopForTesting stops the project quickly for testing purposes and ensures complete cleanup
func (p *Project) StopForTesting() error {
	logger.Info("Stopping and destroying test project", "project", p.Id)

	// Stop components quickly without waiting for channel drainage
	// Note: Test components are completely isolated, so stopping them won't affect production
	for _, in := range p.Inputs {
		// Test inputs are virtual, just clear their downstream connections
		in.DownStream = []*chan map[string]interface{}{}
		// Reset atomic counter for test cleanup
		previousTotal := in.ResetConsumeTotal()
		logger.Debug("Cleared test input and reset counter", "project", p.Id, "input", in.Id, "previous_total", previousTotal)
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

	// For test projects, only update memory status without Redis
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

// waitForCompleteDataProcessing waits for all data to be fully processed through the pipeline
// This includes waiting for channels to empty AND thread pools to complete all tasks
func (p *Project) waitForCompleteDataProcessing() {
	logger.Info("Waiting for complete data processing through pipeline", "project", p.Id)

	overallTimeout := time.After(60 * time.Second) // 60 second overall timeout
	checkInterval := 100 * time.Millisecond
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	logCounter := 0
	for {
		select {
		case <-overallTimeout:
			logger.Warn("Data processing timeout reached, proceeding with shutdown", "project", p.Id)
			return
		case <-ticker.C:
			allProcessed := true
			totalChannelMessages := 0
			totalRunningTasks := 0

			// Check all channels for remaining messages
			for _, channelId := range p.MsgChannels {
				GlobalProject.EdgeMapMu.RLock()
				if ch, exists := GlobalProject.msgChans[channelId]; exists {
					chLen := len(ch)
					if chLen > 0 {
						allProcessed = false
						totalChannelMessages += chLen
					}
				}
				GlobalProject.EdgeMapMu.RUnlock()
			}

			// Check all rulesets for running tasks and pending messages
			for _, rs := range p.Rulesets {
				// Check running tasks in thread pool
				runningTasks := rs.GetRunningTaskCount()
				if runningTasks > 0 {
					allProcessed = false
					totalRunningTasks += runningTasks
				}

				// Check upstream channels for pending messages
				for _, upCh := range rs.UpStream {
					if upCh != nil {
						pendingInUpstream := len(*upCh)
						if pendingInUpstream > 0 {
							allProcessed = false
							totalChannelMessages += pendingInUpstream
						}
					}
				}

				// Check downstream channels
				for _, downCh := range rs.DownStream {
					if downCh != nil {
						pendingInDownstream := len(*downCh)
						if pendingInDownstream > 0 {
							allProcessed = false
							totalChannelMessages += pendingInDownstream
						}
					}
				}
			}

			// Check all outputs for pending data (including internal channels)
			for _, out := range p.Outputs {
				pendingCount := out.GetPendingMessageCount()

				if pendingCount > 0 {
					allProcessed = false
					totalChannelMessages += pendingCount
				}
			}

			if allProcessed {
				logger.Info("All data processing completed", "project", p.Id)
				return
			}

			logCounter++
		}
	}
}

// findInputDownstreamChannelsForProject finds which downstream channels belong to this project
func (p *Project) findInputDownstreamChannelsForProject(inputId string) []chan map[string]interface{} {
	var channelsToRemove []chan map[string]interface{}

	// Get the flow graph to understand the connections
	flowGraph, err := p.parseContent()
	if err != nil {
		logger.Error("Failed to parse project content for channel identification", "error", err)
		return channelsToRemove
	}

	// Find all edges starting from this input
	inputNode := fmt.Sprintf("INPUT.%s", inputId)
	if downstreams, exists := flowGraph[inputNode]; exists {
		for _, downstream := range downstreams {
			// Find the corresponding channel
			edgeKey := fmt.Sprintf("%s->%s", inputNode, downstream)

			GlobalProject.EdgeMapMu.RLock()
			if channelId, exists := GlobalProject.edgeChanIds[edgeKey]; exists {
				if ch, exists := GlobalProject.msgChans[channelId]; exists {
					channelsToRemove = append(channelsToRemove, ch)
				}
			}
			GlobalProject.EdgeMapMu.RUnlock()
		}
	}

	return channelsToRemove
}

// disconnectInputChannels removes specific channels from input's downstream
func (p *Project) disconnectInputChannels(in *input.Input, channelsToRemove []chan map[string]interface{}) {
	if len(channelsToRemove) == 0 {
		return
	}

	// Create a new downstream slice without the channels to remove
	newDownstream := make([]*chan map[string]interface{}, 0, len(in.DownStream))

	for _, downCh := range in.DownStream {
		shouldRemove := false
		for _, removeCh := range channelsToRemove {
			if downCh != nil && *downCh == removeCh {
				shouldRemove = true
				break
			}
		}
		if !shouldRemove {
			newDownstream = append(newDownstream, downCh)
		}
	}

	// Update the input's downstream
	in.DownStream = newDownstream

	logger.Info("Updated input downstream channels",
		"input", in.Id,
		"original_count", len(in.DownStream)+len(channelsToRemove),
		"new_count", len(newDownstream),
		"removed_count", len(channelsToRemove))
}

// loadComponentsFromGlobal loads component references from global registry based on flow graph
func (p *Project) loadComponentsFromGlobal(flowGraph map[string][]string, forceReload bool) error {
	logger.Info("Loading components from global registry", "project", p.Id, "force_reload", forceReload)

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
		// Skip this check if force reload is enabled
		var foundOutput *output.Output
		if !forceReload {
			for _, existingProject := range GlobalProject.Projects {
				if existingOutput, exists := existingProject.Outputs[name]; exists {
					if existingOutput.ProjectNodeSequence == expectedSequence {
						foundOutput = existingOutput
						break
					}
				}
			}
		}

		if foundOutput != nil && !forceReload {
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
		// Skip this check if force reload is enabled
		var foundRuleset *rules_engine.Ruleset
		if !forceReload {
			for _, existingProject := range GlobalProject.Projects {
				if existingRuleset, exists := existingProject.Rulesets[name]; exists {
					if existingRuleset.ProjectNodeSequence == expectedSequence {
						foundRuleset = existingRuleset
						break
					}
				}
			}
		}

		if foundRuleset != nil && !forceReload {
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

		if proj.Status == ProjectStatusRunning || proj.Status == ProjectStatusError {
			logger.Info("Restarting project", "id", projectID, "status", proj.Status)
			startTime := time.Now()

			// If project is in error state, just try to start directly (no need to stop)
			if proj.Status == ProjectStatusError {
				logger.Info("Project in error state, attempting direct start with full reload", "id", projectID)
				// Start the project (will force reload all components)
				if err := proj.Start(); err != nil {
					logger.Error("Failed to start error project during restart", "id", projectID, "error", err)
					// Record failed restart operation to Operations History
					common.RecordProjectOperation(common.OpTypeProjectRestart, projectID, "failed", fmt.Sprintf("Failed to start from error state: %v", err), map[string]interface{}{
						"trigger":   trigger,
						"phase":     "start_from_error",
						"was_error": true,
					})
				} else {
					restartedCount++
					logger.Info("Successfully restarted project from error state", "id", projectID, "duration", time.Since(startTime))
					// Record successful restart operation to Operations History
					common.RecordProjectOperation(common.OpTypeProjectRestart, projectID, "success", "", map[string]interface{}{
						"duration_ms": time.Since(startTime).Milliseconds(),
						"trigger":     trigger,
						"was_error":   true,
					})
				}
			} else {
				// Running project - stop then start
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
func updateProjectStatusRedis(nodeID, projectID string, status ProjectStatus, statusChangedAt *time.Time) {
	if common.GetRedisClient() == nil {
		logger.Warn("Redis client not available, cannot update project status", "node_id", nodeID, "project_id", projectID)
		return
	}

	// Ensure we use the correct node ID for consistency
	actualNodeID := nodeID
	if actualNodeID == "" {
		actualNodeID = common.Config.LocalIP
	}

	// Set real state (actual runtime status)
	if err := common.SetProjectRealState(actualNodeID, projectID, string(status)); err != nil {
		logger.Error("Failed to update project real state in Redis", "node_id", actualNodeID, "project_id", projectID, "status", status, "error", err)
		return
	}

	// Set timestamp
	var ts time.Time
	if statusChangedAt != nil {
		ts = *statusChangedAt
	} else {
		ts = time.Now()
	}

	if err := common.SetProjectStateTimestamp(actualNodeID, projectID, ts); err != nil {
		logger.Error("Failed to update project state timestamp in Redis", "node_id", actualNodeID, "project_id", projectID, "error", err)
	}

	// Note: proj_states should ONLY be modified in 3 scenarios:
	// 1. When user starts a project (API call)
	// 2. When user stops a project (API call)
	// 3. When user deletes a project
	// Any other modification is a BUG - proj_states represents user intention, not runtime state

	// Publish project status event with improved retry mechanism
	var timestamp time.Time
	if statusChangedAt != nil {
		timestamp = *statusChangedAt
	} else {
		timestamp = time.Now()
	}

	evt := map[string]interface{}{
		"node_id":           actualNodeID,
		"project_id":        projectID,
		"status":            string(status),
		"status_changed_at": timestamp.Format(time.RFC3339),
	}
	data, err := json.Marshal(evt)
	if err != nil {
		logger.Error("Failed to marshal project status event", "node_id", actualNodeID, "project_id", projectID, "error", err)
		return
	}

	if err := common.RedisPublishWithRetry("cluster:proj_status", string(data)); err != nil {
		logger.Error("Failed to publish project status after retries", "node_id", actualNodeID, "project_id", projectID, "error", err)
		return
	}

	logger.Debug("Project status updated successfully", "node_id", actualNodeID, "project_id", projectID, "status", status, "timestamp", timestamp.Format(time.RFC3339))
}

// cleanupComponentsOnStartupFailure cleans up components when project startup fails
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
}

// checkProjectContentDuplication checks if the project content is identical to any existing project
// by comparing all projectNodeSequences. Two projects are considered identical if they have
// exactly the same set of projectNodeSequences, regardless of the order in the flow definition.
func checkProjectContentDuplication(newProjectID string, flowGraph map[string][]string) error {
	// Calculate projectNodeSequences for the new project
	newProjectSequences := calculateProjectNodeSequences(newProjectID, flowGraph)
	if len(newProjectSequences) == 0 {
		// Empty project, no need to check duplication
		return nil
	}

	// Compare with existing projects
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	for existingProjectID, existingProject := range GlobalProject.Projects {
		if existingProjectID == newProjectID {
			// Skip self when updating existing project
			continue
		}

		// Parse existing project's content to get its flow graph
		existingFlowGraph, err := existingProject.parseContent()
		if err != nil {
			// Skip projects with parse errors
			continue
		}

		// Calculate projectNodeSequences for the existing project
		existingProjectSequences := calculateProjectNodeSequences(existingProjectID, existingFlowGraph)

		// Compare the two sets of projectNodeSequences
		if areProjectSequencesIdentical(newProjectSequences, existingProjectSequences) {
			return fmt.Errorf("project content is identical to existing project '%s': both projects have the same component flow structure", existingProjectID)
		}
	}

	return nil
}

// calculateProjectNodeSequences calculates all projectNodeSequences for a given project
func calculateProjectNodeSequences(projectID string, flowGraph map[string][]string) []string {
	var sequences []string
	componentSet := make(map[string]bool)

	// Collect all unique components from the flow graph
	for from, tos := range flowGraph {
		componentSet[from] = true
		for _, to := range tos {
			componentSet[to] = true
		}
	}

	// Generate projectNodeSequence for each component
	for component := range componentSet {
		sequence := fmt.Sprintf("%s.%s", projectID, component)
		sequences = append(sequences, sequence)
	}

	// Sort for consistent comparison
	sort.Strings(sequences)
	return sequences
}

// areProjectSequencesIdentical checks if two sets of projectNodeSequences are identical
func areProjectSequencesIdentical(sequences1, sequences2 []string) bool {
	if len(sequences1) != len(sequences2) {
		return false
	}

	// Both slices are already sorted, so we can compare directly
	for i, seq1 := range sequences1 {
		if seq1 != sequences2[i] {
			return false
		}
	}

	return true
}

// SetProjectStatus sets the project status and updates both memory and Redis state (public method)
// This is a unified function that ensures consistent status management across the system
func (p *Project) SetProjectStatus(status ProjectStatus, err error) {
	p.setProjectStatus(status, err)
}

// setProjectStatus sets the project status and updates both memory and Redis state
// This is the unified function for all project status changes
func (p *Project) setProjectStatus(status ProjectStatus, err error) {
	now := time.Now()
	p.Status = status
	p.StatusChangedAt = &now
	p.Err = err

	// Always update Redis status
	// This ensures cluster-wide visibility of status changes
	updateProjectStatusRedis(common.Config.LocalIP, p.Id, status, &now)
}
