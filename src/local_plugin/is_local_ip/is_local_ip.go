package is_local_ip

import (
	"errors"
	"net"
)

func Eval(args ...interface{}) (bool, error) {
	var ipStr string
	var ok bool

	if len(args) == 1 {
		if ipStr, ok = args[0].(string); !ok {
			return false, errors.New("argument must be a string representing an IP address")
		}
	}

	ip := net.ParseIP(ipStr)

	if ip == nil {
		return false, nil
	}
	if ip.IsLoopback() {
		return true, nil
	}

	for _, cidr := range []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	} {
		_, subnet, _ := net.ParseCIDR(cidr)
		if subnet.Contains(ip) {
			return true, nil
		}
	}

	return false, nil
}
