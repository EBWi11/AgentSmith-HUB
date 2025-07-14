package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var followerToken string // Store token read at startup

// ServerStartFollower starts the follower API server with read-only endpoints
func ServerStartFollower(listenAddr string) error {
	if common.IsCurrentNodeLeader() {
		logger.Info("Leader node: skipping follower server startup")
		return nil
	}

	// Read token from Redis once at startup
	var err error
	followerToken, err = ReadTokenFromRedis()
	if err != nil {
		logger.Error("Failed to read token from Redis, follower server will not start: %v", err)
		return err
	}

	e := echo.New()
	e.HideBanner = true

	// Add CORS middleware
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "token"},
	}))

	// Add request logging
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "follower-api ${time_rfc3339} ${method} ${uri} ${status} ${latency_human}\n",
	}))

	// Recovery middleware
	e.Use(middleware.Recover())

	// Authentication middleware for protected endpoints
	authMiddleware := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := c.Request().Header.Get("token")
			if token == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "Token required",
				})
			}

			if token != followerToken {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "Invalid token",
				})
			}

			return next(c)
		}
	}

	// Public endpoints (no authentication required)
	e.GET("/ping", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"message": "pong",
			"role":    "follower",
		})
	})

	e.GET("/follower-status", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"role":    "follower",
			"address": listenAddr,
			"leader":  common.Config.Leader,
		})
	})

	// Protected endpoints (require authentication)
	auth := e.Group("", authMiddleware)

	// Read-only project endpoints
	auth.GET("/projects", getProjects)
	auth.GET("/projects/:id", getProject)
	auth.GET("/project-error/:id", getProjectError)
	auth.GET("/project-inputs/:id", getProjectInputs)
	auth.GET("/project-components/:id", getProjectComponents)
	auth.GET("/project-component-sequences/:id", getProjectComponentSequences)

	// Read-only component endpoints
	auth.GET("/rulesets", getRulesets)
	auth.GET("/rulesets/:id", getRuleset)
	auth.GET("/inputs", getInputs)
	auth.GET("/inputs/:id", getInput)
	auth.GET("/outputs", getOutputs)
	auth.GET("/outputs/:id", getOutput)
	auth.GET("/plugins", getPlugins)
	auth.GET("/plugins/:id", getPlugin)
	auth.GET("/available-plugins", getPlugins) // Use same handler with different default params

	// Read-only testing endpoints
	auth.GET("/connect-check/:type/:id", connectCheck)
	auth.GET("/plugin-parameters/:id", GetPluginParameters)
	auth.GET("/plugin-parameters", GetBatchPluginParameters)
	auth.GET("/plugins/:id/usage", getPluginUsage)

	// Read-only configuration endpoints
	auth.GET("/samplers/data", GetSamplerData)
	auth.GET("/ruleset-fields/:id", GetRulesetFields)
	auth.GET("/ruleset-fields", GetBatchRulesetFields)

	// Read-only analysis endpoints
	auth.GET("/component-usage/:type/:id", GetComponentUsage)
	auth.GET("/search-components", searchComponentsConfig)

	// Block all write operations with helpful error messages
	blockWriteOperation := func(c echo.Context) error {
		return c.JSON(http.StatusForbidden, map[string]interface{}{
			"error":   "Write operations not allowed on follower node",
			"message": "Please use the leader node for write operations",
			"leader":  common.Config.Leader,
		})
	}

	// Block all POST, PUT, DELETE operations
	e.POST("/*", blockWriteOperation)
	e.PUT("/*", blockWriteOperation)
	e.DELETE("/*", blockWriteOperation)
	e.PATCH("/*", blockWriteOperation)

	logger.Info("Starting follower API server on %s", listenAddr)

	if err := e.Start(listenAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("Follower API server failed to start: %v", err)
		return err
	}
	return nil
}
