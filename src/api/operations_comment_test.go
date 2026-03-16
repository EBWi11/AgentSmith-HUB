package api

import (
	"testing"

	"AgentSmith-HUB/common"
)

func TestBuildCommentOperationRecord_PendingWhenAgentScopePresent(t *testing.T) {
	original := common.OperationRecord{
		OperationID:         "op_original_1",
		ComponentType:       "ruleset",
		ComponentID:         "cwpp_whitelist",
		ProjectID:           "project-a",
		ActionScope:         "ruleset_rule",
		ActionType:          "add_rule",
		RulesetID:           "cwpp_whitelist",
		RuleID:              "rule-1",
		AgentID:             "alert_reviewer",
		ProjectNodeSequence: "project-a.AGENT.alert_reviewer",
		ToolName:            "addRule",
	}

	record := buildCommentOperationRecord(original, "scope too broad")

	if record.Type != common.OpTypeOperationComment {
		t.Fatalf("unexpected operation type: %s", record.Type)
	}
	if record.ActionType != "comment" {
		t.Fatalf("unexpected action type: %s", record.ActionType)
	}
	if record.Source != "human" {
		t.Fatalf("unexpected source: %s", record.Source)
	}
	if record.FeedbackForOperationID != original.OperationID {
		t.Fatalf("feedback_for_operation_id mismatch: got %q want %q", record.FeedbackForOperationID, original.OperationID)
	}
	if record.FeedbackComment != "scope too broad" {
		t.Fatalf("unexpected feedback comment: %q", record.FeedbackComment)
	}
	if record.AnalysisStatus != "pending" {
		t.Fatalf("unexpected analysis status: %q", record.AnalysisStatus)
	}
	if record.AnalysisError != "" {
		t.Fatalf("analysis error should be empty, got %q", record.AnalysisError)
	}
	if record.ProjectNodeSequence != original.ProjectNodeSequence {
		t.Fatalf("pns mismatch: got %q want %q", record.ProjectNodeSequence, original.ProjectNodeSequence)
	}
}

func TestBuildCommentOperationRecord_SkippedWhenAgentScopeMissing(t *testing.T) {
	original := common.OperationRecord{
		OperationID:   "op_original_2",
		ComponentType: "ruleset",
		ComponentID:   "cwpp_whitelist",
		RulesetID:     "cwpp_whitelist",
		RuleID:        "rule-2",
	}

	record := buildCommentOperationRecord(original, "needs tighter constraint")

	if record.AnalysisStatus != "skipped" {
		t.Fatalf("unexpected analysis status: %q", record.AnalysisStatus)
	}
	if record.AnalysisError == "" {
		t.Fatal("analysis error should be populated when agent scope is missing")
	}
	if record.Revertible {
		t.Fatal("comment operation should not be revertible")
	}
}
