package api

import (
	"AgentSmith-HUB/project"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func ping(c echo.Context) error {
	return c.String(http.StatusOK, "pong")
}

// getProjects returns a list of all projects
func getProjects(c echo.Context) error {
	projects := project.GetProjects()
	result := make([]map[string]interface{}, 0, len(projects))

	for _, p := range projects {
		result = append(result, map[string]interface{}{
			"id":     p.Id,
			"name":   p.Name,
			"status": p.Status,
		})
	}
	return c.JSON(http.StatusOK, result)
}

// getProject returns details of a specific project
func getProject(c echo.Context) error {
	id := c.Param("id")
	p := project.GetProject(id)
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":         p.Id,
		"name":       p.Name,
		"status":     p.Status,
		"inputs":     p.Inputs,
		"outputs":    p.Outputs,
		"rulesets":   p.Rulesets,
		"uptime":     p.GetUptime().String(),
		"metrics":    p.GetMetrics(),
		"last_error": p.GetLastError(),
	})
}

// getRulesets returns a list of all rulesets
func getRulesets(c echo.Context) error {
	projects := project.GetProjects()
	rulesets := make([]map[string]interface{}, 0)

	for _, p := range projects {
		for _, rs := range p.Rulesets {
			rulesets = append(rulesets, map[string]interface{}{
				"id":   rs.RulesetID,
				"name": rs.RulesetName,
				"type": rs.Type,
			})
		}
	}
	return c.JSON(http.StatusOK, rulesets)
}

// getRuleset returns details of a specific ruleset
func getRuleset(c echo.Context) error {
	id := c.Param("id")
	projects := project.GetProjects()

	for _, p := range projects {
		if rs, ok := p.Rulesets[id]; ok {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"id":           rs.RulesetID,
				"name":         rs.RulesetName,
				"type":         rs.Type,
				"rules":        rs.Rules,
				"is_detection": rs.IsDetection,
			})
		}
	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
}

// getInputs returns a list of all input components
func getInputs(c echo.Context) error {
	projects := project.GetProjects()
	inputs := make([]map[string]interface{}, 0)

	for _, p := range projects {
		for _, in := range p.Inputs {
			inputs = append(inputs, map[string]interface{}{
				"id":   in.Id,
				"name": in.Name,
				"type": in.Type,
			})
		}
	}
	return c.JSON(http.StatusOK, inputs)
}

// getInput returns details of a specific input component
func getInput(c echo.Context) error {
	id := c.Param("id")
	projects := project.GetProjects()

	for _, p := range projects {
		if in, ok := p.Inputs[id]; ok {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"id":            in.Id,
				"name":          in.Name,
				"type":          in.Type,
				"consume_qps":   in.GetConsumeQPS(),
				"consume_total": in.GetConsumeTotal(),
			})
		}
	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "input not found"})
}

// getOutputs returns a list of all output components
func getOutputs(c echo.Context) error {
	projects := project.GetProjects()
	outputs := make([]map[string]interface{}, 0)

	for _, p := range projects {
		for _, out := range p.Outputs {
			outputs = append(outputs, map[string]interface{}{
				"id":   out.Id,
				"name": out.Name,
				"type": out.Type,
			})
		}
	}
	return c.JSON(http.StatusOK, outputs)
}

// getOutput returns details of a specific output component
func getOutput(c echo.Context) error {
	id := c.Param("id")
	projects := project.GetProjects()

	for _, p := range projects {
		if out, ok := p.Outputs[id]; ok {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"id":            out.Id,
				"name":          out.Name,
				"type":          out.Type,
				"produce_qps":   out.GetProduceQPS(),
				"produce_total": out.GetProduceTotal(),
			})
		}
	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "output not found"})
}

// getMetrics returns metrics for all projects
func getMetrics(c echo.Context) error {
	projects := project.GetProjects()
	metrics := make(map[string]interface{})

	for _, p := range projects {
		projectMetrics := p.GetMetrics()
		metrics[p.Id] = map[string]interface{}{
			"input_qps":  projectMetrics.InputQPS,
			"output_qps": projectMetrics.OutputQPS,
		}
	}
	return c.JSON(http.StatusOK, metrics)
}

// getProjectMetrics returns metrics for a specific project
func getProjectMetrics(c echo.Context) error {
	id := c.Param("id")
	p := project.GetProject(id)
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	metrics := p.GetMetrics()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"input_qps":  metrics.InputQPS,
		"output_qps": metrics.OutputQPS,
	})
}

func ServerStart(listener string) error {
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Health check
	e.GET("/ping", ping)

	// Project endpoints
	e.GET("/project", getProjects)
	e.GET("/project/:id", getProject)

	// Ruleset endpoints
	e.GET("/ruleset", getRulesets)
	e.GET("/ruleset/:id", getRuleset)

	// Input endpoints
	e.GET("/input", getInputs)
	e.GET("/input/:id", getInput)

	// Output endpoints
	e.GET("/output", getOutputs)
	e.GET("/output/:id", getOutput)

	// Metrics endpoints
	e.GET("/metrics", getMetrics)
	e.GET("/metrics/:id", getProjectMetrics)

	if err := e.Start(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
