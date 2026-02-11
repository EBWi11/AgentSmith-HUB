package rules_engine

import (
	"AgentSmith-HUB/common"
	"AgentSmith-HUB/logger"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/klauspost/compress/zstd"
)

// ============================================================================
// CEP State Manager
//
// Manages partial match state for CEP sequence detection.
// Supports two backends:
//   - Redis (default): distributed state for cluster deployments
//   - Local cache (Ristretto): high-performance single-node state
//
// State key format: SEQ_{hash(GroupByID + correlateValue)}
// ============================================================================

const (
	// CEPStateKeyPrefix is the prefix for all CEP sequence state keys in Redis/cache.
	CEPStateKeyPrefix = "SEQ_"

	// CEPAbsenceKeyPrefix is the prefix for absence tracking keys in Redis.
	// These keys store metadata for sequences that have absence stages,
	// enabling distributed absence scanning across cluster nodes.
	CEPAbsenceKeyPrefix = "SEQABS_"
)

// ============================================================================
// Zstd Compression for Redis State
//
// Event data snapshots stored in Redis can be large (multiple events * many fields).
// Zstd provides excellent compression ratio with very fast decompression,
// significantly reducing Redis memory usage and network bandwidth.
// ============================================================================

var (
	zstdEncoder *zstd.Encoder
	zstdDecoder *zstd.Decoder
	zstdOnce    sync.Once
)

// initZstd lazily initializes the shared zstd encoder/decoder (both are thread-safe).
func initZstd() {
	zstdOnce.Do(func() {
		var err error
		// SpeedDefault balances compression ratio and speed
		zstdEncoder, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			logger.Error("Failed to create zstd encoder, compression disabled", "error", err)
		}
		zstdDecoder, err = zstd.NewReader(nil)
		if err != nil {
			logger.Error("Failed to create zstd decoder, compression disabled", "error", err)
		}
	})
}

// serializeState serializes a SequenceState to JSON, optionally compressing with zstd.
// When compress=true, returns zstd-compressed bytes as a string.
// When compress=false, returns plain JSON string.
func serializeState(state *SequenceState, compress bool) (string, error) {
	sj := sequenceStateJSON{
		StageMatches: state.StageMatches,
		CreatedAt:    state.CreatedAt,
		ExpiresAt:    state.ExpiresAt,
	}
	jsonData, err := json.Marshal(sj)
	if err != nil {
		return "", err
	}

	if !compress {
		return string(jsonData), nil
	}

	initZstd()
	if zstdEncoder == nil {
		// Fallback: store uncompressed if encoder init failed
		return string(jsonData), nil
	}

	compressed := zstdEncoder.EncodeAll(jsonData, nil)
	return string(compressed), nil
}

// decompressState decompresses a zstd-compressed string and deserializes to SequenceState.
// Transparently handles both compressed and uncompressed (legacy) data.
func decompressState(data string) (*SequenceState, error) {
	raw := []byte(data)

	initZstd()

	var jsonData []byte
	// Zstd magic number: 0x28 0xB5 0x2F 0xFD
	if len(raw) >= 4 && raw[0] == 0x28 && raw[1] == 0xB5 && raw[2] == 0x2F && raw[3] == 0xFD {
		// Compressed data
		if zstdDecoder == nil {
			return nil, fmt.Errorf("zstd decoder not available")
		}
		var err error
		jsonData, err = zstdDecoder.DecodeAll(raw, nil)
		if err != nil {
			return nil, fmt.Errorf("zstd decompress failed: %w", err)
		}
	} else {
		// Uncompressed legacy data (plain JSON)
		jsonData = raw
	}

	var sj sequenceStateJSON
	if err := json.Unmarshal(jsonData, &sj); err != nil {
		return nil, err
	}

	return &SequenceState{
		StageMatches: sj.StageMatches,
		CreatedAt:    sj.CreatedAt,
		ExpiresAt:    sj.ExpiresAt,
	}, nil
}

// CEPStateManager encapsulates state operations for CEP sequences.
type CEPStateManager struct {
	useLocalCache bool
	cache         *ristretto.Cache[string, *SequenceState]
	mu            sync.Mutex // Protects local cache read-modify-write operations

	// Per-key locks prevent concurrent modification of the same SequenceState.
	// Multiple goroutines (from different upstreams) may process events for the
	// same correlation key simultaneously. Without per-key locking, concurrent
	// AddMatch calls would cause map data races.
	keyLocks sync.Map // map[string]*sync.Mutex

	// For absence scanning: track keys that have absence stages (local cache mode)
	absenceKeys   map[string]absenceKeyInfo
	absenceKeysMu sync.Mutex
}

// absenceKeyInfo tracks metadata for absence scanning.
type absenceKeyInfo struct {
	ExpiresAt int64  // When the sequence window expires (unix ms)
	RuleID    string // For locating the rule/sequence when triggering
	SeqID     int    // SequenceMap key
}

// NewCEPStateManager creates a new state manager.
func NewCEPStateManager(useLocalCache bool, cache *ristretto.Cache[string, *SequenceState]) *CEPStateManager {
	return &CEPStateManager{
		useLocalCache: useLocalCache,
		cache:         cache,
		absenceKeys:   make(map[string]absenceKeyInfo),
	}
}

// BuildStateKey constructs a state key from the GroupByID and correlation values.
func BuildStateKey(groupByID string, correlateValues string) string {
	return CEPStateKeyPrefix + common.XXHash64(groupByID+correlateValues)
}

// LockKey acquires a per-key mutex, ensuring exclusive access to a SequenceState.
// Must be paired with UnlockKey. Used to protect the entire read-modify-write cycle.
func (m *CEPStateManager) LockKey(key string) {
	actual, _ := m.keyLocks.LoadOrStore(key, &sync.Mutex{})
	actual.(*sync.Mutex).Lock()
}

// UnlockKey releases the per-key mutex.
func (m *CEPStateManager) UnlockKey(key string) {
	if actual, ok := m.keyLocks.Load(key); ok {
		actual.(*sync.Mutex).Unlock()
	}
}

// CleanupKeyLock removes a key lock entry after the state is deleted.
// Prevents unbounded growth of the keyLocks map.
func (m *CEPStateManager) CleanupKeyLock(key string) {
	m.keyLocks.Delete(key)
}

// GetState retrieves a SequenceState by key.
// Returns nil if not found or expired.
func (m *CEPStateManager) GetState(key string) *SequenceState {
	if m.useLocalCache {
		return m.getLocalState(key)
	}
	return m.getRedisState(key)
}

// SetState stores a SequenceState with the given TTL.
// The compress flag controls whether zstd compression is used for Redis storage.
func (m *CEPStateManager) SetState(key string, state *SequenceState, ttlSeconds int, compress bool) {
	if m.useLocalCache {
		m.setLocalState(key, state, ttlSeconds)
		return
	}
	m.setRedisState(key, state, ttlSeconds, compress)
}

// DeleteState removes a SequenceState by key.
func (m *CEPStateManager) DeleteState(key string) {
	if m.useLocalCache {
		m.deleteLocalState(key)
		return
	}
	m.deleteRedisState(key)
}

// TrackAbsenceKey registers a key for absence scanning.
// In Redis mode, stores a separate tracking key so any cluster node can discover it.
func (m *CEPStateManager) TrackAbsenceKey(key string, info absenceKeyInfo) {
	// Always track locally for fast lookup
	m.absenceKeysMu.Lock()
	m.absenceKeys[key] = info
	m.absenceKeysMu.Unlock()

	// In Redis mode, also store a Redis tracking key for distributed scanning
	if !m.useLocalCache {
		m.setRedisAbsenceTracker(key, info)
	}
}

// UntrackAbsenceKey removes a key from absence scanning.
func (m *CEPStateManager) UntrackAbsenceKey(key string) {
	m.absenceKeysMu.Lock()
	delete(m.absenceKeys, key)
	m.absenceKeysMu.Unlock()

	// In Redis mode, also remove the Redis tracking key
	if !m.useLocalCache {
		absKey := CEPAbsenceKeyPrefix + key
		_ = common.RedisDel(absKey)
	}
}

// GetExpiredAbsenceKeys returns all absence keys that have expired (ExpiresAt <= nowMs).
// In local cache mode, uses the in-memory map.
// In Redis mode, scans Redis for SEQABS_* keys to enable distributed absence detection.
func (m *CEPStateManager) GetExpiredAbsenceKeys(nowMs int64) map[string]absenceKeyInfo {
	if !m.useLocalCache {
		return m.getExpiredAbsenceKeysRedis(nowMs)
	}
	return m.getExpiredAbsenceKeysLocal(nowMs)
}

// getExpiredAbsenceKeysLocal checks the local in-memory map for expired absence keys.
func (m *CEPStateManager) getExpiredAbsenceKeysLocal(nowMs int64) map[string]absenceKeyInfo {
	m.absenceKeysMu.Lock()
	defer m.absenceKeysMu.Unlock()

	expired := make(map[string]absenceKeyInfo)
	for key, info := range m.absenceKeys {
		if info.ExpiresAt <= nowMs {
			expired[key] = info
			delete(m.absenceKeys, key)
		}
	}
	return expired
}

// getExpiredAbsenceKeysRedis scans Redis for expired absence tracking keys.
// This enables any node in the cluster to detect absence timeouts.
func (m *CEPStateManager) getExpiredAbsenceKeysRedis(nowMs int64) map[string]absenceKeyInfo {
	// First check local map (fast path for keys tracked by this node)
	localExpired := m.getExpiredAbsenceKeysLocal(nowMs)

	// Then scan Redis for all SEQABS_* keys (distributed discovery)
	pattern := CEPAbsenceKeyPrefix + "*"
	redisKeys, err := common.RedisKeys(pattern)
	if err != nil {
		if len(localExpired) > 0 {
			return localExpired
		}
		return nil
	}

	expired := localExpired
	if expired == nil {
		expired = make(map[string]absenceKeyInfo)
	}

	for _, absKey := range redisKeys {
		// Extract the original state key
		stateKey := absKey[len(CEPAbsenceKeyPrefix):]

		// Skip if already found via local map
		if _, found := expired[stateKey]; found {
			continue
		}

		// Read the absence tracking info
		info := m.getRedisAbsenceTracker(absKey)
		if info == nil {
			continue
		}

		if info.ExpiresAt <= nowMs {
			expired[stateKey] = *info
			// Clean up the tracking key
			_ = common.RedisDel(absKey)
		}
	}

	return expired
}

// absenceTrackerJSON is the JSON format for Redis absence tracking keys.
type absenceTrackerJSON struct {
	ExpiresAt int64  `json:"ea"`
	RuleID    string `json:"rid"`
	SeqID     int    `json:"sid"`
}

// setRedisAbsenceTracker stores an absence tracking key in Redis.
func (m *CEPStateManager) setRedisAbsenceTracker(stateKey string, info absenceKeyInfo) {
	absKey := CEPAbsenceKeyPrefix + stateKey
	data, err := json.Marshal(absenceTrackerJSON{
		ExpiresAt: info.ExpiresAt,
		RuleID:    info.RuleID,
		SeqID:     info.SeqID,
	})
	if err != nil {
		return
	}

	// TTL = time until expiration + 10s buffer for scanning latency
	ttlMs := info.ExpiresAt - time.Now().UnixMilli() + 10000
	if ttlMs < 5000 {
		ttlMs = 5000 // Minimum 5 seconds
	}
	ttlSec := int(ttlMs / 1000)

	if _, err := common.RedisSet(absKey, string(data), ttlSec); err != nil {
		logger.Warn("Failed to set absence tracker in Redis", "key", absKey, "error", err)
	}
}

// getRedisAbsenceTracker reads an absence tracking key from Redis.
func (m *CEPStateManager) getRedisAbsenceTracker(absKey string) *absenceKeyInfo {
	val, err := common.RedisGet(absKey)
	if err != nil || val == "" {
		return nil
	}

	var at absenceTrackerJSON
	if err := json.Unmarshal([]byte(val), &at); err != nil {
		return nil
	}

	return &absenceKeyInfo{
		ExpiresAt: at.ExpiresAt,
		RuleID:    at.RuleID,
		SeqID:     at.SeqID,
	}
}

// --- Local Cache Backend ---

func (m *CEPStateManager) getLocalState(key string) *SequenceState {
	if m.cache == nil {
		return nil
	}
	val, found := m.cache.Get(key)
	if !found {
		return nil
	}
	return val
}

func (m *CEPStateManager) setLocalState(key string, state *SequenceState, ttlSeconds int) {
	if m.cache == nil {
		return
	}
	m.cache.SetWithTTL(key, state, 1, time.Duration(ttlSeconds)*time.Second)
	m.cache.Wait()
}

func (m *CEPStateManager) deleteLocalState(key string) {
	if m.cache == nil {
		return
	}
	m.cache.Del(key)
	m.UntrackAbsenceKey(key)
}

// --- Redis Backend ---

// sequenceStateJSON is the JSON representation of SequenceState for Redis storage.
type sequenceStateJSON struct {
	StageMatches map[int][]StageMatch `json:"sm"`
	CreatedAt    int64                `json:"ca"`
	ExpiresAt    int64                `json:"ea"`
}

// getRedisState reads state from Redis, auto-detecting compressed vs plain JSON.
func (m *CEPStateManager) getRedisState(key string) *SequenceState {
	val, err := common.RedisGet(key)
	if err != nil || val == "" {
		return nil
	}

	state, err := decompressState(val)
	if err != nil {
		logger.Warn("Failed to deserialize CEP state from Redis", "key", key, "error", err)
		return nil
	}
	return state
}

// setRedisState writes state to Redis, optionally with zstd compression.
func (m *CEPStateManager) setRedisState(key string, state *SequenceState, ttlSeconds int, compress bool) {
	data, err := serializeState(state, compress)
	if err != nil {
		logger.Warn("Failed to serialize CEP state for Redis", "key", key, "error", err)
		return
	}

	if _, err := common.RedisSet(key, data, ttlSeconds); err != nil {
		logger.Warn("Failed to set CEP state in Redis", "key", key, "error", err)
	}
}

func (m *CEPStateManager) deleteRedisState(key string) {
	// Delete main state key
	if err := common.RedisDel(key); err != nil {
		logger.Warn("Failed to delete CEP state from Redis", "key", key, "error", err)
	}
	// Delete absence tracking key
	absKey := CEPAbsenceKeyPrefix + key
	_ = common.RedisDel(absKey)

	// Remove from local tracking
	m.absenceKeysMu.Lock()
	delete(m.absenceKeys, key)
	m.absenceKeysMu.Unlock()
}

// ============================================================================
// Memory Control Constants
// ============================================================================

const (
	// MaxStageMatchesPerStage limits how many matches to keep per stage.
	// Prevents unbounded memory growth when many events match the same stage.
	MaxStageMatchesPerStage = 100

	// MaxPartialMatchesWarning logs a warning when partial matches exceed this count.
	MaxPartialMatchesWarning = 10000
)

// GetOrCreateState atomically gets or creates a SequenceState for the given key.
// The compress flag controls whether zstd compression is used for Redis storage.
// IMPORTANT: The caller MUST hold the per-key lock via LockKey before calling this method.
func (m *CEPStateManager) GetOrCreateState(key string, withinMs int64, ttlSeconds int, compress bool) *SequenceState {
	if m.useLocalCache {
		// Per-key lock is held by caller; no global lock needed for local cache.
		state := m.getLocalState(key)
		if state == nil {
			nowMs := time.Now().UnixMilli()
			state = NewSequenceState(nowMs, nowMs+withinMs)
			m.setLocalState(key, state, ttlSeconds)
		}
		return state
	}

	// Redis: use Lua script for atomic get-or-create
	return m.getOrCreateRedisStateAtomic(key, withinMs, ttlSeconds, compress)
}

// UpdateState atomically updates the state after modifications.
// The compress flag controls whether zstd compression is used for Redis storage.
// Applies memory control before persisting.
// IMPORTANT: The caller MUST hold the per-key lock via LockKey before calling this method.
func (m *CEPStateManager) UpdateState(key string, state *SequenceState, ttlSeconds int, compress bool) {
	// Apply memory control: trim excess stage matches
	trimStageMatches(state)

	if m.useLocalCache {
		// Per-key lock is held by caller; no global lock needed for local cache.
		m.setLocalState(key, state, ttlSeconds)
		return
	}
	m.updateRedisStateAtomic(key, state, ttlSeconds, compress)
}

// trimStageMatches enforces the MaxStageMatchesPerStage limit.
// Keeps the most recent matches (highest timestamps) for each stage.
func trimStageMatches(state *SequenceState) {
	for stageIdx, matches := range state.StageMatches {
		if len(matches) > MaxStageMatchesPerStage {
			// Keep only the last MaxStageMatchesPerStage entries (sorted by timestamp ascending)
			state.StageMatches[stageIdx] = matches[len(matches)-MaxStageMatchesPerStage:]
		}
	}
}

// ============================================================================
// Redis Lua Scripts for Atomic Operations
// ============================================================================

// luaGetOrCreate atomically gets an existing state or creates a new one.
// KEYS[1] = state key
// ARGV[1] = new state JSON (used only if key doesn't exist)
// ARGV[2] = TTL in seconds
// Returns: the state JSON (either existing or newly created)
const luaGetOrCreate = `
local val = redis.call('GET', KEYS[1])
if val then
    return val
end
redis.call('SET', KEYS[1], ARGV[1], 'EX', tonumber(ARGV[2]))
return ARGV[1]
`

// luaAtomicUpdate atomically updates state using compare-and-swap.
// KEYS[1] = state key
// ARGV[1] = new state JSON
// ARGV[2] = TTL in seconds
// Returns: "OK"
const luaAtomicUpdate = `
redis.call('SET', KEYS[1], ARGV[1], 'EX', tonumber(ARGV[2]))
return "OK"
`

// getOrCreateRedisStateAtomic uses a Lua script for atomic get-or-create.
func (m *CEPStateManager) getOrCreateRedisStateAtomic(key string, withinMs int64, ttlSeconds int, compress bool) *SequenceState {
	nowMs := time.Now().UnixMilli()
	newState := NewSequenceState(nowMs, nowMs+withinMs)

	serialized, err := serializeState(newState, compress)
	if err != nil {
		logger.Warn("Failed to serialize new CEP state", "key", key, "error", err)
		return newState
	}

	result, err := common.RedisEval(luaGetOrCreate, []string{key}, serialized, ttlSeconds)
	if err != nil {
		// Fallback to non-atomic get-then-set
		logger.Warn("Lua get-or-create failed, falling back", "key", key, "error", err)
		state := m.getRedisState(key)
		if state == nil {
			m.setRedisState(key, newState, ttlSeconds, compress)
			return newState
		}
		return state
	}

	// Parse the result (auto-detects compressed vs plain JSON)
	resultStr, ok := result.(string)
	if !ok {
		return newState
	}

	state, err := decompressState(resultStr)
	if err != nil {
		return newState
	}
	return state
}

// updateRedisStateAtomic uses a Lua script for atomic state update.
func (m *CEPStateManager) updateRedisStateAtomic(key string, state *SequenceState, ttlSeconds int, compress bool) {
	serialized, err := serializeState(state, compress)
	if err != nil {
		logger.Warn("Failed to serialize CEP state for atomic update", "key", key, "error", err)
		return
	}

	_, err = common.RedisEval(luaAtomicUpdate, []string{key}, serialized, ttlSeconds)
	if err != nil {
		// Fallback to regular SET
		logger.Warn("Lua atomic update failed, falling back", "key", key, "error", err)
		m.setRedisState(key, state, ttlSeconds, compress)
	}
}

// String returns a debug string for the state manager.
func (m *CEPStateManager) String() string {
	if m.useLocalCache {
		return fmt.Sprintf("CEPStateManager{backend=local_cache}")
	}
	return fmt.Sprintf("CEPStateManager{backend=redis}")
}
