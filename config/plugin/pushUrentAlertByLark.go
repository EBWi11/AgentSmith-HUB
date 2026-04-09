package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"time"
)

func getTenantAccessToken(app_id string, app_secret string) (string, error) {
	url := "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"

	payload := "{\"app_id\": \"" + app_id + "\",\"app_secret\": \"" + app_secret + "\"}"

	client := &http.Client{}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(body, &jsonData); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	token, ok := jsonData["tenant_access_token"].(string)
	if !ok || token == "" {
		return "", fmt.Errorf("tenant_access_token not found in response: %s", string(body))
	}
	return token, nil
}

// buildCardContent builds a Feishu interactive card from alert event data.
// Priority fields are shown first; remaining non-blank string fields are
// appended alphabetically. The header color reflects harm_level severity.
func buildCardContent(data map[string]interface{}) (string, error) {
	isBlank := func(v interface{}) bool {
		s, ok := v.(string)
		if !ok {
			return true
		}
		s = strings.TrimSpace(s)
		return s == "" || s == "-" || s == "-5" || s == "null"
	}

	// Header color based on harm_level
	template := "blue"
	if level, ok := data["harm_level"].(string); ok {
		switch strings.ToLower(level) {
		case "critical":
			template = "red"
		case "high":
			template = "orange"
		case "medium":
			template = "orange"
		case "low", "basic":
			template = "yellow"
		}
	}

	// Header title
	title := "🚨 安全告警"
	for _, k := range []string{"alert_desc", "rule_name"} {
		if v, ok := data[k]; ok && !isBlank(v) {
			title = fmt.Sprintf("🚨 %v", v)
			break
		}
	}

	// Priority fields rendered as markdown key-value pairs
	type field struct{ key, label string }
	priority := []field{
		{"harm_level", "危害级别"},
		{"alert_level", "告警级别"},
		{"data_type_str", "数据类型"},
		{"timestamp", "时间"},
		{"hostname", "主机"},
		{"container_name", "容器"},
		{"image_name", "镜像"},
		{"username", "用户"},
		{"uid", "UID"},
		{"exe", "进程"},
		{"argv", "命令行"},
		{"pid", "PID"},
		{"ppid", "父PID"},
		{"pid_tree", "进程树"},
		{"rule_name", "规则"},
		{"_hub_hit_rule_id", "规则ID"},
		{"_hub_input", "输入源"},
	}

	shown := map[string]bool{"alert_desc": true, "alert_detail": true}
	var mainLines []string
	for _, f := range priority {
		shown[f.key] = true
		v, ok := data[f.key]
		if !ok || isBlank(v) {
			continue
		}
		mainLines = append(mainLines, fmt.Sprintf("**%s**: %v", f.label, v))
	}

	// Remaining non-blank string fields, sorted alphabetically
	var rest []string
	for k, v := range data {
		if shown[k] || isBlank(v) {
			continue
		}
		rest = append(rest, k)
	}
	sort.Strings(rest)
	var extraLines []string
	for _, k := range rest {
		extraLines = append(extraLines, fmt.Sprintf("**%s**: %v", k, data[k]))
	}

	// Assemble card elements
	elements := []interface{}{}

	if len(mainLines) > 0 {
		elements = append(elements, map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": strings.Join(mainLines, "\n"),
			},
		})
	}

	if len(extraLines) > 0 {
		elements = append(elements, map[string]interface{}{"tag": "hr"})
		elements = append(elements, map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": strings.Join(extraLines, "\n"),
			},
		})
	}

	if v, ok := data["alert_detail"]; ok && !isBlank(v) {
		elements = append(elements, map[string]interface{}{"tag": "hr"})
		elements = append(elements, map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**告警详情**\n%v", v),
			},
		})
	}

	card := map[string]interface{}{
		"config": map[string]interface{}{"wide_screen_mode": true},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": title,
			},
			"template": template,
		},
		"elements": elements,
	}

	b, err := json.Marshal(card)
	return string(b), err
}

// pushMessage sends a message to a Feishu chat.
// msgType is "interactive" (card) or "text"; content is the pre-built content string.
func pushMessage(msgType string, content string, receive_id string, token string) (string, error) {
	urlstr := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id"

	msg := map[string]string{
		"receive_id": receive_id,
		"msg_type":   msgType,
		"content":    content,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal message: %w", err)
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", urlstr, bytes.NewBuffer(msgBytes))
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(body, &jsonData); err != nil {
		return "", fmt.Errorf("failed to parse push response: %w", err)
	}

	dataField, ok := jsonData["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("missing or invalid 'data' field in response: %s", string(body))
	}
	messageID, ok := dataField["message_id"].(string)
	if !ok {
		return "", fmt.Errorf("missing 'message_id' in response data: %s", string(body))
	}
	return messageID, nil
}

func urgentPhone(msgid string, userid string, token string) error {
	urlStr := "https://open.feishu.cn/open-apis/im/v1/messages/" + msgid + "/urgent_phone" + "?user_id_type=open_id"

	client := &http.Client{}
	payload := bytes.NewBuffer([]byte(`{"user_id_list": ["` + userid + `"]}`))

	req, err := http.NewRequest("PATCH", urlStr, payload)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read urgent_phone response: %w", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse urgent_phone response: %w, body: %s", err, string(body))
	}

	if code, ok := resp["code"].(float64); ok && code != 0 {
		return fmt.Errorf("urgent_phone failed: code=%.0f, msg=%v, body=%s", code, resp["msg"], string(body))
	}

	return nil
}

func pushMessageAndPhone(msgType string, content string, appid string, appsk string, receive_id string, phonetimeleft int, phonetimeright int, userid string) error {
	token, err := getTenantAccessToken(appid, appsk)
	if err != nil {
		return err
	}

	msgId, err := pushMessage(msgType, content, receive_id, token)
	if err != nil {
		return err
	}

	// Use UTC+8 (Asia/Shanghai) explicitly to avoid server timezone mismatch.
	cst := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(cst)
	hour := now.Hour()

	// Skip phone alert during the quiet window [phonetimeleft, phonetimeright).
	if hour > phonetimeleft && hour < phonetimeright {
		return nil
	}

	time.Sleep(5 * time.Second)
	err = urgentPhone(msgId, userid, token)
	time.Sleep(10 * time.Second)
	_ = urgentPhone(msgId, userid, token)
	return err
}

func toInt(v interface{}) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int8:
		return int(n), nil
	case int16:
		return int(n), nil
	case int32:
		return int(n), nil
	case int64:
		return int(n), nil
	case float32:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

func Eval(args ...interface{}) (bool, error) {
	if len(args) < 7 {
		return false, errors.New("plugin requires 7 arguments: data, appid, appsk, receive_id, phonetimeleft, phonetimeright, userid")
	}

	// args[0]: map → interactive card; string → plain text
	var msgType, content string
	switch v := args[0].(type) {
	case map[string]interface{}:
		cardContent, err := buildCardContent(v)
		if err != nil {
			return false, fmt.Errorf("failed to build card: %w", err)
		}
		msgType = "interactive"
		content = cardContent
	case string:
		textBytes, err := json.Marshal(map[string]string{"text": v})
		if err != nil {
			return false, fmt.Errorf("failed to marshal text: %w", err)
		}
		msgType = "text"
		content = string(textBytes)
	default:
		return false, fmt.Errorf("args[0] must be a map or string, got %T", args[0])
	}

	appid, ok := args[1].(string)
	if !ok {
		return false, fmt.Errorf("args[1] (appid) must be a string, got %T", args[1])
	}
	appsk, ok := args[2].(string)
	if !ok {
		return false, fmt.Errorf("args[2] (appsk) must be a string, got %T", args[2])
	}
	receive_id, ok := args[3].(string)
	if !ok {
		return false, fmt.Errorf("args[3] (receive_id) must be a string, got %T", args[3])
	}

	phonetimeleft, err := toInt(args[4])
	if err != nil {
		return false, fmt.Errorf("args[4] (phonetimeleft): %w", err)
	}
	phonetimeright, err := toInt(args[5])
	if err != nil {
		return false, fmt.Errorf("args[5] (phonetimeright): %w", err)
	}

	userid, ok := args[6].(string)
	if !ok {
		return false, fmt.Errorf("args[6] (userid) must be a string, got %T", args[6])
	}

	if err := pushMessageAndPhone(msgType, content, appid, appsk, receive_id, phonetimeleft, phonetimeright, userid); err != nil {
		return false, err
	}
	return true, nil
}
