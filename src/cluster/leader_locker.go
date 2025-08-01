package cluster

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"sync"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
)

var leaderLockerKey = "cluster:leader:lock"

type LeaderLocker struct {
	lock *redsync.Mutex
	done chan struct{}
	once sync.Once
}

func ObtainLeaderLocker() (*LeaderLocker, error) {

	lock := redsync.New(goredis.NewPool(common.GetRedisClient())).NewMutex(leaderLockerKey, redsync.WithExpiry(time.Minute), redsync.WithRetryDelay(time.Second), redsync.WithTries(60))
	err := lock.Lock()
	if err != nil {
		return nil, err
	}
	logger.Debug("Obtained leader locker", "lock_value", lock.Value())

	done := make(chan struct{})
	locker := &LeaderLocker{lock: lock, done: done}

	go locker.startRefreshLoop()

	return locker, nil
}

func (l *LeaderLocker) startRefreshLoop() {
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := l.refreshLock(); err != nil {
				logger.Error("Failed to refresh leader locker", "error", err)
			}
			logger.Debug("Leader locker refreshed", "lock_value", l.lock.Value())
		case <-l.done:
			return
		}
	}
}

func (l *LeaderLocker) refreshLock() error {
	_, err := l.lock.Extend()
	return err
}

func (l *LeaderLocker) Release() {
	l.once.Do(func() {
		close(l.done)

		if _, err := l.lock.Unlock(); err != nil {
			logger.Error("Failed to release leader locker", "error", err)
		}
		logger.Debug("Leader locker released", "lock_value", l.lock.Value())
	})
}
