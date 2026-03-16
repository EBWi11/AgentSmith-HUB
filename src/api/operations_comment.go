package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

type commentOperationRequest struct {
	Comment string `json:"comment"`
}

func buildCommentOperationRecord(record common.OperationRecord, comment string) common.OperationRecord {
	commentRecord := common.OperationRecord{
		Type:                   common.OpTypeOperationComment,
		ComponentType:          record.ComponentType,
		ComponentID:            record.ComponentID,
		ProjectID:              record.ProjectID,
		ActionScope:            record.ActionScope,
		ActionType:             "comment",
		Source:                 "human",
		RulesetID:              record.RulesetID,
		RuleID:                 record.RuleID,
		Revertible:             false,
		Status:                 "success",
		FeedbackComment:        comment,
		FeedbackForOperationID: record.OperationID,
		AgentID:                record.AgentID,
		AgentRunID:             record.AgentRunID,
		AgentSessionID:         record.AgentSessionID,
		ToolName:               record.ToolName,
		ToolCallID:             record.ToolCallID,
		ProjectNodeSequence:    record.ProjectNodeSequence,
		SourceEventID:          record.SourceEventID,
		AgentReasonSummary:     record.AgentReasonSummary,
	}
	if strings.TrimSpace(record.AgentID) != "" && strings.TrimSpace(record.ProjectNodeSequence) != "" {
		commentRecord.AnalysisStatus = "pending"
	} else {
		commentRecord.AnalysisStatus = "skipped"
		commentRecord.AnalysisError = "operation is missing agent or project node sequence context"
	}
	return commentRecord
}

func commentOperation(c echo.Context) error {
	operationID := strings.TrimSpace(c.Param("id"))
	if operationID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "operation id is required"})
	}

	var req commentOperationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	comment := strings.TrimSpace(req.Comment)
	if comment == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "comment is required"})
	}

	record, err := common.GetOperationRecord(operationID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	if record.OperationID == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "operation not found"})
	}
	if record.Type == common.OpTypeRevert || record.Type == common.OpTypeOperationComment || record.ActionType == "revert" || record.ActionType == "comment" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "comments can only be attached to original operations"})
	}

	commentRecord := buildCommentOperationRecord(record, comment)

	commentOperationID, err := common.RecordOperation(commentRecord)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if err := common.SetOperationFeedbackComment(operationID, comment); err != nil {
		logger.Warn("Failed to mirror feedback comment onto original operation",
			"operation_id", operationID,
			"comment_operation_id", commentOperationID,
			"error", err,
		)
	}
	record.FeedbackComment = comment
	if commentRecord.AnalysisStatus == "pending" {
		triggerOperationCommentMemoryExtraction(commentOperationID, record, comment)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":                   "Operation comment saved",
		"operation_id":              commentOperationID,
		"feedback_for_operation_id": operationID,
		"comment":                   comment,
		"analysis_status":           commentRecord.AnalysisStatus,
	})
}
