package project

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/rules_engine"
	"fmt"
	"sync"
	"time"
)

type FlowNode struct {
	FromPNS  string
	ToPNS    string
	Content  string
	FromType string
	ToType   string
	FromID   string
	ToID     string
	FromInit bool
	ToInit   bool
}

type GlobalProjectInfo struct {
	Projects map[string]*Project
	Inputs   map[string]*input.Input
	Outputs  map[string]*output.Output
	Rulesets map[string]*rules_engine.Ruleset

	PNSOutputs  map[string]*output.Output
	PNSRulesets map[string]*rules_engine.Ruleset

	ProjectsNew map[string]string
	InputsNew   map[string]string
	OutputsNew  map[string]string
	RulesetsNew map[string]string
}

// CalculateRefCount dynamically calculates how many running projects are using the given PNS
// excludeProjectID allows excluding a specific project from the count (useful during stopping)
func CalculateRefCount(pns string, excludeProjectID ...string) int {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	return CalculateRefCountUnsafe(pns, excludeProjectID...)
}

// CalculateRefCountUnsafe performs the same calculation as CalculateRefCount but without acquiring locks
// This version should only be called when the caller already holds the appropriate locks
func CalculateRefCountUnsafe(pns string, excludeProjectID ...string) int {
	var excludeID string
	if len(excludeProjectID) > 0 {
		excludeID = excludeProjectID[0]
	}

	count := 0
	for projectID, proj := range GlobalProject.Projects {
		// Skip excluded project
		if projectID == excludeID {
			continue
		}

		// Only count running projects
		if proj.Status != common.StatusRunning {
			continue
		}

		for _, node := range proj.FlowNodes {
			if node.FromPNS == pns || node.ToPNS == pns {
				count++
				break // Each project should only be counted once per PNS
			}
		}
	}
	return count
}

// GetRefCount is kept for backward compatibility, now uses dynamic calculation
func GetRefCount(id string) int {
	return CalculateRefCount(id)
}

// ProjectConfig holds the configuration for a project
type ProjectConfig struct {
	Id        string
	Content   string `yaml:"content"`
	RawConfig string
	Path      string
}

// Project represents a project
type Project struct {
	Id              string        `json:"id"`
	Status          common.Status `json:"status"`
	StatusChangedAt *time.Time    `json:"status_changed_at,omitempty"`
	Err             error         `json:"-"`

	Testing bool `json:"testing"`

	Config *ProjectConfig `json:"config"`

	FlowNodes       []FlowNode
	BackUpFlowNodes []FlowNode

	// Components - these are now treated as temporary caches during initialization/running
	// They should not be relied upon for consistency checks - use the dynamic getters instead
	Inputs   map[string]*input.Input          `json:"-"`
	Outputs  map[string]*output.Output        `json:"-"`
	Rulesets map[string]*rules_engine.Ruleset `json:"-"`

	// Data flow
	MsgChannels map[string]*chan map[string]interface{} `json:"-"` // Channels for message passing between components

	// Restart cooldown
	lastRestartTime time.Time
	restartMu       sync.Mutex

	// Stop signal for graceful shutdown coordination
	stopChan chan struct{} `json:"-"`
	stopOnce sync.Once     `json:"-"`
}

// atomicStatusTransition performs atomic status checking and transition
func (p *Project) atomicStatusTransition(allowedFrom []common.Status, newStatus common.Status) bool {
	// Note: This function assumes the caller already holds appropriate locks
	for _, allowed := range allowedFrom {
		if p.Status == allowed {
			p.SetProjectStatus(newStatus, nil)
			return true
		}
	}
	return false
}

// ProjectMetrics holds runtime metrics for the project
type ProjectMetrics struct {
	InputQPS  map[string]uint64
	OutputQPS map[string]uint64
	mu        sync.RWMutex
}

// ProjectNode interface
type ProjectNode interface {
	Start() error
	Stop() error
}

// ===== Thread-safe accessor functions for GlobalProject =====

// Project dynamic component accessors - these replace the removed project-level component maps
// GetProjectInputs returns all inputs used by this project, dynamically calculated from FlowNodes
func (p *Project) GetProjectInputs() map[string]*input.Input {
	inputs := make(map[string]*input.Input)

	for _, node := range p.FlowNodes {
		if node.FromType == "INPUT" && node.FromInit {
			var inp *input.Input
			if p.Testing {
				// In testing mode, prioritize test-specific instances
				// First try to get test instance with TEST_ prefix
				if testInp, exists := GetInput("TEST_" + node.FromPNS); exists {
					inp = testInp
				} else if testInp, exists := GetInput(node.FromPNS); exists {
					// Fallback: check if the PNS itself is a test instance
					inp = testInp
				}
			} else {
				// Production mode: get original input component
				if originalInp, exists := GetInput(node.FromID); exists {
					inp = originalInp
				}
			}
			if inp != nil {
				inputs[node.FromPNS] = inp
			}
		}
	}
	return inputs
}

// GetProjectOutputs returns all outputs used by this project, dynamically calculated from FlowNodes
func (p *Project) GetProjectOutputs() map[string]*output.Output {
	outputs := make(map[string]*output.Output)

	for _, node := range p.FlowNodes {
		if node.ToType == "OUTPUT" && node.ToInit {
			var out *output.Output
			if p.Testing {
				// In testing mode, prioritize test-specific instances
				// First try to get test instance from PNS (testing outputs are stored with their PNS)
				if testOut, exists := GetPNSOutput(node.ToPNS); exists {
					out = testOut
				} else if testOut, exists := GetOutput("TEST_" + node.ToPNS); exists {
					// Fallback: check for TEST_ prefixed output
					out = testOut
				}
			} else {
				// Production mode: get from PNS first, then original
				if pnsOut, exists := GetPNSOutput(node.ToPNS); exists {
					out = pnsOut
				} else if originalOut, exists := GetOutput(node.ToID); exists {
					out = originalOut
				}
			}
			if out != nil {
				outputs[node.ToPNS] = out
			}
		}
	}
	return outputs
}

// GetProjectRulesets returns all rulesets used by this project, dynamically calculated from FlowNodes
func (p *Project) GetProjectRulesets() map[string]*rules_engine.Ruleset {
	rulesets := make(map[string]*rules_engine.Ruleset)

	for _, node := range p.FlowNodes {
		if node.ToType == "RULESET" && node.ToInit {
			if rs, exists := GetPNSRuleset(node.ToPNS); exists {
				rulesets[node.ToPNS] = rs
			}
		}
		if node.FromType == "RULESET" && node.FromInit {
			if rs, exists := GetPNSRuleset(node.FromPNS); exists {
				rulesets[node.FromPNS] = rs
			}
		}
	}
	return rulesets
}

// GetProjectRulesetsUnsafe returns all rulesets used by this project without acquiring locks
// This version should only be called when the caller already holds the appropriate locks
func (p *Project) GetProjectRulesetsUnsafe() map[string]*rules_engine.Ruleset {
	rulesets := make(map[string]*rules_engine.Ruleset)

	for _, node := range p.FlowNodes {
		if node.ToType == "RULESET" && node.ToInit {
			if rs, exists := GlobalProject.PNSRulesets[node.ToPNS]; exists {
				rulesets[node.ToPNS] = rs
			}
		}
		if node.FromType == "RULESET" && node.FromInit {
			if rs, exists := GlobalProject.PNSRulesets[node.FromPNS]; exists {
				rulesets[node.FromPNS] = rs
			}
		}
	}
	return rulesets
}

// GetProjectInputsUnsafe returns all inputs used by this project without acquiring locks
func (p *Project) GetProjectInputsUnsafe() map[string]*input.Input {
	inputs := make(map[string]*input.Input)

	for _, node := range p.FlowNodes {
		if node.FromType == "INPUT" && node.FromInit {
			var inp *input.Input
			if p.Testing {
				// In testing mode, prioritize test-specific instances
				if testInp, exists := GlobalProject.Inputs["TEST_"+node.FromPNS]; exists {
					inp = testInp
				} else if testInp, exists := GlobalProject.Inputs[node.FromPNS]; exists {
					inp = testInp
				}
			} else {
				// Production mode: get original input component
				if originalInp, exists := GlobalProject.Inputs[node.FromID]; exists {
					inp = originalInp
				}
			}
			if inp != nil {
				inputs[node.FromPNS] = inp
			}
		}
	}
	return inputs
}

// GetProjectOutputsUnsafe returns all outputs used by this project without acquiring locks
func (p *Project) GetProjectOutputsUnsafe() map[string]*output.Output {
	outputs := make(map[string]*output.Output)

	for _, node := range p.FlowNodes {
		if node.ToType == "OUTPUT" && node.ToInit {
			var out *output.Output
			if p.Testing {
				// In testing mode, prioritize test-specific instances
				if testOut, exists := GlobalProject.PNSOutputs[node.ToPNS]; exists {
					out = testOut
				} else if testOut, exists := GlobalProject.Outputs["TEST_"+node.ToPNS]; exists {
					out = testOut
				}
			} else {
				// Production mode: get from PNS first, then original
				if pnsOut, exists := GlobalProject.PNSOutputs[node.ToPNS]; exists {
					out = pnsOut
				} else if originalOut, exists := GlobalProject.Outputs[node.ToID]; exists {
					out = originalOut
				}
			}
			if out != nil {
				outputs[node.ToPNS] = out
			}
		}
	}
	return outputs
}

// Project accessors
func GetProject(id string) (*Project, bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	proj, exists := GlobalProject.Projects[id]
	return proj, exists
}

func SetProject(id string, project *Project) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	GlobalProject.Projects[id] = project
}

func DeleteProject(id string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	delete(GlobalProject.Projects, id)
}

func GetAllProjects() map[string]*Project {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	// Return a copy to avoid external modification
	projects := make(map[string]*Project)
	for id, proj := range GlobalProject.Projects {
		projects[id] = proj
	}
	return projects
}

func GetProjectsCount() int {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	return len(GlobalProject.Projects)
}

// Input accessors
func GetInput(id string) (*input.Input, bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	inp, exists := GlobalProject.Inputs[id]
	return inp, exists
}

func SetInput(id string, inp *input.Input) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	if GlobalProject.Inputs == nil {
		GlobalProject.Inputs = make(map[string]*input.Input)
	}
	GlobalProject.Inputs[id] = inp
}

func DeleteInput(id string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	delete(GlobalProject.Inputs, id)
}

func GetAllInputs() map[string]*input.Input {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	// Return a copy to avoid external modification
	inputs := make(map[string]*input.Input)
	for id, inp := range GlobalProject.Inputs {
		inputs[id] = inp
	}
	return inputs
}

// Output accessors
func GetOutput(id string) (*output.Output, bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	out, exists := GlobalProject.Outputs[id]
	return out, exists
}

func SetOutput(id string, out *output.Output) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	if GlobalProject.Outputs == nil {
		GlobalProject.Outputs = make(map[string]*output.Output)
	}
	GlobalProject.Outputs[id] = out
}

func DeleteOutput(id string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	delete(GlobalProject.Outputs, id)
}

func GetAllOutputs() map[string]*output.Output {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	// Return a copy to avoid external modification
	outputs := make(map[string]*output.Output)
	for id, out := range GlobalProject.Outputs {
		outputs[id] = out
	}
	return outputs
}

// Ruleset accessors
func GetRuleset(id string) (*rules_engine.Ruleset, bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	rs, exists := GlobalProject.Rulesets[id]
	return rs, exists
}

func SetRuleset(id string, rs *rules_engine.Ruleset) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	if GlobalProject.Rulesets == nil {
		GlobalProject.Rulesets = make(map[string]*rules_engine.Ruleset)
	}
	GlobalProject.Rulesets[id] = rs
}

func DeleteRuleset(id string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	delete(GlobalProject.Rulesets, id)
}

func GetAllRulesets() map[string]*rules_engine.Ruleset {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	// Return a copy to avoid external modification
	rulesets := make(map[string]*rules_engine.Ruleset)
	for id, rs := range GlobalProject.Rulesets {
		rulesets[id] = rs
	}
	return rulesets
}

// PNS Output accessors
func GetPNSOutput(pns string) (*output.Output, bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	out, exists := GlobalProject.PNSOutputs[pns]
	return out, exists
}

func SetPNSOutput(pns string, out *output.Output) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	if GlobalProject.PNSOutputs == nil {
		GlobalProject.PNSOutputs = make(map[string]*output.Output)
	}
	GlobalProject.PNSOutputs[pns] = out
}

func DeletePNSOutput(pns string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	delete(GlobalProject.PNSOutputs, pns)
}

// PNS Ruleset accessors
func GetPNSRuleset(pns string) (*rules_engine.Ruleset, bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	rs, exists := GlobalProject.PNSRulesets[pns]
	return rs, exists
}

func SetPNSRuleset(pns string, rs *rules_engine.Ruleset) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	if GlobalProject.PNSRulesets == nil {
		GlobalProject.PNSRulesets = make(map[string]*rules_engine.Ruleset)
	}
	GlobalProject.PNSRulesets[pns] = rs
}

func DeletePNSRuleset(pns string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	delete(GlobalProject.PNSRulesets, pns)
}

// New/Temporary content accessors
func GetProjectNew(id string) (string, bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	content, exists := GlobalProject.ProjectsNew[id]
	return content, exists
}

func SetProjectNew(id string, content string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	if GlobalProject.ProjectsNew == nil {
		GlobalProject.ProjectsNew = make(map[string]string)
	}
	GlobalProject.ProjectsNew[id] = content
}

func DeleteProjectNew(id string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	delete(GlobalProject.ProjectsNew, id)
}

func GetInputNew(id string) (string, bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	content, exists := GlobalProject.InputsNew[id]
	return content, exists
}

func SetInputNew(id string, content string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	if GlobalProject.InputsNew == nil {
		GlobalProject.InputsNew = make(map[string]string)
	}
	GlobalProject.InputsNew[id] = content
}

func DeleteInputNew(id string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	delete(GlobalProject.InputsNew, id)
}

func GetOutputNew(id string) (string, bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	content, exists := GlobalProject.OutputsNew[id]
	return content, exists
}

func SetOutputNew(id string, content string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	if GlobalProject.OutputsNew == nil {
		GlobalProject.OutputsNew = make(map[string]string)
	}
	GlobalProject.OutputsNew[id] = content
}

func DeleteOutputNew(id string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	delete(GlobalProject.OutputsNew, id)
}

func GetRulesetNew(id string) (string, bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	content, exists := GlobalProject.RulesetsNew[id]
	return content, exists
}

func SetRulesetNew(id string, content string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	if GlobalProject.RulesetsNew == nil {
		GlobalProject.RulesetsNew = make(map[string]string)
	}
	GlobalProject.RulesetsNew[id] = content
}

func DeleteRulesetNew(id string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()
	delete(GlobalProject.RulesetsNew, id)
}

// Component validation helpers
func ValidateComponent(componentType, componentID string) (exists bool, tempExists bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

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
	}
	return exists, tempExists
}

// Safe iteration functions
func ForEachProject(fn func(id string, project *Project) bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	for id, proj := range GlobalProject.Projects {
		if !fn(id, proj) {
			break
		}
	}
}

// Unsafe iteration functions - use with caution, caller must ensure proper locking
func ForEachProjectUnsafa(fn func(id string, project *Project) bool) {
	for id, proj := range GlobalProject.Projects {
		if !fn(id, proj) {
			break
		}
	}
}

func ForEachInput(fn func(id string, inp *input.Input) bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	for id, inp := range GlobalProject.Inputs {
		if !fn(id, inp) {
			break
		}
	}
}

func ForEachOutput(fn func(id string, out *output.Output) bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	for id, out := range GlobalProject.Outputs {
		if !fn(id, out) {
			break
		}
	}
}

func ForEachRuleset(fn func(id string, rs *rules_engine.Ruleset) bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	for id, rs := range GlobalProject.Rulesets {
		if !fn(id, rs) {
			break
		}
	}
}

// Helper function to safely access input downstream
func SafeDeleteInputDownstream(inputID, downstreamID string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	if i, exists := GlobalProject.Inputs[inputID]; exists {
		delete(i.DownStream, downstreamID)
	}
}

// Helper function to safely access input downstream
func SafeDeleteRulesetDownstream(rulesetID, downstreamID string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	if i, exists := GlobalProject.Rulesets[rulesetID]; exists {
		delete(i.DownStream, downstreamID)
	}
}

// GetAllProjectsNew returns a copy of all projects new map
func GetAllProjectsNew() map[string]string {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	result := make(map[string]string)
	for id, content := range GlobalProject.ProjectsNew {
		result[id] = content
	}
	return result
}

// GetAllInputsNew returns a copy of all inputs new map
func GetAllInputsNew() map[string]string {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	result := make(map[string]string)
	for id, content := range GlobalProject.InputsNew {
		result[id] = content
	}
	return result
}

// GetAllOutputsNew returns a copy of all outputs new map
func GetAllOutputsNew() map[string]string {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	result := make(map[string]string)
	for id, content := range GlobalProject.OutputsNew {
		result[id] = content
	}
	return result
}

// GetAllRulesetsNew returns a copy of all rulesets new map
func GetAllRulesetsNew() map[string]string {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	result := make(map[string]string)
	for id, content := range GlobalProject.RulesetsNew {
		result[id] = content
	}
	return result
}

// GetInputsCount returns the count of inputs
func GetInputsCount() int {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	return len(GlobalProject.Inputs)
}

// GetOutputsCount returns the count of outputs
func GetOutputsCount() int {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	return len(GlobalProject.Outputs)
}

// GetRulesetsCount returns the count of rulesets
func GetRulesetsCount() int {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	return len(GlobalProject.Rulesets)
}

// ===== Safe deletion functions with internal locking =====

// SafeDeleteRuleset safely deletes a ruleset with all necessary validations and locking
func SafeDeleteRuleset(id string) ([]string, error) {
	// Phase 1: Perform all checks and prepare for deletion
	var componentToStop *rules_engine.Ruleset
	var shouldStop bool

	common.GlobalMu.Lock()

	// Check if component exists
	_, componentExists := GlobalProject.Rulesets[id]
	if !componentExists {
		// Check if only exists in temporary storage
		_, tempExists := GlobalProject.RulesetsNew[id]
		common.GlobalMu.Unlock()
		if !tempExists {
			return nil, fmt.Errorf("ruleset not found: %s", id)
		}
		// Only exists in temp, just remove from temp
		common.GlobalMu.Lock()
		delete(GlobalProject.RulesetsNew, id)
		common.GlobalMu.Unlock()
		common.DeleteRawConfigUnsafe("ruleset", id)
		return []string{}, nil
	}

	// Check if used by any project - use unsafe accessors to avoid deadlock
	for projectID, proj := range GlobalProject.Projects {
		// Skip projects that are not running
		if proj.Status != common.StatusRunning {
			continue
		}
		rulesets := proj.GetProjectRulesetsUnsafe()
		if _, inUse := rulesets[id]; inUse {
			common.GlobalMu.Unlock()
			return nil, fmt.Errorf("ruleset %s is currently in use by project %s", id, projectID)
		}
	}

	// Check if component should be stopped using unsafe version
	if rs, exists := GlobalProject.Rulesets[id]; exists {
		if CalculateRefCountUnsafe(id) == 0 {
			componentToStop = rs
			shouldStop = true
			logger.Info("Scheduling ruleset component for deletion", "id", id)
		}
	}

	// Remove from global mappings if stopping
	if shouldStop {
		delete(GlobalProject.Rulesets, id)
		delete(GlobalProject.RulesetsNew, id)
	}

	common.GlobalMu.Unlock()

	// Phase 2: Execute stop operation outside of lock
	if shouldStop && componentToStop != nil {
		_ = componentToStop.Stop()
		common.GlobalDailyStatsManager.CollectAllComponentsData()
		common.DeleteRawConfigUnsafe("ruleset", id)
	}

	return []string{}, nil
}

// SafeDeleteInput safely deletes an input with all necessary validations and locking
func SafeDeleteInput(id string) ([]string, error) {
	// Phase 1: Perform all checks and prepare for deletion
	var componentToStop *input.Input
	var shouldStop bool

	common.GlobalMu.Lock()

	// Check if component exists
	_, componentExists := GlobalProject.Inputs[id]
	if !componentExists {
		// Check if only exists in temporary storage
		_, tempExists := GlobalProject.InputsNew[id]
		common.GlobalMu.Unlock()
		if !tempExists {
			return nil, fmt.Errorf("input not found: %s", id)
		}
		// Only exists in temp, just remove from temp
		common.GlobalMu.Lock()
		delete(GlobalProject.InputsNew, id)
		common.GlobalMu.Unlock()
		common.DeleteRawConfigUnsafe("input", id)
		return []string{}, nil
	}

	// Check if used by any project - use unsafe accessors to avoid deadlock
	for projectID, proj := range GlobalProject.Projects {
		// Skip projects that are not running
		if proj.Status != common.StatusRunning {
			continue
		}
		inputs := proj.GetProjectInputsUnsafe()
		if _, inUse := inputs[id]; inUse {
			common.GlobalMu.Unlock()
			return nil, fmt.Errorf("input %s is currently in use by project %s", id, projectID)
		}
	}

	// Check if component should be stopped using unsafe version
	if inp, exists := GlobalProject.Inputs[id]; exists {
		if CalculateRefCountUnsafe(id) == 0 {
			componentToStop = inp
			shouldStop = true
			logger.Info("Scheduling input component for deletion", "id", id)
		}
	}

	// Remove from global mappings if stopping
	if shouldStop {
		delete(GlobalProject.Inputs, id)
		delete(GlobalProject.InputsNew, id)
	}

	common.GlobalMu.Unlock()

	// Phase 2: Execute stop operation outside of lock
	if shouldStop && componentToStop != nil {
		_ = componentToStop.Stop()
		common.GlobalDailyStatsManager.CollectAllComponentsData()
		common.DeleteRawConfigUnsafe("input", id)
	}

	return []string{}, nil
}

// SafeDeleteOutput safely deletes an output with all necessary validations and locking
func SafeDeleteOutput(id string) ([]string, error) {
	// Phase 1: Perform all checks and prepare for deletion
	var componentToStop *output.Output
	var shouldStop bool

	common.GlobalMu.Lock()

	// Check if component exists
	_, componentExists := GlobalProject.Outputs[id]
	if !componentExists {
		// Check if only exists in temporary storage
		_, tempExists := GlobalProject.OutputsNew[id]
		common.GlobalMu.Unlock()
		if !tempExists {
			return nil, fmt.Errorf("output not found: %s", id)
		}
		// Only exists in temp, just remove from temp
		common.GlobalMu.Lock()
		delete(GlobalProject.OutputsNew, id)
		common.GlobalMu.Unlock()
		common.DeleteRawConfigUnsafe("output", id)
		return []string{}, nil
	}

	// Check if used by any project - use unsafe accessors to avoid deadlock
	for projectID, proj := range GlobalProject.Projects {
		// Skip projects that are not running
		if proj.Status != common.StatusRunning {
			continue
		}
		outputs := proj.GetProjectOutputsUnsafe()
		if _, inUse := outputs[id]; inUse {
			common.GlobalMu.Unlock()
			return nil, fmt.Errorf("output %s is currently in use by project %s", id, projectID)
		}
	}

	// Check if component should be stopped using unsafe version
	if out, exists := GlobalProject.Outputs[id]; exists {
		if CalculateRefCountUnsafe(id) == 0 {
			componentToStop = out
			shouldStop = true
			logger.Info("Scheduling output component for deletion", "id", id)
		}
	}

	// Remove from global mappings if stopping
	if shouldStop {
		delete(GlobalProject.Outputs, id)
		delete(GlobalProject.OutputsNew, id)
	}

	common.GlobalMu.Unlock()

	// Phase 2: Execute stop operation outside of lock
	if shouldStop && componentToStop != nil {
		_ = componentToStop.Stop()
		common.GlobalDailyStatsManager.CollectAllComponentsData()
		common.DeleteRawConfigUnsafe("output", id)
	}

	return []string{}, nil
}

// SafeDeleteProject safely deletes a project with all necessary validations and locking
func SafeDeleteProject(id string) ([]string, error) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	// Check if component exists
	proj, componentExists := GlobalProject.Projects[id]
	if !componentExists {
		// Check if only exists in temporary storage
		_, tempExists := GlobalProject.ProjectsNew[id]
		if !tempExists {
			return nil, fmt.Errorf("project not found: %s", id)
		}
		// Only exists in temp, just remove from temp
		delete(GlobalProject.ProjectsNew, id)
		common.DeleteRawConfigUnsafe("project", id)
		return []string{}, nil
	}

	// Stop project if running
	if proj.Status == common.StatusRunning || proj.Status == common.StatusStarting || proj.Status == common.StatusError {
		logger.Info("Stopping running project before deletion", "project_id", id)

		// Temporarily release lock for stop operation to prevent deadlock
		common.GlobalMu.Unlock()

		// Note: Project.Stop() already includes final statistics collection
		if err := proj.Stop(true); err != nil {
			logger.Error("failed to stop project before deletion: %v", err)
		}

		// Re-acquire lock after stop
		common.GlobalMu.Lock()

		// Wait for project to fully stop
		timeout := time.After(30 * time.Second)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				return nil, fmt.Errorf("timeout waiting for project to stop")
			case <-ticker.C:
				// Check project status without additional locking since we already hold the lock
				if proj.Status == common.StatusStopped {
					goto projectStopped
				}
			}
		}
	projectStopped:
		logger.Info("Project stopped successfully before deletion", "project_id", id)
	}

	// Remove from global mappings
	delete(GlobalProject.Projects, id)
	delete(GlobalProject.ProjectsNew, id)
	common.DeleteRawConfigUnsafe("project", id)

	return []string{id}, nil
}
