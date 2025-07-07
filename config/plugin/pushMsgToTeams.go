package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
)

func formatMapToReadableString(data map[string]interface{}) string {
	b, _ := json.MarshalIndent(data, "", "  ")
	return string(b)
}

// Eval pushes message to Microsoft Teams via Incoming Webhook
// Param1: WebhookURL
// Param2: (unused)
func Eval(WebhookURL string, _ string, data map[string]interface{}) (bool, error) {
	if WebhookURL == "" {
		return false, errors.New("webhook URL required")
	}
	text := formatMapToReadableString(data)
	payload := map[string]interface{}{
		"text": text,
	}
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
