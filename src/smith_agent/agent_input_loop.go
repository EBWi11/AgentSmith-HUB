package smith_agent

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/input"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/project"
	"context"
	"encoding/json"
	"sort"
	"time"
)

const (
	redisKeyPrefixInputAnalysis = "smith_agent:analysis:input:"
	inputAnalysisTTLSeconds     = 30 * 24 * 3600 // 30 days
	inputAnalysisInterval       = 24 * time.Hour // full refresh interval
	inputAnalysisPollInterval   = 5 * time.Minute
	inputAnalysisStartDelay     = 1 * time.Minute

	// Progressive cold-start strategy:
	// - First result appears quickly with a small sample size.
	// - If data is sparse, force first analysis after max wait.
	firstAnalysisMinSamples = 5
	firstAnalysisMaxWait    = 30 * time.Minute

	// Progressive enhancement thresholds.
	enhancedStageThreshold = 30
	stableStageThreshold   = 100

	// Re-analyze earlier when sample volume grows materially.
	inputAnalysisEnhancementMinInterval = 1 * time.Hour
	inputAnalysisSampleGrowthStep       = 10
)

type analysisStage string

const (
	analysisStageInitial  analysisStage = "initial"
	analysisStageEnhanced analysisStage = "enhanced"
	analysisStageStable   analysisStage = "stable"
)

type inputAnalysisCache struct {
	ArchitectOutput string `json:"ArchitectOutput"`
	AnalyzedAt      string `json:"analyzed_at,omitempty"`
	Stage           string `json:"stage,omitempty"`
	SampleCount     int    `json:"sample_count,omitempty"`
}

func (c inputAnalysisCache) analyzedAtTime() time.Time {
	if c.AnalyzedAt == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, c.AnalyzedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

type analyzeDecision struct {
	ShouldAnalyze bool
	Stage         analysisStage
	Reason        string
}

// StartInputAnalysisLoop runs a progressive analysis loop:
// 1) quick first analysis (small sample threshold or max wait),
// 2) incremental enhancement as sample size grows,
// 3) periodic full refresh every 24h.
func StartInputAnalysisLoop() {
	go func() {
		time.Sleep(inputAnalysisStartDelay)
		startedAt := time.Now()

		for {
			if !Ready() || !common.IsCurrentNodeLeader() {
				time.Sleep(inputAnalysisPollInterval)
				continue
			}
			runProgressiveAnalysis(startedAt)
			time.Sleep(inputAnalysisPollInterval)
		}
	}()
	logger.Info("smith_agent: input analysis loop started (progressive mode)",
		"first_min_samples", firstAnalysisMinSamples,
		"first_max_wait", firstAnalysisMaxWait.String(),
		"poll_interval", inputAnalysisPollInterval.String(),
		"enhanced_threshold", enhancedStageThreshold,
		"stable_threshold", stableStageThreshold,
		"full_refresh_interval", inputAnalysisInterval.String())
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

func getInputAnalysisCache(inputId string) (inputAnalysisCache, bool) {
	raw, ok := GetInputAnalysis(inputId)
	if !ok || raw == "" {
		return inputAnalysisCache{}, false
	}
	var cached inputAnalysisCache
	if err := json.Unmarshal([]byte(raw), &cached); err != nil {
		return inputAnalysisCache{}, false
	}
	if cached.ArchitectOutput == "" {
		return inputAnalysisCache{}, false
	}
	return cached, true
}

func runProgressiveAnalysis(startedAt time.Time) {
	ctx := context.Background()
	now := time.Now()
	stored := 0

	project.ForEachInput(func(inputId string, _ *input.Input) bool {
		samples, count := getInputSamplesForAnalysis(inputId)
		decision := shouldAnalyzeInput(startedAt, now, inputId, count)
		if !decision.ShouldAnalyze {
			return true
		}

		if analyzeAndStoreInput(ctx, inputId, samples, count, decision.Stage) {
			stored++
			logger.Info("smith_agent input analysis: updated",
				"input", inputId,
				"samples", count,
				"stage", string(decision.Stage),
				"reason", decision.Reason)
		}
		return true
	})

	if stored > 0 {
		logger.Info("smith_agent: progressive analysis cycle complete", "stored", stored)
	}
}

func shouldAnalyzeInput(startedAt, now time.Time, inputId string, sampleCount int) analyzeDecision {
	if sampleCount <= 0 {
		return analyzeDecision{ShouldAnalyze: false}
	}

	targetStage := stageBySampleCount(sampleCount)
	cached, hasCached := getInputAnalysisCache(inputId)
	if !hasCached {
		if sampleCount >= firstAnalysisMinSamples {
			return analyzeDecision{
				ShouldAnalyze: true,
				Stage:         targetStage,
				Reason:        "first_analysis_min_samples",
			}
		}
		if now.Sub(startedAt) >= firstAnalysisMaxWait {
			return analyzeDecision{
				ShouldAnalyze: true,
				Stage:         targetStage,
				Reason:        "first_analysis_max_wait",
			}
		}
		logger.Debug("smith_agent: waiting for first analysis",
			"input", inputId,
			"current", sampleCount,
			"need", firstAnalysisMinSamples,
			"max_wait_remaining", firstAnalysisMaxWait-now.Sub(startedAt))
		return analyzeDecision{ShouldAnalyze: false}
	}

	currentStage := normalizeStage(cached.Stage, cached.SampleCount)
	lastAnalyzedAt := cached.analyzedAtTime()

	if stageRank(targetStage) > stageRank(currentStage) && sampleCount > cached.SampleCount {
		return analyzeDecision{
			ShouldAnalyze: true,
			Stage:         targetStage,
			Reason:        "stage_upgrade",
		}
	}

	if !lastAnalyzedAt.IsZero() && now.Sub(lastAnalyzedAt) >= inputAnalysisInterval {
		return analyzeDecision{
			ShouldAnalyze: true,
			Stage:         targetStage,
			Reason:        "periodic_full_refresh",
		}
	}

	// Same-stage enrichment: if sample volume increases materially, refresh earlier.
	if sampleCount >= cached.SampleCount+inputAnalysisSampleGrowthStep {
		if lastAnalyzedAt.IsZero() || now.Sub(lastAnalyzedAt) >= inputAnalysisEnhancementMinInterval {
			nextStage := currentStage
			if stageRank(targetStage) > stageRank(currentStage) {
				nextStage = targetStage
			}
			return analyzeDecision{
				ShouldAnalyze: true,
				Stage:         nextStage,
				Reason:        "sample_growth_refresh",
			}
		}
	}

	return analyzeDecision{ShouldAnalyze: false}
}

func stageBySampleCount(sampleCount int) analysisStage {
	if sampleCount >= stableStageThreshold {
		return analysisStageStable
	}
	if sampleCount >= enhancedStageThreshold {
		return analysisStageEnhanced
	}
	return analysisStageInitial
}

func stageRank(stage analysisStage) int {
	switch stage {
	case analysisStageStable:
		return 3
	case analysisStageEnhanced:
		return 2
	default:
		return 1
	}
}

func normalizeStage(stage string, sampleCount int) analysisStage {
	switch analysisStage(stage) {
	case analysisStageInitial, analysisStageEnhanced, analysisStageStable:
		return analysisStage(stage)
	default:
		// Backward compatibility for old payloads without stage metadata.
		return stageBySampleCount(sampleCount)
	}
}

func getInputSamplesForAnalysis(inputId string) ([]common.SampleData, int) {
	sampler := common.GetSampler("input." + inputId)
	if sampler == nil {
		return nil, 0
	}

	samplesBySeq := sampler.GetSamples()
	if len(samplesBySeq) == 0 {
		return nil, 0
	}

	all := make([]common.SampleData, 0, 128)
	total := 0
	for _, list := range samplesBySeq {
		total += len(list)
		all = append(all, list...)
	}
	if len(all) == 0 {
		return nil, total
	}

	// Keep deterministic chronological order for LLM context.
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.Before(all[j].Timestamp)
	})
	return all, total
}

// analyzeAndStoreInput runs architect analysis on samples of an input
// and stores the result in Redis. Returns true if analysis was stored successfully.
func analyzeAndStoreInput(ctx context.Context, inputId string, samples []common.SampleData, sampleCount int, stage analysisStage) bool {
	if len(samples) == 0 {
		return false
	}

	// Pass all collected samples to help the model infer stable field semantics.
	samplePayload := make([]interface{}, 0, len(samples))
	for i := range samples {
		if samples[i].Data != nil {
			samplePayload = append(samplePayload, samples[i].Data)
		}
	}
	if len(samplePayload) == 0 {
		return false
	}

	sampleJSON, err := json.Marshal(samplePayload)
	if err != nil {
		logger.Warn("smith_agent input analysis: marshal sample failed", "input", inputId, "error", err)
		return false
	}

	result, err := AnalyzeData(ctx, inputId, string(sampleJSON))
	if err != nil {
		if isLLMConfigurationError(err) {
			logger.Error("smith_agent input analysis: analyze failed", "input", inputId, "error", err)
		} else {
			logger.Warn("smith_agent input analysis: analyze failed", "input", inputId, "error", err)
		}
		return false
	}

	payload := map[string]interface{}{
		"ArchitectOutput": result.ArchitectOutput,
		"analyzed_at":     time.Now().UTC().Format(time.RFC3339),
		"sample_count":    sampleCount,
		"stage":           string(stage),
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

	return true
}
