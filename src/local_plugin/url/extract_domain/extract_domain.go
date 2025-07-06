package extract_domain

import (
	"fmt"
	"net/url"
	"strings"
)

// Eval returns registered domain (domain.tld) from URL or hostname.
// Args: urlOrHost string.
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, false, fmt.Errorf("extract_domain requires 1 string arg")
	}
	raw, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("arg must be string")
	}
	host := raw
	if strings.Contains(raw, "/") {
		if u, err := url.Parse(raw); err == nil {
			host = u.Hostname()
		}
	}
	// remove port
	host = strings.Split(host, ":")[0]
	return host, true, nil
}
