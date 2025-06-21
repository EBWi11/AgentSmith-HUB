package project

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/rules_engine"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var GlobalProject *GlobalProjectInfo

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
	GlobalProject.msgChansCounter = make(map[string]int)

	// Register a delayed function to analyze dependencies after all projects are loaded
	go func() {
		// Wait for a while to ensure all projects are loaded
		time.Sleep(5 * time.Second)
		// Analyze project dependencies
		AnalyzeProjectDependencies()
	}()
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
		// Extract line number from error message
		if yamlErr, ok := err.(*yaml.TypeError); ok && len(yamlErr.Errors) > 0 {
			errMsg := yamlErr.Errors[0]
			// Try to extract line number
			lineInfo := ""
			for _, line := range yamlErr.Errors {
				if strings.Contains(line, "line") {
					lineInfo = line
					break
				}
			}
			return fmt.Errorf("failed to parse project configuration: %s (location: %s)", errMsg, lineInfo)
		}
		return fmt.Errorf("failed to parse project configuration: %w", err)
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

	p := &Project{
		Id:          cfg.Id,
		Status:      ProjectStatusStopped, // Default to stopped status, will be started by StartAllProject
		Config:      &cfg,
		Inputs:      make(map[string]*input.Input),
		Outputs:     make(map[string]*output.Output),
		Rulesets:    make(map[string]*rules_engine.Ruleset),
		MsgChannels: make([]string, 0),
		stopChan:    make(chan struct{}),
		metrics: &ProjectMetrics{
			InputQPS:  make(map[string]uint64),
			OutputQPS: make(map[string]uint64),
		},
	}

	// Initialize components
	if err := p.initComponents(); err != nil {
		p.Status = ProjectStatusError
		p.Err = err

		// Save the error status to file
		if saveErr := p.SaveProjectStatus(); saveErr != nil {
			logger.Warn("Failed to save error project status", "id", p.Id, "error", saveErr)
		}

		return p, fmt.Errorf("failed to initialize project components: %w", err)
	}

	// IMPORTANT: After hub restart, all projects start with STOPPED status
	// The .project_status file only records user intention, not current actual status
	// Current actual status is always STOPPED after hub restart
	p.Status = ProjectStatusStopped
	logger.Info("Project created with stopped status (actual status after hub restart)", "id", p.Id, "status", p.Status)

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

	p := &Project{
		Id:          cfg.Id,
		Status:      ProjectStatusStopped, // Start as stopped for testing
		Config:      &cfg,
		Inputs:      make(map[string]*input.Input),
		Outputs:     make(map[string]*output.Output),
		Rulesets:    make(map[string]*rules_engine.Ruleset),
		MsgChannels: make([]string, 0),
		stopChan:    make(chan struct{}),
		metrics: &ProjectMetrics{
			InputQPS:  make(map[string]uint64),
			OutputQPS: make(map[string]uint64),
		},
	}

	// Initialize components with independent instances for testing
	if err := p.initComponentsForTesting(); err != nil {
		p.Status = ProjectStatusError
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
		// 检查正式组件是否存在
		if _, ok := GlobalProject.Outputs[v]; !ok {
			// 检查是否为临时组件，临时组件不应该被引用
			if _, tempExists := GlobalProject.OutputsNew[v]; tempExists {
				return fmt.Errorf("cannot reference temporary output component '%s', please save it first", v)
			}
			return fmt.Errorf("conn't find output %s", v)
		}
	}

	for _, v := range rulesetNames {
		// 检查正式组件是否存在
		if _, ok := GlobalProject.Rulesets[v]; !ok {
			// 检查是否为临时组件，临时组件不应该被引用
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
		// Create a minimal input placeholder for testing
		p.Inputs[name] = &input.Input{
			Id:         name,
			DownStream: make([]*chan map[string]interface{}, 0),
		}
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

		// Create a new independent output instance
		testOutput, err := output.NewOutput("", outputConfig, "test_"+name)
		if err != nil {
			return fmt.Errorf("failed to create test output %s: %v", name, err)
		}
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

		// Create a new independent ruleset instance
		testRuleset, err := rules_engine.NewRuleset("", rulesetConfig, "test_"+name)
		if err != nil {
			return fmt.Errorf("failed to create test ruleset %s: %v", name, err)
		}
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
	edgeSet := make(map[string]struct{}) // 用于检测重复流向

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

		// 检查是否有重复的流向
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
	if p.Status == ProjectStatusRunning {
		return fmt.Errorf("project is already running %s", p.Id)
	}
	if p.Status == ProjectStatusError {
		return fmt.Errorf("project is error %s %s", p.Id, p.Err.Error())
	}

	// Initialize project control channels
	p.stopChan = make(chan struct{})

	// Parse project content to get component flow
	flowGraph, err := p.parseContent()
	if err != nil {
		p.Status = ProjectStatusError
		p.Err = err
		return fmt.Errorf("failed to parse project content: %v", err)
	}

	// Load components from global registry
	err = p.loadComponentsFromGlobal(flowGraph)
	if err != nil {
		p.Status = ProjectStatusError
		p.Err = err
		return fmt.Errorf("failed to load components: %v", err)
	}

	// Create fresh channel connections
	err = p.createChannelConnections(flowGraph)
	if err != nil {
		p.Status = ProjectStatusError
		p.Err = err
		return fmt.Errorf("failed to create channel connections: %v", err)
	}

	// Use centralized component usage counter for better performance and code maintainability

	// Start inputs first - only if not already running in other projects
	for _, in := range p.Inputs {
		runningCount := UsageCounter.CountProjectsUsingInput(in.Id, p.Id)
		if runningCount == 0 {
			// No other project is using this input - start it
			logger.Info("Starting shared input component", "project", p.Id, "input", in.Id, "running_projects", runningCount)
			if err := in.Start(); err != nil {
				p.Status = ProjectStatusError
				p.Err = err
				_ = p.SaveProjectStatus()
				return fmt.Errorf("failed to start shared input %s: %v", in.Id, err)
			}
		} else {
			logger.Info("Reusing already running input component", "project", p.Id, "input", in.Id, "running_projects", runningCount)
		}
	}

	// Start rulesets after inputs - only if not already running in other projects
	for _, rs := range p.Rulesets {
		runningCount := UsageCounter.CountProjectsUsingRuleset(rs.RulesetID, p.Id)
		if runningCount == 0 {
			// No other project is using this ruleset - start it
			logger.Info("Starting shared ruleset component", "project", p.Id, "ruleset", rs.RulesetID, "running_projects", runningCount)
			if err := rs.Start(); err != nil {
				p.Status = ProjectStatusError
				p.Err = err
				_ = p.SaveProjectStatus()
				return fmt.Errorf("failed to start shared ruleset %s: %v", rs.RulesetID, err)
			}
		} else {
			logger.Info("Reusing already running ruleset component", "project", p.Id, "ruleset", rs.RulesetID, "running_projects", runningCount)
		}
	}

	// Start outputs last - only if not already running in other projects
	for _, out := range p.Outputs {
		runningCount := UsageCounter.CountProjectsUsingOutput(out.Id, p.Id)
		if runningCount == 0 {
			// No other project is using this output - start it
			logger.Info("Starting shared output component", "project", p.Id, "output", out.Id, "running_projects", runningCount)
			if err := out.Start(); err != nil {
				p.Status = ProjectStatusError
				p.Err = err
				_ = p.SaveProjectStatus()
				return fmt.Errorf("failed to start shared output %s: %v", out.Id, err)
			}
		} else {
			logger.Info("Reusing already running output component", "project", p.Id, "output", out.Id, "running_projects", runningCount)
		}
	}

	// Start metrics collection
	p.metricsStop = make(chan struct{})
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.collectMetrics()
	}()

	p.Status = ProjectStatusRunning

	// Save the running status to file
	err = p.SaveProjectStatus()
	if err != nil {
		logger.Warn("Failed to save project status", "id", p.Id, "error", err)
	}

	logger.Info("Project started successfully with shared components", "project", p.Id)
	return nil
}

// Stop stops the project and all its components in proper order
func (p *Project) Stop() error {
	if p.Status != ProjectStatusRunning {
		return fmt.Errorf("project is not running %s", p.Id)
	}

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

		// Use centralized component usage counter for better performance and code maintainability

		// Step 1: Stop inputs first to prevent new data (only if not used by other projects)
		logger.Info("Step 1: Stopping inputs", "project", p.Id, "count", len(p.Inputs))
		for _, in := range p.Inputs {
			otherProjectsUsing := UsageCounter.CountProjectsUsingInput(in.Id, p.Id)
			if otherProjectsUsing == 0 {
				logger.Info("Stopping input component", "project", p.Id, "input", in.Id, "other_projects_using", otherProjectsUsing)
				startTime := time.Now()
				if err := in.Stop(); err != nil {
					logger.Error("Failed to stop input", "project", p.Id, "input", in.Id, "error", err)
					// Continue with other inputs instead of failing immediately
				} else {
					logger.Info("Stopped input", "project", p.Id, "input", in.Id, "duration", time.Since(startTime))
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
			otherProjectsUsing := UsageCounter.CountProjectsUsingRuleset(rs.RulesetID, p.Id)
			if otherProjectsUsing == 0 {
				logger.Info("Stopping ruleset component", "project", p.Id, "ruleset", rs.RulesetID, "other_projects_using", otherProjectsUsing)
				startTime := time.Now()
				if err := rs.Stop(); err != nil {
					logger.Error("Failed to stop ruleset", "project", p.Id, "ruleset", rs.RulesetID, "error", err)
					// Continue with other rulesets instead of failing immediately
				} else {
					logger.Info("Stopped ruleset", "project", p.Id, "ruleset", rs.RulesetID, "duration", time.Since(startTime))
				}
			} else {
				logger.Info("Ruleset component still used by other projects, skipping stop", "project", p.Id, "ruleset", rs.RulesetID, "other_projects_using", otherProjectsUsing)
			}
		}

		// Step 4: Stop outputs last (only if not used by other projects)
		logger.Info("Step 4: Stopping outputs", "project", p.Id, "count", len(p.Outputs))
		for _, out := range p.Outputs {
			otherProjectsUsing := UsageCounter.CountProjectsUsingOutput(out.Id, p.Id)
			if otherProjectsUsing == 0 {
				logger.Info("Stopping output component", "project", p.Id, "output", out.Id, "other_projects_using", otherProjectsUsing)
				startTime := time.Now()
				if err := out.Stop(); err != nil {
					logger.Error("Failed to stop output", "project", p.Id, "output", out.Id, "error", err)
					// Continue with other outputs instead of failing immediately
				} else {
					logger.Info("Stopped output", "project", p.Id, "output", out.Id, "duration", time.Since(startTime))
				}
			} else {
				logger.Info("Output component still used by other projects, skipping stop", "project", p.Id, "output", out.Id, "other_projects_using", otherProjectsUsing)
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
			if GlobalProject.msgChansCounter[channelId] > 0 {
				GlobalProject.msgChansCounter[channelId]--
				if GlobalProject.msgChansCounter[channelId] == 0 {
					if ch, exists := GlobalProject.msgChans[channelId]; exists {
						close(ch)
						delete(GlobalProject.msgChans, channelId)
						delete(GlobalProject.msgChansCounter, channelId)
						logger.Info("Closed and cleaned up channel", "project", p.Id, "channel", channelId)
					}
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

		p.Status = ProjectStatusStopped

		// Save the stopped status to file
		err := p.SaveProjectStatus()
		if err != nil {
			logger.Warn("Failed to save project status", "id", p.Id, "error", err)
		}

		stopCompleted <- nil
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
		p.Status = ProjectStatusError

		// Save the error status to file
		if err := p.SaveProjectStatus(); err != nil {
			logger.Warn("Failed to save error project status after timeout", "id", p.Id, "error", err)
		}

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

// 在文件中添加一个新函数，用于分析项目依赖关系
func AnalyzeProjectDependencies() {
	// 清除所有项目的依赖关系
	for _, p := range GlobalProject.Projects {
		p.DependsOn = []string{}
		p.DependedBy = []string{}
		p.SharedInputs = []string{}
		p.SharedOutputs = []string{}
		p.SharedRulesets = []string{}
	}

	// 构建组件使用映射
	inputUsage := make(map[string][]string)   // 输入组件ID -> 使用它的项目ID列表
	outputUsage := make(map[string][]string)  // 输出组件ID -> 使用它的项目ID列表
	rulesetUsage := make(map[string][]string) // 规则集ID -> 使用它的项目ID列表

	// 分析每个项目使用的组件
	for projectID, p := range GlobalProject.Projects {
		// 记录输入组件使用情况
		for inputID := range p.Inputs {
			inputUsage[inputID] = append(inputUsage[inputID], projectID)
		}

		// 记录输出组件使用情况
		for outputID := range p.Outputs {
			outputUsage[outputID] = append(outputUsage[outputID], projectID)
		}

		// 记录规则集使用情况
		for rulesetID := range p.Rulesets {
			rulesetUsage[rulesetID] = append(rulesetUsage[rulesetID], projectID)
		}
	}

	// 更新共享组件信息
	for inputID, projects := range inputUsage {
		if len(projects) > 1 {
			// 这是一个共享输入组件
			for _, projectID := range projects {
				GlobalProject.Projects[projectID].SharedInputs = append(
					GlobalProject.Projects[projectID].SharedInputs,
					inputID,
				)
			}
		}
	}

	for outputID, projects := range outputUsage {
		if len(projects) > 1 {
			// 这是一个共享输出组件
			for _, projectID := range projects {
				GlobalProject.Projects[projectID].SharedOutputs = append(
					GlobalProject.Projects[projectID].SharedOutputs,
					outputID,
				)
			}
		}
	}

	for rulesetID, projects := range rulesetUsage {
		if len(projects) > 1 {
			// 这是一个共享规则集
			for _, projectID := range projects {
				GlobalProject.Projects[projectID].SharedRulesets = append(
					GlobalProject.Projects[projectID].SharedRulesets,
					rulesetID,
				)
			}
		}
	}

	// 分析项目之间的依赖关系
	for projectID, p := range GlobalProject.Projects {
		// 解析项目配置以获取数据流
		flowGraph, err := p.parseContent()
		if err != nil {
			logger.Error("Failed to parse project content", "id", projectID, "error", err)
			continue
		}

		// 分析数据流中的项目间依赖
		for fromNode, toNodes := range flowGraph {
			fromType, fromID := parseNode(fromNode)

			// 检查是否存在跨项目依赖
			for _, toNode := range toNodes {
				toType, toID := parseNode(toNode)

				// 如果源节点是一个项目的输出，目标节点是另一个项目的输入，则存在项目间依赖
				if fromType == "OUTPUT" && toType == "INPUT" {
					// 找出拥有这些组件的项目
					var fromProjectID, toProjectID string

					// 查找拥有源输出的项目
					for pid, proj := range GlobalProject.Projects {
						if _, exists := proj.Outputs[fromID]; exists {
							fromProjectID = pid
							break
						}
					}

					// 查找拥有目标输入的项目
					for pid, proj := range GlobalProject.Projects {
						if _, exists := proj.Inputs[toID]; exists {
							toProjectID = pid
							break
						}
					}

					// 如果找到了两个不同的项目，则存在项目间依赖
					if fromProjectID != "" && toProjectID != "" && fromProjectID != toProjectID {
						// 更新依赖关系
						GlobalProject.Projects[toProjectID].DependsOn = append(
							GlobalProject.Projects[toProjectID].DependsOn,
							fromProjectID,
						)
						GlobalProject.Projects[fromProjectID].DependedBy = append(
							GlobalProject.Projects[fromProjectID].DependedBy,
							toProjectID,
						)
					}
				}
			}
		}
	}

	// 记录依赖关系信息
	for projectID, p := range GlobalProject.Projects {
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
func (p *Project) SaveProjectStatus() error {
	statusFile := ".project_status"

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

	return nil
}

// LoadProjectStatus loads the project status from a file
func (p *Project) LoadProjectStatus() (ProjectStatus, error) {
	statusFile := ".project_status"

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

// StartForTesting starts the project for testing purposes, bypassing normal status checks
func (p *Project) StartForTesting() error {
	// Force set status to stopped to allow starting
	p.Status = ProjectStatusStopped

	// Start inputs
	for _, in := range p.Inputs {
		if err := in.Start(); err != nil {
			p.Status = ProjectStatusError
			p.Err = err
			return fmt.Errorf("failed to start input %s: %v", in.Id, err)
		}
	}

	// Start rulesets
	for _, rs := range p.Rulesets {
		if err := rs.Start(); err != nil {
			p.Status = ProjectStatusError
			p.Err = err
			return fmt.Errorf("failed to start ruleset %s: %v", rs.RulesetID, err)
		}
	}

	// Start outputs in test mode
	for _, out := range p.Outputs {
		if err := out.StartForTesting(); err != nil {
			p.Status = ProjectStatusError
			p.Err = err
			return fmt.Errorf("failed to start output %s: %v", out.Id, err)
		}
	}

	// Start metrics collection
	p.metricsStop = make(chan struct{})
	go p.collectMetrics()

	p.Status = ProjectStatusRunning
	return nil
}

// StopForTesting stops the project quickly for testing purposes
func (p *Project) StopForTesting() error {
	// Quick stop without extensive timeouts for testing
	logger.Info("Quick stopping test project", "project", p.Id)

	// Stop metrics collection first
	if p.metricsStop != nil {
		close(p.metricsStop)
		p.metricsStop = nil
	}

	// Stop components quickly without waiting for channel drainage
	for _, in := range p.Inputs {
		if err := in.Stop(); err != nil {
			logger.Warn("Failed to stop test input", "project", p.Id, "input", in.Id, "error", err)
		}
	}

	for _, rs := range p.Rulesets {
		// Use the quick stop method for rulesets in testing
		if err := rs.StopForTesting(); err != nil {
			logger.Warn("Failed to stop test ruleset", "project", p.Id, "ruleset", rs.RulesetID, "error", err)
		}
	}

	for _, out := range p.Outputs {
		// Use the quick stop method for outputs in testing
		if err := out.StopForTesting(); err != nil {
			logger.Warn("Failed to stop test output", "project", p.Id, "output", out.Id, "error", err)
		}
	}

	p.Status = ProjectStatusStopped
	logger.Info("Test project stopped", "project", p.Id)
	return nil
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

	// Load components from global registry
	for _, name := range inputNames {
		if globalInput, ok := GlobalProject.Inputs[name]; ok {
			p.Inputs[name] = globalInput
		} else {
			return fmt.Errorf("input component %s not found in global registry", name)
		}
	}

	for _, name := range outputNames {
		if globalOutput, ok := GlobalProject.Outputs[name]; ok {
			p.Outputs[name] = globalOutput
		} else {
			return fmt.Errorf("output component %s not found in global registry", name)
		}
	}

	for _, name := range rulesetNames {
		if globalRuleset, ok := GlobalProject.Rulesets[name]; ok {
			p.Rulesets[name] = globalRuleset
		} else {
			return fmt.Errorf("ruleset component %s not found in global registry", name)
		}
	}

	logger.Info("Components loaded from global registry", "project", p.Id, "inputs", len(inputNames), "outputs", len(outputNames), "rulesets", len(rulesetNames))
	return nil
}

// createChannelConnections creates fresh channel connections between components
func (p *Project) createChannelConnections(flowGraph map[string][]string) error {
	logger.Info("Creating channel connections", "project", p.Id)

	// Build ProjectNodeSequence for each component based on data flow paths
	// This enables component reuse across different logical projects
	componentSequences := make(map[string]string)

	// First, find all components that have no upstream (entry points)
	hasUpstream := make(map[string]bool)
	for _, tos := range flowGraph {
		for _, to := range tos {
			hasUpstream[to] = true
		}
	}

	// Build ProjectNodeSequence recursively
	var buildSequence func(component string, visited map[string]bool) string
	buildSequence = func(component string, visited map[string]bool) string {
		// Check for cycles
		if visited[component] {
			return component // Break cycle
		}

		// If already computed, return cached result
		if seq, exists := componentSequences[component]; exists {
			return seq
		}

		// Mark as visited
		visited[component] = true
		defer delete(visited, component)

		// Find upstream components
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
			// No upstream, this is an entry point
			sequence = component
		} else {
			// Has upstream, build sequence
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

	// Set ProjectNodeSequence for each component
	for _, in := range p.Inputs {
		in.DownStream = []*chan map[string]interface{}{}
		componentKey := "input." + in.Id
		if sequence, exists := componentSequences[componentKey]; exists {
			in.ProjectNodeSequence = sequence
		} else {
			in.ProjectNodeSequence = componentKey
		}
	}

	for _, rs := range p.Rulesets {
		rs.UpStream = make(map[string]*chan map[string]interface{})
		rs.DownStream = make(map[string]*chan map[string]interface{})
		componentKey := "ruleset." + rs.RulesetID
		if sequence, exists := componentSequences[componentKey]; exists {
			rs.ProjectNodeSequence = sequence
		} else {
			rs.ProjectNodeSequence = componentKey
		}
	}

	for _, out := range p.Outputs {
		out.UpStream = []*chan map[string]interface{}{}
		componentKey := "output." + out.Id
		if sequence, exists := componentSequences[componentKey]; exists {
			out.ProjectNodeSequence = sequence
		} else {
			out.ProjectNodeSequence = componentKey
		}
	}

	// Create new channel connections
	for from, tos := range flowGraph {
		fromParts := strings.Split(from, ".")
		fromType := fromParts[0]
		fromId := fromParts[1]

		for _, to := range tos {
			toParts := strings.Split(to, ".")
			toType := toParts[0]
			toId := toParts[1]

			// Create a unique channel ID for this connection
			channelId := fmt.Sprintf("%s_%s_%s_%s", p.Id, from, to, time.Now().Format("20060102150405"))

			// Create a new channel
			msgChan := make(chan map[string]interface{}, 1024)
			GlobalProject.msgChans[channelId] = msgChan
			GlobalProject.msgChansCounter[channelId] = 1
			p.MsgChannels = append(p.MsgChannels, channelId)

			// Connect components based on types
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

			logger.Info("Created channel connection", "project", p.Id, "from", from, "to", to, "channel", channelId)
		}
	}

	// Log the final ProjectNodeSequence for each component
	for id, in := range p.Inputs {
		logger.Info("Input component sequence", "project", p.Id, "input", id, "ProjectNodeSequence", in.ProjectNodeSequence)
	}
	for id, rs := range p.Rulesets {
		logger.Info("Ruleset component sequence", "project", p.Id, "ruleset", id, "ProjectNodeSequence", rs.ProjectNodeSequence)
	}
	for id, out := range p.Outputs {
		logger.Info("Output component sequence", "project", p.Id, "output", id, "ProjectNodeSequence", out.ProjectNodeSequence)
	}

	logger.Info("Channel connections created", "project", p.Id, "total_channels", len(p.MsgChannels))
	return nil
}

// RestartProjectsSafely restarts multiple projects with proper shared component handling
// Returns the number of successfully restarted projects
func RestartProjectsSafely(projectIDs []string) (int, error) {
	if len(projectIDs) == 0 {
		return 0, nil
	}

	logger.Info("Starting batch project restart", "count", len(projectIDs))
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
				continue // Skip starting if stop failed
			}
			logger.Info("Stopped project during restart", "id", projectID)

			// Start the project (respects shared components)
			if err := proj.Start(); err != nil {
				logger.Error("Failed to start project during restart", "id", projectID, "error", err)
			} else {
				restartedCount++
				logger.Info("Successfully restarted project", "id", projectID, "duration", time.Since(startTime))
			}
		} else {
			logger.Info("Skipping project restart (not running)", "id", projectID, "status", proj.Status)
		}
	}

	logger.Info("Batch project restart completed", "total_affected", len(projectIDs), "restarted", restartedCount)
	return restartedCount, nil
}

// RestartSingleProjectSafely restarts a single project with proper error handling
func RestartSingleProjectSafely(projectID string) error {
	common.GlobalMu.RLock()
	proj, exists := GlobalProject.Projects[projectID]
	common.GlobalMu.RUnlock()

	if !exists {
		return fmt.Errorf("project not found: %s", projectID)
	}

	if proj.Status != ProjectStatusRunning {
		return fmt.Errorf("project is not running: %s (status: %s)", projectID, proj.Status)
	}

	logger.Info("Restarting single project", "id", projectID)
	startTime := time.Now()

	// Stop the project (respects shared components)
	if err := proj.Stop(); err != nil {
		return fmt.Errorf("failed to stop project %s: %w", projectID, err)
	}
	logger.Info("Stopped project for restart", "id", projectID)

	// Start the project (respects shared components)
	if err := proj.Start(); err != nil {
		return fmt.Errorf("failed to start project %s: %w", projectID, err)
	}

	logger.Info("Successfully restarted single project", "id", projectID, "duration", time.Since(startTime))
	return nil
}
