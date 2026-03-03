package smith_agent

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"strings"
	"sync"
)

var (
	ready bool
	mu    sync.RWMutex
)

// InitIfLLMAvailable initializes the Smith Agent only when LLM config is set and a probe request succeeds.
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
	_, err := callChat("You are a helpful assistant.", "Reply with exactly: OK", "", 16, probeTimeout)
	if err != nil {
		if isLLMConfigurationError(err) {
			logger.Error("smith_agent: LLM probe failed, agent not initialized", "error", err)
		} else {
			logger.Warn("smith_agent: LLM probe failed, agent not initialized", "error", err)
		}
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
