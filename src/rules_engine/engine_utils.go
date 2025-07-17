package rules_engine

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"fmt"
	"strconv"
	"strings"
	"time"

	regexp "github.com/BurntSushi/rure-go"
)

// RedisFRQSum performs frequency sum aggregation using Redis
// groupByKey: Redis key for grouping
// sumData: Value to add to the sum
// rangeInt: Time range in seconds
// threshold: Threshold value to trigger
// Returns: true if threshold is exceeded, false otherwise
func RedisFRQSum(groupByKey string, sumData int, rangeInt int, threshold int) (bool, error) {
	var res = false
	redisSetNXRes, err := common.RedisSetNX(groupByKey, sumData, rangeInt)
	if err != nil {
		return false, fmt.Errorf("failed to set Redis key %s: %w", groupByKey, err)
	}

	if !redisSetNXRes {
		groupByValue, err := common.RedisIncrby(groupByKey, int64(sumData))
		if err != nil {
			return false, fmt.Errorf("failed to increment Redis key %s: %w", groupByKey, err)
		} else {
			if groupByValue > int64(threshold) {
				res = true
				if err := common.RedisDel(groupByKey); err != nil {
					logger.Error("failed to delete Redis key %s: %v", groupByKey, err)
				}
			}
		}
	}
	return res, nil
}

// LocalCacheFRQSum performs frequency sum aggregation using local cache
// Note: In concurrent situations, there may be precision issues
// groupByKey: Cache key for grouping
// sumData: Value to add to the sum
// rangeInt: Time range in seconds
// threshold: Threshold value to trigger
// Returns: true if threshold is exceeded, false otherwise
func (r *Ruleset) LocalCacheFRQSum(groupByKey string, sumData int, rangeInt int, threshold int) (bool, error) {
	if v, ok := r.Cache.Get(groupByKey); ok {
		if v+sumData > threshold {
			r.Cache.Del(groupByKey)
			return true, nil
		} else {
			if tmpTtl, exist := r.Cache.GetTTL(groupByKey); exist {
				r.Cache.SetWithTTL(groupByKey, v+sumData, 0, tmpTtl)
			}
			return false, nil
		}
	} else {
		r.Cache.SetWithTTL(groupByKey, sumData, 0, time.Duration(rangeInt)*time.Second)
		return false, nil
	}
}

// RedisFRQClassify performs frequency classification using Redis
// tmpKey: Temporary key for tracking
// groupByKey: Base key for grouping
// rangeInt: Time range in seconds
// threshold: Threshold value to trigger
// Returns: true if threshold is exceeded, false otherwise
func RedisFRQClassify(tmpKey string, groupByKey string, rangeInt int, threshold int) (bool, error) {
	var res = false
	_, err := common.RedisSet(tmpKey, 1, rangeInt)
	if err != nil {
		return false, fmt.Errorf("failed to set Redis key %s: %w", tmpKey, err)
	}

	tmpRes, err := common.RedisKeys(groupByKey + "*")
	if err != nil {
		return false, fmt.Errorf("failed to get Redis keys matching %s*: %w", groupByKey, err)
	}

	if len(tmpRes) > threshold {
		res = true
		for i := range tmpRes {
			if err := common.RedisDel(tmpRes[i]); err != nil {
				logger.Error("failed to delete Redis key %s: %v", tmpRes[i], err)
			}
		}
	}
	return res, nil
}

func (r *Ruleset) LocalCacheFRQClassify(tmpKey string, groupByKey string, rangeInt int, threshold int) (bool, error) {
	r.CacheMu.Lock()
	defer r.CacheMu.Unlock()

	if keys, ok := r.CacheForClassify.Get(groupByKey); ok {
		count := len(keys) + 1
		for key := range keys {
			if _, okk := r.Cache.Get(key); !okk {
				count = count - 1
				delete(keys, key)
			}
		}

		if count > threshold {
			for key := range keys {
				r.Cache.Del(key)
			}
			r.CacheForClassify.Del(groupByKey)
			return true, nil
		} else {
			keys[tmpKey] = true
			r.CacheForClassify.SetWithTTL(groupByKey, keys, 0, time.Duration(rangeInt*2)*time.Second)
			r.Cache.SetWithTTL(tmpKey, 1, 0, time.Duration(rangeInt)*time.Second)
			return false, nil
		}
	} else {
		keys := map[string]bool{
			tmpKey: true,
		}
		r.Cache.SetWithTTL(tmpKey, 1, 0, time.Duration(rangeInt)*time.Second)
		r.CacheForClassify.SetWithTTL(groupByKey, keys, 0, time.Duration(rangeInt*2)*time.Second)
		return false, nil
	}
}

// convertPluginArgument converts complex objects to strings for plugin consumption
// while preserving simple types
func convertPluginArgument(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string, int, int64, float64, bool:
		// Keep simple types as is
		return v
	case map[string]interface{}, []interface{}, []map[string]interface{}:
		// Convert complex objects to JSON string
		return common.AnyToString(v)
	default:
		// For any other types, convert to string
		return common.AnyToString(v)
	}
}

func GetPluginRealArgs(args []*PluginArg, data map[string]interface{}, cache map[string]common.CheckCoreCache) []interface{} {
	var ok bool
	res := make([]interface{}, len(args))
	for i, v := range args {
		switch v.Type {
		case 0:
			res[i] = v.Value
		case 1:
			key := v.Value.(string)
			keyList := common.StringToList(strings.TrimSpace(key))
			// Get typed data for field reference
			if v.RealValue, ok = GetCheckDataWithTypeFromCache(cache, key, data, keyList); !ok {
				// If field not found, return empty string
				res[i] = ""
			} else {
				// Convert complex objects to string for plugin consumption
				res[i] = convertPluginArgument(v.RealValue)
			}
		case 2:
			res[i] = common.MapDeepCopy(data)
		}
	}
	return res
}

func GetRuleValueFromRawFromCache(cache map[string]common.CheckCoreCache, checkKey string, data map[string]interface{}) string {
	tmpRes, ok := cache[checkKey]
	if ok {
		return tmpRes.Data
	} else {
		checkKeyList := common.StringToList(checkKey[FromRawSymbolLen:])
		res, exist := common.GetCheckData(data, checkKeyList)
		typedRes, _ := common.GetCheckDataWithType(data, checkKeyList)
		cache[checkKey] = common.CheckCoreCache{
			Exist:     exist,
			Data:      res,
			TypedData: typedRes,
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
		typedRes, _ := common.GetCheckDataWithType(data, checkKeyList)
		cache[checkKey] = common.CheckCoreCache{
			Exist:     exist,
			Data:      res,
			TypedData: typedRes,
		}
		return res, exist
	}
}

// GetCheckDataWithTypeFromCache retrieves typed data from cache or fetches and caches it
func GetCheckDataWithTypeFromCache(cache map[string]common.CheckCoreCache, checkKey string, data map[string]interface{}, checkKeyList []string) (res interface{}, exist bool) {
	tmpRes, ok := cache[checkKey]
	if ok {
		return tmpRes.TypedData, tmpRes.Exist
	} else {
		res, exist := common.GetCheckDataWithType(data, checkKeyList)
		strRes := ""
		if exist && res != nil {
			strRes = common.AnyToString(res)
		}
		cache[checkKey] = common.CheckCoreCache{
			Exist:     exist,
			Data:      strRes, // For backward compatibility
			TypedData: res,    // Original typed data
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

func ISNULL(data string, _ string) (res bool, hitData string) {
	if data == "" {
		return true, data
	} else {
		return false, ""
	}
}

func NOTNULL(data string, _ string) (res bool, hitData string) {
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
