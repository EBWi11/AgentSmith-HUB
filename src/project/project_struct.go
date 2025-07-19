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

	RefCount map[string]int
}

func GetRefCount(id string) int {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	return GlobalProject.RefCount[id]
}

func AddRefCount(id string) {
	common.GlobalMu.Lock()
	GlobalProject.RefCount[id] = GlobalProject.RefCount[id] + 1
	common.GlobalMu.Unlock()
}

func ReduceRefCount(id string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	if GlobalProject.RefCount[id] > 0 {
		GlobalProject.RefCount[id] = GlobalProject.RefCount[id] - 1
	} else {
		// Log warning for debugging
		logger.Warn("Attempting to reduce reference count when count is already 0", "id", id)
	}
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

	// Components
	Inputs   map[string]*input.Input          `json:"-"`
	Outputs  map[string]*output.Output        `json:"-"`
	Rulesets map[string]*rules_engine.Ruleset `json:"-"`

	// Data flow
	MsgChannels map[string]*chan map[string]interface{} `json:"-"` // Channels for message passing between components
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
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	// Check if component exists
	_, componentExists := GlobalProject.Rulesets[id]
	if !componentExists {
		// Check if only exists in temporary storage
		_, tempExists := GlobalProject.RulesetsNew[id]
		if !tempExists {
			return nil, fmt.Errorf("ruleset not found: %s", id)
		}
		// Only exists in temp, just remove from temp
		delete(GlobalProject.RulesetsNew, id)
		common.DeleteRawConfig("ruleset", id)
		return []string{}, nil
	}

	// Check if used by any project
	affectedProjects := make([]string, 0)
	for projectID, proj := range GlobalProject.Projects {
		if _, inUse := proj.Rulesets[id]; inUse {
			return nil, fmt.Errorf("ruleset %s is currently in use by project %s", id, projectID)
		}
	}

	// Stop the component if not in use
	if rs, exists := GlobalProject.Rulesets[id]; exists {
		projectsUsingRuleset := UsageCounter.CountProjectsUsing("RULESET", id)
		if projectsUsingRuleset == 0 {
			logger.Info("Stopping ruleset component for deletion", "id", id)
			// Temporarily release lock for stop operation to prevent deadlock
			common.GlobalMu.Unlock()
			err := rs.Stop()
			common.GlobalMu.Lock()
			if err != nil {
				logger.Error("Failed to stop ruleset", "id", id, "error", err)
			}
			common.GlobalDailyStatsManager.CollectAllComponentsData()
		}
	}

	// Remove from global mappings
	delete(GlobalProject.Rulesets, id)
	delete(GlobalProject.RulesetsNew, id)
	common.DeleteRawConfig("ruleset", id)

	return affectedProjects, nil
}

// SafeDeleteInput safely deletes an input with all necessary validations and locking
func SafeDeleteInput(id string) ([]string, error) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	// Check if component exists
	_, componentExists := GlobalProject.Inputs[id]
	if !componentExists {
		// Check if only exists in temporary storage
		_, tempExists := GlobalProject.InputsNew[id]
		if !tempExists {
			return nil, fmt.Errorf("input not found: %s", id)
		}
		// Only exists in temp, just remove from temp
		delete(GlobalProject.InputsNew, id)
		common.DeleteRawConfig("input", id)
		return []string{}, nil
	}

	// Check if used by any project
	affectedProjects := make([]string, 0)
	for projectID, proj := range GlobalProject.Projects {
		if _, inUse := proj.Inputs[id]; inUse {
			return nil, fmt.Errorf("input %s is currently in use by project %s", id, projectID)
		}
	}

	// Stop the component if not in use
	if inp, exists := GlobalProject.Inputs[id]; exists {
		projectsUsingInput := UsageCounter.CountProjectsUsing("INPUT", id)
		if projectsUsingInput == 0 {
			logger.Info("Stopping input component for deletion", "id", id)
			// Temporarily release lock for stop operation to prevent deadlock
			common.GlobalMu.Unlock()
			err := inp.Stop()
			common.GlobalMu.Lock()
			if err != nil {
				logger.Error("Failed to stop input", "id", id, "error", err)
			}
			common.GlobalDailyStatsManager.CollectAllComponentsData()
		}
	}

	// Remove from global mappings
	delete(GlobalProject.Inputs, id)
	delete(GlobalProject.InputsNew, id)
	common.DeleteRawConfig("input", id)

	return affectedProjects, nil
}

// SafeDeleteOutput safely deletes an output with all necessary validations and locking
func SafeDeleteOutput(id string) ([]string, error) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	// Check if component exists
	_, componentExists := GlobalProject.Outputs[id]
	if !componentExists {
		// Check if only exists in temporary storage
		_, tempExists := GlobalProject.OutputsNew[id]
		if !tempExists {
			return nil, fmt.Errorf("output not found: %s", id)
		}
		// Only exists in temp, just remove from temp
		delete(GlobalProject.OutputsNew, id)
		common.DeleteRawConfig("output", id)
		return []string{}, nil
	}

	// Check if used by any project
	affectedProjects := make([]string, 0)
	for projectID, proj := range GlobalProject.Projects {
		if _, inUse := proj.Outputs[id]; inUse {
			return nil, fmt.Errorf("output %s is currently in use by project %s", id, projectID)
		}
	}

	// Stop the component if not in use
	if out, exists := GlobalProject.Outputs[id]; exists {
		projectsUsingOutput := UsageCounter.CountProjectsUsing("OUTPUT", id)
		if projectsUsingOutput == 0 {
			logger.Info("Stopping output component for deletion", "id", id)
			// Temporarily release lock for stop operation to prevent deadlock
			common.GlobalMu.Unlock()
			err := out.Stop()
			common.GlobalMu.Lock()
			if err != nil {
				logger.Error("Failed to stop output", "id", id, "error", err)
			}
			common.GlobalDailyStatsManager.CollectAllComponentsData()
		}
	}

	// Remove from global mappings
	delete(GlobalProject.Outputs, id)
	delete(GlobalProject.OutputsNew, id)
	common.DeleteRawConfig("output", id)

	return affectedProjects, nil
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
		common.DeleteRawConfig("project", id)
		return []string{}, nil
	}

	// Stop project if running
	if proj.Status == common.StatusRunning || proj.Status == common.StatusStarting {
		logger.Info("Stopping running project before deletion", "project_id", id)

		// Temporarily release lock for stop operation to prevent deadlock
		common.GlobalMu.Unlock()

		// Note: Project.Stop() already includes final statistics collection
		if err := proj.Stop(true); err != nil {
			// Re-acquire lock before returning
			common.GlobalMu.Lock()
			return nil, fmt.Errorf("failed to stop project before deletion: %v", err)
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
	common.DeleteRawConfig("project", id)

	return []string{id}, nil
}
