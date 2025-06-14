package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/output"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

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
	p := project.GlobalProject

	// Check plugins with pending changes
	for name, newContent := range plugin.PluginsNew {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing plugin2
		if plugin2, ok := plugin.Plugins[name]; ok {
			oldContent = string(plugin2.Payload)
			isNew = false
		}

		changes = append(changes, PendingChange{
			Type:       "plugin2",
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

		// Check if this is a modification to an existing i
		if i, ok := p.Inputs[id]; ok {
			oldContent = i.Config.RawConfig
			isNew = false
		}

		changes = append(changes, PendingChange{
			Type:       "i",
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

		// Check if this is a modification to an existing o
		if o, ok := p.Outputs[id]; ok {
			oldContent = o.Config.RawConfig
			isNew = false
		}

		changes = append(changes, PendingChange{
			Type:       "o",
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

		// Check if this is a modification to an existing project2
		if project2, ok := p.Projects[id]; ok {
			oldContent = project2.Config.RawConfig
			isNew = false
		}

		changes = append(changes, PendingChange{
			Type:       "project2",
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

	// Apply plugin changes
	for name, content := range plugin.PluginsNew {
		// 验证插件配置
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
			successCount++
		}
	}

	// Apply input changes
	for id, content := range project.GlobalProject.InputsNew {
		// 验证输入配置
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
			successCount++
		}
	}

	// Apply output changes
	for id, content := range project.GlobalProject.OutputsNew {
		// 验证输出配置
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
			successCount++
		}
	}

	// Apply ruleset changes
	for id, content := range project.GlobalProject.RulesetsNew {
		// 验证规则集配置
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
			successCount++
		}
	}

	// Apply project changes
	for id, content := range project.GlobalProject.ProjectsNew {
		// 验证项目配置
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
			successCount++
		}
	}

	// Reload components after applying changes
	reloadComponents()

	if len(verifyFailures) > 0 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":           "Some changes failed verification and were not applied",
			"verify_failures": verifyFailures,
			"success_count":   successCount,
			"failure_count":   failureCount,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success_count": successCount,
		"failure_count": failureCount,
	})
}

// ApplySingleChange applies a single pending change
func ApplySingleChange(c echo.Context) error {
	var req SingleChangeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	// 首先验证配置
	var verifyErr error

	switch req.Type {
	case "plugin":
		if content, ok := plugin.PluginsNew[req.ID]; ok {
			verifyErr = plugin.Verify("", content, req.ID)
		} else {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "No pending changes found for this plugin"})
		}
	case "input":
		if content, ok := project.GlobalProject.InputsNew[req.ID]; ok {
			verifyErr = input.Verify("", content)
		} else {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "No pending changes found for this input"})
		}
	case "output":
		if content, ok := project.GlobalProject.OutputsNew[req.ID]; ok {
			verifyErr = output.Verify("", content)
		} else {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "No pending changes found for this output"})
		}
	case "ruleset":
		if content, ok := project.GlobalProject.RulesetsNew[req.ID]; ok {
			verifyErr = rules_engine.Verify("", content)
		} else {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "No pending changes found for this ruleset"})
		}
	case "project":
		if content, ok := project.GlobalProject.ProjectsNew[req.ID]; ok {
			verifyErr = project.Verify("", content)
		} else {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "No pending changes found for this project"})
		}
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid component type"})
	}

	// 如果验证失败，返回错误
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
	case "input", "output", "ruleset", "project":
		err = mergeComponentFile(req.Type, req.ID)
	}

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to apply change: " + err.Error()})
	}

	// For rulesets, we can reload them directly
	if req.Type == "ruleset" {
		// Reload just this ruleset
		if ruleset, ok := project.GlobalProject.Rulesets[req.ID]; ok {
			err := ruleset.Reload()
			if err != nil {
				logger.Error("Rule reload failed", "id", req.ID, "error", err)
			}
		}
	}

	// Sync changes to follower nodes
	syncComponentToFollowers(req.Type, req.ID)

	return c.JSON(http.StatusOK, map[string]string{"message": "Change applied successfully"})
}

// RestartAllProjects restarts all projects
func RestartAllProjects(c echo.Context) error {
	// First stop all projects
	for id, p := range project.GlobalProject.Projects {
		if p.Status == project.ProjectStatusRunning {
			err := p.Stop()
			if err != nil {
				logger.Error("Stopping project failed", "id", id, "error", err)
			}
			logger.Info("Stopped project", "id", id)
		}
	}

	// Then start all projects
	for id, p := range project.GlobalProject.Projects {
		err := p.Start()
		if err != nil {
			logger.Error("Failed to start project", "id", id, "error", err)
		} else {
			logger.Info("Started project", "id", id)
		}
	}

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
			logger.Error("Reload ruleset error: ", err)
		}
	}

	// Clear the temporary files maps
	plugin.PluginsNew = make(map[string]string)
	project.GlobalProject.InputsNew = make(map[string]string)
	project.GlobalProject.OutputsNew = make(map[string]string)
	project.GlobalProject.RulesetsNew = make(map[string]string)
	project.GlobalProject.ProjectsNew = make(map[string]string)
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
	// Since follower node synchronization is not implemented yet,
	// we'll just log that syncing would happen here
	logger.Info("Component sync requested (not implemented yet)",
		"type", syncData.Type,
		"id", syncData.ID)
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

	switch singularType {
	case "input":
		originalPath = path.Join(configRoot, "input", id+".yaml")
		tempPath = originalPath + ".new"

		if i, ok := project.GlobalProject.Inputs[id]; ok {
			content = i.Config.RawConfig
		} else {
			logger.Error("Input not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "input not found"})
		}

	case "output":
		originalPath = path.Join(configRoot, "output", id+".yaml")
		tempPath = originalPath + ".new"

		if o, ok := project.GlobalProject.Outputs[id]; ok {
			content = o.Config.RawConfig
		} else {
			logger.Error("Output not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "output not found"})
		}

	case "ruleset":
		originalPath = path.Join(configRoot, "ruleset", id+".xml")
		tempPath = originalPath + ".new"

		if ruleset, ok := project.GlobalProject.Rulesets[id]; ok {
			content = ruleset.RawConfig
		} else {
			logger.Error("Ruleset not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
		}

	case "project":
		originalPath = path.Join(configRoot, "project", id+".yaml")
		tempPath = originalPath + ".new"

		if proj, ok := project.GlobalProject.Projects[id]; ok {
			content = proj.Config.RawConfig
		} else {
			logger.Error("Project not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
		}

	case "plugin":
		originalPath = path.Join(configRoot, "plugin", id+".go")
		tempPath = originalPath + ".new"

		if p, ok := plugin.Plugins[id]; ok {
			content = string(p.Payload)
		} else {
			logger.Error("Plugin not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "plugin not found"})
		}

	default:
		logger.Error("Unsupported component type", "type", componentType)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported component type"})
	}

	// Check if temp file already exists
	if _, err := os.Stat(tempPath); err == nil {
		// Temp file already exists, no need to create it again
		logger.Info("Temp file already exists", "path", tempPath)
		return c.JSON(http.StatusOK, map[string]string{"message": "temp file already exists"})
	}

	// Write content to temp file
	err = os.WriteFile(tempPath, []byte(content), 0644)
	if err != nil {
		logger.Error("Failed to create temp file", "path", tempPath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create temp file: " + err.Error()})
	}

	// Store the temp file content in memory
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

	logger.Info("Temp file created successfully", "path", tempPath)
	return c.JSON(http.StatusOK, map[string]string{"message": "temp file created successfully"})
}
