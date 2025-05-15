package rules_engine

import (
	"AgentSmith-HUB/common"
	regexp "github.com/BurntSushi/rure-go"
	"strconv"
	"strings"
)

func GetRuleValueFromRawFromCache(cache map[string]common.CheckCoreCache, checkKey string, data map[string]interface{}) string {
	tmpRes, ok := cache[checkKey]
	if ok {
		return tmpRes.Data
	} else {
		checkKeyList := common.StringToList(checkKey[FROM_RAW_SYMBOL_LEN:])
		res, exist := common.GetCheckData(data, checkKeyList)
		cache[checkKey] = common.CheckCoreCache{
			Exist: exist,
			Data:  res,
		}
		return res
	}
}

func GetCheckDataFromCache(cache map[string]common.CheckCoreCache, checkKey string, data map[string]interface{}, checkKeyList []string) (res string, exist bool) {
	tmpRes, ok := cache[checkKey]
	if ok {
		return tmpRes.Data, tmpRes.Exist
	} else {
		res, exist := common.GetCheckData(data, checkKeyList)
		cache[checkKey] = common.CheckCoreCache{
			Exist: exist,
			Data:  res,
		}
		return res, exist
	}
}

func END(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}

	if data == "" {
		return false, ""
	}

	return strings.HasSuffix(data, ruleData), ruleData
}

func START(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}

	if data == "" {
		return false, ""
	}

	return strings.HasPrefix(data, ruleData), ruleData
}

func NEND(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}

	if data == "" {
		return false, ""
	}

	return !strings.HasSuffix(data, ruleData), ruleData
}

func NSTART(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}

	if data == "" {
		return false, ""
	}

	return !strings.HasPrefix(data, ruleData), ruleData
}

func INCL(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}

	if data == "" {
		return false, ""
	}
	return strings.Contains(data, ruleData), ruleData
}

func NI(data string, ruleData string) (res bool, hitData string) {
	if data == "" {
		return true, ""
	}
	return !strings.Contains(data, ruleData), ruleData
}

func NCS_END(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}

	if data == "" {
		return false, ""
	}

	return strings.HasSuffix(strings.ToLower(data), strings.ToLower(ruleData)), ruleData
}

func NCS_START(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}

	if data == "" {
		return false, ""
	}

	return strings.HasPrefix(strings.ToLower(data), strings.ToLower(ruleData)), ruleData
}

func NCS_NEND(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}

	if data == "" {
		return false, ""
	}

	return !strings.HasSuffix(strings.ToLower(data), strings.ToLower(ruleData)), ruleData
}

func NCS_NSTART(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}

	if data == "" {
		return false, ""
	}

	return !strings.HasPrefix(strings.ToLower(data), strings.ToLower(ruleData)), ruleData
}

func NCS_INCL(data string, ruleData string) (res bool, hitData string) {
	if ruleData == "" {
		return true, ruleData
	}

	if data == "" {
		return false, ""
	}
	return strings.Contains(strings.ToLower(data), strings.ToLower(ruleData)), ruleData
}

func NCS_NI(data string, ruleData string) (res bool, hitData string) {
	if data == "" {
		return true, ""
	}
	return !strings.Contains(strings.ToLower(data), strings.ToLower(ruleData)), ruleData
}

func MT(data string, ruleData string) (res bool, hitData string) {
	ori_int, err := strconv.ParseFloat(data, 64)
	if err != nil {
		return false, ""
	}
	check_int, err := strconv.ParseFloat(ruleData, 64)
	if err != nil {
		return false, ""
	}

	if ori_int > check_int {
		return true, ruleData
	} else {
		return false, ""
	}
}

func LT(data string, ruleData string) (res bool, hitData string) {
	ori_int, err := strconv.ParseFloat(data, 64)
	if err != nil {
		return false, ""
	}
	check_int, err := strconv.ParseFloat(ruleData, 64)
	if err != nil {
		return false, ""
	}

	if ori_int < check_int {
		return true, ruleData
	} else {
		return false, ""
	}
}

func REGEX(data string, regexCompile *regexp.Regex) (res bool, hitData string) {
	start, end, tmp_res := regexCompile.Find(data)
	if tmp_res {
		return true, data[start:end]
	} else {
		return false, ""
	}
}

func ISNULL(data string, ruleData string) (res bool, hitData string) {
	if data == "" {
		return true, data
	} else {
		return false, ""
	}
}

func NOTNULL(data string, ruleData string) (res bool, hitData string) {
	if strings.TrimSpace(data) == "" {
		return false, ""
	} else {
		return true, data
	}
}

func EQU(data string, ruleData string) (res bool, hitData string) {
	return strings.EqualFold(data, ruleData), data
}

func NEQ(data string, ruleData string) (res bool, hitData string) {
	return !strings.EqualFold(data, ruleData), ruleData
}

func NCS_EQU(data string, ruleData string) (res bool, hitData string) {
	return strings.EqualFold(strings.ToLower(data), strings.ToLower(ruleData)), data
}

func NCS_NEQ(data string, ruleData string) (res bool, hitData string) {
	return !strings.EqualFold(strings.ToLower(data), strings.ToLower(ruleData)), ruleData
}
