package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/project"
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

	// Skip plugins - they should not be shown in Load Local Components

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

	// Skip deleted plugins - they should not be shown in Load Local Components

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

	// Skip plugins - they should not be loaded via Load Local Components

	// Load all changes into memory and create temp files
	results := make([]map[string]interface{}, 0)

	for _, change := range changes {
		componentType := change["type"].(string)
		id := change["id"].(string)
		content := change["file_content"].(string)

		success := true
		message := "loaded successfully"

		// Lock for memory operations
		common.GlobalMu.Lock()
		// Load into appropriate memory structure
		switch componentType {
		case "input":
			if project.GlobalProject.InputsNew == nil {
				project.GlobalProject.InputsNew = make(map[string]string)
			}
			project.GlobalProject.InputsNew[id] = content
		case "output":
			if project.GlobalProject.OutputsNew == nil {
				project.GlobalProject.OutputsNew = make(map[string]string)
			}
			project.GlobalProject.OutputsNew[id] = content
		case "ruleset":
			if project.GlobalProject.RulesetsNew == nil {
				project.GlobalProject.RulesetsNew = make(map[string]string)
			}
			project.GlobalProject.RulesetsNew[id] = content
		case "project":
			if project.GlobalProject.ProjectsNew == nil {
				project.GlobalProject.ProjectsNew = make(map[string]string)
			}
			project.GlobalProject.ProjectsNew[id] = content
		// Skip plugin loading - plugins should not be loaded via Load Local Components
		default:
			success = false
			message = "unsupported component type"
		}
		common.GlobalMu.Unlock()

		// Create temp file (without holding lock)
		tempPath, _ := GetComponentPath(componentType, id, true)
		_ = WriteComponentFile(tempPath, content)

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
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "plugins cannot be loaded via Load Local Components"})
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

	// Lock for memory operations
	common.GlobalMu.Lock()

	// Load into appropriate memory structure
	switch req.Type {
	case "input":
		if project.GlobalProject.InputsNew == nil {
			project.GlobalProject.InputsNew = make(map[string]string)
		}
		project.GlobalProject.InputsNew[req.ID] = string(fileContent)
	case "output":
		if project.GlobalProject.OutputsNew == nil {
			project.GlobalProject.OutputsNew = make(map[string]string)
		}
		project.GlobalProject.OutputsNew[req.ID] = string(fileContent)
	case "ruleset":
		if project.GlobalProject.RulesetsNew == nil {
			project.GlobalProject.RulesetsNew = make(map[string]string)
		}
		project.GlobalProject.RulesetsNew[req.ID] = string(fileContent)
	case "project":
		if project.GlobalProject.ProjectsNew == nil {
			project.GlobalProject.ProjectsNew = make(map[string]string)
		}
		project.GlobalProject.ProjectsNew[req.ID] = string(fileContent)
		// Skip plugin loading - plugins should not be loaded via Load Local Components
	}

	// Unlock before file operations
	common.GlobalMu.Unlock()

	// Create temp file (without holding lock)
	tempPath, _ := GetComponentPath(req.Type, req.ID, true)
	_ = WriteComponentFile(tempPath, string(fileContent))

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":   true,
		"message":   "loaded successfully",
		"type":      req.Type,
		"id":        req.ID,
		"file_path": filePath,
		"file_size": len(fileContent),
	})
}
