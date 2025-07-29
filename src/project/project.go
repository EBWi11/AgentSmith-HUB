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
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var GlobalProject *GlobalProjectInfo

// collectAllComponentStats collects current statistics from all running components
// Optimized for high concurrency with minimal lock time
func collectAllComponentStats() []common.DailyStatsData {
	var components []common.DailyStatsData

	// Take a snapshot of running projects first to minimize lock time
	var runningProjects []*Project
	ForEachProject(func(id string, proj *Project) bool {
		if proj.Status == common.StatusRunning {
			runningProjects = append(runningProjects, proj)
		}
		return true
	})

	// Collect statistics from each project without global lock
	for _, proj := range runningProjects {
		// Collect input statistics
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

		// Collect output statistics
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

		// Collect ruleset statistics
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

	// Collect plugin statistics (plugins are global, no project lock needed)
	// Only collect if there are running projects or if increments are greater than 0
	for pluginName, p := range plugin.Plugins {
		// Plugin success statistics - use increment method
		successIncrement := p.GetSuccessIncrementAndUpdate()
		if successIncrement > 0 {
			components = append(components, common.DailyStatsData{
				ProjectID:           "global", // Plugins are global across all projects
				ComponentID:         pluginName,
				ComponentType:       "plugin_success",
				ProjectNodeSequence: fmt.Sprintf("PLUGIN.%s.success", pluginName),
				TotalMessages:       successIncrement, // Now this is the increment, not total
			})
		}

		// Plugin failure statistics - use increment method
		failureIncrement := p.GetFailureIncrementAndUpdate()
		if failureIncrement > 0 {
			components = append(components, common.DailyStatsData{
				ProjectID:           "global", // Plugins are global across all projects
				ComponentID:         pluginName,
				ComponentType:       "plugin_failure",
				ProjectNodeSequence: fmt.Sprintf("PLUGIN.%s.failure", pluginName),
				TotalMessages:       failureIncrement, // Now this is the increment, not total
			})
		}
	}

	return components
}

// GetAffectedProjects returns the list of project IDs affected by component changes
func GetAffectedProjects(componentType string, componentID string) []string {
	affectedProjects := make(map[string]struct{})

	switch componentType {
	case "input":
		// Find all projects using this input
		ForEachProject(func(projectID string, p *Project) bool {
			if p.CheckExist("INPUT", componentID) {
				// Check if user wants this project to be running
				if userWantsRunning, err := common.GetProjectUserIntention(projectID); err == nil && userWantsRunning {
					affectedProjects[projectID] = struct{}{}
				}
			}
			return true
		})
	case "output":
		// Find all projects using this output
		ForEachProject(func(projectID string, p *Project) bool {
			if p.CheckExist("OUTPUT", componentID) {
				// Check if user wants this project to be running
				if userWantsRunning, err := common.GetProjectUserIntention(projectID); err == nil && userWantsRunning {
					affectedProjects[projectID] = struct{}{}
				}
			}
			return true
		})
	case "ruleset":
		// Find all projects using this ruleset
		ForEachProject(func(projectID string, p *Project) bool {
			if p.CheckExist("RULESET", componentID) {
				// Check if user wants this project to be running
				if userWantsRunning, err := common.GetProjectUserIntention(projectID); err == nil && userWantsRunning {
					affectedProjects[projectID] = struct{}{}
				}
			}
			return true
		})
	case "project":
		// For project changes, check if user wants this project to be running
		if userWantsRunning, err := common.GetProjectUserIntention(componentID); err == nil && userWantsRunning {
			affectedProjects[componentID] = struct{}{}
		}
	}

	// Convert to string slice
	result := make([]string, 0, len(affectedProjects))
	for projectID := range affectedProjects {
		result = append(result, projectID)
	}

	return result
}

// projectCommandHandler implements cluster.ProjectCommandHandler interface
type projectCommandHandler struct{}

func (h *projectCommandHandler) ExecuteCommand(projectID, action string) error {
	return h.ExecuteCommandWithOptions(projectID, action, true)
}

func (h *projectCommandHandler) ExecuteCommandWithOptions(projectID, action string, recordOperation bool) error {
	nodeID := common.Config.LocalIP
	proj, exists := GetProject(projectID)
	if !exists {
		return fmt.Errorf("project not found: %s", projectID)
	}

	switch action {
	case "start":
		err := proj.Start(true)
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
		err := proj.Stop(true)
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
		err := proj.Restart(recordOperation, "cluster_command")
		if err != nil {
			return fmt.Errorf("failed to restart project via cluster command: %w", err)
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

// checkAllProjectComponentsImpl implements the actual component checking logic
func checkAllProjectComponentsImpl() []common.ProjectComponentError {
	var errors []common.ProjectComponentError

	// Check all running projects
	ForEachProject(func(projectID string, proj *Project) bool {
		// Only check running projects
		if proj.Status != common.StatusRunning {
			return true // Continue iteration
		}

		// Check input components
		for _, inputComp := range proj.Inputs {
			if inputComp.Err != nil {
				errors = append(errors, common.ProjectComponentError{
					ProjectID:   projectID,
					ComponentID: inputComp.Id,
					Type:        "input",
					Status:      inputComp.Status,
					Error:       inputComp.Err,
				})
			}
		}

		// Check output components
		for _, outputComp := range proj.Outputs {
			if outputComp.Err != nil {
				errors = append(errors, common.ProjectComponentError{
					ProjectID:   projectID,
					ComponentID: outputComp.Id,
					Type:        "output",
					Status:      outputComp.Status,
					Error:       outputComp.Err,
				})
			}
		}

		// Check ruleset components
		for _, rulesetComp := range proj.Rulesets {
			if rulesetComp.Err != nil {
				errors = append(errors, common.ProjectComponentError{
					ProjectID:   projectID,
					ComponentID: rulesetComp.RulesetID,
					Type:        "ruleset",
					Status:      rulesetComp.Status,
					Error:       rulesetComp.Err,
				})
			}
		}

		return true // Continue iteration
	})

	return errors
}

// SetProjectErrorStatus sets a project status to error with detailed error information
func SetProjectErrorStatus(projectID string, componentErrors []common.ProjectComponentError) {
	proj, exists := GetProject(projectID)
	if !exists {
		logger.Warn("Cannot set error status for non-existent project", "project", projectID)
		return
	}

	// Build error message from component errors
	var errorMsg strings.Builder
	errorMsg.WriteString("Component errors detected: ")

	for i, compErr := range componentErrors {
		if i > 0 {
			errorMsg.WriteString("; ")
		}
		errorMsg.WriteString(fmt.Sprintf("%s %s: %v", compErr.Type, compErr.ComponentID, compErr.Error))
	}

	// Set project to error status
	err := fmt.Errorf("%s", errorMsg.String())
	proj.SetProjectStatus(common.StatusStopped, err)

	logger.Error("Project set to error status due to component failures",
		"project", projectID,
		"component_count", len(componentErrors),
		"error", err)
}

func init() {
	GlobalProject = &GlobalProjectInfo{}
	GlobalProject.Projects = make(map[string]*Project)
	GlobalProject.Inputs = make(map[string]*input.Input)
	GlobalProject.Outputs = make(map[string]*output.Output)
	GlobalProject.Rulesets = make(map[string]*rules_engine.Ruleset)

	GlobalProject.PNSOutputs = make(map[string]*output.Output)
	GlobalProject.PNSRulesets = make(map[string]*rules_engine.Ruleset)

	GlobalProject.ProjectsNew = make(map[string]string)
	GlobalProject.InputsNew = make(map[string]string)
	GlobalProject.OutputsNew = make(map[string]string)
	GlobalProject.RulesetsNew = make(map[string]string)

	// AllProjectRawConfig is now managed through common.SetRawConfig functions
	common.SetStatsCollector(collectAllComponentStats)

	// Register the component checker function
	common.SetProjectComponentChecker(checkAllProjectComponentsImpl)

	// Register the project error setter function
	common.SetProjectErrorSetter(SetProjectErrorStatus)
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
		Status: common.StatusStopped,
		Config: &cfg,
	}

	err = p.parseContent()
	if err != nil {
		// Enhance error message with YAML line number adjustment
		errMsg := err.Error()

		// Extract line number from error message
		linePattern := `at line (\d+)`
		if match := regexp.MustCompile(linePattern).FindStringSubmatch(errMsg); len(match) > 1 {
			contentLineNum, _ := strconv.Atoi(match[1])

			// Calculate the actual line number in the full YAML
			// Find the line number of 'content:' in the original YAML
			lines := strings.Split(raw, "\n")
			contentLineIndex := -1
			for i, line := range lines {
				if strings.TrimSpace(line) == "content:" || strings.TrimSpace(line) == "content: |" {
					contentLineIndex = i
					break
				}
			}

			if contentLineIndex != -1 {
				// Adjust line number: content line number + content line index + 1
				actualLineNum := contentLineNum + contentLineIndex + 1
				// Replace the line number in the error message
				errMsg = regexp.MustCompile(`at line \d+`).ReplaceAllString(errMsg, fmt.Sprintf("at line %d", actualLineNum))
			}
		}

		return fmt.Errorf("failed to parse project content: %v", errMsg)
	}

	return nil
}

// NewProject creates a new project instance from a configuration file
// pp: Path to the project configuration file
func NewProject(path string, raw string, id string, test bool) (*Project, error) {
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

	p := &Project{
		Id:          cfg.Id,
		Status:      common.StatusStopped, // Default to stopped status, will be started by StartAllProject
		Config:      &cfg,
		Inputs:      make(map[string]*input.Input),
		Outputs:     make(map[string]*output.Output),
		Rulesets:    make(map[string]*rules_engine.Ruleset),
		MsgChannels: make(map[string]*chan map[string]interface{}, 0),
		Testing:     test,
	}

	// Initialize components
	if err := p.parseContent(); err != nil {
		p.SetProjectStatus(common.StatusError, err)
		return p, fmt.Errorf("failed to initialize project components: %w", err)
	}

	// For test projects, do NOT add to GlobalProject - keep them completely isolated
	if !test {
		// Use safe accessor to set project
		SetProject(p.Id, p)

		// Update global config map using the new accessor function
		common.SetRawConfig("project", p.Id, p.Config.RawConfig)

		// Store project config in Redis for cluster-wide access
		if err := common.StoreProjectConfig(p.Id, p.Config.RawConfig); err != nil {
			logger.Error("Failed to store project config in Redis", "project", p.Id, "error", err)
		}

		logger.Info("Project created successfully", "project", p.Id)
	} else {
		logger.Info("Test project created successfully (isolated)", "project", p.Id, "testing", true)
	}

	return p, nil
}

// parseContent parses the project content to build the data flow graph
func (p *Project) parseContent() error {
	flowGraph := make(map[string][]string)
	lines := strings.Split(p.Config.Content, "\n")
	edgeSet := make(map[string]struct{}) // Used to detect duplicate flows

	p.FlowNodes = []FlowNode{}
	p.BackUpFlowNodes = []FlowNode{}

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip comment lines (lines starting with #)
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Only support standard arrow format: ->
		parts := strings.Split(line, "->")

		if len(parts) != 2 {
			// Check for invalid arrow-like patterns and provide specific error messages
			if strings.Contains(line, "→") {
				return fmt.Errorf("invalid arrow format at line %d: use '->' instead of '→' in %q", lineNum+1, line)
			} else if strings.Contains(line, "—>") {
				return fmt.Errorf("invalid arrow format at line %d: use '->' instead of '—>' in %q", lineNum+1, line)
			} else if strings.Contains(line, "-->") {
				return fmt.Errorf("invalid arrow format at line %d: use '->' instead of '-->' in %q", lineNum+1, line)
			} else if strings.Contains(line, "=>") {
				return fmt.Errorf("invalid arrow format at line %d: use '=>' instead of '=>' in %q", lineNum+1, line)
			} else if strings.Contains(line, "—") || strings.Contains(line, "–") || strings.Contains(line, "―") {
				return fmt.Errorf("invalid arrow format at line %d: use '->' instead of dash characters in %q", lineNum+1, line)
			}
			return fmt.Errorf("invalid line format at line %d: missing or invalid arrow operator in %q (use '->')", lineNum+1, line)
		}

		from := strings.TrimSpace(parts[0])
		to := strings.TrimSpace(parts[1])

		// Validate node types
		fromType, fromID := parseNode(from)
		toType, toID := parseNode(to)

		if fromType == "" || toType == "" {
			return fmt.Errorf("invalid node format at line %d: %s -> %s (expected format: TYPE.ID -> TYPE.ID)", lineNum+1, from, to)
		}

		// Validate flow rules
		if toType == "INPUT" {
			return fmt.Errorf("INPUT node %q cannot be a destination at line %d", to, lineNum+1)
		}

		if fromType == "OUTPUT" {
			return fmt.Errorf("OUTPUT node %q cannot be a source at line %d", from, lineNum+1)
		}

		// Check for duplicate flows
		edgeKey := from + "->" + to
		if _, exists := edgeSet[edgeKey]; exists {
			return fmt.Errorf("duplicate data flow detected at line %d: %s", lineNum+1, edgeKey)
		}
		edgeSet[edgeKey] = struct{}{}

		// Add to flow graph as individual connections (not aggregated by source)
		// Use edge key as the map key to maintain individual connections
		flowGraph[edgeKey] = []string{from, to}

		tmpNode := FlowNode{
			FromType: fromType,
			FromID:   fromID,
			ToID:     toID,
			ToType:   toType,
			Content:  line,
		}

		p.FlowNodes = append(p.FlowNodes, tmpNode)
		p.BackUpFlowNodes = append(p.BackUpFlowNodes, tmpNode)
	}

	// check loop
	if err := p.detectCycle(); err != nil {
		return err
	}

	p.getPNS()

	// Check if all referenced components exist
	if err := p.validateComponentExistence(flowGraph); err != nil {
		return err
	}

	return nil
}

func getNodeToKey(node FlowNode) string {
	return node.ToType + "." + node.ToID
}

func getNodeFromKey(node FlowNode) string {
	return node.FromType + "." + node.FromID
}

// detectCycle detects if there are cycles in the data flow using DFS
func (p *Project) detectCycle() error {
	// Build adjacency list representation of the graph
	graph := make(map[string][]string)
	nodeLines := make(map[string]int) // Track line numbers for error reporting

	// Create a map to store line numbers for each flow node content
	contentLineMap := make(map[string]int)
	lines := strings.Split(p.Config.Content, "\n")
	actualLineNum := 0

	for i, line := range lines {
		actualLineNum = i + 1
		trimmedLine := strings.TrimSpace(line)

		// Skip empty lines and comment lines when building the map
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}

		// Store the actual line number for this content
		contentLineMap[trimmedLine] = actualLineNum
	}

	for _, node := range p.FlowNodes {
		fromKey := getNodeFromKey(node)
		toKey := getNodeToKey(node)

		graph[fromKey] = append(graph[fromKey], toKey)

		// Get the actual line number from our map
		if lineNum, exists := contentLineMap[node.Content]; exists {
			nodeLines[fromKey] = lineNum
			nodeLines[toKey] = lineNum
		} else {
			// Fallback: use a default line number if not found
			nodeLines[fromKey] = 1
			nodeLines[toKey] = 1
		}
	}

	// DFS states: 0=white (unvisited), 1=gray (visiting), 2=black (visited)
	state := make(map[string]int)
	var cyclePath []string

	// DFS function that detects cycles
	var dfs func(node string) bool
	dfs = func(node string) bool {
		state[node] = 1 // Mark as gray (currently visiting)
		cyclePath = append(cyclePath, node)

		for _, neighbor := range graph[node] {
			if state[neighbor] == 1 {
				// Found a back edge - cycle detected
				cyclePath = append(cyclePath, neighbor)
				return true
			}
			if state[neighbor] == 0 && dfs(neighbor) {
				// Cycle found in recursive call
				return true
			}
		}

		state[node] = 2                          // Mark as black (completely visited)
		cyclePath = cyclePath[:len(cyclePath)-1] // Remove from current path
		return false
	}

	// Check all nodes (handle disconnected components)
	for node := range graph {
		if state[node] == 0 {
			cyclePath = []string{}
			if dfs(node) {
				// Build cycle description
				cycleStr := strings.Join(cyclePath, " -> ")
				if lineNum, exists := nodeLines[cyclePath[0]]; exists {
					return fmt.Errorf("data flow cycle detected starting at line %d: %s", lineNum, cycleStr)
				}
				return fmt.Errorf("data flow cycle detected: %s", cycleStr)
			}
		}
	}

	return nil
}

func (p *Project) getPNS() {
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
		for _, conn := range p.FlowNodes {
			if getNodeToKey(conn) == component {
				upstreamComponent = getNodeFromKey(conn)
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

	// Process each connection and directly set PNS values
	for i := range p.FlowNodes {
		// For FROM component: build sequence independently
		fromKey := getNodeFromKey(p.FlowNodes[i])
		fromSequence := buildSequence(fromKey, make(map[string]bool))

		// For TO component: build sequence based on FROM component in THIS connection
		toKey := getNodeToKey(p.FlowNodes[i])
		toSequence := fromSequence + "." + toKey

		// Add project ID isolation for test mode to avoid polluting production environment
		if p.Testing {
			p.FlowNodes[i].FromPNS = fmt.Sprintf("TEST_%s_%s", p.Id, fromSequence)
			p.FlowNodes[i].ToPNS = fmt.Sprintf("TEST_%s_%s", p.Id, toSequence)
		} else {
			p.FlowNodes[i].FromPNS = fromSequence
			p.FlowNodes[i].ToPNS = toSequence
		}
	}
}

// parseNode splits "TYPE.name" into ("TYPE", "name")
func parseNode(s string) (string, string) {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) != 2 {
		return "", ""
	}

	componentType := strings.ToUpper(strings.TrimSpace(parts[0]))
	componentID := strings.TrimSpace(parts[1])

	// Validate component type
	if componentType != "INPUT" && componentType != "OUTPUT" && componentType != "RULESET" {
		return "", ""
	}

	// Validate component ID is not empty
	if componentID == "" {
		return "", ""
	}

	return componentType, componentID
}

// validateComponentExistence checks if all referenced components exist in the system
// and validates that the project content is not identical to existing projects
func (p *Project) validateComponentExistence(flowGraph map[string][]string) error {
	if len(p.FlowNodes) == 0 {
		return fmt.Errorf("project is empty, no flow nodes defined")
	}

	// Create a map to store line numbers for each flow node content
	contentLineMap := make(map[string]int)
	lines := strings.Split(p.Config.Content, "\n")
	actualLineNum := 0

	for i, line := range lines {
		actualLineNum = i + 1
		trimmedLine := strings.TrimSpace(line)

		// Skip empty lines and comment lines when building the map
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}

		// Store the actual line number for this content
		contentLineMap[trimmedLine] = actualLineNum
	}

	for _, node := range p.FlowNodes {
		// Get the line number from our map
		lineNum, exists := contentLineMap[node.Content]
		if !exists {
			// Fallback: try to find the line number by content matching
			lineNum = 0
			for i, line := range lines {
				if strings.TrimSpace(line) == node.Content {
					lineNum = i + 1
					break
				}
			}
		}

		if err := p.validateComponent(node.FromType, node.FromID, lineNum, "source"); err != nil {
			return err
		}

		if err := p.validateComponent(node.ToType, node.ToID, lineNum, "destination"); err != nil {
			return err
		}
	}

	// Skip PNS duplication check for testing projects
	if p.Testing {
		return nil
	}

	// Skip PNS duplication check if project ID is empty (validation mode)
	if strings.TrimSpace(p.Id) == "" {
		return nil
	}

	// Use safe iteration to check existing projects
	var duplicateProjectID string
	ForEachProject(func(existingProjectID string, existingProject *Project) bool {
		if existingProjectID == p.Id {
			return true
		}

		// Skip testing projects in PNS duplication check
		if existingProject.Testing {
			return true
		}

		if len(existingProject.FlowNodes) != len(p.FlowNodes) {
			return true
		}

		existingPNSMap := make(map[string]bool)
		for _, node := range existingProject.FlowNodes {
			existingPNSMap[node.FromPNS] = true
			existingPNSMap[node.ToPNS] = true
		}

		counter := 0
		for _, node := range p.FlowNodes {
			if existingPNSMap[node.FromPNS] {
				counter++
			}
			if existingPNSMap[node.ToPNS] {
				counter++
			}
		}

		if counter == len(p.FlowNodes)*2 {
			duplicateProjectID = existingProjectID
			return false // Stop iteration
		}
		return true
	})

	if duplicateProjectID != "" {
		return fmt.Errorf("project content is identical to existing project '%s': both projects have the same PNS structure", duplicateProjectID)
	}

	return nil
}

// validateComponent validates a single component exists in the system (unified approach)
func (p *Project) validateComponent(componentType, componentID string, lineNum int, position string) error {
	componentType = strings.ToUpper(componentType)

	// Check formal components using safe accessors
	exists, tempExists := ValidateComponent(componentType, componentID)

	if componentType != "INPUT" && componentType != "OUTPUT" && componentType != "RULESET" {
		return fmt.Errorf("unknown component type '%s' at line %d (%s)", componentType, lineNum, position)
	}

	if !exists {
		if tempExists {
			return fmt.Errorf("cannot reference temporary %s component '%s' at line %d (%s), please save it first", strings.ToLower(componentType), componentID, lineNum, position)
		}
		return fmt.Errorf("%s component '%s' not found at line %d (%s)", strings.ToLower(componentType), componentID, lineNum, position)
	}

	return nil
}

// Start starts the project and all its components
func (p *Project) Start(lock bool) error {
	if lock {
		common.ProjectOperationMu.Lock()
	}

	defer func() {
		if lock {
			common.ProjectOperationMu.Unlock()
		}
	}()

	err := p.parseContent()
	if err != nil {
		p.SetProjectStatus(common.StatusError, fmt.Errorf("project parse error: %s", err.Error()))
		return fmt.Errorf("project parse error: %s", err.Error())
	}

	// Add panic recovery for critical state changes
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Panic during project start", "project", p.Id, "panic", r)
			// Ensure cleanup and proper status setting on panic
			p.cleanup()
			p.SetProjectStatus(common.StatusError, fmt.Errorf("panic during start: %v", r))
		}
	}()

	// Atomic status check and transition
	if !p.atomicStatusTransition([]common.Status{common.StatusStopped, common.StatusError}, common.StatusStarting) {
		return fmt.Errorf("project is not in startable state, current status: %s", p.Status)
	}

	// Initialize or reset the stop channel for this start session
	p.stopOnce = sync.Once{}
	p.stopChan = make(chan struct{})

	err = p.initComponents()
	if err != nil {
		// Stop all components that may have been partially initialized
		_ = p.stopComponentsInternal()
		p.SetProjectStatus(common.StatusError, fmt.Errorf("failed to initialize components: %w", err))
		return fmt.Errorf("failed to initialize project components: %w", err)
	}

	err = p.runComponents()
	if err != nil {
		// Stop all components that were initialized and may have been started
		_ = p.stopComponentsInternal()
		p.SetProjectStatus(common.StatusError, fmt.Errorf("failed to run components: %w", err))
		return fmt.Errorf("failed to run project components: %w", err)
	}

	// All components started successfully, set project to running
	p.SetProjectStatus(common.StatusRunning, nil)

	logger.Info("Project started successfully", "project", p.Id)
	return nil
}

// Stop stops the project and all its components in proper order
func (p *Project) Stop(lock bool) error {
	// Use dedicated project operation lock to serialize all project lifecycle operations
	if lock {
		common.ProjectOperationMu.Lock()
	}

	defer func() {
		if lock {
			common.ProjectOperationMu.Unlock()
		}
	}()

	// Add panic recovery for critical state changes
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Panic during project stop", "project", p.Id, "panic", r)
			// Ensure cleanup and proper status setting on panic
			p.cleanup()
			p.SetProjectStatus(common.StatusError, fmt.Errorf("panic during stop: %v", r))
		}
	}()

	// Atomic status check and transition
	if !p.atomicStatusTransition([]common.Status{common.StatusRunning, common.StatusError}, common.StatusStopping) {
		return fmt.Errorf("project is not in stoppable state, current status: %s", p.Status)
	}

	// Signal all components to stop by closing the stop channel
	p.stopOnce.Do(func() {
		if p.stopChan != nil {
			close(p.stopChan)
		}
	})

	// Overall timeout for the entire stop process
	overallTimeout := time.After(2 * time.Minute)
	stopCompleted := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic during project stop goroutine", "project", p.Id, "panic", r)
				stopCompleted <- fmt.Errorf("panic during stop: %v", r)
			}
		}()

		// Use the internal stopComponents function
		err := p.stopComponentsInternal()
		stopCompleted <- err
	}()

	select {
	case err := <-stopCompleted:
		if err != nil {
			p.SetProjectStatus(common.StatusError, fmt.Errorf("failed to stop components: %w", err))
			return fmt.Errorf("failed to stop project components: %w", err)
		}
		p.SetProjectStatus(common.StatusStopped, nil)
		logger.Info("Project stopped successfully", "project", p.Id)
		return nil
	case <-overallTimeout:
		p.SetProjectStatus(common.StatusError, fmt.Errorf("stop operation timed out"))
		return fmt.Errorf("project stop operation timed out")
	}
}

func (p *Project) Restart(recordOperation bool, triggeredBy string) (err error) {
	// Cooldown mechanism to prevent rapid restarts
	p.restartMu.Lock()
	if time.Since(p.lastRestartTime) < 5*time.Second {
		p.restartMu.Unlock()
		logger.Info("Project restart skipped due to cooldown", "project", p.Id)
		return nil
	}
	p.lastRestartTime = time.Now()
	p.restartMu.Unlock()

	common.ProjectOperationMu.Lock()
	defer common.ProjectOperationMu.Unlock()

	logger.Info("Restarting project", "project", p.Id)

	// Defer the recording of the operation
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during restart: %v", r)
			logger.Error("Panic during project restart", "project", p.Id, "panic", r)
			// Ensure cleanup and proper status setting on panic
			_ = p.stopComponentsInternal()
			p.SetProjectStatus(common.StatusError, err)
		}

		if recordOperation {
			status := "success"
			errMsg := ""
			if err != nil {
				status = "failed"
				errMsg = err.Error()
			}
			details := map[string]interface{}{
				"node_id": common.GetNodeID(),
			}
			if triggeredBy != "" {
				details["triggered_by"] = triggeredBy
			}
			common.RecordProjectOperation(common.OpTypeProjectRestart, p.Id, status, errMsg, details)
		}
	}()

	// Check status - Stop() and Start() will handle their own locking via ProjectOperationMu
	if p.Status == common.StatusRunning || p.Status == common.StatusError {
		stopErr := p.Stop(false)
		if stopErr != nil {
			err = fmt.Errorf("failed to stop project during restart: %w", stopErr)
			return err
		}
	}

	// Start the project again with retry mechanism
	err = p.startWithRetry(false)
	if err != nil {
		err = fmt.Errorf("failed to start project after restart (exhausted all retries): %w", err)
		return err
	}

	logger.Info("Project restarted successfully", "project", p.Id)
	return nil
}

// startWithRetry starts the project with retry mechanism for components that fail to reach running status
func (p *Project) startWithRetry(recordOperation bool) error {
	maxRetries := 3
	retryDelays := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		logger.Info("Starting project", "project", p.Id, "attempt", attempt+1, "max_attempts", maxRetries+1)

		// Attempt to start the project
		err := p.Start(recordOperation)
		if err != nil {
			if attempt == maxRetries {
				logger.Error("Failed to start project after all retry attempts", "project", p.Id, "final_error", err)
				p.SetProjectStatus(common.StatusError, fmt.Errorf("failed to start project after %d attempts: %w", maxRetries+1, err))
				return fmt.Errorf("failed to start project after %d attempts: %w", maxRetries+1, err)
			}

			logger.Warn("Project start failed, will retry", "project", p.Id, "attempt", attempt+1, "error", err, "retry_delay", retryDelays[attempt])
			time.Sleep(retryDelays[attempt])
			continue
		}

		// Start succeeded, now check if all components are actually running
		if p.areAllComponentsRunning() {
			logger.Info("Project started successfully with all components running", "project", p.Id, "attempt", attempt+1)
			return nil
		}

		// Some components are not running, retry if we have attempts left
		if attempt == maxRetries {
			err := fmt.Errorf("project started but some components are not in running state after %d attempts", maxRetries+1)
			p.SetProjectStatus(common.StatusError, err)
			return err
		}

		logger.Warn("Project started but some components are not running, will retry", "project", p.Id, "attempt", attempt+1, "retry_delay", retryDelays[attempt])

		// Stop the project before retrying
		_ = p.Stop(false)
		time.Sleep(retryDelays[attempt])
	}

	return fmt.Errorf("unexpected end of retry loop")
}

// areAllComponentsRunning checks if all project components are in running state
func (p *Project) areAllComponentsRunning() bool {
	// Check input components
	inputs := p.GetProjectInputs()
	for _, in := range inputs {
		if in.Status != common.StatusRunning {
			logger.Warn("Input component not running", "project", p.Id, "input", in.Id, "status", in.Status)
			return false
		}
	}

	// Check output components
	outputs := p.GetProjectOutputs()
	for _, out := range outputs {
		if out.Status != common.StatusRunning {
			logger.Warn("Output component not running", "project", p.Id, "output", out.Id, "status", out.Status)
			return false
		}
	}

	// Check ruleset components
	rulesets := p.GetProjectRulesets()
	for _, rs := range rulesets {
		if rs.Status != common.StatusRunning {
			logger.Warn("Ruleset component not running", "project", p.Id, "ruleset", rs.RulesetID, "status", rs.Status)
			return false
		}
	}

	return true
}

func (p *Project) getPartner(t string, pns string) []string {
	res := make([]string, 0)
	for _, node := range p.FlowNodes {
		if t == "right" && node.FromPNS == pns {
			res = append(res, node.ToPNS)
		}

		if t == "left" && node.ToPNS == pns {
			res = append(res, node.FromPNS)
		}
	}
	return res
}

func (p *Project) stopComponentsInternal() error {
	return p.stopComponentsInternalWithTimeout(45 * time.Second) // Leave 75 seconds for overall timeout margin
}

func (p *Project) stopComponentsInternalWithTimeout(dataProcessingTimeout time.Duration) error {
	var stopErrors []error
	logger.Info("Step 1: Disconnecting inputs from downstream", "project", p.Id)
	p.disconnectInputsFromDownstream()

	logger.Info("Step 2: Waiting for data to be fully processed through pipeline", "project", p.Id, "timeout", dataProcessingTimeout)
	p.waitForCompleteDataProcessingWithTimeout(dataProcessingTimeout)

	logger.Info("Step 3: Stopping input components", "project", p.Id)
	inputErrors := p.stopInputComponents()
	if len(inputErrors) > 0 {
		stopErrors = append(stopErrors, inputErrors...)
	}

	if !p.Testing {
		common.GlobalDailyStatsManager.CollectAllComponentsData()
	}

	rulesets := p.GetProjectRulesets()
	logger.Info("Step 4: Stopping rulesets", "project", p.Id, "count", len(rulesets))
	for id, rs := range rulesets {
		DeletePNSRuleset(id)
		if CalculateRefCount(id, p.Id) == 0 {
			stopErr := rs.Stop()
			if stopErr != nil {
				logger.Error("Failed to stop ruleset", "project", p.Id, "ruleset", rs.RulesetID, "error", stopErr)
				stopErrors = append(stopErrors, fmt.Errorf("ruleset %s: %w", rs.RulesetID, stopErr))
			} else {
				logger.Info("Stopped ruleset", "project", p.Id, "ruleset", rs.RulesetID)
			}
		}
	}

	outputs := p.GetProjectOutputs()
	logger.Info("Step 5: Stopping outputs", "project", p.Id, "count", len(outputs))
	for id, out := range outputs {
		DeletePNSOutput(id)
		if CalculateRefCount(id, p.Id) == 0 {
			var stopErr error
			if p.Testing {
				stopErr = out.StopForTesting()
			} else {
				stopErr = out.Stop()
			}
			if stopErr != nil {
				logger.Error("Failed to stop output", "project", p.Id, "output", out.Id, "error", stopErr)
				stopErrors = append(stopErrors, fmt.Errorf("output %s: %w", out.Id, stopErr))
			} else {
				logger.Info("Stopped output", "project", p.Id, "output", out.Id, "sequence", out.ProjectNodeSequence)
			}
		}
	}

	p.cleanup()
	logger.Info("Finished stopping project components", "project", p.Id)

	// Return aggregated errors if any
	if len(stopErrors) > 0 {
		var errorMessages []string
		for _, err := range stopErrors {
			errorMessages = append(errorMessages, err.Error())
		}
		return fmt.Errorf("failed to stop some components: %s", strings.Join(errorMessages, "; "))
	}

	return nil
}

func (p *Project) CheckExist(t string, id string) bool {
	for _, node := range p.BackUpFlowNodes {
		if node.ToType == t && node.ToID == id {
			return true
		}

		if node.FromType == t && node.FromID == id {
			return true
		}
	}
	return false
}

// cleanup performs aggressive cleanup when normal stop fails
func (p *Project) cleanup() {
	p.cleanupInputChannel()
	p.cleanupRulesetChannel()

	for pns, ch := range p.MsgChannels {
		if ch != nil {
			// Safely close channel, ignore if already closed
			func(channel *chan map[string]interface{}, channelName string) {
				defer func() {
					if r := recover(); r != nil {
						logger.Debug("Channel already closed during cleanup", "project", p.Id, "pns", channelName)
					}
				}()
				close(*channel)
			}(ch, pns)
		}
	}

	p.BackUpFlowNodes = make([]FlowNode, len(p.FlowNodes))
	for i := range p.FlowNodes {
		p.BackUpFlowNodes[i] = p.FlowNodes[i]

		// Reset initialization flags without reducing ref count
		// Ref count reduction is handled in stopComponentsInternal
		if p.FlowNodes[i].FromInit {
			p.FlowNodes[i].FromInit = false
		}

		if p.FlowNodes[i].ToInit {
			p.FlowNodes[i].ToInit = false
		}
	}

	p.FlowNodes = []FlowNode{}
	p.Inputs = make(map[string]*input.Input)
	p.Outputs = make(map[string]*output.Output)
	p.Rulesets = make(map[string]*rules_engine.Ruleset)
	p.MsgChannels = make(map[string]*chan map[string]interface{}, 0)

	// Reset stop channel state for next start/stop cycle
	p.stopOnce = sync.Once{}
	p.stopChan = nil
}

// disconnectInputsFromDownstream safely disconnects all input components from their downstream channels
// This should be called before waiting for data processing to complete
func (p *Project) disconnectInputsFromDownstream() {
	inputs := p.GetProjectInputs()
	for id, in := range inputs {
		rightNodes := p.getPartner("right", id)

		for _, downstreamID := range rightNodes {
			// Use the safe deletion function to properly clean up downstream connections
			SafeDeleteInputDownstream(in.Id, downstreamID)
			logger.Debug("Disconnected input from downstream",
				"project", p.Id, "input", in.Id, "downstream", downstreamID)
		}
	}
	logger.Info("All inputs disconnected from downstream", "project", p.Id)
}

// stopInputComponents stops all input components used by this project
// This should be called after data processing is complete
// Returns a list of errors encountered during stopping, but continues to stop all components
func (p *Project) stopInputComponents() []error {
	var stopErrors []error
	inputs := p.GetProjectInputs()
	for id, in := range inputs {
		// Input components need reference counting to determine if they should be stopped
		// Only stop when no other projects are using this input (excluding current project)
		if CalculateRefCount(id, p.Id) == 0 {
			// Use safe accessor to get global input
			globalInput, exists := GetInput(in.Id)

			if exists {
				var err error
				if p.Testing {
					err = globalInput.StopForTesting()
					if err != nil {
						logger.Error("Failed to stop test input", "project", p.Id, "input", in.Id, "error", err)
						stopErrors = append(stopErrors, fmt.Errorf("test input %s: %w", in.Id, err))
					} else {
						logger.Info("Stopped test input", "project", p.Id, "input", in.Id)
					}
				} else {
					err = globalInput.Stop()
					if err != nil {
						logger.Error("Failed to stop input", "project", p.Id, "input", in.Id, "error", err)
						stopErrors = append(stopErrors, fmt.Errorf("input %s: %w", in.Id, err))
					} else {
						logger.Info("Stopped input", "project", p.Id, "input", in.Id)
					}
				}
			} else {
				logger.Warn("Global input not found during stop", "project", p.Id, "input", in.Id)
				stopErrors = append(stopErrors, fmt.Errorf("input %s: global input not found", in.Id))
			}
		} else {
			logger.Debug("Input still in use by other projects, not stopping",
				"project", p.Id, "input", in.Id, "ref_count", CalculateRefCount(id, p.Id))
		}
	}
	return stopErrors
}

// waitForCompleteDataProcessing waits for all data to be fully processed through the pipeline
// This includes waiting for channels to empty AND thread pools to complete all tasks
func (p *Project) waitForCompleteDataProcessing() {
	p.waitForCompleteDataProcessingWithTimeout(60 * time.Second)
}

// waitForCompleteDataProcessingWithTimeout waits for all data to be fully processed with a given timeout
func (p *Project) waitForCompleteDataProcessingWithTimeout(timeout time.Duration) {
	overallTimeout := time.After(timeout)
	checkInterval := 100 * time.Millisecond
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	logger.Info("Waiting for complete data processing", "project", p.Id, "timeout", timeout)

	for {
		select {
		case <-overallTimeout:
			logger.Info("Data processing wait timeout reached", "project", p.Id)
			return
		case <-ticker.C:
			allProcessed := true

			// Check all channels for remaining messages
			channelCount := 0
			messagesRemaining := 0
			for _, ch := range p.MsgChannels {
				channelCount++
				chLen := len(*ch)
				if chLen > 0 {
					messagesRemaining += chLen
					allProcessed = false
				}
			}

			if !allProcessed {
				logger.Debug("Still processing channel messages",
					"project", p.Id,
					"channels", channelCount,
					"pending_messages", messagesRemaining)
				continue
			}

			// Check ruleset running tasks
			rulesets := p.GetProjectRulesets()
			totalRunningTasks := 0
			for _, rs := range rulesets {
				if CalculateRefCount(rs.ProjectNodeSequence, p.Id) == 0 {
					runningTasks := rs.GetRunningTaskCount()
					if runningTasks > 0 {
						totalRunningTasks += runningTasks
						allProcessed = false
					}
				}
			}

			if !allProcessed {
				logger.Debug("Still processing ruleset tasks",
					"project", p.Id,
					"running_tasks", totalRunningTasks)
				continue
			}

			if allProcessed {
				logger.Info("All data processing completed", "project", p.Id)
				// Simple final grace period
				time.Sleep(1 * time.Second)
				return
			}
		}
	}
}

func (p *Project) cleanupInputChannel() {
	// This method is called during cleanup phase to ensure any remaining connections are cleaned up
	// The actual disconnection should already be done in disconnectInputsFromDownstream
	inputs := p.GetProjectInputs()
	for id, in := range inputs {
		rightNodes := p.getPartner("right", id)

		for _, id2 := range rightNodes {
			SafeDeleteInputDownstream(in.Id, id2)
		}
	}
	logger.Debug("Input channel cleanup completed", "project", p.Id)
}

func (p *Project) cleanupRulesetChannel() {
	for i := range p.FlowNodes {
		node := &p.FlowNodes[i]

		if node.FromType == "RULESET" {
			if CalculateRefCount(node.FromPNS, p.Id) > 0 {
				if r, exist := GetRuleset(node.FromPNS); exist {
					delete(r.DownStream, node.ToPNS)
				}
			}
		}
	}
}

func (p *Project) initComponents() error {
	// Track which nodes need new channels created
	nodeChannelStatus := make(map[string]bool) // key: ToPNS, value: whether channel was created

	// Cleanup function to remove any partially initialized components on error
	cleanup := func() {
		logger.Info("Cleaning up partially initialized components due to error", "project", p.Id)

		// Clean up created PNS rulesets
		for pns := range p.Rulesets {
			DeletePNSRuleset(pns)
		}

		// Clean up created PNS outputs (only if not in testing mode)
		if !p.Testing {
			for pns := range p.Outputs {
				DeletePNSOutput(pns)
			}
		}

		// Clear project maps
		p.Inputs = make(map[string]*input.Input)
		p.Outputs = make(map[string]*output.Output)
		p.Rulesets = make(map[string]*rules_engine.Ruleset)
		p.MsgChannels = make(map[string]*chan map[string]interface{}, 0)

		// Reset node initialization flags
		for i := range p.FlowNodes {
			p.FlowNodes[i].FromInit = false
			p.FlowNodes[i].ToInit = false
		}
	}

	// Phase 1: Initialize all TO components and create channels
	for i := range p.FlowNodes {
		node := &p.FlowNodes[i]
		switch node.ToType {
		case "RULESET":
			// Use safe accessor to check PNS rulesets
			rs, exists := GetPNSRuleset(node.ToPNS)

			if exists {
				p.Rulesets[node.ToPNS] = rs
				nodeChannelStatus[node.ToPNS] = false
			} else {
				// Get the original ruleset using safe accessor
				originalRuleset, exists := GetRuleset(node.ToID)

				if !exists {
					cleanup()
					return fmt.Errorf("ruleset component not found: %s", node.ToID)
				}

				rs, err := rules_engine.NewFromExisting(originalRuleset, node.ToPNS)
				if err != nil {
					// Set the original ruleset to error state
					if originalRuleset != nil {
						originalRuleset.SetStatus(common.StatusError, fmt.Errorf("failed to create PNS instance: %w", err))
					}
					cleanup()
					return fmt.Errorf("failed to create ruleset from existing: %s %w", node.ToPNS, err)
				}

				// Use safe accessor to set PNS ruleset
				SetPNSRuleset(node.ToPNS, rs)

				p.Rulesets[node.ToPNS] = rs

				nodeChannelStatus[node.ToPNS] = true
				c := make(chan map[string]interface{}, 512)
				p.MsgChannels[node.ToPNS] = &c
				rs.UpStream[node.ToPNS] = &c
			}
		case "OUTPUT":
			if p.Testing {
				// In testing mode, create a test version of the output component
				// This avoids sending data to real external systems
				originalOutput, ok := GetOutput(node.ToID)

				if !ok {
					cleanup()
					return fmt.Errorf("output component not found for testing: %s", node.ToID)
				}

				// Create a new output instance for testing based on the original config
				testOutput, err := output.NewFromExisting(originalOutput, node.ToPNS)
				if err != nil {
					// Set the original output to error state
					if originalOutput != nil {
						originalOutput.SetStatus(common.StatusError, fmt.Errorf("failed to create test instance: %w", err))
					}
					cleanup()
					return fmt.Errorf("failed to create test output component: %s %w", node.ToPNS, err)
				}

				// Set test-specific properties to avoid pollution
				testOutput.SetTestMode() // Disable sampling and global state interactions

				p.Outputs[node.ToPNS] = testOutput

				nodeChannelStatus[node.ToPNS] = true
				c := make(chan map[string]interface{}, 1024)
				p.MsgChannels[node.ToPNS] = &c
				testOutput.UpStream[node.ToPNS] = &c
			} else {
				// Production mode: use shared PNS output or create new one
				o, exists := GetPNSOutput(node.ToPNS)

				if exists {
					p.Outputs[node.ToPNS] = o
					nodeChannelStatus[node.ToPNS] = false
				} else {
					// Get the original output using safe accessor
					originalOutput, exists := GetOutput(node.ToID)

					if !exists {
						cleanup()
						return fmt.Errorf("output component not found: %s", node.ToID)
					}

					o, err := output.NewFromExisting(originalOutput, node.ToPNS)
					if err != nil {
						// Set the original output to error state
						if originalOutput != nil {
							originalOutput.SetStatus(common.StatusError, fmt.Errorf("failed to create PNS instance: %w", err))
						}
						cleanup()
						return fmt.Errorf("failed to create output from existing: %s %w", node.ToPNS, err)
					}

					// Use safe accessor to set PNS output
					SetPNSOutput(node.ToPNS, o)

					p.Outputs[node.ToPNS] = o

					nodeChannelStatus[node.ToPNS] = true
					c := make(chan map[string]interface{}, 1024)
					p.MsgChannels[node.ToPNS] = &c
					o.UpStream[node.ToPNS] = &c
				}
			}
		}
		node.ToInit = true
	}

	// Phase 2: Initialize all FROM components
	for i := range p.FlowNodes {
		node := &p.FlowNodes[i]
		switch node.FromType {
		case "RULESET":
			// Use safe accessor to check PNS rulesets
			rs, exists := GetPNSRuleset(node.FromPNS)

			if exists {
				p.Rulesets[node.FromPNS] = rs
			} else {
				// Get the original ruleset using safe accessor
				originalRuleset, exists := GetRuleset(node.FromID)

				if !exists {
					cleanup()
					return fmt.Errorf("ruleset component not found: %s", node.FromID)
				}

				rs, err := rules_engine.NewFromExisting(originalRuleset, node.FromPNS)
				if err != nil {
					// Set the original ruleset to error state
					if originalRuleset != nil {
						originalRuleset.SetStatus(common.StatusError, fmt.Errorf("failed to create PNS instance: %w", err))
					}
					cleanup()
					return fmt.Errorf("failed to create ruleset from existing: %s %w", node.FromPNS, err)
				}

				// Use safe accessor to set PNS ruleset
				SetPNSRuleset(node.FromPNS, rs)

				p.Rulesets[node.FromPNS] = rs
			}
		case "INPUT":
			if p.Testing {
				// In testing mode, create a test version of the input component
				// This avoids connecting to real external data sources
				originalInput, ok := GetInput(node.FromID)

				if !ok {
					cleanup()
					return fmt.Errorf("input component not found for testing: %s", node.FromID)
				}

				// Create a new input instance for testing based on the original config
				testInput, err := input.NewFromExisting(originalInput, node.FromPNS)
				if err != nil {
					// Set the original input to error state
					if originalInput != nil {
						originalInput.SetStatus(common.StatusError, fmt.Errorf("failed to create test instance: %w", err))
					}
					cleanup()
					return fmt.Errorf("failed to create test input component: %s %w", node.FromPNS, err)
				}

				// Set test-specific properties to avoid pollution
				testInput.SetTestMode() // Disable sampling and global state interactions

				p.Inputs[node.FromPNS] = testInput
			} else {
				// Production mode: create input instance with correct ProjectNodeSequence
				originalInput, exists := GetInput(node.FromID)
				if !exists {
					cleanup()
					return fmt.Errorf("input component not found: %s", node.FromID)
				}
				p.Inputs[node.FromPNS] = originalInput
			}
		}
		node.FromInit = true
	}

	// Phase 3: Establish all connections after all components are initialized
	for i := range p.FlowNodes {
		node := &p.FlowNodes[i]

		// Establish connections from FROM components to TO components
		switch node.FromType {
		case "RULESET":
			if fromRs, exists := p.Rulesets[node.FromPNS]; exists {
				// Always try to establish connection regardless of channel creation status
				// This ensures shared PNS components get properly connected
				if toChannel, channelExists := p.MsgChannels[node.ToPNS]; channelExists {
					fromRs.DownStream[node.ToPNS] = toChannel
				} else {
					// If no local channel, try to find existing channel in shared PNS component
					if node.ToType == "OUTPUT" {
						if sharedOutput, exists := GetPNSOutput(node.ToPNS); exists {
							if sharedChannel, exists := sharedOutput.UpStream[node.ToPNS]; exists {
								fromRs.DownStream[node.ToPNS] = sharedChannel
							}
						}
					} else if node.ToType == "RULESET" {
						if sharedRuleset, exists := GetPNSRuleset(node.ToPNS); exists {
							if sharedChannel, exists := sharedRuleset.UpStream[node.ToPNS]; exists {
								fromRs.DownStream[node.ToPNS] = sharedChannel
							}
						}
					}
				}
			}
		case "INPUT":
			if fromInput, exists := p.Inputs[node.FromPNS]; exists {
				// Always try to establish connection
				if toChannel, channelExists := p.MsgChannels[node.ToPNS]; channelExists {
					fromInput.DownStream[node.ToPNS] = toChannel
					logger.Info("Input downstream connection established",
						"project", p.Id,
						"input", fromInput.Id,
						"from_pns", node.FromPNS,
						"to_pns", node.ToPNS,
						"to_type", node.ToType)
				} else {
					// If no local channel, try to find existing channel in shared PNS component
					if node.ToType == "OUTPUT" {
						if sharedOutput, exists := GetPNSOutput(node.ToPNS); exists {
							if sharedChannel, exists := sharedOutput.UpStream[node.ToPNS]; exists {
								fromInput.DownStream[node.ToPNS] = sharedChannel
								logger.Info("Input downstream connection established to shared output",
									"project", p.Id,
									"input", fromInput.Id,
									"from_pns", node.FromPNS,
									"to_pns", node.ToPNS)
							}
						}
					} else if node.ToType == "RULESET" {
						if sharedRuleset, exists := GetPNSRuleset(node.ToPNS); exists {
							if sharedChannel, exists := sharedRuleset.UpStream[node.ToPNS]; exists {
								fromInput.DownStream[node.ToPNS] = sharedChannel
								logger.Info("Input downstream connection established to shared ruleset",
									"project", p.Id,
									"input", fromInput.Id,
									"from_pns", node.FromPNS,
									"to_pns", node.ToPNS)
							}
						}
					}
				}
			} else {
				logger.Warn("Input component not found for connection",
					"project", p.Id,
					"from_pns", node.FromPNS,
					"from_id", node.FromID)
			}
		}
	}

	logger.Info("Components initialized successfully", "project", p.Id,
		"inputs", len(p.Inputs),
		"outputs", len(p.Outputs),
		"rulesets", len(p.Rulesets))

	return nil
}

func (p *Project) runComponents() error {
	// Start components in reverse dependency order: outputs -> rulesets -> inputs
	// This ensures downstream components are ready before upstream starts producing data

	// Track started components for cleanup on failure
	var startedOutputs []*output.Output
	var startedRulesets []*rules_engine.Ruleset
	var startedInputs []*input.Input

	// Cleanup function to stop all started components on error
	cleanup := func() {
		logger.Info("Cleaning up started components due to error", "project", p.Id)

		// Stop inputs first (stop data flow)
		for _, in := range startedInputs {
			if p.Testing {
				_ = in.StopForTesting()
			} else {
				_ = in.Stop()
			}
		}

		// Stop rulesets
		for _, rs := range startedRulesets {
			_ = rs.Stop()
		}

		// Stop outputs last
		for _, out := range startedOutputs {
			if p.Testing {
				_ = out.StopForTesting()
			} else {
				_ = out.Stop()
			}
		}
	}

	// 1. Start output components first (they need to be ready to receive data)
	outputs := p.GetProjectOutputs()
	for _, out := range outputs {
		var err error
		if p.Testing {
			// In testing mode, use StartForTesting to avoid external connectivity checks
			err = out.StartForTesting()
		} else {
			// Production mode: normal start
			err = out.Start()
		}

		if err != nil {
			cleanup() // Stop all previously started components
			return fmt.Errorf("failed to start output component %s: %w", out.Id, err)
		}
		startedOutputs = append(startedOutputs, out)
	}

	// 2. Start ruleset components (middle components in the pipeline)
	rulesets := p.GetProjectRulesets()
	for _, rs := range rulesets {
		err := rs.Start()
		if err != nil {
			cleanup() // Stop all previously started components
			return fmt.Errorf("failed to start ruleset component %s: %w", rs.RulesetID, err)
		}
		startedRulesets = append(startedRulesets, rs)
	}

	// 3. Start input components last (they will begin producing data immediately)
	inputs := p.GetProjectInputs()
	for _, in := range inputs {
		var err error
		if p.Testing {
			// In testing mode, use StartForTesting to avoid connecting to external data sources
			err = in.StartForTesting()
		} else {
			// Production mode: normal start
			err = in.Start()
		}

		if err != nil {
			cleanup() // Stop all previously started components
			return fmt.Errorf("failed to start input component %s: %w", in.Id, err)
		}
		startedInputs = append(startedInputs, in)
	}

	logger.Info("All components started successfully", "project", p.Id,
		"outputs", len(startedOutputs),
		"rulesets", len(startedRulesets),
		"inputs", len(startedInputs))

	return nil
}

// updateProjectStatusRedis writes status to Redis hash and publishes event with error handling
func updateProjectStatusRedis(projectID string, status common.Status, t time.Time) {
	nodeid := common.GetNodeID()

	if err := common.SetProjectRealState(common.GetNodeID(), projectID, string(status)); err != nil {
		logger.Error("Failed to update project real state in Redis", "node_id", nodeid, "project_id", projectID, "status", status, "error", err)
		return
	}

	// Set timestamp
	if err := common.SetProjectStateTimestamp(nodeid, projectID, t); err != nil {
		logger.Error("Failed to update project state timestamp in Redis", "node_id", nodeid, "project_id", projectID, "error", err)
	}

	evt := map[string]interface{}{
		"node_id":           nodeid,
		"project_id":        projectID,
		"status":            string(status),
		"status_changed_at": t.Format(time.RFC3339),
	}

	data, _ := json.Marshal(evt)
	if err := common.RedisPublishWithRetry("cluster:proj_status", string(data)); err != nil {
		logger.Error("Failed to publish project status after retries", "node_id", nodeid, "project_id", projectID, "error", err)
		return
	}
}

func (p *Project) SetProjectStatus(status common.Status, err error) {
	if err != nil {
		p.Err = err
	}
	p.Status = status
	t := time.Now()
	p.StatusChangedAt = &t
	updateProjectStatusRedis(p.Id, status, t)
}
