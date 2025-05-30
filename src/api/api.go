package api

import (
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/project"
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

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

// getRedisMetrics returns Redis server metrics
func getRedisMetrics(c echo.Context) error {
	metrics, err := common.GetRedisMetrics()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to get Redis metrics: %v", err),
		})
	}
	return c.JSON(http.StatusOK, metrics)
}

// handleHeartbeat handles incoming heartbeat requests from cluster nodes
func handleHeartbeat(c echo.Context) error {
	var payload struct {
		NodeID    string `json:"node_id"`
		NodeAddr  string `json:"node_addr"`
		Timestamp string `json:"timestamp"`
	}

	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("invalid heartbeat payload: %v", err),
		})
	}

	cm := cluster.ClusterInstance
	if cm == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "cluster manager not initialized",
		})
	}

	// Update node heartbeat
	cm.UpdateNodeHeartbeat(payload.NodeID)

	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// getClusterStatus returns the current cluster status
func getClusterStatus(c echo.Context) error {
	cm := cluster.ClusterInstance
	if cm == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "cluster manager not initialized",
		})
	}

	return c.JSON(http.StatusOK, cm.GetClusterStatus())
}

// downloadConfig handles downloading the entire config directory
func downloadConfig(c echo.Context) error {
	configRoot := project.ConfigRoot
	if configRoot == "" {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "config root not set",
		})
	}

	// Create a zip file in memory
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Walk through the config directory
	err := filepath.Walk(configRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Create a new file in the zip
		relPath, err := filepath.Rel(configRoot, path)
		if err != nil {
			return err
		}

		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		// Read and write file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		_, err = writer.Write(content)
		return err
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to create zip: %v", err),
		})
	}

	// Close the zip writer
	if err := zipWriter.Close(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to close zip: %v", err),
		})
	}

	// Set response headers
	c.Response().Header().Set(echo.HeaderContentType, "application/zip")
	c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename=config.zip")

	// Send the zip file
	return c.Blob(http.StatusOK, "application/zip", buf.Bytes())
}

// FileChecksum represents a file's checksum information
type FileChecksum struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

// verifyConfig handles verification of downloaded config files
func verifyConfig(c echo.Context) error {
	// Get the uploaded zip file
	file, err := c.FormFile("config")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("failed to get uploaded file: %v", err),
		})
	}

	// Open the uploaded file
	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to open uploaded file: %v", err),
		})
	}
	defer src.Close()

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "config-verify-*")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to create temp directory: %v", err),
		})
	}
	defer os.RemoveAll(tempDir)

	// Extract the zip file
	zipReader, err := zip.NewReader(src, file.Size)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("failed to read zip file: %v", err),
		})
	}

	// Calculate checksums for all files
	checksums := make([]FileChecksum, 0)
	for _, f := range zipReader.File {
		if f.FileInfo().IsDir() {
			continue
		}

		// Open the file in the zip
		rc, err := f.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("failed to open file in zip: %v", err),
			})
		}

		// Calculate SHA-256 checksum
		hash := sha256.New()
		if _, err := io.Copy(hash, rc); err != nil {
			rc.Close()
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("failed to calculate checksum: %v", err),
			})
		}
		rc.Close()

		checksums = append(checksums, FileChecksum{
			Path:     f.Name,
			Size:     f.FileInfo().Size(),
			Checksum: fmt.Sprintf("%x", hash.Sum(nil)),
		})
	}

	// Compare with current config files
	configRoot := project.ConfigRoot
	if configRoot == "" {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "config root not set",
		})
	}

	verificationResults := make(map[string]interface{})
	for _, cs := range checksums {
		currentPath := filepath.Join(configRoot, cs.Path)
		currentInfo, err := os.Stat(currentPath)
		if err != nil {
			verificationResults[cs.Path] = map[string]string{
				"status": "missing",
				"error":  err.Error(),
			}
			continue
		}

		// Check file size
		if currentInfo.Size() != cs.Size {
			verificationResults[cs.Path] = map[string]string{
				"status":   "size_mismatch",
				"expected": fmt.Sprintf("%d", cs.Size),
				"actual":   fmt.Sprintf("%d", currentInfo.Size()),
			}
			continue
		}

		// Calculate current file's checksum
		currentFile, err := os.Open(currentPath)
		if err != nil {
			verificationResults[cs.Path] = map[string]string{
				"status": "error",
				"error":  err.Error(),
			}
			continue
		}

		hash := sha256.New()
		if _, err := io.Copy(hash, currentFile); err != nil {
			currentFile.Close()
			verificationResults[cs.Path] = map[string]string{
				"status": "error",
				"error":  err.Error(),
			}
			continue
		}
		currentFile.Close()

		currentChecksum := fmt.Sprintf("%x", hash.Sum(nil))
		if currentChecksum != cs.Checksum {
			verificationResults[cs.Path] = map[string]string{
				"status":   "checksum_mismatch",
				"expected": cs.Checksum,
				"actual":   currentChecksum,
			}
			continue
		}

		verificationResults[cs.Path] = map[string]string{
			"status": "ok",
		}
	}

	return c.JSON(http.StatusOK, verificationResults)
}

type CtrlProjectRequest struct {
	ProjectID string `json:"project_id"`
}

// StartProject starts a project with the given configuration
func StartProject(c echo.Context) error {
	var req CtrlProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Get project
	p := project.GetProject(req.ProjectID)
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Project not found",
		})
	}

	// Check if project is already running
	if p.Status == project.ProjectStatusRunning {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is already running",
		})
	}

	// Start the project
	if err := p.Start(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to start project: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Project started successfully",
		"project": map[string]interface{}{
			"id":     p.Id,
			"name":   p.Name,
			"status": p.Status,
		},
	})
}

// StopProject stops a running project
func StopProject(c echo.Context) error {
	var req CtrlProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Get project
	p := project.GetProject(req.ProjectID)
	if p == nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Project not found",
		})
	}

	// Check if project is running
	if p.Status != project.ProjectStatusRunning {
		return c.JSON(http.StatusConflict, map[string]string{
			"error": "Project is not running",
		})
	}

	// Stop the project
	if err := p.Stop(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to stop project: %v", err),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Project stopped successfully",
		"project": map[string]interface{}{
			"id":     p.Id,
			"name":   p.Name,
			"status": p.Status,
		},
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
	e.POST("/project/start", StartProject)
	e.POST("/project/stop", StopProject)

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

	// Redis monitoring endpoints
	e.GET("/redis/metrics", getRedisMetrics)

	// Cluster endpoints
	e.POST("/cluster/heartbeat", handleHeartbeat)
	e.GET("/cluster/status", getClusterStatus)

	// Config endpoints
	e.GET("/config/download", downloadConfig)
	e.POST("/config/verify", verifyConfig)

	if err := e.Start(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
