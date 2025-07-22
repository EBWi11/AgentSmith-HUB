package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
)

func formatMapToReadableString(data map[string]interface{}) string {
	if data == nil {
		return "{}"
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		flat, _ := json.Marshal(data)
		return string(flat)
	}
	return string(b)
}

// Eval pushes message to Microsoft Teams via Incoming Webhook
// Param1: WebhookURL
func Eval(WebhookURL string, data map[string]interface{}) (bool, error) {
	if WebhookURL == "" {
		return false, errors.New("webhook URL required")
	}

	text := formatMapToReadableString(data)

	// Microsoft Teams supports multiple message formats
	// Using MessageCard format for better presentation
	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": "0076D7",
		"summary":    "AgentSmith-HUB Alert",
		"sections": []map[string]interface{}{
			{
				"activityTitle":    "🔔 Security Alert",
				"activitySubtitle": "AgentSmith-HUB Detection",
				"text":             text,
				"markdown":         true,
			},
		},
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}

	resp, err := http.Post(WebhookURL, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return false, errors.New(string(body))
	}
	return true, nil
}
