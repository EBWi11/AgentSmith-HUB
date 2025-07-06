package now

import "time"

// Eval returns current Unix timestamp in seconds.
// Args: none.
func Eval(args ...interface{}) (interface{}, bool, error) {
	return time.Now().Unix(), true, nil
}
