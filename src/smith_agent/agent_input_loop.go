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
	inputAnalysisNoDataInterval = 5 * time.Minute // when no sample data yet, retry soon
	inputAnalysisStartDelay     = 1 * time.Minute  // first run shortly after start
)

// StartInputAnalysisLoop starts a goroutine that, when LLM is ready and node is leader,
// runs: for each input, reads the latest sample data, runs architect analysis,
// and stores the result in Redis (TTL 30 days, overwritten on next run).
// If at startup there is no sample data, it retries every 5 min until some input has data, then runs immediately; thereafter runs daily.
func StartInputAnalysisLoop() {
	go func() {
		time.Sleep(inputAnalysisStartDelay)
		interval := inputAnalysisNoDataInterval // start with short interval until we see data

		for {
			if !Ready() {
				time.Sleep(interval)
				continue
			}
			if !common.IsCurrentNodeLeader() {
				time.Sleep(interval)
				continue
			}
			stored := runInputAnalysisOnce()
			if stored > 0 {
				interval = inputAnalysisInterval // had data, switch to daily
			} else {
				interval = inputAnalysisNoDataInterval // no data yet, retry soon
			}
			time.Sleep(interval)
		}
	}()
	logger.Info("smith_agent: input analysis loop started (when LLM ready on leader; retries soon if no sample data)")
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

// runInputAnalysisOnce runs analysis for each input that has sample data; returns how many results were stored in Redis.
func runInputAnalysisOnce() int {
	ctx := context.Background()
	stored := 0
	project.ForEachInput(func(inputId string, _ *input.Input) bool {
		sampler := common.GetSampler("input." + inputId)
		if sampler == nil {
			return true
		}
		samplesBySeq := sampler.GetSamples()
		if len(samplesBySeq) == 0 {
			return true
		}
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
			return true
		}
		sampleJSON, err := json.Marshal(latest.Data)
		if err != nil {
			logger.Warn("smith_agent input analysis: marshal sample failed", "input", inputId, "error", err)
			return true
		}
		result, err := AnalyzeData(ctx, inputId, string(sampleJSON))
		if err != nil {
			logger.Warn("smith_agent input analysis: analyze failed", "input", inputId, "error", err)
			return true
		}
		// Include analyzed_at for API/frontend display
		payload := map[string]interface{}{
			"ArchitectOutput": result.ArchitectOutput,
			"analyzed_at":     time.Now().UTC().Format(time.RFC3339),
		}
		resultJSON, err := json.Marshal(payload)
		if err != nil {
			logger.Warn("smith_agent input analysis: marshal result failed", "input", inputId, "error", err)
			return true
		}
		key := redisKeyPrefixInputAnalysis + inputId
		_, err = common.RedisSet(key, string(resultJSON), inputAnalysisTTLSeconds)
		if err != nil {
			logger.Warn("smith_agent input analysis: redis set failed", "input", inputId, "error", err)
			return true
		}
		stored++
		logger.Info("smith_agent input analysis: stored", "input", inputId)
		return true
	})
	return stored
}
