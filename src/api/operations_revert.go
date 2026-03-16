package api

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"AgentSmith-HUB/project"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

type revertOperationRequest struct {
	Comment string `json:"comment"`
}

func revertOperation(c echo.Context) error {
	operationID := strings.TrimSpace(c.Param("id"))
	if operationID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "operation id is required"})
	}

	var req revertOperationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	record, err := common.GetOperationRecord(operationID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	if record.Type == common.OpTypeRevert || record.ActionType == "revert" || !record.Revertible {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "this operation cannot be reverted"})
	}
	if record.Status != "success" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "only successful operations can be reverted"})
	}
	if record.Reverted {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "this operation has already been reverted"})
	}
	if record.ComponentType != "ruleset" || record.RulesetID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "only applied ruleset operations can be reverted"})
	}
	if _, hasPending := project.GetRulesetNew(record.RulesetID); hasPending {
		return c.JSON(http.StatusConflict, map[string]string{"error": "ruleset has pending changes; clear them before reverting history"})
	}

	var currentRaw string
	if rs, exists := project.GetRuleset(record.RulesetID); exists {
		currentRaw = rs.RawConfig
	} else {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "ruleset not found"})
	}

	var revertedContent string
	switch record.ActionType {
	case "add_rule":
		if record.RuleID == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "add_rule history is missing rule_id"})
		}
		currentRuleRaw, currentRuleErr := extractRuleFromXML(currentRaw, record.RuleID)
		if currentRuleErr != nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": "rule is no longer present in current ruleset"})
		}
		originalRuleRaw := ""
		if record.Details != nil {
			if raw, ok := record.Details["rule_content"].(string); ok {
				originalRuleRaw = strings.TrimSpace(raw)
			}
		}
		if originalRuleRaw == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "add_rule history is missing original rule content"})
		}
		currentFP, err := simpleRuleFingerprintFromXML(currentRuleRaw)
		if err != nil {
			return c.JSON(http.StatusConflict, map[string]string{"error": "failed to fingerprint current rule for safe revert"})
		}
		originalFP, err := simpleRuleFingerprintFromXML(originalRuleRaw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "stored add_rule history is invalid"})
		}
		if currentFP != originalFP {
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "rule has changed since it was added; refusing to revert add_rule against modified content",
			})
		}
		revertedContent, err = removeRuleFromXML(currentRaw, record.RuleID)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
	case "update_ruleset":
		if strings.TrimSpace(record.OldContent) == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "update_ruleset history is missing old_content"})
		}
		if strings.TrimSpace(record.NewContent) != "" && strings.TrimSpace(currentRaw) != strings.TrimSpace(record.NewContent) {
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "ruleset has changed since this update; only latest full ruleset updates can be reverted safely",
			})
		}
		revertedContent = record.OldContent
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unsupported revert action: %s", record.ActionType)})
	}

	revertRecord := &common.OperationRecord{
		Type:               common.OpTypeRevert,
		ActionScope:        record.ActionScope,
		ActionType:         "revert",
		Source:             "human",
		RulesetID:          record.RulesetID,
		RuleID:             record.RuleID,
		Revertible:         false,
		RevertsOperationID: record.OperationID,
		RevertReason:       strings.TrimSpace(req.Comment),
	}

	_, revertOperationID, err := reloadComponentUnified(&ComponentReloadRequest{
		Type:        "ruleset",
		ID:          record.RulesetID,
		NewContent:  revertedContent,
		OldContent:  currentRaw,
		Source:      SourceChangePush,
		SkipVerify:  false,
		WriteToFile: true,
		Operation:   revertRecord,
	})
	if err != nil {
		logger.Error("Failed to revert operation", "operation_id", operationID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to revert operation: " + err.Error()})
	}

	if revertOperationID != "" {
		if err := common.MarkOperationReverted(record.OperationID, revertOperationID); err != nil {
			logger.Warn("Failed to mark operation reverted", "operation_id", record.OperationID, "revert_operation_id", revertOperationID, "error", err)
		}
		triggerRevertMemoryExtraction(record, revertOperationID)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":               "Operation reverted successfully",
		"reverted_operation_id": record.OperationID,
		"operation_id":          revertOperationID,
		"ruleset_id":            record.RulesetID,
		"rule_id":               record.RuleID,
	})
}
