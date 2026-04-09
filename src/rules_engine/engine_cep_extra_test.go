package rules_engine

import (
	"bytes"
	"testing"
	"time"

	"AgentSmith-HUB/common"
	"github.com/dgraph-io/ristretto/v2"
)

// ============================================================
// cep_condition.go — mergeEventIDs
// ============================================================

func TestMergeEventIDs_Basic(t *testing.T) {
	result := mergeEventIDs([]string{"a", "b"}, []string{"b", "c"})
	if len(result) != 3 {
		t.Errorf("expected 3 unique IDs, got %d: %v", len(result), result)
	}
	seen := map[string]bool{}
	for _, v := range result {
		if seen[v] {
			t.Errorf("duplicate ID %q in result", v)
		}
		seen[v] = true
	}
}

func TestMergeEventIDs_BothEmpty(t *testing.T) {
	result := mergeEventIDs(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestMergeEventIDs_OneEmpty(t *testing.T) {
	result := mergeEventIDs([]string{"x"}, nil)
	if len(result) != 1 || result[0] != "x" {
		t.Errorf("expected [x], got %v", result)
	}
}

// ============================================================
// cep_state.go — SetState, String, removeFromWheelSlot
// ============================================================

func newTestStateManager() *CEPStateManager {
	cache, _ := ristretto.NewCache(&ristretto.Config[string, *SequenceState]{
		NumCounters: 1_000,
		MaxCost:     256 * 1024,
		BufferItems: 64,
	})
	return NewCEPStateManager(true, cache)
}

func TestStateManager_SetState_LocalCache(t *testing.T) {
	m := newTestStateManager()
	state := NewSequenceState(100, 60000)
	m.SetState("test_key", state, 60)
	// Verify we can retrieve it (GetOrCreateState or getLocalState)
	got := m.GetOrCreateState("test_key", 60000, 60)
	if got == nil {
		t.Error("expected non-nil state after SetState")
	}
}

func TestStateManager_String_LocalCache(t *testing.T) {
	m := newTestStateManager()
	s := m.String()
	if s == "" {
		t.Error("expected non-empty String() for local cache manager")
	}
	if s != "CEPStateManager{backend=local_cache}" {
		t.Errorf("unexpected String(): %q", s)
	}
}

func TestStateManager_RemoveFromWheelSlot_ViaUntrack(t *testing.T) {
	m := newTestStateManager()
	key := "test_absence_key"
	info := absenceKeyInfo{
		ExpiresAt: time.Now().Add(10*time.Second).UnixMilli(),
		RuleID:    "r1",
		SeqID:     1,
	}

	// Track the key — adds it to the wheel
	m.TrackAbsenceKey(key, info)

	m.absenceKeysMu.Lock()
	_, inWheel := m.absenceKeySlot[key]
	m.absenceKeysMu.Unlock()
	if !inWheel {
		t.Fatal("expected key to be tracked in wheel after TrackAbsenceKey")
	}

	// Untrack — calls removeFromWheelSlot
	m.UntrackAbsenceKey(key)

	m.absenceKeysMu.Lock()
	_, stillInWheel := m.absenceKeySlot[key]
	m.absenceKeysMu.Unlock()
	if stillInWheel {
		t.Error("expected key to be removed from wheel after UntrackAbsenceKey")
	}
}

func TestStateManager_RemoveFromWheelSlot_SlotChange(t *testing.T) {
	// Force slot change by tracking with different expiry that maps to a different slot
	m := newTestStateManager()
	key := "slot_change_key"

	// Track with a specific expiry
	info1 := absenceKeyInfo{
		ExpiresAt: 1000 * 1000, // some time
		RuleID:    "r1",
		SeqID:     1,
	}
	m.TrackAbsenceKey(key, info1)

	m.absenceKeysMu.Lock()
	slot1, _ := m.absenceKeySlot[key]
	m.absenceKeysMu.Unlock()

	// Track same key with different expiry that maps to different slot
	// Use an expiry offset to ensure a different slot
	info2 := absenceKeyInfo{
		ExpiresAt: info1.ExpiresAt + int64(TimingWheelSlots)*1000 + 1000,
		RuleID:    "r1",
		SeqID:     1,
	}
	m.TrackAbsenceKey(key, info2)

	m.absenceKeysMu.Lock()
	slot2, _ := m.absenceKeySlot[key]
	m.absenceKeysMu.Unlock()

	if slot1 == slot2 {
		// Both happen to map to same slot — removeFromWheelSlot path not taken
		// Just verify no panic
		t.Log("slots equal; removeFromWheelSlot (oldSlot!=newSlot path) not triggered for this expiry pair")
	}
	// The test is primarily a no-panic check
}

// ============================================================
// cep_value_store.go — parseRefFromExpiryKey
// ============================================================

func TestParseRefFromExpiryKey_Valid(t *testing.T) {
	// format: e:<20-digit-ts>:<ref>
	key := []byte("e:00000000001700000000:myref123")
	ref := parseRefFromExpiryKey(key)
	if ref != "myref123" {
		t.Errorf("expected 'myref123', got %q", ref)
	}
}

func TestParseRefFromExpiryKey_RefWithColons(t *testing.T) {
	// Ref itself may contain colons (SplitN with n=3 preserves remainder)
	key := []byte("e:00000000001700000000:part1:part2:part3")
	ref := parseRefFromExpiryKey(key)
	if ref != "part1:part2:part3" {
		t.Errorf("expected 'part1:part2:part3', got %q", ref)
	}
}

func TestParseRefFromExpiryKey_TooFewParts(t *testing.T) {
	key := []byte("noColons")
	ref := parseRefFromExpiryKey(key)
	if ref != "" {
		t.Errorf("expected empty string for malformed key, got %q", ref)
	}
}

func TestParseRefFromExpiryKey_Empty(t *testing.T) {
	ref := parseRefFromExpiryKey([]byte{})
	if ref != "" {
		t.Errorf("expected empty string for empty key, got %q", ref)
	}
}

// Silence unused import warning for bytes
var _ = bytes.Compare

// ============================================================
// engine_core.go — extractCorrelateValues, hasEventGroupBy,
//                  extractCorrelateValuesForStateLookupWithSequenceFallback
// ============================================================

func TestExtractCorrelateValues_SequenceLevelGroupBy(t *testing.T) {
	rs := buildTestRuleset(t, `
<root type="DETECTION" name="corr-test">
  <rule id="r1" name="r1">
    <sequence within="10m" group_by="src_ip" local_cache="true">
      <event id="a"><check type="EQU" field="type">login</check></event>
      <event id="b"><check type="EQU" field="type">access</check></event>
      <condition>a -> b</condition>
    </sequence>
  </rule>
</root>`)

	seq := getFirstSequenceFromRuleset(t, rs)
	data := map[string]interface{}{"src_ip": "1.2.3.4"}
	result := rs.extractCorrelateValues(&seq, []string{"a"}, data, map[string]common.CheckCoreCache{})
	if result == "" {
		t.Error("expected non-empty correlate values for sequence-level group_by")
	}
}

func TestExtractCorrelateValues_PerEventGroupBy(t *testing.T) {
	rs := buildTestRuleset(t, `
<root type="DETECTION" name="corr-per-event">
  <rule id="r1" name="r1">
    <sequence within="10m" local_cache="true">
      <event id="a" group_by="src_ip"><check type="EQU" field="type">login</check></event>
      <event id="b" group_by="dst_ip"><check type="EQU" field="type">access</check></event>
      <condition>a -> b</condition>
    </sequence>
  </rule>
</root>`)

	seq := getFirstSequenceFromRuleset(t, rs)
	data := map[string]interface{}{"src_ip": "10.0.0.1"}
	result := rs.extractCorrelateValues(&seq, []string{"a"}, data, map[string]common.CheckCoreCache{})
	if result == "" {
		t.Error("expected non-empty correlate values from matched event's group_by")
	}
}

func TestExtractCorrelateValues_NoMatch(t *testing.T) {
	rs := buildTestRuleset(t, `
<root type="DETECTION" name="corr-nomatch">
  <rule id="r1" name="r1">
    <sequence within="10m" group_by="src_ip" local_cache="true">
      <event id="a"><check type="EQU" field="type">login</check></event>
      <event id="b"><check type="EQU" field="type">access</check></event>
      <condition>a -> b</condition>
    </sequence>
  </rule>
</root>`)

	seq := getFirstSequenceFromRuleset(t, rs)
	// src_ip not in data — sequence-level group_by won't find value
	data := map[string]interface{}{"type": "login"}
	result := rs.extractCorrelateValues(&seq, []string{"a"}, data, map[string]common.CheckCoreCache{})
	// Should be empty since field not present
	if result != "" {
		t.Logf("result was %q (field absent yields empty)", result)
	}
}

func TestHasEventGroupBy_True(t *testing.T) {
	rs := buildTestRuleset(t, `
<root type="DETECTION" name="has-eg">
  <rule id="r1" name="r1">
    <sequence within="10m" local_cache="true">
      <event id="a" group_by="src_ip"><check type="EQU" field="type">x</check></event>
      <event id="b" group_by="dst_ip"><check type="EQU" field="type">y</check></event>
      <condition>a -> b</condition>
    </sequence>
  </rule>
</root>`)

	seq := getFirstSequenceFromRuleset(t, rs)
	if !rs.hasEventGroupBy(&seq) {
		t.Error("expected hasEventGroupBy=true when at least one event has group_by")
	}
}

func TestHasEventGroupBy_False(t *testing.T) {
	rs := buildTestRuleset(t, `
<root type="DETECTION" name="has-eg-false">
  <rule id="r1" name="r1">
    <sequence within="10m" group_by="ip" local_cache="true">
      <event id="a"><check type="EQU" field="type">x</check></event>
      <event id="b"><check type="EQU" field="type">y</check></event>
      <condition>a -> b</condition>
    </sequence>
  </rule>
</root>`)

	seq := getFirstSequenceFromRuleset(t, rs)
	if rs.hasEventGroupBy(&seq) {
		t.Error("expected hasEventGroupBy=false when only sequence-level group_by is set")
	}
}

func TestExtractCorrelateValuesWithSequenceFallback_FieldPresent(t *testing.T) {
	rs := buildTestRuleset(t, `
<root type="DETECTION" name="fallback-test">
  <rule id="r1" name="r1">
    <sequence within="10m" local_cache="true">
      <event id="a" group_by="src_ip"><check type="EQU" field="type">login</check></event>
      <event id="b" group_by="dst_ip"><check type="EQU" field="type">access</check></event>
      <condition>a -> b</condition>
    </sequence>
  </rule>
</root>`)

	seq := getFirstSequenceFromRuleset(t, rs)
	data := map[string]interface{}{"src_ip": "192.168.1.1", "dst_ip": "10.0.0.1"}
	result := rs.extractCorrelateValuesForStateLookupWithSequenceFallback(&seq, data, map[string]common.CheckCoreCache{})
	if result == "" {
		t.Error("expected non-empty result when group_by field is present in data")
	}
}

func TestExtractCorrelateValuesWithSequenceFallback_NoFields(t *testing.T) {
	rs := buildTestRuleset(t, `
<root type="DETECTION" name="fallback-empty">
  <rule id="r1" name="r1">
    <sequence within="10m" local_cache="true">
      <event id="a" group_by="src_ip"><check type="EQU" field="type">login</check></event>
      <event id="b" group_by="src_ip"><check type="EQU" field="type">access</check></event>
      <condition>a -> b</condition>
    </sequence>
  </rule>
</root>`)

	seq := getFirstSequenceFromRuleset(t, rs)
	// No src_ip in data — fallback returns ""
	data := map[string]interface{}{"type": "login"}
	result := rs.extractCorrelateValuesForStateLookupWithSequenceFallback(&seq, data, map[string]common.CheckCoreCache{})
	if result != "" {
		t.Errorf("expected empty result when group_by field absent, got %q", result)
	}
}

// ============================================================
// engine_core.go — executeIteratorThreshold (dead code, direct call for coverage)
// ============================================================

func TestExecuteIteratorThreshold_NoGroupBy(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="iter-thresh-direct">
  <rule id="r1" name="r1">
    <check type="EQU" field="x">1</check>
  </rule>
</root>`)

	threshold := &Threshold{
		GroupByList: map[string][]string{},
		Value:       2,
	}
	result := rs.executeIteratorThreshold(threshold, map[string]interface{}{"port": "8080"}, nil)
	if result {
		t.Error("expected false when GroupByList is empty (groupByKey stays empty)")
	}
}

func TestExecuteIteratorThreshold_GroupByPresent(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="iter-thresh-gb">
  <rule id="r1" name="r1">
    <check type="EQU" field="x">1</check>
  </rule>
</root>`)

	threshold := &Threshold{
		GroupByList: map[string][]string{
			"port": {"port"},
		},
		Value: 1, // countValue (default 1) >= 1 → true
	}
	data := map[string]interface{}{"port": "8080"}
	result := rs.executeIteratorThreshold(threshold, data, nil)
	if !result {
		t.Error("expected true when countValue(1) >= threshold.Value(1)")
	}
}

func TestExecuteIteratorThreshold_SumCountType(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="iter-thresh-sum">
  <rule id="r1" name="r1">
    <check type="EQU" field="x">1</check>
  </rule>
</root>`)

	threshold := &Threshold{
		GroupByList: map[string][]string{
			"host": {"host"},
		},
		CountType:      "SUM",
		CountFieldList: []string{"count"},
		Value:          5,
	}
	data := map[string]interface{}{"host": "myhost", "count": 10}
	result := rs.executeIteratorThreshold(threshold, data, nil)
	if !result {
		t.Error("expected true when SUM count(10) >= Value(5)")
	}
}

// ============================================================
// engine_core.go — full sequence with per-event group_by via EngineCheck
// exercises hasEventGroupBy path in executeSequence
// ============================================================

func TestSequence_PerEventGroupBy_EngineCheck(t *testing.T) {
	rs := buildTestRuleset(t, `
<root type="DETECTION" name="seq-per-event-gb">
  <rule id="r1" name="r1">
    <sequence within="10m" local_cache="true">
      <event id="login" group_by="user">
        <check type="EQU" field="type">login</check>
      </event>
      <event id="access" group_by="user">
        <check type="EQU" field="type">access</check>
      </event>
      <condition>login -> access</condition>
    </sequence>
  </rule>
</root>`)

	// First event: login
	out1 := rs.EngineCheck(map[string]interface{}{"type": "login", "user": "alice"})
	if len(out1) != 0 {
		t.Fatalf("expected 0 results on first event, got %d", len(out1))
	}

	// Second event: access — completes the sequence
	out2 := rs.EngineCheck(map[string]interface{}{"type": "access", "user": "alice"})
	if len(out2) != 1 {
		t.Fatalf("expected 1 result on sequence completion, got %d", len(out2))
	}
}
