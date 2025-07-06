package ago

import (
	"errors"
	"strconv"
	"time"
)

// Eval returns a Unix timestamp that is N seconds ago.
// Args: seconds(int|float64|string).
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, false, errors.New("ago requires 1 argument: seconds")
	}
	var sec float64
	switch v := args[0].(type) {
	case int:
		sec = float64(v)
	case int64:
		sec = float64(v)
	case float64:
		sec = v
	case string:
		// try parse
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			sec = parsed
		} else {
			return nil, false, err
		}
	default:
		return nil, false, errors.New("unsupported seconds type")
	}
	ts := time.Now().Add(-time.Duration(sec) * time.Second).Unix()
	return ts, true, nil
}
