package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/project"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func getProjectStructure(p *project.Project) (map[string]interface{}, error) {
	// Build nodes and edges for visualization
	nodes := []map[string]interface{}{}
	edges := []map[string]interface{}{}

	// Try to get flow graph from project content
	var flowGraph map[string][]string
	var err error

	if p.Config != nil && p.Config.Content != "" {
		// Parse from Config.Content if available
		flowGraph = make(map[string][]string)
		lines := strings.Split(p.Config.Content, "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.Split(line, "->")
			if len(parts) != 2 {
				continue
			}

			from := strings.TrimSpace(parts[0])
			to := strings.TrimSpace(parts[1])

			// Add to flow graph
			flowGraph[from] = append(flowGraph[from], to)
		}
	} else {
		// Fallback: try to parse from project's parseContent method
		flowGraph, err = p.ParseContentForVisualization()
		if err != nil {
			// If parsing fails, create a simple flow graph from existing components
			flowGraph = make(map[string][]string)
			logger.Warn("Failed to parse project content for visualization, using fallback", "project", p.Id, "error", err)
		}
	}

	// Add nodes from actual project components
	for _, input := range p.Inputs {
		nodes = append(nodes, map[string]interface{}{
			"id":   "input." + input.Id,
			"type": "input",
			"name": input.Id,
		})
	}

	for _, ruleset := range p.Rulesets {
		nodes = append(nodes, map[string]interface{}{
			"id":   "ruleset." + ruleset.RulesetID,
			"type": "ruleset",
			"name": ruleset.RulesetID,
		})
	}

	for _, output := range p.Outputs {
		nodes = append(nodes, map[string]interface{}{
			"id":   "output." + output.Id,
			"type": "output",
			"name": output.Id,
		})
	}

	// Add edges from flow graph
	for from, tos := range flowGraph {
		for _, to := range tos {
			edges = append(edges, map[string]interface{}{
				"from": from,
				"to":   to,
			})
		}
	}

	return map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
	}, nil
}

func ServerStart(listener string) error {
	e := echo.New()
	e.HideBanner = true

	// Add CORS middleware with more permissive configuration
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"*"},          // Allow all origins
		AllowHeaders:     []string{"*", "token"}, // Allow all headers and explicitly allow token
		AllowMethods:     []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowCredentials: true,                       // Allow credentials
		ExposeHeaders:    []string{"Content-Length"}, // Expose these headers
		MaxAge:           86400,                      // Cache preflight requests for 24 hours
	}))

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Global authentication middleware (skip for health check and token check)
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Path() == "/ping" || c.Path() == "/token-check" {
				return next(c)
			}

			token := c.Request().Header.Get("token")
			if token == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "missing token",
				})
			}

			if token != common.Config.Token {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "authentication failed",
				})
			}

			return next(c)
		}
	})

	// Health check and token verification
	e.GET("/ping", ping)
	e.GET("/token-check", tokenCheck)

	// Project endpoints (use plural form for consistency)
	e.GET("/projects", getProjects)
	e.GET("/projects/:id", getProject)
	e.POST("/projects", createProject)
	e.DELETE("/projects/:id", deleteProject)
	e.PUT("/projects/:id", updateProject)
	e.POST("/start-project", StartProject)
	e.POST("/stop-project", StopProject)
	e.POST("/restart-project", RestartProject)
	e.POST("/restart-all-projects", RestartAllProjects)
	e.GET("/project-error/:id", getProjectError)
	e.GET("/project-inputs/:id", getProjectInputs)

	// Ruleset endpoints (use plural form for consistency)
	e.GET("/rulesets", getRulesets)
	e.GET("/rulesets/:id", getRuleset)
	e.POST("/rulesets", createRuleset)
	e.PUT("/rulesets/:id", updateRuleset)
	e.DELETE("/rulesets/:id", deleteRuleset)

	// Input endpoints (use plural form for consistency)
	e.GET("/inputs", getInputs)
	e.GET("/inputs/:id", getInput)
	e.POST("/inputs", createInput)
	e.PUT("/inputs/:id", updateInput)
	e.DELETE("/inputs/:id", deleteInput)

	// Output endpoints (use plural form for consistency)
	e.GET("/outputs", getOutputs)
	e.GET("/outputs/:id", getOutput)
	e.POST("/outputs", createOutput)
	e.PUT("/outputs/:id", updateOutput)
	e.DELETE("/outputs/:id", deleteOutput)

	// Plugin endpoints (use plural form and :id for consistency)
	e.GET("/plugins", getPlugins)
	e.GET("/plugins/:id", getPlugin)
	e.POST("/plugins", createPlugin)
	e.PUT("/plugins/:id", updatePlugin)
	e.DELETE("/plugins/:id", deletePlugin)
	e.GET("/available-plugins", getAvailablePlugins)

	// Component verification and testing
	e.POST("/verify/:type/:id", verifyComponent)
	e.GET("/connect-check/:type/:id", connectCheck)
	e.POST("/test-plugin/:id", testPlugin)
	e.POST("/test-ruleset/:id", testRuleset)
	e.POST("/test-ruleset-content", testRulesetContent)
	e.POST("/test-output/:id", testOutput)
	e.POST("/test-project/:id", testProject)

	// Metrics endpoints
	e.GET("/metrics", getMetrics)
	e.GET("/metrics/project/:id", getProjectMetrics)
	e.GET("/metrics/redis", getRedisMetrics)

	// Cluster endpoints
	e.GET("/cluster", getCluster)
	e.GET("/cluster-status", getClusterStatus)
	e.POST("/cluster/heartbeat", handleHeartbeat)
	e.POST("/component-sync", handleComponentSync)
	e.GET("/config_root", leaderConfig)
	e.GET("/download-config", downloadConfig)

	// Pending changes management (enhanced)
	e.GET("/pending-changes", GetPendingChanges)                   // Legacy endpoint
	e.GET("/pending-changes/enhanced", GetEnhancedPendingChanges)  // Enhanced endpoint with status info
	e.POST("/apply-changes", ApplyPendingChanges)                  // Legacy endpoint
	e.POST("/apply-changes/enhanced", ApplyPendingChangesEnhanced) // Enhanced endpoint with transaction support
	e.POST("/apply-single-change", ApplySingleChange)              // Legacy endpoint
	e.POST("/verify-changes", VerifyPendingChanges)                // Verify all changes
	e.POST("/verify-change/:type/:id", VerifySinglePendingChange)  // Verify single change
	e.DELETE("/cancel-change/:type/:id", CancelPendingChange)      // Cancel single change
	e.DELETE("/cancel-all-changes", CancelAllPendingChanges)       // Cancel all changes

	// Temporary file management
	e.POST("/temp-file/:type/:id", CreateTempFile)
	e.GET("/temp-file/:type/:id", CheckTempFile)
	e.DELETE("/temp-file/:type/:id", DeleteTempFile)

	// Sampler endpoints
	e.GET("/samplers/data", GetSamplerData)
	e.GET("/samplers/stats", GetSamplerStats)
	e.POST("/samplers/reset", ResetSampler)

	// Cancel upgrade routes
	e.POST("/cancel-upgrade/rulesets/:id", cancelRulesetUpgrade)
	e.POST("/cancel-upgrade/inputs/:id", cancelInputUpgrade)
	e.POST("/cancel-upgrade/outputs/:id", cancelOutputUpgrade)
	e.POST("/cancel-upgrade/projects/:id", cancelProjectUpgrade)
	e.POST("/cancel-upgrade/plugins/:id", cancelPluginUpgrade)

	// Component usage analysis
	e.GET("/component-usage/:type/:id", GetComponentUsage)

	// Load local components routes
	e.GET("/local-changes", getLocalChanges)
	e.POST("/load-local-changes", loadLocalChanges)
	e.POST("/load-single-local-change", loadSingleLocalChange)

	if err := e.Start(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
