package mcp

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"AgentSmith-HUB/common"

	"github.com/labstack/echo/v4"
)

// loadMCPConfigFile loads a JSON configuration file from mcp_config directory
// It tries ./mcp_config first, then ../mcp_config as fallback
func loadMCPConfigFile(filename string) ([]byte, error) {
	// Try current directory first
	currentPath := filepath.Join("mcp_config", filename)
	if data, err := ioutil.ReadFile(currentPath); err == nil {
		return data, nil
	}

	// Fallback to parent directory
	parentPath := filepath.Join("../mcp_config", filename)
	if data, err := ioutil.ReadFile(parentPath); err == nil {
		return data, nil
	}

	return nil, fmt.Errorf("config file %s not found in ./mcp_config or ../mcp_config", filename)
}

// LoadMCPPrompts loads all MCP prompts from configuration
func LoadMCPPrompts() ([]common.MCPPrompt, error) {
	data, err := loadMCPConfigFile("prompts.json")
	if err != nil {
		return nil, fmt.Errorf("failed to load prompts config: %w", err)
	}

	var promptFile struct {
		Prompts []common.MCPPrompt `json:"prompts"`
	}
	if err := json.Unmarshal(data, &promptFile); err != nil {
		return nil, fmt.Errorf("failed to parse prompts file: %w", err)
	}

	return promptFile.Prompts, nil
}

// GetMCPPrompt gets a specific prompt by name
func GetMCPPrompt(name string) (*common.MCPPrompt, error) {
	prompts, err := LoadMCPPrompts()
	if err != nil {
		return nil, err
	}

	for _, prompt := range prompts {
		if prompt.Name == name {
			return &prompt, nil
		}
	}

	return nil, fmt.Errorf("prompt '%s' not found", name)
}

// GetRulesetTemplates provides comprehensive ruleset templates covering all syntax combinations
func GetRulesetTemplates(c echo.Context) error {
	// Load templates from JSON file
	data, err := loadMCPConfigFile("ruleset_templates.json")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to load ruleset templates: " + err.Error(),
		})
	}

	var templates map[string]interface{}
	if err := json.Unmarshal(data, &templates); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to parse ruleset templates: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, templates)
}

// GetRulesetSyntaxGuide provides comprehensive syntax documentation based on actual validation logic
func GetRulesetSyntaxGuide(c echo.Context) error {
	// Load syntax guide from JSON file
	data, err := loadMCPConfigFile("syntax_guide.json")
	if err != nil {
		// If syntax_guide.json doesn't exist, try loading from rule_templates.json
		data, err = loadMCPConfigFile("rule_templates.json")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"error": "Failed to load syntax guide: " + err.Error(),
			})
		}

		// Extract syntax reference from rule_templates.json
		var templates map[string]interface{}
		if err := json.Unmarshal(data, &templates); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{
				"error": "Failed to parse rule templates for syntax guide: " + err.Error(),
			})
		}

		// Look for SYNTAX_REFERENCE section in rule templates
		if ruleTemplates, ok := templates["RULE_TEMPLATES"].(map[string]interface{}); ok {
			if syntaxRef, ok := ruleTemplates["SYNTAX_REFERENCE"].(map[string]interface{}); ok {
				return c.JSON(http.StatusOK, map[string]interface{}{
					"SYNTAX_REFERENCE": syntaxRef,
					"source":           "rule_templates.json",
					"note":             "Syntax guide extracted from rule templates",
				})
			}
		}

		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "No syntax reference found in available configuration files",
		})
	}

	var guide map[string]interface{}
	if err := json.Unmarshal(data, &guide); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to parse syntax guide: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, guide)
}

// GetRuleTemplates provides comprehensive templates for individual rules covering all node types
func GetRuleTemplates(c echo.Context) error {
	// Load templates from JSON file
	data, err := loadMCPConfigFile("rule_templates.json")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to load rule templates: " + err.Error(),
		})
	}

	var templates map[string]interface{}
	if err := json.Unmarshal(data, &templates); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to parse rule templates: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, templates)
}

// GetMCPPrompts loads the main MCP prompts configuration (HTTP handler)
func GetMCPPrompts(c echo.Context) error {
	// Load prompts from JSON file
	data, err := loadMCPConfigFile("prompts.json")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to load MCP prompts: " + err.Error(),
		})
	}

	var prompts map[string]interface{}
	if err := json.Unmarshal(data, &prompts); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to parse MCP prompts: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, prompts)
}

// GetAllMCPConfigs returns all MCP configuration files in one response
func GetAllMCPConfigs(c echo.Context) error {
	configs := map[string]interface{}{}

	// Load rule templates
	if data, err := loadMCPConfigFile("rule_templates.json"); err == nil {
		var ruleTemplates map[string]interface{}
		if err := json.Unmarshal(data, &ruleTemplates); err == nil {
			configs["rule_templates"] = ruleTemplates
		}
	}

	// Load ruleset templates
	if data, err := loadMCPConfigFile("ruleset_templates.json"); err == nil {
		var rulesetTemplates map[string]interface{}
		if err := json.Unmarshal(data, &rulesetTemplates); err == nil {
			configs["ruleset_templates"] = rulesetTemplates
		}
	}

	// Load prompts
	if data, err := loadMCPConfigFile("prompts.json"); err == nil {
		var prompts map[string]interface{}
		if err := json.Unmarshal(data, &prompts); err == nil {
			configs["prompts"] = prompts
		}
	}

	if len(configs) == 0 {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to load any MCP configuration files",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"configs":     configs,
		"loaded_at":   fmt.Sprintf("%d", len(configs)),
		"description": "All available MCP configuration files",
	})
}
