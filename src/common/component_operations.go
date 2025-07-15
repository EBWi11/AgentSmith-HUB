package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ComponentOperations provides unified component operations for both API and cluster
type ComponentOperations struct{}

var GlobalComponentOperations = &ComponentOperations{}

// CreateComponentDirect creates a component directly without HTTP context (for API/leader)
func (co *ComponentOperations) CreateComponentDirect(componentType, id, content string) error {
	// Enhanced ID validation
	if id == "" || strings.TrimSpace(id) == "" {
		return fmt.Errorf("id cannot be empty")
	}

	// Normalize ID by trimming spaces
	id = strings.TrimSpace(id)

	// Determine file path and extension
	var suffix string
	var dir string

	switch componentType {
	case "ruleset":
		suffix = ".xml"
		dir = "ruleset"
	case "input":
		suffix = ".yaml"
		dir = "input"
	case "output":
		suffix = ".yaml"
		dir = "output"
	case "project":
		suffix = ".yaml"
		dir = "project"
	case "plugin":
		suffix = ".go"
		dir = "plugin"
	default:
		return fmt.Errorf("invalid component type")
	}

	configRoot := Config.ConfigRoot
	dirPath := filepath.Join(configRoot, dir)
	filePath := filepath.Join(dirPath, id+suffix)

	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		return fmt.Errorf("component already exists")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dirPath, err)
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	// Update global configuration
	SetRawConfig(componentType, id, content)

	return nil
}

// CreateComponentMemoryOnly creates a component in memory only (for follower)
func (co *ComponentOperations) CreateComponentMemoryOnly(componentType, id, content string) error {
	// Enhanced ID validation
	if id == "" || strings.TrimSpace(id) == "" {
		return fmt.Errorf("id cannot be empty")
	}

	// Normalize ID by trimming spaces
	id = strings.TrimSpace(id)

	// Update global configuration only (no file operations)
	SetRawConfig(componentType, id, content)

	return nil
}

// UpdateComponentDirect updates a component directly without HTTP context (for API/leader)
func (co *ComponentOperations) UpdateComponentDirect(componentType, id, content string) error {
	// Determine file path and extension
	var suffix string
	var dir string

	switch componentType {
	case "ruleset":
		suffix = ".xml"
		dir = "ruleset"
	case "input":
		suffix = ".yaml"
		dir = "input"
	case "output":
		suffix = ".yaml"
		dir = "output"
	case "project":
		suffix = ".yaml"
		dir = "project"
	case "plugin":
		suffix = ".go"
		dir = "plugin"
	default:
		return fmt.Errorf("invalid component type")
	}

	configRoot := Config.ConfigRoot
	filePath := filepath.Join(configRoot, dir, id+suffix)

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("component not found")
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	// Update global configuration
	SetRawConfig(componentType, id, content)

	return nil
}

// UpdateComponentMemoryOnly updates a component in memory only (for follower)
func (co *ComponentOperations) UpdateComponentMemoryOnly(componentType, id, content string) error {
	// Update global configuration only (no file operations)
	SetRawConfig(componentType, id, content)

	return nil
}

// DeleteComponentDirect deletes a component directly without HTTP context (for API/leader)
func (co *ComponentOperations) DeleteComponentDirect(componentType, id string) error {
	// Determine file path and extension
	var suffix string
	var dir string

	switch componentType {
	case "ruleset":
		suffix = ".xml"
		dir = "ruleset"
	case "input":
		suffix = ".yaml"
		dir = "input"
	case "output":
		suffix = ".yaml"
		dir = "output"
	case "project":
		suffix = ".yaml"
		dir = "project"
	case "plugin":
		suffix = ".go"
		dir = "plugin"
	default:
		return fmt.Errorf("invalid component type")
	}

	configRoot := Config.ConfigRoot
	filePath := filepath.Join(configRoot, dir, id+suffix)

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("component not found")
	}

	// Delete file
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", filePath, err)
	}

	// Remove from global configuration
	DeleteRawConfig(componentType, id)

	return nil
}

// DeleteComponentMemoryOnly deletes a component from memory only (for follower)
func (co *ComponentOperations) DeleteComponentMemoryOnly(componentType, id string) error {
	// Remove from global configuration only (no file operations)
	DeleteRawConfig(componentType, id)

	return nil
}
