package project

import (
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/rules_engine"
	"fmt"
	"os"
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

	GlobalProject.ProjectsNew = make(map[string]string, 0)
	GlobalProject.InputsNew = make(map[string]string, 0)
	GlobalProject.OutputsNew = make(map[string]string, 0)
	GlobalProject.RulesetsNew = make(map[string]string, 0)

	GlobalProject.msgChans = make(map[string]chan map[string]interface{})
	GlobalProject.msgChansCounter = make(map[string]int)

	// 注册一个延迟函数，在所有项目加载完成后分析依赖关系
	go func() {
		// 等待一段时间，确保所有项目都已加载完成
		time.Sleep(5 * time.Second)
		// 分析项目依赖关系
		AnalyzeProjectDependencies()
	}()
}

func Verify(path string, raw string) error {
	var err error
	var cfg ProjectConfig
	var data []byte
	var p *Project

	if path != "" {
		data, err = os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read project configuration file: %w", err)
		}

		cfg.RawConfig = string(data)
	} else {
		cfg.RawConfig = raw
		data = []byte(raw)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		// 从错误信息中提取行号
		if yamlErr, ok := err.(*yaml.TypeError); ok && len(yamlErr.Errors) > 0 {
			errMsg := yamlErr.Errors[0]
			// 尝试提取行号
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
		return fmt.Errorf("project content cannot be empty in configuration file (line: unknown)")
	}

	p = &Project{
		Id:     cfg.Id,
		Status: ProjectStatusStopped,
		Config: &cfg,
	}

	_, err = p.parseContent()
	if err != nil {
		return fmt.Errorf("failed to parse project content: %v (line: unknown)", err)
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
		Status:      ProjectStatusRunning, // Default to running status for new projects
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
		return p, fmt.Errorf("failed to initialize project components: %w", err)
	}

	// Load saved status if available
	savedStatus, err := p.LoadProjectStatus()
	if err == nil {
		// Only update to stopped or error status
		// Running status will be handled by StartAllProject
		if savedStatus == ProjectStatusError {
			p.Status = ProjectStatusError
			logger.Warn("Project loaded with error status from previous run", "id", p.Id)
		} else if savedStatus == ProjectStatusStopped {
			p.Status = ProjectStatusStopped
			logger.Info("Project loaded with stopped status from previous run", "id", p.Id)
		}
	}

	// Save the initial status to file
	err = p.SaveProjectStatus()
	if err != nil {
		logger.Warn("Failed to save initial project status", "id", p.Id, "error", err)
	}

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
		Status:      ProjectStatusRunning,
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
		// 检查正式组件是否存在
		if _, ok := GlobalProject.Inputs[v]; !ok {
			// 检查是否为临时组件，临时组件不应该被引用
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

	projectNodeSequence := ""
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
			var msgChan chan map[string]interface{}
			projectNodeSequence = projectNodeSequence + "-" + fmt.Sprintf("%s-%s", from, to)

			if GlobalProject.msgChans[projectNodeSequence] != nil {
				msgChan = GlobalProject.msgChans[projectNodeSequence]
				GlobalProject.msgChansCounter[projectNodeSequence] = GlobalProject.msgChansCounter[projectNodeSequence] + 1
			} else {
				msgChan = make(chan map[string]interface{}, 1024)
				GlobalProject.msgChans[projectNodeSequence] = msgChan
				GlobalProject.msgChansCounter[projectNodeSequence] = 1
			}

			p.MsgChannels = append(p.MsgChannels, projectNodeSequence)

			// Connect based on component types
			switch fromType {
			case "INPUT":
				if in, ok := p.Inputs[fromId]; ok {
					in.DownStream = append(in.DownStream, &msgChan)
					in.ProjectNodeSequence = projectNodeSequence
				}
			case "RULESET":
				if rs, ok := p.Rulesets[fromId]; ok {
					rs.DownStream[to] = &msgChan
					rs.ProjectNodeSequence = projectNodeSequence
				}
			}

			switch toType {
			case "RULESET":
				if rs, ok := p.Rulesets[toId]; ok {
					rs.UpStream[from] = &msgChan
					rs.ProjectNodeSequence = projectNodeSequence
				}
			case "OUTPUT":
				if out, ok := p.Outputs[toId]; ok {
					out.UpStream = append(out.UpStream, &msgChan)
					out.ProjectNodeSequence = projectNodeSequence
				}
			}
		}
	}

	return nil
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

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "->")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid line: %q", line)
		}

		from := strings.TrimSpace(parts[0])
		to := strings.TrimSpace(parts[1])

		// Validate node types
		fromType, _ := parseNode(from)
		toType, _ := parseNode(to)

		if fromType == "" || toType == "" {
			return nil, fmt.Errorf("invalid node format: %s -> %s", from, to)
		}

		// Validate flow rules
		if toType == "INPUT" {
			return nil, fmt.Errorf("INPUT node %q cannot be a destination", to)
		}
		if fromType == "OUTPUT" {
			return nil, fmt.Errorf("OUTPUT node %q cannot be a source", from)
		}

		// 检查是否有重复的流向
		edgeKey := from + "->" + to
		if _, exists := edgeSet[edgeKey]; exists {
			return nil, fmt.Errorf("duplicate data flow detected: %s", edgeKey)
		}
		edgeSet[edgeKey] = struct{}{}

		// Add to flow graph
		flowGraph[from] = append(flowGraph[from], to)
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

// Start starts the project and all its components
func (p *Project) Start() error {
	if p.Status == ProjectStatusRunning {
		return fmt.Errorf("project is already running %s", p.Id)
	}
	if p.Status == ProjectStatusError {
		return fmt.Errorf("project is error %s %s", p.Id, p.Err.Error())
	}

	// Start inputs
	for _, in := range p.Inputs {
		if err := in.Start(); err != nil {
			p.Status = ProjectStatusError
			p.Err = err
			// Save the error status to file
			_ = p.SaveProjectStatus()
			return fmt.Errorf("failed to start input %s: %v", in.Id, err)
		}
	}

	// Start rulesets
	for _, rs := range p.Rulesets {
		if err := rs.Start(); err != nil {
			p.Status = ProjectStatusError
			p.Err = err
			// Save the error status to file
			_ = p.SaveProjectStatus()
			return fmt.Errorf("failed to start ruleset %s: %v", rs.RulesetID, err)
		}
	}

	// Start outputs
	for _, out := range p.Outputs {
		if err := out.Start(); err != nil {
			p.Status = ProjectStatusError
			p.Err = err
			// Save the error status to file
			_ = p.SaveProjectStatus()
			return fmt.Errorf("failed to start output %s: %v", out.Id, err)
		}
	}

	// Start metrics collection
	p.metricsStop = make(chan struct{})
	go p.collectMetrics()

	p.Status = ProjectStatusRunning

	// Save the running status to file
	err := p.SaveProjectStatus()
	if err != nil {
		logger.Warn("Failed to save project status", "id", p.Id, "error", err)
	}

	return nil
}

// Stop stops the project and all its components
func (p *Project) Stop() error {
	if p.Status != ProjectStatusRunning {
		return fmt.Errorf("project is not running %s", p.Id)
	}

	// Check if project is in error state
	if p.Err != nil {
		logger.Warn("Stopping project with errors", "id", p.Id, "error", p.Err)
	}

	// Stop all components
	for _, in := range p.Inputs {
		if err := in.Stop(); err != nil {
			return fmt.Errorf("failed to stop input %s: %v", in.Id, err)
		}
	}

	for _, rs := range p.Rulesets {
		if err := rs.Stop(); err != nil {
			return fmt.Errorf("failed to stop ruleset %s: %v", rs.RulesetID, err)
		}
	}

	for _, out := range p.Outputs {
		if err := out.Stop(); err != nil {
			return fmt.Errorf("failed to stop output %s: %v", out.Id, err)
		}
	}

	// Stop metrics collection
	if p.metricsStop != nil {
		close(p.metricsStop)
	}

	for i := range p.MsgChannels {
		id := p.MsgChannels[i]
		GlobalProject.msgChansCounter[id] = GlobalProject.msgChansCounter[id] - 1
		if GlobalProject.msgChansCounter[id] == 0 {
			close(GlobalProject.msgChans[id])
			delete(GlobalProject.msgChans, id)
			delete(GlobalProject.msgChansCounter, id)
		}
	}

	// Close all channels
	close(p.stopChan)

	// Wait for all goroutines to finish
	p.wg.Wait()

	p.Status = ProjectStatusStopped

	// Save the stopped status to file
	err := p.SaveProjectStatus()
	if err != nil {
		logger.Warn("Failed to save project status", "id", p.Id, "error", err)
	}

	return nil
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
