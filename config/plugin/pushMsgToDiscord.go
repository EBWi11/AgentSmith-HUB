// This file is compiled only when building the Discord plugin.

package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
)

// formatMapToReadableString converts map into a formatted JSON string
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

// Eval sends a map payload to a Discord channel via Webhook.
// Param1: WebhookURL (required)
// Param2: (unused, keep for signature compatibility)
func Eval(WebhookURL string, data map[string]interface{}) (bool, error) {
	if WebhookURL == "" {
		return false, errors.New("webhook URL is required")
	}
	content := formatMapToReadableString(data)
	payload := map[string]string{"content": content}
	b, _ := json.Marshal(payload)

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
