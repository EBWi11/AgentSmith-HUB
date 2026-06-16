package api

import (
	"AgentSmith-HUB/logger"
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
		AllowOrigins:     []string{"*"},                           // Allow all origins
		AllowHeaders:     []string{"*", "token", "Authorization"}, // Allow all headers and explicitly allow token and Authorization
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

	// Authentication middleware will be applied selectively via AuthenticateRequest

	// Public endpoints (no authentication required)
	// Health check and token verification
	e.GET("/ping", ping)
	e.GET("/token-check", tokenCheck)
	registerSharedPublicRoutes(e)

	// Statistics and metrics endpoints (public access for monitoring)
	e.GET("/daily-messages", getDailyMessages)
	e.GET("/system-metrics", getSystemMetrics)
	e.GET("/cluster-system-metrics", getClusterSystemMetrics)
	e.GET("/cluster-status", getClusterStatus)

	// Create authenticated group for management endpoints
	auth := e.Group("", buildAuthMiddleware(""))

	registerSharedReadRoutes(auth)

	// Project write endpoints - REQUIRE AUTH
	auth.POST("/projects", createProject)
	auth.DELETE("/projects/:id", deleteProject)
	auth.PUT("/projects/:id", updateProject)
	auth.POST("/start-project", StartProject)
	auth.POST("/stop-project", StopProject)
	auth.POST("/restart-project", RestartProject)
	auth.GET("/cluster-project-states", getClusterProjectStates)

	// Ruleset write endpoints - REQUIRE AUTH
	auth.POST("/rulesets", createRuleset)
	auth.PUT("/rulesets/:id", updateRuleset)
	auth.DELETE("/rulesets/:id", deleteRuleset)

	// Ruleset rule management endpoints - REQUIRE AUTH
	auth.DELETE("/rulesets/:id/rules/:ruleId", deleteRulesetRule)
	auth.POST("/rulesets/:id/rules", addRulesetRule)

	// Ruleset folder management endpoints - REQUIRE AUTH
	auth.GET("/ruleset-folders", getRulesetFolders)
	auth.POST("/ruleset-folders", createRulesetFolder)
	auth.PUT("/ruleset-folders/:name", renameRulesetFolder)
	auth.DELETE("/ruleset-folders/:name", deleteRulesetFolder)
	auth.PUT("/rulesets/:id/move", moveRuleset)

	// Input endpoints (write) - REQUIRE AUTH
	auth.POST("/inputs", createInput)
	auth.PUT("/inputs/:id", updateInput)
	auth.DELETE("/inputs/:id", deleteInput)

	// Output endpoints (write) - REQUIRE AUTH
	auth.POST("/outputs", createOutput)
	auth.PUT("/outputs/:id", updateOutput)
	auth.DELETE("/outputs/:id", deleteOutput)

	// Plugin endpoints (write) - REQUIRE AUTH
	auth.POST("/plugins", createPlugin)
	auth.PUT("/plugins/:id", updatePlugin)
	auth.DELETE("/plugins/:id", deletePlugin)

	// Agent endpoints - REQUIRE AUTH
	auth.POST("/agents", createAgent)
	auth.PUT("/agents/:id", updateAgent)
	auth.DELETE("/agents/:id", deleteAgentHandler)
	auth.POST("/agents/:id/memory-notes", updateAgentMemoryNotes)
	auth.POST("/agents/:id/memory-notes/generate-from-log", generateAgentMemoryFromLog)

	auth.POST("/skills", createSkill)
	auth.PUT("/skills/:id", updateSkill)
	auth.DELETE("/skills/:id", deleteSkillHandler)

	// Component verification and testing - REQUIRE AUTH
	auth.POST("/verify/:type/:id", verifyComponent)
	auth.POST("/connect-check/:type/:id", connectCheck)
	auth.POST("/test-plugin/:id", testPlugin)
	auth.POST("/test-plugin-content", testPlugin)
	auth.POST("/test-ruleset/:id", testRuleset)
	auth.POST("/test-ruleset-content", testRuleset)
	auth.POST("/test-agent/:id", testAgent)
	auth.POST("/test-agent-content", testAgent)
	auth.POST("/test-output/:id", testOutput)
	auth.POST("/test-output-content", testOutput)
	auth.POST("/test-project/:id", testProject)
	auth.POST("/test-project-content/:inputNode", testProject)

	// Cluster management endpoints - REQUIRE AUTH
	auth.GET("/config_root", leaderConfig)
	auth.GET("/config/download", downloadConfig)
	auth.GET("/cluster/instruction-stats", getInstructionStats)
	auth.GET("/cluster/follower-execution-status", getFollowerExecutionStatus)

	// Pending changes management (enhanced) - REQUIRE AUTH
	auth.GET("/pending-changes/enhanced", GetEnhancedPendingChanges) // Enhanced endpoint with status info
	auth.POST("/apply-single-change", ApplySingleChange)             // Legacy endpoint
	auth.POST("/verify-changes", VerifyPendingChanges)               // Verify all changes
	auth.DELETE("/cancel-change/:type/:id", CancelPendingChange)     // Cancel single change
	auth.DELETE("/cancel-all-changes", CancelAllPendingChanges)      // Cancel all changes

	// Temporary file management - REQUIRE AUTH
	auth.POST("/temp-file/:type/:id", CreateTempFile)
	auth.GET("/temp-file/:type/:id", CheckTempFile)
	auth.DELETE("/temp-file/:type/:id", DeleteTempFile)

	// Sampler endpoints - REQUIRE AUTH
	auth.POST("/samplers/data/intelligent", GetSamplersDataIntelligent)

	// Cancel upgrade routes - REQUIRE AUTH
	auth.POST("/cancel-upgrade/rulesets/:id", cancelRulesetUpgrade)
	auth.POST("/cancel-upgrade/inputs/:id", cancelInputUpgrade)
	auth.POST("/cancel-upgrade/outputs/:id", cancelOutputUpgrade)
	auth.POST("/cancel-upgrade/projects/:id", cancelProjectUpgrade)
	auth.POST("/cancel-upgrade/plugins/:id", cancelPluginUpgrade)
	auth.POST("/cancel-upgrade/agents/:id", cancelAgentUpgrade)
	auth.POST("/cancel-upgrade/skills/:id", cancelSkillUpgrade)

	// Load local components routes - REQUIRE AUTH
	auth.GET("/local-changes", getLocalChanges)
	auth.GET("/local-changes/count", getLocalChangesCount) // Lightweight count endpoint
	auth.POST("/load-local-changes", loadLocalChanges)
	auth.POST("/load-single-local-change", loadSingleLocalChange)

	// Error log endpoints - REQUIRE AUTH
	auth.GET("/error-logs", getErrorLogs)
	auth.GET("/error-logs/nodes", getErrorLogNodes)
	auth.GET("/cluster-error-logs", getClusterErrorLogs)

	// Agent log endpoints - REQUIRE AUTH
	auth.GET("/agent-logs", getAgentLogs)
	auth.POST("/agent-logs/:agentId/:logId/comments", postAgentLogComment)

	// Operations history endpoints - REQUIRE AUTH
	auth.GET("/operations-history", GetOperationsHistory)
	auth.GET("/operations-history/nodes", GetOperationsHistoryNodes)

	// Plugin statistics endpoint - REQUIRE AUTH
	auth.GET("/plugin-stats", GetPluginStats)

	if err := e.Start(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
