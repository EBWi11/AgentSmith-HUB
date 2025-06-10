package project

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/rules_engine"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var GlobalProject *GlobalProjectInfo

func init() {
	GlobalProject = &GlobalProjectInfo{}
	GlobalProject.Projects = make(map[string]*Project)
	GlobalProject.msgChans = make(map[string]chan map[string]interface{})
	GlobalProject.msgChansCounter = make(map[string]int)
}

// NewProject creates a new project instance from a configuration file
// pp: Path to the project configuration file
func NewProject(pp string, raw string, id string) (*Project, error) {
	var err error
	var cfg ProjectConfig
	var data []byte

	if pp != "" {
		data, err = os.ReadFile(pp)
		if err != nil {
			return nil, fmt.Errorf("failed to read project configuration file: %w", err)
		}

		cfg.RawConfig = string(data)
		cfg.Id = common.GetFileNameWithoutExt(pp)
	} else {
		cfg.RawConfig = raw
		cfg.Id = id
		data = []byte(raw)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse project configuration: %w", err)
	}

	if strings.TrimSpace(cfg.Id) == "" {
		return nil, fmt.Errorf("project ID cannot be empty in configuration file: %s", pp)
	}
	if strings.TrimSpace(cfg.Content) == "" {
		return nil, fmt.Errorf("project content cannot be empty in configuration file: %s", pp)
	}

	p := &Project{
		Id:          cfg.Id,
		Status:      ProjectStatusStopped,
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
		return nil, fmt.Errorf("failed to initialize project components: %w", err)
	}

	GlobalProject.Projects[p.Id] = p

	return p, nil
}

// loadComponents loads and initializes all project components
// inputNames: List of input component IDs
// outputNames: List of output component IDs
// rulesetNames: List of ruleset IDs
func (p *Project) loadComponents(inputNames []string, outputNames []string, rulesetNames []string) error {
	var err error
	if cluster.IsLeader {
		for _, v := range inputNames {
			p.Inputs[v], err = input.NewInput(path.Join(common.Config.ConfigRoot, "input", v+".yaml"), "", v)
			if err != nil {
				return fmt.Errorf("failed to initialize input component %s: %w", v, err)
			}
		}
		for _, v := range outputNames {
			p.Outputs[v], err = output.NewOutput(path.Join(common.Config.ConfigRoot, "output", v+".yaml"), "", v)
			if err != nil {
				return fmt.Errorf("failed to initialize output component %s: %w", v, err)
			}
		}
		for _, v := range rulesetNames {
			p.Rulesets[v], err = rules_engine.NewRuleset(path.Join(common.Config.ConfigRoot, "ruleset", v+".xml"), "", v)
			if err != nil {
				return fmt.Errorf("failed to initialize ruleset %s: %w", v, err)
			}
		}
	} else {
		for _, v := range inputNames {
			p.Inputs[v], err = input.NewInput("", common.AllInputsRawConfig[v], v)
			if err != nil {
				return fmt.Errorf("failed to initialize input component %s: %w", v, err)
			}
		}
		for _, v := range outputNames {
			p.Outputs[v], err = output.NewOutput("", common.AllOutputsRawConfig[v], v)
			if err != nil {
				return fmt.Errorf("failed to initialize output component %s: %w", v, err)
			}
		}
		for _, v := range rulesetNames {
			p.Rulesets[v], err = rules_engine.NewRuleset("", common.AllRulesetsRawConfig[v], v)
			if err != nil {
				return fmt.Errorf("failed to initialize ruleset %s: %w", v, err)
			}
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

	// Start inputs
	for _, in := range p.Inputs {
		if err := in.Start(); err != nil {
			p.Status = ProjectStatusError
			return fmt.Errorf("failed to start input %s: %v", in.Id, err)
		}
	}

	// Start rulesets
	for _, rs := range p.Rulesets {
		if err := rs.Start(); err != nil {
			p.Status = ProjectStatusError
			return fmt.Errorf("failed to start ruleset %s: %v", rs.RulesetID, err)
		}
	}

	// Start outputs
	for _, out := range p.Outputs {
		if err := out.Start(); err != nil {
			p.Status = ProjectStatusError
			return fmt.Errorf("failed to start output %s: %v", out.Id, err)
		}
	}

	// Start metrics collection
	p.metricsStop = make(chan struct{})
	go p.collectMetrics()

	p.Status = ProjectStatusRunning
	return nil
}

// Stop stops the project and all its components
func (p *Project) Stop() error {
	if p.Status != ProjectStatusRunning {
		return fmt.Errorf("project is not running ", p.Id)
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

// GetProjects returns a list of all projects
func GetProjects() []*Project {
	if GlobalProject == nil {
		return []*Project{}
	}

	projects := make([]*Project, 0, len(GlobalProject.Projects))
	for _, p := range GlobalProject.Projects {
		projects = append(projects, p)
	}
	return projects
}

// GetProject returns a project by ID
func GetProject(id string) *Project {
	if GlobalProject == nil {
		return nil
	}

	return GlobalProject.Projects[id]
}
