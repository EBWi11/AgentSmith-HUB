package rules_engine

// Tests for threshold rule behavior (count, SUM, CLASSIFY) via the full
// EngineCheck pipeline: XML → ParseRuleset → RulesetBuild → EngineCheck.
//
// All tests use local_cache="true" to exercise the in-process threshold cache.
//
// Design notes:
//   - LocalCacheFRQSum always seeds on the first call and returns false.
//     Threshold fires on a subsequent call when accumulated > threshold.
//   - CLASSIFY fires when distinct count > threshold (strictly greater).
//   - Count threshold fires when event count > threshold (strictly greater).
//   - Composite group_by (multi-field) is not tested here because Go's
//     map iteration order is non-deterministic, making the composite key
//     non-stable across calls in the same process. Single-field group_by
//     is used throughout.

import (
	"fmt"
	"sync"
	"testing"
)

// buildThresholdRuleset wraps buildRulesetFromXML and registers cache cleanup.
func buildThresholdRuleset(tb testing.TB, xml string) *Ruleset {
	tb.Helper()
	rs := buildRulesetFromXML(tb, xml)
	tb.Cleanup(func() { rs.cleanup() })
	return rs
}

// ---------------------------------------------------------------------------
// Default count threshold
// ---------------------------------------------------------------------------

func TestThreshold_Count_NoFireAtThreshold(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="count-at">
    <check type="EQU" field="type">login</check>
    <threshold group_by="uid" range="60s" local_cache="true">3</threshold>
  </rule>
</root>`)
	data := map[string]interface{}{"type": "login", "uid": "alice"}
	// Exactly threshold (3) events — count never exceeds 3, so no fire
	for i := 1; i <= 3; i++ {
		if out := rs.EngineCheck(data); len(out) != 0 {
			t.Fatalf("event %d: should not fire when count == threshold, got %d", i, len(out))
		}
	}
}

func TestThreshold_Count_FiresOnExceed(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="count-fire">
    <check type="EQU" field="type">login</check>
    <threshold group_by="uid" range="60s" local_cache="true">3</threshold>
  </rule>
</root>`)
	data := map[string]interface{}{"type": "login", "uid": "bob"}
	// First 3 events: count <= threshold, no fire
	for i := 0; i < 3; i++ {
		rs.EngineCheck(data)
	}
	// 4th event: count(4) > threshold(3) → fire
	if out := rs.EngineCheck(data); len(out) != 1 {
		t.Fatalf("4th event: expected fire (count > threshold), got %d", len(out))
	}
}

func TestThreshold_Count_ResetsAfterFire(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="count-reset">
    <threshold group_by="uid" range="60s" local_cache="true">2</threshold>
  </rule>
</root>`)
	data := map[string]interface{}{"uid": "carol"}

	// 3 events trigger fire on the 3rd (3 > 2)
	rs.EngineCheck(data)
	rs.EngineCheck(data)
	if out := rs.EngineCheck(data); len(out) != 1 {
		t.Fatalf("3rd event should fire (count > 2), got %d", len(out))
	}

	// Counter deleted on fire; next event seeds fresh → no fire
	if out := rs.EngineCheck(data); len(out) != 0 {
		t.Fatalf("first event after fire should not fire (counter reset), got %d", len(out))
	}
}

func TestThreshold_Count_GroupBySeparatesKeys(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="count-grp">
    <threshold group_by="uid" range="60s" local_cache="true">2</threshold>
  </rule>
</root>`)
	alice := map[string]interface{}{"uid": "alice"}
	bob := map[string]interface{}{"uid": "bob"}

	// Alice: 2 events (no fire)
	rs.EngineCheck(alice)
	rs.EngineCheck(alice)

	// Bob's first event must not be affected by Alice's counter
	if out := rs.EngineCheck(bob); len(out) != 0 {
		t.Fatalf("bob's 1st event should not fire (isolated counter), got %d", len(out))
	}

	// Alice's 3rd event → fire
	if out := rs.EngineCheck(alice); len(out) != 1 {
		t.Fatalf("alice's 3rd event should fire, got %d", len(out))
	}
}

func TestThreshold_Count_CheckPlusThreshold(t *testing.T) {
	// Check + threshold in sequence: the check acts as a pre-filter before counting.
	// Only events matching event_code=1 are counted.
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="check-threshold">
    <check type="EQU" field="event_code">1</check>
    <threshold group_by="uid" range="60s" local_cache="true">2</threshold>
  </rule>
</root>`)

	// event_code=2 events should not increment the counter (rule fails at check)
	for i := 0; i < 5; i++ {
		rs.EngineCheck(map[string]interface{}{"event_code": "2", "uid": "u1"})
	}

	// event_code=1: 3 events → fire on 3rd
	rs.EngineCheck(map[string]interface{}{"event_code": "1", "uid": "u1"})
	rs.EngineCheck(map[string]interface{}{"event_code": "1", "uid": "u1"})
	if out := rs.EngineCheck(map[string]interface{}{"event_code": "1", "uid": "u1"}); len(out) != 1 {
		t.Fatalf("3rd matching event should fire, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// SUM threshold (count_type="SUM")
// ---------------------------------------------------------------------------

func TestThreshold_SUM_FiresWhenSumExceeds(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="sum-fire">
    <check type="EQU" field="type">transfer</check>
    <threshold group_by="uid" range="60s" count_type="SUM" count_field="amount" local_cache="true">100</threshold>
  </rule>
</root>`)

	mk := func(uid string, amount int) map[string]interface{} {
		return map[string]interface{}{"type": "transfer", "uid": uid, "amount": fmt.Sprintf("%d", amount)}
	}

	// First call: seeds cache with 60, returns false (first call always false)
	if out := rs.EngineCheck(mk("u1", 60)); len(out) != 0 {
		t.Fatalf("first event (sum=60): should not fire, got %d", len(out))
	}
	// Second call: 60+60=120 > 100 → fire
	if out := rs.EngineCheck(mk("u1", 60)); len(out) != 1 {
		t.Fatalf("second event (sum=120): should fire, got %d", len(out))
	}
}

func TestThreshold_SUM_NoFireWhenSumEquals(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="sum-eq">
    <threshold group_by="uid" range="60s" count_type="SUM" count_field="bytes" local_cache="true">100</threshold>
  </rule>
</root>`)

	mk := func(b int) map[string]interface{} {
		return map[string]interface{}{"uid": "u1", "bytes": fmt.Sprintf("%d", b)}
	}

	// 50 + 50 = 100 — not > 100 → no fire
	rs.EngineCheck(mk(50))
	if out := rs.EngineCheck(mk(50)); len(out) != 0 {
		t.Fatalf("sum == threshold should not fire, got %d", len(out))
	}
}

func TestThreshold_SUM_AccumulatesAcrossEvents(t *testing.T) {
	// Verify accumulation: 3 events of 40 bytes; 40+40=80 (no fire), 80+40=120>100 (fire)
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="sum-accum">
    <threshold group_by="uid" range="60s" count_type="SUM" count_field="bytes" local_cache="true">100</threshold>
  </rule>
</root>`)

	mk := func(b int) map[string]interface{} {
		return map[string]interface{}{"uid": "u1", "bytes": fmt.Sprintf("%d", b)}
	}

	rs.EngineCheck(mk(40)) // seeds 40
	if out := rs.EngineCheck(mk(40)); len(out) != 0 { // 40+40=80 ≤ 100
		t.Fatalf("2nd event (sum=80): should not fire, got %d", len(out))
	}
	if out := rs.EngineCheck(mk(40)); len(out) != 1 { // 80+40=120 > 100
		t.Fatalf("3rd event (sum=120): should fire, got %d", len(out))
	}
}

func TestThreshold_SUM_CountFieldMissingNoFire(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="sum-miss">
    <threshold group_by="uid" range="60s" count_type="SUM" count_field="amount" local_cache="true">10</threshold>
  </rule>
</root>`)

	// count_field absent → rule skips without error or fire
	if out := rs.EngineCheck(map[string]interface{}{"uid": "u1"}); len(out) != 0 {
		t.Fatalf("missing count_field should not fire, got %d", len(out))
	}
}

func TestThreshold_SUM_GroupBySeparatesUsers(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="sum-grp">
    <threshold group_by="uid" range="60s" count_type="SUM" count_field="amount" local_cache="true">100</threshold>
  </rule>
</root>`)

	mk := func(uid string, amount int) map[string]interface{} {
		return map[string]interface{}{"uid": uid, "amount": fmt.Sprintf("%d", amount)}
	}

	// u1: fires at 60+60=120
	rs.EngineCheck(mk("u1", 60))
	if out := rs.EngineCheck(mk("u1", 60)); len(out) != 1 {
		t.Fatalf("u1 should fire at sum=120, got %d", len(out))
	}

	// u2: first event (sum=60) — isolated counter, no fire
	if out := rs.EngineCheck(mk("u2", 60)); len(out) != 0 {
		t.Fatalf("u2 should not fire (isolated counter), got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// CLASSIFY threshold (count_type="CLASSIFY")
// ---------------------------------------------------------------------------

func TestThreshold_CLASSIFY_NoFireAtThreshold(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cls-at">
    <check type="EQU" field="action">get</check>
    <threshold group_by="uid" range="60s" count_type="CLASSIFY" count_field="resource" local_cache="true">3</threshold>
  </rule>
</root>`)

	mk := func(res string) map[string]interface{} {
		return map[string]interface{}{"action": "get", "uid": "alice", "resource": res}
	}

	// Exactly 3 distinct → no fire (3 is not > 3)
	for i, r := range []string{"secret/a", "secret/b", "secret/c"} {
		if out := rs.EngineCheck(mk(r)); len(out) != 0 {
			t.Fatalf("event %d (%s): should not fire at distinct == threshold, got %d", i+1, r, len(out))
		}
	}
}

func TestThreshold_CLASSIFY_FiresOnExceed(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cls-fire">
    <check type="EQU" field="action">get</check>
    <threshold group_by="uid" range="60s" count_type="CLASSIFY" count_field="resource" local_cache="true">3</threshold>
  </rule>
</root>`)

	mk := func(res string) map[string]interface{} {
		return map[string]interface{}{"action": "get", "uid": "alice", "resource": res}
	}

	// 3 distinct → no fire
	rs.EngineCheck(mk("secret/a"))
	rs.EngineCheck(mk("secret/b"))
	rs.EngineCheck(mk("secret/c"))

	// 4th distinct → fire
	if out := rs.EngineCheck(mk("secret/d")); len(out) != 1 {
		t.Fatalf("4th distinct should fire, got %d", len(out))
	}
}

func TestThreshold_CLASSIFY_DuplicatesNotCounted(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cls-dup">
    <threshold group_by="uid" range="60s" count_type="CLASSIFY" count_field="resource" local_cache="true">3</threshold>
  </rule>
</root>`)

	data := map[string]interface{}{"uid": "bob", "resource": "secret/x"}
	// Same resource 10 times — distinct count stays 1, never exceeds 3
	for i := 0; i < 10; i++ {
		if out := rs.EngineCheck(data); len(out) != 0 {
			t.Fatalf("iter %d: duplicate resource should not increment distinct count, got %d", i, len(out))
		}
	}
}

func TestThreshold_CLASSIFY_ResetsAfterFire(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cls-reset">
    <threshold group_by="uid" range="60s" count_type="CLASSIFY" count_field="res" local_cache="true">2</threshold>
  </rule>
</root>`)

	mk := func(res string) map[string]interface{} {
		return map[string]interface{}{"uid": "carol", "res": res}
	}

	// 3rd distinct fires
	rs.EngineCheck(mk("a"))
	rs.EngineCheck(mk("b"))
	if out := rs.EngineCheck(mk("c")); len(out) != 1 {
		t.Fatalf("3rd distinct should fire, got %d", len(out))
	}

	// Keys deleted on fire; next event restarts from 1 → no fire
	if out := rs.EngineCheck(mk("d")); len(out) != 0 {
		t.Fatalf("first event after classify reset should not fire, got %d", len(out))
	}
}

func TestThreshold_CLASSIFY_GroupBySeparatesUsers(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cls-grp">
    <threshold group_by="uid" range="60s" count_type="CLASSIFY" count_field="res" local_cache="true">2</threshold>
  </rule>
</root>`)

	mk := func(uid, res string) map[string]interface{} {
		return map[string]interface{}{"uid": uid, "res": res}
	}

	// u1: 3 distinct → fire
	rs.EngineCheck(mk("u1", "a"))
	rs.EngineCheck(mk("u1", "b"))
	if out := rs.EngineCheck(mk("u1", "c")); len(out) != 1 {
		t.Fatalf("u1 should fire at 3rd distinct, got %d", len(out))
	}

	// u2: 1 distinct → no fire (isolated counter)
	if out := rs.EngineCheck(mk("u2", "a")); len(out) != 0 {
		t.Fatalf("u2 first event should not fire (isolated counter), got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// Threshold inside a checklist condition
// ---------------------------------------------------------------------------

func TestThreshold_InChecklist_WithCondition(t *testing.T) {
	// The threshold is inside a checklist with condition="sensitive and freq".
	// Note: when a checklist has ConditionFlag=true, ALL nodes (checks AND
	// thresholds) are always evaluated to populate the condition map.
	// So the threshold counter increments on every event reaching the rule,
	// regardless of whether sensitiveResource evaluates to true.
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cl-threshold">
    <check type="EQU" field="verb">get</check>
    <checklist condition="sensitive and freq">
      <check id="sensitive" type="START" field="resource">/secrets/</check>
      <threshold id="freq" group_by="uid" range="60s" local_cache="true">2</threshold>
    </checklist>
  </rule>
</root>`)

	// 3 events all targeting /secrets/, threshold=2:
	//   event 1: sensitive=T, freq=false(count=1, 1<=2) → T and F = false
	//   event 2: sensitive=T, freq=false(count=2, 2<=2) → T and F = false
	//   event 3: sensitive=T, freq=true(count=3, 3>2)   → T and T = true → fire

	mk := func(res string) map[string]interface{} {
		return map[string]interface{}{"verb": "get", "uid": "u1", "resource": res}
	}

	if out := rs.EngineCheck(mk("/secrets/a")); len(out) != 0 {
		t.Fatalf("event 1: should not fire, got %d", len(out))
	}
	if out := rs.EngineCheck(mk("/secrets/b")); len(out) != 0 {
		t.Fatalf("event 2: should not fire, got %d", len(out))
	}
	if out := rs.EngineCheck(mk("/secrets/c")); len(out) != 1 {
		t.Fatalf("event 3: should fire (freq exceeded + sensitive), got %d", len(out))
	}
}

func TestThreshold_InChecklist_NonSensitiveStillCounts(t *testing.T) {
	// Demonstrates that a non-sensitive resource STILL increments the threshold
	// counter because checklist condition evaluation always runs all nodes.
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cl-threshold2">
    <check type="EQU" field="verb">get</check>
    <checklist condition="sensitive and freq">
      <check id="sensitive" type="START" field="resource">/secrets/</check>
      <threshold id="freq" group_by="uid" range="60s" local_cache="true">2</threshold>
    </checklist>
  </rule>
</root>`)

	mk := func(res string) map[string]interface{} {
		return map[string]interface{}{"verb": "get", "uid": "u1", "resource": res}
	}

	// Non-sensitive: sensitive=F, freq increments (count=1), but F and * = false
	rs.EngineCheck(mk("/config/app"))
	// Sensitive: sensitive=T, freq increments (count=2, 2<=2 → F), T and F = false
	rs.EngineCheck(mk("/secrets/a"))
	// Sensitive: sensitive=T, freq increments (count=3, 3>2 → T), T and T = true → fire
	if out := rs.EngineCheck(mk("/secrets/b")); len(out) != 1 {
		t.Fatalf("3rd event overall should fire (non-sensitive still counted), got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// Concurrent safety
// ---------------------------------------------------------------------------

func TestThreshold_Count_Concurrent(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="concurrent">
    <threshold group_by="uid" range="60s" local_cache="true">10000</threshold>
  </rule>
</root>`)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		uid := fmt.Sprintf("user-%d", i)
		go func(uid string) {
			defer wg.Done()
			data := map[string]interface{}{"uid": uid}
			for j := 0; j < 50; j++ {
				rs.EngineCheck(data)
			}
		}(uid)
	}
	wg.Wait()
	// No race or panic = pass
}

func TestThreshold_SUM_Concurrent(t *testing.T) {
	rs := buildThresholdRuleset(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="concurrent-sum">
    <threshold group_by="uid" range="60s" count_type="SUM" count_field="amount" local_cache="true">1000000</threshold>
  </rule>
</root>`)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		uid := fmt.Sprintf("user-%d", i)
		go func(uid string) {
			defer wg.Done()
			data := map[string]interface{}{"uid": uid, "amount": "1"}
			for j := 0; j < 50; j++ {
				rs.EngineCheck(data)
			}
		}(uid)
	}
	wg.Wait()
	// No race or panic = pass
}
