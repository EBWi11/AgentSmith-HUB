package project

import (
	"AgentSmith-HUB/agent"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/rules_engine"
	"AgentSmith-HUB/skill"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var GlobalProject *GlobalProjectInfo
var projectAutoStopMu sync.Mutex
var projectAutoStopInFlight = make(map[string]struct{})

func shouldRestartProjectForComponentChange(projectID string, p *Project) bool {
	userWantsRunning, err := common.GetProjectUserIntention(projectID)
	if err != nil {
		logger.Error("Failed to get user intention, using actual status as fallback",
			"project", projectID,
			"error", err,
			"actual_status", p.Status)
		return p.Status == common.StatusRunning
	}

	return userWantsRunning
}

func rulesetUsesPlugin(ruleset *rules_engine.Ruleset, pluginID string) bool {
	if ruleset == nil {
		return false
	}

	for _, rule := range ruleset.Rules {
		for _, checkNode := range rule.CheckMap {
			if checkNode.Plugin != nil && checkNode.Plugin.Name == pluginID {
				return true
			}
		}

		for _, checklist := range rule.ChecklistMap {
			for _, checkNode := range checklist.CheckNodes {
				if checkNode.Plugin != nil && checkNode.Plugin.Name == pluginID {
					return true
				}
			}
		}

		for _, appendOp := range rule.AppendsMap {
			if appendOp.Plugin != nil && appendOp.Plugin.Name == pluginID {
				return true
			}
		}

		for _, modifyOp := range rule.ModifyMap {
			if modifyOp.Plugin != nil && modifyOp.Plugin.Name == pluginID {
				return true
			}
		}

		for _, pluginOp := range rule.PluginMap {
			if pluginOp.Plugin != nil && pluginOp.Plugin.Name == pluginID {
				return true
			}
		}
	}

	return false
}

func getRulesetsUsingPlugin(pluginID string) []string {
	rulesetSet := make(map[string]struct{})

	ForEachRuleset(func(rulesetID string, ruleset *rules_engine.Ruleset) bool {
		if rulesetUsesPlugin(ruleset, pluginID) {
			rulesetSet[rulesetID] = struct{}{}
		}
		return true
	})

	rulesetIDs := make([]string, 0, len(rulesetSet))
	for rulesetID := range rulesetSet {
		rulesetIDs = append(rulesetIDs, rulesetID)
	}
	sort.Strings(rulesetIDs)

	return rulesetIDs
}

func getProjectsUsingRulesets(rulesetIDs []string, shouldInclude func(projectID string, p *Project) bool) []string {
	projectSet := make(map[string]struct{})

	for _, rulesetID := range rulesetIDs {
		ForEachProject(func(projectID string, p *Project) bool {
			if p.CheckExist("RULESET", rulesetID) && shouldInclude(projectID, p) {
				projectSet[projectID] = struct{}{}
			}
			return true
		})
	}

	projectIDs := make([]string, 0, len(projectSet))
	for projectID := range projectSet {
		projectIDs = append(projectIDs, projectID)
	}
	sort.Strings(projectIDs)

	return projectIDs
}

func GetRulesetsUsingPlugin(pluginID string) []string {
	return getRulesetsUsingPlugin(pluginID)
}

func SafeDeletePluginComponent(id string) ([]string, error) {
	referencingRulesets := GetRulesetsUsingPlugin(id)
	if len(referencingRulesets) > 0 {
		return nil, fmt.Errorf("plugin %s is referenced by ruleset(s): %v", id, referencingRulesets)
	}

	return plugin.SafeDeletePlugin(id)
}

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

		// Collect agent statistics
		for _, a := range proj.GetProjectAgents() {
			increment := a.GetIncrementAndUpdate()
			if increment > 0 {
				components = append(components, common.DailyStatsData{
					ProjectID:           proj.Id,
					ComponentID:         a.Id,
					ComponentType:       "agent",
					ProjectNodeSequence: a.ProjectNodeSequence,
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
				if shouldRestartProjectForComponentChange(projectID, p) {
					affectedProjects[projectID] = struct{}{}
				}
			}
			return true
		})
	case "output":
		// Find all projects using this output
		ForEachProject(func(projectID string, p *Project) bool {
			if p.CheckExist("OUTPUT", componentID) {
				if shouldRestartProjectForComponentChange(projectID, p) {
					affectedProjects[projectID] = struct{}{}
				}
			}
			return true
		})
	case "ruleset":
		// Find all projects using this ruleset
		ForEachProject(func(projectID string, p *Project) bool {
			if p.CheckExist("RULESET", componentID) {
				if shouldRestartProjectForComponentChange(projectID, p) {
					affectedProjects[projectID] = struct{}{}
				}
			}
			return true
		})
	case "plugin":
		rulesetIDs := getRulesetsUsingPlugin(componentID)
		for _, projectID := range getProjectsUsingRulesets(rulesetIDs, shouldRestartProjectForComponentChange) {
			affectedProjects[projectID] = struct{}{}
		}

		logger.Info("Plugin change affects rulesets and projects",
			"plugin", componentID,
			"affected_rulesets", len(rulesetIDs),
			"affected_projects", len(affectedProjects))

	case "agent":
		ForEachProject(func(projectID string, p *Project) bool {
			if p.CheckExist("AGENT", componentID) {
				if shouldRestartProjectForComponentChange(projectID, p) {
					affectedProjects[projectID] = struct{}{}
				}
			}
			return true
		})
	case "skill":
		// Skill change -> find agents that reference this skill -> find projects using those agents
		var affectedAgentIDs []string
		ForEachAgent(func(agentID string, a *agent.Agent) bool {
			if a.Config != nil {
				for _, sid := range a.Config.Skills {
					if sid == componentID {
						affectedAgentIDs = append(affectedAgentIDs, agentID)
						break
					}
				}
			}
			return true
		})
		for _, agentID := range affectedAgentIDs {
			ForEachProject(func(projectID string, p *Project) bool {
				if p.CheckExist("AGENT", agentID) {
					if shouldRestartProjectForComponentChange(projectID, p) {
						affectedProjects[projectID] = struct{}{}
					}
				}
				return true
			})
		}
	case "project":
		// For project changes, check if user wants this project to be running
		if p, exists := GetProject(componentID); exists {
			if shouldRestartProjectForComponentChange(componentID, p) {
				affectedProjects[componentID] = struct{}{}
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
		err := proj.StartConverged()
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

func markProjectAutoStopInFlight(projectID string) bool {
	projectAutoStopMu.Lock()
	defer projectAutoStopMu.Unlock()
	if _, exists := projectAutoStopInFlight[projectID]; exists {
		return false
	}
	projectAutoStopInFlight[projectID] = struct{}{}
	return true
}

func clearProjectAutoStopInFlight(projectID string) {
	projectAutoStopMu.Lock()
	defer projectAutoStopMu.Unlock()
	delete(projectAutoStopInFlight, projectID)
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
		for _, inputComp := range proj.GetProjectInputs() {
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
		for _, outputComp := range proj.GetProjectOutputs() {
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
		for _, rulesetComp := range proj.GetProjectRulesets() {
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

		// Check agent components
		for _, agentComp := range proj.GetProjectAgents() {
			if agentComp.Err != nil {
				errors = append(errors, common.ProjectComponentError{
					ProjectID:   projectID,
					ComponentID: agentComp.Id,
					Type:        "agent",
					Status:      agentComp.Status,
					Error:       agentComp.Err,
				})
			}
		}

		return true // Continue iteration
	})

	return errors
}

// SetProjectErrorStatus force-stops a project after component failure and leaves
// the project in error state so callers can distinguish crash recovery from a
// user-requested stop.
func SetProjectErrorStatus(projectID string, componentErrors []common.ProjectComponentError) {
	proj, exists := GetProject(projectID)
	if !exists {
		logger.Error("Cannot set error status for non-existent project", "project", projectID)
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

	componentErr := fmt.Errorf("%s", errorMsg.String())

	if !markProjectAutoStopInFlight(projectID) {
		logger.Info("Project auto-stop already in progress; skipping duplicate request",
			"project", projectID)
		return
	}

	defer clearProjectAutoStopInFlight(projectID)

	// Run the stop path inline so callers observe a fully converged error state
	// once this function returns.
	stopErr := proj.Stop(true)
	if stopErr != nil {
		componentErr = fmt.Errorf("%s; project stop failed during crash recovery: %v", componentErr.Error(), stopErr)
	}

	proj.SetProjectStatus(common.StatusError, componentErr)

	logger.Error("Project set to error status due to component failures",
		"project", projectID,
		"component_count", len(componentErrors),
		"error", componentErr)
}

func init() {
	GlobalProject = &GlobalProjectInfo{}
	GlobalProject.Projects = make(map[string]*Project)
	GlobalProject.Inputs = make(map[string]*input.Input)
	GlobalProject.Outputs = make(map[string]*output.Output)
	GlobalProject.Rulesets = make(map[string]*rules_engine.Ruleset)
	GlobalProject.Agents = make(map[string]*agent.Agent)
	GlobalProject.Skills = make(map[string]*skill.Skill)

	GlobalProject.PNSOutputs = make(map[string]*output.Output)
	GlobalProject.PNSRulesets = make(map[string]*rules_engine.Ruleset)
	GlobalProject.PNSAgents = make(map[string]*agent.Agent)

	GlobalProject.ProjectsNew = make(map[string]string)
	GlobalProject.InputsNew = make(map[string]string)
	GlobalProject.OutputsNew = make(map[string]string)
	GlobalProject.RulesetsNew = make(map[string]string)
	GlobalProject.AgentsNew = make(map[string]string)
	GlobalProject.SkillsNew = make(map[string]string)

	// Register skill resolver for agents to look up skill components
	agent.RegisterSkillResolver(func(id string) (*skill.Skill, bool) {
		return GetSkill(id)
	})

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
		Status:      common.StatusStopped,
		Config:      &cfg,
		Inputs:      make(map[string]*input.Input),
		Outputs:     make(map[string]*output.Output),
		Rulesets:    make(map[string]*rules_engine.Ruleset),
		Agents:      make(map[string]*agent.Agent),
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
	if componentType != "INPUT" && componentType != "OUTPUT" && componentType != "RULESET" && componentType != "AGENT" {
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

	if componentType != "INPUT" && componentType != "OUTPUT" && componentType != "RULESET" && componentType != "AGENT" {
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

	// Panic recovery must be deferred before parseContent so that any panic
	// inside parseContent (or any subsequent step) is caught and the project
	// is left in a clean error state rather than crashing the process.
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Panic during project start", "project", p.Id, "panic", r)
			p.cleanup()
			p.SetProjectStatus(common.StatusError, fmt.Errorf("panic during start: %v", r))
		}
	}()

	if p.Status == common.StatusRunning && p.areAllComponentsRunning() {
		logger.Info("Project already running; start request is a no-op", "project", p.Id)
		return nil
	}

	if p.Status == common.StatusError || p.Status == common.StatusStarting || p.Status == common.StatusRunning {
		logger.Info("Reconciling project to stopped before start",
			"project", p.Id,
			"current_status", p.Status)
		if stopErr := p.Stop(false); stopErr != nil {
			logger.Error("Project pre-start reconciliation stop returned error; continuing with restart attempt",
				"project", p.Id,
				"error", stopErr)
		}
	}

	err := p.parseContent()
	if err != nil {
		p.SetProjectStatus(common.StatusError, fmt.Errorf("project parse error: %s", err.Error()))
		return fmt.Errorf("project parse error: %s", err.Error())
	}

	// Atomic status check and transition
	// Allow starting only once the project has converged to a quiescent state.
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
	// Allow stopping from starting state to recover from partial startup and avoid restart races.
	if !p.atomicStatusTransition([]common.Status{common.StatusRunning, common.StatusError, common.StatusStarting}, common.StatusStopping) {
		return fmt.Errorf("project is not in stoppable state, current status: %s", p.Status)
	}

	// Signal all components to stop by closing the stop channel
	p.stopOnce.Do(func() {
		if p.stopChan != nil {
			close(p.stopChan)
		}
	})

	// Overall timeout for the entire stop process
	overallTimeout := time.After(6 * time.Minute)
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
		// CRITICAL: Timeout occurred but goroutine may still be running
		// Force cleanup and set status to stopped to allow restart
		// The goroutine will eventually finish but we don't wait
		logger.Error("Stop operation timed out, forcing cleanup and stopped status (goroutine may still be running)", "project", p.Id)
		p.cleanup()
		p.SetProjectStatus(common.StatusStopped, nil)

		// Give components extra time to actually stop before next start
		// This mitigates "component is not stopped" errors on restart
		return fmt.Errorf("project stop operation timed out (forced to stopped, sleep recommended)")
	}
}

func (p *Project) Restart(recordOperation bool, triggeredBy string) (err error) {
	// Cooldown mechanism to prevent rapid restarts
	p.restartMu.Lock()
	if time.Since(p.lastRestartTime) < 5*time.Second {
		p.restartMu.Unlock()
		logger.Info("Project restart skipped due to cooldown", "project", p.Id)
		return fmt.Errorf("project restart skipped due to cooldown")
	}
	p.lastRestartTime = time.Now()
	p.restartMu.Unlock()

	common.ProjectOperationMu.Lock()
	// locked tracks whether we still hold ProjectOperationMu so the deferred
	// function can decide whether to unlock.  We release the lock before the
	// post-stop sleep and before calling startWithRetry (which calls Start(true)
	// and therefore acquires its own lock per attempt).  Holding a global mutex
	// across multi-second sleeps and multi-retry loops would block all other
	// project operations across the entire node.
	locked := true

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during restart: %v", r)
			logger.Error("Panic during project restart", "project", p.Id, "panic", r)
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

		if locked {
			common.ProjectOperationMu.Unlock()
		}
	}()

	logger.Info("Restarting project", "project", p.Id)

	// Check status - Stop() is called with lock=false because we already hold the lock.
	// Include starting state to ensure partially started components are fully stopped before re-start.
	if p.Status == common.StatusRunning || p.Status == common.StatusError || p.Status == common.StatusStarting {
		stopErr := p.Stop(false)
		if stopErr != nil {
			// Stop() guarantees status is Stopped even on error/timeout
			logger.Error("Stop returned error during restart, but status should be stopped", "project", p.Id, "error", stopErr)
		}
	}

	// Release the global lock before sleeping and before the retry loop.
	// Start(true) inside startWithRetry acquires its own per-attempt lock, so
	// other project operations are not blocked during the wait or retries.
	locked = false
	common.ProjectOperationMu.Unlock()

	// Sleep after stop to ensure components are fully released
	time.Sleep(10 * time.Second)

	// Start the project again with retry mechanism
	err = p.startWithRetry()
	if err != nil {
		err = fmt.Errorf("failed to start project after restart (exhausted all retries): %w", err)
		return err
	}

	logger.Info("Project restarted successfully", "project", p.Id)
	return nil
}

// StartConverged starts the project and retries until all components reach
// running state or the retry budget is exhausted.
func (p *Project) StartConverged() error {
	return p.startWithRetry()
}

type rulesetInboundEdge struct {
	fromType string
	fromID   string
	fromPNS  string
	toPNS    string
}

type rulesetSwapPlan struct {
	pns              string
	oldRuleset       *rules_engine.Ruleset
	newRuleset       *rules_engine.Ruleset
	upstreamBackup   map[string]*chan map[string]interface{}
	downstreamBackup map[string]*chan map[string]interface{}
}

type agentInboundEdge struct {
	fromType string
	fromID   string
	fromPNS  string
	toPNS    string
}

type agentSwapPlan struct {
	pns              string
	oldAgent         *agent.Agent
	newAgent         *agent.Agent
	upstreamBackup   map[string]*chan map[string]interface{}
	downstreamBackup map[string]*chan map[string]interface{}
}

func (p *Project) collectRulesetPNSByID(rulesetID string) map[string]struct{} {
	targetPNS := make(map[string]struct{})
	for _, node := range p.FlowNodes {
		if node.ToType == "RULESET" && node.ToID == rulesetID && node.ToInit {
			targetPNS[node.ToPNS] = struct{}{}
		}
		if node.FromType == "RULESET" && node.FromID == rulesetID && node.FromInit {
			targetPNS[node.FromPNS] = struct{}{}
		}
	}
	return targetPNS
}

func (p *Project) collectInboundEdges(targetPNS map[string]struct{}) []rulesetInboundEdge {
	edges := make([]rulesetInboundEdge, 0)
	for _, node := range p.FlowNodes {
		if node.ToType != "RULESET" {
			continue
		}
		if _, ok := targetPNS[node.ToPNS]; !ok {
			continue
		}
		edges = append(edges, rulesetInboundEdge{
			fromType: node.FromType,
			fromID:   node.FromID,
			fromPNS:  node.FromPNS,
			toPNS:    node.ToPNS,
		})
	}
	return edges
}

func (p *Project) disconnectRulesetInboundEdges(edges []rulesetInboundEdge) {
	for _, e := range edges {
		switch e.fromType {
		case "INPUT":
			SafeDeleteInputDownstream(e.fromID, e.toPNS)
		case "RULESET":
			SafeDeletePNSRulesetDownstream(e.fromPNS, e.toPNS)
		case "AGENT":
			SafeDeleteAgentDownstream(e.fromPNS, e.toPNS)
		}
	}
}

func (p *Project) reconnectRulesetInboundEdges(edges []rulesetInboundEdge) {
	for _, e := range edges {
		ch, exists := p.MsgChannels[e.toPNS]
		if !exists || ch == nil {
			continue
		}
		switch e.fromType {
		case "INPUT":
			SafeSetInputDownstream(e.fromID, e.toPNS, ch)
		case "RULESET":
			SafeSetPNSRulesetDownstream(e.fromPNS, e.toPNS, ch)
		case "AGENT":
			SafeSetAgentDownstream(e.fromPNS, e.toPNS, ch)
		}
	}
}

func (p *Project) collectAgentPNSByID(agentID string) map[string]struct{} {
	targetPNS := make(map[string]struct{})
	for _, node := range p.FlowNodes {
		if node.ToType == "AGENT" && node.ToID == agentID && node.ToInit {
			targetPNS[node.ToPNS] = struct{}{}
		}
		if node.FromType == "AGENT" && node.FromID == agentID && node.FromInit {
			targetPNS[node.FromPNS] = struct{}{}
		}
	}
	return targetPNS
}

func (p *Project) collectAgentInboundEdges(targetPNS map[string]struct{}) []agentInboundEdge {
	edges := make([]agentInboundEdge, 0)
	for _, node := range p.FlowNodes {
		if node.ToType != "AGENT" {
			continue
		}
		if _, ok := targetPNS[node.ToPNS]; !ok {
			continue
		}
		edges = append(edges, agentInboundEdge{
			fromType: node.FromType,
			fromID:   node.FromID,
			fromPNS:  node.FromPNS,
			toPNS:    node.ToPNS,
		})
	}
	return edges
}

func (p *Project) disconnectAgentInboundEdges(edges []agentInboundEdge) {
	for _, e := range edges {
		switch e.fromType {
		case "INPUT":
			SafeDeleteInputDownstream(e.fromID, e.toPNS)
		case "RULESET":
			SafeDeletePNSRulesetDownstream(e.fromPNS, e.toPNS)
		case "AGENT":
			SafeDeleteAgentDownstream(e.fromPNS, e.toPNS)
		}
	}
}

func (p *Project) reconnectAgentInboundEdges(edges []agentInboundEdge) {
	for _, e := range edges {
		ch, exists := p.MsgChannels[e.toPNS]
		if !exists || ch == nil {
			continue
		}
		switch e.fromType {
		case "INPUT":
			SafeSetInputDownstream(e.fromID, e.toPNS, ch)
		case "RULESET":
			SafeSetPNSRulesetDownstream(e.fromPNS, e.toPNS, ch)
		case "AGENT":
			SafeSetAgentDownstream(e.fromPNS, e.toPNS, ch)
		}
	}
}

func (p *Project) waitForAgentDrain(agentID string, targetPNS map[string]struct{}) {
	checkInterval := 100 * time.Millisecond
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	logger.Info("Waiting for agent drain before hot reload", "project", p.Id, "agent", agentID, "instances", len(targetPNS))

	for {
		allDrained := true
		pendingMessages := 0

		for pns := range targetPNS {
			oldAgent, ok := GetPNSAgent(pns)
			if !ok || oldAgent == nil {
				continue
			}
			for _, ch := range oldAgent.UpStream {
				if ch == nil {
					continue
				}
				if chLen := len(*ch); chLen > 0 {
					pendingMessages += chLen
					allDrained = false
				}
			}
		}

		if allDrained {
			logger.Info("Agent drain completed", "project", p.Id, "agent", agentID)
			return
		}

		logger.Debug("Waiting for agent pending messages to drain",
			"project", p.Id,
			"agent", agentID,
			"pending_messages", pendingMessages)
		<-ticker.C
	}
}

// HotReloadRuleset swaps all running PNS instances of a ruleset in this project without restarting the project.
// It fully stops old ruleset instances to ensure cleanup and prevent memory leaks.
func (p *Project) HotReloadRuleset(rulesetID string, triggeredBy string) (err error) {
	common.ProjectOperationMu.Lock()
	defer common.ProjectOperationMu.Unlock()

	if p.Status != common.StatusRunning {
		return nil
	}

	template, exists := GetRuleset(rulesetID)
	if !exists || template == nil {
		return fmt.Errorf("ruleset template not found: %s", rulesetID)
	}

	targetPNS := p.collectRulesetPNSByID(rulesetID)
	if len(targetPNS) == 0 {
		return nil
	}

	swapPlans := make([]rulesetSwapPlan, 0, len(targetPNS))
	for pns := range targetPNS {
		oldRS, ok := GetPNSRuleset(pns)
		if !ok || oldRS == nil {
			return fmt.Errorf("pns ruleset not found: %s", pns)
		}
		newRS, newErr := rules_engine.NewFromExisting(template, pns)
		if newErr != nil {
			return fmt.Errorf("failed to build new ruleset instance for %s: %w", pns, newErr)
		}

		upstreamBackup := make(map[string]*chan map[string]interface{}, len(oldRS.UpStream))
		for k, ch := range oldRS.UpStream {
			upstreamBackup[k] = ch
		}
		downstreamBackup := oldRS.CopyDownstream()

		swapPlans = append(swapPlans, rulesetSwapPlan{
			pns:              pns,
			oldRuleset:       oldRS,
			newRuleset:       newRS,
			upstreamBackup:   upstreamBackup,
			downstreamBackup: downstreamBackup,
		})
	}

	inboundEdges := p.collectInboundEdges(targetPNS)
	p.disconnectRulesetInboundEdges(inboundEdges)
	activatedPNS := make(map[string]struct{}, len(swapPlans))

	defer func() {
		if err != nil {
			// Release all newly created ruleset instances that did not successfully take over.
			for _, plan := range swapPlans {
				if _, activated := activatedPNS[plan.pns]; !activated && plan.newRuleset != nil {
					plan.newRuleset.SetStatus(common.StatusError, fmt.Errorf("discarding ruleset instance after hot reload failure"))
					_ = plan.newRuleset.Stop()
				}
			}

			// If any step fails before the final successful reconnect, restore inbound routing.
			p.reconnectRulesetInboundEdges(inboundEdges)
		}
	}()

	for _, plan := range swapPlans {
		stopErr := plan.oldRuleset.Stop()
		if stopErr != nil {
			err = fmt.Errorf("failed to stop old ruleset %s: %w", plan.pns, stopErr)
			return err
		}

		// Bind channels preserved from old instance so new instance keeps the same project routing.
		plan.newRuleset.UpStream = plan.upstreamBackup
		plan.newRuleset.ResetDownstream()
		for k, ch := range plan.downstreamBackup {
			plan.newRuleset.SetDownstream(k, ch)
		}

		startErr := plan.newRuleset.Start()
		if startErr != nil {
			err = fmt.Errorf("failed to start new ruleset %s: %w", plan.pns, startErr)
			return err
		}

		SetPNSRuleset(plan.pns, plan.newRuleset)
		p.Rulesets[plan.pns] = plan.newRuleset
		activatedPNS[plan.pns] = struct{}{}
	}

	p.reconnectRulesetInboundEdges(inboundEdges)
	logger.Info("Ruleset hot reload completed", "project", p.Id, "ruleset", rulesetID, "triggered_by", triggeredBy, "instances", len(swapPlans))
	return nil
}

// HotReloadAgent swaps all running PNS instances of an agent in this project without restarting the project.
func (p *Project) HotReloadAgent(agentID string, triggeredBy string) (err error) {
	common.ProjectOperationMu.Lock()
	defer common.ProjectOperationMu.Unlock()

	if p.Status != common.StatusRunning {
		return nil
	}

	template, exists := GetAgent(agentID)
	if !exists || template == nil {
		return fmt.Errorf("agent template not found: %s", agentID)
	}

	targetPNS := p.collectAgentPNSByID(agentID)
	if len(targetPNS) == 0 {
		return nil
	}

	swapPlans := make([]agentSwapPlan, 0, len(targetPNS))
	for pns := range targetPNS {
		oldAgent, ok := GetPNSAgent(pns)
		if !ok || oldAgent == nil {
			return fmt.Errorf("pns agent not found: %s", pns)
		}
		newAgent, newErr := agent.NewFromExisting(template, pns)
		if newErr != nil {
			return fmt.Errorf("failed to build new agent instance for %s: %w", pns, newErr)
		}
		newAgent.ProjectID = oldAgent.ProjectID

		upstreamBackup := make(map[string]*chan map[string]interface{}, len(oldAgent.UpStream))
		for k, ch := range oldAgent.UpStream {
			upstreamBackup[k] = ch
		}
		downstreamBackup := oldAgent.CopyDownstream()

		swapPlans = append(swapPlans, agentSwapPlan{
			pns:              pns,
			oldAgent:         oldAgent,
			newAgent:         newAgent,
			upstreamBackup:   upstreamBackup,
			downstreamBackup: downstreamBackup,
		})
	}

	inboundEdges := p.collectAgentInboundEdges(targetPNS)
	p.disconnectAgentInboundEdges(inboundEdges)
	p.waitForAgentDrain(agentID, targetPNS)
	activatedPNS := make(map[string]struct{}, len(swapPlans))

	defer func() {
		if err != nil {
			for _, plan := range swapPlans {
				if _, activated := activatedPNS[plan.pns]; !activated && plan.newAgent != nil {
					plan.newAgent.SetStatus(common.StatusError, fmt.Errorf("discarding agent instance after hot reload failure"))
					_ = plan.newAgent.Stop()
				}
			}
			p.reconnectAgentInboundEdges(inboundEdges)
		}
	}()

	for _, plan := range swapPlans {
		stopErr := plan.oldAgent.Stop()
		if stopErr != nil {
			err = fmt.Errorf("failed to stop old agent %s: %w", plan.pns, stopErr)
			return err
		}

		plan.newAgent.UpStream = plan.upstreamBackup
		plan.newAgent.ResetDownstream()
		for k, ch := range plan.downstreamBackup {
			plan.newAgent.SetDownstream(k, ch)
		}

		startErr := plan.newAgent.Start()
		if startErr != nil {
			err = fmt.Errorf("failed to start new agent %s: %w", plan.pns, startErr)
			return err
		}

		SetPNSAgent(plan.pns, plan.newAgent)
		p.Agents[plan.pns] = plan.newAgent
		activatedPNS[plan.pns] = struct{}{}
	}

	p.reconnectAgentInboundEdges(inboundEdges)
	logger.Info("Agent hot reload completed", "project", p.Id, "agent", agentID, "triggered_by", triggeredBy, "instances", len(swapPlans))
	return nil
}

// startWithRetry starts the project with retry mechanism for components that fail to reach running status.
// It must be called WITHOUT holding ProjectOperationMu — each attempt calls Start(true) which
// acquires and releases its own lock.  Retry delays therefore do not block other project operations.
func (p *Project) startWithRetry() error {
	maxRetries := 3
	retryDelays := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		logger.Info("Starting project", "project", p.Id, "attempt", attempt+1, "max_attempts", maxRetries+1)

		// Start(true) acquires ProjectOperationMu for the duration of the attempt only.
		err := p.Start(true)
		if err != nil {
			if attempt == maxRetries {
				logger.Error("Failed to start project after all retry attempts", "project", p.Id, "final_error", err)
				p.SetProjectStatus(common.StatusError, fmt.Errorf("failed to start project after %d attempts: %w", maxRetries+1, err))
				return fmt.Errorf("failed to start project after %d attempts: %w", maxRetries+1, err)
			}

			logger.Error("Project start failed, will retry", "project", p.Id, "attempt", attempt+1, "error", err, "retry_delay", retryDelays[attempt])
			time.Sleep(retryDelays[attempt]) // no lock held during sleep
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

		logger.Error("Project started but some components are not running, will retry", "project", p.Id, "attempt", attempt+1, "retry_delay", retryDelays[attempt])

		// Stop the project before retrying (Stop(true) acquires its own lock)
		_ = p.Stop(true)
		time.Sleep(retryDelays[attempt]) // no lock held during sleep
	}

	return fmt.Errorf("unexpected end of retry loop")
}

// areAllComponentsRunning checks if all project components are in running state
func (p *Project) areAllComponentsRunning() bool {
	// Check input components
	inputs := p.GetProjectInputs()
	for _, in := range inputs {
		if in.Status != common.StatusRunning {
			logger.Error("Input component not running", "project", p.Id, "input", in.Id, "status", in.Status)
			return false
		}
	}

	// Check output components
	outputs := p.GetProjectOutputs()
	for _, out := range outputs {
		if out.Status != common.StatusRunning {
			logger.Error("Output component not running", "project", p.Id, "output", out.Id, "status", out.Status)
			return false
		}
	}

	// Check ruleset components
	rulesets := p.GetProjectRulesets()
	for _, rs := range rulesets {
		if rs.Status != common.StatusRunning {
			logger.Error("Ruleset component not running", "project", p.Id, "ruleset", rs.RulesetID, "status", rs.Status)
			return false
		}
	}

	// Check agent components
	for _, a := range p.Agents {
		if a.Status != common.StatusRunning {
			logger.Error("Agent component not running", "project", p.Id, "agent", a.Id, "status", a.Status)
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

	agentComponents := p.GetProjectAgents()
	logger.Info("Step 4b: Stopping agents", "project", p.Id, "count", len(agentComponents))
	for id, a := range agentComponents {
		DeletePNSAgent(id)
		if CalculateRefCount(id, p.Id) == 0 {
			stopErr := a.Stop()
			if stopErr != nil {
				logger.Error("Failed to stop agent", "project", p.Id, "agent", a.Id, "error", stopErr)
				stopErrors = append(stopErrors, fmt.Errorf("agent %s: %w", a.Id, stopErr))
			} else {
				logger.Info("Stopped agent", "project", p.Id, "agent", a.Id)
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
	// Serialise concurrent cleanup calls: Stop() may return on timeout while
	// its background goroutine is still inside stopComponentsInternal, which
	// also calls cleanup at the end.  The second caller waits here, then finds
	// all maps already empty and returns cheaply.
	p.cleanupMu.Lock()
	defer p.cleanupMu.Unlock()

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
	p.Agents = make(map[string]*agent.Agent)
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
			in.DeleteDownstream(downstreamID)
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
			var err error
			if p.Testing {
				err = in.StopForTesting()
				if err != nil {
					logger.Error("Failed to stop test input", "project", p.Id, "input", in.Id, "error", err)
					stopErrors = append(stopErrors, fmt.Errorf("test input %s: %w", in.Id, err))
				} else {
					logger.Info("Stopped test input", "project", p.Id, "input", in.Id)
				}
			} else {
				err = in.Stop()
				if err != nil {
					logger.Error("Failed to stop input", "project", p.Id, "input", in.Id, "error", err)
					stopErrors = append(stopErrors, fmt.Errorf("input %s: %w", in.Id, err))
				} else {
					logger.Info("Stopped input", "project", p.Id, "input", in.Id)
				}
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
					r.DeleteDownstream(node.ToPNS)
				}
			}
		}

		if node.FromType == "AGENT" {
			if CalculateRefCount(node.FromPNS, p.Id) > 0 {
				SafeDeleteAgentDownstream(node.FromPNS, node.ToPNS)
			}
		}
	}
}

func (p *Project) initComponents() error {
	// If Stop() timed out and returned early, its background goroutine may still
	// be executing cleanup().  Wait for it to finish before we start writing new
	// channel and component state, preventing concurrent map modification.
	p.cleanupMu.Lock()
	p.cleanupMu.Unlock() //nolint:staticcheck — intentional lock-then-immediate-unlock barrier

	// Track which nodes need new channels created
	nodeChannelStatus := make(map[string]bool) // key: ToPNS, value: whether channel was created

	// Cleanup function to remove any partially initialized components on error
	cleanup := func() {
		logger.Info("Cleaning up partially initialized components due to error", "project", p.Id)

		// Clean up created PNS rulesets
		for pns := range p.Rulesets {
			DeletePNSRuleset(pns)
		}

		// Clean up created PNS agents
		for pns := range p.Agents {
			DeletePNSAgent(pns)
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
		p.Agents = make(map[string]*agent.Agent)
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
				c := make(chan map[string]interface{}, common.PipelineRulesetBuffer)
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
				c := make(chan map[string]interface{}, common.PipelineOutputBuffer)
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
					c := make(chan map[string]interface{}, common.PipelineOutputBuffer)
					p.MsgChannels[node.ToPNS] = &c
					o.UpStream[node.ToPNS] = &c
				}
			}
		case "AGENT":
			a, exists := GetPNSAgent(node.ToPNS)
			if exists {
				p.Agents[node.ToPNS] = a
				nodeChannelStatus[node.ToPNS] = false
			} else {
				originalAgent, exists := GetAgent(node.ToID)
				if !exists {
					cleanup()
					return fmt.Errorf("agent component not found: %s", node.ToID)
				}

				a, err := agent.NewFromExisting(originalAgent, node.ToPNS)
				if err != nil {
					if originalAgent != nil {
						originalAgent.SetStatus(common.StatusError, fmt.Errorf("failed to create PNS instance: %w", err))
					}
					cleanup()
					return fmt.Errorf("failed to create agent from existing: %s %w", node.ToPNS, err)
				}

				SetPNSAgent(node.ToPNS, a)
				p.Agents[node.ToPNS] = a

				nodeChannelStatus[node.ToPNS] = true
				c := make(chan map[string]interface{}, common.PipelineAgentBuffer)
				p.MsgChannels[node.ToPNS] = &c
				a.UpStream[node.ToPNS] = &c
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
		case "AGENT":
			a, exists := GetPNSAgent(node.FromPNS)
			if exists {
				a.ProjectID = p.Id
				p.Agents[node.FromPNS] = a
			} else {
				originalAgent, exists := GetAgent(node.FromID)
				if !exists {
					cleanup()
					return fmt.Errorf("agent component not found: %s", node.FromID)
				}

				a, err := agent.NewFromExisting(originalAgent, node.FromPNS)
				if err != nil {
					if originalAgent != nil {
						originalAgent.SetStatus(common.StatusError, fmt.Errorf("failed to create PNS instance: %w", err))
					}
					cleanup()
					return fmt.Errorf("failed to create agent from existing: %s %w", node.FromPNS, err)
				}
				a.ProjectID = p.Id

				SetPNSAgent(node.FromPNS, a)
				p.Agents[node.FromPNS] = a
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
					fromRs.SetDownstream(node.ToPNS, toChannel)
				} else {
					// If no local channel, try to find existing channel in shared PNS component
					if node.ToType == "OUTPUT" {
						if sharedOutput, exists := GetPNSOutput(node.ToPNS); exists {
							if sharedChannel, exists := sharedOutput.UpStream[node.ToPNS]; exists {
								fromRs.SetDownstream(node.ToPNS, sharedChannel)
							}
						}
					} else if node.ToType == "RULESET" {
						if sharedRuleset, exists := GetPNSRuleset(node.ToPNS); exists {
							if sharedChannel, exists := sharedRuleset.UpStream[node.ToPNS]; exists {
								fromRs.SetDownstream(node.ToPNS, sharedChannel)
							}
						}
					} else if node.ToType == "AGENT" {
						if sharedAgent, exists := GetPNSAgent(node.ToPNS); exists {
							if sharedChannel, exists := sharedAgent.UpStream[node.ToPNS]; exists {
								fromRs.SetDownstream(node.ToPNS, sharedChannel)
							}
						}
					}
				}
			}
		case "AGENT":
			if fromAgent, exists := p.Agents[node.FromPNS]; exists {
				if toChannel, channelExists := p.MsgChannels[node.ToPNS]; channelExists {
					fromAgent.SetDownstream(node.ToPNS, toChannel)
				} else {
					if node.ToType == "OUTPUT" {
						if sharedOutput, exists := GetPNSOutput(node.ToPNS); exists {
							if sharedChannel, exists := sharedOutput.UpStream[node.ToPNS]; exists {
								fromAgent.SetDownstream(node.ToPNS, sharedChannel)
							}
						}
					} else if node.ToType == "RULESET" {
						if sharedRuleset, exists := GetPNSRuleset(node.ToPNS); exists {
							if sharedChannel, exists := sharedRuleset.UpStream[node.ToPNS]; exists {
								fromAgent.SetDownstream(node.ToPNS, sharedChannel)
							}
						}
					} else if node.ToType == "AGENT" {
						if sharedAgent, exists := GetPNSAgent(node.ToPNS); exists {
							if sharedChannel, exists := sharedAgent.UpStream[node.ToPNS]; exists {
								fromAgent.SetDownstream(node.ToPNS, sharedChannel)
							}
						}
					}
				}
			}
		case "INPUT":
			if fromInput, exists := p.Inputs[node.FromPNS]; exists {
				if toChannel, channelExists := p.MsgChannels[node.ToPNS]; channelExists {
					fromInput.SetDownstream(node.ToPNS, toChannel)
					logger.Info("Input downstream connection established",
						"project", p.Id,
						"input", fromInput.Id,
						"from_pns", node.FromPNS,
						"to_pns", node.ToPNS,
						"to_type", node.ToType)
				} else {
					if node.ToType == "OUTPUT" {
						if sharedOutput, exists := GetPNSOutput(node.ToPNS); exists {
							if sharedChannel, exists := sharedOutput.UpStream[node.ToPNS]; exists {
								fromInput.SetDownstream(node.ToPNS, sharedChannel)
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
								fromInput.SetDownstream(node.ToPNS, sharedChannel)
								logger.Info("Input downstream connection established to shared ruleset",
									"project", p.Id,
									"input", fromInput.Id,
									"from_pns", node.FromPNS,
									"to_pns", node.ToPNS)
							}
						}
					} else if node.ToType == "AGENT" {
						if sharedAgent, exists := GetPNSAgent(node.ToPNS); exists {
							if sharedChannel, exists := sharedAgent.UpStream[node.ToPNS]; exists {
								fromInput.SetDownstream(node.ToPNS, sharedChannel)
								logger.Info("Input downstream connection established to shared agent",
									"project", p.Id,
									"input", fromInput.Id,
									"from_pns", node.FromPNS,
									"to_pns", node.ToPNS)
							}
						}
					}
				}
			} else {
				logger.Error("Input component not found for connection",
					"project", p.Id,
					"from_pns", node.FromPNS,
					"from_id", node.FromID)
			}
		}
	}

	logger.Info("Components initialized successfully", "project", p.Id,
		"inputs", len(p.Inputs),
		"outputs", len(p.Outputs),
		"rulesets", len(p.Rulesets),
		"agents", len(p.Agents))

	return nil
}

func (p *Project) runComponents() error {
	// Start components in reverse dependency order: outputs -> agents -> rulesets -> inputs
	// This ensures downstream components are ready before upstream starts producing data

	// Track started components for cleanup on failure
	var startedOutputs []*output.Output
	var startedAgents []*agent.Agent
	var startedRulesets []*rules_engine.Ruleset
	var startedInputs []*input.Input

	// Cleanup function to stop all started components on error
	cleanup := func() {
		logger.Info("Cleaning up started components due to error", "project", p.Id)

		for _, in := range startedInputs {
			if p.Testing {
				_ = in.StopForTesting()
			} else {
				_ = in.Stop()
			}
		}

		for _, rs := range startedRulesets {
			_ = rs.Stop()
		}

		for _, a := range startedAgents {
			_ = a.Stop()
		}

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

	// 2. Start agent components
	agents := p.GetProjectAgents()
	for _, a := range agents {
		err := a.Start()
		if err != nil {
			cleanup()
			return fmt.Errorf("failed to start agent component %s: %w", a.Id, err)
		}
		startedAgents = append(startedAgents, a)
	}

	// 3. Start ruleset components (middle components in the pipeline)
	rulesets := p.GetProjectRulesets()
	for _, rs := range rulesets {
		err := rs.Start()
		if err != nil {
			cleanup() // Stop all previously started components
			return fmt.Errorf("failed to start ruleset component %s: %w", rs.RulesetID, err)
		}
		startedRulesets = append(startedRulesets, rs)
	}

	// 4. Start input components last (they will begin producing data immediately)
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
		"agents", len(startedAgents),
		"rulesets", len(startedRulesets),
		"inputs", len(startedInputs))

	return nil
}

// updateProjectStatusRedis writes status to Redis hash and publishes event with error handling
func updateProjectStatusRedis(projectID string, status common.Status, t time.Time) {
	nodeid := common.GetNodeID()
	if common.GetRedisClient() == nil {
		logger.Debug("Skipping project status Redis update because Redis client is not initialized",
			"project_id", projectID,
			"status", status)
		return
	}
	if strings.TrimSpace(nodeid) == "" {
		logger.Debug("Skipping project status Redis update because node id is empty",
			"project_id", projectID,
			"status", status)
		return
	}

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
