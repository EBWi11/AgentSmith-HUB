package common

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()
var rdb *redis.Client

// RedisMetrics holds Redis server metrics
type RedisMetrics struct {
	ConnectedClients    int64  `json:"connected_clients"`
	UsedMemory          int64  `json:"used_memory"`
	UsedMemoryPeak      int64  `json:"used_memory_peak"`
	TotalConnections    int64  `json:"total_connections_received"`
	TotalCommands       int64  `json:"total_commands_processed"`
	InstantaneousOps    int64  `json:"instantaneous_ops_per_sec"`
	KeyspaceHits        int64  `json:"keyspace_hits"`
	KeyspaceMisses      int64  `json:"keyspace_misses"`
	ExpiredKeys         int64  `json:"expired_keys"`
	EvictedKeys         int64  `json:"evicted_keys"`
	UptimeInSeconds     int64  `json:"uptime_in_seconds"`
	UptimeInDays        int64  `json:"uptime_in_days"`
	ConnectedSlaves     int64  `json:"connected_slaves"`
	RejectedConnections int64  `json:"rejected_connections"`
	SyncFull            int64  `json:"sync_full"`
	SyncPartialOK       int64  `json:"sync_partial_ok"`
	SyncPartialErr      int64  `json:"sync_partial_err"`
	PubsubChannels      int64  `json:"pubsub_channels"`
	PubsubPatterns      int64  `json:"pubsub_patterns"`
	LatestForkUsec      int64  `json:"latest_fork_usec"`
	Role                string `json:"role"`
	Version             string `json:"version"`
	OS                  string `json:"os"`
	ProcessID           int64  `json:"process_id"`
	RunID               string `json:"run_id"`
	TCPPort             int64  `json:"tcp_port"`
	ConfigFile          string `json:"config_file"`
}

// GetRedisMetrics returns current Redis server metrics
func GetRedisMetrics() (*RedisMetrics, error) {
	info, err := rdb.Info(ctx).Result()
	if err != nil {
		return nil, err
	}

	metrics := &RedisMetrics{}
	lines := strings.Split(info, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "connected_clients":
			metrics.ConnectedClients, _ = strconv.ParseInt(value, 10, 64)
		case "used_memory":
			metrics.UsedMemory, _ = strconv.ParseInt(value, 10, 64)
		case "used_memory_peak":
			metrics.UsedMemoryPeak, _ = strconv.ParseInt(value, 10, 64)
		case "total_connections_received":
			metrics.TotalConnections, _ = strconv.ParseInt(value, 10, 64)
		case "total_commands_processed":
			metrics.TotalCommands, _ = strconv.ParseInt(value, 10, 64)
		case "instantaneous_ops_per_sec":
			metrics.InstantaneousOps, _ = strconv.ParseInt(value, 10, 64)
		case "keyspace_hits":
			metrics.KeyspaceHits, _ = strconv.ParseInt(value, 10, 64)
		case "keyspace_misses":
			metrics.KeyspaceMisses, _ = strconv.ParseInt(value, 10, 64)
		case "expired_keys":
			metrics.ExpiredKeys, _ = strconv.ParseInt(value, 10, 64)
		case "evicted_keys":
			metrics.EvictedKeys, _ = strconv.ParseInt(value, 10, 64)
		case "uptime_in_seconds":
			metrics.UptimeInSeconds, _ = strconv.ParseInt(value, 10, 64)
		case "uptime_in_days":
			metrics.UptimeInDays, _ = strconv.ParseInt(value, 10, 64)
		case "connected_slaves":
			metrics.ConnectedSlaves, _ = strconv.ParseInt(value, 10, 64)
		case "rejected_connections":
			metrics.RejectedConnections, _ = strconv.ParseInt(value, 10, 64)
		case "sync_full":
			metrics.SyncFull, _ = strconv.ParseInt(value, 10, 64)
		case "sync_partial_ok":
			metrics.SyncPartialOK, _ = strconv.ParseInt(value, 10, 64)
		case "sync_partial_err":
			metrics.SyncPartialErr, _ = strconv.ParseInt(value, 10, 64)
		case "pubsub_channels":
			metrics.PubsubChannels, _ = strconv.ParseInt(value, 10, 64)
		case "pubsub_patterns":
			metrics.PubsubPatterns, _ = strconv.ParseInt(value, 10, 64)
		case "latest_fork_usec":
			metrics.LatestForkUsec, _ = strconv.ParseInt(value, 10, 64)
		case "role":
			metrics.Role = value
		case "redis_version":
			metrics.Version = value
		case "os":
			metrics.OS = value
		case "process_id":
			metrics.ProcessID, _ = strconv.ParseInt(value, 10, 64)
		case "run_id":
			metrics.RunID = value
		case "tcp_port":
			metrics.TCPPort, _ = strconv.ParseInt(value, 10, 64)
		case "config_file":
			metrics.ConfigFile = value
		}
	}

	return metrics, nil
}

func RedisInit(addr string, passwd string) error {
	// Initialize Redis client
	rdb = redis.NewClient(&redis.Options{
		Addr:            addr,
		Password:        passwd,
		PoolSize:        64,
		MinIdleConns:    50,
		ConnMaxIdleTime: 30 * time.Second,
		ConnMaxLifetime: 5 * time.Minute,
		PoolTimeout:     2 * time.Second,
		DialTimeout:     2 * time.Second,
		ReadTimeout:     1 * time.Second,
		WriteTimeout:    1 * time.Second,
		MaxRetries:      2,
	})

	return RedisPing()
}

func RedisPing() error {
	// Ping the Redis server to check connection
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return err
	}
	return nil
}

func RedisGet(key string) (string, error) {
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return "", err
	}
	return val, nil
}

func RedisKeys(key string) ([]string, error) {
	return rdb.Keys(ctx, key).Result()
}

func RedisSet(key string, value interface{}, expiration int) (string, error) {
	return rdb.Set(ctx, key, value, time.Duration(expiration)*time.Second).Result()
}

func RedisSetNX(key string, value interface{}, expiration int) (bool, error) {
	return rdb.SetNX(ctx, key, value, time.Duration(expiration)*time.Second).Result()
}

func RedisIncr(key string) (int64, error) {
	return rdb.Incr(ctx, key).Result()
}

func RedisIncrby(key string, value int64) (int64, error) {
	return rdb.IncrBy(ctx, key, value).Result()
}

func RedisDel(key string) error {
	return rdb.Del(ctx, key).Err()
}

// ===================== Hash and Pub/Sub Helpers =====================

// RedisHSet sets a field in a Redis hash (no expiration)
func RedisHSet(hash string, field string, value interface{}) error {
	return rdb.HSet(ctx, hash, field, value).Err()
}

// RedisHGet gets a field from a Redis hash
func RedisHGet(hash string, field string) (string, error) {
	res, err := rdb.HGet(ctx, hash, field).Result()
	if err == redis.Nil {
		return "", nil
	}
	return res, err
}

// RedisPublish publishes a message to a Redis channel
func RedisPublish(channel string, message interface{}) error {
	return rdb.Publish(ctx, channel, message).Err()
}

// RedisHGetAll returns all field-value pairs of a hash
func RedisHGetAll(hash string) (map[string]string, error) {
	return rdb.HGetAll(ctx, hash).Result()
}

// GetRedisClient returns underlying redis client for advanced operations
func GetRedisClient() *redis.Client {
	return rdb
}

// ===================== List Helpers =====================

// RedisLPush pushes value to list head, keeps maxLen if >0
func RedisLPush(key string, value interface{}, maxLen int64) error {
	if err := rdb.LPush(ctx, key, value).Err(); err != nil {
		return err
	}
	if maxLen > 0 {
		_ = rdb.LTrim(ctx, key, 0, maxLen-1).Err()
	}
	return nil
}

// RedisLRange returns list range
func RedisLRange(key string, start, stop int64) ([]string, error) {
	return rdb.LRange(ctx, key, start, stop).Result()
}
