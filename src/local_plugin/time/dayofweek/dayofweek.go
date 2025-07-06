package dayofweek

import (
	"fmt"
	"time"
)

// Eval returns day of week (0=Sunday) of provided timestamp or current time.
// Args: optional timestamp(int64 seconds). If omitted uses now.
func Eval(args ...interface{}) (interface{}, bool, error) {
	var t time.Time
	if len(args) == 0 {
		t = time.Now()
	} else {
		ts, ok := args[0].(int64)
		if !ok {
			return nil, false, fmt.Errorf("argument must be int64 timestamp")
		}
		t = time.Unix(ts, 0)
	}
	return int(t.Weekday()), true, nil
}
