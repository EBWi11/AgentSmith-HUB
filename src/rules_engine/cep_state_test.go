package rules_engine

import (
	"testing"
	"time"

	"github.com/dgraph-io/ristretto/v2"
)

// ============================================================================
// CEP State Manager Tests (Local Cache Backend)
// ============================================================================

func newTestLocalCache() *ristretto.Cache[string, *SequenceState] {
	cache, err := ristretto.NewCache(&ristretto.Config[string, *SequenceState]{
		NumCounters: 10_000,
		MaxCost:     1024 * 1024,
		BufferItems: 64,
	})
	if err != nil {
		panic(err)
	}
	return cache
}

func TestCEPStateManager_LocalCache_SetGet(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	state := NewSequenceState(1000, 2000)
	state.AddMatch(0, StageMatch{Timestamp: 1000, Data: map[string]interface{}{"a": "1"}})

	mgr.SetState("test_key", state, 60)
	time.Sleep(10 * time.Millisecond) // Wait for Ristretto async set

	got := mgr.GetState("test_key")
	if got == nil {
		t.Fatal("expected state, got nil")
	}
	if got.CreatedAt != 1000 {
		t.Errorf("expected CreatedAt=1000, got %d", got.CreatedAt)
	}
	if got.ExpiresAt != 2000 {
		t.Errorf("expected ExpiresAt=2000, got %d", got.ExpiresAt)
	}
	matches, exists := got.StageMatches[0]
	if !exists || len(matches) != 1 {
		t.Fatalf("expected 1 stage match at index 0, got %v", got.StageMatches)
	}
	if matches[0].Timestamp != 1000 {
		t.Errorf("expected timestamp 1000, got %d", matches[0].Timestamp)
	}
}

func TestCEPStateManager_LocalCache_GetMissing(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	got := mgr.GetState("nonexistent_key")
	if got != nil {
		t.Errorf("expected nil for missing key, got %+v", got)
	}
}

func TestCEPStateManager_LocalCache_Delete(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	state := NewSequenceState(1000, 2000)
	mgr.SetState("delete_key", state, 60)
	time.Sleep(10 * time.Millisecond)

	mgr.DeleteState("delete_key")
	time.Sleep(10 * time.Millisecond)

	got := mgr.GetState("delete_key")
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

func TestCEPStateManager_LocalCache_GetOrCreate(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	// First call should create
	state1 := mgr.GetOrCreateState("gor_key", 5000, 5)
	if state1 == nil {
		t.Fatal("expected state, got nil")
	}
	if state1.ExpiresAt-state1.CreatedAt != 5000 {
		t.Errorf("expected 5000ms window, got %d", state1.ExpiresAt-state1.CreatedAt)
	}

	// Add a match to distinguish
	state1.AddMatch(0, StageMatch{Timestamp: 100})
	mgr.UpdateState("gor_key", state1, 5)
	time.Sleep(10 * time.Millisecond)

	// Second call should get existing
	state2 := mgr.GetOrCreateState("gor_key", 5000, 5)
	if state2 == nil {
		t.Fatal("expected state on second call, got nil")
	}
	matches, exists := state2.StageMatches[0]
	if !exists || len(matches) != 1 {
		t.Fatalf("expected 1 match from first state, got %v", state2.StageMatches)
	}
}

// ============================================================================
// Absence Key Tracking Tests
// ============================================================================

func TestCEPStateManager_AbsenceKeyTracking(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	mgr.TrackAbsenceKey("key1", absenceKeyInfo{ExpiresAt: 1000, RuleID: "r1", SeqID: 1})
	mgr.TrackAbsenceKey("key2", absenceKeyInfo{ExpiresAt: 2000, RuleID: "r2", SeqID: 2})
	mgr.TrackAbsenceKey("key3", absenceKeyInfo{ExpiresAt: 3000, RuleID: "r3", SeqID: 3})

	// At time 1500, only key1 should be expired
	expired := mgr.GetExpiredAbsenceKeys(1500)
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired key at t=1500, got %d", len(expired))
	}
	if _, ok := expired["key1"]; !ok {
		t.Error("expected key1 to be expired")
	}

	// At time 2500, key2 should be expired (key1 already removed)
	expired = mgr.GetExpiredAbsenceKeys(2500)
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired key at t=2500, got %d", len(expired))
	}
	if _, ok := expired["key2"]; !ok {
		t.Error("expected key2 to be expired")
	}

	// Untrack key3
	mgr.UntrackAbsenceKey("key3")
	expired = mgr.GetExpiredAbsenceKeys(5000)
	if len(expired) != 0 {
		t.Errorf("expected 0 expired keys after untrack, got %d", len(expired))
	}
}

func TestCEPStateManager_AbsenceWheel_KeyReuseNoStaleTrigger(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	// First lifecycle for the same key
	mgr.TrackAbsenceKey("reused_key", absenceKeyInfo{ExpiresAt: 10000, RuleID: "r1", SeqID: 1})
	mgr.UntrackAbsenceKey("reused_key")

	// Reuse the same key for a new lifecycle with a different expiry
	mgr.TrackAbsenceKey("reused_key", absenceKeyInfo{ExpiresAt: 20000, RuleID: "r2", SeqID: 2})

	// At old expiry slot/time, stale wheel entry must NOT trigger the new state
	expired := mgr.GetExpiredAbsenceKeys(10000)
	if len(expired) != 0 {
		t.Fatalf("expected no expired keys at old expiry, got %d", len(expired))
	}

	// At new expiry, key should expire exactly once with latest metadata
	expired = mgr.GetExpiredAbsenceKeys(20000)
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired key at new expiry, got %d", len(expired))
	}
	info, ok := expired["reused_key"]
	if !ok {
		t.Fatal("expected reused_key to be expired at new expiry")
	}
	if info.RuleID != "r2" || info.SeqID != 2 {
		t.Fatalf("expected latest absence info (r2,2), got (%s,%d)", info.RuleID, info.SeqID)
	}
}

func TestCEPStateManager_AbsenceWheel_ReTrackUpdatesEntry(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	// Re-track same key in the same slot with updated metadata.
	mgr.TrackAbsenceKey("k", absenceKeyInfo{ExpiresAt: 30000, RuleID: "r1", SeqID: 1})
	mgr.TrackAbsenceKey("k", absenceKeyInfo{ExpiresAt: 30000, RuleID: "r1", SeqID: 99})

	expired := mgr.GetExpiredAbsenceKeys(30000)
	if len(expired) != 1 {
		t.Fatalf("expected exactly one expired entry, got %d", len(expired))
	}
	info := expired["k"]
	if info.SeqID != 99 {
		t.Fatalf("expected updated SeqID=99, got %d", info.SeqID)
	}
}

func TestCEPStateManager_AbsenceWheel_CatchUpMissedSlots(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	mgr.TrackAbsenceKey("k", absenceKeyInfo{ExpiresAt: 2000, RuleID: "r1", SeqID: 1})

	// First scan at t=1000, key is not expired yet.
	expired := mgr.GetExpiredAbsenceKeys(1000)
	if len(expired) != 0 {
		t.Fatalf("expected 0 expired keys at t=1000, got %d", len(expired))
	}

	// Jump directly to t=4000. Catch-up should process missed slots and expire key.
	expired = mgr.GetExpiredAbsenceKeys(4000)
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired key at t=4000 after slot catch-up, got %d", len(expired))
	}
	if _, ok := expired["k"]; !ok {
		t.Fatal("expected key k to expire during catch-up scan")
	}

	// Ensure key is removed and not emitted again.
	expired = mgr.GetExpiredAbsenceKeys(5000)
	if len(expired) != 0 {
		t.Fatalf("expected no duplicate expiry at t=5000, got %d", len(expired))
	}
}

// ============================================================================
// BuildStateKey Tests
// ============================================================================

func TestBuildStateKey(t *testing.T) {
	key1 := BuildStateKey("ruleset1rule1", "192.168.1.1|")
	key2 := BuildStateKey("ruleset1rule1", "192.168.1.2|")
	key3 := BuildStateKey("ruleset1rule1", "192.168.1.1|")

	// Same input should produce same key
	if key1 != key3 {
		t.Errorf("expected same keys for same input, got %s vs %s", key1, key3)
	}
	// Different input should produce different key
	if key1 == key2 {
		t.Errorf("expected different keys for different input, got same: %s", key1)
	}
	// Key should have the prefix
	if key1[:4] != CEPStateKeyPrefix {
		t.Errorf("expected key to start with %s, got %s", CEPStateKeyPrefix, key1[:4])
	}
}

// ============================================================================
// Memory Control Tests
// ============================================================================

func TestTrimStageMatches(t *testing.T) {
	state := NewSequenceState(1000, 2000)

	// Add more than MaxStageMatchesPerStage matches
	for i := 0; i < MaxStageMatchesPerStage+50; i++ {
		state.AddMatch(0, StageMatch{Timestamp: int64(i + 1)})
	}

	if len(state.StageMatches[0]) != MaxStageMatchesPerStage+50 {
		t.Fatalf("expected %d matches before trim, got %d", MaxStageMatchesPerStage+50, len(state.StageMatches[0]))
	}

	trimStageMatches(state)

	if len(state.StageMatches[0]) != MaxStageMatchesPerStage {
		t.Fatalf("expected %d matches after trim, got %d", MaxStageMatchesPerStage, len(state.StageMatches[0]))
	}

	// Verify that the most recent matches are kept (highest timestamps)
	firstMatch := state.StageMatches[0][0]
	if firstMatch.Timestamp != 51 {
		t.Errorf("expected first match timestamp=51 (kept recent), got %d", firstMatch.Timestamp)
	}
	lastMatch := state.StageMatches[0][MaxStageMatchesPerStage-1]
	if lastMatch.Timestamp != int64(MaxStageMatchesPerStage+50) {
		t.Errorf("expected last match timestamp=%d, got %d", MaxStageMatchesPerStage+50, lastMatch.Timestamp)
	}
}

func TestTrimStageMatches_NoTrimNeeded(t *testing.T) {
	state := NewSequenceState(1000, 2000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(0, StageMatch{Timestamp: 200})

	trimStageMatches(state)

	if len(state.StageMatches[0]) != 2 {
		t.Errorf("expected 2 matches (no trim needed), got %d", len(state.StageMatches[0]))
	}
}

// ============================================================================
// Compression Tests
// ============================================================================

func TestSerializeState_CompressedAndBackwardCompatibleDecode(t *testing.T) {
	state := NewSequenceState(1000, 2000)
	state.AddMatch(0, StageMatch{Timestamp: 100, Data: map[string]interface{}{"field": "value", "ip": "1.2.3.4"}})

	data, err := serializeState(state)
	if err != nil {
		t.Fatalf("serializeState failed: %v", err)
	}

	// Compressed data should start with zstd magic (0x28 0xB5 0x2F 0xFD)
	if len(data) < 4 || data[0] != 0x28 || data[1] != 0xB5 {
		t.Errorf("expected zstd magic header, got 0x%x 0x%x", data[0], data[1])
	}

	// Deserialize should work for compressed data
	restored, err := decompressState(data)
	if err != nil {
		t.Fatalf("decompressState failed: %v", err)
	}
	if restored.CreatedAt != 1000 {
		t.Errorf("expected CreatedAt=1000, got %d", restored.CreatedAt)
	}
	matches := restored.StageMatches[0]
	if len(matches) != 1 || matches[0].Data["field"] != "value" {
		t.Errorf("unexpected restored data: %v", matches)
	}

	// Backward compatibility: plain JSON should still decode.
	legacyPlain := `{"sm":{"0":[{"Timestamp":100,"Data":{"field":"legacy"}}]},"ca":1000,"ea":2000}`
	legacyRestored, err := decompressState(legacyPlain)
	if err != nil {
		t.Fatalf("decompressState legacy plain json failed: %v", err)
	}
	if legacyRestored.CreatedAt != 1000 || legacyRestored.ExpiresAt != 2000 {
		t.Errorf("unexpected legacy restored state: %+v", legacyRestored)
	}
}

// ============================================================================
// Timestamp Parsing Tests
// ============================================================================

func TestParseTimestampToNs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"unix seconds", "1700000000", 1700000000 * int64(time.Second)},
		{"unix milliseconds", "1700000000000", 1700000000000 * int64(time.Millisecond)},
		{"unix microseconds", "1700000000000000", 1700000000000000 * int64(time.Microsecond)},
		{"float seconds", "1700000000.123", int64(1700000000.123 * float64(time.Second))},
		{"RFC3339", "2023-11-14T22:13:20Z", 1700000000 * int64(time.Second)},
		{"ISO 8601 with T", "2023-11-14T22:13:20", 1700000000 * int64(time.Second)},
		{"ISO 8601 space", "2023-11-14 22:13:20", 1700000000 * int64(time.Second)},
		{"invalid", "not_a_timestamp", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimestampToNs(tt.input)
			if tt.expected == 0 {
				if result != 0 {
					t.Errorf("expected 0 for invalid input, got %d", result)
				}
				return
			}
			// Allow 1 second tolerance for time format parsing
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > int64(time.Second) {
				t.Errorf("expected ~%d, got %d (diff=%d)", tt.expected, result, diff)
			}
		})
	}
}
