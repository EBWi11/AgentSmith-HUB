package extract_subdomain

import (
	"fmt"
	"strings"
)

// Eval returns subdomain part (everything before first dot).
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, false, fmt.Errorf("extract_subdomain requires 1 string arg")
	}
	host, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("arg must be string")
	}
	parts := strings.Split(host, ".")
	if len(parts) > 2 {
		return strings.Join(parts[:len(parts)-2], "."), true, nil
	}
	return "", true, nil
}
