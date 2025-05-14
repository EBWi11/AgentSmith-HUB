package common

import (
	"bytes"
	"github.com/bytedance/sonic"
	"net/url"
	"strconv"
	"strings"
)

func StringsJoin(strList []string) string {
	var buffer bytes.Buffer

	strListLen := len(strList)

	for i := 0; i < strListLen; i++ {
		buffer.WriteString(strList[i])
	}

	return buffer.String()
}

func UrlValueToMap(data map[string][]string) map[string]interface{} {
	res := make(map[string]interface{})
	for k, v := range data {
		tmpV := StringsJoin(v)
		res[k] = tmpV
	}
	return res
}

func AnyToString(tmp interface{}) string {
	res := ""
	if value, ok := tmp.(string); ok {
		res = value
	} else if value, ok := tmp.(int); ok {
		res = strconv.Itoa(value)
	} else if value, ok := tmp.(bool); ok {
		res = strconv.FormatBool(value)
	} else if value, ok := tmp.(float64); ok {
		res = strconv.FormatFloat(value, 'f', -1, 64)
	} else if value, ok := tmp.(int64); ok {
		res = strconv.FormatInt(value, 10)
	} else {
		resBytes, _ := sonic.Marshal(tmp)
		res = string(resBytes)
	}

	return res
}

func GetCheckData(data map[string]interface{}, checkKeyList []string) (res string, exist bool) {
	tmp := data
	res = ""
	keyListLen := len(checkKeyList) - 1
	for i, k := range checkKeyList {
		tmpRes, ok := tmp[k]
		if tmpRes != nil {
			if keyListLen != i {
				if value, ok := tmpRes.(map[string]interface{}); ok {
					tmp = value
				} else if value, ok := tmpRes.([]interface{}); ok {
					tmp_map_for_list := make(map[string]interface{})
					for i, v := range value {
						tmp_key := "#_" + strconv.Itoa(i)
						tmp_map_for_list[tmp_key] = v
					}
					tmp = tmp_map_for_list
				} else if value, ok := tmpRes.(string); ok {
					jsonFlage := false
					if strings.Contains(value, ":") || strings.Contains(value, "{") || strings.Contains(value, "[") {
						tmpValue := make(map[string]interface{})
						err := sonic.Unmarshal([]byte(value), &tmpValue)
						if err == nil {
							tmp = tmpValue
							jsonFlage = true
						}
					}
					if !jsonFlage {
						tmpValue, err := url.ParseQuery(value)
						if err == nil {
							tmp = UrlValueToMap(tmpValue)
						}
					}
				}
			} else if keyListLen == i {
				res = AnyToString(tmpRes)
				exist = true
			}
		} else if ok {
			return "", true
		} else {
			return "", false
		}
	}
	if res == "" {
		return "", exist
	} else {
		return res, exist
	}
}
