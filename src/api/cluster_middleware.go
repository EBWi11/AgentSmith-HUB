package api

import (
	"AgentSmith-HUB/common"
	"net/http"

	"github.com/labstack/echo/v4"
)

// RequireLeaderMiddleware returns middleware that requires the current node to be a leader
func RequireLeaderMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if err := common.RequireLeader(); err != nil {
				return c.JSON(http.StatusForbidden, map[string]interface{}{
					"error":           "This operation is only available on leader nodes",
					"message":         "The current node is not the cluster leader. Please connect to the leader node.",
					"leader_required": true,
					"current_node":    common.GetNodeID(),
					"leader_node":     common.GetLeaderID(),
				})
			}
			return next(c)
		}
	}
}

// RequireFollowerMiddleware returns middleware that requires the current node to be a follower
func RequireFollowerMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if err := common.RequireFollower(); err != nil {
				return c.JSON(http.StatusForbidden, map[string]interface{}{
					"error":             "This operation is only available on follower nodes",
					"message":           "The current node is the cluster leader. This operation should be performed on follower nodes.",
					"follower_required": true,
					"current_node":      common.GetNodeID(),
				})
			}
			return next(c)
		}
	}
}

// CheckLeaderStatus returns a helper function to check leader status in handlers
func CheckLeaderStatus(c echo.Context) error {
	if !common.IsCurrentNodeLeader() {
		return c.JSON(http.StatusForbidden, map[string]interface{}{
			"error":        "Leader node required",
			"current_node": common.GetNodeID(),
			"leader_node":  common.GetLeaderID(),
			"is_leader":    false,
		})
	}
	return nil
}

// CheckFollowerStatus returns a helper function to check follower status in handlers
func CheckFollowerStatus(c echo.Context) error {
	if common.IsCurrentNodeLeader() {
		return c.JSON(http.StatusForbidden, map[string]interface{}{
			"error":        "Follower node required",
			"current_node": common.GetNodeID(),
			"is_leader":    true,
		})
	}
	return nil
}
