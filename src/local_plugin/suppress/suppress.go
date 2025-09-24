package suppress

import (
	"AgentSmith-HUB/common"
	"fmt"
	"strconv"
)

// Eval implements a suppression plugin: for the same key, return true only once
// within the provided time window (seconds). Args:
//
//	0: window — suppression window (int seconds or duration string like "30s", "2m")
//	1..n: key parts — remaining args are concatenated (stringified) to form the key
//
// Behavior: returns true at most once per key within the window; subsequent calls
// until expiry return false. Implemented via Redis SETNX with TTL.
func Eval(args ...interface{}) (bool, error) {
	if len(args) < 2 {
		return false, fmt.Errorf("suppress requires at least 2 arguments: windows and the suppress key")
	}

	// parse window seconds
	var winSec int
	switch v := args[0].(type) {
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

	redisKey := "suppress"
	for _, arg := range args[1:] {
		redisKey += fmt.Sprintf(":%v", arg)
	}

	ok, err := common.RedisSetNX(redisKey, 1, winSec)
	if err != nil {
		return false, err
	}
	// ok==true means first time within window → return true; else false
	return ok, nil
}
