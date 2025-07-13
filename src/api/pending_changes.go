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
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// Enhanced pending change management structures
type ChangeStatus int

const (
	ChangeStatusDraft ChangeStatus = iota
	ChangeStatusVerified
	ChangeStatusInvalid
	ChangeStatusApplied
	ChangeStatusFailed
)

func (cs ChangeStatus) String() string {
	switch cs {
	case ChangeStatusDraft:
		return "draft"
	case ChangeStatusVerified:
		return "verified"
	case ChangeStatusInvalid:
		return "invalid"
	case ChangeStatusApplied:
		return "applied"
	case ChangeStatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Enhanced change tracking
type EnhancedPendingChange struct {
	Type         string       `json:"type"`
	ID           string       `json:"id"`
	IsNew        bool         `json:"is_new"`
	OldContent   string       `json:"old_content"`
	NewContent   string       `json:"new_content"`
	Status       ChangeStatus `json:"status"`
	ErrorMessage string       `json:"error_message,omitempty"`
	LastUpdated  time.Time    `json:"last_updated"`
	VerifiedAt   *time.Time   `json:"verified_at,omitempty"`
}

// Transaction result for batch operations
type ChangeTransactionResult struct {
	TotalChanges      int                `json:"total_changes"`
	SuccessCount      int                `json:"success_count"`
	FailureCount      int                `json:"failure_count"`
	SuccessfulIDs     []string           `json:"successful_ids"`
	FailedChanges     []FailedChangeInfo `json:"failed_changes"`
	ProjectsToRestart []string           `json:"projects_to_restart"`
}

type FailedChangeInfo struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Error string `json:"error"`
}

// PendingChangeManager provides centralized management of pending changes
type PendingChangeManager struct {
	changes map[string]*EnhancedPendingChange // key: type:id
	mu      sync.RWMutex
}

var globalPendingChangeManager = &PendingChangeManager{
	changes: make(map[string]*EnhancedPendingChange),
}

func (pcm *PendingChangeManager) getKey(changeType, id string) string {
	return fmt.Sprintf("%s:%s", changeType, id)
}

// AddChange adds or updates a pending change
func (pcm *PendingChangeManager) AddChange(changeType, id, newContent, oldContent string, isNew bool) {
	// Input validation
	if changeType == "" || id == "" {
		logger.Error("Invalid change parameters", "type", changeType, "id", id)
		return
	}

	// Validate component type
	validTypes := map[string]bool{
		"plugin": true, "input": true, "output": true, "ruleset": true, "project": true,
	}
	if !validTypes[changeType] {
		logger.Error("Invalid component type", "type", changeType)
		return
	}

	pcm.mu.Lock()
	defer pcm.mu.Unlock()

	key := pcm.getKey(changeType, id)
	change := &EnhancedPendingChange{
		Type:        changeType,
		ID:          id,
		IsNew:       isNew,
		OldContent:  oldContent,
		NewContent:  newContent,
		Status:      ChangeStatusDraft,
		LastUpdated: time.Now(),
	}
	pcm.changes[key] = change
}

// GetChange retrieves a specific pending change
func (pcm *PendingChangeManager) GetChange(changeType, id string) (*EnhancedPendingChange, bool) {
	pcm.mu.RLock()
	defer pcm.mu.RUnlock()

	key := pcm.getKey(changeType, id)
	change, exists := pcm.changes[key]
	return change, exists
}

// GetAllChanges returns all pending changes
func (pcm *PendingChangeManager) GetAllChanges() []*EnhancedPendingChange {
	pcm.mu.RLock()
	defer pcm.mu.RUnlock()

	changes := make([]*EnhancedPendingChange, 0, len(pcm.changes))
	for _, change := range pcm.changes {
		changes = append(changes, change)
	}
	return changes
}

// RemoveChange removes a pending change
func (pcm *PendingChangeManager) RemoveChange(changeType, id string) {
	pcm.mu.Lock()
	defer pcm.mu.Unlock()

	key := pcm.getKey(changeType, id)
	delete(pcm.changes, key)
}

// UpdateChangeStatus updates the status of a pending change
func (pcm *PendingChangeManager) UpdateChangeStatus(changeType, id string, status ChangeStatus, errorMsg string) {
	pcm.mu.Lock()
	defer pcm.mu.Unlock()

	key := pcm.getKey(changeType, id)
	if change, exists := pcm.changes[key]; exists {
		change.Status = status
		change.ErrorMessage = errorMsg
		change.LastUpdated = time.Now()

		if status == ChangeStatusVerified {
			now := time.Now()
			change.VerifiedAt = &now
		}
	}
}

// VerifyChange verifies a single pending change
func (pcm *PendingChangeManager) VerifyChange(changeType, id string) error {
	change, exists := pcm.GetChange(changeType, id)
	if !exists {
		return fmt.Errorf("change not found: %s:%s", changeType, id)
	}

	var err error
	switch changeType {
	case "plugin":
		err = plugin.Verify("", change.NewContent, id)
	case "input":
		err = input.Verify("", change.NewContent)
	case "output":
		err = output.Verify("", change.NewContent)
	case "ruleset":
		err = rules_engine.Verify("", change.NewContent)
	case "project":
		err = project.Verify("", change.NewContent)
	default:
		err = fmt.Errorf("unsupported component type: %s", changeType)
	}

	if err != nil {
		pcm.UpdateChangeStatus(changeType, id, ChangeStatusInvalid, err.Error())
		return err
	}

	pcm.UpdateChangeStatus(changeType, id, ChangeStatusVerified, "")
	return nil
}

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

// GetPendingChanges returns all components with pending changes (.new files)
// GetPendingChanges returns all pending changes (legacy format for backward compatibility)
func GetPendingChanges(c echo.Context) error {
	// First, sync from legacy storage to new manager
	syncLegacyToEnhancedManager()

	// Get enhanced changes
	enhancedChanges := globalPendingChangeManager.GetAllChanges()

	// Convert to legacy format for backward compatibility
	changes := make([]PendingChange, 0, len(enhancedChanges))
	for _, enhanced := range enhancedChanges {
		changes = append(changes, PendingChange{
			Type:       enhanced.Type,
			ID:         enhanced.ID,
			IsNew:      enhanced.IsNew,
			OldContent: enhanced.OldContent,
			NewContent: enhanced.NewContent,
		})
	}

	return c.JSON(http.StatusOK, changes)
}

// GetEnhancedPendingChanges returns all pending changes with enhanced status information
func GetEnhancedPendingChanges(c echo.Context) error {
	// Sync from legacy storage to new manager
	syncLegacyToEnhancedManager()

	changes := globalPendingChangeManager.GetAllChanges()
	return c.JSON(http.StatusOK, changes)
}

// syncLegacyToEnhancedManager synchronizes data from legacy storage to the enhanced manager
func syncLegacyToEnhancedManager() {
	// Lock for reading all pending changes and existing components
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	p := project.GlobalProject

	// First, get all currently managed changes to detect if we need to clean up
	existingChanges := globalPendingChangeManager.GetAllChanges()

	// Create a map of what should exist based on current legacy storage
	shouldExist := make(map[string]bool)

	// Sync plugins with pending changes
	for name, newContent := range plugin.PluginsNew {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing plugin
		if plugin, ok := plugin.Plugins[name]; ok {
			oldContent = string(plugin.Payload)
			isNew = false
		}

		key := fmt.Sprintf("plugin:%s", name)
		shouldExist[key] = true

		// Always update or add to ensure current state
		if existingChange, exists := globalPendingChangeManager.GetChange("plugin", name); exists {
			// Update existing change with current content
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("plugin", name, newContent, oldContent, isNew)
			}
		} else {
			// Add new change
			globalPendingChangeManager.AddChange("plugin", name, newContent, oldContent, isNew)
		}
	}

	// Sync inputs with pending changes
	for id, newContent := range p.InputsNew {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing input
		if i, ok := p.Inputs[id]; ok {
			oldContent = i.Config.RawConfig
			isNew = false
		}

		key := fmt.Sprintf("input:%s", id)
		shouldExist[key] = true

		// Always update or add to ensure current state
		if existingChange, exists := globalPendingChangeManager.GetChange("input", id); exists {
			// Update existing change with current content
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("input", id, newContent, oldContent, isNew)
			}
		} else {
			// Add new change
			globalPendingChangeManager.AddChange("input", id, newContent, oldContent, isNew)
		}
	}

	// Sync outputs with pending changes
	for id, newContent := range p.OutputsNew {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing output
		if o, ok := p.Outputs[id]; ok {
			oldContent = o.Config.RawConfig
			isNew = false
		}

		key := fmt.Sprintf("output:%s", id)
		shouldExist[key] = true

		// Always update or add to ensure current state
		if existingChange, exists := globalPendingChangeManager.GetChange("output", id); exists {
			// Update existing change with current content
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("output", id, newContent, oldContent, isNew)
			}
		} else {
			// Add new change
			globalPendingChangeManager.AddChange("output", id, newContent, oldContent, isNew)
		}
	}

	// Sync rulesets with pending changes
	for id, newContent := range p.RulesetsNew {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing ruleset
		if ruleset, ok := p.Rulesets[id]; ok {
			oldContent = ruleset.RawConfig
			isNew = false
		}

		key := fmt.Sprintf("ruleset:%s", id)
		shouldExist[key] = true

		// Always update or add to ensure current state
		if existingChange, exists := globalPendingChangeManager.GetChange("ruleset", id); exists {
			// Update existing change with current content
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("ruleset", id, newContent, oldContent, isNew)
			}
		} else {
			// Add new change
			globalPendingChangeManager.AddChange("ruleset", id, newContent, oldContent, isNew)
		}
	}

	// Sync projects with pending changes
	for id, newContent := range p.ProjectsNew {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing project
		if proj, ok := p.Projects[id]; ok {
			oldContent = proj.Config.RawConfig
			isNew = false
		}

		key := fmt.Sprintf("project:%s", id)
		shouldExist[key] = true

		// Always update or add to ensure current state
		if existingChange, exists := globalPendingChangeManager.GetChange("project", id); exists {
			// Update existing change with current content
			if existingChange.NewContent != newContent || existingChange.OldContent != oldContent {
				globalPendingChangeManager.AddChange("project", id, newContent, oldContent, isNew)
			}
		} else {
			// Add new change
			globalPendingChangeManager.AddChange("project", id, newContent, oldContent, isNew)
		}
	}

	// Clean up obsolete changes that no longer exist in legacy storage
	for _, change := range existingChanges {
		key := fmt.Sprintf("%s:%s", change.Type, change.ID)
		if !shouldExist[key] {
			// This change no longer exists in legacy storage, remove it from Enhanced Manager
			globalPendingChangeManager.RemoveChange(change.Type, change.ID)
			logger.Info("Removed obsolete pending change from Enhanced Manager",
				"type", change.Type,
				"id", change.ID)
		}
	}
}

// ApplyPendingChangesEnhanced applies all pending changes with improved transaction handling
func ApplyPendingChangesEnhanced(c echo.Context) error {
	// Add panic recovery
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Panic in ApplyPendingChangesEnhanced", "panic", r)
			c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Internal server error during change application",
			})
		}
	}()

	// Sync from legacy storage first
	syncLegacyToEnhancedManager()

	// Get all pending changes
	changes := globalPendingChangeManager.GetAllChanges()
	if len(changes) == 0 {
		return c.JSON(http.StatusOK, ChangeTransactionResult{
			TotalChanges: 0,
			SuccessCount: 0,
			FailureCount: 0,
		})
	}

	// Check for nil changes
	filteredChanges := make([]*EnhancedPendingChange, 0, len(changes))
	for _, change := range changes {
		if change != nil && change.Type != "" && change.ID != "" {
			filteredChanges = append(filteredChanges, change)
		} else {
			logger.Warn("Skipping invalid change", "change", change)
		}
	}
	changes = filteredChanges

	if len(changes) == 0 {
		return c.JSON(http.StatusOK, ChangeTransactionResult{
			TotalChanges: 0,
			SuccessCount: 0,
			FailureCount: 0,
		})
	}

	// Phase 1: Verify all changes first
	logger.Info("Starting enhanced apply process", "total_changes", len(changes))

	verificationErrors := make([]FailedChangeInfo, 0)
	validChanges := make([]*EnhancedPendingChange, 0)

	for _, change := range changes {
		if change.Status == ChangeStatusVerified {
			validChanges = append(validChanges, change)
			continue
		}

		err := globalPendingChangeManager.VerifyChange(change.Type, change.ID)
		if err != nil {
			verificationErrors = append(verificationErrors, FailedChangeInfo{
				Type:  change.Type,
				ID:    change.ID,
				Error: fmt.Sprintf("Verification failed: %v", err),
			})
			logger.Error("Change verification failed", "type", change.Type, "id", change.ID, "error", err)
		} else {
			validChanges = append(validChanges, change)
		}
	}

	// If any verification failed, return early
	if len(verificationErrors) > 0 {
		return c.JSON(http.StatusBadRequest, ChangeTransactionResult{
			TotalChanges:  len(changes),
			SuccessCount:  0,
			FailureCount:  len(verificationErrors),
			FailedChanges: verificationErrors,
		})
	}

	// Phase 2: Apply changes with transaction-like behavior
	result := ChangeTransactionResult{
		TotalChanges:      len(validChanges),
		SuccessfulIDs:     make([]string, 0),
		FailedChanges:     make([]FailedChangeInfo, 0),
		ProjectsToRestart: make([]string, 0),
	}

	projectsToRestart := make(map[string]struct{})

	// Apply changes by type to maintain dependencies
	changesByType := make(map[string][]*EnhancedPendingChange)
	for _, change := range validChanges {
		changesByType[change.Type] = append(changesByType[change.Type], change)
	}

	// Apply in dependency order: plugins -> inputs/outputs -> rulesets -> projects
	applyOrder := []string{"plugin", "input", "output", "ruleset", "project"}

	for _, changeType := range applyOrder {
		changes := changesByType[changeType]
		for _, change := range changes {
			err := applyEnhancedSingleChange(change, projectsToRestart)
			if err != nil {
				result.FailedChanges = append(result.FailedChanges, FailedChangeInfo{
					Type:  change.Type,
					ID:    change.ID,
					Error: err.Error(),
				})
				result.FailureCount++
				globalPendingChangeManager.UpdateChangeStatus(change.Type, change.ID, ChangeStatusFailed, err.Error())
			} else {
				result.SuccessfulIDs = append(result.SuccessfulIDs, fmt.Sprintf("%s:%s", change.Type, change.ID))
				result.SuccessCount++
				globalPendingChangeManager.UpdateChangeStatus(change.Type, change.ID, ChangeStatusApplied, "")
				// Remove from enhanced manager after successful application
				globalPendingChangeManager.RemoveChange(change.Type, change.ID)
			}
		}
	}

	// Convert projects to restart to slice
	for projectID := range projectsToRestart {
		result.ProjectsToRestart = append(result.ProjectsToRestart, projectID)
	}

	// Actually restart the affected projects
	if len(result.ProjectsToRestart) > 0 {
		logger.Info("Restarting affected projects", "count", len(result.ProjectsToRestart))

		// Use unified restart function for better maintainability
		restartedCount, err := project.RestartProjectsSafely(result.ProjectsToRestart, "component_change")
		if err != nil {
			logger.Error("Error during batch project restart", "error", err)
		} else {
			logger.Info("Successfully restarted projects", "count", restartedCount)
		}

		// Publish project restart instructions to followers
		if err := cluster.GlobalInstructionManager.PublishProjectsRestart(result.ProjectsToRestart, "component_change"); err != nil {
			logger.Error("Failed to publish project restart instructions", "affected_projects", result.ProjectsToRestart, "error", err)
		}
	}

	logger.Info("Enhanced apply process completed",
		"total", result.TotalChanges,
		"success", result.SuccessCount,
		"failed", result.FailureCount,
		"projects_to_restart", len(result.ProjectsToRestart))

	return c.JSON(http.StatusOK, result)
}

// VerifyPendingChanges verifies all pending changes without applying them
func VerifyPendingChanges(c echo.Context) error {
	// Sync from legacy storage first
	syncLegacyToEnhancedManager()

	changes := globalPendingChangeManager.GetAllChanges()
	if len(changes) == 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"total_changes":   0,
			"valid_changes":   0,
			"invalid_changes": 0,
			"results":         []map[string]interface{}{},
		})
	}

	results := make([]map[string]interface{}, 0, len(changes))
	validCount := 0
	invalidCount := 0

	for _, change := range changes {
		result := map[string]interface{}{
			"type":   change.Type,
			"id":     change.ID,
			"is_new": change.IsNew,
			"valid":  false,
			"error":  "",
		}

		err := globalPendingChangeManager.VerifyChange(change.Type, change.ID)
		if err != nil {
			result["error"] = err.Error()
			invalidCount++
		} else {
			result["valid"] = true
			validCount++
		}

		results = append(results, result)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"total_changes":   len(changes),
		"valid_changes":   validCount,
		"invalid_changes": invalidCount,
		"results":         results,
	})
}

// VerifySinglePendingChange verifies a single pending change
func VerifySinglePendingChange(c echo.Context) error {
	changeType := c.Param("type")
	id := c.Param("id")

	// Sync from legacy storage first
	syncLegacyToEnhancedManager()

	change, exists := globalPendingChangeManager.GetChange(changeType, id)
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Pending change not found",
		})
	}

	err := globalPendingChangeManager.VerifyChange(changeType, id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"valid":  true,
		"change": change,
	})
}

// CancelPendingChange cancels a single pending change and removes associated files
func CancelPendingChange(c echo.Context) error {
	changeType := c.Param("type")
	id := c.Param("id")

	// Input validation
	if changeType == "" || id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Missing component type or ID",
		})
	}

	// Validate component type
	validTypes := map[string]bool{
		"plugin": true, "input": true, "output": true, "ruleset": true, "project": true,
	}
	if !validTypes[changeType] {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid component type: " + changeType,
		})
	}

	// Sync from legacy storage first
	syncLegacyToEnhancedManager()

	change, exists := globalPendingChangeManager.GetChange(changeType, id)
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Pending change not found",
		})
	}

	// Remove from enhanced manager
	globalPendingChangeManager.RemoveChange(changeType, id)

	// Remove from legacy storage
	common.GlobalMu.Lock()
	switch changeType {
	case "plugin":
		delete(plugin.PluginsNew, id)
	case "input":
		delete(project.GlobalProject.InputsNew, id)
	case "output":
		delete(project.GlobalProject.OutputsNew, id)
	case "ruleset":
		delete(project.GlobalProject.RulesetsNew, id)
	case "project":
		delete(project.GlobalProject.ProjectsNew, id)
	}
	common.GlobalMu.Unlock()

	// Remove .new file if it exists
	configRoot := common.Config.ConfigRoot
	var tempPath string
	switch changeType {
	case "plugin":
		tempPath = path.Join(configRoot, "plugin", id+".go.new")
	case "input":
		tempPath = path.Join(configRoot, "input", id+".yaml.new")
	case "output":
		tempPath = path.Join(configRoot, "output", id+".yaml.new")
	case "ruleset":
		tempPath = path.Join(configRoot, "ruleset", id+".xml.new")
	case "project":
		tempPath = path.Join(configRoot, "project", id+".yaml.new")
	}

	if tempPath != "" {
		if _, err := os.Stat(tempPath); err == nil {
			err = os.Remove(tempPath)
			if err != nil {
				logger.Warn("Failed to remove temp file", "path", tempPath, "error", err)
			} else {
				logger.Info("Temp file removed", "path", tempPath)
			}
		}
	}

	logger.Info("Pending change cancelled", "type", changeType, "id", id)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Pending change cancelled successfully",
		"change":  change,
	})
}

// CancelAllPendingChanges cancels all pending changes
func CancelAllPendingChanges(c echo.Context) error {
	// Sync from legacy storage first
	syncLegacyToEnhancedManager()

	changes := globalPendingChangeManager.GetAllChanges()
	cancelledCount := 0

	for _, change := range changes {
		// Remove from enhanced manager
		globalPendingChangeManager.RemoveChange(change.Type, change.ID)

		// Remove from legacy storage
		common.GlobalMu.Lock()
		switch change.Type {
		case "plugin":
			delete(plugin.PluginsNew, change.ID)
		case "input":
			delete(project.GlobalProject.InputsNew, change.ID)
		case "output":
			delete(project.GlobalProject.OutputsNew, change.ID)
		case "ruleset":
			delete(project.GlobalProject.RulesetsNew, change.ID)
		case "project":
			delete(project.GlobalProject.ProjectsNew, change.ID)
		}
		common.GlobalMu.Unlock()

		// Remove .new file if it exists
		configRoot := common.Config.ConfigRoot
		var tempPath string
		switch change.Type {
		case "plugin":
			tempPath = path.Join(configRoot, "plugin", change.ID+".go.new")
		case "input":
			tempPath = path.Join(configRoot, "input", change.ID+".yaml.new")
		case "output":
			tempPath = path.Join(configRoot, "output", change.ID+".yaml.new")
		case "ruleset":
			tempPath = path.Join(configRoot, "ruleset", change.ID+".xml.new")
		case "project":
			tempPath = path.Join(configRoot, "project", change.ID+".yaml.new")
		}

		if tempPath != "" {
			if _, err := os.Stat(tempPath); err == nil {
				err = os.Remove(tempPath)
				if err != nil {
					logger.Warn("Failed to remove temp file", "path", tempPath, "error", err)
				}
			}
		}

		cancelledCount++
	}

	logger.Info("All pending changes cancelled", "count", cancelledCount)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":         "All pending changes cancelled successfully",
		"cancelled_count": cancelledCount,
	})
}

// applyEnhancedSingleChange applies a single enhanced pending change
func applyEnhancedSingleChange(change *EnhancedPendingChange, projectsToRestart map[string]struct{}) error {
	switch change.Type {
	case "plugin":
		affectedProjects, err := applyPluginChange(change)
		if err != nil {
			return err
		}
		for _, projectID := range affectedProjects {
			projectsToRestart[projectID] = struct{}{}
		}
		return nil
	case "input":
		affectedProjects, err := applyInputChange(change)
		if err != nil {
			return err
		}
		for _, projectID := range affectedProjects {
			projectsToRestart[projectID] = struct{}{}
		}
		return nil
	case "output":
		affectedProjects, err := applyOutputChange(change)
		if err != nil {
			return err
		}
		for _, projectID := range affectedProjects {
			projectsToRestart[projectID] = struct{}{}
		}
		return nil
	case "ruleset":
		affectedProjects, err := applyRulesetChange(change)
		if err != nil {
			return err
		}
		for _, projectID := range affectedProjects {
			projectsToRestart[projectID] = struct{}{}
		}
		return nil
	case "project":
		return applyProjectChange(change)
	default:
		return fmt.Errorf("unsupported component type: %s", change.Type)
	}
}

// Component-specific apply functions
func applyPluginChange(change *EnhancedPendingChange) ([]string, error) {
	pluginID := change.ID
	newContent := change.NewContent

	logger.Info("Applying plugin change", "plugin", pluginID)

	// Get affected projects for restart
	affectedProjects := project.GetAffectedProjects("plugin", pluginID)

	// Update plugin configuration file
	configRoot := common.Config.ConfigRoot
	pluginPath := path.Join(configRoot, "plugin", pluginID+".go")

	if err := os.WriteFile(pluginPath, []byte(newContent), 0644); err != nil {
		logger.Error("Failed to write plugin file", "plugin", pluginID, "error", err)
		return nil, fmt.Errorf("failed to write plugin file: %w", err)
	}

	// Update plugin in memory
	if err := plugin.NewPlugin(pluginPath, "", pluginID, plugin.YAEGI_PLUGIN); err != nil {
		logger.Error("Failed to reload plugin", "plugin", pluginID, "error", err)
		// Record failed operation
		RecordChangePush("plugin", pluginID, change.OldContent, newContent, "", "failed", err.Error())
		return nil, fmt.Errorf("failed to reload plugin: %w", err)
	}

	// Clear from legacy storage
	common.GlobalMu.Lock()
	delete(plugin.PluginsNew, pluginID)
	common.GlobalMu.Unlock()

	// Sync to followers using instruction system
	if err := cluster.GlobalInstructionManager.PublishComponentPushChange("plugin", pluginID, newContent, affectedProjects); err != nil {
		logger.Error("Failed to publish plugin push change instruction", "plugin", pluginID, "error", err)
	}

	// Record operation history
	RecordChangePush("plugin", pluginID, change.OldContent, newContent, "", "success", "")

	logger.Info("Plugin change applied successfully", "plugin", pluginID, "affected_projects", len(affectedProjects))

	return affectedProjects, nil
}

// Generic component application with type-specific handling
func applyComponentChange(change *EnhancedPendingChange) ([]string, error) {
	configRoot := common.Config.ConfigRoot
	var filePath string
	var affectedProjects []string

	// Determine file path and extension
	switch change.Type {
	case "input":
		filePath = path.Join(configRoot, "input", change.ID+".yaml")
	case "output":
		filePath = path.Join(configRoot, "output", change.ID+".yaml")
	case "ruleset":
		filePath = path.Join(configRoot, "ruleset", change.ID+".xml")
	default:
		return nil, fmt.Errorf("unsupported component type: %s", change.Type)
	}

	// Write directly to file
	err := os.WriteFile(filePath, []byte(change.NewContent), 0644)
	if err != nil {
		logger.Error("Failed to write component file", "type", change.Type, "id", change.ID, "error", err)
		return nil, fmt.Errorf("failed to write %s file: %w", change.Type, err)
	}

	// Type-specific component handling
	switch change.Type {
	case "input":
		// Stop old component if it exists
		common.GlobalMu.RLock()
		oldInput, exists := project.GlobalProject.Inputs[change.ID]
		common.GlobalMu.RUnlock()
		if exists {
			err := oldInput.Stop()
			if err != nil {
				logger.Error("Failed to stop old input", "type", change.Type, "id", change.ID, "error", err)
			}
			common.GlobalDailyStatsManager.CollectAllComponentsData()
		}

		// Reload component
		newInput, reloadErr := input.NewInput(filePath, "", change.ID)
		if reloadErr != nil {
			return nil, fmt.Errorf("failed to reload input: %w", reloadErr)
		}

		common.GlobalMu.Lock()
		project.GlobalProject.Inputs[change.ID] = newInput
		delete(project.GlobalProject.InputsNew, change.ID)
		common.GlobalMu.Unlock()

		affectedProjects = project.GetAffectedProjects("input", change.ID)

	case "output":
		// Stop old component if it exists
		common.GlobalMu.RLock()
		oldOutput, exists := project.GlobalProject.Outputs[change.ID]
		common.GlobalMu.RUnlock()
		if exists {
			err := oldOutput.Stop()
			if err != nil {
				logger.Error("Failed to stop old output", "type", change.Type, "id", change.ID, "error", err)
			}
			common.GlobalDailyStatsManager.CollectAllComponentsData()
		}

		// Reload component
		newOutput, reloadErr := output.NewOutput(filePath, "", change.ID)
		if reloadErr != nil {
			return nil, fmt.Errorf("failed to reload output: %w", reloadErr)
		}

		common.GlobalMu.Lock()
		project.GlobalProject.Outputs[change.ID] = newOutput
		delete(project.GlobalProject.OutputsNew, change.ID)
		common.GlobalMu.Unlock()

		affectedProjects = project.GetAffectedProjects("output", change.ID)

	case "ruleset":
		// Reload component
		newRuleset, reloadErr := rules_engine.NewRuleset(filePath, "", change.ID)
		if reloadErr != nil {
			return nil, fmt.Errorf("failed to reload ruleset: %w", reloadErr)
		}

		common.GlobalMu.Lock()
		project.GlobalProject.Rulesets[change.ID] = newRuleset
		delete(project.GlobalProject.RulesetsNew, change.ID)
		common.GlobalMu.Unlock()

		affectedProjects = project.GetAffectedProjects("ruleset", change.ID)
	}

	// Sync to followers using instruction system
	if err := cluster.GlobalInstructionManager.PublishComponentPushChange(change.Type, change.ID, change.NewContent, affectedProjects); err != nil {
		logger.Error("Failed to publish component push change instruction", "type", change.Type, "id", change.ID, "error", err)
	}

	logger.Info("Component change applied successfully", "type", change.Type, "id", change.ID, "affected_projects", len(affectedProjects))

	// 新增：记录操作历史
	RecordChangePush(change.Type, change.ID, change.OldContent, change.NewContent, "", "success", "")

	return affectedProjects, nil
}

func applyInputChange(change *EnhancedPendingChange) ([]string, error) {
	affectedProjects, err := applyComponentChange(change)
	if err != nil {
		logger.Error("Failed to apply input change", "id", change.ID, "error", err)
		return nil, err
	}
	return affectedProjects, nil
}

func applyOutputChange(change *EnhancedPendingChange) ([]string, error) {
	affectedProjects, err := applyComponentChange(change)
	if err != nil {
		logger.Error("Failed to apply output change", "id", change.ID, "error", err)
		return nil, err
	}
	return affectedProjects, nil
}

func applyRulesetChange(change *EnhancedPendingChange) ([]string, error) {
	affectedProjects, err := applyComponentChange(change)
	if err != nil {
		logger.Error("Failed to apply ruleset change", "id", change.ID, "error", err)
		return nil, err
	}
	return affectedProjects, nil
}

func applyProjectChange(change *EnhancedPendingChange) error {
	configRoot := common.Config.ConfigRoot
	projectPath := path.Join(configRoot, "project", change.ID+".yaml")

	// Write directly to file
	err := os.WriteFile(projectPath, []byte(change.NewContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write project file: %w", err)
	}

	// Stop old component if it exists
	common.GlobalMu.RLock()
	oldProject, exists := project.GlobalProject.Projects[change.ID]
	common.GlobalMu.RUnlock()
	if exists {
		oldProject.Stop()
	}

	// Reload component
	newProject, reloadErr := project.NewProject(projectPath, "", change.ID)
	if reloadErr != nil {
		return fmt.Errorf("failed to reload project: %w", reloadErr)
	}

	common.GlobalMu.Lock()
	project.GlobalProject.Projects[change.ID] = newProject
	delete(project.GlobalProject.ProjectsNew, change.ID)
	common.GlobalMu.Unlock()

	// Sync to followers using instruction system
	if err := cluster.GlobalInstructionManager.PublishComponentPushChange("project", change.ID, change.NewContent, nil); err != nil {
		logger.Error("Failed to publish project push change instruction", "project", change.ID, "error", err)
	}

	logger.Info("Project change applied successfully", "id", change.ID)

	// 新增：记录操作历史
	RecordChangePush("project", change.ID, change.OldContent, change.NewContent, "", "success", "")

	return nil
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
		// Write plugin file directly
		configRoot := common.Config.ConfigRoot
		pluginPath := path.Join(configRoot, "plugin", req.ID+".go")

		var oldContent string
		if existingPlugin, exists := plugin.Plugins[req.ID]; exists {
			oldContent = string(existingPlugin.Payload)
		}

		err = os.WriteFile(pluginPath, []byte(content), 0644)
		if err == nil {
			// Reload the plugin component
			err = plugin.NewPlugin(pluginPath, "", req.ID, plugin.YAEGI_PLUGIN)
			if err != nil {
				logger.Error("Failed to reload plugin after update", "id", req.ID, "error", err)
				RecordChangePush("plugin", req.ID, oldContent, content, "", "failed", err.Error())
			} else {
				// Clear the memory map entry after successful update
				common.GlobalMu.Lock()
				delete(plugin.PluginsNew, req.ID)
				common.GlobalMu.Unlock()

				RecordChangePush("plugin", req.ID, oldContent, content, "", "success", "")
			}
		}
	case "input", "output", "ruleset", "project":
		err = mergeComponentFile(req.Type, req.ID)
		if err == nil {
			// Clear the memory map entry after successful merge and reload components
			switch req.Type {
			case "input":
				// Reload the input component
				configRoot := common.Config.ConfigRoot
				inputPath := path.Join(configRoot, "input", req.ID+".yaml")

				// Check if old component exists and count projects using it (using centralized counter)
				common.GlobalMu.RLock()
				oldInput, exists := project.GlobalProject.Inputs[req.ID]
				common.GlobalMu.RUnlock()

				var oldContent string
				if exists {
					oldContent = oldInput.Config.RawConfig
				}

				var projectsUsingInput int
				if exists {
					projectsUsingInput = project.UsageCounter.CountProjectsUsingInput(req.ID)
				}

				// Only stop old component if no running projects are using it
				if exists && projectsUsingInput == 0 {
					logger.Info("Stopping old input component for reload", "id", req.ID, "projects_using", projectsUsingInput)
					err := oldInput.Stop()
					common.GlobalDailyStatsManager.CollectAllComponentsData()
					if err != nil {
						logger.Error("Failed to stop old input", "id", req.ID, "error", err)
					}
				} else if exists {
					logger.Info("Input component still in use, skipping stop during reload", "id", req.ID, "projects_using", projectsUsingInput)
				}

				newInput, reloadErr := input.NewInput(inputPath, "", req.ID)
				if reloadErr != nil {
					logger.Error("Failed to reload input after merge", "id", req.ID, "error", reloadErr)
					// Record failed operation
					RecordChangePush("input", req.ID, oldContent, content, "", "failed", reloadErr.Error())
				} else {
					common.GlobalMu.Lock()
					project.GlobalProject.Inputs[req.ID] = newInput
					common.GlobalMu.Unlock()
					logger.Info("Successfully reloaded input component", "id", req.ID)
					// Record successful operation
					RecordChangePush("input", req.ID, oldContent, content, "", "success", "")
				}

				// Only clear the memory map entry after successful recording
				common.GlobalMu.Lock()
				delete(project.GlobalProject.InputsNew, req.ID)
				common.GlobalMu.Unlock()
			case "output":
				// Reload the output component
				configRoot := common.Config.ConfigRoot
				outputPath := path.Join(configRoot, "output", req.ID+".yaml")

				// Check if old component exists and count projects using it (using centralized counter)
				common.GlobalMu.RLock()
				oldOutput, exists := project.GlobalProject.Outputs[req.ID]
				common.GlobalMu.RUnlock()

				var oldContent string
				if exists {
					oldContent = oldOutput.Config.RawConfig
				}

				var projectsUsingOutput int
				if exists {
					projectsUsingOutput = project.UsageCounter.CountProjectsUsingOutput(req.ID)
				}

				// Only stop old component if no running projects are using it
				if exists && projectsUsingOutput == 0 {
					logger.Info("Stopping old output component for reload", "id", req.ID, "projects_using", projectsUsingOutput)
					// Collect final statistics before stopping
					err := oldOutput.Stop()
					common.GlobalDailyStatsManager.CollectAllComponentsData()
					if err != nil {
						logger.Error("Failed to stop old output", "id", req.ID, "error", err)
					}
				} else if exists {
					logger.Info("Output component still in use, skipping stop during reload", "id", req.ID, "projects_using", projectsUsingOutput)
				}

				newOutput, reloadErr := output.NewOutput(outputPath, "", req.ID)
				if reloadErr != nil {
					logger.Error("Failed to reload output after merge", "id", req.ID, "error", reloadErr)
					// Record failed operation
					RecordChangePush("output", req.ID, oldContent, content, "", "failed", reloadErr.Error())
				} else {
					common.GlobalMu.Lock()
					project.GlobalProject.Outputs[req.ID] = newOutput
					common.GlobalMu.Unlock()
					logger.Info("Successfully reloaded output component", "id", req.ID)
					// Record successful operation
					RecordChangePush("output", req.ID, oldContent, content, "", "success", "")
				}

				// Only clear the memory map entry after successful recording
				common.GlobalMu.Lock()
				delete(project.GlobalProject.OutputsNew, req.ID)
				common.GlobalMu.Unlock()
			case "ruleset":
				// Reload the ruleset component
				configRoot := common.Config.ConfigRoot
				rulesetPath := path.Join(configRoot, "ruleset", req.ID+".xml")

				// Check if old component exists and count projects using it (using centralized counter)
				common.GlobalMu.RLock()
				oldRuleset, exists := project.GlobalProject.Rulesets[req.ID]
				common.GlobalMu.RUnlock()

				var oldContent string
				if exists {
					oldContent = oldRuleset.RawConfig
				}

				var projectsUsingRuleset int
				if exists {
					projectsUsingRuleset = project.UsageCounter.CountProjectsUsingRuleset(req.ID)
				}

				// Only stop old component if no running projects are using it
				if exists && projectsUsingRuleset == 0 {
					logger.Info("Stopping old ruleset component for reload", "id", req.ID, "projects_using", projectsUsingRuleset)
					err := oldRuleset.Stop()
					common.GlobalDailyStatsManager.CollectAllComponentsData()
					if err != nil {
						logger.Error("Failed to stop old ruleset", "id", req.ID, "error", err)
					}
				} else if exists {
					logger.Info("Ruleset component still in use, skipping stop during reload", "id", req.ID, "projects_using", projectsUsingRuleset)
				}

				newRuleset, reloadErr := rules_engine.NewRuleset(rulesetPath, "", req.ID)
				if reloadErr != nil {
					logger.Error("Failed to reload ruleset after merge", "id", req.ID, "error", reloadErr)
					// Record failed operation
					RecordChangePush("ruleset", req.ID, oldContent, content, "", "failed", reloadErr.Error())
				} else {
					common.GlobalMu.Lock()
					project.GlobalProject.Rulesets[req.ID] = newRuleset
					common.GlobalMu.Unlock()
					logger.Info("Successfully reloaded ruleset component", "id", req.ID)
					// Record successful operation
					RecordChangePush("ruleset", req.ID, oldContent, content, "", "success", "")
				}

				// Only clear the memory map entry after successful recording
				common.GlobalMu.Lock()
				delete(project.GlobalProject.RulesetsNew, req.ID)
				common.GlobalMu.Unlock()
			case "project":
				// Reload the project component
				configRoot := common.Config.ConfigRoot
				projectPath := path.Join(configRoot, "project", req.ID+".yaml")

				// Handle project lifecycle carefully
				var wasRunning bool
				var oldContent string
				common.GlobalMu.RLock()
				oldProject, exists := project.GlobalProject.Projects[req.ID]
				common.GlobalMu.RUnlock()
				if exists {
					wasRunning = (oldProject.Status == project.ProjectStatusRunning)
					oldContent = oldProject.Config.RawConfig
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
					// Record failed operation
					RecordChangePush("project", req.ID, oldContent, content, "", "failed", reloadErr.Error())
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
					// Record successful operation
					RecordChangePush("project", req.ID, oldContent, content, "", "success", "")
				}

				// Only clear the memory map entry after successful recording
				common.GlobalMu.Lock()
				delete(project.GlobalProject.ProjectsNew, req.ID)
				common.GlobalMu.Unlock()
			}
		}
	}

	if err != nil {
		logger.Error("Failed to apply change", "type", req.Type, "id", req.ID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to apply change: " + err.Error()})
	}

	// Note: We don't perform hot reload here as we will restart affected projects instead

	// Get affected projects first
	affectedProjects := project.GetAffectedProjects(req.Type, req.ID)

	// Sync changes to follower nodes using instruction system
	if req.Type == "project" {
		if err := cluster.GlobalInstructionManager.PublishComponentPushChange(req.Type, req.ID, content, nil); err != nil {
			logger.Error("Failed to publish component push change instruction", "type", req.Type, "id", req.ID, "error", err)
		}
	} else {
		if err := cluster.GlobalInstructionManager.PublishComponentPushChange(req.Type, req.ID, content, affectedProjects); err != nil {
			logger.Error("Failed to publish component push change instruction", "type", req.Type, "id", req.ID, "error", err)
		}
	}
	if len(affectedProjects) > 0 {
		logger.Info("Restarting affected projects", "count", len(affectedProjects))

		// Use unified restart function for better maintainability
		restartedCount, err := project.RestartProjectsSafely(affectedProjects, "component_change")
		if err != nil {
			logger.Error("Error during affected project restart", "error", err)
		}

		// Publish project restart instructions to followers
		if err := cluster.GlobalInstructionManager.PublishProjectsRestart(affectedProjects, "component_change"); err != nil {
			logger.Error("Failed to publish project restart instructions", "affected_projects", affectedProjects, "error", err)
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"message":            "Change applied successfully",
			"restarted_projects": restartedCount,
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

	// Log component details before restarting
	for i, p := range projectList {
		if p.Status == project.ProjectStatusRunning {
			logger.Info("Project details", "id", projectIDs[i], "inputs", len(p.Inputs), "rulesets", len(p.Rulesets), "outputs", len(p.Outputs))
			for inputID := range p.Inputs {
				logger.Info("Project has input", "project", projectIDs[i], "input", inputID)
			}
			for rulesetID := range p.Rulesets {
				logger.Info("Project has ruleset", "project", projectIDs[i], "ruleset", rulesetID)
			}
			for outputID := range p.Outputs {
				logger.Info("Project has output", "project", projectIDs[i], "output", outputID)
			}
		}
	}

	// Use unified restart function for better maintainability
	startedCount, err := project.RestartProjectsSafely(projectIDs, "user_action")
	if err != nil {
		logger.Error("Error during all projects restart", "error", err)
	}

	// Publish project restart instructions to followers
	if len(projectIDs) > 0 {
		if err := cluster.GlobalInstructionManager.PublishProjectsRestart(projectIDs, "user_action"); err != nil {
			logger.Error("Failed to publish project restart instructions for restart all", "affected_projects", projectIDs, "error", err)
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
			logger.PluginError("Plugin not found", "id", id)
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
