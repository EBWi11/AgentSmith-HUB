package rules_engine

// Tests for all check-node operators and checklist conditions via the full
// EngineCheck pipeline: XML → ParseRuleset → RulesetBuild → EngineCheck.
//
// All data maps use simple flat field names (no dots) because the engine's
// field-path resolver splits on '.' and navigates nested maps. For nested
// access use map[string]interface{} values; for unit testing purposes simple
// names are cleaner and less fragile.

import (
	"testing"
)

// ---------------------------------------------------------------------------
// EQU / NEQ (case-insensitive equality — EQU uses strings.EqualFold)
// ---------------------------------------------------------------------------

func TestCheck_EQU_Match(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="equ-match">
    <check type="EQU" field="status">active</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"status": "active"}); len(out) != 1 {
		t.Fatalf("expected 1 match, got %d", len(out))
	}
}

func TestCheck_EQU_CaseInsensitive(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="equ-ci">
    <check type="EQU" field="status">ACTIVE</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"status": "active"}); len(out) != 1 {
		t.Fatalf("EQU is case-insensitive: expected 1, got %d", len(out))
	}
}

func TestCheck_EQU_NoMatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="equ-no">
    <check type="EQU" field="status">active</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"status": "inactive"}); len(out) != 0 {
		t.Fatalf("expected no match, got %d", len(out))
	}
}

func TestCheck_NEQ_Match(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="neq-match">
    <check type="NEQ" field="result">success</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"result": "failure"}); len(out) != 1 {
		t.Fatalf("expected 1 match, got %d", len(out))
	}
}

func TestCheck_NEQ_NoMatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="neq-no">
    <check type="NEQ" field="result">failure</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"result": "failure"}); len(out) != 0 {
		t.Fatalf("expected no match, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// NCS_EQU / NCS_NEQ (explicitly case-insensitive equality)
// ---------------------------------------------------------------------------

func TestCheck_NCS_EQU_SameCase(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-equ-s">
    <check type="NCS_EQU" field="verb">GET</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"verb": "GET"}); len(out) != 1 {
		t.Fatalf("expected 1, got %d", len(out))
	}
}

func TestCheck_NCS_EQU_DifferentCase(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-equ-d">
    <check type="NCS_EQU" field="verb">get</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"verb": "GET"}); len(out) != 1 {
		t.Fatalf("NCS_EQU cross-case should match, got %d", len(out))
	}
}

func TestCheck_NCS_EQU_NoMatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-equ-no">
    <check type="NCS_EQU" field="verb">delete</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"verb": "GET"}); len(out) != 0 {
		t.Fatalf("expected no match, got %d", len(out))
	}
}

func TestCheck_NCS_NEQ_DifferentStrings(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-neq-d">
    <check type="NCS_NEQ" field="verb">delete</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"verb": "GET"}); len(out) != 1 {
		t.Fatalf("NCS_NEQ different strings should fire, got %d", len(out))
	}
}

func TestCheck_NCS_NEQ_SameCaseNoFire(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-neq-s">
    <check type="NCS_NEQ" field="verb">GET</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"verb": "GET"}); len(out) != 0 {
		t.Fatalf("NCS_NEQ equal strings should not fire, got %d", len(out))
	}
}

func TestCheck_NCS_NEQ_CrossCaseNoFire(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-neq-cc">
    <check type="NCS_NEQ" field="verb">get</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"verb": "GET"}); len(out) != 0 {
		t.Fatalf("NCS_NEQ case-insensitive equal should not fire, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// INCL / NI (case-sensitive substring)
// ---------------------------------------------------------------------------

func TestCheck_INCL_Contains(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="incl">
    <check type="INCL" field="cmd">powershell</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"cmd": "powershell -enc abc"}); len(out) != 1 {
		t.Fatalf("expected 1, got %d", len(out))
	}
}

func TestCheck_INCL_NoMatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="incl-no">
    <check type="INCL" field="cmd">powershell</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"cmd": "cmd.exe /c dir"}); len(out) != 0 {
		t.Fatalf("expected no match, got %d", len(out))
	}
}

func TestCheck_NI_NotContains(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ni">
    <check type="NI" field="executable">powershell</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"executable": "cmd.exe"}); len(out) != 1 {
		t.Fatalf("NI should fire when not containing pattern, got %d", len(out))
	}
}

func TestCheck_NI_ContainsNoFire(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ni-no">
    <check type="NI" field="executable">powershell</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"executable": "powershell.exe"}); len(out) != 0 {
		t.Fatalf("NI should not fire when containing pattern, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// NCS_INCL / NCS_NI (case-insensitive substring)
// ---------------------------------------------------------------------------

func TestCheck_NCS_INCL_CaseMismatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-incl">
    <check type="NCS_INCL" field="cmd">POWERSHELL</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"cmd": "c:\\windows\\powershell.exe"}); len(out) != 1 {
		t.Fatalf("NCS_INCL case-insensitive should match, got %d", len(out))
	}
}

func TestCheck_NCS_NI_CaseInsensitiveNotContains(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-ni">
    <check type="NCS_NI" field="cmd">POWERSHELL</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"cmd": "cmd.exe /c dir"}); len(out) != 1 {
		t.Fatalf("NCS_NI should fire when not containing (case-insensitive), got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// START / NSTART / END / NEND
// ---------------------------------------------------------------------------

func TestCheck_START_Match(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="start">
    <check type="START" field="path">/admin</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"path": "/admin/users"}); len(out) != 1 {
		t.Fatalf("expected match, got %d", len(out))
	}
}

func TestCheck_START_NoMatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="start-no">
    <check type="START" field="path">/admin</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"path": "/public/index"}); len(out) != 0 {
		t.Fatalf("expected no match, got %d", len(out))
	}
}

func TestCheck_NSTART_Match(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="nstart">
    <check type="NSTART" field="path">/public</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"path": "/admin/secret"}); len(out) != 1 {
		t.Fatalf("NSTART should fire when not starting with pattern, got %d", len(out))
	}
}

func TestCheck_END_Match(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="end">
    <check type="END" field="filename">.exe</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"filename": "malware.exe"}); len(out) != 1 {
		t.Fatalf("expected match, got %d", len(out))
	}
}

func TestCheck_NEND_Match(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="nend">
    <check type="NEND" field="filename">.txt</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"filename": "payload.exe"}); len(out) != 1 {
		t.Fatalf("NEND should fire when not ending with pattern, got %d", len(out))
	}
}

func TestCheck_NCS_START_CaseMismatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-start">
    <check type="NCS_START" field="path">/ADMIN</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"path": "/admin/secret"}); len(out) != 1 {
		t.Fatalf("NCS_START case-insensitive should match, got %d", len(out))
	}
}

func TestCheck_NCS_END_CaseMismatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-end">
    <check type="NCS_END" field="filename">.EXE</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"filename": "payload.exe"}); len(out) != 1 {
		t.Fatalf("NCS_END case-insensitive should match, got %d", len(out))
	}
}

func TestCheck_NCS_NSTART_CaseInsensitive(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-nstart">
    <check type="NCS_NSTART" field="path">/PUBLIC</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"path": "/admin/secret"}); len(out) != 1 {
		t.Fatalf("NCS_NSTART should fire when not starting with (case-insensitive), got %d", len(out))
	}
}

func TestCheck_NCS_NEND_CaseInsensitive(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="ncs-nend">
    <check type="NCS_NEND" field="filename">.TXT</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"filename": "malware.exe"}); len(out) != 1 {
		t.Fatalf("NCS_NEND should fire when not ending with (case-insensitive), got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// MT / LT (numeric comparison — strictly greater / strictly less)
// ---------------------------------------------------------------------------

func TestCheck_MT_GreaterThan(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="mt">
    <check type="MT" field="score">80</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"score": "95"}); len(out) != 1 {
		t.Fatalf("95 > 80 should fire, got %d", len(out))
	}
}

func TestCheck_MT_EqualNoFire(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="mt-eq">
    <check type="MT" field="score">80</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"score": "80"}); len(out) != 0 {
		t.Fatalf("MT: equal should not fire (strictly greater), got %d", len(out))
	}
}

func TestCheck_LT_LessThan(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="lt">
    <check type="LT" field="status_code">300</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"status_code": "200"}); len(out) != 1 {
		t.Fatalf("200 < 300 should fire, got %d", len(out))
	}
}

func TestCheck_LT_EqualNoFire(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="lt-eq">
    <check type="LT" field="status_code">300</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"status_code": "300"}); len(out) != 0 {
		t.Fatalf("LT: equal should not fire (strictly less), got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// NOTNULL / ISNULL
// ---------------------------------------------------------------------------

func TestCheck_NOTNULL_FieldExists(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="notnull">
    <check type="NOTNULL" field="username" />
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"username": "alice"}); len(out) != 1 {
		t.Fatalf("NOTNULL: existing field should fire, got %d", len(out))
	}
}

func TestCheck_NOTNULL_FieldMissingNoFire(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="notnull-miss">
    <check type="NOTNULL" field="username" />
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"other": "value"}); len(out) != 0 {
		t.Fatalf("NOTNULL: missing field should not fire, got %d", len(out))
	}
}

func TestCheck_ISNULL_FieldMissing(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="isnull">
    <check type="ISNULL" field="optional_field" />
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"other": "value"}); len(out) != 1 {
		t.Fatalf("ISNULL: missing field should fire, got %d", len(out))
	}
}

func TestCheck_ISNULL_FieldExistsNoFire(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="isnull-no">
    <check type="ISNULL" field="optional_field" />
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"optional_field": "something"}); len(out) != 0 {
		t.Fatalf("ISNULL: non-empty field should not fire, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// REGEX
// ---------------------------------------------------------------------------

func TestCheck_REGEX_Match(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="regex">
    <check type="REGEX" field="executable">(?i)(powershell|pwsh)\.exe$</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"executable": "C:\\Windows\\powershell.exe"}); len(out) != 1 {
		t.Fatalf("REGEX should match powershell, got %d", len(out))
	}
}

func TestCheck_REGEX_NoMatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="regex-no">
    <check type="REGEX" field="executable">(?i)(powershell|pwsh)\.exe$</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"executable": "C:\\Windows\\cmd.exe"}); len(out) != 0 {
		t.Fatalf("REGEX should not match cmd.exe, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// Multi-value check (logic="OR" / logic="AND" with delimiter)
// ---------------------------------------------------------------------------

func TestCheck_MultiValue_OR_OneMatches(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="multi-or">
    <check type="INCL" field="verb" logic="OR" delimiter="|">create|update|patch</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"verb": "create"}); len(out) != 1 {
		t.Fatalf("multi-value OR: 'create' should match, got %d", len(out))
	}
}

func TestCheck_MultiValue_OR_NoMatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="multi-or-no">
    <check type="INCL" field="verb" logic="OR" delimiter="|">create|update|patch</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"verb": "delete"}); len(out) != 0 {
		t.Fatalf("multi-value OR: 'delete' should not match, got %d", len(out))
	}
}

func TestCheck_MultiValue_AND_AllMatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="multi-and">
    <check type="INCL" field="cmd" logic="AND" delimiter="|">powershell|-enc</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"cmd": "powershell -enc SGVsbG8="}); len(out) != 1 {
		t.Fatalf("multi-value AND: all patterns present should match, got %d", len(out))
	}
}

func TestCheck_MultiValue_AND_PartialNoMatch(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="multi-and-no">
    <check type="INCL" field="cmd" logic="AND" delimiter="|">powershell|-enc</check>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"cmd": "powershell -version"}); len(out) != 0 {
		t.Fatalf("multi-value AND: partial match should not fire, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// Multiple rules in one ruleset (OR relationship)
// ---------------------------------------------------------------------------

func TestCheck_MultiRule_EitherFires(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="rule-ps">
    <check type="EQU" field="event_code">1</check>
    <check type="INCL" field="executable">powershell</check>
  </rule>
  <rule id="r2" name="rule-lsass">
    <check type="EQU" field="event_code">10</check>
    <check type="INCL" field="target_image">lsass</check>
  </rule>
</root>`)

	// Only r2 matches
	out := rs.EngineCheck(map[string]interface{}{
		"event_code":   "10",
		"target_image": "lsass.exe",
		"executable":   "procdump.exe",
	})
	if len(out) != 1 {
		t.Fatalf("only r2 should fire, got %d results", len(out))
	}

	// Only r1 matches
	out = rs.EngineCheck(map[string]interface{}{
		"event_code":   "1",
		"executable":   "powershell.exe",
		"target_image": "other.exe",
	})
	if len(out) != 1 {
		t.Fatalf("only r1 should fire, got %d results", len(out))
	}

	// Neither matches
	out = rs.EngineCheck(map[string]interface{}{
		"event_code": "3",
		"executable": "explorer.exe",
	})
	if len(out) != 0 {
		t.Fatalf("neither rule should fire, got %d results", len(out))
	}
}

// ---------------------------------------------------------------------------
// Checklist — logical conditions (AND / OR / NOT / complex)
// ---------------------------------------------------------------------------

func TestChecklist_AND_BothTrue(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cl-and">
    <checklist condition="enc and hidden">
      <check id="enc" type="INCL" field="cmd">-enc</check>
      <check id="hidden" type="INCL" field="cmd">hidden</check>
    </checklist>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"cmd": "powershell -enc abc -w hidden"}); len(out) != 1 {
		t.Fatalf("AND: both true should fire, got %d", len(out))
	}
}

func TestChecklist_AND_OneFalse(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cl-and-f">
    <checklist condition="enc and hidden">
      <check id="enc" type="INCL" field="cmd">-enc</check>
      <check id="hidden" type="INCL" field="cmd">hidden</check>
    </checklist>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"cmd": "powershell -enc abc"}); len(out) != 0 {
		t.Fatalf("AND: one false should not fire, got %d", len(out))
	}
}

func TestChecklist_OR_OneTrue(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cl-or">
    <checklist condition="enc or bypass">
      <check id="enc" type="INCL" field="cmd">-enc</check>
      <check id="bypass" type="INCL" field="cmd">bypass</check>
    </checklist>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"cmd": "powershell -executionpolicy bypass"}); len(out) != 1 {
		t.Fatalf("OR: one true should fire, got %d", len(out))
	}
}

func TestChecklist_OR_BothFalse(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cl-or-f">
    <checklist condition="enc or bypass">
      <check id="enc" type="INCL" field="cmd">-enc</check>
      <check id="bypass" type="INCL" field="cmd">bypass</check>
    </checklist>
  </rule>
</root>`)
	if out := rs.EngineCheck(map[string]interface{}{"cmd": "powershell -version"}); len(out) != 0 {
		t.Fatalf("OR: both false should not fire, got %d", len(out))
	}
}

func TestChecklist_NOT_NegatedTrue_NoFire(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cl-not">
    <checklist condition="not trusted">
      <check id="trusted" type="EQU" field="role">system</check>
    </checklist>
  </rule>
</root>`)
	// role=system → trusted=true → not trusted=false → no fire
	if out := rs.EngineCheck(map[string]interface{}{"role": "system"}); len(out) != 0 {
		t.Fatalf("NOT: negated true should not fire, got %d", len(out))
	}
}

func TestChecklist_NOT_NegatedFalse_Fires(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="cl-not-f">
    <checklist condition="not trusted">
      <check id="trusted" type="EQU" field="role">system</check>
    </checklist>
  </rule>
</root>`)
	// role=attacker → trusted=false → not trusted=true → fire
	if out := rs.EngineCheck(map[string]interface{}{"role": "attacker"}); len(out) != 1 {
		t.Fatalf("NOT: negated false should fire, got %d", len(out))
	}
}

func TestChecklist_Complex_Condition(t *testing.T) {
	// Mimics S-B001: event_code=1, executable contains powershell,
	// checklist: enc AND (hidden OR bypass)
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="S-B001" name="suspicious-powershell">
    <check type="EQU" field="event_code">1</check>
    <check type="INCL" field="executable">powershell</check>
    <checklist condition="enc and (hidden or bypass)">
      <check id="enc" type="INCL" field="cmd">-enc</check>
      <check id="hidden" type="INCL" field="cmd">hidden</check>
      <check id="bypass" type="INCL" field="cmd">bypass</check>
    </checklist>
  </rule>
</root>`)

	base := map[string]interface{}{
		"event_code": "1",
		"executable": "powershell.exe",
	}

	// enc=T, hidden=F, bypass=T → T and (F or T) = true → fire
	d1 := map[string]interface{}{"cmd": "powershell.exe -enc abc -executionpolicy bypass"}
	for k, v := range base {
		d1[k] = v
	}
	if out := rs.EngineCheck(d1); len(out) != 1 {
		t.Fatalf("T and (F or T): expected fire, got %d", len(out))
	}

	// enc=F → F and (...) = false → no fire
	d2 := map[string]interface{}{"cmd": "powershell.exe -w hidden"}
	for k, v := range base {
		d2[k] = v
	}
	if out := rs.EngineCheck(d2); len(out) != 0 {
		t.Fatalf("F and (...): expected no fire, got %d", len(out))
	}

	// enc=T, hidden=F, bypass=F → T and (F or F) = false → no fire
	d3 := map[string]interface{}{"cmd": "powershell.exe -enc abc -version"}
	for k, v := range base {
		d3[k] = v
	}
	if out := rs.EngineCheck(d3); len(out) != 0 {
		t.Fatalf("T and (F or F): expected no fire, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// Dynamic field reference (_$ prefix)
// ---------------------------------------------------------------------------

func TestCheck_DynamicValue_FromField(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="dynamic">
    <check type="EQU" field="username">_$expected_user</check>
  </rule>
</root>`)

	// username matches expected_user field value → fire
	if out := rs.EngineCheck(map[string]interface{}{
		"username":      "alice",
		"expected_user": "alice",
	}); len(out) != 1 {
		t.Fatalf("dynamic reference: matching fields should fire, got %d", len(out))
	}

	// username doesn't match expected_user → no fire
	if out := rs.EngineCheck(map[string]interface{}{
		"username":      "bob",
		"expected_user": "alice",
	}); len(out) != 0 {
		t.Fatalf("dynamic reference: mismatched fields should not fire, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// Nested field access (proper nested map structure)
// ---------------------------------------------------------------------------

func TestCheck_NestedField_Access(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="nested">
    <check type="EQU" field="event.code">1</check>
    <check type="INCL" field="process.executable">powershell</check>
  </rule>
</root>`)

	data := map[string]interface{}{
		"event": map[string]interface{}{
			"code": "1",
		},
		"process": map[string]interface{}{
			"executable": "powershell.exe",
		},
	}
	if out := rs.EngineCheck(data); len(out) != 1 {
		t.Fatalf("nested field access should fire, got %d", len(out))
	}
}

// ---------------------------------------------------------------------------
// Append / Del — output enrichment
// ---------------------------------------------------------------------------

func TestCheck_Append_AddsField(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="append-test">
    <check type="EQU" field="event_code">1</check>
    <append field="severity">high</append>
    <append field="mitre_technique_id">T1059.001</append>
  </rule>
</root>`)
	out := rs.EngineCheck(map[string]interface{}{"event_code": "1"})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	if out[0]["severity"] != "high" {
		t.Fatalf("expected severity=high, got %v", out[0]["severity"])
	}
	if out[0]["mitre_technique_id"] != "T1059.001" {
		t.Fatalf("expected mitre_technique_id=T1059.001, got %v", out[0]["mitre_technique_id"])
	}
}

func TestCheck_Del_RemovesField(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="test">
  <rule id="r1" name="del-test">
    <check type="EQU" field="action">login</check>
    <del>password,secret_token</del>
  </rule>
</root>`)
	out := rs.EngineCheck(map[string]interface{}{
		"action":       "login",
		"password":     "hunter2",
		"secret_token": "tok-abc",
		"username":     "alice",
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	if _, ok := out[0]["password"]; ok {
		t.Fatal("password field should have been deleted")
	}
	if _, ok := out[0]["secret_token"]; ok {
		t.Fatal("secret_token field should have been deleted")
	}
	if out[0]["username"] != "alice" {
		t.Fatal("username should be preserved")
	}
}

// ---------------------------------------------------------------------------
// EXCLUDE type ruleset
// ---------------------------------------------------------------------------

func TestExclude_MatchFiltersData(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="EXCLUDE" name="test-excl">
  <rule id="x1" name="allowlist-system">
    <check type="EQU" field="username">system</check>
  </rule>
</root>`)

	// system user → excluded (empty result)
	if out := rs.EngineCheck(map[string]interface{}{"username": "system"}); len(out) != 0 {
		t.Fatalf("EXCLUDE: matching rule should filter data, got %d results", len(out))
	}

	// non-system user → passes through
	if out := rs.EngineCheck(map[string]interface{}{"username": "attacker"}); len(out) != 1 {
		t.Fatalf("EXCLUDE: non-matching rule should pass data through, got %d results", len(out))
	}
}
