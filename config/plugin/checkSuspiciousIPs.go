package plugin

import (
	"sync"
	"time"
)

var lastTime time.Time
var lock = &sync.RWMutex{}
var suspiciousIpsList = make(map[string]bool)

var defaultSuspiciousIpsList = map[string]bool{
	"204.79.46.27":    true,
	"29.37.14.55":     true,
	"130.198.28.202":  true,
	"151.119.173.220": true,
	"28.105.225.46":   true,
	"254.72.24.166":   true,
	"63.215.82.210":   true,
	"222.165.122.9":   true,
	"99.94.123.210":   true,
	"13.33.10.105":    true,
}

func getSuspiciousIps() {
	lock.Lock()
	// This function should fetch the latest suspicious IPs from a database or an external source.
	lock.Unlock()
	lastTime = time.Now()
}

func init() {
	getSuspiciousIps()
}

func Eval(ip string) (bool, error) {
	t := time.Now()
	if t.Sub(lastTime) > 60*time.Minute {
		getSuspiciousIps()
	}

	lock.RLock()
	res := suspiciousIpsList[ip]
	if !res {
		res = defaultSuspiciousIpsList[ip]
	}
	lock.RUnlock()

	return res, nil
}
