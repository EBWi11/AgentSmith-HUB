package plugin

import (
	"sync"
	"time"
)

var lastTime time.Time
var lock = &sync.RWMutex{}
var suspiciousIpsList = make(map[string]bool)

var defaultSuspiciousIpsList = map[string]bool{
	"1.2.3.4": true,
	"1.2.3.5": true,
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
