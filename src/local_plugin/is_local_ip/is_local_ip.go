package is_local_ip

import "net"

func Eval(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	if ip.IsLoopback() {
		return true
	}

	for _, cidr := range []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16", // 链路本地地址
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 本地地址（ULA）
		"fe80::/10",      // IPv6 链路本地地址
	} {
		_, subnet, _ := net.ParseCIDR(cidr)
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}
