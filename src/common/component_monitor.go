package common

import (
	"AgentSmith-HUB/logger"
	"context"
	"fmt"
	"sync"
	"time"
)

// ComponentMonitor monitors the health of all components in running projects
type ComponentMonitor struct {
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	interval time.Duration
}

// NewComponentMonitor creates a new component monitor instance
func NewComponentMonitor(interval time.Duration) *ComponentMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &ComponentMonitor{
		ctx:      ctx,
		cancel:   cancel,
		interval: interval,
	}
}

// Start begins the component monitoring process
func (cm *ComponentMonitor) Start() error {
	cm.wg.Add(1)

	go func() {
		defer cm.wg.Done()

		logger.Info("Component monitor started", "interval", cm.interval)

		ticker := time.NewTicker(cm.interval)
		defer ticker.Stop()

		// Run initial check
		cm.performHealthCheck()

		for {
			select {
			case <-cm.ctx.Done():
				logger.Info("Component monitor stopping due to context cancellation")
				return
			case <-ticker.C:
				cm.performHealthCheck()
			}
		}
	}()

	return nil
}

// Stop gracefully stops the component monitor
func (cm *ComponentMonitor) Stop() error {
	logger.Info("Stopping component monitor...")

	// Cancel context to signal goroutine to stop
	cm.cancel()

	// Wait for goroutine to finish with timeout
	done := make(chan struct{})
	go func() {
		cm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("Component monitor stopped successfully")
		return nil
	case <-time.After(30 * time.Second):
		logger.Warn("Component monitor stop timeout, forcing shutdown")
		return fmt.Errorf("timeout waiting for component monitor to stop")
	}
}

// performHealthCheck performs the actual health check on all components
func (cm *ComponentMonitor) performHealthCheck() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Panic in component health check", "panic", r)
		}
	}()

	// Get all component errors using the registered checker
	componentErrors := CheckAllProjectComponents()

	if len(componentErrors) == 0 {
		// No errors found, all components are healthy
		return
	}

	// Group errors by project
	projectErrors := make(map[string][]ProjectComponentError)
	for _, compErr := range componentErrors {
		projectErrors[compErr.ProjectID] = append(projectErrors[compErr.ProjectID], compErr)
	}

	// Process each project with errors
	for projectID, errors := range projectErrors {
		logger.Warn("Component errors detected in project",
			"project", projectID,
			"error_count", len(errors))

		// Set project status to error using the global function
		SetProjectErrorStatus(projectID, errors)
	}

	logger.Info("Component health check completed",
		"total_errors", len(componentErrors),
		"affected_projects", len(projectErrors))
}

// ComponentHealthInfo contains health information about a component
type ComponentHealthInfo struct {
	ProjectID   string
	ComponentID string
	Type        string // "input", "output", "ruleset"
	Status      Status
	Error       error
	LastCheck   time.Time
}

// GetComponentHealth returns health information for all components
func (cm *ComponentMonitor) GetComponentHealth() []ComponentHealthInfo {
	var healthInfo []ComponentHealthInfo

	// Get all component errors
	componentErrors := CheckAllProjectComponents()

	// Convert to health info format
	for _, compErr := range componentErrors {
		healthInfo = append(healthInfo, ComponentHealthInfo{
			ProjectID:   compErr.ProjectID,
			ComponentID: compErr.ComponentID,
			Type:        compErr.Type,
			Status:      compErr.Status,
			Error:       compErr.Error,
			LastCheck:   time.Now(),
		})
	}

	return healthInfo
}
