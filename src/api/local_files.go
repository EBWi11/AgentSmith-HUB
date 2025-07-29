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
	"crypto/md5"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
)

// getLocalChanges returns a list of local changes compared to memory
func getLocalChanges(c echo.Context) error {
	changes := make([]map[string]interface{}, 0)
	configRoot := common.Config.ConfigRoot

	// Use safe accessors instead of long-term locking

	// Check inputs
	inputDir := filepath.Join(configRoot, "input")
	if _, err := os.Stat(inputDir); err == nil {
		if err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // Skip errors
			}
			if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return nil
			}

			filename := d.Name()
			id := strings.TrimSuffix(filename, ".yaml")

			// Read file content
			fileContent, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			// Check if exists in memory
			memoryInput, exists := project.GetInput(id)
			var memoryContent string
			if exists {
				memoryContent = memoryInput.Config.RawConfig
			}

			// Compare content
			if !exists || strings.TrimSpace(string(fileContent)) != strings.TrimSpace(memoryContent) {
				changeType := "modified"
				if !exists {
					changeType = "new"
				}

				changes = append(changes, map[string]interface{}{
					"type":           "input",
					"id":             id,
					"change_type":    changeType,
					"file_path":      path,
					"file_size":      len(fileContent),
					"checksum":       fmt.Sprintf("%x", md5.Sum(fileContent)),
					"local_content":  string(fileContent),
					"memory_content": memoryContent,
					"has_local":      true,
					"has_memory":     exists,
				})
			}

			return nil
		}); err != nil {
			// Continue even if there's an error
		}
	}

	// Check outputs
	outputDir := filepath.Join(configRoot, "output")
	if _, err := os.Stat(outputDir); err == nil {
		if err := filepath.WalkDir(outputDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return nil
			}

			filename := d.Name()
			id := strings.TrimSuffix(filename, ".yaml")

			fileContent, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			memoryOutput, exists := project.GetOutput(id)
			var memoryContent string
			if exists {
				memoryContent = memoryOutput.Config.RawConfig
			}

			if !exists || strings.TrimSpace(string(fileContent)) != strings.TrimSpace(memoryContent) {
				changeType := "modified"
				if !exists {
					changeType = "new"
				}

				changes = append(changes, map[string]interface{}{
					"type":           "output",
					"id":             id,
					"change_type":    changeType,
					"file_path":      path,
					"file_size":      len(fileContent),
					"checksum":       fmt.Sprintf("%x", md5.Sum(fileContent)),
					"local_content":  string(fileContent),
					"memory_content": memoryContent,
					"has_local":      true,
					"has_memory":     exists,
				})
			}

			return nil
		}); err != nil {
			// Continue
		}
	}

	// Check rulesets
	rulesetDir := filepath.Join(configRoot, "ruleset")
	if _, err := os.Stat(rulesetDir); err == nil {
		if err := filepath.WalkDir(rulesetDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() || !strings.HasSuffix(path, ".xml") {
				return nil
			}

			filename := d.Name()
			id := strings.TrimSuffix(filename, ".xml")

			fileContent, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			memoryRuleset, exists := project.GetRuleset(id)
			var memoryContent string
			if exists {
				memoryContent = memoryRuleset.RawConfig
			}

			if !exists || strings.TrimSpace(string(fileContent)) != strings.TrimSpace(memoryContent) {
				changeType := "modified"
				if !exists {
					changeType = "new"
				}

				changes = append(changes, map[string]interface{}{
					"type":           "ruleset",
					"id":             id,
					"change_type":    changeType,
					"file_path":      path,
					"file_size":      len(fileContent),
					"checksum":       fmt.Sprintf("%x", md5.Sum(fileContent)),
					"local_content":  string(fileContent),
					"memory_content": memoryContent,
					"has_local":      true,
					"has_memory":     exists,
				})
			}

			return nil
		}); err != nil {
			// Continue
		}
	}

	// Check projects
	projectDir := filepath.Join(configRoot, "project")
	if _, err := os.Stat(projectDir); err == nil {
		if err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return nil
			}

			filename := d.Name()
			id := strings.TrimSuffix(filename, ".yaml")

			fileContent, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			memoryProject, exists := project.GetProject(id)
			var memoryContent string
			if exists {
				memoryContent = memoryProject.Config.RawConfig
			}

			if !exists || strings.TrimSpace(string(fileContent)) != strings.TrimSpace(memoryContent) {
				changeType := "modified"
				if !exists {
					changeType = "new"
				}

				changes = append(changes, map[string]interface{}{
					"type":           "project",
					"id":             id,
					"change_type":    changeType,
					"file_path":      path,
					"file_size":      len(fileContent),
					"checksum":       fmt.Sprintf("%x", md5.Sum(fileContent)),
					"local_content":  string(fileContent),
					"memory_content": memoryContent,
					"has_local":      true,
					"has_memory":     exists,
				})
			}

			return nil
		}); err != nil {
			// Continue
		}
	}

	// Check plugins
	pluginDir := filepath.Join(configRoot, "plugin")
	if _, err := os.Stat(pluginDir); err == nil {
		if err := filepath.WalkDir(pluginDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}

			filename := d.Name()
			id := strings.TrimSuffix(filename, ".go")

			fileContent, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			memoryPlugin, exists := plugin.Plugins[id]
			var memoryContent string
			if exists && memoryPlugin.Type == plugin.YAEGI_PLUGIN {
				memoryContent = string(memoryPlugin.Payload)
			}

			// Also check if there's content in temporary memory (PluginsNew)
			// If plugin was loaded but not yet applied, use temporary content for comparison
			if tempContent, existsInTemp := plugin.PluginsNew[id]; existsInTemp {
				memoryContent = tempContent
				exists = true // Treat as existing if it's in temporary memory
			}

			if !exists || strings.TrimSpace(string(fileContent)) != strings.TrimSpace(memoryContent) {
				changeType := "modified"
				if !exists {
					changeType = "new"
				}

				changes = append(changes, map[string]interface{}{
					"type":           "plugin",
					"id":             id,
					"change_type":    changeType,
					"file_path":      path,
					"file_size":      len(fileContent),
					"checksum":       fmt.Sprintf("%x", md5.Sum(fileContent)),
					"local_content":  string(fileContent),
					"memory_content": memoryContent,
					"has_local":      true,
					"has_memory":     exists,
				})
			}

			return nil
		}); err != nil {
			// Continue
		}
	}

	// Check for components that exist in memory but not in local files (deleted locally)
	// configRoot is already defined above

	// Check for deleted inputs
	project.ForEachInput(func(id string, input *input.Input) bool {
		inputPath := filepath.Join(configRoot, "input", id+".yaml")
		if _, err := os.Stat(inputPath); os.IsNotExist(err) {
			changes = append(changes, map[string]interface{}{
				"type":           "input",
				"id":             id,
				"change_type":    "deleted",
				"file_path":      inputPath,
				"file_size":      0,
				"checksum":       "",
				"local_content":  "",
				"memory_content": input.Config.RawConfig,
				"has_local":      false,
				"has_memory":     true,
			})
		}
		return true
	})

	// Check for deleted outputs
	project.ForEachOutput(func(id string, output *output.Output) bool {
		outputPath := filepath.Join(configRoot, "output", id+".yaml")
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			changes = append(changes, map[string]interface{}{
				"type":           "output",
				"id":             id,
				"change_type":    "deleted",
				"file_path":      outputPath,
				"file_size":      0,
				"checksum":       "",
				"local_content":  "",
				"memory_content": output.Config.RawConfig,
				"has_local":      false,
				"has_memory":     true,
			})
		}
		return true
	})

	// Check for deleted rulesets
	project.ForEachRuleset(func(id string, ruleset *rules_engine.Ruleset) bool {
		rulesetPath := filepath.Join(configRoot, "ruleset", id+".xml")
		if _, err := os.Stat(rulesetPath); os.IsNotExist(err) {
			changes = append(changes, map[string]interface{}{
				"type":           "ruleset",
				"id":             id,
				"change_type":    "deleted",
				"file_path":      rulesetPath,
				"file_size":      0,
				"checksum":       "",
				"local_content":  "",
				"memory_content": ruleset.RawConfig,
				"has_local":      false,
				"has_memory":     true,
			})
		}
		return true
	})

	// Check for deleted projects
	project.ForEachProject(func(id string, proj *project.Project) bool {
		projectPath := filepath.Join(configRoot, "project", id+".yaml")
		if _, err := os.Stat(projectPath); os.IsNotExist(err) {
			changes = append(changes, map[string]interface{}{
				"type":           "project",
				"id":             id,
				"change_type":    "deleted",
				"file_path":      projectPath,
				"file_size":      0,
				"checksum":       "",
				"local_content":  "",
				"memory_content": proj.Config.RawConfig,
				"has_local":      false,
				"has_memory":     true,
			})
		}
		return true
	})

	// Check for deleted plugins
	for id, pluginInstance := range plugin.Plugins {
		// Only check yaegi plugins (skip local/built-in plugins)
		if pluginInstance.Type != plugin.YAEGI_PLUGIN {
			continue
		}
		pluginPath := filepath.Join(configRoot, "plugin", id+".go")
		if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
			changes = append(changes, map[string]interface{}{
				"type":           "plugin",
				"id":             id,
				"change_type":    "deleted",
				"file_path":      pluginPath,
				"file_size":      0,
				"checksum":       "",
				"local_content":  "",
				"memory_content": string(pluginInstance.Payload),
				"has_local":      false,
				"has_memory":     true,
			})
		}
	}

	return c.JSON(http.StatusOK, changes)
}

// loadLocalChanges loads all local changes into memory
func loadLocalChanges(c echo.Context) error {
	// Get all local changes first
	changes := make([]map[string]interface{}, 0)
	configRoot := common.Config.ConfigRoot

	// Get all local changes (reuse the logic from getLocalChanges)
	// Check inputs
	inputDir := filepath.Join(configRoot, "input")
	filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		filename := d.Name()
		id := strings.TrimSuffix(filename, ".yaml")

		fileContent, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		memoryInput, exists := project.GetInput(id)
		var memoryContent string
		if exists {
			memoryContent = memoryInput.Config.RawConfig
		}

		if !exists || strings.TrimSpace(string(fileContent)) != strings.TrimSpace(memoryContent) {
			changes = append(changes, map[string]interface{}{
				"type":         "input",
				"id":           id,
				"file_path":    path,
				"file_content": string(fileContent),
			})
		}
		return nil
	})

	// Check outputs
	outputDir := filepath.Join(configRoot, "output")
	filepath.WalkDir(outputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		filename := d.Name()
		id := strings.TrimSuffix(filename, ".yaml")

		fileContent, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		memoryOutput, exists := project.GetOutput(id)
		var memoryContent string
		if exists {
			memoryContent = memoryOutput.Config.RawConfig
		}

		if !exists || strings.TrimSpace(string(fileContent)) != strings.TrimSpace(memoryContent) {
			changes = append(changes, map[string]interface{}{
				"type":         "output",
				"id":           id,
				"file_path":    path,
				"file_content": string(fileContent),
			})
		}
		return nil
	})

	// Check rulesets
	rulesetDir := filepath.Join(configRoot, "ruleset")
	filepath.WalkDir(rulesetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".xml") {
			return nil
		}

		filename := d.Name()
		id := strings.TrimSuffix(filename, ".xml")

		fileContent, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		memoryRuleset, exists := project.GetRuleset(id)
		var memoryContent string
		if exists {
			memoryContent = memoryRuleset.RawConfig
		}

		if !exists || strings.TrimSpace(string(fileContent)) != strings.TrimSpace(memoryContent) {
			changes = append(changes, map[string]interface{}{
				"type":         "ruleset",
				"id":           id,
				"file_path":    path,
				"file_content": string(fileContent),
			})
		}
		return nil
	})

	// Check projects
	projectDir := filepath.Join(configRoot, "project")
	filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		filename := d.Name()
		id := strings.TrimSuffix(filename, ".yaml")

		fileContent, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		memoryProject, exists := project.GetProject(id)
		var memoryContent string
		if exists {
			memoryContent = memoryProject.Config.RawConfig
		}

		if !exists || strings.TrimSpace(string(fileContent)) != strings.TrimSpace(memoryContent) {
			changes = append(changes, map[string]interface{}{
				"type":         "project",
				"id":           id,
				"file_path":    path,
				"file_content": string(fileContent),
			})
		}
		return nil
	})

	// Check plugins
	pluginDir := filepath.Join(configRoot, "plugin")
	filepath.WalkDir(pluginDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		filename := d.Name()
		id := strings.TrimSuffix(filename, ".go")

		fileContent, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		memoryPlugin, exists := plugin.Plugins[id]
		var memoryContent string
		if exists && memoryPlugin.Type == plugin.YAEGI_PLUGIN {
			memoryContent = string(memoryPlugin.Payload)
		}

		// Also check if there's content in temporary memory (PluginsNew)
		// If plugin was loaded but not yet applied, use temporary content for comparison
		if tempContent, existsInTemp := plugin.PluginsNew[id]; existsInTemp {
			memoryContent = tempContent
			exists = true // Treat as existing if it's in temporary memory
		}

		if !exists || strings.TrimSpace(string(fileContent)) != strings.TrimSpace(memoryContent) {
			changes = append(changes, map[string]interface{}{
				"type":         "plugin",
				"id":           id,
				"file_path":    path,
				"file_content": string(fileContent),
			})
		}
		return nil
	})

	// Load all changes directly into official memory (bypassing temporary storage)
	results := make([]map[string]interface{}, 0)
	successfullyLoaded := make([]map[string]string, 0)

	for _, change := range changes {
		componentType := change["type"].(string)
		id := change["id"].(string)
		content := change["file_content"].(string)

		success := true
		message := "loaded successfully"

		// Load directly into official component storage
		err := loadComponentDirectly(componentType, id, content)
		if err != nil {
			success = false
			message = "failed to load component: " + err.Error()
			// Record failed operation
			RecordLocalPush(componentType, id, content, "failed", err.Error())
		} else {
			// Record successful operation
			RecordLocalPush(componentType, id, content, "success", "")

			// Track successfully loaded components for project restart
			if componentType != "project" {
				successfullyLoaded = append(successfullyLoaded, map[string]string{
					"type": componentType,
					"id":   id,
				})
			}
		}

		results = append(results, map[string]interface{}{
			"type":    componentType,
			"id":      id,
			"success": success,
			"message": message,
		})
	}

	// Collect all affected projects for successfully loaded components
	for _, component := range successfullyLoaded {
		affectedProjects := project.GetAffectedProjects(component["type"], component["id"])
		for _, projectID := range affectedProjects {
			if p, ok := project.GetProject(projectID); ok {
				err := p.Restart(true, "local_change")
				if err != nil {
					logger.Error("Failed to restart project after component change", "project_id", projectID, "error", err)
				}
				if err := cluster.GlobalInstructionManager.PublishProjectRestart(projectID); err != nil {
					logger.Error("Failed to publish project restart instructions", "affected_projects", projectID, "error", err)
				}
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"results": results,
		"total":   len(results),
	})
}

// loadSingleLocalChange loads a single local change into memory
func loadSingleLocalChange(c echo.Context) error {
	var req struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.ID == "" || req.Type == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "id and type are required"})
	}

	configRoot := common.Config.ConfigRoot
	var filePath string

	// Determine file path based on component type
	switch req.Type {
	case "input":
		filePath = filepath.Join(configRoot, "input", req.ID+".yaml")
	case "output":
		filePath = filepath.Join(configRoot, "output", req.ID+".yaml")
	case "ruleset":
		filePath = filepath.Join(configRoot, "ruleset", req.ID+".xml")
	case "project":
		filePath = filepath.Join(configRoot, "project", req.ID+".yaml")
	case "plugin":
		filePath = filepath.Join(configRoot, "plugin", req.ID+".go")
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "unsupported component type"})
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "file not found"})
	}

	// Read file content
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read file: " + err.Error()})
	}

	content := string(fileContent)

	// Load directly into official component storage
	err = loadComponentDirectly(req.Type, req.ID, content)
	if err != nil {
		// Record failed operation
		RecordLocalPush(req.Type, req.ID, content, "failed", err.Error())
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load component: " + err.Error()})
	}

	// Record successful operation
	RecordLocalPush(req.Type, req.ID, content, "success", "")

	affectedProjects := project.GetAffectedProjects(req.Type, req.ID)

	for _, projectID := range affectedProjects {
		if p, ok := project.GetProject(projectID); ok {
			err := p.Restart(true, "local_change")
			if err != nil {
				logger.Error("Failed to restart project after component change", "project_id", projectID, "error", err)
			}
			if err := cluster.GlobalInstructionManager.PublishProjectRestart(projectID); err != nil {
				logger.Error("Failed to publish project restart instructions", "affected_projects", projectID, "error", err)
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":   true,
		"message":   "loaded successfully",
		"type":      req.Type,
		"id":        req.ID,
		"file_path": filePath,
		"file_size": len(fileContent),
	})
}

// loadComponentDirectly loads a component directly into official storage using unified reload logic
// This bypasses the temporary file system and *New mappings
func loadComponentDirectly(componentType, id, content string) error {
	// Use unified reload logic with local file source
	_, err := reloadComponentUnified(&ComponentReloadRequest{
		Type:        componentType,
		ID:          id,
		NewContent:  content,
		OldContent:  "", // Local file changes don't track old content
		Source:      SourceLocalFile,
		SkipVerify:  false, // Local file changes should be verified
		WriteToFile: false, // Local file changes read from file, don't write
	})
	return err
}
