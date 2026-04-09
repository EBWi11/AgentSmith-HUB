package rules_engine

import (
	"testing"
)

// ============================================================
// addHitRuleID — direct tests for all three branches
// ============================================================

func TestAddHitRuleID_NewField(t *testing.T) {
	data := map[string]interface{}{}
	addHitRuleID(data, "rs1.r1")
	if data[HitRuleIdFieldName] != "rs1.r1" {
		t.Errorf("expected 'rs1.r1', got %v", data[HitRuleIdFieldName])
	}
}

func TestAddHitRuleID_SameIDNoDuplicate(t *testing.T) {
	data := map[string]interface{}{}
	addHitRuleID(data, "rs1.r1")
	addHitRuleID(data, "rs1.r1") // should not duplicate
	if data[HitRuleIdFieldName] != "rs1.r1" {
		t.Errorf("expected 'rs1.r1' (no duplicate), got %v", data[HitRuleIdFieldName])
	}
}

func TestAddHitRuleID_DifferentIDsConcatenate(t *testing.T) {
	data := map[string]interface{}{}
	addHitRuleID(data, "rs1.r1")
	addHitRuleID(data, "rs1.r2")
	result, ok := data[HitRuleIdFieldName].(string)
	if !ok {
		t.Fatal("expected string HitRuleId")
	}
	if result != "rs1.r1,rs1.r2" {
		t.Errorf("expected 'rs1.r1,rs1.r2', got %q", result)
	}
}

// ============================================================
// executeModify — PLUGIN mode (bool-return and interface-return)
// ============================================================

func TestModify_PluginBoolReturn(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="modify-plugin-bool">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">check</check>
    <modify type="PLUGIN" field="is_private">isPrivateIP("192.168.1.1")</modify>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{"event": "check"})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	v, ok := out[0]["is_private"].(bool)
	if !ok || !v {
		t.Errorf("expected is_private=true, got %v", out[0]["is_private"])
	}
}

func TestModify_PluginInterfaceReturn(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="modify-plugin-iface">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">hash</check>
    <modify type="PLUGIN" field="hash_val">hashMD5("hello")</modify>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{"event": "hash"})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	hash, ok := out[0]["hash_val"].(string)
	if !ok || hash == "" {
		t.Errorf("expected non-empty hash string, got %v", out[0]["hash_val"])
	}
}

// ============================================================
// executePlugin — interface-return plugin (FuncEvalOther branch)
// ============================================================

func TestPlugin_InterfaceReturnSideEffect(t *testing.T) {
	// hashMD5 is an interface-return plugin — exercises the else branch of executePlugin
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="plugin-iface">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">process</check>
    <plugin>hashMD5("seed_value")</plugin>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{"event": "process"})
	if len(out) != 1 {
		t.Fatalf("expected rule to fire, got %d results", len(out))
	}
}

// ============================================================
// executeThresholdNode — SUM and CLASSIFY count types
// ============================================================

func TestThreshold_SumCountType(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="thresh-sum">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">transfer</check>
    <threshold group_by="host" count_type="SUM" count_field="bytes" range="60s" local_cache="true">100</threshold>
  </rule>
</root>`)
	t.Cleanup(func() { rs.cleanup() })

	// Accumulate 30 bytes per call; threshold fires when sum > 100 (at call 4: 4*30=120)
	fired := false
	for i := 0; i < 6; i++ {
		out := rs.EngineCheck(map[string]interface{}{"event": "transfer", "host": "server1", "bytes": "30"})
		if len(out) == 1 {
			fired = true
			break
		}
	}
	if !fired {
		t.Fatal("expected SUM threshold to fire after accumulating >100 bytes across calls")
	}
}

func TestThreshold_SumCountType_MissingField(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="thresh-sum-miss">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">transfer</check>
    <threshold group_by="host" count_type="SUM" count_field="bytes" range="60s" local_cache="true">10</threshold>
  </rule>
</root>`)
	t.Cleanup(func() { rs.cleanup() })

	// bytes field missing — should return false (not fire)
	out := rs.EngineCheck(map[string]interface{}{"event": "transfer", "host": "server1"})
	if len(out) != 0 {
		t.Fatalf("expected 0 results when count_field is missing, got %d", len(out))
	}
}

func TestThreshold_ClassifyCountType(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="thresh-classify">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">scan</check>
    <threshold group_by="src_ip" count_type="CLASSIFY" count_field="dst_port" range="60s" local_cache="true">3</threshold>
  </rule>
</root>`)
	t.Cleanup(func() { rs.cleanup() })

	// Send events with different dst_port values from the same src_ip
	ports := []string{"80", "443", "8080", "22"}
	var lastOut []map[string]interface{}
	for _, port := range ports {
		lastOut = rs.EngineCheck(map[string]interface{}{"event": "scan", "src_ip": "1.2.3.4", "dst_port": port})
	}
	// After 4 distinct ports (threshold is >3), should fire
	if len(lastOut) != 1 {
		t.Fatalf("expected threshold to fire after 4 distinct ports (>3), got %d results", len(lastOut))
	}
}

func TestThreshold_ClassifyCountType_MissingField(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="thresh-classify-miss">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">scan</check>
    <threshold group_by="src_ip" count_type="CLASSIFY" count_field="dst_port" range="60s" local_cache="true">2</threshold>
  </rule>
</root>`)
	t.Cleanup(func() { rs.cleanup() })

	// dst_port missing — should not fire
	out := rs.EngineCheck(map[string]interface{}{"event": "scan", "src_ip": "1.2.3.4"})
	if len(out) != 0 {
		t.Fatalf("expected 0 results when classify field missing, got %d", len(out))
	}
}

// ============================================================
// ValidateWithDetails — validateModify PLUGIN paths
// ============================================================

func TestValidate_ModifyPluginValid(t *testing.T) {
	raw := `<root type="DETECTION" name="v-modify-ok">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <modify type="PLUGIN" field="hash">hashMD5("data")</modify>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if !result.IsValid {
		t.Errorf("expected valid ruleset, errors: %v", result.Errors)
	}
}

func TestValidate_ModifyPluginEmptyValue(t *testing.T) {
	raw := `<root type="DETECTION" name="v-modify-empty">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <modify type="PLUGIN" field="result"></modify>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: PLUGIN modify with empty value")
	}
}

func TestValidate_ModifyPluginUnknown(t *testing.T) {
	raw := `<root type="DETECTION" name="v-modify-unknown">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <modify type="PLUGIN" field="r">nonExistentPlugin("data")</modify>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: unknown plugin in modify")
	}
}

func TestValidate_ModifyInvalidType(t *testing.T) {
	raw := `<root type="DETECTION" name="v-modify-bad-type">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <modify type="BADTYPE" field="r">value</modify>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: modify type must be empty or PLUGIN")
	}
}

// ============================================================
// ValidateWithDetails — validateChecklist paths
// ============================================================

func TestValidate_ChecklistValid(t *testing.T) {
	raw := `<root type="DETECTION" name="v-checklist-ok">
  <rule id="r1" name="test">
    <checklist>
      <check type="EQU" field="x">1</check>
    </checklist>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if !result.IsValid {
		t.Errorf("expected valid checklist, errors: %v", result.Errors)
	}
}

func TestValidate_ChecklistInvalidNodeType(t *testing.T) {
	raw := `<root type="DETECTION" name="v-checklist-bad">
  <rule id="r1" name="test">
    <checklist>
      <check type="BADTYPE" field="x">1</check>
    </checklist>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: unknown check type in checklist")
	}
}

func TestValidate_ChecklistMissingField(t *testing.T) {
	raw := `<root type="DETECTION" name="v-checklist-nofield">
  <rule id="r1" name="test">
    <checklist>
      <check type="EQU">1</check>
    </checklist>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: check node missing field")
	}
}

// ============================================================
// ValidateWithDetails — validateAppend PLUGIN paths
// ============================================================

func TestValidate_AppendPluginValid(t *testing.T) {
	raw := `<root type="DETECTION" name="v-append-plugin-ok">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <append type="PLUGIN" field="h">hashMD5("data")</append>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if !result.IsValid {
		t.Errorf("expected valid, errors: %v", result.Errors)
	}
}

func TestValidate_AppendPluginEmptyValue(t *testing.T) {
	raw := `<root type="DETECTION" name="v-append-empty">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <append type="PLUGIN" field="h"></append>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: PLUGIN append with empty value")
	}
}

func TestValidate_AppendPluginUnknown(t *testing.T) {
	raw := `<root type="DETECTION" name="v-append-unknown">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <append type="PLUGIN" field="h">unknownPlugin("x")</append>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: unknown plugin in append")
	}
}

func TestValidate_AppendMissingField(t *testing.T) {
	raw := `<root type="DETECTION" name="v-append-nofield">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <append>value</append>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: append missing field attribute")
	}
}

// ============================================================
// ValidateWithDetails — validatePlugin paths
// ============================================================

func TestValidate_PluginValid(t *testing.T) {
	raw := `<root type="DETECTION" name="v-plugin-ok">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <plugin>isPrivateIP("10.0.0.1")</plugin>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if !result.IsValid {
		t.Errorf("expected valid, errors: %v", result.Errors)
	}
}

func TestValidate_PluginUnknown(t *testing.T) {
	raw := `<root type="DETECTION" name="v-plugin-unknown">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <plugin>unknownPlugin("x")</plugin>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: unknown plugin in <plugin>")
	}
}

func TestValidate_PluginEmptyValue(t *testing.T) {
	raw := `<root type="DETECTION" name="v-plugin-empty">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <plugin></plugin>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: plugin with empty value")
	}
}

// ============================================================
// validateIteratorCheckNode and validateIteratorThreshold
// via ValidateWithDetails with invalid iterator content
// ============================================================

func TestValidate_IteratorCheckNodeInvalidType(t *testing.T) {
	// validateIteratorCheckNode only requires type to be non-empty;
	// it does NOT validate against an allowlist. Test that empty type fails.
	raw := `<root type="DETECTION" name="v-iter-check-bad">
  <rule id="r1" name="test">
    <iterator type="ANY" field="items" variable="item">
      <check type="" field="item">val</check>
    </iterator>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: empty check type inside iterator")
	}
}

func TestValidate_IteratorThresholdInvalidRange(t *testing.T) {
	// validateIteratorThreshold checks that range is non-empty, not valid format
	// Test that empty range fails
	raw := `<root type="DETECTION" name="v-iter-thresh-bad">
  <rule id="r1" name="test">
    <iterator type="ANY" field="items" variable="item">
      <check type="EQU" field="item">val</check>
      <threshold group_by="item" range="" local_cache="true">2</threshold>
    </iterator>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: empty threshold range in iterator")
	}
}

// ============================================================
// validatePluginParameters path coverage
// ============================================================

func TestValidate_PluginInvalidCallSyntax(t *testing.T) {
	// A plugin call with syntax error should fail validation
	raw := `<root type="DETECTION" name="v-plugin-syntax">
  <rule id="r1" name="test">
    <check type="EQU" field="x">1</check>
    <plugin>isPrivateIP(</plugin>
  </rule>
</root>`
	result, _ := ValidateWithDetails("", raw)
	if result.IsValid {
		t.Error("expected invalid: syntax error in plugin call")
	}
}

// ============================================================
// engine_core.go — evaluateEventChecklist via sequence EngineCheck
// ============================================================

func TestSequence_WithChecklist(t *testing.T) {
	rs := buildTestRuleset(t, `
<root type="DETECTION" name="seq-checklist">
  <rule id="r1" name="r1">
    <sequence within="10m" group_by="user" local_cache="true">
      <event id="login">
        <checklist>
          <check type="EQU" field="type">login</check>
          <check type="NOTNULL" field="user"></check>
        </checklist>
      </event>
      <event id="access">
        <check type="EQU" field="type">access</check>
      </event>
      <condition>login -> access</condition>
    </sequence>
  </rule>
</root>`)

	rs.EngineCheck(map[string]interface{}{"type": "login", "user": "alice"})
	out := rs.EngineCheck(map[string]interface{}{"type": "access", "user": "alice"})
	if len(out) != 1 {
		t.Fatalf("expected sequence to complete, got %d results", len(out))
	}
}
