package base64_encode

import (
	"encoding/base64"
	"fmt"
)

// Eval encodes input string to base64. Args: plain string.
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, false, fmt.Errorf("base64_encode requires 1 string arg")
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("argument must be string")
	}
	return base64.StdEncoding.EncodeToString([]byte(s)), true, nil
}
