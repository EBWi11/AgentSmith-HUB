package project

import "AgentSmith-HUB/common"

// ComponentUsageCounter provides thread-safe methods to count component usage across projects
type ComponentUsageCounter struct{}

var UsageCounter = &ComponentUsageCounter{}

// CountProjectsUsingInput counts how many running projects are using the specified input component
// excludeProjectID: optional project ID to exclude from the count (for self-exclusion during stop operations)
func (c *ComponentUsageCounter) CountProjectsUsingInput(inputID string, excludeProjectID ...string) int {
	count := 0
	var excludeID string
	if len(excludeProjectID) > 0 {
		excludeID = excludeProjectID[0]
	}

	ForEachProject(func(projectID string, proj *Project) bool {
		if projectID != excludeID && proj.Status == common.StatusRunning {
			if _, exists := proj.Inputs[inputID]; exists {
				count++
			}
		}
		return true
	})

	return count
}

// CountProjectsUsingOutput counts how many running projects are using the specified output component
// excludeProjectID: optional project ID to exclude from the count (for self-exclusion during stop operations)
func (c *ComponentUsageCounter) CountProjectsUsingOutput(outputID string, excludeProjectID ...string) int {
	count := 0
	var excludeID string
	if len(excludeProjectID) > 0 {
		excludeID = excludeProjectID[0]
	}

	ForEachProject(func(projectID string, proj *Project) bool {
		if projectID != excludeID && proj.Status == common.StatusRunning {
			if _, exists := proj.Outputs[outputID]; exists {
				count++
			}
		}
		return true
	})

	return count
}

// CountProjectsUsingRuleset counts how many running projects are using the specified ruleset component
// excludeProjectID: optional project ID to exclude from the count (for self-exclusion during stop operations)
func (c *ComponentUsageCounter) CountProjectsUsingRuleset(rulesetID string, excludeProjectID ...string) int {
	count := 0
	var excludeID string
	if len(excludeProjectID) > 0 {
		excludeID = excludeProjectID[0]
	}

	ForEachProject(func(projectID string, proj *Project) bool {
		if projectID != excludeID && proj.Status == common.StatusRunning {
			if _, exists := proj.Rulesets[rulesetID]; exists {
				count++
			}
		}
		return true
	})

	return count
}

// ComponentUsageInfo provides usage information for all component types
type ComponentUsageInfo struct {
	InputUsage   map[string]int // inputID -> count of projects using it
	OutputUsage  map[string]int // outputID -> count of projects using it
	RulesetUsage map[string]int // rulesetID -> count of projects using it
}

// BatchCountComponentUsage performs batch counting for multiple components to improve performance
// This is especially useful for batch operations like ApplyPendingChanges
type BatchUsageResult struct {
	InputUsageCount   map[string]int // inputID -> count
	OutputUsageCount  map[string]int // outputID -> count
	RulesetUsageCount map[string]int // rulesetID -> count
}
