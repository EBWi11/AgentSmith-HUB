package smith_agent

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/project"
	"context"
	"encoding/json"
	"time"
)

const (
	redisKeyPrefixInputAnalysis = "smith_agent:analysis:input:"
	inputAnalysisTTLSeconds     = 30 * 24 * 3600 // 30 days
	inputAnalysisInterval       = 24 * time.Hour
	inputAnalysisNoDataInterval = 5 * time.Minute  // cold-start poll interval
	inputAnalysisStartDelay     = 1 * time.Minute  // first run shortly after start
	coldStartSampleThreshold    = 100              // require 100 samples before first analysis
)

// StartInputAnalysisLoop starts a goroutine that, when LLM is ready and node is leader,
// runs a two-phase analysis loop:
//
// Phase 1 (Cold Start): For each input WITHOUT existing analysis in Redis, poll every 5 min
// until >= 100 sample data points are accumulated, then run analysis and store.
// Inputs that already have analysis in Redis are skipped (they'll be refreshed in Phase 2).
//
// Phase 2 (Steady State): Sleep 24h, then re-analyze ALL inputs with fresh sample data. Repeat.
func StartInputAnalysisLoop() {
	go func() {
		time.Sleep(inputAnalysisStartDelay)

		// Phase 1: Cold start — for inputs without existing analysis,
		// wait until they accumulate enough samples, then analyze.
		for {
			if !Ready() || !common.IsCurrentNodeLeader() {
				time.Sleep(inputAnalysisNoDataInterval)
				continue
			}
			if runColdStartAnalysis() {
				break // all inputs now have analysis (or no inputs exist)
			}
			time.Sleep(inputAnalysisNoDataInterval)
		}

		logger.Info("smith_agent: cold start complete, entering 24h steady-state cycle")

		// Phase 2: Steady state — re-analyze all inputs every 24h.
		for {
			time.Sleep(inputAnalysisInterval)
			if !Ready() || !common.IsCurrentNodeLeader() {
				continue
			}
			runFullAnalysis()
		}
	}()
	logger.Info("smith_agent: input analysis loop started (cold start: wait for 100 samples; steady state: every 24h)")
}

// GetInputAnalysis returns the cached LLM analysis JSON for an input, if any.
func GetInputAnalysis(inputId string) (string, bool) {
	if inputId == "" {
		return "", false
	}
	s, err := common.RedisGet(redisKeyPrefixInputAnalysis + inputId)
	if err != nil || s == "" {
		return "", false
	}
	return s, true
}

// runColdStartAnalysis processes only inputs that have NO existing analysis in Redis.
// For each such input, it checks whether sample data count >= coldStartSampleThreshold.
// If yes, it runs analysis and stores the result.
// Returns true when every input has analysis (either pre-existing or just created),
// meaning the cold-start phase is complete.
func runColdStartAnalysis() bool {
	ctx := context.Background()
	allCovered := true

	project.ForEachInput(func(inputId string, _ *input.Input) bool {
		if _, exists := GetInputAnalysis(inputId); exists {
			return true // already has analysis, skip
		}

		count := countInputSamples(inputId)
		if count < coldStartSampleThreshold {
			logger.Debug("smith_agent: cold start waiting for samples",
				"input", inputId, "current", count, "need", coldStartSampleThreshold)
			allCovered = false
			return true
		}

		// Enough samples accumulated — analyze
		if analyzeAndStoreInput(ctx, inputId) {
			logger.Info("smith_agent: cold start analysis complete", "input", inputId, "samples", count)
		}
		return true
	})

	return allCovered
}

// runFullAnalysis re-analyzes all inputs with the latest sample data (24h steady-state cycle).
func runFullAnalysis() {
	ctx := context.Background()
	stored := 0

	project.ForEachInput(func(inputId string, _ *input.Input) bool {
		if analyzeAndStoreInput(ctx, inputId) {
			stored++
		}
		return true
	})

	if stored > 0 {
		logger.Info("smith_agent: steady-state analysis cycle complete", "stored", stored)
	}
}

// countInputSamples returns the total number of sample data points across all
// project-node-sequences for the given input.
func countInputSamples(inputId string) int {
	sampler := common.GetSampler("input." + inputId)
	if sampler == nil {
		return 0
	}
	samplesBySeq := sampler.GetSamples()
	total := 0
	for _, list := range samplesBySeq {
		total += len(list)
	}
	return total
}

// analyzeAndStoreInput runs architect analysis on the latest sample of an input
// and stores the result in Redis. Returns true if analysis was stored successfully.
func analyzeAndStoreInput(ctx context.Context, inputId string) bool {
	sampler := common.GetSampler("input." + inputId)
	if sampler == nil {
		return false
	}
	samplesBySeq := sampler.GetSamples()
	if len(samplesBySeq) == 0 {
		return false
	}

	// Find the latest sample across all sequences
	var latest *common.SampleData
	for _, list := range samplesBySeq {
		for i := range list {
			s := &list[i]
			if latest == nil || s.Timestamp.After(latest.Timestamp) {
				latest = s
			}
		}
	}
	if latest == nil || latest.Data == nil {
		return false
	}

	sampleJSON, err := json.Marshal(latest.Data)
	if err != nil {
		logger.Warn("smith_agent input analysis: marshal sample failed", "input", inputId, "error", err)
		return false
	}

	result, err := AnalyzeData(ctx, inputId, string(sampleJSON))
	if err != nil {
		logger.Warn("smith_agent input analysis: analyze failed", "input", inputId, "error", err)
		return false
	}

	payload := map[string]interface{}{
		"ArchitectOutput": result.ArchitectOutput,
		"analyzed_at":     time.Now().UTC().Format(time.RFC3339),
	}
	resultJSON, err := json.Marshal(payload)
	if err != nil {
		logger.Warn("smith_agent input analysis: marshal result failed", "input", inputId, "error", err)
		return false
	}

	key := redisKeyPrefixInputAnalysis + inputId
	_, err = common.RedisSet(key, string(resultJSON), inputAnalysisTTLSeconds)
	if err != nil {
		logger.Warn("smith_agent input analysis: redis set failed", "input", inputId, "error", err)
		return false
	}

	logger.Info("smith_agent input analysis: stored", "input", inputId)
	return true
}
