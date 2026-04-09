package api

import (
	"AgentSmith-HUB/agent"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"AgentSmith-HUB/skill"
	"fmt"
	"sync"
	"time"
)

var pendingChangeSyncState = struct {
	mu       sync.Mutex
	dirty    bool
	lastSync time.Time
}{
	dirty: true,
}

const pendingChangeSyncInterval = 2 * time.Second

func ensurePendingChangesSynced(force bool) {
	pendingChangeSyncState.mu.Lock()
	shouldSync := force || pendingChangeSyncState.dirty || time.Since(pendingChangeSyncState.lastSync) >= pendingChangeSyncInterval
	if !shouldSync {
		pendingChangeSyncState.mu.Unlock()
		return
	}
	pendingChangeSyncState.dirty = false
	pendingChangeSyncState.lastSync = time.Now()
	pendingChangeSyncState.mu.Unlock()

	syncLegacyToEnhancedManager()
}

func markPendingChangesDirty() {
	pendingChangeSyncState.mu.Lock()
	pendingChangeSyncState.dirty = true
	pendingChangeSyncState.mu.Unlock()
}

// syncLegacyToEnhancedManager synchronizes data from legacy storage to the enhanced manager
func syncLegacyToEnhancedManager() {
	syncPluginsToEnhancedManager()
	syncInputsToEnhancedManager()
	syncOutputsToEnhancedManager()
	syncRulesetsToEnhancedManager()
	syncProjectsToEnhancedManager()
	syncAgentsToEnhancedManager()
	syncSkillsToEnhancedManager()
	cleanupObsoleteChanges()
}

func syncPluginsToEnhancedManager() {
	pluginsData := plugin.GetAllPluginsNew()
	for name, newContent := range pluginsData {
		var oldContent string
		isNew := true

		oldContent = getExistingPluginContent(name)
		if oldContent != "" {
			isNew = false
		}

		if existingChange, exists := globalPendingChangeManager.GetChange("plugin", name); exists {
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("plugin", name, newContent, oldContent, isNew)
			}
		} else {
			globalPendingChangeManager.AddChange("plugin", name, newContent, oldContent, isNew)
		}
	}
}

func syncInputsToEnhancedManager() {
	inputsData := project.GetAllInputsNew()
	for id, newContent := range inputsData {
		var oldContent string
		isNew := true

		if i, ok := project.GetInput(id); ok {
			oldContent = i.Config.RawConfig
			isNew = false
		}

		if existingChange, exists := globalPendingChangeManager.GetChange("input", id); exists {
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("input", id, newContent, oldContent, isNew)
			}
		} else {
			globalPendingChangeManager.AddChange("input", id, newContent, oldContent, isNew)
		}
	}
}

func syncOutputsToEnhancedManager() {
	outputsData := project.GetAllOutputsNew()
	for id, newContent := range outputsData {
		var oldContent string
		isNew := true

		if o, ok := project.GetOutput(id); ok {
			oldContent = o.Config.RawConfig
			isNew = false
		}

		if existingChange, exists := globalPendingChangeManager.GetChange("output", id); exists {
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("output", id, newContent, oldContent, isNew)
			}
		} else {
			globalPendingChangeManager.AddChange("output", id, newContent, oldContent, isNew)
		}
	}
}

func syncRulesetsToEnhancedManager() {
	rulesetsData := project.GetAllRulesetsNew()
	for id, newContent := range rulesetsData {
		var oldContent string
		isNew := true

		if ruleset, ok := project.GetRuleset(id); ok {
			oldContent = ruleset.RawConfig
			isNew = false
		}

		if existingChange, exists := globalPendingChangeManager.GetChange("ruleset", id); exists {
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("ruleset", id, newContent, oldContent, isNew)
			}
		} else {
			globalPendingChangeManager.AddChange("ruleset", id, newContent, oldContent, isNew)
		}
	}
}

func syncProjectsToEnhancedManager() {
	projectsData := project.GetAllProjectsNew()
	for id, newContent := range projectsData {
		var oldContent string
		isNew := true

		if proj, ok := project.GetProject(id); ok {
			oldContent = proj.Config.RawConfig
			isNew = false
		}

		if existingChange, exists := globalPendingChangeManager.GetChange("project", id); exists {
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("project", id, newContent, oldContent, isNew)
			}
		} else {
			globalPendingChangeManager.AddChange("project", id, newContent, oldContent, isNew)
		}
	}
}

func syncAgentsToEnhancedManager() {
	agentsData := project.GetAllAgentsNew()
	for id, newContent := range agentsData {
		var oldContent string
		isNew := true

		if a, ok := project.GetAgent(id); ok {
			oldContent = a.RawConfig
			isNew = false
		}

		if existingChange, exists := globalPendingChangeManager.GetChange("agent", id); exists {
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("agent", id, newContent, oldContent, isNew)
			}
		} else {
			globalPendingChangeManager.AddChange("agent", id, newContent, oldContent, isNew)
		}
	}
}

func syncSkillsToEnhancedManager() {
	skillsData := project.GetAllSkillsNew()
	for id, newContent := range skillsData {
		var oldContent string
		isNew := true

		if s, ok := project.GetSkill(id); ok {
			oldContent = s.RawConfig
			isNew = false
		}

		if existingChange, exists := globalPendingChangeManager.GetChange("skill", id); exists {
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("skill", id, newContent, oldContent, isNew)
			}
		} else {
			globalPendingChangeManager.AddChange("skill", id, newContent, oldContent, isNew)
		}
	}
}

func getExistingPluginContent(id string) string {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	if pluginInstance, exists := plugin.Plugins[id]; exists {
		return string(pluginInstance.Payload)
	}
	return ""
}

func cleanupObsoleteChanges() {
	existingChanges := globalPendingChangeManager.GetAllChanges()
	shouldExist := make(map[string]bool)

	pluginsNewData := plugin.GetAllPluginsNew()
	for name := range pluginsNewData {
		shouldExist[fmt.Sprintf("plugin:%s", name)] = true
	}
	for id := range project.GetAllInputsNew() {
		shouldExist[fmt.Sprintf("input:%s", id)] = true
	}
	for id := range project.GetAllOutputsNew() {
		shouldExist[fmt.Sprintf("output:%s", id)] = true
	}
	for id := range project.GetAllRulesetsNew() {
		shouldExist[fmt.Sprintf("ruleset:%s", id)] = true
	}
	for id := range project.GetAllProjectsNew() {
		shouldExist[fmt.Sprintf("project:%s", id)] = true
	}
	for id := range project.GetAllAgentsNew() {
		shouldExist[fmt.Sprintf("agent:%s", id)] = true
	}
	for id := range project.GetAllSkillsNew() {
		shouldExist[fmt.Sprintf("skill:%s", id)] = true
	}

	for _, change := range existingChanges {
		key := fmt.Sprintf("%s:%s", change.Type, change.ID)
		if !shouldExist[key] {
			globalPendingChangeManager.RemoveChange(change.Type, change.ID)
			logger.Info("Removed obsolete pending change from Enhanced Manager",
				"type", change.Type,
				"id", change.ID)
		}
	}
}

func verifyPendingChangeContent(changeType, id, content string) error {
	switch changeType {
	case "plugin":
		return plugin.Verify("", content, id)
	case "input":
		return input.Verify("", content)
	case "output":
		return output.Verify("", content)
	case "ruleset":
		return rules_engine.Verify("", content)
	case "project":
		return project.Verify("", content)
	case "agent":
		return agent.Verify("", content)
	case "skill":
		return skill.Verify("", content)
	default:
		return fmt.Errorf("unsupported component type: %s", changeType)
	}
}
