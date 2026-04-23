package suppress_once

import (
	"AgentSmith-HUB/common"
	"encoding/json"
	"fmt"
	"strconv"
)

// Eval implements a suppression plugin: for the same key, return true only once
// within the provided time window (seconds). Args:
//
//	0: key string / any comparable value converted to string
//	1: window int (seconds) – suppression period
//	2: ruleid string (optional) – rule identifier to isolate different rules
//
// It uses Redis SETNX with TTL to track fired keys.
func Eval(args ...interface{}) (bool, error) {
	if len(args) < 2 {
		return false, fmt.Errorf("suppressOnce requires at least 2 arguments: key and window(sec), optionally ruleid")
	}
	keyStr := stableKeyPart(args[0])

	// parse window seconds
	var winSec int
	switch v := args[1].(type) {
	case int:
		winSec = v
	case int64:
		winSec = int(v)
	case float64:
		winSec = int(v)
	case string:
		i, err := strconv.Atoi(v)
		if err != nil {
			i, err = common.ParseDurationToSecondsInt(v)
			if err != nil {
				return false, fmt.Errorf("invalid window seconds: %v", v)
			}
		}
		winSec = i
	default:
		return false, fmt.Errorf("unsupported window type %T", v)
	}
	if winSec <= 0 {
		return false, fmt.Errorf("window must be positive seconds")
	}

	// Optional ruleid parameter for rule isolation
	var redisKey string
	if len(args) >= 3 {
		ruleid := stableKeyPart(args[2])
		redisKey = "suppress_once:" + ruleid + ":" + keyStr
	} else {
		// Backward compatibility: no ruleid specified
		redisKey = "suppress_once:" + keyStr
	}

	ok, err := common.RedisSetNX(redisKey, 1, winSec)
	if err != nil {
		return false, err
	}
	// ok==true means first time within window → return true; else false
	return ok, nil
}

func stableKeyPart(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprintf("%v", val)
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", v)
	}
}
