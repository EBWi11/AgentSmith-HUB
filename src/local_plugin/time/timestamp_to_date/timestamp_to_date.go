package timestamp_to_date

import (
	"fmt"
	"time"
)

// Eval converts unix timestamp to RFC3339 date string.
// Args: timestamp(int64)
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, false, fmt.Errorf("timestamp_to_date requires 1 int64 arg")
	}
	ts, ok := args[0].(int64)
	if !ok {
		return nil, false, fmt.Errorf("argument must be int64")
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339), true, nil
}
