package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// PendingChange represents a component with pending changes
type PendingChange struct {
	Type       string `json:"type"`        // Component type (input, output, ruleset, project, plugin)
	ID         string `json:"id"`          // Component ID
	IsNew      bool   `json:"is_new"`      // Whether this is a new component
	OldContent string `json:"old_content"` // Original content
	NewContent string `json:"new_content"` // New content
}

// SingleChangeRequest represents a request to apply a single change
type SingleChangeRequest struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// ComponentSyncRequest represents a request to sync a component to follower nodes
type ComponentSyncRequest struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Content string `json:"content"`
}

// GetPendingChanges returns all components with pending changes (.new files)
func GetPendingChanges(c echo.Context) error {
	changes := []PendingChange{}

	// Lock for reading all pending changes and existing components
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	p := project.GlobalProject

	// Check plugins with pending changes
	for name, newContent := range plugin.PluginsNew {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing plugin
		if p, ok := plugin.Plugins[name]; ok {
			oldContent = string(p.Payload)
			isNew = false
		}

		changes = append(changes, PendingChange{
			Type:       "plugin",
			ID:         name,
			IsNew:      isNew,
			OldContent: oldContent,
			NewContent: newContent,
		})
	}

	// Check inputs with pending changes
	for id, newContent := range p.InputsNew {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing input
		if i, ok := p.Inputs[id]; ok {
			oldContent = i.Config.RawConfig
			isNew = false
		}

		changes = append(changes, PendingChange{
			Type:       "input",
			ID:         id,
			IsNew:      isNew,
			OldContent: oldContent,
			NewContent: newContent,
		})
	}

	// Check outputs with pending changes
	for id, newContent := range p.OutputsNew {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing output
		if o, ok := p.Outputs[id]; ok {
			oldContent = o.Config.RawConfig
			isNew = false
		}

		changes = append(changes, PendingChange{
			Type:       "output",
			ID:         id,
			IsNew:      isNew,
			OldContent: oldContent,
			NewContent: newContent,
		})
	}

	// Check rulesets with pending changes
	for id, newContent := range p.RulesetsNew {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing ruleset
		if ruleset, ok := p.Rulesets[id]; ok {
			oldContent = ruleset.RawConfig
			isNew = false
		}

		changes = append(changes, PendingChange{
			Type:       "ruleset",
			ID:         id,
			IsNew:      isNew,
			OldContent: oldContent,
			NewContent: newContent,
		})
	}

	// Check projects with pending changes
	for id, newContent := range p.ProjectsNew {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing project
		if p, ok := p.Projects[id]; ok {
			oldContent = p.Config.RawConfig
			isNew = false
		}

		changes = append(changes, PendingChange{
			Type:       "project",
			ID:         id,
			IsNew:      isNew,
			OldContent: oldContent,
			NewContent: newContent,
		})
	}

	return c.JSON(http.StatusOK, changes)
}

// ApplyPendingChanges applies all pending changes by merging .new files with their originals
// and syncs changes to follower nodes
func ApplyPendingChanges(c echo.Context) error {
	successCount := 0
	failureCount := 0
	verifyFailures := []map[string]string{}

	// Track projects that need restart
	projectsToRestart := make(map[string]struct{})

	// Lock for reading all pending changes
	common.GlobalMu.RLock()

	// Create local copies to avoid holding lock during processing
	pluginsToProcess := make(map[string]string)
	for k, v := range plugin.PluginsNew {
		pluginsToProcess[k] = v
	}

	inputsToProcess := make(map[string]string)
	for k, v := range project.GlobalProject.InputsNew {
		inputsToProcess[k] = v
	}

	outputsToProcess := make(map[string]string)
	for k, v := range project.GlobalProject.OutputsNew {
		outputsToProcess[k] = v
	}

	rulesetsToProcess := make(map[string]string)
	for k, v := range project.GlobalProject.RulesetsNew {
		rulesetsToProcess[k] = v
	}

	projectsToProcess := make(map[string]string)
	for k, v := range project.GlobalProject.ProjectsNew {
		projectsToProcess[k] = v
	}

	common.GlobalMu.RUnlock()

	// Apply plugin changes
	for name, content := range pluginsToProcess {
		// Remove existing plugin from memory before verification to avoid name conflict
		common.GlobalMu.Lock()
		delete(plugin.Plugins, name)
		common.GlobalMu.Unlock()

		// Verify plugin configuration
		err := plugin.Verify("", content, name)
		if err != nil {
			logger.Error("Plugin verification failed", "name", name, "error", err)
			failureCount++
			verifyFailures = append(verifyFailures, map[string]string{
				"type":  "plugin",
				"id":    name,
				"error": err.Error(),
			})
			continue
		}

		err = mergePluginFile(name)
		if err != nil {
			logger.Error("Failed to apply plugin changes", "name", name, "error", err)
			failureCount++
		} else {
			// Reload the plugin into memory after successful merge
			configRoot := common.Config.ConfigRoot
			pluginPath := path.Join(configRoot, "plugin", name+".go")
			reloadErr := plugin.NewPlugin(pluginPath, "", name, plugin.YAEGI_PLUGIN)
			if reloadErr != nil {
				logger.Error("Failed to reload plugin after merge", "name", name, "error", reloadErr)
				failureCount++
			} else {
				// Clear the memory map entry after successful merge
				common.GlobalMu.Lock()
				delete(plugin.PluginsNew, name)
				common.GlobalMu.Unlock()

				successCount++
				// Sync to follower nodes
				syncComponentToFollowers("plugin", name)

				// Plugin changes may affect all projects, but we don't automatically restart projects
				logger.Info("Plugin updated, manual restart of affected projects may be required", "name", name)
			}
		}
	}

	// Apply input changes
	for id, content := range inputsToProcess {
		// Verify input configuration
		err := input.Verify("", content)
		if err != nil {
			logger.Error("Input verification failed", "id", id, "error", err)
			failureCount++
			verifyFailures = append(verifyFailures, map[string]string{
				"type":  "input",
				"id":    id,
				"error": err.Error(),
			})
			continue
		}

		err = mergeComponentFile("input", id)
		if err != nil {
			logger.Error("Failed to apply input changes", "id", id, "error", err)
			failureCount++
		} else {
			// Reload the input component into memory after successful merge
			configRoot := common.Config.ConfigRoot
			inputPath := path.Join(configRoot, "input", id+".yaml")

			// Stop old component if it exists
			common.GlobalMu.RLock()
			oldInput, exists := project.GlobalProject.Inputs[id]
			common.GlobalMu.RUnlock()
			if exists {
				stopErr := oldInput.Stop()
				if stopErr != nil {
					logger.Error("Failed to stop old input", "id", id, "error", stopErr)
				}
			}

			newInput, reloadErr := input.NewInput(inputPath, "", id)
			if reloadErr != nil {
				logger.Error("Failed to reload input after merge", "id", id, "error", reloadErr)
				failureCount++
			} else {
				common.GlobalMu.Lock()
				project.GlobalProject.Inputs[id] = newInput
				// Clear the memory map entry after successful merge
				delete(project.GlobalProject.InputsNew, id)
				common.GlobalMu.Unlock()

				successCount++
				// Sync to follower nodes
				syncComponentToFollowers("input", id)

				// Get affected projects
				affectedProjects := project.GetAffectedProjects("input", id)
				for _, projectID := range affectedProjects {
					projectsToRestart[projectID] = struct{}{}
				}
			}
		}
	}

	// Apply output changes
	for id, content := range outputsToProcess {
		// Verify output configuration
		err := output.Verify("", content)
		if err != nil {
			logger.Error("Output verification failed", "id", id, "error", err)
			failureCount++
			verifyFailures = append(verifyFailures, map[string]string{
				"type":  "output",
				"id":    id,
				"error": err.Error(),
			})
			continue
		}

		err = mergeComponentFile("output", id)
		if err != nil {
			logger.Error("Failed to apply output changes", "id", id, "error", err)
			failureCount++
		} else {
			// Reload the output component into memory after successful merge
			configRoot := common.Config.ConfigRoot
			outputPath := path.Join(configRoot, "output", id+".yaml")

			// Stop old component if it exists
			common.GlobalMu.RLock()
			oldOutput, exists := project.GlobalProject.Outputs[id]
			common.GlobalMu.RUnlock()
			if exists {
				stopErr := oldOutput.Stop()
				if stopErr != nil {
					logger.Error("Failed to stop old output", "id", id, "error", stopErr)
				}
			}

			newOutput, reloadErr := output.NewOutput(outputPath, "", id)
			if reloadErr != nil {
				logger.Error("Failed to reload output after merge", "id", id, "error", reloadErr)
				failureCount++
			} else {
				common.GlobalMu.Lock()
				project.GlobalProject.Outputs[id] = newOutput
				// Clear the memory map entry after successful merge
				delete(project.GlobalProject.OutputsNew, id)
				common.GlobalMu.Unlock()

				successCount++
				// Sync to follower nodes
				syncComponentToFollowers("output", id)

				// Get affected projects
				affectedProjects := project.GetAffectedProjects("output", id)
				for _, projectID := range affectedProjects {
					projectsToRestart[projectID] = struct{}{}
				}
			}
		}
	}

	// Apply ruleset changes
	for id, content := range rulesetsToProcess {
		// Verify ruleset configuration
		err := rules_engine.Verify("", content)
		if err != nil {
			logger.Error("Ruleset verification failed", "id", id, "error", err)
			failureCount++
			verifyFailures = append(verifyFailures, map[string]string{
				"type":  "ruleset",
				"id":    id,
				"error": err.Error(),
			})
			continue
		}

		err = mergeComponentFile("ruleset", id)
		if err != nil {
			logger.Error("Failed to apply ruleset changes", "id", id, "error", err)
			failureCount++
		} else {
			// Reload the ruleset component into memory after successful merge
			configRoot := common.Config.ConfigRoot
			rulesetPath := path.Join(configRoot, "ruleset", id+".xml")

			// Stop old component if it exists
			common.GlobalMu.RLock()
			oldRuleset, exists := project.GlobalProject.Rulesets[id]
			common.GlobalMu.RUnlock()
			if exists {
				stopErr := oldRuleset.Stop()
				if stopErr != nil {
					logger.Error("Failed to stop old ruleset", "id", id, "error", stopErr)
				}
			}

			newRuleset, reloadErr := rules_engine.NewRuleset(rulesetPath, "", id)
			if reloadErr != nil {
				logger.Error("Failed to reload ruleset after merge", "id", id, "error", reloadErr)
				failureCount++
			} else {
				common.GlobalMu.Lock()
				project.GlobalProject.Rulesets[id] = newRuleset
				// Clear the memory map entry after successful merge
				delete(project.GlobalProject.RulesetsNew, id)
				common.GlobalMu.Unlock()

				successCount++
				// Sync to follower nodes
				syncComponentToFollowers("ruleset", id)

				// Get affected projects
				affectedProjects := project.GetAffectedProjects("ruleset", id)
				for _, projectID := range affectedProjects {
					projectsToRestart[projectID] = struct{}{}
				}
			}
		}
	}

	// Apply project changes
	for id, content := range projectsToProcess {
		// Verify project configuration
		err := project.Verify("", content)
		if err != nil {
			logger.Error("Project verification failed", "id", id, "error", err)
			failureCount++
			verifyFailures = append(verifyFailures, map[string]string{
				"type":  "project",
				"id":    id,
				"error": err.Error(),
			})
			continue
		}

		err = mergeComponentFile("project", id)
		if err != nil {
			logger.Error("Failed to apply project changes", "id", id, "error", err)
			failureCount++
		} else {
			// Reload the project component into memory after successful merge
			configRoot := common.Config.ConfigRoot
			projectPath := path.Join(configRoot, "project", id+".yaml")

			// Handle project lifecycle carefully
			var wasRunning bool
			common.GlobalMu.RLock()
			oldProject, exists := project.GlobalProject.Projects[id]
			common.GlobalMu.RUnlock()
			if exists {
				wasRunning = (oldProject.Status == project.ProjectStatusRunning)
				if wasRunning {
					stopErr := oldProject.Stop()
					if stopErr != nil {
						logger.Error("Failed to stop old project", "id", id, "error", stopErr)
					}
				}
			}

			newProject, reloadErr := project.NewProject(projectPath, "", id)
			if reloadErr != nil {
				logger.Error("Failed to reload project after merge", "id", id, "error", reloadErr)
				failureCount++
			} else {
				common.GlobalMu.Lock()
				project.GlobalProject.Projects[id] = newProject
				// Clear the memory map entry after successful merge
				delete(project.GlobalProject.ProjectsNew, id)
				common.GlobalMu.Unlock()

				// Restart project if it was previously running
				if wasRunning {
					startErr := newProject.Start()
					if startErr != nil {
						logger.Error("Failed to restart project after reload", "id", id, "error", startErr)
					}
				}

				successCount++
				// Sync to follower nodes
				syncComponentToFollowers("project", id)

				// Get affected projects (the project itself and projects that depend on it)
				affectedProjects := project.GetAffectedProjects("project", id)
				for _, projectID := range affectedProjects {
					projectsToRestart[projectID] = struct{}{}
				}
			}
		}
	}

	// Reload components after applying changes
	reloadComponents()

	// Update project dependencies
	project.AnalyzeProjectDependencies()

	// Restart affected projects
	if len(projectsToRestart) > 0 {
		logger.Info("Restarting affected projects", "count", len(projectsToRestart))

		// Convert map to sorted slice to ensure consistent restart order
		projectIDs := make([]string, 0, len(projectsToRestart))
		for id := range projectsToRestart {
			projectIDs = append(projectIDs, id)
		}
		sort.Strings(projectIDs)

		// First stop all affected projects
		for _, id := range projectIDs {
			common.GlobalMu.RLock()
			p, exists := project.GlobalProject.Projects[id]
			common.GlobalMu.RUnlock()

			if !exists {
				logger.Error("Project not found for restart", "id", id)
				continue
			}

			if p.Status == project.ProjectStatusRunning {
				logger.Info("Stopping project for restart", "id", id)
				err := p.Stop()
				if err != nil {
					logger.Error("Failed to stop project", "id", id, "error", err)
				}
			}
		}

		// Then start all affected projects
		for _, id := range projectIDs {
			common.GlobalMu.RLock()
			p, exists := project.GlobalProject.Projects[id]
			common.GlobalMu.RUnlock()

			if !exists {
				continue
			}

			if p.Status == project.ProjectStatusStopped {
				logger.Info("Starting project after changes", "id", id)
				err := p.Start()
				if err != nil {
					logger.Error("Failed to start project", "id", id, "error", err)
				}
			}
		}
	}

	if len(verifyFailures) > 0 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":           "Some changes failed verification and were not applied",
			"verify_failures": verifyFailures,
			"success_count":   successCount,
			"failure_count":   failureCount,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success_count":      successCount,
		"failure_count":      failureCount,
		"restarted_projects": len(projectsToRestart),
	})
}

// ApplySingleChange applies a single pending change
func ApplySingleChange(c echo.Context) error {
	// Add panic recovery
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Panic in ApplySingleChange", "panic", r)
		}
	}()

	var req SingleChangeRequest
	if err := c.Bind(&req); err != nil {
		logger.Error("Failed to bind request in ApplySingleChange", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	logger.Info("ApplySingleChange request", "type", req.Type, "id", req.ID)

	// First verify configuration with lock protection
	var verifyErr error
	var content string
	var found bool

	// Lock for reading pending changes
	common.GlobalMu.RLock()
	switch req.Type {
	case "plugin":
		content, found = plugin.PluginsNew[req.ID]
	case "input":
		content, found = project.GlobalProject.InputsNew[req.ID]
	case "output":
		content, found = project.GlobalProject.OutputsNew[req.ID]
	case "ruleset":
		content, found = project.GlobalProject.RulesetsNew[req.ID]
	case "project":
		content, found = project.GlobalProject.ProjectsNew[req.ID]
	default:
		common.GlobalMu.RUnlock()
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid component type"})
	}
	common.GlobalMu.RUnlock()

	if !found {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("No pending changes found for this %s", req.Type)})
	}

	// Verify configuration (without holding lock)
	switch req.Type {
	case "plugin":
		// Remove existing plugin from memory before verification to avoid name conflict
		common.GlobalMu.Lock()
		delete(plugin.Plugins, req.ID)
		common.GlobalMu.Unlock()
		verifyErr = plugin.Verify("", content, req.ID)
	case "input":
		verifyErr = input.Verify("", content)
	case "output":
		verifyErr = output.Verify("", content)
	case "ruleset":
		verifyErr = rules_engine.Verify("", content)
	case "project":
		verifyErr = project.Verify("", content)
	}

	// If verification fails, return error
	if verifyErr != nil {
		logger.Error("Configuration verification failed", "type", req.Type, "id", req.ID, "error", verifyErr)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Configuration verification failed: %s", verifyErr.Error()),
		})
	}

	var err error
	switch req.Type {
	case "plugin":
		err = mergePluginFile(req.ID)
		if err == nil {
			// Clear the memory map entry after successful merge
			common.GlobalMu.Lock()
			delete(plugin.PluginsNew, req.ID)
			common.GlobalMu.Unlock()

			// Reload the plugin component
			configRoot := common.Config.ConfigRoot
			pluginPath := path.Join(configRoot, "plugin", req.ID+".go")
			err = plugin.NewPlugin(pluginPath, "", req.ID, plugin.YAEGI_PLUGIN)
			if err != nil {
				logger.Error("Failed to reload plugin after merge", "id", req.ID, "error", err)
			}
		}
	case "input", "output", "ruleset", "project":
		err = mergeComponentFile(req.Type, req.ID)
		if err == nil {
			// Clear the memory map entry after successful merge and reload components
			switch req.Type {
			case "input":
				common.GlobalMu.Lock()
				delete(project.GlobalProject.InputsNew, req.ID)
				common.GlobalMu.Unlock()

				// Reload the input component
				configRoot := common.Config.ConfigRoot
				inputPath := path.Join(configRoot, "input", req.ID+".yaml")

				// Stop old component if it exists
				common.GlobalMu.RLock()
				oldInput, exists := project.GlobalProject.Inputs[req.ID]
				common.GlobalMu.RUnlock()
				if exists {
					err := oldInput.Stop()
					if err != nil {
						logger.Error("Failed to stop old input", "id", req.ID, "error", err)
					}
				}

				newInput, reloadErr := input.NewInput(inputPath, "", req.ID)
				if reloadErr != nil {
					logger.Error("Failed to reload input after merge", "id", req.ID, "error", reloadErr)
				} else {
					common.GlobalMu.Lock()
					project.GlobalProject.Inputs[req.ID] = newInput
					common.GlobalMu.Unlock()
					logger.Info("Successfully reloaded input component", "id", req.ID)
				}
			case "output":
				common.GlobalMu.Lock()
				delete(project.GlobalProject.OutputsNew, req.ID)
				common.GlobalMu.Unlock()
				// Reload the output component
				configRoot := common.Config.ConfigRoot
				outputPath := path.Join(configRoot, "output", req.ID+".yaml")

				// Stop old component if it exists
				common.GlobalMu.RLock()
				oldOutput, exists := project.GlobalProject.Outputs[req.ID]
				common.GlobalMu.RUnlock()
				if exists {
					err := oldOutput.Stop()
					if err != nil {
						logger.Error("Failed to stop old output", "id", req.ID, "error", err)
					}
				}

				newOutput, reloadErr := output.NewOutput(outputPath, "", req.ID)
				if reloadErr != nil {
					logger.Error("Failed to reload output after merge", "id", req.ID, "error", reloadErr)
				} else {
					common.GlobalMu.Lock()
					project.GlobalProject.Outputs[req.ID] = newOutput
					common.GlobalMu.Unlock()
					logger.Info("Successfully reloaded output component", "id", req.ID)
				}
			case "ruleset":
				common.GlobalMu.Lock()
				delete(project.GlobalProject.RulesetsNew, req.ID)
				common.GlobalMu.Unlock()
				// Reload the ruleset component
				configRoot := common.Config.ConfigRoot
				rulesetPath := path.Join(configRoot, "ruleset", req.ID+".xml")

				// Stop old component if it exists
				common.GlobalMu.RLock()
				oldRuleset, exists := project.GlobalProject.Rulesets[req.ID]
				common.GlobalMu.RUnlock()
				if exists {
					err := oldRuleset.Stop()
					if err != nil {
						logger.Error("Failed to stop old ruleset", "id", req.ID, "error", err)
					}
				}

				newRuleset, reloadErr := rules_engine.NewRuleset(rulesetPath, "", req.ID)
				if reloadErr != nil {
					logger.Error("Failed to reload ruleset after merge", "id", req.ID, "error", reloadErr)
				} else {
					common.GlobalMu.Lock()
					project.GlobalProject.Rulesets[req.ID] = newRuleset
					common.GlobalMu.Unlock()
					logger.Info("Successfully reloaded ruleset component", "id", req.ID)
				}
			case "project":
				common.GlobalMu.Lock()
				delete(project.GlobalProject.ProjectsNew, req.ID)
				common.GlobalMu.Unlock()
				// Reload the project component
				configRoot := common.Config.ConfigRoot
				projectPath := path.Join(configRoot, "project", req.ID+".yaml")

				// Handle project lifecycle carefully
				var wasRunning bool
				common.GlobalMu.RLock()
				oldProject, exists := project.GlobalProject.Projects[req.ID]
				common.GlobalMu.RUnlock()
				if exists {
					wasRunning = (oldProject.Status == project.ProjectStatusRunning)
					if wasRunning {
						err := oldProject.Stop()
						if err != nil {
							logger.Error("Failed to stop old project", "id", req.ID, "error", err)
						}
					}
				}

				newProject, reloadErr := project.NewProject(projectPath, "", req.ID)
				if reloadErr != nil {
					logger.Error("Failed to reload project after merge", "id", req.ID, "error", reloadErr)
				} else {
					common.GlobalMu.Lock()
					project.GlobalProject.Projects[req.ID] = newProject
					common.GlobalMu.Unlock()
					logger.Info("Successfully reloaded project component", "id", req.ID)
					// Restart project if it was previously running
					if wasRunning {
						startErr := newProject.Start()
						if startErr != nil {
							logger.Error("Failed to restart project after reload", "id", req.ID, "error", startErr)
						}
					}
				}
			}
		}
	}

	if err != nil {
		logger.Error("Failed to apply change", "type", req.Type, "id", req.ID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to apply change: " + err.Error()})
	}

	// For rulesets, we can reload them directly
	if req.Type == "ruleset" {
		// Reload just this ruleset
		if ruleset, ok := project.GlobalProject.Rulesets[req.ID]; ok {
			err := ruleset.Reload()
			if err != nil {
				logger.Error("Failed to reload ruleset", "id", req.ID, "error", err)
			}
		}
	}

	// Sync changes to follower nodes
	syncComponentToFollowers(req.Type, req.ID)

	// Get affected projects and restart them
	affectedProjects := project.GetAffectedProjects(req.Type, req.ID)
	if len(affectedProjects) > 0 {
		logger.Info("Restarting affected projects", "count", len(affectedProjects))

		// First stop all affected projects
		for _, projectID := range affectedProjects {
			common.GlobalMu.RLock()
			p, exists := project.GlobalProject.Projects[projectID]
			common.GlobalMu.RUnlock()

			if !exists {
				logger.Error("Project not found for restart", "id", projectID)
				continue
			}

			if p.Status == project.ProjectStatusRunning {
				logger.Info("Stopping project for restart", "id", projectID)
				err := p.Stop()
				if err != nil {
					logger.Error("Failed to stop project", "id", projectID, "error", err)
				}
			}
		}

		// Then start all affected projects
		for _, projectID := range affectedProjects {
			common.GlobalMu.RLock()
			p, exists := project.GlobalProject.Projects[projectID]
			common.GlobalMu.RUnlock()

			if !exists {
				continue
			}

			if p.Status == project.ProjectStatusStopped {
				logger.Info("Starting project after changes", "id", projectID)
				err := p.Start()
				if err != nil {
					logger.Error("Failed to start project", "id", projectID, "error", err)
				}
			}
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"message":            "Change applied successfully",
			"restarted_projects": len(affectedProjects),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Change applied successfully"})
}

// RestartAllProjects restarts all projects
func RestartAllProjects(c echo.Context) error {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Panic in RestartAllProjects", "panic", r)
		}
	}()

	// Acquire read lock to get project list safely
	common.GlobalMu.RLock()
	projectList := make([]*project.Project, 0, len(project.GlobalProject.Projects))
	projectIDs := make([]string, 0, len(project.GlobalProject.Projects))
	for id, p := range project.GlobalProject.Projects {
		projectList = append(projectList, p)
		projectIDs = append(projectIDs, id)
	}
	common.GlobalMu.RUnlock()

	logger.Info("Starting project restart", "count", len(projectList))

	// First stop all projects
	stoppedCount := 0
	for i, p := range projectList {
		if p.Status == project.ProjectStatusRunning {
			logger.Info("Stopping project", "id", projectIDs[i], "inputs", len(p.Inputs), "rulesets", len(p.Rulesets), "outputs", len(p.Outputs))
			startTime := time.Now()

			// Log component details before stopping
			for inputID := range p.Inputs {
				logger.Info("Project has input", "project", projectIDs[i], "input", inputID)
			}
			for rulesetID := range p.Rulesets {
				logger.Info("Project has ruleset", "project", projectIDs[i], "ruleset", rulesetID)
			}
			for outputID := range p.Outputs {
				logger.Info("Project has output", "project", projectIDs[i], "output", outputID)
			}

			err := p.Stop()
			duration := time.Since(startTime)
			if err != nil {
				logger.Error("Failed to stop project", "id", projectIDs[i], "error", err, "duration", duration)
			} else {
				stoppedCount++
				logger.Info("Stopped project", "id", projectIDs[i], "duration", duration)
			}
		} else {
			logger.Info("Skipping project (not running)", "id", projectIDs[i], "status", p.Status)
		}
	}

	logger.Info("Stop phase completed", "stopped", stoppedCount)

	// Then start all projects
	startedCount := 0
	for i, p := range projectList {
		logger.Info("Starting project", "id", projectIDs[i])
		err := p.Start()
		if err != nil {
			logger.Error("Failed to start project", "id", projectIDs[i], "error", err)
		} else {
			startedCount++
			logger.Info("Started project", "id", projectIDs[i])
		}
	}

	logger.Info("Restart completed", "total", len(projectList), "started", startedCount)

	return c.JSON(http.StatusOK, map[string]string{"message": "All projects restarted"})
}

// mergeComponentFile merges a .new file with its original
func mergeComponentFile(componentType string, id string) error {
	var suffix string
	var dir string

	switch componentType {
	case "input":
		suffix = ".yaml"
		dir = "input"
	case "output":
		suffix = ".yaml"
		dir = "output"
	case "ruleset":
		suffix = ".xml"
		dir = "ruleset"
	case "project":
		suffix = ".yaml"
		dir = "project"
	default:
		return fmt.Errorf("unsupported component type: %s", componentType)
	}

	configRoot := common.Config.ConfigRoot
	originalPath := path.Join(configRoot, dir, id+suffix)
	tempPath := originalPath + ".new"

	// Read the temp file
	tempData, err := os.ReadFile(tempPath)
	if err != nil {
		return fmt.Errorf("failed to read temp file: %w", err)
	}

	// Write to the original file
	err = os.WriteFile(originalPath, tempData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write to original file: %w", err)
	}

	// Delete the temp file
	err = os.Remove(tempPath)
	if err != nil {
		logger.Warn("Failed to delete temp file after merging", "path", tempPath, "error", err)
	}

	return nil
}

// mergePluginFile merges a plugin .new file with its original
func mergePluginFile(name string) error {
	configRoot := common.Config.ConfigRoot
	originalPath := path.Join(configRoot, "plugin", name+".go")
	tempPath := originalPath + ".new"

	// Read the temp file
	tempData, err := os.ReadFile(tempPath)
	if err != nil {
		return fmt.Errorf("failed to read temp plugin file: %w", err)
	}

	// Write to the original file
	err = os.WriteFile(originalPath, tempData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write to original plugin file: %w", err)
	}

	// Delete the temp file
	err = os.Remove(tempPath)
	if err != nil {
		logger.Warn("Failed to delete temp plugin file after merging", "path", tempPath, "error", err)
	}

	return nil
}

// reloadComponents reloads components after applying changes
func reloadComponents() {
	// Reload rulesets (they support hot reload)
	for _, ruleset := range project.GlobalProject.Rulesets {
		err := ruleset.Reload()
		if err != nil {
			logger.Error("ruleset reload error", "error", err)
		}
	}

	// Clear the temporary files maps with proper locking
	common.GlobalMu.Lock()
	plugin.PluginsNew = make(map[string]string)
	project.GlobalProject.InputsNew = make(map[string]string)
	project.GlobalProject.OutputsNew = make(map[string]string)
	project.GlobalProject.RulesetsNew = make(map[string]string)
	project.GlobalProject.ProjectsNew = make(map[string]string)
	common.GlobalMu.Unlock()
}

// syncComponentToFollowers syncs a component change to follower nodes
func syncComponentToFollowers(componentType string, id string) {
	// Determine the file path based on component type
	var suffix string
	var dir string

	switch componentType {
	case "input":
		suffix = ".yaml"
		dir = "input"
	case "output":
		suffix = ".yaml"
		dir = "output"
	case "ruleset":
		suffix = ".xml"
		dir = "ruleset"
	case "project":
		suffix = ".yaml"
		dir = "project"
	case "plugin":
		suffix = ".go"
		dir = "plugin"
	default:
		logger.Error("Unsupported component type for sync", "type", componentType)
		return
	}

	configRoot := common.Config.ConfigRoot
	filePath := path.Join(configRoot, dir, id+suffix)

	// Read the file content
	data, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error("Failed to read component file for sync", "path", filePath, "error", err)
		return
	}

	// Prepare sync data
	syncData := ComponentSyncRequest{
		Type:    componentType,
		ID:      id,
		Content: string(data),
	}

	// Sync to followers
	syncComponentToFollowersWithData(syncData)
}

// syncComponentToFollowersWithData syncs a component to follower nodes
func syncComponentToFollowersWithData(syncData ComponentSyncRequest) {
	cm := cluster.ClusterInstance
	if cm == nil {
		logger.Warn("Cluster manager not initialized, skipping follower sync")
		return
	}

	// Get all follower nodes
	cm.Mu.RLock()
	followers := make([]*cluster.NodeInfo, 0)
	for _, node := range cm.Nodes {
		if node.Status == cluster.NodeStatusFollower &&
			node.IsHealthy &&
			node.Address != cm.SelfAddress {
			followers = append(followers, node)
		}
	}
	cm.Mu.RUnlock()

	if len(followers) == 0 {
		logger.Info("No healthy follower nodes found, skipping sync")
		return
	}

	// Prepare sync data
	jsonData, err := json.Marshal(syncData)
	if err != nil {
		logger.Error("Failed to marshal sync data", "error", err)
		return
	}

	// Track sync results
	syncResults := struct {
		successful []string
		failed     map[string]string // nodeID -> error message
		mu         sync.Mutex
	}{
		successful: make([]string, 0),
		failed:     make(map[string]string),
	}

	// Start a goroutine for each follower to sync
	var wg sync.WaitGroup
	for _, node := range followers {
		wg.Add(1)
		go func(node *cluster.NodeInfo) {
			defer wg.Done()

			// Build request URL
			url := fmt.Sprintf("http://%s/component-sync", node.Address)

			// Create request
			req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				syncResults.mu.Lock()
				syncResults.failed[node.ID] = fmt.Sprintf("failed to create request: %v", err)
				syncResults.mu.Unlock()
				return
			}

			// Set request headers
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("token", common.Config.Token)

			// Set timeout
			client := &http.Client{
				Timeout: 10 * time.Second,
			}

			// Send request with up to 3 retries
			var resp *http.Response
			var respErr error

			for retry := 0; retry < 3; retry++ {
				resp, respErr = client.Do(req)
				if respErr == nil && resp.StatusCode == http.StatusOK {
					_ = resp.Body.Close()
					syncResults.mu.Lock()
					syncResults.successful = append(syncResults.successful, node.ID)
					syncResults.mu.Unlock()
					return
				}

				if resp != nil {
					_ = resp.Body.Close()
				}

				// If it's not a timeout error, don't retry
				if respErr != nil && !strings.Contains(respErr.Error(), "timeout") &&
					!strings.Contains(respErr.Error(), "connection refused") {
					break
				}

				// Wait before retry
				time.Sleep(time.Duration(retry+1) * time.Second)
			}

			// All retries failed
			errorMsg := "unknown error"
			if respErr != nil {
				errorMsg = respErr.Error()
			} else if resp != nil {
				errorMsg = fmt.Sprintf("status code: %d", resp.StatusCode)
			}

			syncResults.mu.Lock()
			syncResults.failed[node.ID] = errorMsg
			syncResults.mu.Unlock()
		}(node)
	}

	// Wait for all sync operations to complete
	wg.Wait()

	// Log sync results
	logger.Info("Component sync completed",
		"type", syncData.Type,
		"id", syncData.ID,
		"successful_nodes", len(syncResults.successful),
		"failed_nodes", len(syncResults.failed),
	)

	// If there are failed nodes, log detailed information
	if len(syncResults.failed) > 0 {
		for nodeID, errMsg := range syncResults.failed {
			logger.Error("Failed to sync component to follower",
				"node_id", nodeID,
				"type", syncData.Type,
				"id", syncData.ID,
				"error", errMsg,
			)
		}
	}
}

// CreateTempFile creates a temporary file for editing
func CreateTempFile(c echo.Context) error {
	componentType := c.Param("type")
	id := c.Param("id")

	var originalPath string
	var tempPath string
	var content string
	var err error

	configRoot := common.Config.ConfigRoot

	// Log request details for debugging
	logger.Info("CreateTempFile request received",
		"type", componentType,
		"id", id,
		"configRoot", configRoot)

	// Handle both singular and plural forms of component types
	// Strip trailing 's' if present to normalize component type
	singularType := strings.TrimSuffix(componentType, "s")

	// Lock for reading component data
	common.GlobalMu.RLock()

	switch singularType {
	case "input":
		originalPath = path.Join(configRoot, "input", id+".yaml")
		tempPath = originalPath + ".new"

		if i, ok := project.GlobalProject.Inputs[id]; ok {
			content = i.Config.RawConfig
		} else {
			common.GlobalMu.RUnlock()
			logger.Error("Input not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "input not found"})
		}

	case "output":
		originalPath = path.Join(configRoot, "output", id+".yaml")
		tempPath = originalPath + ".new"

		if o, ok := project.GlobalProject.Outputs[id]; ok {
			content = o.Config.RawConfig
		} else {
			common.GlobalMu.RUnlock()
			logger.Error("Output not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "output not found"})
		}

	case "ruleset":
		originalPath = path.Join(configRoot, "ruleset", id+".xml")
		tempPath = originalPath + ".new"

		if ruleset, ok := project.GlobalProject.Rulesets[id]; ok {
			content = ruleset.RawConfig
		} else {
			common.GlobalMu.RUnlock()
			logger.Error("Ruleset not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
		}

	case "project":
		originalPath = path.Join(configRoot, "project", id+".yaml")
		tempPath = originalPath + ".new"

		if proj, ok := project.GlobalProject.Projects[id]; ok {
			content = proj.Config.RawConfig
		} else {
			common.GlobalMu.RUnlock()
			logger.Error("Project not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
		}

	case "plugin":
		originalPath = path.Join(configRoot, "plugin", id+".go")
		tempPath = originalPath + ".new"

		if p, ok := plugin.Plugins[id]; ok {
			content = string(p.Payload)
		} else {
			common.GlobalMu.RUnlock()
			logger.Error("Plugin not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "plugin not found"})
		}

	default:
		common.GlobalMu.RUnlock()
		logger.Error("Unsupported component type", "type", componentType)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported component type"})
	}

	common.GlobalMu.RUnlock()

	// Check if temp file already exists
	if _, err := os.Stat(tempPath); err == nil {
		// Temp file already exists, no need to create it again
		logger.Info("Temp file already exists", "path", tempPath)
		return c.JSON(http.StatusOK, map[string]string{"message": "temp file already exists"})
	}

	// Read original file content to compare
	originalContent, err := os.ReadFile(originalPath)
	if err != nil {
		logger.Error("Failed to read original file", "path", originalPath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read original file: " + err.Error()})
	}

	// Compare content with original file
	memoryContent := strings.TrimSpace(content)
	fileContent := strings.TrimSpace(string(originalContent))

	logger.Info("Content comparison",
		"memory_content", memoryContent,
		"file_content", fileContent,
		"memory_len", len(memoryContent),
		"file_len", len(fileContent),
		"equal", memoryContent == fileContent)

	if memoryContent == fileContent {
		logger.Info("Content is identical to original file, not creating temp file", "path", tempPath)
		return c.JSON(http.StatusOK, map[string]string{"message": "content identical to original file, no temp file needed"})
	}

	// Write content to temp file
	err = os.WriteFile(tempPath, []byte(content), 0644)
	if err != nil {
		logger.Error("Failed to create temp file", "path", tempPath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create temp file: " + err.Error()})
	}

	// Store the temp file content in memory with lock protection
	common.GlobalMu.Lock()
	switch singularType {
	case "input":
		project.GlobalProject.InputsNew[id] = content
	case "output":
		project.GlobalProject.OutputsNew[id] = content
	case "ruleset":
		project.GlobalProject.RulesetsNew[id] = content
	case "project":
		project.GlobalProject.ProjectsNew[id] = content
	case "plugin":
		plugin.PluginsNew[id] = content
	}
	common.GlobalMu.Unlock()

	logger.Info("Temp file created successfully", "path", tempPath)
	return c.JSON(http.StatusOK, map[string]string{"message": "temp file created successfully"})
}

// CheckTempFile checks if component has temporary file
func CheckTempFile(c echo.Context) error {
	componentType := c.Param("type")
	id := c.Param("id")

	// Normalize component type
	singularType := strings.TrimSuffix(componentType, "s")

	// Get temporary file path
	tempPath, tempExists := GetComponentPath(singularType, id, true)

	// Check if temporary file exists
	if !tempExists {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"has_temp": false,
		})
	}

	// Read temporary file content
	content, err := ReadComponent(tempPath)
	if err != nil {
		logger.Error("Failed to read temp file", "path", tempPath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to read temp file: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"has_temp": true,
		"content":  content,
		"path":     tempPath,
	})
}

// DeleteTempFile deletes component's temporary file
func DeleteTempFile(c echo.Context) error {
	componentType := c.Param("type")
	id := c.Param("id")

	// Normalize component type
	singularType := strings.TrimSuffix(componentType, "s")

	// Get temporary file path
	tempPath, tempExists := GetComponentPath(singularType, id, true)

	// Check if temporary file exists
	if !tempExists {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Temp file not found",
		})
	}

	// Delete temporary file
	err := os.Remove(tempPath)
	if err != nil {
		logger.Error("Failed to delete temp file", "path", tempPath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to delete temp file: " + err.Error(),
		})
	}

	// Remove temporary file content from memory with lock protection
	common.GlobalMu.Lock()
	switch singularType {
	case "input":
		delete(project.GlobalProject.InputsNew, id)
	case "output":
		delete(project.GlobalProject.OutputsNew, id)
	case "ruleset":
		delete(project.GlobalProject.RulesetsNew, id)
	case "project":
		delete(project.GlobalProject.ProjectsNew, id)
	case "plugin":
		delete(plugin.PluginsNew, id)
	}
	common.GlobalMu.Unlock()

	logger.Info("Temp file deleted successfully", "path", tempPath)
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Temp file deleted successfully",
	})
}
