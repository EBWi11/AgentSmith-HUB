package common

import "sync"

var Config *HubConfig

// for follower node
// id:raw
var AllInputsRawConfig map[string]string
var AllOutputsRawConfig map[string]string
var AllRulesetsRawConfig map[string]string
var AllProjectRawConfig map[string]string
var AllPluginsRawConfig map[string]string

// Dedicated lock for AllRawConfig variables
var RawConfigMu sync.RWMutex

var GlobalMu sync.RWMutex

// Dedicated lock for project lifecycle operations (start, stop, restart)
// This ensures project operations are serialized and prevents concurrent cleanup issues
var ProjectOperationMu sync.Mutex

// Global cluster state
var (
	IsLeader bool
	Leader   string
)

// Global component monitor instance
var GlobalComponentMonitor *ComponentMonitor

// SetLeaderState sets the leader state for this node
func SetLeaderState(isLeader bool, leaderID string) {
	IsLeader = isLeader
	Leader = leaderID
}

// ===================== AllRawConfig Accessor Functions =====================

// GetRawConfig retrieves raw configuration by type and ID
func GetRawConfig(componentType, id string) (string, bool) {
	RawConfigMu.RLock()
	defer RawConfigMu.RUnlock()

	switch componentType {
	case "input":
		config, exists := AllInputsRawConfig[id]
		return config, exists
	case "output":
		config, exists := AllOutputsRawConfig[id]
		return config, exists
	case "ruleset":
		config, exists := AllRulesetsRawConfig[id]
		return config, exists
	case "project":
		config, exists := AllProjectRawConfig[id]
		return config, exists
	case "plugin":
		config, exists := AllPluginsRawConfig[id]
		return config, exists
	default:
		return "", false
	}
}

// SetRawConfig stores raw configuration by type and ID
func SetRawConfig(componentType, id, config string) {
	RawConfigMu.Lock()
	defer RawConfigMu.Unlock()

	switch componentType {
	case "input":
		if AllInputsRawConfig == nil {
			AllInputsRawConfig = make(map[string]string)
		}
		AllInputsRawConfig[id] = config
	case "output":
		if AllOutputsRawConfig == nil {
			AllOutputsRawConfig = make(map[string]string)
		}
		AllOutputsRawConfig[id] = config
	case "ruleset":
		if AllRulesetsRawConfig == nil {
			AllRulesetsRawConfig = make(map[string]string)
		}
		AllRulesetsRawConfig[id] = config
	case "project":
		if AllProjectRawConfig == nil {
			AllProjectRawConfig = make(map[string]string)
		}
		AllProjectRawConfig[id] = config
	case "plugin":
		if AllPluginsRawConfig == nil {
			AllPluginsRawConfig = make(map[string]string)
		}
		AllPluginsRawConfig[id] = config
	}
}

// DeleteRawConfig removes raw configuration by type and ID
func DeleteRawConfig(componentType, id string) {
	RawConfigMu.Lock()
	defer RawConfigMu.Unlock()

	switch componentType {
	case "input":
		delete(AllInputsRawConfig, id)
	case "output":
		delete(AllOutputsRawConfig, id)
	case "ruleset":
		delete(AllRulesetsRawConfig, id)
	case "project":
		delete(AllProjectRawConfig, id)
	case "plugin":
		delete(AllPluginsRawConfig, id)
	}
}

// ClearAllRawConfigsForAllTypes clears all raw configurations for all types
func ClearAllRawConfigsForAllTypes() {
	RawConfigMu.Lock()
	defer RawConfigMu.Unlock()

	AllInputsRawConfig = make(map[string]string)
	AllOutputsRawConfig = make(map[string]string)
	AllRulesetsRawConfig = make(map[string]string)
	AllProjectRawConfig = make(map[string]string)
	AllPluginsRawConfig = make(map[string]string)
}

// ForEachRawConfig safely iterates over all raw configurations for a specific type
func ForEachRawConfig(componentType string, fn func(id, config string) bool) {
	RawConfigMu.RLock()
	defer RawConfigMu.RUnlock()

	var targetMap map[string]string

	switch componentType {
	case "input":
		targetMap = AllInputsRawConfig
	case "output":
		targetMap = AllOutputsRawConfig
	case "ruleset":
		targetMap = AllRulesetsRawConfig
	case "project":
		targetMap = AllProjectRawConfig
	case "plugin":
		targetMap = AllPluginsRawConfig
	default:
		return
	}

	for id, config := range targetMap {
		if !fn(id, config) {
			break
		}
	}
}

// ===================== Component Monitor Integration =====================

// ProjectComponentChecker defines the function signature for checking project components
type ProjectComponentChecker func() []ProjectComponentError

// ProjectComponentError represents an error found in a project component
type ProjectComponentError struct {
	ProjectID   string
	ComponentID string
	Type        string // "input", "output", "ruleset"
	Status      Status
	Error       error
}

// ProjectErrorSetter defines the function signature for setting project error status
type ProjectErrorSetter func(projectID string, componentErrors []ProjectComponentError)

// Global component checker function - will be set by project package to avoid circular imports
var GlobalProjectComponentChecker ProjectComponentChecker
var GlobalProjectErrorSetter ProjectErrorSetter

// SetProjectComponentChecker sets the global project component checker function
func SetProjectComponentChecker(checker ProjectComponentChecker) {
	GlobalMu.Lock()
	defer GlobalMu.Unlock()
	GlobalProjectComponentChecker = checker
}

// SetProjectErrorSetter sets the global project error setter function
func SetProjectErrorSetter(setter ProjectErrorSetter) {
	GlobalMu.Lock()
	defer GlobalMu.Unlock()
	GlobalProjectErrorSetter = setter
}

// CheckAllProjectComponents calls the registered project component checker
func CheckAllProjectComponents() []ProjectComponentError {
	GlobalMu.RLock()
	checker := GlobalProjectComponentChecker
	GlobalMu.RUnlock()

	if checker != nil {
		return checker()
	}
	return nil
}

// SetProjectErrorStatus calls the registered project error setter
func SetProjectErrorStatus(projectID string, componentErrors []ProjectComponentError) {
	GlobalMu.RLock()
	setter := GlobalProjectErrorSetter
	GlobalMu.RUnlock()

	if setter != nil {
		setter(projectID, componentErrors)
	}
}

func init() {
	AllInputsRawConfig = make(map[string]string, 0)
	AllOutputsRawConfig = make(map[string]string, 0)
	AllRulesetsRawConfig = make(map[string]string, 0)
	AllProjectRawConfig = make(map[string]string, 0)
	AllPluginsRawConfig = make(map[string]string, 0)
}
