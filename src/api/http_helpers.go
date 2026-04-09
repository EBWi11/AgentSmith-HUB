package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func jsonError(c echo.Context, status int, message string) error {
	return c.JSON(status, map[string]string{
		"error": message,
	})
}

func jsonMessage(c echo.Context, status int, message string) error {
	return c.JSON(status, map[string]string{
		"message": message,
	})
}

func unauthorizedError(c echo.Context, message string) error {
	return jsonError(c, http.StatusUnauthorized, message)
}

func forbiddenError(c echo.Context, message string) error {
	return jsonError(c, http.StatusForbidden, message)
}

func badRequestError(c echo.Context, message string) error {
	return jsonError(c, http.StatusBadRequest, message)
}

func notFoundError(c echo.Context, message string) error {
	return jsonError(c, http.StatusNotFound, message)
}

func internalServerError(c echo.Context, message string) error {
	return jsonError(c, http.StatusInternalServerError, message)
}
