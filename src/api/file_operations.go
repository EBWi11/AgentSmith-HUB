package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/plugin"
	"AgentSmith-HUB/project"
)

// FileOperations handles all file system operations for components
// This includes reading, writing, and managing both formal and temporary files

// Constants for file extensions
const (
	RULESET_EXT     = ".xml"
	RULESET_EXT_NEW = ".xml.new"

	PLUGIN_EXT     = ".go"
	PLUGIN_EXT_NEW = ".go.new"

	EXT     = ".yaml"
	EXT_NEW = ".yaml.new"
)

// GetExt returns the appropriate file extension for a component type
// Parameters:
//   - componentType: type of component (input, output, ruleset, project, plugin)
//   - new: true for temporary files (.new), false for formal files
//
// Returns the file extension
func GetExt(componentType string, new bool) string {
	var ext string
	switch componentType {
	case "ruleset":
		if new {
			ext = RULESET_EXT_NEW
		} else {
			ext = RULESET_EXT
		}
	case "plugin":
		if new {
			ext = PLUGIN_EXT_NEW
		} else {
			ext = PLUGIN_EXT
		}
	default: // input, output, project
		if new {
			ext = EXT_NEW
		} else {
			ext = EXT
		}
	}
	return ext
}

// GetComponentPath returns the full file path for a component
// It also creates the directory if it doesn't exist
// Parameters:
//   - componentType: type of component (input, output, ruleset, project, plugin)
//   - id: component identifier
//   - new: true for temporary files, false for formal files
//
// Returns:
//   - filePath: full path to the component file
//   - exists: whether the file already exists
func GetComponentPath(componentType string, id string, new bool) (string, bool) {
	dirPath := filepath.Join(common.Config.ConfigRoot, componentType)
	filePath := filepath.Join(dirPath, id+GetExt(componentType, new))

	// Check if directory exists, create if not
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			logger.Error("failed to create component directory", "path", dirPath, "error", err)
			return filePath, false
		}
	}

	// Check if file exists
	_, err := os.Stat(filePath)
	exists := !os.IsNotExist(err)

	return filePath, exists
}

// WriteComponentFile writes content to a component file
// It also updates the in-memory cache for temporary files
// Parameters:
//   - path: full file path to write to
//   - content: content to write
//
// Returns error if write fails
func WriteComponentFile(filePath string, content string) error {
	// Write to file system
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	// Update in-memory cache for temporary files
	if strings.HasSuffix(filePath, ".new") {
		updateInMemoryCache(filePath, content)
	}

	return nil
}

func writeFileAtomically(filePath string, content []byte, mode os.FileMode) (err error) {
	tempFile, err := os.CreateTemp(filepath.Dir(filePath), "."+filepath.Base(filePath)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()

	if err = tempFile.Chmod(mode); err != nil {
		return err
	}
	if _, err = tempFile.Write(content); err != nil {
		return err
	}
	if err = tempFile.Sync(); err != nil {
		return err
	}
	if err = tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, filePath)
}

// updateInMemoryCache extracts component type and ID from file path and updates the memory cache
func updateInMemoryCache(filePath string, content string) {
	componentType, id := extractComponentInfo(filePath)
	if componentType == "" || id == "" {
		return
	}

	// Update in-memory copy based on component type
	switch componentType {
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
	case "agent":
		project.SetAgentNew(id, content)
	case "skill":
		project.SetSkillNew(id, content)
	}
}

// extractComponentInfo extracts component type and ID from a file path
// Returns (componentType, componentId)
func extractComponentInfo(filePath string) (string, string) {
	// Use filepath to get the directory and filename (cross-platform safe)
	basePath := filepath.Dir(filePath)
	filename := filepath.Base(filePath)

	// Get the component type from the directory name
	// The path structure is: ConfigRoot/componentType/filename
	// or: ConfigRoot/componentType/folder/filename (for rulesets with folders)
	componentType := filepath.Base(basePath)

	// If the directory is a subfolder of a component type (e.g., ruleset subfolder),
	// we need to go up one more level to get the actual component type
	configRoot := common.Config.ConfigRoot
	parentDir := filepath.Dir(basePath)
	parentDirName := filepath.Base(parentDir)

	// Check if the parent directory is a known component type directory
	knownTypes := map[string]bool{"input": true, "output": true, "ruleset": true, "project": true, "plugin": true, "agent": true, "skill": true}
	if knownTypes[parentDirName] {
		// This is a subfolder path: ConfigRoot/componentType/subfolder/filename
		componentType = parentDirName
	} else if !knownTypes[componentType] {
		// Try relative path from config root
		relPath, err := filepath.Rel(configRoot, filePath)
		if err == nil {
			parts := strings.SplitN(relPath, string(filepath.Separator), 3)
			if len(parts) >= 2 && knownTypes[parts[0]] {
				componentType = parts[0]
			}
		}
	}

	// Determine ID based on file extension
	var id string
	switch {
	case strings.HasSuffix(filename, ".xml.new"):
		id = filename[:len(filename)-8] // Remove .xml.new
	case strings.HasSuffix(filename, ".go.new"):
		id = filename[:len(filename)-7] // Remove .go.new
	case strings.HasSuffix(filename, ".yaml.new"):
		id = filename[:len(filename)-9] // Remove .yaml.new
	default:
		return "", ""
	}

	return componentType, id
}

// ReadComponent reads content from a component file
// Parameters:
//   - path: full file path to read from
//
// Returns:
//   - content: file content as string
//   - error: any error encountered during read
func ReadComponent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
