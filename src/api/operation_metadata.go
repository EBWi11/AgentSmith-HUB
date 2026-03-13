package api

import (
	"AgentSmith-HUB/common"
	"encoding/json"
	"fmt"
	"strings"
)

const rulesetPendingOperationMetaPrefix = "cluster:pending:ruleset_op_meta:"

func rulesetPendingOperationMetaKey(rulesetID string) string {
	return rulesetPendingOperationMetaPrefix + rulesetID
}

func savePendingRulesetOperationMeta(rulesetID string, record common.OperationRecord) error {
	if strings.TrimSpace(rulesetID) == "" {
		return fmt.Errorf("ruleset id is required")
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = common.RedisSet(rulesetPendingOperationMetaKey(rulesetID), string(raw), 7*24*60*60)
	return err
}

func loadPendingRulesetOperationMeta(rulesetID string) (*common.OperationRecord, error) {
	if strings.TrimSpace(rulesetID) == "" {
		return nil, fmt.Errorf("ruleset id is required")
	}
	raw, err := common.RedisGet(rulesetPendingOperationMetaKey(rulesetID))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var record common.OperationRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func clearPendingRulesetOperationMeta(rulesetID string) {
	if strings.TrimSpace(rulesetID) == "" {
		return
	}
	_ = common.RedisDel(rulesetPendingOperationMetaKey(rulesetID))
}
