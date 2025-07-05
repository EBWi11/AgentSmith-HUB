package mcp

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"AgentSmith-HUB/common"

	"github.com/labstack/echo/v4"
)

// simple in-memory cache entry
type cachedCfg struct {
	data     []byte
	loadedAt time.Time
}

var cfgCache sync.Map // key: filename -> *cachedCfg

// loadMCPConfigFile loads a JSON configuration file from mcp_config directory
// It tries ./mcp_config first, then ../mcp_config as fallback
func loadMCPConfigFile(filename string) ([]byte, error) {
	// fast path: cached
	if v, ok := cfgCache.Load(filename); ok {
		if entry, ok := v.(*cachedCfg); ok {
			return entry.data, nil
		}
	}

	// Try current directory first
	currentPath := filepath.Join("mcp_config", filename)
	if data, err := ioutil.ReadFile(currentPath); err == nil {
		cfgCache.Store(filename, &cachedCfg{data: data, loadedAt: time.Now()})
		return data, nil
	}

	// Fallback to parent directory
	parentPath := filepath.Join("../mcp_config", filename)
	if data, err := ioutil.ReadFile(parentPath); err == nil {
		cfgCache.Store(filename, &cachedCfg{data: data, loadedAt: time.Now()})
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

// GetMCPConfigs returns requested MCP configuration blocks in one response.
// Query param include=rule,ruleset,prompt,syntax ; empty means all.
func GetMCPConfigs(c echo.Context) error {
	include := c.QueryParam("include")
	wantAll := include == "" || include == "all"
	wants := map[string]bool{}
	if !wantAll {
		for _, p := range strings.Split(include, ",") {
			wants[strings.TrimSpace(strings.ToLower(p))] = true
		}
	}

	resp := make(map[string]interface{})
	var etagBuilder strings.Builder

	// helper closure
	add := func(name, filename string) {
		if !wantAll && !wants[name] {
			return
		}
		if data, err := loadMCPConfigFile(filename); err == nil {
			var v interface{}
			if json.Unmarshal(data, &v) == nil {
				resp[name] = v
				etagBuilder.Write(data)
			}
		}
	}

	add("rule", "rule_templates.json")
	add("ruleset", "ruleset_templates.json")
	add("prompt", "prompts.json")
	add("syntax", "syntax_guide.json")

	if len(resp) == 0 {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "no config found"})
	}

	// Generate ETag
	sum := sha1.Sum([]byte(etagBuilder.String()))
	etag := hex.EncodeToString(sum[:])
	if match := c.Request().Header.Get("If-None-Match"); match != "" && match == etag {
		return c.NoContent(http.StatusNotModified)
	}
	c.Response().Header().Set("ETag", etag)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"version": time.Now().Format(time.RFC3339),
		"data":    resp,
	})
}

// Deprecated: use GetMCPConfigs instead.
func GetAllMCPConfigs(c echo.Context) error {
	return GetMCPConfigs(c)
}
