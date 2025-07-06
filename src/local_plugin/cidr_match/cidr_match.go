package cidr_match

import (
	"errors"
	"net"
)

// Eval returns true if ipStr is within cidrStr.
// Usage:
//
//	<node type="PLUGIN" func="cidr_match" args="{ip},{cidr}"/>
func Eval(args ...interface{}) (bool, error) {
	if len(args) != 2 {
		return false, errors.New("cidr_match requires 2 arguments: ip, cidr")
	}
	ipStr, ok1 := args[0].(string)
	cidrStr, ok2 := args[1].(string)
	if !ok1 || !ok2 {
		return false, errors.New("arguments must be strings")
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false, nil
	}
	_, subnet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return false, err
	}
	return subnet.Contains(ip), nil
}
