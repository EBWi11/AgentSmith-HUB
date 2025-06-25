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
	"reflect"
	"sort"
	"strings"
	"time"
)

type response struct {
	Code int    `json:"errcode"`
	Msg  string `json:"errmsg"`
}

// formatMapToReadableString converts map[string]interface{} to a readable string format
func formatMapToReadableString(data map[string]interface{}) string {
	if data == nil {
		return "(empty data)"
	}

	var result strings.Builder
	result.WriteString("📊 Event Data:\n")
	result.WriteString("════════════════════════════════════════\n")

	// Sort keys for consistent output
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, key := range keys {
		value := data[key]

		// Add some spacing between entries (except for first one)
		if i > 0 {
			result.WriteString("\n")
		}

		// Format key-value pair
		result.WriteString(fmt.Sprintf("🔹 %s: ", key))

		// Format value based on its type
		formattedValue := formatValue(value, "    ")
		result.WriteString(formattedValue)
		result.WriteString("\n")
	}

	result.WriteString("════════════════════════════════════════")
	return result.String()
}

// formatValue formats a single value with proper indentation
func formatValue(value interface{}, indent string) string {
	if value == nil {
		return "(null)"
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String:
		str := value.(string)
		if strings.Contains(str, "\n") {
			// Multi-line string - add indentation to each line
			lines := strings.Split(str, "\n")
			var result strings.Builder
			result.WriteString("\n")
			for i, line := range lines {
				result.WriteString(indent)
				result.WriteString("│ ")
				result.WriteString(line)
				if i < len(lines)-1 {
					result.WriteString("\n")
				}
			}
			return result.String()
		}
		return fmt.Sprintf("\"%s\"", str)

	case reflect.Map:
		if mapValue, ok := value.(map[string]interface{}); ok {
			if len(mapValue) == 0 {
				return "{}"
			}

			var result strings.Builder
			result.WriteString("{\n")

			// Sort nested map keys
			keys := make([]string, 0, len(mapValue))
			for k := range mapValue {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for i, k := range keys {
				result.WriteString(indent)
				result.WriteString("  ")
				result.WriteString(k)
				result.WriteString(": ")
				result.WriteString(formatValue(mapValue[k], indent+"  "))
				if i < len(keys)-1 {
					result.WriteString(",")
				}
				result.WriteString("\n")
			}
			result.WriteString(indent)
			result.WriteString("}")
			return result.String()
		}
		// Fallback for other map types
		jsonBytes, _ := json.MarshalIndent(value, indent, "  ")
		return string(jsonBytes)

	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			return "[]"
		}

		var result strings.Builder
		result.WriteString("[\n")
		for i := 0; i < v.Len(); i++ {
			result.WriteString(indent)
			result.WriteString("  ")
			result.WriteString(formatValue(v.Index(i).Interface(), indent+"  "))
			if i < v.Len()-1 {
				result.WriteString(",")
			}
			result.WriteString("\n")
		}
		result.WriteString(indent)
		result.WriteString("]")
		return result.String()

	case reflect.Bool:
		if value.(bool) {
			return "✅ true"
		}
		return "❌ false"

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", value)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", value)

	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%.6g", value)

	default:
		// Fallback to JSON representation
		jsonBytes, err := json.MarshalIndent(value, indent, "  ")
		if err != nil {
			return fmt.Sprintf("%v", value)
		}
		return string(jsonBytes)
	}
}

// Eval sends formatted message data to DingTalk
func Eval(AccessToken string, Secret string, data map[string]interface{}) (bool, error) {
	// Convert map to readable text format
	text := formatMapToReadableString(data)

	msg := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": text,
		},
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return false, err
	}
	resp, err := http.Post(getURL(AccessToken, Secret), "application/json", bytes.NewBuffer(b))
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var r response
	err = json.Unmarshal(body, &r)
	if err != nil {
		return false, err
	}
	if r.Code != 0 {
		return false, errors.New(fmt.Sprintf("response error: %s", string(body)))
	}
	return true, nil
}

func hmacSha256(stringToSign string, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func getURL(AccessToken string, Secret string) string {
	wh := "https://oapi.dingtalk.com/robot/send?access_token=" + AccessToken
	timestamp := time.Now().UnixNano() / 1e6
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, Secret)
	sign := hmacSha256(stringToSign, Secret)
	url := fmt.Sprintf("%s&timestamp=%d&sign=%s", wh, timestamp, sign)
	return url
}
