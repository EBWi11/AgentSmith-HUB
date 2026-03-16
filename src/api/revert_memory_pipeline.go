package api

import (
	"AgentSmith-HUB/agent"
	"AgentSmith-HUB/cluster"
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/memory"
	"AgentSmith-HUB/project"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const revertMemoryExtractorAgentID = "revert_memory_extractor"

type revertMemoryPayload struct {
	Scope             map[string]interface{} `json:"scope"`
	Feedback          map[string]interface{} `json:"feedback,omitempty"`
	OriginalOperation map[string]interface{} `json:"original_operation"`
	ProjectContext    map[string]interface{} `json:"project_context"`
	AgentContext      map[string]interface{} `json:"agent_context"`
	SampleData        map[string]interface{} `json:"sample_data"`
}

func triggerRevertMemoryExtraction(original common.OperationRecord, revertOperationID string) {
	go func() {
		if err := analyzeRevertIntoMemory(original, revertOperationID); err != nil {
			if stateErr := common.SetOperationAnalysisState(revertOperationID, "failed", err.Error()); stateErr != nil {
				logger.Warn("Failed to persist revert memory analysis state",
					"operation_id", revertOperationID,
					"error", stateErr,
				)
			}
			logger.Warn("Failed to analyze revert into memory",
				"operation_id", original.OperationID,
				"revert_operation_id", revertOperationID,
				"project_node_sequence", original.ProjectNodeSequence,
				"error", err,
			)
			return
		}
		if err := common.SetOperationAnalysisState(revertOperationID, "success", ""); err != nil {
			logger.Warn("Failed to persist revert memory analysis success state",
				"operation_id", revertOperationID,
				"error", err,
			)
		}
	}()
}

func triggerOperationCommentMemoryExtraction(commentOperationID string, original common.OperationRecord, comment string) {
	go func() {
		if err := analyzeCommentIntoMemory(commentOperationID, original, comment); err != nil {
			if stateErr := common.SetOperationAnalysisState(commentOperationID, "failed", err.Error()); stateErr != nil {
				logger.Warn("Failed to persist comment memory analysis state",
					"operation_id", commentOperationID,
					"error", stateErr,
				)
			}
			logger.Warn("Failed to analyze operation comment into memory",
				"operation_id", original.OperationID,
				"comment_operation_id", commentOperationID,
				"project_node_sequence", original.ProjectNodeSequence,
				"error", err,
			)
			return
		}
		if err := common.SetOperationAnalysisState(commentOperationID, "success", ""); err != nil {
			logger.Warn("Failed to persist comment memory analysis success state",
				"operation_id", commentOperationID,
				"error", err,
			)
		}
	}()
}

func analyzeRevertIntoMemory(original common.OperationRecord, revertOperationID string) error {
	revertRecord, err := common.GetOperationRecord(revertOperationID)
	if err != nil {
		return fmt.Errorf("load revert record: %w", err)
	}

	payload, scope, err := buildRevertMemoryPayload(original, revertRecord)
	if err != nil {
		return err
	}

	result, err := extractRevertMemory(payload)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Summary) == "" && len(result.AvoidPatterns) == 0 && len(result.PreferredPatterns) == 0 {
		return fmt.Errorf("extractor returned empty memory result")
	}

	err = persistScopedMemoryUpdate(scope, result, memory.RecentFeedback{
		FeedbackType:      "revert",
		OperationID:       original.OperationID,
		RevertOperationID: revertOperationID,
		RulesetID:         original.RulesetID,
		RuleID:            original.RuleID,
		Reason:            revertRecord.RevertReason,
		SourceOperationID: revertOperationID,
		CreatedAt:         revertRecord.Timestamp,
	}, revertOperationID)
	if err != nil {
		return err
	}

	logger.Info("Revert memory updated",
		"operation_id", original.OperationID,
		"revert_operation_id", revertOperationID,
		"project_node_sequence", scope.ProjectNodeSequence,
		"agent", scope.AgentID,
	)
	return nil
}

func analyzeCommentIntoMemory(commentOperationID string, record common.OperationRecord, comment string) error {
	payload, scope, err := buildOperationFeedbackPayload(record, map[string]interface{}{
		"type":         "comment",
		"operation_id": commentOperationID,
		"comment":      strings.TrimSpace(comment),
		"timestamp":    time.Now(),
	})
	if err != nil {
		return err
	}

	result, err := extractRevertMemory(payload)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Summary) == "" && len(result.AvoidPatterns) == 0 && len(result.PreferredPatterns) == 0 {
		return fmt.Errorf("extractor returned empty memory result")
	}

	err = persistScopedMemoryUpdate(scope, result, memory.RecentFeedback{
		FeedbackType:      "comment",
		OperationID:       record.OperationID,
		RulesetID:         record.RulesetID,
		RuleID:            record.RuleID,
		Reason:            strings.TrimSpace(comment),
		SourceOperationID: commentOperationID,
		CreatedAt:         timeNow(),
	}, commentOperationID)
	if err != nil {
		return err
	}

	logger.Info("Operation comment memory updated",
		"operation_id", record.OperationID,
		"comment_operation_id", commentOperationID,
		"project_node_sequence", scope.ProjectNodeSequence,
		"agent", scope.AgentID,
	)
	return nil
}

func buildRevertMemoryPayload(original, revertRecord common.OperationRecord) (map[string]interface{}, memory.Scope, error) {
	return buildOperationFeedbackPayload(original, map[string]interface{}{
		"type":                 "revert",
		"operation_id":         revertRecord.OperationID,
		"reverts_operation_id": revertRecord.RevertsOperationID,
		"comment":              revertRecord.RevertReason,
		"timestamp":            revertRecord.Timestamp,
	})
}

func buildOperationFeedbackPayload(original common.OperationRecord, feedback map[string]interface{}) (map[string]interface{}, memory.Scope, error) {
	scope := memory.Scope{
		AgentID:             original.AgentID,
		ProjectID:           strings.TrimSpace(original.ProjectID),
		ProjectNodeSequence: original.ProjectNodeSequence,
	}
	scope.InputIDs = extractInputIDs(scope.ProjectNodeSequence)

	if scope.ProjectID == "" {
		scope.ProjectID = findProjectIDForPNS(scope.ProjectNodeSequence)
	}

	var projectRaw string
	var projectSummary map[string]interface{}
	if scope.ProjectID != "" {
		if proj, exists := project.GetProject(scope.ProjectID); exists && proj != nil {
			projectRaw = strings.TrimSpace(proj.Config.RawConfig)
			projectSummary = map[string]interface{}{
				"id":                 proj.Id,
				"flow_nodes":         len(proj.FlowNodes),
				"testing":            proj.Testing,
				"project_config_raw": truncateString(projectRaw, 6000),
			}
		}
	}

	extractorPayload := revertMemoryPayload{
		Scope: map[string]interface{}{
			"agent_id":              scope.AgentID,
			"project_id":            scope.ProjectID,
			"project_node_sequence": scope.ProjectNodeSequence,
			"input_ids":             scope.InputIDs,
		},
		Feedback: feedback,
		OriginalOperation: map[string]interface{}{
			"operation_id":          original.OperationID,
			"action_scope":          original.ActionScope,
			"action_type":           original.ActionType,
			"source":                original.Source,
			"ruleset_id":            original.RulesetID,
			"rule_id":               original.RuleID,
			"agent_id":              original.AgentID,
			"project_node_sequence": original.ProjectNodeSequence,
			"agent_reason_summary":  original.AgentReasonSummary,
			"old_content":           truncateString(strings.TrimSpace(original.OldContent), 6000),
			"new_content":           truncateString(strings.TrimSpace(original.NewContent), 6000),
			"details":               original.Details,
		},
		ProjectContext: map[string]interface{}{
			"project":   projectSummary,
			"input_ids": scope.InputIDs,
		},
		AgentContext: map[string]interface{}{
			"agent_id":              original.AgentID,
			"project_node_sequence": original.ProjectNodeSequence,
		},
		SampleData: collectRelevantSamples(scope.ProjectNodeSequence, original.AgentID, scope.InputIDs),
	}

	if strings.TrimSpace(original.AgentID) != "" {
		if ag, exists := project.GetAgent(original.AgentID); exists && ag != nil && ag.Config != nil {
			extractorPayload.AgentContext["model"] = ag.Config.Model
			extractorPayload.AgentContext["tools"] = ag.Config.Tools
			extractorPayload.AgentContext["skills"] = ag.Config.Skills
			extractorPayload.AgentContext["system_prompt"] = truncateString(strings.TrimSpace(ag.Config.SystemPrompt), 6000)
			extractorPayload.AgentContext["config_raw"] = truncateString(strings.TrimSpace(ag.RawConfig), 6000)
		}
	}

	return map[string]interface{}{
		"scope":              extractorPayload.Scope,
		"feedback":           extractorPayload.Feedback,
		"original_operation": extractorPayload.OriginalOperation,
		"project_context":    extractorPayload.ProjectContext,
		"agent_context":      extractorPayload.AgentContext,
		"sample_data":        extractorPayload.SampleData,
	}, scope, nil
}

func persistScopedMemoryUpdate(scope memory.Scope, result memory.ExtractorResult, feedback memory.RecentFeedback, sourceOperationID string) error {
	existing, err := memory.LoadPNSMemory(scope.ProjectNodeSequence)
	if err != nil {
		return fmt.Errorf("load existing memory: %w", err)
	}
	previousRaw, hadPrevious, err := memory.LoadPNSMemoryRaw(scope.ProjectNodeSequence)
	if err != nil {
		return fmt.Errorf("load existing memory raw: %w", err)
	}
	updated := memory.BuildUpdatedFeedbackConfig(existing, scope, result, feedback, sourceOperationID)
	raw, err := memory.MarshalConfig(updated)
	if err != nil {
		return fmt.Errorf("marshal updated memory: %w", err)
	}
	if err := memory.SavePNSMemoryRaw(scope.ProjectNodeSequence, raw); err != nil {
		return fmt.Errorf("persist memory: %w", err)
	}
	if err := publishMemorySync(scope.ProjectNodeSequence, raw); err != nil {
		if rollbackErr := rollbackScopedMemory(scope.ProjectNodeSequence, previousRaw, hadPrevious); rollbackErr != nil {
			return fmt.Errorf("publish memory sync: %w (rollback failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("publish memory sync: %w", err)
	}
	return nil
}

func rollbackScopedMemory(pns, previousRaw string, hadPrevious bool) error {
	if hadPrevious {
		return memory.SavePNSMemoryRaw(pns, previousRaw)
	}
	return memory.DeletePNSMemory(pns)
}

func publishMemorySync(pns, raw string) error {
	if !common.IsCurrentNodeLeader() || cluster.GlobalInstructionManager == nil {
		return nil
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := cluster.GlobalInstructionManager.PublishComponentPushChange("memory", pns, raw, nil); err != nil {
		return fmt.Errorf("publish memory sync: %w", err)
	}
	return nil
}

func timeNow() time.Time {
	return time.Now()
}

func extractRevertMemory(payload map[string]interface{}) (memory.ExtractorResult, error) {
	var result memory.ExtractorResult

	extractor, exists := project.GetAgent(revertMemoryExtractorAgentID)
	if !exists || extractor == nil {
		path := filepath.Join(common.Config.ConfigRoot, "agent", revertMemoryExtractorAgentID+".yaml")
		var err error
		extractor, err = agent.NewAgent(path, "", revertMemoryExtractorAgentID)
		if err != nil {
			return result, fmt.Errorf("load extractor agent: %w", err)
		}
	}

	msgCopy, _ := common.MapDeepCopyAction(payload).(map[string]interface{})
	if msgCopy == nil {
		msgCopy = payload
	}
	processed, _ := extractor.ProcessMessageWithTrace(msgCopy)
	llmBlock, ok := processed["llm"].(map[string]interface{})
	if !ok {
		return result, fmt.Errorf("extractor response missing llm block")
	}
	agentBlock, ok := llmBlock[extractor.Id].(map[string]interface{})
	if !ok {
		return result, fmt.Errorf("extractor response missing agent output")
	}

	if summary, ok := agentBlock["summary"].(string); ok {
		result.Summary = strings.TrimSpace(summary)
	}
	if category, ok := agentBlock["category"].(string); ok {
		result.Category = strings.TrimSpace(category)
	}
	if confidence, ok := agentBlock["confidence"].(float64); ok {
		result.Confidence = confidence
	}
	result.Signals = toStringSlice(agentBlock["signals"])
	result.AvoidPatterns = toStringSlice(agentBlock["avoid_patterns"])
	result.PreferredPatterns = toStringSlice(agentBlock["preferred_patterns"])
	result.InputIDs = toStringSlice(agentBlock["input_ids"])

	return result, nil
}

func collectRelevantSamples(pns, agentID string, inputIDs []string) map[string]interface{} {
	out := map[string]interface{}{
		"agent_samples": make([]common.SampleData, 0),
		"input_samples": map[string][]common.SampleData{},
	}

	if strings.TrimSpace(agentID) != "" {
		agentSampler := common.GetSampler(agentID)
		if agentSampler != nil {
			if samplesByPNS, err := getSamplesForPNS(agentSampler, pns); err == nil {
				out["agent_samples"] = samplesByPNS
			}
		}
	}

	inputSamples := make(map[string][]common.SampleData)
	for _, inputID := range inputIDs {
		sampler := common.GetSampler("input." + inputID)
		if sampler == nil {
			continue
		}
		samples := collectLatestSamplesAcrossKeys(sampler.GetSamples(), 5)
		if len(samples) > 0 {
			inputSamples[inputID] = samples
		}
	}
	out["input_samples"] = inputSamples
	return out
}

func getSamplesForPNS(sampler *common.Sampler, pns string) ([]common.SampleData, error) {
	samplesByPNS := sampler.GetSamples()
	if len(samplesByPNS) == 0 {
		return nil, nil
	}
	if samples, exists := samplesByPNS[pns]; exists {
		return trimSamples(samples, 5), nil
	}
	for key, samples := range samplesByPNS {
		if strings.EqualFold(key, pns) {
			return trimSamples(samples, 5), nil
		}
	}
	return collectLatestSamplesAcrossKeys(samplesByPNS, 5), nil
}

func collectLatestSamplesAcrossKeys(samplesByKey map[string][]common.SampleData, limit int) []common.SampleData {
	all := make([]common.SampleData, 0)
	for _, samples := range samplesByKey {
		all = append(all, samples...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].Timestamp.After(all[j].Timestamp)
	})
	return trimSamples(all, limit)
}

func trimSamples(samples []common.SampleData, limit int) []common.SampleData {
	if len(samples) == 0 {
		return nil
	}
	if len(samples) > limit {
		return samples[:limit]
	}
	return samples
}

func extractInputIDs(pns string) []string {
	components := common.ParseProjectNodeSequence(pns)
	inputIDs := make([]string, 0)
	for _, component := range components {
		if component.Type == "input" {
			inputIDs = append(inputIDs, component.ID)
		}
	}
	return uniqueStrings(inputIDs)
}

func findProjectIDForPNS(pns string) string {
	projectID := ""
	project.ForEachProject(func(id string, proj *project.Project) bool {
		for _, node := range proj.FlowNodes {
			if node.FromPNS == pns || node.ToPNS == pns {
				projectID = id
				return false
			}
		}
		return true
	})
	return projectID
}

func toStringSlice(v interface{}) []string {
	switch vv := v.(type) {
	case []string:
		return uniqueStrings(vv)
	case []interface{}:
		out := make([]string, 0, len(vv))
		for _, item := range vv {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return uniqueStrings(out)
	default:
		return nil
	}
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("... (truncated, %d bytes total)", len(s))
}
