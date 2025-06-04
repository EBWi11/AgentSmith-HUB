package common

import (
	"archive/zip"
	"errors"
	"fmt"
	"github.com/bytedance/sonic"
	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func GetFileNameWithoutExt(path string) string {
	base := filepath.Base(path)
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
		return false, nil // 不存在
	}
	if err != nil {
		return false, err // 其他错误
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
