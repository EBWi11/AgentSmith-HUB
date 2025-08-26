package common

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
)

func GetComponentFromSequenceID(sequence string) (string, string) {
	parts := strings.Split(sequence, ".")
	if len(parts) < 2 {
		return "", ""
	}

	// Special handling for PLUGIN sequences
	// Format: "PLUGIN.pluginName.success" or "PLUGIN.pluginName.failure"
	if len(parts) == 3 && strings.ToUpper(parts[0]) == "PLUGIN" {
		pluginName := parts[1]
		status := strings.ToLower(parts[2])

		if status == "success" {
			return "plugin_success", pluginName
		} else if status == "failure" {
			return "plugin_failure", pluginName
		}
	}

	// For other components, use the last two parts
	return parts[len(parts)-2], parts[len(parts)-1]
}

func GetFileNameWithoutExt(path string) string {
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".new") {
		base = base[0 : len(base)-len(".new")]
	}
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func NewUUID() string {
	id := uuid.New()
	return id.String()
}

func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ip4 := ipNet.IP.To4(); ip4 != nil {
				return ip4.String(), nil
			}
		}
	}
	return "127.0.0.1", errors.New("not found local ip")
}

func ParseDurationToSecondsInt(input string) (int, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	re := regexp.MustCompile(`^([\d.]+)\s*([smhd])$`)
	matches := re.FindStringSubmatch(input)
	if len(matches) != 3 {
		return 0, errors.New("invalid format: expected number + unit (s, m, h, d)")
	}

	numStr, unit := matches[1], matches[2]

	if unit == "s" && strings.Contains(numStr, ".") {
		return 0, errors.New("seconds unit 's' must be an integer")
	}

	value, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %w", err)
	}

	var seconds float64
	switch unit {
	case "s":
		seconds = value
	case "m":
		seconds = value * 60
	case "h":
		seconds = value * 3600
	case "d":
		seconds = value * 86400
	default:
		return 0, errors.New("unsupported unit")
	}

	if seconds <= 5 {
		return 0, errors.New("duration must be greater than 5 seconds")
	}

	return int(seconds), nil
}

func MapDeepCopy(m map[string]interface{}) map[string]interface{} {
	return MapDeepCopyAction(m).(map[string]interface{})
}

func MapDeepCopyAction(m interface{}) interface{} {
	vm, ok := m.(map[string]interface{})
	if ok {
		cp := map[string]interface{}{}
		for k, v := range vm {
			vm, ok := v.(map[string]interface{})
			if ok {
				cp[k] = MapDeepCopyAction(vm)
			} else {
				vm, ok := v.([]interface{})
				if ok {
					cp[k] = MapDeepCopyAction(vm)
				} else {
					cp[k] = v
				}
			}
		}
		return cp
	} else {
		vm, ok := m.([]interface{})
		if ok {
			cp := []interface{}{}
			for _, v := range vm {
				cp = append(cp, MapDeepCopyAction(v))
			}
			return cp
		} else {
			return m
		}
	}
}

func XXHash64(s string) string {
	hash := xxhash.Sum64([]byte(s))
	return strconv.FormatUint(hash, 10)
}

func MapDel(data map[string]interface{}, key []string) {
	tmpKey := []string{}
	l := len(key) - 1
	for i := range key {
		if l != i {
			if value, ok := data[key[i]].(map[string]interface{}); ok {
				tmpKey = append(tmpKey, key[i])
				data = value
			} else {
				delete(data, key[i])
				break
			}
		} else {
			delete(data, key[i])
			break
		}
	}
}

func StringToList(checkKey string) []string {
	if len(checkKey) == 0 {
		return nil
	}
	var res []string
	var sb strings.Builder
	for i := 0; i < len(checkKey); i++ {
		if checkKey[i] == '\\' && i+1 < len(checkKey) && checkKey[i+1] == '.' {
			sb.WriteByte('.')
			i++
		} else if checkKey[i] == '.' {
			res = append(res, sb.String())
			sb.Reset()
		} else {
			sb.WriteByte(checkKey[i])
		}
	}
	if sb.Len() > 0 {
		res = append(res, sb.String())
	}
	return res
}

// UrlValueToMap converts url.Values (map[string][]string) to map[string]interface{}.
// Joins multiple values into a single string.
func UrlValueToMap(data map[string][]string) map[string]interface{} {
	res := make(map[string]interface{}, len(data))
	for k, v := range data {
		res[k] = strings.Join(v, "")
	}
	return res
}

// AnyToString converts various types to their string representation.
// Supports string, int, bool, float64, int64, and falls back to JSON for others.
func AnyToString(tmp interface{}) string {
	switch value := tmp.(type) {
	case string:
		return value
	case int:
		return strconv.Itoa(value)
	case bool:
		return strconv.FormatBool(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(value, 10)
	default:
		// Marshal to JSON string for unsupported types
		resBytes, _ := sonic.Marshal(tmp)
		return string(resBytes)
	}
}

// GetCheckData traverses a nested map[string]interface{} using a key path (checkKeyList).
// Returns the string value and whether it exists.
// Handles map, slice, JSON string, and URL query string as intermediate nodes.
func GetCheckData(data map[string]interface{}, checkKeyList []string) (res string, exist bool) {
	tmp := data
	res = ""
	keyListLen := len(checkKeyList) - 1
	for i, k := range checkKeyList {
		tmpRes, ok := tmp[k]
		if !ok || tmpRes == nil {
			// Key not found or value is nil
			return "", false
		}
		if i != keyListLen {
			switch value := tmpRes.(type) {
			case map[string]interface{}:
				// Continue traversing nested map
				tmp = value
			case string:
				// Try to parse as JSON if it looks like JSON
				if (strings.Contains(value, ":") || strings.Contains(value, "{") || strings.Contains(value, "[")) && len(value) > 2 {
					tmpValue := make(map[string]interface{})
					if err := sonic.Unmarshal([]byte(value), &tmpValue); err == nil {
						tmp = tmpValue
						continue
					}
				}
				// Try to parse as URL query string
				if tmpValue, err := url.ParseQuery(value); err == nil {
					tmp = UrlValueToMap(tmpValue)
					continue
				}
				// Not a traversable structure
				return "", false
			default:
				rv := reflect.ValueOf(value)
				switch rv.Kind() {
				case reflect.Slice, reflect.Array:
					// for array and slice
					tmpMapForList := make(map[string]interface{}, rv.Len())
					for i := 0; i < rv.Len(); i++ {
						tmpMapForList["#"+strconv.Itoa(i)] = rv.Index(i).Interface()
					}
					tmp = tmpMapForList
				default:
					return "", false
				}
			}
		} else {
			// Last key, convert value to string
			res = AnyToString(tmpRes)
			exist = true
		}
	}
	if res == "" {
		return "", exist
	}
	return res, exist
}

// GetCheckDataWithType traverses a nested map[string]interface{} using a key path (checkKeyList).
// Returns the original typed value and whether it exists.
// Handles map, slice, JSON string, and URL query string as intermediate nodes.
// Unlike GetCheckData, this function preserves the original data type.
func GetCheckDataWithType(data map[string]interface{}, checkKeyList []string) (res interface{}, exist bool) {
	tmp := data
	res = nil
	keyListLen := len(checkKeyList) - 1
	for i, k := range checkKeyList {
		tmpRes, ok := tmp[k]
		if !ok || tmpRes == nil {
			// Key not found or value is nil
			return nil, false
		}
		if i != keyListLen {
			switch value := tmpRes.(type) {
			case map[string]interface{}:
				// Continue traversing nested map
				tmp = value
			case string:
				// Try to parse as JSON if it looks like JSON
				if (strings.Contains(value, ":") || strings.Contains(value, "{") || strings.Contains(value, "[")) && len(value) > 2 {
					tmpValue := make(map[string]interface{})
					if err := sonic.Unmarshal([]byte(value), &tmpValue); err == nil {
						tmp = tmpValue
						continue
					}
				}
				// Try to parse as URL query string
				if tmpValue, err := url.ParseQuery(value); err == nil {
					tmp = UrlValueToMap(tmpValue)
					continue
				}
				// Not a traversable structure
				return nil, false
			default:
				rv := reflect.ValueOf(value)
				switch rv.Kind() {
				case reflect.Slice, reflect.Array:
					// 所有数组/切片都会走这里
					tmpMapForList := make(map[string]interface{}, rv.Len())
					for i := 0; i < rv.Len(); i++ {
						tmpMapForList["#"+strconv.Itoa(i)] = rv.Index(i).Interface()
					}
					tmp = tmpMapForList
				default:
					return "", false
				}
			}
		} else {
			// Last key, return original typed value
			res = tmpRes
			exist = true
		}
	}
	return res, exist
}

// ReadContentFromPathOrRaw reads content from file path or returns raw content
// This is a common utility function used by all component verification functions
func ReadContentFromPathOrRaw(path string, raw string) ([]byte, error) {
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file at %s: %w", path, err)
		}
		return data, nil
	} else {
		return []byte(raw), nil
	}
}

// getConfigDir returns the appropriate config directory based on the operating system
func GetConfigDir() string {
	if runtime.GOOS == "darwin" {
		return "." // Current directory for macOS
	}
	return "/etc/hub" // /etc/hub for Linux
}

// GetConfigPath returns the full path for a specific config file
// It ensures the directory exists and creates it if necessary
func GetConfigPath(filename string) string {
	configDir := GetConfigDir()

	// For macOS (current directory), just return the path
	if configDir == "." {
		return filepath.Join(configDir, filename)
	}

	// For Linux, ensure the directory exists
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		// Try to create the directory
		if err := os.MkdirAll(configDir, 0755); err != nil {
			// Log the failure and fallback to current directory
			if Config != nil && Config.LocalIP != "" {
				// If logging is available, log the error
				fmt.Printf("Failed to create config directory %s: %v, falling back to current directory\n", configDir, err)
			}
			configDir = "."
		} else {
			// Successfully created directory
			if Config != nil && Config.LocalIP != "" {
				fmt.Printf("Created config directory: %s\n", configDir)
			}
		}
	} else if err != nil {
		// Other error accessing directory (permission issues, etc.)
		if Config != nil && Config.LocalIP != "" {
			fmt.Printf("Error accessing config directory %s: %v, falling back to current directory\n", configDir, err)
		}
		configDir = "."
	}

	return filepath.Join(configDir, filename)
}
