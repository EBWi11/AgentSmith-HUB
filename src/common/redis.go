package common

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Circuit breaker states
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// CircuitBreaker implements circuit breaker pattern for Redis operations
type CircuitBreaker struct {
	maxFailures     int
	resetTimeout    time.Duration
	state           CircuitState
	failures        int
	lastFailureTime time.Time
	mutex           sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        CircuitClosed,
	}
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Check if we should transition from Open to Half-Open
	if cb.state == CircuitOpen {
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			cb.failures = 0
		} else {
			return fmt.Errorf("circuit breaker is open")
		}
	}

	// Execute the function
	err := fn()

	if err != nil {
		cb.failures++
		cb.lastFailureTime = time.Now()

		if cb.failures >= cb.maxFailures {
			cb.state = CircuitOpen
		}
		return err
	}

	// Success - reset circuit breaker
	cb.failures = 0
	cb.state = CircuitClosed
	return nil
}

// IsOpen returns true if circuit breaker is open
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state == CircuitOpen
}

// RedisFailureHandler handles Redis operation failures with retry and circuit breaker
type RedisFailureHandler struct {
	circuitBreaker *CircuitBreaker
	localCache     map[string]CacheEntry
	cacheMutex     sync.RWMutex
}

// CacheEntry represents a cached Redis value
type CacheEntry struct {
	Value     string
	Timestamp time.Time
	TTL       time.Duration
}

// NewRedisFailureHandler creates a new Redis failure handler
func NewRedisFailureHandler() *RedisFailureHandler {
	return &RedisFailureHandler{
		circuitBreaker: NewCircuitBreaker(5, 30*time.Second), // 5 failures, 30s reset
		localCache:     make(map[string]CacheEntry),
	}
}

// Global Redis failure handler instance
var redisFailureHandler *RedisFailureHandler

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

	// Initialize failure handler
	redisFailureHandler = NewRedisFailureHandler()

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

// RedisHDel deletes a field from a Redis hash
func RedisHDel(key string, field string) error {
	return rdb.HDel(ctx, key, field).Err()
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

// RedisExpire sets the expiration time for a key
func RedisExpire(key string, expiration int) error {
	return rdb.Expire(ctx, key, time.Duration(expiration)*time.Second).Err()
}

// ===================== Project Config Helpers =====================

// StoreProjectConfig stores project configuration in Redis
func StoreProjectConfig(projectID string, config string) error {
	key := "cluster:project_config:" + projectID
	// Store with 7 days TTL
	return rdb.Set(ctx, key, config, 7*24*time.Hour).Err()
}

// GetProjectConfig retrieves project configuration from Redis
func GetProjectConfig(projectID string) (string, error) {
	key := "cluster:project_config:" + projectID
	return rdb.Get(ctx, key).Result()
}

// DeleteProjectConfig removes project configuration from Redis
func DeleteProjectConfig(projectID string) error {
	key := "cluster:project_config:" + projectID
	return rdb.Del(ctx, key).Err()
}

// RetryWithExponentialBackoff executes a function with exponential backoff retry
func RetryWithExponentialBackoff(fn func() error, maxRetries int, baseDelay time.Duration) error {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		if i < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<uint(i)) // Exponential backoff
			time.Sleep(delay)
		}
	}

	return lastErr
}

// RedisPublishWithRetry publishes a message with exponential backoff retry
func RedisPublishWithRetry(channel string, message string) error {
	return RetryWithExponentialBackoff(func() error {
		return redisFailureHandler.circuitBreaker.Call(func() error {
			return rdb.Publish(ctx, channel, message).Err()
		})
	}, 3, 100*time.Millisecond)
}

// DistributedLock represents a Redis-based distributed lock
type DistributedLock struct {
	key        string
	value      string
	expiration time.Duration
	acquired   bool
}

// NewDistributedLock creates a new distributed lock
func NewDistributedLock(key string, expiration time.Duration) *DistributedLock {
	return &DistributedLock{
		key:        "lock:" + key,
		value:      fmt.Sprintf("%d", time.Now().UnixNano()),
		expiration: expiration,
	}
}

// Acquire attempts to acquire the distributed lock
func (dl *DistributedLock) Acquire() error {
	return redisFailureHandler.circuitBreaker.Call(func() error {
		acquired, err := rdb.SetNX(ctx, dl.key, dl.value, dl.expiration).Result()
		if err != nil {
			return err
		}
		if !acquired {
			return fmt.Errorf("lock already acquired")
		}
		dl.acquired = true
		return nil
	})
}

// Release releases the distributed lock
func (dl *DistributedLock) Release() error {
	if !dl.acquired {
		return nil
	}

	// Use Lua script to ensure atomic release
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`

	return redisFailureHandler.circuitBreaker.Call(func() error {
		_, err := rdb.Eval(ctx, script, []string{dl.key}, dl.value).Result()
		if err != nil {
			return err
		}
		dl.acquired = false
		return nil
	})
}

// TryAcquire attempts to acquire the lock with a timeout
func (dl *DistributedLock) TryAcquire(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		err := dl.Acquire()
		if err == nil {
			return nil
		}

		// Wait a bit before retrying
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("failed to acquire lock within timeout")
}

// ===================== Project State Management (New Design) =====================

// SetProjectUserIntention sets the user intention for a project (what user wants)
// Only "running" is stored; "stopped" projects have their keys removed
// Uses proj_states key for user intention
func SetProjectUserIntention(nodeID, projectID string, wantRunning bool) error {
	key := ProjectLegacyStateKeyPrefix + nodeID

	if wantRunning {
		// User wants project to be running
		return RedisHSet(key, projectID, "running")
	} else {
		// User wants project to be stopped - remove the key
		return RedisHDel(key, projectID)
	}
}

// GetProjectUserIntention gets the user intention for a project
// Returns true if user wants it running, false if stopped/not found
// Uses proj_states key for user intention
func GetProjectUserIntention(nodeID, projectID string) (bool, error) {
	key := ProjectLegacyStateKeyPrefix + nodeID
	value, err := RedisHGet(key, projectID)
	if err != nil {
		// Key not found means user wants it stopped
		return false, nil
	}
	return value == "running", nil
}

// GetAllProjectUserIntentions gets all user intentions for a node
// Returns map of projectID -> bool (true=running, false=stopped)
// Uses proj_states key for user intention
func GetAllProjectUserIntentions(nodeID string) (map[string]bool, error) {
	key := ProjectLegacyStateKeyPrefix + nodeID
	values, err := RedisHGetAll(key)
	if err != nil {
		return make(map[string]bool), nil
	}

	result := make(map[string]bool)
	for projectID, state := range values {
		result[projectID] = (state == "running")
	}
	return result, nil
}

// SetProjectRealState sets the actual runtime state for a project
// Stores the real current status: running, stopped, error, starting, stopping
// Uses proj_real key for actual state
func SetProjectRealState(nodeID, projectID string, actualState string) error {
	key := ProjectRealStateKeyPrefix + nodeID
	return RedisHSet(key, projectID, actualState)
}

// GetProjectRealState gets the actual runtime state for a project
// Returns the actual state string or empty string if not found
// Uses proj_real key for actual state
func GetProjectRealState(nodeID, projectID string) (string, error) {
	key := ProjectRealStateKeyPrefix + nodeID
	return RedisHGet(key, projectID)
}

// GetAllProjectRealStates gets all actual states for a node
// Returns map of projectID -> actualState
// Uses proj_real key for actual state
func GetAllProjectRealStates(nodeID string) (map[string]string, error) {
	key := ProjectRealStateKeyPrefix + nodeID
	values, err := RedisHGetAll(key)
	if err != nil {
		return make(map[string]string), nil
	}
	return values, nil
}

// SetProjectStateTimestamp sets the timestamp for a project state change
func SetProjectStateTimestamp(nodeID, projectID string, timestamp time.Time) error {
	key := ProjectStateTimestampKeyPrefix + nodeID
	return RedisHSet(key, projectID, timestamp.Format(time.RFC3339))
}

// GetAllProjectStateTimestamps gets all state change timestamps for a node
func GetAllProjectStateTimestamps(nodeID string) (map[string]time.Time, error) {
	key := ProjectStateTimestampKeyPrefix + nodeID
	values, err := RedisHGetAll(key)
	if err != nil {
		return make(map[string]time.Time), nil
	}

	result := make(map[string]time.Time)
	for projectID, timestampStr := range values {
		if timestamp, err := time.Parse(time.RFC3339, timestampStr); err == nil {
			result[projectID] = timestamp
		}
	}
	return result, nil
}

// GetKnownNodes returns all known nodes that have been tracked by the leader
func GetKnownNodes() ([]string, error) {
	if rdb == nil {
		return nil, fmt.Errorf("Redis client not initialized")
	}

	// Get all known node keys
	pattern := "cluster:known_nodes:*"
	keys, err := RedisKeys(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to get known node keys: %w", err)
	}

	// Extract unique node IDs from keys
	var nodes []string
	for _, key := range keys {
		// Extract node ID from key format: cluster:known_nodes:nodeID
		if strings.HasPrefix(key, "cluster:known_nodes:") {
			nodeID := key[len("cluster:known_nodes:"):]
			if nodeID != "" {
				nodes = append(nodes, nodeID)
			}
		}
	}

	// Sort nodes for consistent ordering
	sort.Strings(nodes)

	return nodes, nil
}
