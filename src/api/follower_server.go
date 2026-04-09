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
		logger.Error("Failed to read token from Redis, follower server will not start", "error", err)
		return err
	}

	e := echo.New()
	e.HideBanner = true

	// Add CORS middleware
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "token", "Authorization"},
	}))

	// Add request logging
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "follower-api ${time_rfc3339} ${method} ${uri} ${status} ${latency_human}\n",
	}))

	// Recovery middleware
	e.Use(middleware.Recover())

	// Public endpoints (no authentication required)
	e.GET("/ping", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"message": "pong",
			"role":    "follower",
		})
	})

	registerSharedPublicRoutes(e)

	e.GET("/follower-status", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"role":    "follower",
			"address": listenAddr,
			"leader":  common.Config.Leader,
		})
	})

	// Protected endpoints (require authentication)
	auth := e.Group("", buildAuthMiddleware(followerToken))
	registerSharedReadRoutes(auth)

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

	logger.Info("Starting follower API server", "addr", listenAddr)

	if err := e.Start(listenAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("Follower API server failed to start", "error", err)
		return err
	}
	return nil
}
