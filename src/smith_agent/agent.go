package smith_agent

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"context"
	"fmt"
	"strings"
	"sync"
)

var (
	ready bool
	mu    sync.RWMutex
)

// InitIfLLMAvailable initializes the Smith Agent only when LLM config is set and a probe request succeeds.
// Call this after Redis init and LLM config sync (e.g. after plugin.RegisterLLMCallIfConfigured).
func InitIfLLMAvailable() {
	mu.Lock()
	defer mu.Unlock()
	if ready {
		return
	}
	if common.Config == nil || strings.TrimSpace(common.Config.LLMApiKey) == "" {
		logger.Info("smith_agent: skipped (no llm_api_key)")
		return
	}
	// Probe: minimal chat to verify endpoint and key work
	_, err := callChat("You are a helpful assistant.", "Reply with exactly: OK", "", 16, probeTimeout)
	if err != nil {
		logger.Warn("smith_agent: LLM probe failed, agent not initialized", "error", err)
		return
	}
	ready = true
	logger.Info("smith_agent: initialized (LLM available)")
}

// Ready returns whether the Smith Agent is initialized and can run analysis.
func Ready() bool {
	mu.RLock()
	defer mu.RUnlock()
	return ready
}

// AnalysisResult holds the architect analysis output.
type AnalysisResult struct {
	ArchitectOutput string // Summary, field meanings, classification
}

// AnalyzeData runs the architect data analysis (single step).
// Returns error if agent is not Ready() or the LLM call fails.
func AnalyzeData(ctx context.Context, logName, sampleData string) (*AnalysisResult, error) {
	if !Ready() {
		return nil, fmt.Errorf("smith_agent not initialized: configure llm_api_key and ensure LLM is available")
	}
	if strings.TrimSpace(sampleData) == "" {
		return nil, fmt.Errorf("sampleData is empty")
	}
	if logName == "" {
		logName = "log"
	}

	user := fmt.Sprintf("Log name: %s\nLog data:\n%s", logName, sampleData)
	architectOut, err := callChat(promptArchitect, user, "", defaultMaxTokens, requestTimeout)
	if err != nil {
		return nil, fmt.Errorf("architect step: %w", err)
	}

	return &AnalysisResult{
		ArchitectOutput: architectOut,
	}, nil
}
