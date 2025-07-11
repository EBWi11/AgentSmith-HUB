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

	// Lock for reading memory state
	common.GlobalMu.RLock()
	defer common.GlobalMu.RUnlock()

	// Check inputs
	inputDir := filepath.Join(configRoot, "input")
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
		memoryInput, exists := project.GlobalProject.Inputs[id]
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

	// Check outputs
	outputDir := filepath.Join(configRoot, "output")
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

		memoryOutput, exists := project.GlobalProject.Outputs[id]
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

	// Check rulesets
	rulesetDir := filepath.Join(configRoot, "ruleset")
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

		memoryRuleset, exists := project.GlobalProject.Rulesets[id]
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

	// Check projects
	projectDir := filepath.Join(configRoot, "project")
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

		memoryProject, exists := project.GlobalProject.Projects[id]
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

	// Check plugins
	pluginDir := filepath.Join(configRoot, "plugin")
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

	// Check for components that exist in memory but not in local files (deleted locally)
	// configRoot is already defined above

	// Check for deleted inputs
	for id, input := range project.GlobalProject.Inputs {
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
	}

	// Check for deleted outputs
	for id, output := range project.GlobalProject.Outputs {
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
	}

	// Check for deleted rulesets
	for id, ruleset := range project.GlobalProject.Rulesets {
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
	}

	// Check for deleted projects
	for id, proj := range project.GlobalProject.Projects {
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
	}

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

		memoryInput, exists := project.GlobalProject.Inputs[id]
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

		memoryOutput, exists := project.GlobalProject.Outputs[id]
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

		memoryRuleset, exists := project.GlobalProject.Rulesets[id]
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

		memoryProject, exists := project.GlobalProject.Projects[id]
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
		}

		results = append(results, map[string]interface{}{
			"type":    componentType,
			"id":      id,
			"success": success,
			"message": message,
		})
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

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":   true,
		"message":   "loaded successfully",
		"type":      req.Type,
		"id":        req.ID,
		"file_path": filePath,
		"file_size": len(fileContent),
	})
}

// loadComponentDirectly loads a component directly into official storage
// This bypasses the temporary file system and *New mappings
func loadComponentDirectly(componentType, id, content string) error {
	switch componentType {
	case "input":
		// Check if old component exists and count projects using it (using centralized counter)
		common.GlobalMu.RLock()
		oldInput, exists := project.GlobalProject.Inputs[id]
		common.GlobalMu.RUnlock()

		var projectsUsingInput int
		if exists {
			projectsUsingInput = project.UsageCounter.CountProjectsUsingInput(id)

			// Only stop old component if no running projects are using it
			if projectsUsingInput == 0 {
				logger.Info("Stopping old input component for direct load", "id", id, "projects_using", projectsUsingInput)
				oldInput.Stop()
			} else {
				logger.Info("Input component still in use, skipping stop during direct load", "id", id, "projects_using", projectsUsingInput)
			}
		}

		// Use the existing NewInput constructor
		inputInstance, err := input.NewInput("", content, id)
		if err != nil {
			return fmt.Errorf("failed to create input: %w", err)
		}

		// Store directly in official storage with proper locking
		common.GlobalMu.Lock()
		// Ensure global inputs map exists
		if project.GlobalProject.Inputs == nil {
			project.GlobalProject.Inputs = make(map[string]*input.Input)
		}
		project.GlobalProject.Inputs[id] = inputInstance
		// Clear any temporary version to avoid confusion
		delete(project.GlobalProject.InputsNew, id)
		common.GlobalMu.Unlock()

		// Start the new input component if no projects are currently using it
		// (running projects will start it when needed)
		if projectsUsingInput == 0 {
			logger.Info("Starting new input component after direct load", "id", id)
			if err := inputInstance.Start(); err != nil {
				logger.Error("Failed to start new input component after direct load", "id", id, "error", err)
				return fmt.Errorf("failed to start new input component: %w", err)
			}
		}

	case "output":
		// Check if old component exists and count projects using it (using centralized counter)
		common.GlobalMu.RLock()
		oldOutput, exists := project.GlobalProject.Outputs[id]
		common.GlobalMu.RUnlock()

		var projectsUsingOutput int
		if exists {
			projectsUsingOutput = project.UsageCounter.CountProjectsUsingOutput(id)

			// Only stop old component if no running projects are using it
			if projectsUsingOutput == 0 {
				logger.Info("Stopping old output component for direct load", "id", id, "projects_using", projectsUsingOutput)
				oldOutput.Stop()
			} else {
				logger.Info("Output component still in use, skipping stop during direct load", "id", id, "projects_using", projectsUsingOutput)
			}
		}

		// Use the existing NewOutput constructor
		outputInstance, err := output.NewOutput("", content, id)
		if err != nil {
			return fmt.Errorf("failed to create output: %w", err)
		}

		// Store directly in official storage with proper locking
		common.GlobalMu.Lock()
		// Ensure global outputs map exists
		if project.GlobalProject.Outputs == nil {
			project.GlobalProject.Outputs = make(map[string]*output.Output)
		}
		project.GlobalProject.Outputs[id] = outputInstance
		// Clear any temporary version to avoid confusion
		delete(project.GlobalProject.OutputsNew, id)
		common.GlobalMu.Unlock()

		// Start the new output component if no projects are currently using it
		// (running projects will start it when needed)
		if projectsUsingOutput == 0 {
			logger.Info("Starting new output component after direct load", "id", id)
			if err := outputInstance.Start(); err != nil {
				logger.Error("Failed to start new output component after direct load", "id", id, "error", err)
				return fmt.Errorf("failed to start new output component: %w", err)
			}
		}

	case "ruleset":
		// Check if old component exists and count projects using it (using centralized counter)
		common.GlobalMu.RLock()
		oldRuleset, exists := project.GlobalProject.Rulesets[id]
		common.GlobalMu.RUnlock()

		var projectsUsingRuleset int
		if exists {
			projectsUsingRuleset = project.UsageCounter.CountProjectsUsingRuleset(id)

			// Only stop old component if no running projects are using it
			if projectsUsingRuleset == 0 {
				logger.Info("Stopping old ruleset component for direct load", "id", id, "projects_using", projectsUsingRuleset)
				oldRuleset.Stop()
			} else {
				logger.Info("Ruleset component still in use, skipping stop during direct load", "id", id, "projects_using", projectsUsingRuleset)
			}
		}

		// Use the existing NewRuleset constructor
		rulesetInstance, err := rules_engine.NewRuleset("", content, id)
		if err != nil {
			return fmt.Errorf("failed to create ruleset: %w", err)
		}

		// Store directly in official storage with proper locking
		common.GlobalMu.Lock()
		// Ensure global rulesets map exists
		if project.GlobalProject.Rulesets == nil {
			project.GlobalProject.Rulesets = make(map[string]*rules_engine.Ruleset)
		}
		project.GlobalProject.Rulesets[id] = rulesetInstance
		// Clear any temporary version to avoid confusion
		delete(project.GlobalProject.RulesetsNew, id)
		common.GlobalMu.Unlock()

		// Start the new ruleset component if no projects are currently using it
		// (running projects will start it when needed)
		if projectsUsingRuleset == 0 {
			logger.Info("Starting new ruleset component after direct load", "id", id)
			if err := rulesetInstance.Start(); err != nil {
				logger.Error("Failed to start new ruleset component after direct load", "id", id, "error", err)
				return fmt.Errorf("failed to start new ruleset component: %w", err)
			}
		}

	case "project":
		// Stop old project if it exists (projects are not shared between other projects)
		common.GlobalMu.RLock()
		oldProject, exists := project.GlobalProject.Projects[id]
		common.GlobalMu.RUnlock()

		if exists {
			logger.Info("Stopping old project for direct load", "id", id)
			oldProject.Stop()
		}

		// Use the existing NewProject constructor
		projectInstance, err := project.NewProject("", content, id)
		if err != nil {
			return fmt.Errorf("failed to create project: %w", err)
		}

		// Store directly in official storage with proper locking
		common.GlobalMu.Lock()
		// Ensure global projects map exists
		if project.GlobalProject.Projects == nil {
			project.GlobalProject.Projects = make(map[string]*project.Project)
		}
		project.GlobalProject.Projects[id] = projectInstance
		// Clear any temporary version to avoid confusion
		delete(project.GlobalProject.ProjectsNew, id)
		common.GlobalMu.Unlock()

	case "plugin":
		// Remove existing plugin from memory before loading new one
		common.GlobalMu.Lock()
		delete(plugin.Plugins, id)
		common.GlobalMu.Unlock()

		// Use the existing NewPlugin constructor to properly compile and initialize the plugin
		err := plugin.NewPlugin("", content, id, plugin.YAEGI_PLUGIN)
		if err != nil {
			return fmt.Errorf("failed to create plugin: %w", err)
		}

		// Plugin is automatically added to plugin.Plugins by NewPlugin function
		// No need to manually add it to the map

		// Clear any temporary version to avoid confusion
		common.GlobalMu.Lock()
		delete(plugin.PluginsNew, id)
		common.GlobalMu.Unlock()

	default:
		return fmt.Errorf("unsupported component type: %s", componentType)
	}

	// Update global component config maps for follower access (if leader)
	if cluster.IsLeader {
		updateGlobalComponentConfigMapForLocalLoad(componentType, id, content)

		// Sync to followers - components loaded directly should be synchronized immediately
		// since they bypass the temporary file system
		syncComponentToFollowersForLocalLoad(componentType, id)
	}

	return nil
}

// updateGlobalComponentConfigMapForLocalLoad updates the global component config map for local loads
func updateGlobalComponentConfigMapForLocalLoad(componentType, id, content string) {
	common.GlobalMu.Lock()
	defer common.GlobalMu.Unlock()

	// Initialize maps if they are nil
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

	// Update the appropriate global config map
	switch componentType {
	case "input":
		common.AllInputsRawConfig[id] = content
	case "output":
		common.AllOutputsRawConfig[id] = content
	case "ruleset":
		common.AllRulesetsRawConfig[id] = content
	case "project":
		common.AllProjectRawConfig[id] = content
	case "plugin":
		common.AllPluginsRawConfig[id] = content
	}

	logger.Debug("Updated global component config map for local load", "type", componentType, "id", id)
}

// syncComponentToFollowersForLocalLoad syncs a locally loaded component to follower nodes
func syncComponentToFollowersForLocalLoad(componentType string, id string) {
	// Prepare sync data using the content from the loaded component
	var content string
	var isRunning bool

	common.GlobalMu.RLock()
	switch componentType {
	case "input":
		if input, exists := project.GlobalProject.Inputs[id]; exists {
			content = input.Config.RawConfig
		}
	case "output":
		if output, exists := project.GlobalProject.Outputs[id]; exists {
			content = output.Config.RawConfig
		}
	case "ruleset":
		if ruleset, exists := project.GlobalProject.Rulesets[id]; exists {
			content = ruleset.RawConfig
		}
	case "project":
		if proj, exists := project.GlobalProject.Projects[id]; exists {
			content = proj.Config.RawConfig
			isRunning = (proj.Status == project.ProjectStatusRunning)
		}
	case "plugin":
		if plug, exists := plugin.Plugins[id]; exists && plug.Type == plugin.YAEGI_PLUGIN {
			content = string(plug.Payload)
		}
	}
	common.GlobalMu.RUnlock()

	if content == "" {
		logger.Error("Failed to get content for local load sync", "type", componentType, "id", id)
		return
	}

	// Publish instruction for component update
	if componentType == "project" {
		if err := cluster.GlobalInstructionManager.PublishComponentLocalPush("project", id, content, nil); err != nil {
			logger.Error("Failed to publish project local push instruction", "project", id, "error", err)
		}
		if isRunning {
			if err := cluster.GlobalInstructionManager.PublishProjectStart(id); err != nil {
				logger.Error("Failed to publish project start instruction", "project", id, "error", err)
			}
		}
	} else {
		// For other components, publish local push instruction
		affectedProjects := project.GetAffectedProjects(componentType, id)
		if err := cluster.GlobalInstructionManager.PublishComponentLocalPush(componentType, id, content, affectedProjects); err != nil {
			logger.Error("Failed to publish component local push instruction", "type", componentType, "id", id, "error", err)
		}

		// Restart affected projects
		if len(affectedProjects) > 0 {
			if err := cluster.GlobalInstructionManager.PublishProjectsRestart(affectedProjects, "component_local_push"); err != nil {
				logger.Error("Failed to publish project restart instructions", "affected_projects", affectedProjects, "error", err)
			}
		}
	}

	logger.Info("Published local load component sync event via Redis",
		"type", componentType,
		"id", id,
	)
}
