package add_rule

import (
	"AgentSmith-HUB/common"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const requestTimeout = 30 * time.Second

// Eval calls the Hub API POST /rulesets/:id/rules to add a rule to a ruleset.
// Args: rulesetId string, ruleContent string (full rule XML).
// Returns (responseBody, success, error). On API error (e.g. verify failure), responseBody contains the error message so the agent can fix and retry.
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) < 2 {
		return nil, false, fmt.Errorf("addRule requires 2 arguments: rulesetId, ruleContent")
	}
	rulesetId, ok1 := args[0].(string)
	ruleContent, ok2 := args[1].(string)
	if !ok1 {
		return nil, false, fmt.Errorf("rulesetId must be a string")
	}
	if !ok2 {
		return nil, false, fmt.Errorf("ruleContent must be a string")
	}
	rulesetId = strings.TrimSpace(rulesetId)
	ruleContent = strings.TrimSpace(ruleContent)
	if rulesetId == "" {
		return nil, false, fmt.Errorf("rulesetId cannot be empty")
	}
	if ruleContent == "" {
		return nil, false, fmt.Errorf("ruleContent cannot be empty")
	}

	baseURL := common.GetLeaderAPIBaseURL()
	if baseURL == "" {
		return nil, false, fmt.Errorf("leader API address not available: leader may not have written cluster:leader:node_id to Redis yet")
	}
	token := ""
	if common.Config != nil {
		token = strings.TrimSpace(common.Config.Token)
	}
	if token == "" {
		return nil, false, fmt.Errorf("Hub token not configured; addRule plugin requires token in config")
	}

	url := baseURL + "/rulesets/" + rulesetId + "/rules"
	body := map[string]string{"rule_raw": ruleContent}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, false, err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("token", token)

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	respStr := string(respBody)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return respStr, true, nil
	}
	// 4xx/5xx: return body so agent can see verify error and fix
	return respStr, false, fmt.Errorf("API returned %d: %s", resp.StatusCode, respStr)
}
