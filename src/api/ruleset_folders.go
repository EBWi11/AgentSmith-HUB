package api

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/project"
	"AgentSmith-HUB/rules_engine"

	"github.com/labstack/echo/v4"
)

// Regex for valid folder names: alphanumeric, underscore, hyphen
var validFolderNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func isValidRulesetFolderName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && validFolderNameRegex.MatchString(name)
}

// getRulesetBaseDir returns the base directory for rulesets
func getRulesetBaseDir() string {
	return filepath.Join(common.Config.ConfigRoot, "ruleset")
}

// getRulesetFolderFromPath extracts the folder name from a ruleset's full file path.
// Returns "" if the ruleset is in the root directory.
func getRulesetFolderFromPath(rulesetPath string) string {
	if rulesetPath == "" {
		return ""
	}
	baseDir := getRulesetBaseDir()
	relPath, err := filepath.Rel(baseDir, rulesetPath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(relPath)
	if dir == "." {
		return ""
	}
	// Only support one-level folders, so return just the first segment
	parts := strings.SplitN(dir, string(filepath.Separator), 2)
	return parts[0]
}

// findRulesetPaths resolves the actual file paths for a ruleset by checking the loaded
// ruleset's Path field or searching the filesystem.
// Returns (formalPath, tempPath) where the files are or should be.
func findRulesetPaths(id string) (formalPath string, tempPath string) {
	baseDir := getRulesetBaseDir()

	// Check loaded rulesets first - they have the authoritative Path
	if rs, exists := project.GetRuleset(id); exists && rs.Path != "" {
		formalPath = rs.Path
		tempPath = formalPath + ".new"
		return
	}

	// Search filesystem for the formal file
	_ = filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		filename := d.Name()
		if filename == id+RULESET_EXT {
			formalPath = path
			return filepath.SkipAll
		}
		return nil
	})

	if formalPath != "" {
		tempPath = formalPath + ".new"
		return
	}

	// Search filesystem for the temp file
	_ = filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		filename := d.Name()
		if filename == id+RULESET_EXT_NEW {
			tempPath = path
			formalPath = strings.TrimSuffix(path, ".new")
			return filepath.SkipAll
		}
		return nil
	})

	if formalPath == "" {
		// Default to root directory
		formalPath = filepath.Join(baseDir, id+RULESET_EXT)
		tempPath = formalPath + ".new"
	}

	return
}

// findRulesetPathInFolder constructs the file paths for a ruleset in a specific folder.
func findRulesetPathInFolder(id string, folder string) (formalPath string, tempPath string) {
	baseDir := getRulesetBaseDir()
	if folder != "" {
		formalPath = filepath.Join(baseDir, folder, id+RULESET_EXT)
	} else {
		formalPath = filepath.Join(baseDir, id+RULESET_EXT)
	}
	tempPath = formalPath + ".new"
	return
}

// getRulesetFolders returns a list of all folders under the ruleset directory
func getRulesetFolders(c echo.Context) error {
	baseDir := getRulesetBaseDir()

	// Ensure the base directory exists
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return c.JSON(http.StatusOK, []map[string]interface{}{})
	}

	folders := make([]map[string]interface{}, 0)

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		logger.Error("failed to read ruleset directory", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to read ruleset directory",
		})
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden directories
		if strings.HasPrefix(name, ".") {
			continue
		}

		// Count rulesets in this folder
		folderPath := filepath.Join(baseDir, name)
		rulesetCount := 0
		folderEntries, err := os.ReadDir(folderPath)
		if err == nil {
			for _, fe := range folderEntries {
				if !fe.IsDir() && strings.HasSuffix(fe.Name(), RULESET_EXT) && !strings.HasSuffix(fe.Name(), RULESET_EXT_NEW) {
					rulesetCount++
				}
			}
		}

		// Also count temp-only rulesets
		tempOnlyCount := 0
		if err == nil {
			for _, fe := range folderEntries {
				if !fe.IsDir() && strings.HasSuffix(fe.Name(), RULESET_EXT_NEW) {
					baseName := strings.TrimSuffix(fe.Name(), ".new")
					// Check if formal file exists
					hasNormal := false
					for _, fe2 := range folderEntries {
						if fe2.Name() == baseName {
							hasNormal = true
							break
						}
					}
					if !hasNormal {
						tempOnlyCount++
					}
				}
			}
		}

		folders = append(folders, map[string]interface{}{
			"name":           name,
			"ruleset_count":  rulesetCount + tempOnlyCount,
		})
	}

	return c.JSON(http.StatusOK, folders)
}

// createRulesetFolder creates a new folder in the ruleset directory
func createRulesetFolder(c echo.Context) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "folder name cannot be empty"})
	}

	if !isValidRulesetFolderName(req.Name) {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "folder name can only contain letters, numbers, underscores, and hyphens",
		})
	}

	baseDir := getRulesetBaseDir()
	folderPath := filepath.Join(baseDir, req.Name)

	// Check if folder already exists
	if _, err := os.Stat(folderPath); err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "folder already exists"})
	}

	// Create the folder
	if err := os.MkdirAll(folderPath, 0755); err != nil {
		logger.Error("failed to create ruleset folder", "path", folderPath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create folder"})
	}

	logger.Info("created ruleset folder", "name", req.Name)

	return c.JSON(http.StatusCreated, map[string]string{
		"message": fmt.Sprintf("Folder '%s' created successfully", req.Name),
		"name":    req.Name,
	})
}

// renameRulesetFolder renames a folder in the ruleset directory
func renameRulesetFolder(c echo.Context) error {
	oldName := strings.TrimSpace(c.Param("name"))
	if !isValidRulesetFolderName(oldName) {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid folder name",
		})
	}

	var req struct {
		NewName string `json:"new_name"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	req.NewName = strings.TrimSpace(req.NewName)
	if req.NewName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "new folder name cannot be empty"})
	}

	if !isValidRulesetFolderName(req.NewName) {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "folder name can only contain letters, numbers, underscores, and hyphens",
		})
	}

	if oldName == req.NewName {
		return c.JSON(http.StatusOK, map[string]string{"message": "no change needed"})
	}

	baseDir := getRulesetBaseDir()
	oldPath := filepath.Join(baseDir, oldName)
	newPath := filepath.Join(baseDir, req.NewName)

	// Check if old folder exists
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "folder not found"})
	}

	// Check if new folder already exists
	if _, err := os.Stat(newPath); err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "a folder with the new name already exists"})
	}

	// Rename the folder
	if err := os.Rename(oldPath, newPath); err != nil {
		logger.Error("failed to rename ruleset folder", "old", oldPath, "new", newPath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to rename folder"})
	}

	// Update the Path field for all loaded rulesets that were in this folder
	project.ForEachRuleset(func(rulesetId string, rs *rules_engine.Ruleset) bool {
		folder := getRulesetFolderFromPath(rs.Path)
		if folder == oldName {
			newRulesetPath := filepath.Join(newPath, filepath.Base(rs.Path))
			rs.Path = newRulesetPath
		}
		return true
	})

	logger.Info("renamed ruleset folder", "old", oldName, "new", req.NewName)

	return c.JSON(http.StatusOK, map[string]string{
		"message":  fmt.Sprintf("Folder renamed from '%s' to '%s'", oldName, req.NewName),
		"old_name": oldName,
		"new_name": req.NewName,
	})
}

// deleteRulesetFolder deletes an empty folder from the ruleset directory
func deleteRulesetFolder(c echo.Context) error {
	name := strings.TrimSpace(c.Param("name"))
	if !isValidRulesetFolderName(name) {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid folder name",
		})
	}

	baseDir := getRulesetBaseDir()
	folderPath := filepath.Join(baseDir, name)

	// Check if folder exists
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "folder not found"})
	}

	// Check if folder is empty (no xml files)
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read folder"})
	}

	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), RULESET_EXT) || strings.HasSuffix(entry.Name(), RULESET_EXT_NEW)) {
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "folder is not empty, please move or delete all rulesets first",
			})
		}
	}

	// Delete the folder
	if err := os.RemoveAll(folderPath); err != nil {
		logger.Error("failed to delete ruleset folder", "path", folderPath, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete folder"})
	}

	logger.Info("deleted ruleset folder", "name", name)

	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Folder '%s' deleted successfully", name),
	})
}

// moveRuleset moves a ruleset to a different folder
func moveRuleset(c echo.Context) error {
	id := c.Param("id")

	var req struct {
		Folder string `json:"folder"` // Target folder, empty string means root
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	req.Folder = strings.TrimSpace(req.Folder)

	// Validate folder name if not empty
	if req.Folder != "" && !isValidRulesetFolderName(req.Folder) {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "folder name can only contain letters, numbers, underscores, and hyphens",
		})
	}

	// Find current paths
	currentFormal, currentTemp := findRulesetPaths(id)
	currentFolder := getRulesetFolderFromPath(currentFormal)

	// If already in the target folder, no-op
	if currentFolder == req.Folder {
		return c.JSON(http.StatusOK, map[string]string{"message": "ruleset is already in this folder"})
	}

	// Calculate new paths
	newFormal, newTemp := findRulesetPathInFolder(id, req.Folder)

	// Ensure target directory exists
	targetDir := filepath.Dir(newFormal)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create target directory"})
	}

	// Check if target files already exist
	if _, err := os.Stat(newFormal); err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "a file already exists at the target location"})
	}
	if _, err := os.Stat(newTemp); err == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "a temp file already exists at the target location"})
	}

	currentFormalExists := false
	if _, err := os.Stat(currentFormal); err == nil {
		currentFormalExists = true
	}
	currentTempExists := false
	if _, err := os.Stat(currentTemp); err == nil {
		currentTempExists = true
	}
	if !currentFormalExists && !currentTempExists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset files not found"})
	}

	// Move formal file if it exists
	formalMoved := false
	if currentFormalExists {
		if err := os.Rename(currentFormal, newFormal); err != nil {
			logger.Error("failed to move ruleset file", "from", currentFormal, "to", newFormal, "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to move ruleset file"})
		}
		formalMoved = true
	}

	// Move temp file if it exists
	if currentTempExists {
		if err := os.Rename(currentTemp, newTemp); err != nil {
			// Roll back formal file movement to avoid partial moves.
			if formalMoved {
				if rollbackErr := os.Rename(newFormal, currentFormal); rollbackErr != nil {
					logger.Error("failed to rollback moved ruleset file",
						"from", newFormal, "to", currentFormal, "error", rollbackErr)
				}
			}
			logger.Error("failed to move ruleset temp file", "from", currentTemp, "to", newTemp, "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to move ruleset temp file"})
		}
	}

	// Update the Path field in the loaded ruleset
	if rs, exists := project.GetRuleset(id); exists {
		rs.Path = newFormal
	}

	logger.Info("moved ruleset", "id", id, "from_folder", currentFolder, "to_folder", req.Folder)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":     fmt.Sprintf("Ruleset '%s' moved successfully", id),
		"from_folder": currentFolder,
		"to_folder":   req.Folder,
	})
}
