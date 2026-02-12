package rules_engine

import (
	"testing"
	"time"

	"github.com/dgraph-io/ristretto/v2"
)

// ============================================================================
// CEP Integration Tests
//
// Tests the full CEP pipeline: XML parsing -> RulesetBuild -> execution.
// Uses local cache mode to avoid Redis dependency in tests.
// ============================================================================

// --- Helper: parse and build a ruleset from XML ---

func buildTestRuleset(t *testing.T, xmlContent string) *Ruleset {
	t.Helper()
	ruleset, err := ParseRuleset([]byte(xmlContent))
	if err != nil {
		t.Fatalf("ParseRuleset failed: %v", err)
	}
	ruleset.RulesetID = "test_ruleset"
	ruleset.IsDetection = true

	err = RulesetBuild(ruleset)
	if err != nil {
		t.Fatalf("RulesetBuild failed: %v", err)
	}

	// Initialize state manager for local cache tests
	if ruleset.seqStateManager == nil {
		cache, _ := ristretto.NewCache(&ristretto.Config[string, *SequenceState]{
			NumCounters: 10_000,
			MaxCost:     1024 * 1024,
			BufferItems: 64,
		})
		ruleset.seqStateManager = NewCEPStateManager(true, cache)
		ruleset.SequenceCache = cache
	}

	t.Cleanup(func() {
		if ruleset != nil {
			ruleset.cleanup()
		}
	})

	return ruleset
}

// ============================================================================
// XML Parsing Tests
// ============================================================================

func TestCEP_ParseBasicSequence(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="basic seq">
			<sequence within="10m" group_by="source_ip" local_cache="true">
				<event id="login" event_time="timestamp">
					<check type="EQU" field="event_type">login</check>
				</event>
				<event id="exfil" event_time="timestamp">
					<check type="EQU" field="event_type">file_transfer</check>
				</event>
				<condition>login -> exfil</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	if len(rs.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rs.Rules))
	}
	rule := rs.Rules[0]
	if len(rule.SequenceMap) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(rule.SequenceMap))
	}

	// Find the sequence (key is the operatorIDCounter)
	var seq Sequence
	for _, s := range rule.SequenceMap {
		seq = s
		break
	}

	if seq.Within != "10m" {
		t.Errorf("expected within='10m', got '%s'", seq.Within)
	}
	if seq.WithinSec != 600 {
		t.Errorf("expected WithinSec=600, got %d", seq.WithinSec)
	}
	if seq.WithinMs != 600000 {
		t.Errorf("expected WithinMs=600000, got %d", seq.WithinMs)
	}
	if seq.GroupBy != "source_ip" {
		t.Errorf("expected group_by='source_ip', got '%s'", seq.GroupBy)
	}
	if !seq.LocalCache {
		t.Error("expected local_cache=true")
	}
	if len(seq.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(seq.Events))
	}

	loginEvent := seq.Events["login"]
	if loginEvent == nil {
		t.Fatal("expected 'login' event")
	}
	if loginEvent.EventTime != "timestamp" {
		t.Errorf("expected event_time='timestamp', got '%s'", loginEvent.EventTime)
	}
	if len(loginEvent.CheckNodes) != 1 {
		t.Errorf("expected 1 check node, got %d", len(loginEvent.CheckNodes))
	}

	if seq.Condition == nil {
		t.Fatal("expected parsed condition")
	}
	if seq.Condition.StageCount() != 2 {
		t.Errorf("expected 2 stages, got %d", seq.Condition.StageCount())
	}
}

func TestCEP_ParseAbsenceSequence(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="absence test">
			<sequence within="2m" group_by="user_id" local_cache="true">
				<event id="login">
					<check type="EQU" field="event_type">login</check>
				</event>
				<event id="mfa">
					<check type="EQU" field="event_type">mfa_verify</check>
				</event>
				<condition>login -> !mfa</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	var seq Sequence
	for _, s := range rs.Rules[0].SequenceMap {
		seq = s
		break
	}

	if seq.Condition.StageCount() != 2 {
		t.Fatalf("expected 2 stages, got %d", seq.Condition.StageCount())
	}
	if !seq.Condition.Stages[1].IsAbsent {
		t.Error("expected stage 1 to be absence")
	}
	if !rs.hasAbsenceSequences {
		t.Error("expected hasAbsenceSequences=true")
	}
}

func TestCEP_ParseMultiSourceSequence(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="multi source">
			<sequence within="5m" local_cache="true">
				<event id="fw_block" group_by="src_ip">
					<check type="EQU" field="action">block</check>
				</event>
				<event id="auth_success" group_by="client_ip">
					<check type="EQU" field="event_type">auth</check>
				</event>
				<condition>fw_block -> auth_success</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	var seq Sequence
	for _, s := range rs.Rules[0].SequenceMap {
		seq = s
		break
	}

	if seq.GroupBy != "" {
		t.Errorf("expected no sequence-level group_by, got '%s'", seq.GroupBy)
	}

	fwEvent := seq.Events["fw_block"]
	if fwEvent.GroupBy != "src_ip" {
		t.Errorf("expected fw_block group_by='src_ip', got '%s'", fwEvent.GroupBy)
	}

	authEvent := seq.Events["auth_success"]
	if authEvent.GroupBy != "client_ip" {
		t.Errorf("expected auth_success group_by='client_ip', got '%s'", authEvent.GroupBy)
	}
}

func TestCEP_ParseWithChecklist(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="checklist in event">
			<sequence within="10m" group_by="source_ip" local_cache="true">
				<event id="brute">
					<checklist condition="a and b">
						<check id="a" type="EQU" field="event_type">login</check>
						<check id="b" type="EQU" field="result">failure</check>
					</checklist>
				</event>
				<event id="success">
					<check type="EQU" field="event_type">login</check>
					<check type="EQU" field="result">success</check>
				</event>
				<condition>brute -> success</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	var seq Sequence
	for _, s := range rs.Rules[0].SequenceMap {
		seq = s
		break
	}

	bruteEvent := seq.Events["brute"]
	if len(bruteEvent.Checklists) != 1 {
		t.Fatalf("expected 1 checklist in brute event, got %d", len(bruteEvent.Checklists))
	}
	if !bruteEvent.Checklists[0].ConditionFlag {
		t.Error("expected checklist to have condition flag")
	}

	successEvent := seq.Events["success"]
	if len(successEvent.CheckNodes) != 2 {
		t.Errorf("expected 2 check nodes in success event, got %d", len(successEvent.CheckNodes))
	}
}

// ============================================================================
// XML Validation Error Tests
// ============================================================================

func TestCEP_ParseError_MissingWithin(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="test">
			<sequence group_by="ip" local_cache="true">
				<event id="a"><check type="EQU" field="f">v</check></event>
				<event id="b"><check type="EQU" field="f">v</check></event>
				<condition>a -> b</condition>
			</sequence>
		</rule>
	</root>`

	_, err := ParseRuleset([]byte(xml))
	if err == nil {
		t.Fatal("expected error for missing 'within' attribute")
	}
}

func TestCEP_ParseError_TooFewEvents(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="test">
			<sequence within="5m" group_by="ip" local_cache="true">
				<event id="a"><check type="EQU" field="f">v</check></event>
				<condition>a -> b</condition>
			</sequence>
		</rule>
	</root>`

	_, err := ParseRuleset([]byte(xml))
	if err == nil {
		t.Fatal("expected error for too few events")
	}
}

func TestCEP_ParseError_MissingCondition(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="test">
			<sequence within="5m" group_by="ip" local_cache="true">
				<event id="a"><check type="EQU" field="f">v</check></event>
				<event id="b"><check type="EQU" field="f">v</check></event>
			</sequence>
		</rule>
	</root>`

	_, err := ParseRuleset([]byte(xml))
	if err == nil {
		t.Fatal("expected error for missing condition")
	}
}

func TestCEP_ParseError_DuplicateEventID(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="test">
			<sequence within="5m" group_by="ip" local_cache="true">
				<event id="a"><check type="EQU" field="f">v1</check></event>
				<event id="a"><check type="EQU" field="f">v2</check></event>
				<condition>a -> a</condition>
			</sequence>
		</rule>
	</root>`

	_, err := ParseRuleset([]byte(xml))
	if err == nil {
		t.Fatal("expected error for duplicate event ID")
	}
}

func TestCEP_BuildError_UndefinedEventInCondition(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="test">
			<sequence within="5m" group_by="ip" local_cache="true">
				<event id="a"><check type="EQU" field="f">v</check></event>
				<event id="b"><check type="EQU" field="f">v</check></event>
				<condition>a -> c</condition>
			</sequence>
		</rule>
	</root>`

	rs, err := ParseRuleset([]byte(xml))
	if err != nil {
		t.Fatalf("parse should succeed: %v", err)
	}
	rs.RulesetID = "test"
	err = RulesetBuild(rs)
	if err == nil {
		t.Fatal("expected build error for undefined event 'c' in condition")
	}
}

func TestCEP_BuildError_UnreferencedEvent(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="test">
			<sequence within="5m" group_by="ip" local_cache="true">
				<event id="a"><check type="EQU" field="f">v</check></event>
				<event id="b"><check type="EQU" field="f">v</check></event>
				<event id="c"><check type="EQU" field="f">v</check></event>
				<condition>a -> b</condition>
			</sequence>
		</rule>
	</root>`

	rs, err := ParseRuleset([]byte(xml))
	if err != nil {
		t.Fatalf("parse should succeed: %v", err)
	}
	rs.RulesetID = "test"
	err = RulesetBuild(rs)
	if err == nil {
		t.Fatal("expected build error for unreferenced event 'c'")
	}
}

func TestCEP_BuildError_NoGroupBy(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="test">
			<sequence within="5m" local_cache="true">
				<event id="a"><check type="EQU" field="f">v</check></event>
				<event id="b"><check type="EQU" field="f">v</check></event>
				<condition>a -> b</condition>
			</sequence>
		</rule>
	</root>`

	rs, err := ParseRuleset([]byte(xml))
	if err != nil {
		t.Fatalf("parse should succeed: %v", err)
	}
	rs.RulesetID = "test"
	err = RulesetBuild(rs)
	if err == nil {
		t.Fatal("expected build error for missing group_by")
	}
}

// ============================================================================
// Sequence Execution Tests
// ============================================================================

func TestCEP_Execute_BasicSequence(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="login then exfil">
			<sequence within="10m" group_by="source_ip" local_cache="true">
				<event id="login">
					<check type="EQU" field="event_type">login</check>
				</event>
				<event id="exfil">
					<check type="EQU" field="event_type">file_transfer</check>
				</event>
				<condition>login -> exfil</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Event 1: login - should NOT trigger
	event1 := map[string]interface{}{
		"event_type": "login",
		"source_ip":  "10.0.0.1",
	}
	results := rs.EngineCheck(event1)
	if len(results) != 0 {
		t.Fatalf("expected 0 results after event 1, got %d", len(results))
	}

	// Event 2: unrelated event - should NOT trigger
	event2 := map[string]interface{}{
		"event_type": "dns_query",
		"source_ip":  "10.0.0.1",
	}
	results = rs.EngineCheck(event2)
	if len(results) != 0 {
		t.Fatalf("expected 0 results after event 2, got %d", len(results))
	}

	// Wait a bit for state to be stored
	time.Sleep(20 * time.Millisecond)

	// Event 3: file_transfer from same IP - should trigger
	event3 := map[string]interface{}{
		"event_type": "file_transfer",
		"source_ip":  "10.0.0.1",
	}
	results = rs.EngineCheck(event3)
	if len(results) != 1 {
		t.Fatalf("expected 1 result after sequence completion, got %d", len(results))
	}
	if results[0][HitRuleIdFieldName] == nil {
		t.Error("expected hit rule ID in result")
	}
}

func TestCEP_Execute_DifferentCorrelationKeys(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="test">
			<sequence within="10m" group_by="source_ip" local_cache="true">
				<event id="a">
					<check type="EQU" field="event_type">login</check>
				</event>
				<event id="b">
					<check type="EQU" field="event_type">exfil</check>
				</event>
				<condition>a -> b</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Login from IP1
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.1"})
	time.Sleep(20 * time.Millisecond)

	// Login from IP2
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.2"})
	time.Sleep(20 * time.Millisecond)

	// Exfil from IP3 - should NOT trigger (no login from IP3)
	results := rs.EngineCheck(map[string]interface{}{"event_type": "exfil", "source_ip": "10.0.0.3"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results for unmatched IP, got %d", len(results))
	}

	// Exfil from IP1 - should trigger
	results = rs.EngineCheck(map[string]interface{}{"event_type": "exfil", "source_ip": "10.0.0.1"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for IP1 sequence, got %d", len(results))
	}
}

func TestCEP_Execute_ThreeStageSequence(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="three stage">
			<sequence within="10m" group_by="ip" local_cache="true">
				<event id="scan">
					<check type="EQU" field="type">scan</check>
				</event>
				<event id="exploit">
					<check type="EQU" field="type">exploit</check>
				</event>
				<event id="persist">
					<check type="EQU" field="type">persist</check>
				</event>
				<condition>scan -> exploit -> persist</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Stage 1: scan
	results := rs.EngineCheck(map[string]interface{}{"type": "scan", "ip": "1.2.3.4"})
	if len(results) != 0 {
		t.Fatalf("expected 0 after stage 1, got %d", len(results))
	}
	time.Sleep(20 * time.Millisecond)

	// Stage 2: exploit
	results = rs.EngineCheck(map[string]interface{}{"type": "exploit", "ip": "1.2.3.4"})
	if len(results) != 0 {
		t.Fatalf("expected 0 after stage 2, got %d", len(results))
	}
	time.Sleep(20 * time.Millisecond)

	// Stage 3: persist - completes sequence
	results = rs.EngineCheck(map[string]interface{}{"type": "persist", "ip": "1.2.3.4"})
	if len(results) != 1 {
		t.Fatalf("expected 1 after stage 3, got %d", len(results))
	}
}

func TestCEP_Execute_OrBranch(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="or branch">
			<sequence within="10m" group_by="user" local_cache="true">
				<event id="login">
					<check type="EQU" field="type">login</check>
				</event>
				<event id="priv">
					<check type="EQU" field="type">priv_esc</check>
				</event>
				<event id="data">
					<check type="EQU" field="type">data_access</check>
				</event>
				<condition>login -> (priv or data)</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Login
	rs.EngineCheck(map[string]interface{}{"type": "login", "user": "alice"})
	time.Sleep(20 * time.Millisecond)

	// data_access (matches "or" branch)
	results := rs.EngineCheck(map[string]interface{}{"type": "data_access", "user": "alice"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for OR branch match, got %d", len(results))
	}
}

func TestCEP_Execute_WithPreFilter(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="pre-filter test">
			<check type="EQU" field="source">internal</check>
			<sequence within="10m" group_by="ip" local_cache="true">
				<event id="a">
					<check type="EQU" field="type">scan</check>
				</event>
				<event id="b">
					<check type="EQU" field="type">exploit</check>
				</event>
				<condition>a -> b</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Event from external source - should be filtered
	rs.EngineCheck(map[string]interface{}{"type": "scan", "ip": "1.1.1.1", "source": "external"})
	time.Sleep(20 * time.Millisecond)
	results := rs.EngineCheck(map[string]interface{}{"type": "exploit", "ip": "1.1.1.1", "source": "external"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results for external source, got %d", len(results))
	}

	// Events from internal source - should work
	rs.EngineCheck(map[string]interface{}{"type": "scan", "ip": "1.1.1.1", "source": "internal"})
	time.Sleep(20 * time.Millisecond)
	results = rs.EngineCheck(map[string]interface{}{"type": "exploit", "ip": "1.1.1.1", "source": "internal"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for internal source, got %d", len(results))
	}
}

func TestCEP_Execute_CrossEventFieldReference(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="cross-event ref">
			<sequence within="10m" group_by="user" local_cache="true">
				<event id="login">
					<check type="EQU" field="type">login</check>
				</event>
				<event id="exfil">
					<check type="EQU" field="type">exfil</check>
				</event>
				<condition>login -> exfil</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Login with source_ip
	rs.EngineCheck(map[string]interface{}{
		"type":      "login",
		"user":      "bob",
		"source_ip": "192.168.1.100",
	})
	time.Sleep(20 * time.Millisecond)

	// Exfil - completes sequence
	results := rs.EngineCheck(map[string]interface{}{
		"type": "exfil",
		"user": "bob",
		"dest": "evil.com",
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Internal #event fields should not be exposed in output payloads.
	if _, exists := results[0]["#login"]; exists {
		t.Fatal("expected #login to be removed from output payload")
	}

	// Check _sequence_events contains ALL events keyed by event ID
	seqEvents, ok := results[0]["_sequence_events"]
	if !ok {
		t.Fatal("expected _sequence_events in result")
	}
	seqEventsMap, ok := seqEvents.(map[string]interface{})
	if !ok {
		t.Fatal("expected _sequence_events to be a map")
	}
	if len(seqEventsMap) != 2 {
		t.Errorf("expected 2 events in _sequence_events, got %d", len(seqEventsMap))
	}
	// Verify login event data
	loginEvt, ok := seqEventsMap["login"]
	if !ok {
		t.Fatal("expected 'login' key in _sequence_events")
	}
	loginEvtMap := loginEvt.(map[string]interface{})
	if loginEvtMap["source_ip"] != "192.168.1.100" {
		t.Errorf("expected login source_ip=192.168.1.100, got %v", loginEvtMap["source_ip"])
	}
	// Verify exfil event data
	exfilEvt, ok := seqEventsMap["exfil"]
	if !ok {
		t.Fatal("expected 'exfil' key in _sequence_events")
	}
	exfilEvtMap := exfilEvt.(map[string]interface{})
	if exfilEvtMap["dest"] != "evil.com" {
		t.Errorf("expected exfil dest=evil.com, got %v", exfilEvtMap["dest"])
	}

	// Check sequence condition details
	cond, ok := results[0]["_sequence_condition"]
	if !ok {
		t.Fatal("expected _sequence_condition in result")
	}
	condMap, ok := cond.(map[string]interface{})
	if !ok {
		t.Fatal("expected _sequence_condition to be a map")
	}
	if condMap["content"] != "login -> exfil" {
		t.Errorf("expected content 'login -> exfil', got %v", condMap["content"])
	}
}

func TestCEP_Execute_WithEventTimestamp(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="event time test">
			<sequence within="10m" group_by="ip" local_cache="true">
				<event id="a" event_time="ts">
					<check type="EQU" field="type">a</check>
				</event>
				<event id="b" event_time="ts">
					<check type="EQU" field="type">b</check>
				</event>
				<condition>a -> b</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Send events with explicit timestamps (b before a in delivery, but a is earlier in time)
	// Event b arrives first but has later timestamp
	rs.EngineCheck(map[string]interface{}{
		"type": "b",
		"ip":   "1.1.1.1",
		"ts":   "1700000002",
	})
	time.Sleep(20 * time.Millisecond)

	// Event a arrives second but has earlier timestamp
	results := rs.EngineCheck(map[string]interface{}{
		"type": "a",
		"ip":   "1.1.1.1",
		"ts":   "1700000001",
	})

	// With "match first, check time later", both events matched their stages
	// and CheckComplete should verify a.ts < b.ts, which is true
	if len(results) != 1 {
		t.Fatalf("expected 1 result for out-of-order events, got %d", len(results))
	}
}

func TestCEP_Execute_SequenceNotTriggeredOutOfOrder(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="order check">
			<sequence within="10m" group_by="ip" local_cache="true">
				<event id="a" event_time="ts">
					<check type="EQU" field="type">a</check>
				</event>
				<event id="b" event_time="ts">
					<check type="EQU" field="type">b</check>
				</event>
				<condition>a -> b</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Event a with timestamp 2000 (later)
	rs.EngineCheck(map[string]interface{}{
		"type": "a",
		"ip":   "1.1.1.1",
		"ts":   "1700000002",
	})
	time.Sleep(20 * time.Millisecond)

	// Event b with timestamp 1000 (earlier) - should NOT complete because b.ts < a.ts
	results := rs.EngineCheck(map[string]interface{}{
		"type": "b",
		"ip":   "1.1.1.1",
		"ts":   "1700000001",
	})
	if len(results) != 0 {
		t.Fatalf("expected 0 results when b.ts < a.ts (wrong order), got %d", len(results))
	}
}

// ============================================================================
// Queue Order Tests (sequence mixed with other operations)
// ============================================================================

func TestCEP_Execute_SequenceWithAppend(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="seq with append">
			<sequence within="10m" group_by="ip" local_cache="true">
				<event id="a">
					<check type="EQU" field="type">a</check>
				</event>
				<event id="b">
					<check type="EQU" field="type">b</check>
				</event>
				<condition>a -> b</condition>
			</sequence>
			<append field="alert_type">sequence_detected</append>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Stage 1
	rs.EngineCheck(map[string]interface{}{"type": "a", "ip": "1.1.1.1"})
	time.Sleep(20 * time.Millisecond)

	// Stage 2 - completes and triggers append
	results := rs.EngineCheck(map[string]interface{}{"type": "b", "ip": "1.1.1.1"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0]["alert_type"] != "sequence_detected" {
		t.Errorf("expected alert_type='sequence_detected', got '%v'", results[0]["alert_type"])
	}
}

func TestCEP_Execute_OrBranch_OverlappingEvents_BindsSingleBranch(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="test" name="test">
			<sequence within="5m" group_by="agent_id" local_cache="true">
				<event id="login" event_time="timestamp">
					<check type="EQU" field="event_type">login</check>
				</event>
				<event id="exfil1" event_time="timestamp">
					<check type="EQU" field="event_type">file_transfer</check>
				</event>
				<event id="exfil2" event_time="timestamp">
					<check type="EQU" field="event_type">file_transfer</check>
				</event>
				<condition>login -> (exfil1 or exfil2)</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Stage 1
	res := rs.EngineCheck(map[string]interface{}{
		"agent_id":   "a1b2",
		"event_type": "login",
		"timestamp":  "1770868609",
	})
	if len(res) != 0 {
		t.Fatalf("expected 0 results after login, got %d", len(res))
	}

	// Stage 2 (matches both exfil1 and exfil2 definitions by condition, but OR branch should bind one)
	res = rs.EngineCheck(map[string]interface{}{
		"agent_id":   "a1b2",
		"event_type": "file_transfer",
		"timestamp":  "1770868619",
	})
	if len(res) != 1 {
		t.Fatalf("expected 1 result after exfil event, got %d", len(res))
	}

	seqEventsRaw, ok := res[0]["_sequence_events"]
	if !ok {
		t.Fatal("expected _sequence_events in output")
	}
	seqEvents, ok := seqEventsRaw.(map[string]interface{})
	if !ok {
		t.Fatal("expected _sequence_events to be map[string]interface{}")
	}
	if _, ok := seqEvents["login"]; !ok {
		t.Fatal("expected login key in _sequence_events")
	}
	if _, ok := seqEvents["exfil1"]; !ok {
		t.Fatal("expected exfil1 key in _sequence_events")
	}
	if _, ok := seqEvents["exfil2"]; ok {
		t.Fatal("did not expect exfil2 key for OR first-branch binding")
	}
}

func TestCEP_Execute_OrBranch_OverlappingEvents_OnlyBoundBranchAppliesContextAppend(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="test" name="test">
			<sequence within="5m" group_by="agent_id" local_cache="true">
				<event id="login">
					<check type="EQU" field="event_type">login</check>
				</event>
				<event id="exfil1">
					<check type="EQU" field="event_type">file_transfer</check>
					<append field="_@branch">left</append>
				</event>
				<event id="exfil2">
					<check type="EQU" field="event_type">file_transfer</check>
					<append field="_@branch">right</append>
				</event>
				<event id="exec">
					<check type="EQU" field="event_type">exec</check>
					<check type="EQU" field="branch">_@branch</check>
				</event>
				<condition>login -> (exfil1 or exfil2) -> exec</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Stage 1
	res := rs.EngineCheck(map[string]interface{}{
		"agent_id":   "a1b2",
		"event_type": "login",
	})
	if len(res) != 0 {
		t.Fatalf("expected 0 results after login, got %d", len(res))
	}

	// Stage 2, both exfil definitions overlap by checks.
	res = rs.EngineCheck(map[string]interface{}{
		"agent_id":   "a1b2",
		"event_type": "file_transfer",
	})
	if len(res) != 0 {
		t.Fatalf("expected 0 results after second stage, got %d", len(res))
	}

	// Stage 3 should read left-branch context value.
	res = rs.EngineCheck(map[string]interface{}{
		"agent_id":   "a1b2",
		"event_type": "exec",
		"branch":     "left",
	})
	if len(res) != 1 {
		t.Fatalf("expected 1 result after exec, got %d", len(res))
	}
}

func TestCEP_Execute_OrBranch_OverlappingEvents_EventTimeUsesBoundBranch(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="test" name="test">
			<sequence within="5m" group_by="agent_id" local_cache="true">
				<event id="login" event_time="login_ts">
					<check type="EQU" field="event_type">login</check>
				</event>
				<event id="exfil1" event_time="left_ts">
					<check type="EQU" field="event_type">file_transfer</check>
				</event>
				<event id="exfil2" event_time="right_ts">
					<check type="EQU" field="event_type">file_transfer</check>
				</event>
				<event id="exec">
					<check type="EQU" field="event_type">exec</check>
				</event>
				<condition>login -> (exfil1 or exfil2) -> exec</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Stage 1 with small event_time
	res := rs.EngineCheck(map[string]interface{}{
		"agent_id":   "a1b2",
		"event_type": "login",
		"login_ts":   "2",
	})
	if len(res) != 0 {
		t.Fatalf("expected 0 results after login, got %d", len(res))
	}

	// Stage 2 overlaps both branches. Bound branch is exfil1 (left-first), which has missing left_ts.
	// It must not incorrectly use right_ts from exfil2.
	res = rs.EngineCheck(map[string]interface{}{
		"agent_id":   "a1b2",
		"event_type": "file_transfer",
		"right_ts":   "1",
	})
	if len(res) != 0 {
		t.Fatalf("expected 0 results after second stage, got %d", len(res))
	}

	// Stage 3 should complete. If stage 2 had incorrectly used right_ts=1 (older than login_ts=2),
	// the sequence would be blocked by temporal ordering and this would return 0.
	res = rs.EngineCheck(map[string]interface{}{
		"agent_id":   "a1b2",
		"event_type": "exec",
	})
	if len(res) != 1 {
		t.Fatalf("expected 1 result after exec, got %d", len(res))
	}
}

func TestCEP_Execute_SequenceContextReference_WithAtPrefix(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="ctx ref test">
			<sequence within="10m" group_by="user_id" local_cache="true">
				<event id="download">
					<check type="EQU" field="event_type">download</check>
					<append field="_@file.current">_$file_path</append>
				</event>
				<event id="exec">
					<check type="EQU" field="event_type">exec</check>
					<check type="EQU" field="file_path">_@file.current</check>
				</event>
				<condition>download -> exec</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Stage 1: create partial state
	results := rs.EngineCheck(map[string]interface{}{
		"user_id":    "u-1",
		"event_type": "download",
		"file_path":  "/tmp/b",
	})
	if len(results) != 0 {
		t.Fatalf("expected 0 results after stage 1, got %d", len(results))
	}

	// Stage 2: exec matches only when file_path equals _@file.current
	results = rs.EngineCheck(map[string]interface{}{
		"user_id":    "u-1",
		"event_type": "exec",
		"file_path":  "/tmp/b",
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result with _@ context match, got %d", len(results))
	}
}
