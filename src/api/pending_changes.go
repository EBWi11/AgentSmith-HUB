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
	// Use safe accessors instead of direct locking to avoid deadlock
	syncPluginsToEnhancedManager()
	syncInputsToEnhancedManager()
	syncOutputsToEnhancedManager()
	syncRulesetsToEnhancedManager()
	syncProjectsToEnhancedManager()
	cleanupObsoleteChanges()
}

// syncPluginsToEnhancedManager synchronizes plugin changes using safe accessors
func syncPluginsToEnhancedManager() {

	// Get plugins data using safe accessors
	pluginsData := plugin.GetAllPluginsNew()

	// Sync plugins with pending changes
	for name, newContent := range pluginsData {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing plugin using safe accessor
		oldContent = getExistingPluginContent(name)
		if oldContent != "" {
			isNew = false
		}

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
}

// syncInputsToEnhancedManager synchronizes input changes using safe accessors
func syncInputsToEnhancedManager() {
	// Get inputs data safely using safe accessors
	inputsData := project.GetAllInputsNew()

	// Sync inputs with pending changes
	for id, newContent := range inputsData {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing input
		if i, ok := project.GetInput(id); ok {
			oldContent = i.Config.RawConfig
			isNew = false
		}

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
}

// syncOutputsToEnhancedManager synchronizes output changes using safe accessors
func syncOutputsToEnhancedManager() {
	// Get outputs data safely using safe accessors
	outputsData := project.GetAllOutputsNew()

	// Sync outputs with pending changes
	for id, newContent := range outputsData {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing output
		if o, ok := project.GetOutput(id); ok {
			oldContent = o.Config.RawConfig
			isNew = false
		}

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
}

// syncRulesetsToEnhancedManager synchronizes ruleset changes using safe accessors
func syncRulesetsToEnhancedManager() {
	// Get rulesets data safely using safe accessors
	rulesetsData := project.GetAllRulesetsNew()

	// Sync rulesets with pending changes
	for id, newContent := range rulesetsData {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing ruleset
		if ruleset, ok := project.GetRuleset(id); ok {
			oldContent = ruleset.RawConfig
			isNew = false
		}

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
}

// syncProjectsToEnhancedManager synchronizes project changes using safe accessors
func syncProjectsToEnhancedManager() {
	// Get projects data safely using safe accessors
	projectsData := project.GetAllProjectsNew()

	// Sync projects with pending changes
	for id, newContent := range projectsData {
		var oldContent string
		isNew := true

		// Check if this is a modification to an existing project
		if proj, ok := project.GetProject(id); ok {
			oldContent = proj.Config.RawConfig
			isNew = false
		}

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
}

// Helper functions for safe access to plugin data
func getPendingPluginChange(id string) (string, bool) {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	content, exists := plugin.PluginsNew[id]
	return content, exists
}

func getExistingPluginContent(id string) string {
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()
	if pluginInstance, exists := plugin.Plugins[id]; exists {
		return string(pluginInstance.Payload)
	}
	return ""
}

// cleanupObsoleteChanges removes changes that no longer exist in legacy storage
func cleanupObsoleteChanges() {
	// Get all existing changes
	existingChanges := globalPendingChangeManager.GetAllChanges()

	// Create a map of what should exist based on current legacy storage
	shouldExist := make(map[string]bool)

	// Check what should exist from all sources using safe accessors
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

	// Remove from legacy storage using safe accessors
	switch changeType {
	case "plugin":
		plugin.DeletePluginNew(id)
	case "input":
		project.DeleteInputNew(id)
	case "output":
		project.DeleteOutputNew(id)
	case "ruleset":
		project.DeleteRulesetNew(id)
	case "project":
		project.DeleteProjectNew(id)
	}

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

		// Remove from legacy storage using safe accessors
		switch change.Type {
		case "plugin":
			plugin.DeletePluginNew(change.ID)
		case "input":
			project.DeleteInputNew(change.ID)
		case "output":
			project.DeleteOutputNew(change.ID)
		case "ruleset":
			project.DeleteRulesetNew(change.ID)
		case "project":
			project.DeleteProjectNew(change.ID)
		}

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

// ComponentReloadSource represents the source of component reload
type ComponentReloadSource string

const (
	SourceChangePush  ComponentReloadSource = "change_push"
	SourceLocalFile   ComponentReloadSource = "local_file"
	SourceClusterSync ComponentReloadSource = "cluster_sync"
)

// ComponentReloadRequest represents a request to reload a component
type ComponentReloadRequest struct {
	Type        string                `json:"type"`
	ID          string                `json:"id"`
	NewContent  string                `json:"new_content"`
	OldContent  string                `json:"old_content,omitempty"`
	Source      ComponentReloadSource `json:"source"`
	SkipVerify  bool                  `json:"skip_verify,omitempty"`
	WriteToFile bool                  `json:"write_to_file,omitempty"`
}

// reloadComponentUnified provides unified component reload logic for all sources
func reloadComponentUnified(req *ComponentReloadRequest) ([]string, error) {
	logger.Info("Starting unified component reload", "type", req.Type, "id", req.ID, "source", req.Source)

	// Phase 1: Validation
	if req.Type == "" || req.ID == "" {
		return nil, fmt.Errorf("component type and ID are required")
	}

	// Phase 2: Verification (optional based on source)
	if !req.SkipVerify {
		var verifyErr error
		switch req.Type {
		case "plugin":
			verifyErr = plugin.Verify("", req.NewContent, req.ID)
		case "input":
			verifyErr = input.Verify("", req.NewContent)
		case "output":
			verifyErr = output.Verify("", req.NewContent)
		case "ruleset":
			verifyErr = rules_engine.Verify("", req.NewContent)
		case "project":
			verifyErr = project.Verify("", req.NewContent)
		default:
			return nil, fmt.Errorf("unsupported component type: %s", req.Type)
		}

		if verifyErr != nil {
			logger.Error("Component verification failed", "type", req.Type, "id", req.ID, "error", verifyErr)
			return nil, fmt.Errorf("verification failed: %w", verifyErr)
		}
	}

	// Phase 3: Write to file (optional based on source)
	var filePath string
	if req.WriteToFile {
		configRoot := common.Config.ConfigRoot
		switch req.Type {
		case "input":
			filePath = path.Join(configRoot, "input", req.ID+".yaml")
		case "output":
			filePath = path.Join(configRoot, "output", req.ID+".yaml")
		case "ruleset":
			filePath = path.Join(configRoot, "ruleset", req.ID+".xml")
		case "project":
			filePath = path.Join(configRoot, "project", req.ID+".yaml")
		case "plugin":
			filePath = path.Join(configRoot, "plugin", req.ID+".go")
		default:
			return nil, fmt.Errorf("unsupported component type for file write: %s", req.Type)
		}

		err := os.WriteFile(filePath, []byte(req.NewContent), 0644)
		if err != nil {
			logger.Error("Failed to write component file", "type", req.Type, "id", req.ID, "error", err)
			return nil, fmt.Errorf("failed to write %s file: %w", req.Type, err)
		}

		// Remove .new file if it exists (after successful write)
		var tempPath string
		switch req.Type {
		case "plugin":
			tempPath = path.Join(configRoot, "plugin", req.ID+".go.new")
		case "input":
			tempPath = path.Join(configRoot, "input", req.ID+".yaml.new")
		case "output":
			tempPath = path.Join(configRoot, "output", req.ID+".yaml.new")
		case "ruleset":
			tempPath = path.Join(configRoot, "ruleset", req.ID+".xml.new")
		case "project":
			tempPath = path.Join(configRoot, "project", req.ID+".yaml.new")
		}

		if tempPath != "" {
			if _, err := os.Stat(tempPath); err == nil {
				err = os.Remove(tempPath)
				if err != nil {
					logger.Warn("Failed to remove temp file after successful apply", "path", tempPath, "error", err)
				} else {
					logger.Info("Temp file removed after successful apply", "path", tempPath)
				}
			}
		}
	}

	// Phase 4: Stop old component and create new one
	var affectedProjects []string
	switch req.Type {
	case "input":
		// Create new component instance
		var newInput *input.Input
		var err error
		if req.WriteToFile && filePath != "" {
			newInput, err = input.NewInput(filePath, "", req.ID)
		} else {
			newInput, err = input.NewInput("", req.NewContent, req.ID)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create input: %w", err)
		}

		// Replace in global registry using safe accessors
		project.SetInput(req.ID, newInput)
		project.DeleteInputNew(req.ID)

		affectedProjects = project.GetAffectedProjects("input", req.ID)

	case "output":
		// Create new component instance
		var newOutput *output.Output
		var err error
		if req.WriteToFile && filePath != "" {
			newOutput, err = output.NewOutput(filePath, "", req.ID)
		} else {
			newOutput, err = output.NewOutput("", req.NewContent, req.ID)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create output: %w", err)
		}

		// Replace in global registry using safe accessors
		project.SetOutput(req.ID, newOutput)
		project.DeleteOutputNew(req.ID)

		affectedProjects = project.GetAffectedProjects("output", req.ID)

	case "ruleset":
		// Create new component instance
		var newRuleset *rules_engine.Ruleset
		var err error
		if req.WriteToFile && filePath != "" {
			newRuleset, err = rules_engine.NewRuleset(filePath, "", req.ID)
		} else {
			newRuleset, err = rules_engine.NewRuleset("", req.NewContent, req.ID)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create ruleset: %w", err)
		}

		// Replace in global registry using safe accessors
		project.SetRuleset(req.ID, newRuleset)
		project.DeleteRulesetNew(req.ID)

		affectedProjects = project.GetAffectedProjects("ruleset", req.ID)

	case "project":
		// Create new component instance
		var newProject *project.Project
		var err error
		if req.WriteToFile && filePath != "" {
			newProject, err = project.NewProject(filePath, "", req.ID, false)
		} else {
			newProject, err = project.NewProject("", req.NewContent, req.ID, false)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create project: %w", err)
		}

		// Replace in global registry using safe accessors
		project.SetProject(req.ID, newProject)
		project.DeleteProjectNew(req.ID)

		// Projects don't have affected projects since they restart themselves
		affectedProjects = []string{req.ID}

	case "plugin":
		// Create new component instance
		var err error
		if req.WriteToFile && filePath != "" {
			err = plugin.NewPlugin(filePath, "", req.ID, plugin.YAEGI_PLUGIN)
		} else {
			err = plugin.NewPlugin("", req.NewContent, req.ID, plugin.YAEGI_PLUGIN)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create plugin: %w", err)
		}

		// Clear temporary version using safe accessor
		plugin.DeletePluginNew(req.ID)

		affectedProjects = project.GetAffectedProjects("plugin", req.ID)

	default:
		return nil, fmt.Errorf("unsupported component type: %s", req.Type)
	}

	// Phase 5: Update global config maps and sync to followers
	if common.IsCurrentNodeLeader() {
		updateGlobalComponentConfigMap(req.Type, req.ID, req.NewContent)

		// Sync to followers using instruction system
		if err := cluster.GlobalInstructionManager.PublishComponentPushChange(req.Type, req.ID, req.NewContent, affectedProjects); err != nil {
			logger.Error("Failed to publish component push change instruction", "type", req.Type, "id", req.ID, "error", err)
		}
	}

	// Phase 6: Record operation history
	switch req.Source {
	case SourceChangePush:
		RecordChangePush(req.Type, req.ID, req.OldContent, req.NewContent, "", "success", "")
	case SourceLocalFile:
		RecordLocalPush(req.Type, req.ID, req.NewContent, "success", "")
	case SourceClusterSync:
		// Cluster sync doesn't need to record history to avoid loops
	}

	logger.Info("Component reload completed successfully", "type", req.Type, "id", req.ID, "source", req.Source, "affected_projects", len(affectedProjects))
	return affectedProjects, nil
}

// updateGlobalComponentConfigMap updates the global component config map
func updateGlobalComponentConfigMap(componentType, id, content string) {
	common.SetRawConfig(componentType, id, content)
	logger.Debug("Updated global component config map", "type", componentType, "id", id)
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

	// Get pending change using safe accessors
	var content string
	var oldContent string
	var found bool

	switch req.Type {
	case "plugin":
		content, found = getPendingPluginChange(req.ID)
		if found {
			oldContent = getExistingPluginContent(req.ID)
		}
	case "input":
		content, found = project.GetInputNew(req.ID)
		if found {
			if inp, exists := project.GetInput(req.ID); exists {
				oldContent = inp.Config.RawConfig
			}
		}
	case "output":
		content, found = project.GetOutputNew(req.ID)
		if found {
			if out, exists := project.GetOutput(req.ID); exists {
				oldContent = out.Config.RawConfig
			}
		}
	case "ruleset":
		content, found = project.GetRulesetNew(req.ID)
		if found {
			if rs, exists := project.GetRuleset(req.ID); exists {
				oldContent = rs.RawConfig
			}
		}
	case "project":
		content, found = project.GetProjectNew(req.ID)
		if found {
			if proj, exists := project.GetProject(req.ID); exists {
				oldContent = proj.Config.RawConfig
			}
		}
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid component type"})
	}

	if !found {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("No pending changes found for this %s", req.Type)})
	}

	// Use unified reload logic to avoid scattered lock operations
	reloadReq := &ComponentReloadRequest{
		Type:        req.Type,
		ID:          req.ID,
		NewContent:  content,
		OldContent:  oldContent,
		Source:      SourceChangePush,
		SkipVerify:  false, // Always verify for single changes
		WriteToFile: true,  // Always write to file for persistence
	}

	affectedProjects, err := reloadComponentUnified(reloadReq)
	if err != nil {
		logger.Error("Failed to apply single change", "type", req.Type, "id", req.ID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to apply change: " + err.Error()})
	}

	// Publish cluster instructions and restart affected projects
	if err := cluster.GlobalInstructionManager.PublishComponentPushChange(req.Type, req.ID, content, affectedProjects); err != nil {
		logger.Error("Failed to publish component push change instruction", "type", req.Type, "id", req.ID, "error", err)
	}

	if len(affectedProjects) > 0 {
		logger.Info("Restarting affected projects asynchronously", "count", len(affectedProjects))
		go func() {
			for _, id := range affectedProjects {
				// Use safe accessor without additional locking
				if p, ok := project.GetProject(id); ok {
					err := p.Restart()
					if err != nil {
						logger.Error("Failed to restart affected project", "id", id, "error", err)
						if err := cluster.GlobalInstructionManager.PublishProjectRestart(id); err != nil {
							logger.Error("Failed to publish project restart instructions", "affected_projects", affectedProjects, "error", err)
						}
					}
				}
			}
		}()

		return c.JSON(http.StatusOK, map[string]interface{}{
			"message":            "Change applied successfully, projects are restarting asynchronously",
			"restarted_projects": len(affectedProjects),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Change applied successfully"})
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

	// Use safe accessors for reading component data
	switch singularType {
	case "input":
		originalPath = path.Join(configRoot, "input", id+".yaml")
		tempPath = originalPath + ".new"

		if i, ok := project.GetInput(id); ok {
			content = i.Config.RawConfig
		} else {
			logger.Error("Input not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "input not found"})
		}

	case "output":
		originalPath = path.Join(configRoot, "output", id+".yaml")
		tempPath = originalPath + ".new"

		if o, ok := project.GetOutput(id); ok {
			content = o.Config.RawConfig
		} else {
			logger.Error("Output not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "output not found"})
		}

	case "ruleset":
		originalPath = path.Join(configRoot, "ruleset", id+".xml")
		tempPath = originalPath + ".new"

		if ruleset, ok := project.GetRuleset(id); ok {
			content = ruleset.RawConfig
		} else {
			logger.Error("Ruleset not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
		}

	case "project":
		originalPath = path.Join(configRoot, "project", id+".yaml")
		tempPath = originalPath + ".new"

		if proj, ok := project.GetProject(id); ok {
			content = proj.Config.RawConfig
		} else {
			logger.Error("Project not found", "id", id)
			return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
		}

	case "plugin":
		originalPath = path.Join(configRoot, "plugin", id+".go")
		tempPath = originalPath + ".new"

		content = getExistingPluginContent(id)
		if content == "" {
			logger.PluginError("Plugin not found", "id", id)
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

	switch singularType {
	case "input":
		project.SetInputNew(id, content)
	case "output":
		project.SetOutputNew(id, content)
	case "ruleset":
		project.SetRulesetNew(id, content)
	case "project":
		project.SetProjectNew(id, content)
	case "plugin":
		plugin.SetPluginNew(id, content)
	}

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

	// Remove tempo`rary file content from memory using safe accessors
	switch singularType {
	case "input":
		project.DeleteInputNew(id)
	case "output":
		project.DeleteOutputNew(id)
	case "ruleset":
		project.DeleteRulesetNew(id)
	case "project":
		project.DeleteProjectNew(id)
	case "plugin":
		plugin.DeletePluginNew(id)
	}

	logger.Info("Temp file deleted successfully", "path", tempPath)
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Temp file deleted successfully",
	})
}
