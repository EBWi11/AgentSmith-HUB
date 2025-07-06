package extract_tld

import (
	"fmt"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Eval returns TLD for a domain.
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, false, fmt.Errorf("extract_tld requires 1 arg")
	}
	d, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("arg must be string")
	}
	_, icann := publicsuffix.PublicSuffix(d)
	if icann {
		parts := strings.Split(d, ".")
		if len(parts) > 0 {
			return parts[len(parts)-1], true, nil
		}
	}
	return "", true, nil
}
