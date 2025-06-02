package api

import (
	"github.com/labstack/echo/v4"
	"net/http"
)

func TokenAuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		token := c.Request().Header.Get("token")
		if token == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "missing token",
			})
		}

		if token != hubConfig.Token {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "authentication failed",
			})
		}

		return next(c)
	}
}
