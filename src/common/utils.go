package common

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
)

func GetFileNameWithoutExt(path string) string {
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".new") {
		base = base[0 : len(base)-len(".new")]
	}
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func Unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			_ = outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		_ = outFile.Close()
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func CopyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		dstFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()
		_, err = io.Copy(dstFile, srcFile)
		return err
	})
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

func DirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil // does not exist
	}
	if err != nil {
		return false, err // other error
	}
	return info.IsDir(), nil
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
			case []interface{}:
				// Convert slice to map with index keys
				tmpMapForList := make(map[string]interface{}, len(value))
				for idx, v := range value {
					tmpKey := "#_" + strconv.Itoa(idx)
					tmpMapForList[tmpKey] = v
				}
				tmp = tmpMapForList
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
				// Unsupported type for traversal
				return "", false
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

// WriteConfigFile writes a configuration file to the config root directory
func WriteConfigFile(componentType string, id string, content string) error {
	if Config.ConfigRoot == "" {
		return fmt.Errorf("config root is not set")
	}

	var fileExt string
	switch componentType {
	case "ruleset":
		fileExt = ".xml"
	case "project", "input", "output":
		fileExt = ".yaml"
	default:
		return fmt.Errorf("unsupported component type: %s", componentType)
	}

	dirPath := path.Join(Config.ConfigRoot, componentType)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dirPath, err)
	}

	filePath := path.Join(dirPath, id+fileExt)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return nil
}

// DeleteConfigFile deletes a configuration file from the config root directory
func DeleteConfigFile(componentType string, id string) error {
	if Config.ConfigRoot == "" {
		return fmt.Errorf("config root is not set")
	}

	var fileExt string
	switch componentType {
	case "ruleset":
		fileExt = ".xml"
	case "project", "input", "output":
		fileExt = ".yaml"
	default:
		return fmt.Errorf("unsupported component type: %s", componentType)
	}

	filePath := path.Join(Config.ConfigRoot, componentType, id+fileExt)
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, which is fine for deletion
		}
		return fmt.Errorf("failed to delete file %s: %w", filePath, err)
	}

	return nil
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

// ensureConfigDir creates the config directory if it doesn't exist
func EnsureConfigDir() error {
	configDir := GetConfigDir()
	if configDir == "." {
		// For macOS (current directory), no need to create
		return nil
	}

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return err
		}
	}
	return nil
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

// EnsureConfigDirExists explicitly ensures the config directory exists
// This can be called during initialization to set up directories proactively
func EnsureConfigDirExists() error {
	configDir := GetConfigDir()

	// For macOS (current directory), no need to create
	if configDir == "." {
		return nil
	}

	// Check if directory exists
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		// Create directory with proper permissions
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory %s: %w", configDir, err)
		}
		fmt.Printf("Successfully created config directory: %s\n", configDir)
	} else if err != nil {
		return fmt.Errorf("error accessing config directory %s: %w", configDir, err)
	}

	return nil
}

// MapShallowCopy returns a shallow copy of a map[string]interface{}. Only top-level keys are copied.
func MapShallowCopy(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
