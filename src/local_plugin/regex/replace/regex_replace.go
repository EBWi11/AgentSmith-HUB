package replace

import (
	"fmt"
	"regexp"
)

// Eval replaces text matching a regular expression pattern with a replacement string.
// Supports capture group references in replacement (e.g., $1, $2).
// Args: input string, regex pattern, replacement string.
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 3 {
		return nil, false, fmt.Errorf("regexReplace requires 3 arguments: input, pattern, replacement")
	}

	input, ok1 := args[0].(string)
	pattern, ok2 := args[1].(string)
	replacement, ok3 := args[2].(string)

	if !ok1 || !ok2 || !ok3 {
		return nil, false, fmt.Errorf("all arguments must be strings")
	}

	// Compile the regex pattern
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, false, fmt.Errorf("invalid regex pattern: %v", err)
	}

	// Replace all matches
	result := regex.ReplaceAllString(input, replacement)
	return result, true, nil
}
