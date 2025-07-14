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
	"strings"
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

	//ProjectStatusRunning
	common.GlobalMu.RLock()
	for _, proj := range GlobalProject.Projects {
		if proj.Status == common.StatusRunning {
			runningProjects = append(runningProjects, proj)
		}
	}
	common.GlobalMu.RUnlock()

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

// GetAffectedProjects returns the list of project IDs affected by component changes
func GetAffectedProjects(componentType string, componentID string) []string {
	affectedProjects := make(map[string]struct{})

	common.GlobalMu.RLock()
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
		affectedProjects[componentID] = struct{}{}
	}
	common.GlobalMu.RUnlock()

	// Convert to string slice
	result := make([]string, 0, len(affectedProjects))
	for projectID := range affectedProjects {
		result = append(result, projectID)
	}

	return result
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
			newProj, err := NewProject("", projectConfig, projectID, false)
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
		if proj.Status == common.StatusRunning {
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
		if proj.Status == common.StatusStopped {
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
		if proj.Status == common.StatusRunning {
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

	GlobalProject.PNSOutputs = make(map[string]*output.Output)
	GlobalProject.PNSRulesets = make(map[string]*rules_engine.Ruleset)

	GlobalProject.ProjectsNew = make(map[string]string)
	GlobalProject.InputsNew = make(map[string]string)
	GlobalProject.OutputsNew = make(map[string]string)
	GlobalProject.RulesetsNew = make(map[string]string)
	GlobalProject.RefCount = make(map[string]int)

	common.AllProjectRawConfig = make(map[string]string)
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
		Status: common.StatusStopped,
		Config: &cfg,
	}

	err = p.parseContent()
	if err != nil {
		return fmt.Errorf("failed to parse project content: %v", err)
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
		p.SetProjectStatus(common.StatusStopped, err)
		return p, fmt.Errorf("failed to initialize project components: %w", err)
	}

	common.GlobalMu.Lock()
	GlobalProject.Projects[p.Id] = p
	common.AllProjectRawConfig[p.Id] = p.Config.RawConfig
	common.GlobalMu.Unlock()

	// Store project config in Redis for cluster-wide access
	if err := common.StoreProjectConfig(p.Id, p.Config.RawConfig); err != nil {
		logger.Error("Failed to store project config in Redis", "project", p.Id, "error", err)
	}

	logger.Info("Project created successfully", "project", p.Id)
	return p, nil
}

// parseContent parses the project content to build the data flow graph
func (p *Project) parseContent() error {
	flowGraph := make(map[string][]string)
	lines := strings.Split(p.Config.Content, "\n")
	edgeSet := make(map[string]struct{}) // Used to detect duplicate flows

	p.FlowNodes = []FlowNode{}

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
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
				return fmt.Errorf("invalid arrow format at line %d: use '->' instead of '=>' in %q", lineNum+1, line)
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
			return fmt.Errorf("invalid node format at line %d: %s -> %s", lineNum+1, from, to)
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

	for i, node := range p.FlowNodes {
		fromKey := getNodeFromKey(node)
		toKey := getNodeToKey(node)

		graph[fromKey] = append(graph[fromKey], toKey)

		// Store line numbers for error reporting (assuming 1-based line numbers)
		nodeLines[fromKey] = i + 1
		nodeLines[toKey] = i + 1
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
	return strings.ToUpper(strings.TrimSpace(parts[0])), strings.TrimSpace(parts[1])
}

// validateComponentExistence checks if all referenced components exist in the system
// and validates that the project content is not identical to existing projects
func (p *Project) validateComponentExistence(flowGraph map[string][]string) error {
	if len(p.FlowNodes) == 0 {
		return fmt.Errorf("project is empty, no flow nodes defined")
	}

	for _, node := range p.FlowNodes {
		lineNum := 0
		for i, line := range strings.Split(p.Config.Content, "\n") {
			if strings.TrimSpace(line) == node.Content {
				lineNum = i + 1
				break
			}
		}

		if err := p.validateComponent(node.FromType, node.FromID, lineNum, "source"); err != nil {
			return err
		}

		if err := p.validateComponent(node.ToType, node.ToID, lineNum, "destination"); err != nil {
			return err
		}
	}

	for existingProjectID, existingProject := range GlobalProject.Projects {
		if existingProjectID == p.Id {
			continue
		}

		if len(existingProject.FlowNodes) != len(p.FlowNodes) {
			continue
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
			return fmt.Errorf("project content is identical to existing project '%s': both projects have the same PNS structure", existingProjectID)
		}
	}

	return nil
}

// validateComponent validates a single component exists in the system (unified approach)
func (p *Project) validateComponent(componentType, componentID string, lineNum int, position string) error {
	componentType = strings.ToUpper(componentType)

	// Check formal components
	var exists bool
	var tempExists bool

	switch componentType {
	case "INPUT":
		_, exists = GlobalProject.Inputs[componentID]
		_, tempExists = GlobalProject.InputsNew[componentID]
	case "OUTPUT":
		_, exists = GlobalProject.Outputs[componentID]
		_, tempExists = GlobalProject.OutputsNew[componentID]
	case "RULESET":
		_, exists = GlobalProject.Rulesets[componentID]
		_, tempExists = GlobalProject.RulesetsNew[componentID]
	default:
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
func (p *Project) Start() error {
	if p.Status != common.StatusStopped {
		return fmt.Errorf("project is not stopped %s", p.Id)
	}

	p.SetProjectStatus(common.StatusStarting, nil)

	err := p.initComponents()
	if err != nil {
		p.SetProjectStatus(common.StatusStopped, fmt.Errorf("failed to initialize components: %w", err))
		p.cleanup()
		return fmt.Errorf("failed to initialize project components: %w", err)
	}

	err = p.runComponents()
	if err != nil {
		p.SetProjectStatus(common.StatusStopped, fmt.Errorf("failed to run components: %w", err))
		p.cleanup()
		return fmt.Errorf("failed to run project components: %w", err)
	}

	if p.Testing {
		p.Status = common.StatusRunning
	} else {
		p.SetProjectStatus(common.StatusRunning, nil)
	}

	logger.Info("Project started successfully", "project", p.Id)
	return nil
}

// Stop stops the project and all its components in proper order
func (p *Project) Stop() error {
	if p.Status != common.StatusRunning {
		return fmt.Errorf("project is not running %s", p.Id)
	}

	// Set status to stopping immediately to prevent duplicate operations
	p.SetProjectStatus(common.StatusStopping, nil)

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
		p.cleanup()
		p.SetProjectStatus(common.StatusStopped, nil)
		logger.Info("Project stopped successfully", "project", p.Id)
		return nil
	case <-overallTimeout:
		p.cleanup()
		p.SetProjectStatus(common.StatusStopped, nil)
		return fmt.Errorf("project stop timeout exceeded, forced cleanup completed for %s", p.Id)
	}
}

func (p *Project) Restart() error {
	if p.Status != common.StatusRunning {
		return fmt.Errorf("project is not running %s", p.Id)
	}

	logger.Info("Restarting project", "project", p.Id)

	// Stop the project first
	err := p.Stop()
	if err != nil {
		return fmt.Errorf("failed to stop project for restart: %w", err)
	}

	// Start the project again
	err = p.Start()
	if err != nil {
		return fmt.Errorf("failed to start project after restart: %w", err)
	}

	logger.Info("Project restarted successfully", "project", p.Id)
	return nil
}

func (p *Project) getPartner(t string, pns string) []string {
	res := make([]string, 0)
	for _, node := range p.FlowNodes {
		if t == "rignt" && node.FromPNS == pns {
			res = append(res, node.ToPNS)
		}

		if t == "left" && node.ToPNS == pns {
			res = append(res, node.ToPNS)
		}
	}
	return res
}

func (p *Project) stopComponentsInternal() error {
	logger.Info("Step 1: Stopping inputs to prevent new data", "project", p.Id, "count", len(p.Inputs))
	for id, in := range p.Inputs {
		rightNodes := p.getPartner("right", in.ProjectNodeSequence)

		for _, id2 := range rightNodes {
			common.GlobalMu.Lock()
			delete(GlobalProject.Inputs[in.Id].DownStream, id2)
			common.GlobalMu.Unlock()
		}

		if GetRefCount(id) == 1 {
			err := GlobalProject.Inputs[in.Id].Stop()
			if err != nil {
				logger.Error("Failed to stop input", "project", p.Id, "input", in.Id, "error", err)
			}
		}
	}

	logger.Info("Step 2: Waiting for data to be fully processed through pipeline", "project", p.Id)
	p.waitForCompleteDataProcessing()

	common.GlobalDailyStatsManager.CollectAllComponentsData()

	logger.Info("Step 3: Stopping rulesets", "project", p.Id, "count", len(p.Rulesets))
	for id, rs := range p.Rulesets {
		if GetRefCount(id) == 1 {
			err := rs.Stop()
			if err != nil {
				logger.Error("Failed to stop ruleset", "project", p.Id, "ruleset", rs.RulesetID, "error", err)
			} else {
				logger.Info("Stopped ruleset", "project", p.Id, "ruleset", rs.RulesetID, "sequence", id)
			}
		}
	}

	logger.Info("Step 4: Stopping outputs", "project", p.Id, "count", len(p.Outputs))
	for id, out := range p.Outputs {
		if GetRefCount(id) == 1 {
			err := out.Stop()
			if err != nil {
				logger.Error("Failed to stop output", "project", p.Id, "output", out.Id, "error", err)
			} else {
				logger.Info("Stopped output", "project", p.Id, "output", out.Id, "sequence", out.ProjectNodeSequence)
			}
		}
	}

	p.cleanup()
	p.SetProjectStatus(common.StatusStopped, nil)
	logger.Info("Finished stopping project components", "project", p.Id)
	return nil
}

// cleanup performs aggressive cleanup when normal stop fails
func (p *Project) cleanup() {
	for _, node := range p.FlowNodes {
		if node.FromInit {
			node.FromInit = false
			ReduceRefCount(node.FromPNS)
		}

		if node.ToInit {
			node.ToInit = false
			ReduceRefCount(node.ToPNS)
		}
	}
	p.FlowNodes = []FlowNode{}
	p.Inputs = make(map[string]*input.Input)
	p.Outputs = make(map[string]*output.Output)
	p.Rulesets = make(map[string]*rules_engine.Ruleset)
	p.MsgChannels = make(map[string]*chan map[string]interface{}, 0)
}

// waitForCompleteDataProcessing waits for all data to be fully processed through the pipeline
// This includes waiting for channels to empty AND thread pools to complete all tasks
func (p *Project) waitForCompleteDataProcessing() {
	overallTimeout := time.After(60 * time.Second) // 60 second overall timeout
	checkInterval := 100 * time.Millisecond
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-overallTimeout:
			return
		case <-ticker.C:
			allProcessed := true

			// Check all channels for remaining messages
			for _, ch := range p.MsgChannels {
				chLen := len(*ch)
				if chLen > 0 {
					allProcessed = false
					break
				}
			}

			if !allProcessed {
				continue
			}

			for _, rs := range p.Rulesets {
				if GetRefCount(rs.ProjectNodeSequence) == 1 {
					runningTasks := rs.GetRunningTaskCount()
					if runningTasks > 0 {
						allProcessed = false
						break
					}
				}
			}

			if allProcessed {
				logger.Info("All data processing completed", "project", p.Id)
				time.Sleep(5 * time.Second)
				return
			}
		}
	}
}

func (p *Project) initComponents() error {
	var createChannel bool

	for _, node := range p.FlowNodes {
		switch node.ToType {
		case "RULESET":
			if rs, ok := GlobalProject.PNSRulesets[node.ToPNS]; ok {
				p.Rulesets[node.ToPNS] = rs
				createChannel = false
			} else {
				rs, err := rules_engine.NewFromExisting(GlobalProject.Rulesets[node.ToID], node.ToPNS)
				if err != nil {
					return fmt.Errorf("failed to create ruleset from existing: %s %w", node.ToPNS, err)
				}
				common.GlobalMu.Lock()
				GlobalProject.PNSRulesets[node.ToPNS] = rs
				common.GlobalMu.Unlock()
				p.Rulesets[node.ToPNS] = rs

				createChannel = true
				c := make(chan map[string]interface{}, 1024)
				p.MsgChannels[node.ToPNS] = &c
				rs.UpStream[node.ToPNS] = &c
			}
		case "OUTPUT":
			if p.Testing {
				// In testing mode, create a test version of the output component
				// This avoids sending data to real external systems
				originalOutput, ok := GlobalProject.Outputs[node.ToID]
				if !ok {
					return fmt.Errorf("output component not found for testing: %s", node.ToID)
				}

				// Create a new output instance for testing based on the original config
				testOutput, err := output.NewFromExisting(originalOutput, node.ToPNS)
				if err != nil {
					return fmt.Errorf("failed to create test output component: %s %w", node.ToPNS, err)
				}

				// Set test-specific properties to avoid pollution
				testOutput.SetTestMode()                  // Disable sampling and global state interactions
				testOutput.OwnerProjects = []string{p.Id} // Mark as owned by this test project

				p.Outputs[node.ToPNS] = testOutput

				createChannel = true
				c := make(chan map[string]interface{}, 1024)
				p.MsgChannels[node.ToPNS] = &c
				testOutput.UpStream[node.ToPNS] = &c
			} else {
				// Production mode: use shared PNS output or create new one
				if o, ok := GlobalProject.PNSOutputs[node.ToPNS]; ok {
					p.Outputs[node.ToPNS] = o
					createChannel = false
				} else {
					o, err := output.NewFromExisting(GlobalProject.Outputs[node.ToID], node.ToPNS)
					if err != nil {
						return fmt.Errorf("failed to create output from existing: %s %w", node.ToPNS, err)
					}
					common.GlobalMu.Lock()
					GlobalProject.PNSOutputs[node.ToPNS] = o
					common.GlobalMu.Unlock()
					p.Outputs[node.ToPNS] = o

					createChannel = true
					c := make(chan map[string]interface{}, 1024)
					p.MsgChannels[node.ToPNS] = &c
					o.UpStream[node.ToPNS] = &c
				}
			}
		}

		node.ToInit = true
		AddRefCount(node.ToPNS)

		switch node.FromType {
		case "RULESET":
			if rs, ok := GlobalProject.PNSRulesets[node.FromPNS]; ok {
				p.Rulesets[node.FromPNS] = rs
			} else {
				rs, err := rules_engine.NewFromExisting(GlobalProject.Rulesets[node.FromID], node.FromPNS)
				if err != nil {
					return fmt.Errorf("failed to create ruleset from existing: %s %w", node.FromPNS, err)
				}
				common.GlobalMu.Lock()
				GlobalProject.PNSRulesets[node.FromPNS] = rs
				common.GlobalMu.Unlock()
				p.Rulesets[node.FromPNS] = rs
			}

			if createChannel {
				p.Rulesets[node.FromPNS].DownStream[node.ToPNS] = p.MsgChannels[node.ToPNS]
			}
		case "INPUT":
			if p.Testing {
				// In testing mode, create a test version of the input component
				// This avoids connecting to real external data sources
				originalInput, ok := GlobalProject.Inputs[node.FromID]
				if !ok {
					return fmt.Errorf("input component not found for testing: %s", node.FromID)
				}

				// Create a new input instance for testing based on the original config
				testInput, err := input.NewFromExisting(originalInput, node.FromPNS)
				if err != nil {
					return fmt.Errorf("failed to create test input component: %s %w", node.FromPNS, err)
				}

				// Set test-specific properties to avoid pollution
				testInput.SetTestMode()                  // Disable sampling and global state interactions
				testInput.OwnerProjects = []string{p.Id} // Mark as owned by this test project

				p.Inputs[node.FromPNS] = testInput
			} else {
				// Production mode: use the real input component
				if in, ok := GlobalProject.Inputs[node.FromID]; !ok {
					return fmt.Errorf("input component not found: %s", node.FromID)
				} else {
					p.Inputs[node.FromPNS] = in
				}
			}

			if createChannel {
				p.Inputs[node.FromPNS].DownStream[node.ToPNS] = p.MsgChannels[node.ToPNS]
			}
		}

		node.FromInit = true
		AddRefCount(node.FromPNS)
	}
	return nil
}

func (p *Project) runComponents() error {
	for _, in := range p.Inputs {
		if p.Testing {
			// In testing mode, use StartForTesting to avoid connecting to external data sources
			if err := in.StartForTesting(); err != nil {
				return fmt.Errorf("failed to start input component in testing mode %s: %w", in.Id, err)
			}
		} else {
			// Production mode: normal start
			if err := in.Start(); err != nil {
				return fmt.Errorf("failed to start input component %s: %w", in.Id, err)
			}
		}
	}

	for _, rs := range p.Rulesets {
		if err := rs.Start(); err != nil {
			return fmt.Errorf("failed to start ruleset component %s: %w", rs.RulesetID, err)
		}
	}

	for _, out := range p.Outputs {
		if p.Testing {
			// In testing mode, use StartForTesting to avoid external connectivity checks
			if err := out.StartForTesting(); err != nil {
				return fmt.Errorf("failed to start output component in testing mode %s: %w", out.Id, err)
			}
		} else {
			// Production mode: normal start
			if err := out.Start(); err != nil {
				return fmt.Errorf("failed to start output component %s: %w", out.Id, err)
			}
		}
	}
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
