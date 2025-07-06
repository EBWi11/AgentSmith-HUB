package is_private_ip

import (
	"errors"
	"net"
)

// Eval returns true if the given IP string is a private / RFC1918 / link-local / loopback address.
// Usage in ruleset:
//
//	<node id="check_private" type="PLUGIN" func="is_private_ip" field="src_ip"/>
func Eval(args ...interface{}) (bool, error) {
	if len(args) != 1 {
		return false, errors.New("is_private_ip requires exactly 1 argument: ip string")
	}
	ipStr, ok := args[0].(string)
	if !ok {
		return false, errors.New("argument must be a string")
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false, nil // not IP
	}
	if ip.IsLoopback() {
		return true, nil
	}
	// Private / link-local ranges
	ranges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
	for _, cidr := range ranges {
		_, subnet, _ := net.ParseCIDR(cidr)
		if subnet.Contains(ip) {
			return true, nil
		}
	}
	return false, nil
}
