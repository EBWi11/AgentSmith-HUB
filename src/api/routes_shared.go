package api

import (
	"AgentSmith-HUB/logger"

	"github.com/labstack/echo/v4"
)

func buildAuthMiddleware(legacyToken string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if legacyToken != "" {
				token := c.Request().Header.Get("token")
				if token != "" && token == legacyToken {
					return next(c)
				}
			}

			if err := AuthenticateRequest(c); err != nil {
				logger.Error("authentication failed", "error", err)
				return unauthorizedError(c, "Authentication failed")
			}

			return next(c)
		}
	}
}

func registerSharedPublicRoutes(e *echo.Echo) {
	e.GET("/auth/config", getAuthConfig)
	e.GET("/features", getFeatures)
}

func registerSharedReadRoutes(auth *echo.Group) {
	auth.GET("/projects", getProjects)
	auth.GET("/projects/:id", getProject)
	auth.GET("/project-error/:id", getProjectError)
	auth.GET("/project-inputs/:id", getProjectInputs)
	auth.GET("/project-components/:id", getProjectComponents)
	auth.GET("/project-component-sequences/:id", getProjectComponentSequences)

	auth.GET("/rulesets", getRulesets)
	auth.GET("/rulesets/:id", getRuleset)
	auth.GET("/inputs", getInputs)
	auth.GET("/inputs/:id", getInput)
	auth.GET("/outputs", getOutputs)
	auth.GET("/outputs/:id", getOutput)
	auth.GET("/plugins", getPlugins)
	auth.GET("/plugins/:id", getPlugin)
	auth.GET("/plugin-parameters/:id", GetPluginParameters)
	auth.GET("/plugins/:id/usage", getPluginUsage)
	auth.GET("/agents", getAgents)
	auth.GET("/agents/:id", getAgentDetail)
	auth.GET("/skills", getSkills)
	auth.GET("/skills/:id", getSkillDetail)

	auth.GET("/samplers/data", GetSamplerData)
	auth.GET("/ruleset-fields/:id", GetRulesetFields)
	auth.GET("/component-usage/:type/:id", GetComponentUsage)
	auth.GET("/search-components", searchComponentsConfig)
	auth.GET("/connect-check/:type/:id", connectCheck)
}
