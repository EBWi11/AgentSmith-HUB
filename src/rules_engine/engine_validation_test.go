package rules_engine

import (
	"strings"
	"testing"

	"AgentSmith-HUB/plugin"
)

// ============================================================
// extractLineFromEnhancedError — direct unit test (dead code coverage)
// ============================================================

func TestExtractLineFromEnhancedError_AtLine(t *testing.T) {
	line := extractLineFromEnhancedError("some error at line 42 in ruleset")
	if line != 42 {
		t.Errorf("expected 42, got %d", line)
	}
}

func TestExtractLineFromEnhancedError_OnLine(t *testing.T) {
	line := extractLineFromEnhancedError("XML syntax error on line 7:")
	if line != 7 {
		t.Errorf("expected 7, got %d", line)
	}
}

func TestExtractLineFromEnhancedError_NoLine(t *testing.T) {
	line := extractLineFromEnhancedError("some error with no line number")
	if line != 1 {
		t.Errorf("expected fallback 1, got %d", line)
	}
}

// ============================================================
// ValidateWithDetails — exercises validateRulesetStructure and
// all its sub-validators (validateThreshold, validateIterator, etc.)
// ============================================================

func TestValidate_ValidRuleset(t *testing.T) {
	raw := `
<root type="DETECTION" name="valid-ruleset">
  <rule id="r1" name="simple-check">
    <check type="EQU" field="status">active</check>
  </rule>
  <rule id="r2" name="threshold-rule">
    <check type="EQU" field="event">login</check>
    <threshold group_by="user" range="10s" local_cache="true">5</threshold>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected valid ruleset, got errors: %v", result.Errors)
	}
}

func TestValidate_InvalidRootType(t *testing.T) {
	raw := `
<root type="UNKNOWN" name="bad-type">
  <rule id="r1" name="r1">
    <check type="EQU" field="x">1</check>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for unknown root type")
	}
	// Error may come from ParseRuleset or validateRulesetStructure; just check invalid
	if len(result.Errors) == 0 {
		t.Error("expected at least one error for invalid root type")
	}
}

func TestValidate_DuplicateRuleIDs(t *testing.T) {
	raw := `
<root type="DETECTION" name="dup-ids">
  <rule id="r1" name="first">
    <check type="EQU" field="x">1</check>
  </rule>
  <rule id="r1" name="duplicate">
    <check type="EQU" field="x">2</check>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for duplicate rule IDs")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "Duplicate rule ID") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'Duplicate rule ID' error, got: %v", result.Errors)
	}
}

func TestValidate_CheckMissingField(t *testing.T) {
	raw := `
<root type="DETECTION" name="missing-field">
  <rule id="r1" name="r1">
    <check type="EQU">somevalue</check>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for check with no field")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "field cannot be empty") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'field cannot be empty' error, got: %v", result.Errors)
	}
}

// TestValidate_ThresholdMissingGroupBy validates that a threshold without
// group_by triggers the validateThreshold error path.
func TestValidate_ThresholdMissingGroupBy(t *testing.T) {
	raw := `
<root type="DETECTION" name="threshold-no-groupby">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">login</check>
    <threshold range="10s" local_cache="true">5</threshold>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for threshold with no group_by")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}
}

// TestValidate_ThresholdZeroValue validates that a threshold value of 0 fails.
func TestValidate_ThresholdZeroValue(t *testing.T) {
	raw := `
<root type="DETECTION" name="threshold-zero-value">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">login</check>
    <threshold group_by="user" range="10s" local_cache="true">0</threshold>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for threshold value=0")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error for zero threshold")
	}
}

// TestValidate_ThresholdSUMMissingCountField validates SUM type needs count_field.
func TestValidate_ThresholdSUMMissingCountField(t *testing.T) {
	raw := `
<root type="DETECTION" name="threshold-sum-no-count-field">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">transfer</check>
    <threshold group_by="user" range="10s" count_type="SUM" local_cache="true">1000</threshold>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for SUM threshold without count_field")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}
}

// TestValidate_IteratorMissingVariable validates that an iterator without
// variable triggers the validateIterator error path.
func TestValidate_IteratorMissingVariable(t *testing.T) {
	raw := `
<root type="DETECTION" name="iter-no-var">
  <rule id="r1" name="r1">
    <iterator type="ANY" field="items">
      <check type="EQU" field="val">x</check>
    </iterator>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for iterator without variable")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}
}

// TestValidate_IteratorInvalidType validates that an iterator with wrong type errors.
func TestValidate_IteratorInvalidType(t *testing.T) {
	raw := `
<root type="DETECTION" name="iter-bad-type">
  <rule id="r1" name="r1">
    <iterator type="SOME" field="items" variable="it">
      <check type="EQU" field="it">x</check>
    </iterator>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for iterator with invalid type")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}
}

// TestValidate_IteratorEmpty validates that an iterator with no checks errors.
func TestValidate_IteratorEmpty(t *testing.T) {
	raw := `
<root type="DETECTION" name="iter-empty">
  <rule id="r1" name="r1">
    <iterator type="ANY" field="items" variable="it">
    </iterator>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for empty iterator")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}
}

// TestValidate_InvalidRegexPattern triggers the REGEX node validation path.
func TestValidate_InvalidRegexPattern(t *testing.T) {
	raw := `
<root type="DETECTION" name="bad-regex">
  <rule id="r1" name="r1">
    <check type="REGEX" field="path">[invalid</check>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for bad regex pattern")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error for bad regex")
	}
}

// ============================================================
// NewRuleset — from raw string input
// ============================================================

func TestNewRuleset_FromRaw(t *testing.T) {
	raw := `
<root type="DETECTION" name="new-ruleset-test">
  <rule id="r1" name="r1">
    <check type="EQU" field="status">active</check>
  </rule>
</root>`
	rs, err := NewRuleset("", raw, "test-ruleset-id")
	if err != nil {
		t.Fatalf("NewRuleset failed: %v", err)
	}
	if rs == nil {
		t.Fatal("expected non-nil ruleset")
	}
	if rs.RulesetID != "test-ruleset-id" {
		t.Errorf("expected RulesetID=test-ruleset-id, got %q", rs.RulesetID)
	}
	t.Cleanup(func() { rs.cleanup() })
}

func TestNewRuleset_InvalidXML_Errors(t *testing.T) {
	// Use clearly-invalid XML with unclosed tag
	_, err := NewRuleset("", "<root><rule id=\"r1\"><bad</root>", "bad-ruleset")
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

func TestNewRuleset_DuplicateRuleID_Errors(t *testing.T) {
	raw := `
<root type="DETECTION" name="dup">
  <rule id="dup1" name="a"><check type="EQU" field="x">1</check></rule>
  <rule id="dup1" name="b"><check type="EQU" field="x">2</check></rule>
</root>`
	_, err := NewRuleset("", raw, "dup-test")
	if err == nil {
		t.Fatal("expected error for duplicate rule IDs")
	}
}

// ============================================================
// ParseRuleset — parser error paths (parseModify, parsePlugin)
// ============================================================

func TestParseRuleset_ModifyInvalidType(t *testing.T) {
	raw := `
<root type="DETECTION" name="bad-modify">
  <rule id="r1" name="r1">
    <check type="EQU" field="x">1</check>
    <modify type="INVALID" field="out">value</modify>
  </rule>
</root>`
	_, err := ParseRuleset([]byte(raw))
	if err == nil {
		t.Fatal("expected error for invalid modify type")
	}
	if !strings.Contains(err.Error(), "PLUGIN") {
		t.Errorf("expected error mentioning PLUGIN, got: %v", err)
	}
}

func TestParseRuleset_ModifyLiteralNoField(t *testing.T) {
	raw := `
<root type="DETECTION" name="modify-no-field">
  <rule id="r1" name="r1">
    <check type="EQU" field="x">1</check>
    <modify>some_value</modify>
  </rule>
</root>`
	_, err := ParseRuleset([]byte(raw))
	if err == nil {
		t.Fatal("expected error for modify without field")
	}
	if !strings.Contains(err.Error(), "field cannot be empty") {
		t.Errorf("expected 'field cannot be empty' error, got: %v", err)
	}
}

func TestParseRuleset_PluginEmpty(t *testing.T) {
	raw := `
<root type="DETECTION" name="plugin-empty">
  <rule id="r1" name="r1">
    <check type="EQU" field="x">1</check>
    <plugin></plugin>
  </rule>
</root>`
	_, err := ParseRuleset([]byte(raw))
	if err == nil {
		t.Fatal("expected error for empty plugin value")
	}
}

func TestParseRuleset_PluginNotFound(t *testing.T) {
	raw := `
<root type="DETECTION" name="plugin-missing">
  <rule id="r1" name="r1">
    <check type="EQU" field="x">1</check>
    <plugin>nonExistentPlugin("arg")</plugin>
  </rule>
</root>`
	_, err := ParseRuleset([]byte(raw))
	if err == nil {
		t.Fatal("expected error for unknown plugin")
	}
	if !strings.Contains(err.Error(), "plugin not found") {
		t.Errorf("expected 'plugin not found' error, got: %v", err)
	}
}

func TestParseRuleset_ModifyPluginNotFound(t *testing.T) {
	raw := `
<root type="DETECTION" name="modify-plugin-missing">
  <rule id="r1" name="r1">
    <check type="EQU" field="x">1</check>
    <modify type="PLUGIN" field="out">noSuchPlugin("arg")</modify>
  </rule>
</root>`
	_, err := ParseRuleset([]byte(raw))
	if err == nil {
		t.Fatal("expected error for unknown modify plugin")
	}
	if !strings.Contains(err.Error(), "plugin not found") {
		t.Errorf("expected 'plugin not found' error, got: %v", err)
	}
}

func TestParseRuleset_PluginInvalidSyntax(t *testing.T) {
	raw := `
<root type="DETECTION" name="plugin-bad-syntax">
  <rule id="r1" name="r1">
    <check type="EQU" field="x">1</check>
    <plugin>notAFunctionCall</plugin>
  </rule>
</root>`
	_, err := ParseRuleset([]byte(raw))
	if err == nil {
		t.Fatal("expected error for plugin with invalid syntax (not a function call)")
	}
}

// ============================================================
// ValidateWithDetails — invalid XML triggers parse error path
// ============================================================

func TestValidate_InvalidXML(t *testing.T) {
	result, err := ValidateWithDetails("", "<root><unclosed>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for malformed XML")
	}
}

// ============================================================
// ValidateWithDetails — valid ruleset with checklist exercises
// validateChecklist code path
// ============================================================

func TestValidate_ChecklistWithCondition(t *testing.T) {
	raw := `
<root type="DETECTION" name="checklist-valid">
  <rule id="r1" name="r1">
    <checklist condition="c1 AND c2">
      <check id="c1" type="EQU" field="status">active</check>
      <check id="c2" type="EQU" field="role">admin</check>
    </checklist>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected valid checklist, got errors: %v", result.Errors)
	}
}

func TestValidate_ChecklistNodeMissingIDWhenCondition(t *testing.T) {
	raw := `
<root type="DETECTION" name="checklist-no-id">
  <rule id="r1" name="r1">
    <checklist condition="c1 AND c2">
      <check id="c1" type="EQU" field="status">active</check>
      <check type="EQU" field="role">admin</check>
    </checklist>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for checklist node missing ID when condition is set")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "id cannot be empty") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'id cannot be empty' error, got: %v", result.Errors)
	}
}

// ============================================================
// ValidateWithDetails — VALID rulesets to exercise validate* happy paths
// These tests ensure validateIterator, validateAppend, validateModify,
// validatePlugin, validateCheckNodePluginCall are all exercised.
// ============================================================

// TestValidate_ValidIterator exercises the validateIterator + validateIteratorCheckNode happy path.
func TestValidate_ValidIterator(t *testing.T) {
	raw := `
<root type="DETECTION" name="valid-iter">
  <rule id="r1" name="r1">
    <iterator type="ANY" field="items" variable="it">
      <check type="EQU" field="it">x</check>
    </iterator>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected valid iterator, got errors: %v", result.Errors)
	}
}

// TestValidate_ValidIteratorAllType exercises ALL iterator type path.
func TestValidate_ValidIteratorAllType(t *testing.T) {
	raw := `
<root type="DETECTION" name="valid-iter-all">
  <rule id="r1" name="r1">
    <iterator type="ALL" field="ports" variable="port">
      <check type="MT" field="port">1024</check>
    </iterator>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected valid iterator ALL, got errors: %v", result.Errors)
	}
}

// TestValidate_ValidAppendStatic exercises validateAppend happy path for static value.
func TestValidate_ValidAppendStatic(t *testing.T) {
	raw := `
<root type="DETECTION" name="valid-append-static">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">login</check>
    <append field="alert_type">brute_force</append>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected valid static append, got errors: %v", result.Errors)
	}
}

// TestValidate_ValidAppendPlugin exercises validateAppend with plugin type (validates validatePluginParameters).
func TestValidate_ValidAppendPlugin(t *testing.T) {
	raw := `
<root type="DETECTION" name="valid-append-plugin">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">scan</check>
    <append type="PLUGIN" field="hash">hashMD5("static_seed")</append>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected valid plugin append, got errors: %v", result.Errors)
	}
	// Expect a warning about the plugin return type
	if len(result.Warnings) == 0 {
		t.Error("expected plugin return type warning")
	}
}

// TestValidate_ValidModifyLiteral exercises validateModify literal path.
func TestValidate_ValidModifyLiteral(t *testing.T) {
	raw := `
<root type="DETECTION" name="valid-modify-literal">
  <rule id="r1" name="r1">
    <check type="EQU" field="action">exec</check>
    <modify field="severity">high</modify>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected valid literal modify, got errors: %v", result.Errors)
	}
}

// TestValidate_ValidPlugin exercises validatePlugin happy path.
func TestValidate_ValidPlugin(t *testing.T) {
	raw := `
<root type="DETECTION" name="valid-plugin">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">alert</check>
    <plugin>isPrivateIP("192.168.1.1")</plugin>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected valid plugin element, got errors: %v", result.Errors)
	}
	// Expect a warning about bool return type
	if len(result.Warnings) == 0 {
		t.Error("expected plugin return type warning for bool plugin")
	}
}

// TestValidate_ValidPluginCheck exercises validateCheckNodePluginCall + ParseCheckNodePluginCall.
func TestValidate_ValidPluginCheck(t *testing.T) {
	raw := `
<root type="DETECTION" name="valid-plugin-check">
  <rule id="r1" name="r1">
    <check type="PLUGIN">isPrivateIP("10.0.0.1")</check>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected valid plugin check, got errors: %v", result.Errors)
	}
}

// TestValidate_RuleWithoutID exercises the getLineNumber fallback path in validateRule.
func TestValidate_RuleWithoutID(t *testing.T) {
	// A rule without an id attribute — flagged as invalid
	raw := `
<root type="DETECTION" name="rule-no-id">
  <rule name="no-id-rule">
    <check type="EQU" field="x">1</check>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid result for rule without ID")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error for rule without ID")
	}
}

// TestValidate_ThresholdCountFieldWarning exercises the warning path for
// unused count_field in default count mode.
func TestValidate_ThresholdCountFieldWarning(t *testing.T) {
	raw := `
<root type="DETECTION" name="threshold-count-field-warn">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">login</check>
    <threshold group_by="user" range="10s" count_field="bytes" local_cache="true">5</threshold>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// This is valid (just a warning)
	if !result.IsValid {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected at least one warning about unused count_field")
	}
}

// ============================================================
// ValidateWithDetails — iterator with threshold (validateIteratorThreshold path)
// ============================================================

func TestValidate_IteratorWithThreshold(t *testing.T) {
	raw := `
<root type="DETECTION" name="iter-threshold">
  <rule id="r1" name="r1">
    <iterator type="ANY" field="items" variable="it">
      <check type="EQU" field="it">x</check>
      <threshold group_by="it" range="10s" local_cache="true">3</threshold>
    </iterator>
  </rule>
</root>`
	result, err := ValidateWithDetails("", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected valid iterator-with-threshold, got errors: %v", result.Errors)
	}
}

// ============================================================
// Direct unit tests for utility functions in engine_struct.go
// ============================================================

func TestGetLineNumber_Found(t *testing.T) {
	xml := "<root>\n  <rule id=\"r1\">\n  </rule>\n</root>"
	line := getLineNumber(xml, "<rule", 0)
	if line != 2 {
		t.Errorf("expected line 2, got %d", line)
	}
}

func TestGetLineNumber_NotFound(t *testing.T) {
	xml := "<root></root>"
	line := getLineNumber(xml, "<rule", 0)
	if line != 1 {
		t.Errorf("expected fallback 1, got %d", line)
	}
}

func TestValidatePluginCall_Valid(t *testing.T) {
	result := &ValidationResult{IsValid: true}
	validatePluginCall(`isPrivateIP("10.0.0.1")`, 1, "r1", result)
	// isPrivateIP is a valid bool plugin
	if !result.IsValid {
		t.Errorf("expected valid plugin call, got errors: %v", result.Errors)
	}
}

func TestValidatePluginCall_NotFound(t *testing.T) {
	result := &ValidationResult{IsValid: true}
	validatePluginCall(`noSuchPlugin("arg")`, 1, "r1", result)
	if result.IsValid {
		t.Fatal("expected invalid for unknown plugin")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "not found") || strings.Contains(e.Message, "Plugin not found") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected plugin not found error, got: %v", result.Errors)
	}
}

func TestValidatePluginCall_InvalidSyntax(t *testing.T) {
	result := &ValidationResult{IsValid: true}
	validatePluginCall(`notAFunctionCall`, 1, "r1", result)
	if result.IsValid {
		t.Fatal("expected invalid for bad syntax")
	}
}

func TestGetExpectedArgumentCount(t *testing.T) {
	if n := getExpectedArgumentCount("isLocalIP"); n != 1 {
		t.Errorf("expected 1 for isLocalIP, got %d", n)
	}
	if n := getExpectedArgumentCount("unknownPlugin"); n != 0 {
		t.Errorf("expected 0 for unknownPlugin, got %d", n)
	}
}

func TestGetArgumentTypeDescription(t *testing.T) {
	tests := []struct {
		arg      *PluginArg
		expected string
	}{
		{nil, "unknown"},
		{&PluginArg{Type: 2}, "raw data (${RAWDATA})"},
		{&PluginArg{Type: 1, Value: "myfield"}, "field reference (myfield)"},
		{&PluginArg{Type: 0, Value: "hello"}, "string"},
		{&PluginArg{Type: 0, Value: 42}, "int"},
		{&PluginArg{Type: 0, Value: 3.14}, "float"},
		{&PluginArg{Type: 0, Value: true}, "bool"},
	}
	for _, tc := range tests {
		desc := getArgumentTypeDescription(tc.arg)
		if !strings.Contains(desc, tc.expected) {
			t.Errorf("expected %q to contain %q", desc, tc.expected)
		}
	}
}

func TestFormatRequiredParameters(t *testing.T) {
	params := []plugin.PluginParameter{
		{Name: "ip", Type: "string", Required: true},
		{Name: "cidr", Type: "string", Required: true},
		{Name: "timeout", Type: "int", Required: false},
	}
	result := formatRequiredParameters(params)
	if !strings.Contains(result, "ip") || !strings.Contains(result, "cidr") {
		t.Errorf("expected ip and cidr in result, got %q", result)
	}
	if strings.Contains(result, "timeout") {
		t.Errorf("optional param timeout should not be in required list, got %q", result)
	}
}

func TestFormatExpectedParameters(t *testing.T) {
	params := []plugin.PluginParameter{
		{Name: "ip", Type: "string", Required: true},
		{Name: "timeout", Type: "int", Required: false},
	}
	result := formatExpectedParameters(params)
	if !strings.Contains(result, "ip") || !strings.Contains(result, "timeout") {
		t.Errorf("expected both params in result, got %q", result)
	}
	if !strings.Contains(result, "[optional]") {
		t.Errorf("expected [optional] marker for optional param, got %q", result)
	}
}

func TestIsArgumentTypeCompatible(t *testing.T) {
	tests := []struct {
		arg      *PluginArg
		argType  string
		expected bool
	}{
		{nil, "string", false},
		{&PluginArg{Type: 2}, "string", true},
		{&PluginArg{Type: 1, Value: "field"}, "string", true},
		{&PluginArg{Type: 0, Value: "hello"}, "string", true},
		{&PluginArg{Type: 0, Value: "hello"}, "int", false},
		{&PluginArg{Type: 0, Value: 42}, "int", true},
		{&PluginArg{Type: 0, Value: 3.14}, "float", true},
		{&PluginArg{Type: 0, Value: 42}, "float", true},
		{&PluginArg{Type: 0, Value: true}, "bool", true},
		{&PluginArg{Type: 0, Value: "x"}, "interface{}", true},
		{&PluginArg{Type: 0, Value: "x"}, "[]string", true},
		{&PluginArg{Type: 0, Value: "x"}, "unknowntype", true},
	}
	for _, tc := range tests {
		result := isArgumentTypeCompatible(tc.arg, tc.argType)
		if result != tc.expected {
			t.Errorf("isArgumentTypeCompatible(type=%d,val=%v, %q) = %v, want %v",
				tc.arg.Type, tc.arg.Value, tc.argType, result, tc.expected)
		}
	}
}
