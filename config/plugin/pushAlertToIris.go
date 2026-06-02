package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type irisAsset struct {
	AssetName        string `json:"asset_name,omitempty"`
	AssetDescription string `json:"asset_description,omitempty"`
	AssetIP          string `json:"asset_ip,omitempty"`
	AssetDomain      string `json:"asset_domain,omitempty"`
}

type irisIOC struct {
	IOCValue       string `json:"ioc_value"`
	IOCDescription string `json:"ioc_description,omitempty"`
	IOCTLPID       int    `json:"ioc_tlp_id,omitempty"`
	IOCTypeID      int    `json:"ioc_type_id,omitempty"`
	IOCTags        string `json:"ioc_tags,omitempty"`
}

func Eval(BaseURL string, APIKey string, CustomerID int, ClassificationID int, data map[string]interface{}) (bool, error) {
	if strings.TrimSpace(BaseURL) == "" {
		return false, errors.New("BaseURL is required")
	}
	if strings.TrimSpace(APIKey) == "" {
		return false, errors.New("APIKey is required")
	}
	if CustomerID <= 0 {
		return false, errors.New("CustomerID must be greater than 0")
	}
	if ClassificationID <= 0 {
		return false, errors.New("ClassificationID must be greater than 0")
	}
	if data == nil {
		return false, errors.New("data is required")
	}

	payload := buildIrisAlertPayload(CustomerID, ClassificationID, data)
	body, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("failed to marshal IRIS alert payload: %w", err)
	}

	endpoint := strings.TrimRight(BaseURL, "/") + "/api/v2/alerts"
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	respBody, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("IRIS alert creation failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return true, nil
}

func buildIrisAlertPayload(customerID int, classificationID int, data map[string]interface{}) map[string]interface{} {
	title := firstNonBlank(data,
		"rule_name_us",
		"rule_name",
		"alert_desc_us",
		"alert_desc",
		"alarm_id",
	)
	if title == "" {
		title = "AgentSmith-HUB security alert"
	}

	sourceRef := firstNonBlank(data, "alarm_id", "trace_id", "span_trace_id", "_id")
	if sourceRef == "" {
		sourceRef = fmt.Sprintf("agentsmith-%d", time.Now().UnixNano())
	}

	payload := map[string]interface{}{
		"alert_title":             title,
		"alert_description":       buildIrisAlertDescription(data),
		"alert_source":            buildIrisAlertSource(data),
		"alert_source_ref":        sourceRef,
		"alert_source_event_time": buildIrisEventTime(data),
		"alert_severity_id":       irisSeverityID(data),
		"alert_customer_id":       customerID,
		"alert_classification_id": classificationID,
		"alert_source_content":    data,
		"alert_context":           buildIrisAlertContext(data),
		"alert_tags":              buildIrisTags(data),
	}

	if assets := buildIrisAssets(data); len(assets) > 0 {
		payload["alert_assets"] = assets
	}
	if iocs := buildIrisIOCs(data); len(iocs) > 0 {
		payload["alert_iocs"] = iocs
	}
	if sourceLink := firstNonBlank(data, "iris_source_link", "source_link", "alert_url"); sourceLink != "" {
		payload["alert_source_link"] = sourceLink
	}
	if statusID := intFromField(data, "iris_status_id"); statusID > 0 {
		payload["alert_status_id"] = statusID
	}

	return payload
}

func buildIrisAlertDescription(data map[string]interface{}) string {
	lines := []string{}
	add := func(label string, value string) {
		if value != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", label, value))
		}
	}

	add("Rule", firstNonBlank(data, "rule_name", "rule_name_us"))
	add("Rule ID", firstNonBlank(data, "RuleID", "rule_id", "_hub_hit_rule_id"))
	add("Severity", firstNonBlank(data, "harm_level", "risk"))
	add("Host", firstNonBlank(data, "hostname", "nodename", "agent_id"))
	add("User", firstNonBlank(data, "username", "uid"))
	add("Process", firstNonBlank(data, "exe", "comm"))
	add("Command line", firstNonBlank(data, "argv"))
	add("PID", firstNonBlank(data, "pid"))
	add("Parent PID", firstNonBlank(data, "ppid"))
	add("Network", firstNonBlank(data, "connect_info"))
	add("Source IP", firstNonBlank(data, "sip", "in_ipv4_list"))
	add("Destination IP", firstNonBlank(data, "dip"))
	add("Destination Port", firstNonBlank(data, "dport"))
	add("ATT&CK", firstNonBlank(data, "attack_id"))

	detail := firstNonBlank(data, "alert_detail", "alert_detail_us")
	if detail != "" {
		lines = append(lines, "", "Alert detail:", detail)
	}

	suggestion := firstNonBlank(data, "suggestion", "suggestion_us")
	if suggestion != "" {
		lines = append(lines, "", "Suggestion:", suggestion)
	}

	if len(lines) == 0 {
		raw, _ := json.MarshalIndent(data, "", "  ")
		return string(raw)
	}
	return strings.Join(lines, "\n")
}

func buildIrisAlertSource(data map[string]interface{}) string {
	parts := []string{"AgentSmith-HUB"}
	for _, key := range []string{"product", "type", "data_type_str_en", "cloud_provider"} {
		if value := stringValue(data[key]); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " / ")
}

func buildIrisEventTime(data map[string]interface{}) string {
	if ts := firstNonBlank(data, "timestamp"); ts != "" {
		return ts
	}
	for _, key := range []string{"time", "time_pkg", "__insert_time"} {
		if value := int64FromValue(data[key]); value > 0 {
			return time.Unix(value, 0).UTC().Format(time.RFC3339)
		}
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func buildIrisAlertContext(data map[string]interface{}) map[string]interface{} {
	keys := []string{
		"account_id", "agent_id", "alarm_id", "alert_type", "alert_type_us",
		"attack_id", "cloud_provider", "hostname", "nodename", "os_type",
		"rule_name", "rule_name_us", "harm_level", "risk", "pid", "ppid",
		"pgid", "tgid", "sid", "tracing_id", "trace_id", "span_trace_id",
		"exe", "argv", "comm", "exe_hash", "pid_tree", "connect_info",
		"sip", "sport", "dip", "dport", "uid", "username", "docker",
		"pod_name", "group_id", "group_path", "pns", "root_pns",
	}
	context := map[string]interface{}{}
	for _, key := range keys {
		if value, ok := data[key]; ok {
			context[key] = value
		}
	}
	return context
}

func buildIrisAssets(data map[string]interface{}) []irisAsset {
	host := firstNonBlank(data, "hostname", "nodename", "agent_id")
	ip := firstNonBlank(data, "sip", "in_ipv4_list")
	if strings.Contains(ip, ",") {
		ip = strings.TrimSpace(strings.Split(ip, ",")[0])
	}

	if host == "" && ip == "" {
		return nil
	}

	descParts := []string{}
	for _, key := range []string{"account_id", "cloud_provider", "os_type", "agent_id"} {
		if value := stringValue(data[key]); value != "" {
			descParts = append(descParts, key+"="+value)
		}
	}

	return []irisAsset{{
		AssetName:        host,
		AssetIP:          ip,
		AssetDescription: strings.Join(descParts, ", "),
	}}
}

func buildIrisIOCs(data map[string]interface{}) []irisIOC {
	seen := map[string]bool{}
	iocs := []irisIOC{}

	add := func(value string, desc string, typeID int, tags []string) {
		value = strings.TrimSpace(value)
		if value == "" || value == "-" || seen[value] {
			return
		}
		seen[value] = true
		iocs = append(iocs, irisIOC{
			IOCValue:       value,
			IOCDescription: desc,
			IOCTypeID:      typeID,
			IOCTLPID:       2,
			IOCTags:        strings.Join(tags, ","),
		})
	}

	add(firstNonBlank(data, "dip"), "Destination IP", 1, []string{"destination", "ip"})
	add(firstNonBlank(data, "sip"), "Source IP", 1, []string{"source", "ip"})
	add(firstNonBlank(data, "client_public_ip"), "Client public IP", 1, []string{"public", "ip"})

	for _, ip := range splitList(firstNonBlank(data, "ip", "in_ipv4_list", "ex_ipv4_list")) {
		add(ip, "Related IP", 1, []string{"related", "ip"})
	}

	add(firstNonBlank(data, "exe_hash"), "Process hash", 11, []string{"process", "hash"})
	return iocs
}

func buildIrisTags(data map[string]interface{}) string {
	tags := []string{"agentsmith-hub", "cwpp"}
	for _, key := range []string{"harm_level", "alert_type_us", "attack_id", "cloud_provider", "product", "rule_name_us"} {
		if value := normalizeTag(stringValue(data[key])); value != "" {
			tags = append(tags, value)
		}
	}
	for _, value := range splitList(firstNonBlank(data, "tags")) {
		if tag := normalizeTag(value); tag != "" {
			tags = append(tags, tag)
		}
	}

	seen := map[string]bool{}
	out := []string{}
	for _, tag := range tags {
		if tag != "" && !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return strings.Join(out, ",")
}

func irisSeverityID(data map[string]interface{}) int {
	if severityID := intFromField(data, "iris_severity_id"); severityID > 0 {
		return severityID
	}

	level := strings.ToLower(firstNonBlank(data, "harm_level"))
	switch level {
	case "critical", "fatal":
		return 5
	case "high":
		return 4
	case "medium", "middle":
		return 3
	case "low", "basic":
		return 2
	case "info", "informational":
		return 1
	}

	switch intFromField(data, "risk") {
	case 4:
		return 5
	case 3:
		return 4
	case 2:
		return 3
	case 1:
		return 2
	}
	return 3
}

func firstNonBlank(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(data[key]); value != "" {
			return value
		}
		if nested := nestedRuleInfoValue(data, key); nested != "" {
			return nested
		}
	}
	return ""
}

func nestedRuleInfoValue(data map[string]interface{}, key string) string {
	wrapper, ok := data["SMITH_ALERT_DATA"].(map[string]interface{})
	if !ok {
		return ""
	}
	ruleInfo, ok := wrapper["RULE_INFO"].(map[string]interface{})
	if !ok {
		return ""
	}
	return stringValue(ruleInfo[key])
}

func stringValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		s := strings.TrimSpace(v)
		if s == "" || s == "-" || strings.EqualFold(s, "null") {
			return ""
		}
		return s
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
}

func intFromField(data map[string]interface{}, key string) int {
	value := data[key]
	if value == nil {
		if nested := nestedRuleInfoValue(data, key); nested != "" {
			value = nested
		}
	}
	return int(int64FromValue(value))
}

func int64FromValue(value interface{}) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

func splitList(value string) []string {
	if value == "" {
		return nil
	}
	raw := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	})
	out := []string{}
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" && item != "-" {
			out = append(out, item)
		}
	}
	return out
}

func normalizeTag(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "-" {
		return ""
	}
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", "_")
	return value
}
