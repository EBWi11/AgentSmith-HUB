package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/plugin"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// CompSyncEvt defines component sync event structure
type CompSyncEvt struct {
	Op        string `json:"op"`   // add|update|delete
	Type      string `json:"type"` // input|output|ruleset|plugin|project
	ID        string `json:"id"`
	Raw       string `json:"raw,omitempty"`
	IsRunning bool   `json:"is_running,omitempty"`
}

// PublishComponentSync is called by leader after component change
func PublishComponentSync(evt *CompSyncEvt) {
	if evt == nil {
		return
	}

	// Only leader should publish component sync events
	if !IsLeader {
		logger.Warn("Non-leader attempted to publish component sync event", "type", evt.Type, "id", evt.ID)
		return
	}

	data, err := json.Marshal(evt)
	if err != nil {
		logger.Error("Failed to marshal component sync event", "error", err)
		return
	}

	if err := common.RedisPublish("cluster:component_sync", string(data)); err != nil {
		logger.Error("Failed to publish component sync event", "error", err)
		return
	}

	logger.Debug("Published component sync event", "type", evt.Type, "id", evt.ID, "op", evt.Op)
}

// startComponentSyncSubscriber starts follower listener
func (cm *ClusterManager) startComponentSyncSubscriber() {
	if cm.IsLeader() {
		return // leader doesn't need to subscribe
	}

	client := common.GetRedisClient()
	if client == nil {
		logger.Error("Redis client not initialized, component sync subscriber not started")
		return
	}

	pubsub := client.Subscribe(context.Background(), "cluster:component_sync")

	// Create dedicated stop channel for component sync subscriber
	if cm.stopChan == nil {
		cm.stopChan = make(chan struct{})
	}
	stopComponentSync := make(chan struct{})

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Component sync subscriber panic", "panic", r)
			}
			_ = pubsub.Close()
		}()

		ch := pubsub.Channel()
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					logger.Warn("Component sync channel closed")
					return
				}

				var evt CompSyncEvt
				if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil {
					logger.Error("Failed to unmarshal component sync event", "error", err)
					continue
				}

				// Apply component sync with error handling
				if err := applyComponentSyncFollower(&evt); err != nil {
					logger.Error("Failed to apply component sync", "error", err, "type", evt.Type, "id", evt.ID)
				} else {
					// Successfully applied configuration
					logger.Debug("Component sync applied successfully", "type", evt.Type, "id", evt.ID)

					// For non-project components, restart affected projects
					if evt.Type != "project" {
						go restartAffectedProjectsOnFollower(evt.Type, evt.ID)
					}

					// Note: We don't update config timestamp here because this is passive sync,
					// not an active configuration change. The config version will be updated
					// through the heartbeat mechanism when Leader detects drift.
				}

			case <-stopComponentSync:
				logger.Info("Component sync subscriber stopped")
				return
			case <-cm.stopChan:
				logger.Info("Component sync subscriber stopped via global stop")
				return
			}
		}
	}()

	// Store the stop channel for proper cleanup
	cm.stopComponentSync = stopComponentSync
	logger.Info("Component sync subscriber started")
}

// applyComponentSyncFollower handles component change on follower with error handling
func applyComponentSyncFollower(evt *CompSyncEvt) error {
	if evt == nil {
		return fmt.Errorf("component sync event is nil")
	}

	// Validate event
	if evt.Type == "" || evt.ID == "" {
		return fmt.Errorf("invalid component sync event: missing type or id")
	}

	// For brevity we only update raw config maps; deeper hot-update reuse existing logic may be added later
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	// Initialize maps if they don't exist
	if common.AllInputsRawConfig == nil {
		common.AllInputsRawConfig = make(map[string]string)
	}
	if common.AllOutputsRawConfig == nil {
		common.AllOutputsRawConfig = make(map[string]string)
	}
	if common.AllRulesetsRawConfig == nil {
		common.AllRulesetsRawConfig = make(map[string]string)
	}
	if common.AllProjectRawConfig == nil {
		common.AllProjectRawConfig = make(map[string]string)
	}
	if common.AllPluginsRawConfig == nil {
		common.AllPluginsRawConfig = make(map[string]string)
	}

	switch evt.Type {
	case "ruleset":
		if evt.Op == "delete" {
			delete(common.AllRulesetsRawConfig, evt.ID)
		} else {
			if evt.Raw == "" && evt.Op != "delete" {
				return fmt.Errorf("empty content for ruleset %s", evt.ID)
			}
			common.AllRulesetsRawConfig[evt.ID] = evt.Raw
		}
	case "input":
		if evt.Op == "delete" {
			delete(common.AllInputsRawConfig, evt.ID)
		} else {
			if evt.Raw == "" && evt.Op != "delete" {
				return fmt.Errorf("empty content for input %s", evt.ID)
			}
			common.AllInputsRawConfig[evt.ID] = evt.Raw
		}
	case "output":
		if evt.Op == "delete" {
			delete(common.AllOutputsRawConfig, evt.ID)
		} else {
			if evt.Raw == "" && evt.Op != "delete" {
				return fmt.Errorf("empty content for output %s", evt.ID)
			}
			common.AllOutputsRawConfig[evt.ID] = evt.Raw
		}
	case "project":
		if evt.Op == "delete" {
			delete(common.AllProjectRawConfig, evt.ID)
		} else {
			if evt.Raw == "" && evt.Op != "delete" {
				return fmt.Errorf("empty content for project %s", evt.ID)
			}
			common.AllProjectRawConfig[evt.ID] = evt.Raw
		}
	case "plugin":
		if evt.Op == "delete" {
			delete(common.AllPluginsRawConfig, evt.ID)
		} else {
			if evt.Raw == "" && evt.Op != "delete" {
				return fmt.Errorf("empty content for plugin %s", evt.ID)
			}
			common.AllPluginsRawConfig[evt.ID] = evt.Raw
		}
	default:
		return fmt.Errorf("unsupported component type: %s", evt.Type)
	}

	logger.Info("Applied component sync", "type", evt.Type, "op", evt.Op, "id", evt.ID)
	return nil
}

// updateComponentInstanceOnFollower updates the actual component instance on follower nodes
func updateComponentInstanceOnFollower(evt *CompSyncEvt) error {
	if evt.Op == "delete" {
		return nil // Deletion is handled by the config map update
	}

	// We need to avoid circular imports, so we'll use a different approach
	// For now, we'll rely on the project restart mechanism to pick up the new config
	// TODO: Implement proper component instance updates without circular imports

	logger.Debug("Component instance update deferred to project restart", "type", evt.Type, "id", evt.ID)
	return nil
}

// restartAffectedProjectsOnFollower restarts projects affected by component changes on follower nodes
func restartAffectedProjectsOnFollower(componentType, componentID string) {
	// For plugin updates, use safe update mechanism
	if componentType == "plugin" {
		handlePluginUpdateOnFollower(componentType, componentID)
		return
	}

	// For other components, use the existing restart logic
	affectedProjects := getAffectedProjectsOnFollower(componentType, componentID)

	if len(affectedProjects) == 0 {
		logger.Debug("No projects affected by component update", "type", componentType, "id", componentID)
		return
	}

	logger.Info("Restarting affected projects on follower", "component_type", componentType, "component_id", componentID, "affected_count", len(affectedProjects))

	// Restart affected projects using the same logic as project package
	restartedCount := 0
	for _, projectID := range affectedProjects {
		if err := restartProjectOnFollower(projectID); err != nil {
			logger.Error("Failed to restart affected project on follower", "project_id", projectID, "error", err)
		} else {
			restartedCount++
			logger.Info("Successfully restarted affected project on follower", "project_id", projectID)
		}
	}

	logger.Info("Completed restarting affected projects on follower", "component_type", componentType, "component_id", componentID, "restarted", restartedCount, "total", len(affectedProjects))
}

// handlePluginUpdateOnFollower handles safe plugin updates on follower nodes
func handlePluginUpdateOnFollower(componentType, componentID string) {
	if componentType != "plugin" {
		return
	}

	pluginID := componentID
	logger.Info("Starting safe plugin update on follower", "plugin", pluginID)

	// Phase 1: Get affected projects and stop them
	affectedProjects := getAffectedProjectsOnFollower("plugin", pluginID)
	if len(affectedProjects) > 0 {
		logger.Info("Stopping affected projects for plugin update on follower", "plugin", pluginID, "projects", affectedProjects)

		for _, projectID := range affectedProjects {
			if err := stopProjectOnFollower(projectID); err != nil {
				logger.Error("Failed to stop project on follower for plugin update", "project", projectID, "error", err)
			}
		}

		// Wait a moment for projects to stop
		time.Sleep(2 * time.Second)
		logger.Info("Affected projects stopped on follower", "plugin", pluginID)
	}

	// Phase 2: Plugin update is already handled by applyComponentSyncFollower
	// The new plugin content is already in the global config map
	// We need to reload the plugin instance

	// Get the new plugin content from global config map
	common.GlobalMu.RLock()
	newContent, exists := common.AllPluginsRawConfig[pluginID]
	common.GlobalMu.RUnlock()

	if !exists || newContent == "" {
		logger.Error("Plugin content not found in global config map", "plugin", pluginID)
		return
	}

	// Update plugin instance safely on follower
	if err := updatePluginInstanceSafelyOnFollower(pluginID, newContent); err != nil {
		logger.Error("Failed to update plugin instance on follower", "plugin", pluginID, "error", err)
		return
	}

	// Phase 3: Restart affected projects
	logger.Info("Restarting affected projects with new plugin on follower", "plugin", pluginID, "projects", affectedProjects)
	for _, projectID := range affectedProjects {
		if err := restartProjectOnFollower(projectID); err != nil {
			logger.Error("Failed to restart project on follower after plugin update", "project", projectID, "error", err)
		} else {
			logger.Info("Successfully restarted project on follower after plugin update", "project", projectID)
		}
	}

	logger.Info("Safe plugin update completed on follower", "plugin", pluginID)
}

// updatePluginInstanceSafelyOnFollower safely updates plugin instance on follower
func updatePluginInstanceSafelyOnFollower(pluginID, newContent string) error {
	// Import the required packages at the top level to avoid issues
	// This is similar to the leader implementation but adapted for follower

	// Phase 1: Clean up old plugin instance
	common.GlobalMu.Lock()
	oldPlugin, exists := plugin.Plugins[pluginID]
	if exists {
		// Clear Yaegi interpreter instance to prevent memory leaks
		if oldPlugin.Type == plugin.YAEGI_PLUGIN && oldPlugin != nil {
			// The yaegi interpreter will be garbage collected
			oldPlugin = nil
		}
		// Remove from global mapping
		delete(plugin.Plugins, pluginID)
	}
	common.GlobalMu.Unlock()

	// Phase 2: Force garbage collection
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// Phase 3: Create new plugin instance from content (no file write on follower)
	if err := plugin.NewPlugin("", newContent, pluginID, plugin.YAEGI_PLUGIN); err != nil {
		return fmt.Errorf("failed to create new plugin on follower: %w", err)
	}

	logger.Info("Plugin instance updated safely on follower", "plugin", pluginID)
	return nil
}

// stopProjectOnFollower stops a project on follower node
func stopProjectOnFollower(projectID string) error {
	if globalProjectCmdHandler == nil {
		return fmt.Errorf("project command handler not initialized")
	}

	return globalProjectCmdHandler.ExecuteCommand(projectID, "stop")
}

// getAffectedProjectsOnFollower returns the list of project IDs affected by component changes (follower version)
func getAffectedProjectsOnFollower(componentType string, componentID string) []string {
	affectedProjects := make(map[string]struct{})

	// Access project information through global config maps
	// We'll check which projects reference this component in their raw config
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	// Check all project configurations for references to this component
	if common.AllProjectRawConfig != nil {
		for projectID, projectConfig := range common.AllProjectRawConfig {
			// Check if the project config references this component
			componentRef := ""
			switch componentType {
			case "input":
				componentRef = "INPUT." + componentID
			case "output":
				componentRef = "OUTPUT." + componentID
			case "ruleset":
				componentRef = "RULESET." + componentID
			case "plugin":
				// Plugins are not directly referenced in project content, skip for now
				continue
			}

			if componentRef != "" && strings.Contains(projectConfig, componentRef) {
				affectedProjects[projectID] = struct{}{}
				logger.Debug("Project affected by component change", "project_id", projectID, "component_type", componentType, "component_id", componentID)
			}
		}
	}

	// Convert to string slice
	result := make([]string, 0, len(affectedProjects))
	for projectID := range affectedProjects {
		result = append(result, projectID)
	}

	logger.Debug("Determined affected projects", "component_type", componentType, "component_id", componentID, "affected_count", len(result))
	return result
}

// restartProjectOnFollower restarts a single project on follower node
func restartProjectOnFollower(projectID string) error {
	if globalProjectCmdHandler == nil {
		return fmt.Errorf("project command handler not initialized")
	}

	// Use the project command handler to restart the project
	return globalProjectCmdHandler.ExecuteCommand(projectID, "restart")
}
