package extract

import (
	"fmt"
	"regexp"
)

// Eval extracts text using a regular expression pattern.
// Returns the first match if no capture groups, or an array of capture groups.
// Args: input string, regex pattern.
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 2 {
		return nil, false, fmt.Errorf("regexExtract requires 2 arguments: input, pattern")
	}

	input, ok1 := args[0].(string)
	pattern, ok2 := args[1].(string)

	if !ok1 || !ok2 {
		return nil, false, fmt.Errorf("both arguments must be strings")
	}

	// Compile the regex pattern
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, false, fmt.Errorf("invalid regex pattern: %v", err)
	}

	// Find submatch (includes full match and capture groups)
	matches := regex.FindStringSubmatch(input)
	if matches == nil {
		return nil, false, nil // No match found
	}

	// If there are capture groups, return them (excluding the full match at index 0)
	if len(matches) > 1 {
		captureGroups := make([]string, len(matches)-1)
		copy(captureGroups, matches[1:])
		return captureGroups, true, nil
	}

	// If no capture groups, return the full match
	return matches[0], true, nil
}
