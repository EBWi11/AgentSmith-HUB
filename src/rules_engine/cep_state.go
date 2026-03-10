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

	// TimingWheelSlots is the number of slots in the absence timing wheel (local cache mode).
	// One slot per second; 3600 slots = 1 hour. Expired keys are only checked in the current
	// second's bucket, reducing scan cost from O(all keys) to O(bucket size) per tick.
	TimingWheelSlots = 3600
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

// serializeState serializes a SequenceState to zstd-compressed JSON.
func serializeState(state *SequenceState) (string, error) {
	sj := sequenceStateJSON{
		StageMatches: state.StageMatches,
		Context:      state.Context,
		CreatedAt:    state.CreatedAt,
		ExpiresAt:    state.ExpiresAt,
	}
	jsonData, err := json.Marshal(sj)
	if err != nil {
		return "", err
	}

	initZstd()
	if zstdEncoder == nil {
		return "", fmt.Errorf("zstd encoder not available")
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
		Context:      sj.Context,
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

	// Timing wheel for absence expiry (local cache only). Only the current second's bucket
	// is scanned each tick instead of the full key set. Slot index = (ExpiresAt/1000) % TimingWheelSlots.
	absenceWheel [][]absenceSlotItem
	// Tracks which slot a key is currently in, allowing O(bucket) replacement/removal
	// instead of append-only growth for repeated TrackAbsenceKey calls.
	absenceKeySlot map[string]int
	// Last second processed by local timing wheel scan; used to catch up missed ticks.
	lastProcessedSecond int64
}

// absenceKeyInfo tracks metadata for absence scanning.
type absenceKeyInfo struct {
	ExpiresAt int64  // When the sequence window expires (unix ms)
	RuleID    string // For locating the rule/sequence when triggering
	SeqID     int    // SequenceMap key
}

// absenceSlotItem is a single entry in a timing wheel bucket (for local cache absence scanning).
type absenceSlotItem struct {
	Key  string
	Info absenceKeyInfo
}

// NewCEPStateManager creates a new state manager.
func NewCEPStateManager(useLocalCache bool, cache *ristretto.Cache[string, *SequenceState]) *CEPStateManager {
	m := &CEPStateManager{
		useLocalCache: useLocalCache,
		cache:         cache,
		absenceKeys:   make(map[string]absenceKeyInfo),
	}
	if useLocalCache {
		m.absenceWheel = make([][]absenceSlotItem, TimingWheelSlots)
		m.absenceKeySlot = make(map[string]int)
		m.lastProcessedSecond = -1
	}
	return m
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
// NOTE: intentionally kept as no-op to avoid lock replacement races:
// deleting a lock while another goroutine still holds it can cause a later
// goroutine to recreate a new mutex for the same key, breaking exclusion and
// leading to unlock panics. We prefer correctness/stability here.
func (m *CEPStateManager) CleanupKeyLock(key string) {
	_ = key
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
func (m *CEPStateManager) SetState(key string, state *SequenceState, ttlSeconds int) {
	if m.useLocalCache {
		m.setLocalState(key, state, ttlSeconds)
		return
	}
	m.setRedisState(key, state, ttlSeconds)
}

// DeleteState removes a SequenceState by key.
func (m *CEPStateManager) DeleteState(key string) {
	if m.useLocalCache {
		m.deleteLocalState(key)
		return
	}
	m.deleteRedisState(key)
}

func timingWheelSlot(expiresAtMs int64) int {
	// Round up to the next second boundary so the scanner processes this slot
	// AFTER the key has definitely expired. Without rounding up, the key lands
	// in the slot for the truncated second, and the scanner may visit that slot
	// while nowMs < ExpiresAt (because ExpiresAt has sub-second precision).
	// By the next second the scanner moves to a different slot and the key is
	// never found.
	sec := (expiresAtMs + 999) / 1000
	slot := sec % int64(TimingWheelSlots)
	if slot < 0 {
		slot += int64(TimingWheelSlots)
	}
	return int(slot)
}

// removeFromWheelSlot removes a key from a specific wheel slot.
// Caller must hold absenceKeysMu.
func (m *CEPStateManager) removeFromWheelSlot(key string, slot int) {
	if m.absenceWheel == nil || slot < 0 || slot >= len(m.absenceWheel) {
		return
	}
	bucket := m.absenceWheel[slot]
	for i := range bucket {
		if bucket[i].Key == key {
			m.absenceWheel[slot] = append(bucket[:i], bucket[i+1:]...)
			return
		}
	}
}

// collectExpiredFromWheelSlot collects expired entries from a given slot into expired map.
// Uses absenceKeys as canonical source to prevent stale wheel entries from triggering.
// Caller must hold absenceKeysMu.
func (m *CEPStateManager) collectExpiredFromWheelSlot(slot int, nowMs int64, expired map[string]absenceKeyInfo) {
	if m.absenceWheel == nil || slot < 0 || slot >= len(m.absenceWheel) {
		return
	}
	bucket := m.absenceWheel[slot]
	keep := bucket[:0]
	for _, item := range bucket {
		if item.Info.ExpiresAt <= nowMs {
			if trackedInfo, tracked := m.absenceKeys[item.Key]; tracked && trackedInfo.ExpiresAt == item.Info.ExpiresAt {
				expired[item.Key] = trackedInfo
				delete(m.absenceKeys, item.Key)
				delete(m.absenceKeySlot, item.Key)
			}
		} else {
			keep = append(keep, item)
		}
	}
	m.absenceWheel[slot] = keep
}

// TrackAbsenceKey registers a key for absence scanning.
// In Redis mode, stores a separate tracking key so any cluster node can discover it.
// In local cache mode, tracks one canonical wheel entry per key to avoid duplicates.
func (m *CEPStateManager) TrackAbsenceKey(key string, info absenceKeyInfo) {
	m.absenceKeysMu.Lock()
	m.absenceKeys[key] = info
	if m.useLocalCache && m.absenceWheel != nil {
		newSlot := timingWheelSlot(info.ExpiresAt)

		if oldSlot, exists := m.absenceKeySlot[key]; exists {
			if oldSlot == newSlot {
				// Update existing slot entry in place
				bucket := m.absenceWheel[newSlot]
				for i := range bucket {
					if bucket[i].Key == key {
						bucket[i].Info = info
						m.absenceWheel[newSlot] = bucket
						m.absenceKeysMu.Unlock()
						return
					}
				}
			} else {
				// Key moved to a new slot: remove stale old entry first
				m.removeFromWheelSlot(key, oldSlot)
			}
		}

		m.absenceWheel[newSlot] = append(m.absenceWheel[newSlot], absenceSlotItem{Key: key, Info: info})
		m.absenceKeySlot[key] = newSlot
	}
	m.absenceKeysMu.Unlock()

	if !m.useLocalCache {
		m.setRedisAbsenceTracker(key, info)
	}
}

// UntrackAbsenceKey removes a key from absence scanning.
func (m *CEPStateManager) UntrackAbsenceKey(key string) {
	m.absenceKeysMu.Lock()
	delete(m.absenceKeys, key)
	if m.useLocalCache && m.absenceWheel != nil {
		if slot, exists := m.absenceKeySlot[key]; exists {
			m.removeFromWheelSlot(key, slot)
			delete(m.absenceKeySlot, key)
		}
	}
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

// getExpiredAbsenceKeysLocal returns absence keys that have expired (ExpiresAt <= nowMs).
// When the timing wheel is used (local cache), only the current second's bucket is scanned
// instead of the full key set, reducing cost from O(n) to O(bucket size) per tick.
func (m *CEPStateManager) getExpiredAbsenceKeysLocal(nowMs int64) map[string]absenceKeyInfo {
	m.absenceKeysMu.Lock()
	defer m.absenceKeysMu.Unlock()

	expired := make(map[string]absenceKeyInfo)

	if m.absenceWheel != nil {
		// Timing wheel catch-up: process all missed second-slots since last scan.
		nowSec := nowMs / 1000
		if m.lastProcessedSecond < 0 {
			m.lastProcessedSecond = nowSec - 1
		}
		if nowSec < m.lastProcessedSecond {
			// Clock moved backwards: process current slot only and reset baseline.
			m.collectExpiredFromWheelSlot(timingWheelSlot(nowMs), nowMs, expired)
			m.lastProcessedSecond = nowSec
			return expired
		}

		delta := nowSec - m.lastProcessedSecond
		if delta >= int64(TimingWheelSlots) {
			// Long pause/gap: scan all slots once.
			for slot := 0; slot < TimingWheelSlots; slot++ {
				m.collectExpiredFromWheelSlot(slot, nowMs, expired)
			}
		} else {
			for sec := m.lastProcessedSecond + 1; sec <= nowSec; sec++ {
				m.collectExpiredFromWheelSlot(timingWheelSlot(sec*1000), nowMs, expired)
			}
		}
		m.lastProcessedSecond = nowSec
		return expired
	}

	// Fallback: full map scan (e.g. wheel not initialized)
	for key, info := range m.absenceKeys {
		if info.ExpiresAt <= nowMs {
			expired[key] = info
			delete(m.absenceKeys, key)
			if m.absenceKeySlot != nil {
				delete(m.absenceKeySlot, key)
			}
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
		logger.Error("Failed to set absence tracker in Redis", "key", absKey, "error", err)
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
	// Add grace period so the absence scanner can still read the state after
	// the sequence window expires. Without this, ristretto evicts the entry
	// at exactly the same moment the scanner tries to read it.
	cacheTTL := time.Duration(ttlSeconds+localCacheTTLGracePeriod) * time.Second
	m.cache.SetWithTTL(key, state, 1, cacheTTL)
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
	StageMatches map[int][]StageMatch   `json:"sm"`
	Context      map[string]interface{} `json:"ctx,omitempty"`
	CreatedAt    int64                  `json:"ca"`
	ExpiresAt    int64                  `json:"ea"`
}

// getRedisState reads state from Redis, auto-detecting compressed vs plain JSON.
func (m *CEPStateManager) getRedisState(key string) *SequenceState {
	val, err := common.RedisGet(key)
	if err != nil || val == "" {
		return nil
	}

	state, err := decompressState(val)
	if err != nil {
		logger.Error("Failed to deserialize CEP state from Redis", "key", key, "error", err)
		return nil
	}
	return state
}

// setRedisState writes state to Redis as zstd-compressed JSON.
func (m *CEPStateManager) setRedisState(key string, state *SequenceState, ttlSeconds int) {
	data, err := serializeState(state)
	if err != nil {
		logger.Error("Failed to serialize CEP state for Redis", "key", key, "error", err)
		return
	}

	if _, err := common.RedisSet(key, data, ttlSeconds); err != nil {
		logger.Error("Failed to set CEP state in Redis", "key", key, "error", err)
	}
}

func (m *CEPStateManager) deleteRedisState(key string) {
	// Delete main state key
	if err := common.RedisDel(key); err != nil {
		logger.Error("Failed to delete CEP state from Redis", "key", key, "error", err)
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

	// localCacheTTLGracePeriod is extra seconds added to ristretto cache TTL beyond the
	// sequence's "within" duration. This ensures the absence scanner (which fires at
	// ExpiresAt) can still read the state from cache before ristretto evicts it.
	// Without this grace period, the cache TTL and ExpiresAt expire simultaneously,
	// causing a race where the absence scanner finds the state already evicted.
	localCacheTTLGracePeriod = 30
)

// GetOrCreateState atomically gets or creates a SequenceState for the given key.
// IMPORTANT: The caller MUST hold the per-key lock via LockKey before calling this method.
func (m *CEPStateManager) GetOrCreateState(key string, withinMs int64, ttlSeconds int) *SequenceState {
	if m.useLocalCache {
		// Per-key lock is held by caller; no global lock needed for local cache.
		state := m.getLocalState(key)
		if state != nil && time.Now().UnixMilli() > state.ExpiresAt {
			// The sequence window has expired. The ristretto entry still exists
			// due to the grace period (kept longer for the absence scanner), but
			// new events should not match against a stale state. Delete and
			// create a fresh state. The absence scanner uses GetState (not
			// GetOrCreateState), so it will still see the entry if it runs
			// before this point.
			m.deleteLocalState(key)
			state = nil
		}
		if state == nil {
			nowMs := time.Now().UnixMilli()
			state = NewSequenceState(nowMs, nowMs+withinMs)
			m.setLocalState(key, state, ttlSeconds)
		}
		return state
	}

	// Redis: use Lua script for atomic get-or-create
	return m.getOrCreateRedisStateAtomic(key, withinMs, ttlSeconds)
}

// UpdateState atomically updates the state after modifications.
// Applies memory control before persisting.
// IMPORTANT: The caller MUST hold the per-key lock via LockKey before calling this method.
func (m *CEPStateManager) UpdateState(key string, state *SequenceState, ttlSeconds int) {
	// Apply memory control: trim excess stage matches
	trimStageMatches(state)

	if m.useLocalCache {
		// Per-key lock is held by caller; no global lock needed for local cache.
		m.setLocalState(key, state, ttlSeconds)
		return
	}
	m.updateRedisStateAtomic(key, state, ttlSeconds)
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
func (m *CEPStateManager) getOrCreateRedisStateAtomic(key string, withinMs int64, ttlSeconds int) *SequenceState {
	nowMs := time.Now().UnixMilli()
	newState := NewSequenceState(nowMs, nowMs+withinMs)

	serialized, err := serializeState(newState)
	if err != nil {
		logger.Error("Failed to serialize new CEP state", "key", key, "error", err)
		return newState
	}

	result, err := common.RedisEval(luaGetOrCreate, []string{key}, serialized, ttlSeconds)
	if err != nil {
		// Fallback to non-atomic get-then-set
		logger.Warn("Lua get-or-create failed, falling back", "key", key, "error", err)
		state := m.getRedisState(key)
		if state == nil {
			m.setRedisState(key, newState, ttlSeconds)
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
func (m *CEPStateManager) updateRedisStateAtomic(key string, state *SequenceState, ttlSeconds int) {
	serialized, err := serializeState(state)
	if err != nil {
		logger.Error("Failed to serialize CEP state for atomic update", "key", key, "error", err)
		return
	}

	_, err = common.RedisEval(luaAtomicUpdate, []string{key}, serialized, ttlSeconds)
	if err != nil {
		// Fallback to regular SET
		logger.Warn("Lua atomic update failed, falling back", "key", key, "error", err)
		m.setRedisState(key, state, ttlSeconds)
	}
}

// String returns a debug string for the state manager.
func (m *CEPStateManager) String() string {
	if m.useLocalCache {
		return fmt.Sprintf("CEPStateManager{backend=local_cache}")
	}
	return fmt.Sprintf("CEPStateManager{backend=redis}")
}
