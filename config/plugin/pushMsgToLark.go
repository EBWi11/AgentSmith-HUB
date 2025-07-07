package plugin

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// formatMapToReadableString converts map to pretty JSON
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

// Eval pushes message to Lark/Feishu via webhook.
// Param1: WebhookURL (the url without timestamp&sign)
// Param2: Secret (optional)
func Eval(WebhookURL string, Secret string, data map[string]interface{}) (bool, error) {
	if WebhookURL == "" {
		return false, errors.New("webhook URL is required")
	}

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	if Secret != "" {
		strToSign := fmt.Sprintf("%s\n%s", timestamp, Secret)
		h := hmac.New(sha256.New, []byte(Secret))
		h.Write([]byte(strToSign))
		sign := base64.StdEncoding.EncodeToString(h.Sum(nil))
		WebhookURL = fmt.Sprintf("%s&timestamp=%s&sign=%s", WebhookURL, timestamp, sign)
	}

	content := formatMapToReadableString(data)
	payload := map[string]interface{}{
		"msg_type": "text",
		"content":  map[string]string{"text": content},
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
