package replace

import (
	"fmt"
	"strings"
)

// Eval replaces all occurrences of old substring with new substring.
// Args: input string, old substring, new substring.
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 3 {
		return nil, false, fmt.Errorf("replace requires 3 arguments: input, old, new")
	}

	input, ok1 := args[0].(string)
	old, ok2 := args[1].(string)
	new, ok3 := args[2].(string)

	if !ok1 || !ok2 || !ok3 {
		return nil, false, fmt.Errorf("all arguments must be strings")
	}

	result := strings.ReplaceAll(input, old, new)
	return result, true, nil
}
