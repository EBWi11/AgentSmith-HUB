package project

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

	// Use dedicated project lock to reduce contention with configuration operations
	GlobalProject.ProjectMu.RLock()
	defer GlobalProject.ProjectMu.RUnlock()

	for projectID, proj := range GlobalProject.Projects {
		if projectID != excludeID && proj.Status == ProjectStatusRunning {
			if _, exists := proj.Inputs[inputID]; exists {
				count++
			}
		}
	}
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

	// Use dedicated project lock to reduce contention with configuration operations
	GlobalProject.ProjectMu.RLock()
	defer GlobalProject.ProjectMu.RUnlock()

	for projectID, proj := range GlobalProject.Projects {
		if projectID != excludeID && proj.Status == ProjectStatusRunning {
			if _, exists := proj.Outputs[outputID]; exists {
				count++
			}
		}
	}
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

	// Use dedicated project lock to reduce contention with configuration operations
	GlobalProject.ProjectMu.RLock()
	defer GlobalProject.ProjectMu.RUnlock()

	for projectID, proj := range GlobalProject.Projects {
		if projectID != excludeID && proj.Status == ProjectStatusRunning {
			if _, exists := proj.Rulesets[rulesetID]; exists {
				count++
			}
		}
	}
	return count
}

// ComponentUsageInfo provides usage information for all component types
type ComponentUsageInfo struct {
	InputUsage   map[string]int // inputID -> count of projects using it
	OutputUsage  map[string]int // outputID -> count of projects using it
	RulesetUsage map[string]int // rulesetID -> count of projects using it
}

// GetAllComponentUsage returns comprehensive usage information for all components
// This is useful for batch operations and administrative purposes
func (c *ComponentUsageCounter) GetAllComponentUsage() *ComponentUsageInfo {
	info := &ComponentUsageInfo{
		InputUsage:   make(map[string]int),
		OutputUsage:  make(map[string]int),
		RulesetUsage: make(map[string]int),
	}

	// Use dedicated project lock to reduce contention with configuration operations
	GlobalProject.ProjectMu.RLock()
	defer GlobalProject.ProjectMu.RUnlock()

	for _, proj := range GlobalProject.Projects {
		if proj.Status == ProjectStatusRunning {
			// Count input usage
			for inputID := range proj.Inputs {
				info.InputUsage[inputID]++
			}
			// Count output usage
			for outputID := range proj.Outputs {
				info.OutputUsage[outputID]++
			}
			// Count ruleset usage
			for rulesetID := range proj.Rulesets {
				info.RulesetUsage[rulesetID]++
			}
		}
	}

	return info
}

// IsComponentInUse checks if any component (input, output, or ruleset) is currently in use
// Returns the count of projects using it and a boolean indicating if it's in use
func (c *ComponentUsageCounter) IsComponentInUse(componentType, componentID string, excludeProjectID ...string) (int, bool) {
	var count int

	switch componentType {
	case "input":
		count = c.CountProjectsUsingInput(componentID, excludeProjectID...)
	case "output":
		count = c.CountProjectsUsingOutput(componentID, excludeProjectID...)
	case "ruleset":
		count = c.CountProjectsUsingRuleset(componentID, excludeProjectID...)
	default:
		return 0, false
	}

	return count, count > 0
}

// BatchCountComponentUsage performs batch counting for multiple components to improve performance
// This is especially useful for batch operations like ApplyPendingChanges
type BatchUsageResult struct {
	InputUsageCount   map[string]int // inputID -> count
	OutputUsageCount  map[string]int // outputID -> count
	RulesetUsageCount map[string]int // rulesetID -> count
}

// GetBatchComponentUsage returns usage counts for multiple components in a single lock operation
// This is more efficient than calling individual Count functions when you need multiple counts
func (c *ComponentUsageCounter) GetBatchComponentUsage(
	inputIDs []string,
	outputIDs []string,
	rulesetIDs []string,
	excludeProjectID string,
) *BatchUsageResult {
	result := &BatchUsageResult{
		InputUsageCount:   make(map[string]int),
		OutputUsageCount:  make(map[string]int),
		RulesetUsageCount: make(map[string]int),
	}

	// Use dedicated project lock to reduce contention with configuration operations
	GlobalProject.ProjectMu.RLock()
	defer GlobalProject.ProjectMu.RUnlock()

	// Initialize counts to zero
	for _, id := range inputIDs {
		result.InputUsageCount[id] = 0
	}
	for _, id := range outputIDs {
		result.OutputUsageCount[id] = 0
	}
	for _, id := range rulesetIDs {
		result.RulesetUsageCount[id] = 0
	}

	// Count usage for all components in a single pass
	for projectID, proj := range GlobalProject.Projects {
		if projectID != excludeProjectID && proj.Status == ProjectStatusRunning {
			// Count input usage
			for inputID := range proj.Inputs {
				if _, exists := result.InputUsageCount[inputID]; exists {
					result.InputUsageCount[inputID]++
				}
			}

			// Count output usage
			for outputID := range proj.Outputs {
				if _, exists := result.OutputUsageCount[outputID]; exists {
					result.OutputUsageCount[outputID]++
				}
			}

			// Count ruleset usage
			for rulesetID := range proj.Rulesets {
				if _, exists := result.RulesetUsageCount[rulesetID]; exists {
					result.RulesetUsageCount[rulesetID]++
				}
			}
		}
	}

	return result
}

// IsAnyComponentInUse checks if any of the specified components are currently in use
// This is useful for batch operations to quickly determine if any components need special handling
func (c *ComponentUsageCounter) IsAnyComponentInUse(
	inputIDs []string,
	outputIDs []string,
	rulesetIDs []string,
	excludeProjectID string,
) bool {
	// Use dedicated project lock to reduce contention with configuration operations
	GlobalProject.ProjectMu.RLock()
	defer GlobalProject.ProjectMu.RUnlock()

	// Create lookup maps for faster checking
	inputMap := make(map[string]bool)
	for _, id := range inputIDs {
		inputMap[id] = true
	}
	outputMap := make(map[string]bool)
	for _, id := range outputIDs {
		outputMap[id] = true
	}
	rulesetMap := make(map[string]bool)
	for _, id := range rulesetIDs {
		rulesetMap[id] = true
	}

	// Check if any running project is using any of the specified components
	for projectID, proj := range GlobalProject.Projects {
		if projectID != excludeProjectID && proj.Status == ProjectStatusRunning {
			// Check input usage
			for inputID := range proj.Inputs {
				if inputMap[inputID] {
					return true
				}
			}

			// Check output usage
			for outputID := range proj.Outputs {
				if outputMap[outputID] {
					return true
				}
			}

			// Check ruleset usage
			for rulesetID := range proj.Rulesets {
				if rulesetMap[rulesetID] {
					return true
				}
			}
		}
	}

	return false
}

// CountProjectsUsingOutputInstance counts how many running projects are using the specified output instance
// This is used for independent output instances created with different ProjectNodeSequence
func (c *ComponentUsageCounter) CountProjectsUsingOutputInstance(outputID string, projectNodeSequence string, excludeProjectID ...string) int {
	count := 0
	var excludeID string
	if len(excludeProjectID) > 0 {
		excludeID = excludeProjectID[0]
	}

	// Use dedicated project lock to reduce contention with configuration operations
	GlobalProject.ProjectMu.RLock()
	defer GlobalProject.ProjectMu.RUnlock()

	for projectID, proj := range GlobalProject.Projects {
		if projectID != excludeID && proj.Status == ProjectStatusRunning {
			if output, exists := proj.Outputs[outputID]; exists {
				// Check if this is the exact same instance by comparing ProjectNodeSequence
				if output.ProjectNodeSequence == projectNodeSequence {
					count++
				}
			}
		}
	}
	return count
}

// CountProjectsUsingRulesetInstance counts how many running projects are using the specified ruleset instance
// This is used for independent ruleset instances created with different ProjectNodeSequence
func (c *ComponentUsageCounter) CountProjectsUsingRulesetInstance(rulesetID string, projectNodeSequence string, excludeProjectID ...string) int {
	count := 0
	var excludeID string
	if len(excludeProjectID) > 0 {
		excludeID = excludeProjectID[0]
	}

	// Use dedicated project lock to reduce contention with configuration operations
	GlobalProject.ProjectMu.RLock()
	defer GlobalProject.ProjectMu.RUnlock()

	for projectID, proj := range GlobalProject.Projects {
		if projectID != excludeID && proj.Status == ProjectStatusRunning {
			if ruleset, exists := proj.Rulesets[rulesetID]; exists {
				// Check if this is the exact same instance by comparing ProjectNodeSequence
				if ruleset.ProjectNodeSequence == projectNodeSequence {
					count++
				}
			}
		}
	}
	return count
}

// IsComponentInstanceInUse checks if a specific component instance is currently in use
// Returns the count of projects using it and a boolean indicating if it's in use
func (c *ComponentUsageCounter) IsComponentInstanceInUse(componentType, componentID, projectNodeSequence string, excludeProjectID ...string) (int, bool) {
	var count int

	switch componentType {
	case "input":
		// For inputs, we still use the original logic since inputs are typically shared
		count = c.CountProjectsUsingInput(componentID, excludeProjectID...)
	case "output":
		count = c.CountProjectsUsingOutputInstance(componentID, projectNodeSequence, excludeProjectID...)
	case "ruleset":
		count = c.CountProjectsUsingRulesetInstance(componentID, projectNodeSequence, excludeProjectID...)
	default:
		return 0, false
	}

	return count, count > 0
}
