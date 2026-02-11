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

	mgr.SetState("test_key", state, 60, false)
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
	mgr.SetState("delete_key", state, 60, false)
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
	state1 := mgr.GetOrCreateState("gor_key", 5000, 5, false)
	if state1 == nil {
		t.Fatal("expected state, got nil")
	}
	if state1.ExpiresAt-state1.CreatedAt != 5000 {
		t.Errorf("expected 5000ms window, got %d", state1.ExpiresAt-state1.CreatedAt)
	}

	// Add a match to distinguish
	state1.AddMatch(0, StageMatch{Timestamp: 100})
	mgr.UpdateState("gor_key", state1, 5, false)
	time.Sleep(10 * time.Millisecond)

	// Second call should get existing
	state2 := mgr.GetOrCreateState("gor_key", 5000, 5, false)
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

func TestSerializeState_Uncompressed(t *testing.T) {
	state := NewSequenceState(1000, 2000)
	state.AddMatch(0, StageMatch{Timestamp: 100, Data: map[string]interface{}{"field": "value", "ip": "1.2.3.4"}})

	data, err := serializeState(state, false)
	if err != nil {
		t.Fatalf("serializeState failed: %v", err)
	}

	// Should be plain JSON (starts with '{')
	if data[0] != '{' {
		t.Errorf("expected plain JSON, got byte 0x%x", data[0])
	}

	// Deserialize should work
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
}

func TestSerializeState_Compressed(t *testing.T) {
	state := NewSequenceState(1000, 2000)
	// Add some data to compress
	state.AddMatch(0, StageMatch{Timestamp: 100, Data: map[string]interface{}{
		"event_type": "login", "source_ip": "192.168.1.100", "user": "admin",
		"hostname": "server-01", "port": "22", "protocol": "ssh",
	}})
	state.AddMatch(1, StageMatch{Timestamp: 200, Data: map[string]interface{}{
		"event_type": "file_transfer", "source_ip": "192.168.1.100", "user": "admin",
		"dest": "evil.com", "size": "1048576", "direction": "outbound",
	}})

	compressed, err := serializeState(state, true)
	if err != nil {
		t.Fatalf("serializeState compressed failed: %v", err)
	}

	uncompressed, err := serializeState(state, false)
	if err != nil {
		t.Fatalf("serializeState uncompressed failed: %v", err)
	}

	// Compressed data should start with zstd magic (0x28 0xB5 0x2F 0xFD)
	if len(compressed) < 4 || compressed[0] != 0x28 || compressed[1] != 0xB5 {
		t.Errorf("expected zstd magic header, got 0x%x 0x%x", compressed[0], compressed[1])
	}

	t.Logf("Uncompressed size: %d bytes, Compressed size: %d bytes, Ratio: %.1f%%",
		len(uncompressed), len(compressed), float64(len(compressed))/float64(len(uncompressed))*100)

	// Decompress should restore original data
	restored, err := decompressState(compressed)
	if err != nil {
		t.Fatalf("decompressState failed: %v", err)
	}
	if restored.CreatedAt != 1000 {
		t.Errorf("expected CreatedAt=1000, got %d", restored.CreatedAt)
	}
	if len(restored.StageMatches) != 2 {
		t.Errorf("expected 2 stages, got %d", len(restored.StageMatches))
	}
	if restored.StageMatches[0][0].Data["source_ip"] != "192.168.1.100" {
		t.Errorf("unexpected restored stage 0 data")
	}
	if restored.StageMatches[1][0].Data["dest"] != "evil.com" {
		t.Errorf("unexpected restored stage 1 data")
	}
}

// ============================================================================
// Timestamp Parsing Tests
// ============================================================================

func TestParseTimestampToMs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"unix seconds", "1700000000", 1700000000000},
		{"unix milliseconds", "1700000000000", 1700000000000},
		{"unix microseconds", "1700000000000000", 1700000000000},
		{"float seconds", "1700000000.123", 1700000000123},
		{"RFC3339", "2023-11-14T22:13:20Z", 1700000000000},
		{"ISO 8601 with T", "2023-11-14T22:13:20", 1700000000000},
		{"ISO 8601 space", "2023-11-14 22:13:20", 1700000000000},
		{"invalid", "not_a_timestamp", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimestampToMs(tt.input)
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
			if diff > 1000 {
				t.Errorf("expected ~%d, got %d (diff=%d)", tt.expected, result, diff)
			}
		})
	}
}
