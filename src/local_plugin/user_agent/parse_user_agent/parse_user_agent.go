package parse_user_agent

import (
	"fmt"

	"github.com/mssola/user_agent"
)

// Eval parses user agent string and returns map with browser, version, os, platform.
func Eval(args ...interface{}) (interface{}, bool, error) {
	if len(args) != 1 {
		return nil, false, fmt.Errorf("parse_user_agent requires 1 string arg")
	}
	uaStr, ok := args[0].(string)
	if !ok {
		return nil, false, fmt.Errorf("arg must be string")
	}
	ua := user_agent.New(uaStr)
	name, ver := ua.Browser()
	out := map[string]interface{}{
		"browser":  name,
		"version":  ver,
		"os":       ua.OS(),
		"platform": ua.Platform(),
		"mobile":   ua.Mobile(),
		"bot":      ua.Bot(),
	}
	return out, true, nil
}
