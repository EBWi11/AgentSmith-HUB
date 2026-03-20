package add_rule

import (
	"AgentSmith-HUB/common"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const requestTimeout = 30 * time.Second

// Eval calls the Hub API to add a rule to a ruleset.
//
// Args:
//   rulesetId   string  — target ruleset ID
//   ruleContent string  — a single <rule>...</rule> XML element
//   autoApply   bool    — (optional, default true) apply the change immediately;
//                         pass false to leave the rule in pending state for human review
//
// Returns (responseBody, success, error).
// On API error (e.g. verify failure), responseBody contains the details so the agent can fix and retry.
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) < 2 {
		return nil, false, fmt.Errorf("addRule requires at least 2 arguments: rulesetId, ruleContent [, autoApply]")
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

	extractRuleID := func(xml string) string {
		// Best-effort: capture <rule ... id="..."> attribute value.
		// This is only for returning a compact success payload; backend validation is authoritative.
		re := regexp.MustCompile(`<rule\s+[^>]*id\s*=\s*["']([^"']+)["']`)
		m := re.FindStringSubmatch(xml)
		if len(m) >= 2 {
			return strings.TrimSpace(m[1])
		}
		return ""
	}

	ruleID := extractRuleID(ruleContent)

	// autoApply defaults to true; caller can pass false to keep change in pending state.
	autoApply := true
	if len(args) >= 3 {
		if v, ok := args[2].(bool); ok {
			autoApply = v
		}
	}

	baseURL, token, err := resolveHubAccess()
	if err != nil {
		return nil, false, err
	}

	addResp, err := postJSON(baseURL+"/rulesets/"+rulesetId+"/rules", token, map[string]string{"rule_raw": ruleContent})
	if err != nil {
		return addResp, false, err
	}

	if !autoApply {
		result, _ := json.Marshal(map[string]interface{}{
			"status":  "pending",
			"rule_id": ruleID,
			"message": "Rule staged successfully",
		})
		return string(result), true, nil
	}

	applyResp, err := postJSON(baseURL+"/apply-single-change", token, map[string]string{"type": "ruleset", "id": rulesetId})
	if err != nil {
		// apply error: keep details, but still return a compact object.
		result, _ := json.Marshal(map[string]interface{}{
			"status":     "pending",
			"rule_id":    ruleID,
			"message":    "Rule added but apply failed",
			"apply_error": err.Error(),
			"apply_resp": applyResp,
		})
		return string(result), false, err
	}

	_ = applyResp
	result, _ := json.Marshal(map[string]interface{}{
		"status":  "applied",
		"rule_id": ruleID,
		"message": "Rule applied successfully",
	})
	return string(result), true, nil
}

// resolveHubAccess returns the leader base URL and auth token, or an error if unavailable.
func resolveHubAccess() (baseURL, token string, err error) {
	baseURL = common.GetLeaderAPIBaseURL()
	if baseURL == "" {
		return "", "", fmt.Errorf("leader API address not available: leader may not have written cluster:leader:node_id to Redis yet")
	}
	if common.Config != nil {
		token = strings.TrimSpace(common.Config.Token)
	}
	if token == "" {
		return "", "", fmt.Errorf("Hub token not configured; addRule plugin requires token in config")
	}
	return baseURL, token, nil
}

// postJSON sends a POST request with a JSON body and returns the response body string.
// Returns (body, nil) on 2xx, (body, error) on 4xx/5xx or network error.
func postJSON(url, token string, payload interface{}) (string, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("token", token)

	resp, err := (&http.Client{Timeout: requestTimeout}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	respStr := string(respBody)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return respStr, nil
	}
	return respStr, fmt.Errorf("API returned %d: %s", resp.StatusCode, respStr)
}
