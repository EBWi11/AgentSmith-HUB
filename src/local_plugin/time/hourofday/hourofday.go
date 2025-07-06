package hourofday

import (
	"fmt"
	"time"
)

// Eval returns hour of day (0-23).
// Args: optional timestamp(int64 sec).
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
	return t.Hour(), true, nil
}
