package project

import "AgentSmith-HUB/common"

// ComponentUsageCounter provides thread-safe methods to count component usage across projects
type ComponentUsageCounter struct{}

var UsageCounter = &ComponentUsageCounter{}

func (c *ComponentUsageCounter) CountProjectsUsing(t string, id string, excludeProjectID ...string) int {
	count := 0
	var excludeID string
	if len(excludeProjectID) > 0 {
		excludeID = excludeProjectID[0]
	}

	ForEachProjectUnsafa(func(projectID string, proj *Project) bool {
		if projectID != excludeID {
			if userWantsRunning, err := common.GetProjectUserIntention(projectID); err == nil && userWantsRunning {
				if proj.CheckExist(t, id) {
					count++
				}
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
