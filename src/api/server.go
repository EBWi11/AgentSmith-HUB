package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/mcp"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

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

	// Initialize access logger and verify it works
	accessLogWriter := logger.GetAccessLogger()
	if accessLogWriter == nil {
		logger.Error("failed to initialize access logger")
		return errors.New("access logger initialization failed")
	}
	logger.Info("access logger configured successfully")

	// Test access logger to ensure it works
	if err := logger.TestAccessLogger(); err != nil {
		logger.Error("access logger test failed", "error", err)
		return err
	}

	// Configure access logger with custom format and output to access.log
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Output: accessLogWriter,
		Format: `{"time":"${time_rfc3339}","id":"${id}","remote_ip":"${remote_ip}","host":"${host}","method":"${method}","uri":"${uri}","user_agent":"${user_agent}","status":${status},"error":"${error}","latency":${latency},"latency_human":"${latency_human}","bytes_in":${bytes_in},"bytes_out":${bytes_out}}` + "\n",
	}))
	e.Use(middleware.Recover())

	// Authentication middleware function - will be applied selectively
	authMiddleware := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
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
	}

	// Public endpoints (no authentication required)
	// Health check and token verification
	e.GET("/ping", ping)
	e.GET("/token-check", tokenCheck)

	// Statistics and metrics endpoints (public access for monitoring)
	e.GET("/qps-data", getQPSData)
	e.GET("/qps-stats", getQPSStats)
	e.GET("/hourly-messages", getHourlyMessages)
	e.GET("/daily-messages", getDailyMessages)
	e.GET("/system-metrics", getSystemMetrics)
	e.GET("/system-stats", getSystemStats)
	e.GET("/cluster-system-metrics", getClusterSystemMetrics)
	e.GET("/cluster-system-stats", getClusterSystemStats)
	e.GET("/cluster-status", getClusterStatus)
	e.GET("/cluster", getCluster)

	// Create authenticated group for management endpoints
	auth := e.Group("", authMiddleware)

	// Project endpoints (use plural form for consistency) - REQUIRE AUTH
	auth.GET("/projects", getProjects)
	auth.GET("/projects/:id", getProject)
	auth.POST("/projects", createProject)
	auth.DELETE("/projects/:id", deleteProject)
	auth.PUT("/projects/:id", updateProject)
	auth.POST("/start-project", StartProject)
	auth.POST("/stop-project", StopProject)
	auth.POST("/restart-project", RestartProject)
	auth.POST("/restart-all-projects", RestartAllProjects)
	auth.GET("/project-error/:id", getProjectError)
	auth.GET("/project-inputs/:id", getProjectInputs)
	auth.GET("/project-components/:id", getProjectComponents)
	auth.GET("/project-component-sequences/:id", getProjectComponentSequences)
	auth.GET("/cluster-project-states", getClusterProjectStates)

	// Ruleset endpoints (use plural form for consistency) - REQUIRE AUTH
	auth.GET("/rulesets", getRulesets)
	auth.GET("/rulesets/:id", getRuleset)
	auth.POST("/rulesets", createRuleset)
	auth.PUT("/rulesets/:id", updateRuleset)
	auth.DELETE("/rulesets/:id", deleteRuleset)

	// Ruleset rule management endpoints - REQUIRE AUTH
	auth.DELETE("/rulesets/:id/rules/:ruleId", deleteRulesetRule)
	auth.POST("/rulesets/:id/rules", addRulesetRule)

	// Ruleset templates and documentation - REQUIRE AUTH (Updated to use MCP module)
	auth.GET("/ruleset-templates", mcp.GetRulesetTemplates)
	auth.GET("/ruleset-syntax-guide", mcp.GetRulesetSyntaxGuide)
	auth.GET("/rule-templates", mcp.GetRuleTemplates)

	// Input endpoints (use plural form for consistency) - REQUIRE AUTH
	auth.GET("/inputs", getInputs)
	auth.GET("/inputs/:id", getInput)
	auth.POST("/inputs", createInput)
	auth.PUT("/inputs/:id", updateInput)
	auth.DELETE("/inputs/:id", deleteInput)

	// Output endpoints (use plural form for consistency) - REQUIRE AUTH
	auth.GET("/outputs", getOutputs)
	auth.GET("/outputs/:id", getOutput)
	auth.POST("/outputs", createOutput)
	auth.PUT("/outputs/:id", updateOutput)
	auth.DELETE("/outputs/:id", deleteOutput)

	// Plugin endpoints (use plural form and :id for consistency) - REQUIRE AUTH
	auth.GET("/plugins", getPlugins)
	auth.GET("/plugins/:id", getPlugin)
	auth.POST("/plugins", createPlugin)
	auth.PUT("/plugins/:id", updatePlugin)
	auth.DELETE("/plugins/:id", deletePlugin)
	auth.GET("/available-plugins", getAvailablePlugins)
	auth.GET("/plugin-parameters/:id", GetPluginParameters)

	// Component verification and testing - REQUIRE AUTH
	auth.POST("/verify/:type/:id", verifyComponent)
	auth.GET("/connect-check/:type/:id", connectCheck)
	auth.POST("/connect-check/:type/:id", connectCheck)
	auth.POST("/test-plugin/:id", testPlugin)
	auth.POST("/test-plugin-content", testPluginContent)
	auth.POST("/test-ruleset/:id", testRuleset)
	auth.POST("/test-ruleset-content", testRulesetContent)
	auth.POST("/test-output/:id", testOutput)
	auth.POST("/test-project/:id", testProject)
	auth.POST("/test-project-content/:inputNode", testProjectContent)

	// Cluster management endpoints - REQUIRE AUTH
	auth.POST("/cluster/heartbeat", handleHeartbeat)
	auth.POST("/component-sync", handleComponentSync)
	auth.POST("/project-status-sync", handleProjectStatusSync)
	auth.POST("/qps-sync", handleQPSSync)
	auth.GET("/config_root", leaderConfig)
	auth.GET("/config/download", downloadConfig)

	// Pending changes management (enhanced) - REQUIRE AUTH
	auth.GET("/pending-changes", GetPendingChanges)                   // Legacy endpoint
	auth.GET("/pending-changes/enhanced", GetEnhancedPendingChanges)  // Enhanced endpoint with status info
	auth.POST("/apply-changes", ApplyPendingChanges)                  // Legacy endpoint
	auth.POST("/apply-changes/enhanced", ApplyPendingChangesEnhanced) // Enhanced endpoint with transaction support
	auth.POST("/apply-single-change", ApplySingleChange)              // Legacy endpoint
	auth.POST("/verify-changes", VerifyPendingChanges)                // Verify all changes
	auth.POST("/verify-change/:type/:id", VerifySinglePendingChange)  // Verify single change
	auth.DELETE("/cancel-change/:type/:id", CancelPendingChange)      // Cancel single change
	auth.DELETE("/cancel-all-changes", CancelAllPendingChanges)       // Cancel all changes

	// Temporary file management - REQUIRE AUTH
	auth.POST("/temp-file/:type/:id", CreateTempFile)
	auth.GET("/temp-file/:type/:id", CheckTempFile)
	auth.DELETE("/temp-file/:type/:id", DeleteTempFile)

	// Sampler endpoints - REQUIRE AUTH
	auth.GET("/samplers/data", GetSamplerData)
	auth.POST("/samplers/data/intelligent", GetSamplersDataIntelligent)
	auth.GET("/ruleset-fields/:id", GetRulesetFields)

	// Cancel upgrade routes - REQUIRE AUTH
	auth.POST("/cancel-upgrade/rulesets/:id", cancelRulesetUpgrade)
	auth.POST("/cancel-upgrade/inputs/:id", cancelInputUpgrade)
	auth.POST("/cancel-upgrade/outputs/:id", cancelOutputUpgrade)
	auth.POST("/cancel-upgrade/projects/:id", cancelProjectUpgrade)
	auth.POST("/cancel-upgrade/plugins/:id", cancelPluginUpgrade)

	// Component usage analysis - REQUIRE AUTH
	auth.GET("/component-usage/:type/:id", GetComponentUsage)

	// Component configuration search - REQUIRE AUTH
	auth.GET("/search-components", searchComponentsConfig)

	// Load local components routes - REQUIRE AUTH
	auth.GET("/local-changes", getLocalChanges)
	auth.POST("/load-local-changes", loadLocalChanges)
	auth.POST("/load-single-local-change", loadSingleLocalChange)

	// Combined metrics sync endpoint (only on leader) - REQUIRE AUTH
	auth.POST("/metrics-sync", handleMetricsSync)

	// Error log endpoints - REQUIRE AUTH
	auth.GET("/error-logs", getErrorLogs)
	auth.GET("/cluster-error-logs", getClusterErrorLogs)

	// Operations history endpoints - REQUIRE AUTH
	auth.GET("/operations-history", GetOperationsHistory)
	auth.GET("/operations-stats", GetOperationsStats)

	// MCP (Model Context Protocol) endpoints - REQUIRE AUTH
	auth.POST("/mcp", handleMCP)              // Main MCP JSON-RPC endpoint
	auth.GET("/mcp", handleMCP)               // MCP SSE endpoint (for Cline and similar clients)
	auth.DELETE("/mcp", handleMCP)            // MCP session termination endpoint
	auth.POST("/mcp/batch", handleMCPBatch)   // Batch MCP requests
	auth.GET("/mcp/info", getMCPInfo)         // MCP server information
	auth.GET("/mcp/manifest", getMCPManifest) // MCP server manifest
	auth.GET("/mcp/stats", getMCPStats)       // MCP statistics
	auth.GET("/mcp/health", mcpHealthCheck)   // MCP health check
	auth.GET("/mcp/ws", handleMCPWebSocket)   // WebSocket endpoint (future)

	// MCP Configuration endpoints - REQUIRE AUTH
	auth.GET("/mcp/prompts", mcp.GetMCPPrompts)    // MCP prompts configuration
	auth.GET("/mcp/configs", mcp.GetAllMCPConfigs) // All MCP configurations

	// MCP Installation endpoints (public access for easy setup)
	e.GET("/mcp/install", getMCPInstallConfig) // MCP installation configuration

	if err := e.Start(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
