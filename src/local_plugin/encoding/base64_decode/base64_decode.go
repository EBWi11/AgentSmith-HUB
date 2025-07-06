package base64_decode

import (
	"encoding/base64"
	"fmt"
)

// Eval decodes base64 string. Args: encoded string.
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, false, fmt.Errorf("base64_decode requires 1 arg")
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("argument must be string")
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, false, err
	}
	return string(b), true, nil
}
