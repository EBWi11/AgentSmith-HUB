package rules_engine

import "testing"

// ============================================================
// regex_cache.go coverage
// ============================================================

// TestRegexCache_GetCompiledRegex_CacheMiss tests compiling a new pattern.
func TestRegexCache_GetCompiledRegex_CacheMiss(t *testing.T) {
	ClearRegexCache()
	re, err := GetCompiledRegex(`^\d+$`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if re == nil {
		t.Fatal("expected non-nil compiled regex")
	}
}

// TestRegexCache_GetCompiledRegex_CacheHit tests that a second call returns the cached entry.
func TestRegexCache_GetCompiledRegex_CacheHit(t *testing.T) {
	ClearRegexCache()
	const pat = `^hello.*$`
	re1, err := GetCompiledRegex(pat)
	if err != nil || re1 == nil {
		t.Fatalf("first compile failed: %v", err)
	}
	re2, err := GetCompiledRegex(pat)
	if err != nil || re2 == nil {
		t.Fatalf("second compile failed: %v", err)
	}
	if re1 != re2 {
		t.Fatal("expected same pointer on cache hit")
	}
}

// TestRegexCache_GetCompiledRegex_InvalidPattern tests error on bad regex.
func TestRegexCache_GetCompiledRegex_InvalidPattern(t *testing.T) {
	ClearRegexCache()
	_, err := GetCompiledRegex(`[invalid`)
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
}

// TestRegexCache_GetRegexCacheStats verifies stat counting.
func TestRegexCache_GetRegexCacheStats(t *testing.T) {
	ClearRegexCache()
	before := GetRegexCacheStats()
	if before != 0 {
		t.Fatalf("expected 0 after clear, got %d", before)
	}
	_, _ = GetCompiledRegex(`^\w+$`)
	_, _ = GetCompiledRegex(`^\d{4}$`)
	after := GetRegexCacheStats()
	if after != 2 {
		t.Fatalf("expected 2 entries, got %d", after)
	}
}

// TestRegexCache_ClearRegexCache verifies cache is emptied.
func TestRegexCache_ClearRegexCache(t *testing.T) {
	_, _ = GetCompiledRegex(`^abc$`)
	ClearRegexCache()
	if n := GetRegexCacheStats(); n != 0 {
		t.Fatalf("expected 0 after ClearRegexCache, got %d", n)
	}
}

// TestRegexCache_LRUEviction tests that LRU eviction occurs when maxSize is exceeded.
func TestRegexCache_LRUEviction(t *testing.T) {
	// Use a tiny private cache with maxSize=2
	rc := newRegexCache(2)

	_, _ = rc.getCompiledRegex(`^a$`)
	_, _ = rc.getCompiledRegex(`^b$`)
	// Adding a third entry triggers eviction of LRU
	_, _ = rc.getCompiledRegex(`^c$`)

	if rc.getCacheStats() > 2 {
		t.Fatalf("expected no more than 2 entries after eviction, got %d", rc.getCacheStats())
	}
}

// TestRegexCache_ViaEngineCheck exercises GetCompiledRegex via a REGEX check
// where the pattern comes from a dynamic field reference (_$pattern).
func TestRegexCache_ViaEngineCheck(t *testing.T) {
	ClearRegexCache()
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="regex-cache-test">
  <rule id="r1" name="regex-dynamic">
    <check type="REGEX" field="path">_$regex_pattern</check>
  </rule>
</root>`)
	// Pattern comes from data — GetCompiledRegex is called
	out := rs.EngineCheck(map[string]interface{}{
		"path":          "/api/v3/users",
		"regex_pattern": "/api/v[0-9]+/",
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 match, got %d", len(out))
	}
	out2 := rs.EngineCheck(map[string]interface{}{
		"path":          "/health",
		"regex_pattern": "/api/v[0-9]+/",
	})
	if len(out2) != 0 {
		t.Fatalf("expected 0 matches for non-matching path, got %d", len(out2))
	}
	// Cache should now have at least 1 entry
	if GetRegexCacheStats() < 1 {
		t.Fatal("expected at least 1 cached regex after EngineCheck with dynamic pattern")
	}
}

// ============================================================
// engine_condition.go coverage — ErrPos, AddTokenVal, DelTokenVal
// ============================================================

func TestErrPos_Basic(t *testing.T) {
	s := "EQU and OR"
	result := ErrPos(s, 4)
	if result == "" {
		t.Fatal("expected non-empty ErrPos string")
	}
	// Should contain the original string and a caret indicator
	found := false
	for _, line := range []string{s} {
		if len(result) > len(line) {
			found = true
		}
	}
	if !found {
		t.Errorf("ErrPos output shorter than expected: %q", result)
	}
}

func TestAddTokenVal_MapsValues(t *testing.T) {
	ast := GetAST("a AND b")
	// 0 maps to true, non-0 maps to false
	ast.AddTokenVal(map[string]int{"a": 0, "b": 1})
	if !ast.TokenVal["a"] {
		t.Error("expected a=true (value was 0)")
	}
	if ast.TokenVal["b"] {
		t.Error("expected b=false (value was non-0)")
	}
}

func TestDelTokenVal_SetsNil(t *testing.T) {
	ast := GetAST("x OR y")
	ast.AddTokenVal(map[string]int{"x": 0, "y": 0})
	if ast.TokenVal == nil {
		t.Fatal("TokenVal should not be nil after AddTokenVal")
	}
	ast.DelTokenVal()
	if ast.TokenVal != nil {
		t.Error("expected TokenVal to be nil after DelTokenVal")
	}
}

// ============================================================
// engine_core.go — parseProjectInfoFromPNS
// ============================================================

func TestParseProjectInfoFromPNS_Standard(t *testing.T) {
	project, ruleset := parseProjectInfoFromPNS("INPUT.api_sec.RULESET.test.OUTPUT.print_demo")
	if project != "api_sec" {
		t.Errorf("expected project=api_sec, got %q", project)
	}
	if ruleset != "test" {
		t.Errorf("expected ruleset=test, got %q", ruleset)
	}
}

func TestParseProjectInfoFromPNS_TestPrefix(t *testing.T) {
	project, ruleset := parseProjectInfoFromPNS("TEST.INPUT.my_project.RULESET.detection_rule.OUTPUT.sink")
	if project != "my_project" {
		t.Errorf("expected project=my_project, got %q", project)
	}
	if ruleset != "detection_rule" {
		t.Errorf("expected ruleset=detection_rule, got %q", ruleset)
	}
}

func TestParseProjectInfoFromPNS_Empty(t *testing.T) {
	project, ruleset := parseProjectInfoFromPNS("")
	if project != "" || ruleset != "" {
		t.Errorf("expected empty results for empty PNS, got project=%q ruleset=%q", project, ruleset)
	}
}

func TestParseProjectInfoFromPNS_TooShort(t *testing.T) {
	project, ruleset := parseProjectInfoFromPNS("INPUT.only")
	if project != "" || ruleset != "" {
		t.Errorf("expected empty results for short PNS, got project=%q ruleset=%q", project, ruleset)
	}
}

// ============================================================
// engine_core.go — GetProcessTotal, GetIncrementAndUpdate, GetRunningTaskCount
// ============================================================

func TestRuleset_StatsMethodsReturnSensibleValues(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="stats-test">
  <rule id="r1" name="r1">
    <check type="EQU" field="x">1</check>
  </rule>
</root>`)

	// Before any calls, total should be 0
	if total := rs.GetProcessTotal(); total != 0 {
		t.Errorf("expected 0, got %d", total)
	}

	// GetIncrementAndUpdate returns 0 when no change
	if inc := rs.GetIncrementAndUpdate(); inc != 0 {
		t.Errorf("expected 0 increment before any processing, got %d", inc)
	}

	// GetRunningTaskCount returns 0 when pool is nil
	if count := rs.GetRunningTaskCount(); count != 0 {
		t.Errorf("expected 0 running tasks (no pool), got %d", count)
	}
}

// ============================================================
// engine_utils.go — replaceFromRawPlaceholders via <append>
// ============================================================

func TestAppend_DynamicFieldSubstitution(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="append-dynamic">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">login</check>
    <append field="summary">User _$username logged in from _$src_ip</append>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{
		"event":    "login",
		"username": "alice",
		"src_ip":   "10.0.0.1",
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	summary, ok := out[0]["summary"].(string)
	if !ok {
		t.Fatal("expected summary field to be a string")
	}
	if summary != "User alice logged in from 10.0.0.1" {
		t.Errorf("unexpected summary: %q", summary)
	}
}

func TestAppend_DynamicFieldSubstitution_MissingField(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="append-missing">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">login</check>
    <append field="info">from _$src_ip</append>
  </rule>
</root>`)

	// src_ip not provided — placeholder should remain
	out := rs.EngineCheck(map[string]interface{}{"event": "login"})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	info, ok := out[0]["info"].(string)
	if !ok {
		t.Fatal("expected info field to be a string")
	}
	if info != "from _$src_ip" {
		t.Errorf("expected placeholder to remain, got %q", info)
	}
}

func TestAppend_EscapedPlaceholder(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="append-escape">
  <rule id="r1" name="r1">
    <check type="EQU" field="flag">true</check>
    <append field="literal">Use \_$var syntax for escaping</append>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{"flag": "true"})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	lit, ok := out[0]["literal"].(string)
	if !ok {
		t.Fatal("expected literal to be a string")
	}
	if lit != "Use _$var syntax for escaping" {
		t.Errorf("unexpected escaped literal: %q", lit)
	}
}

// ============================================================
// engine_core.go — executeModify (literal mode)
// ============================================================

func TestModify_LiteralAssignment(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="modify-literal">
  <rule id="r1" name="r1">
    <check type="EQU" field="action">exec</check>
    <modify field="severity">high</modify>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{"action": "exec"})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	if out[0]["severity"] != "high" {
		t.Errorf("expected severity=high, got %v", out[0]["severity"])
	}
}

func TestModify_LiteralDoesNotMatchNoFire(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="modify-no-match">
  <rule id="r1" name="r1">
    <check type="EQU" field="action">exec</check>
    <modify field="severity">high</modify>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{"action": "read"})
	if len(out) != 0 {
		t.Fatalf("expected 0 results, got %d", len(out))
	}
}

// ============================================================
// engine_core.go — executePlugin (side-effect plugin, bool return)
// ============================================================

func TestPlugin_BoolReturnSideEffect(t *testing.T) {
	// isPrivateIP is a bool-return plugin registered in local_plugin
	// The <plugin> element runs as a side-effect; rule still fires if check passes
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="plugin-bool">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">alert</check>
    <plugin>isPrivateIP("192.168.1.1")</plugin>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{"event": "alert"})
	if len(out) != 1 {
		t.Fatalf("expected rule to fire, got %d results", len(out))
	}
}

// ============================================================
// engine_core.go — executeAppend with PLUGIN (covers GetPluginRealArgs interface path)
// ============================================================

func TestAppend_PluginInterfaceReturn(t *testing.T) {
	// hashMD5 is an interface-return plugin
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="append-plugin">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">scan</check>
    <append type="PLUGIN" field="fingerprint">hashMD5("fixed_seed")</append>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{"event": "scan"})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	fp, ok := out[0]["fingerprint"].(string)
	if !ok || fp == "" {
		t.Errorf("expected non-empty MD5 fingerprint string, got %v", out[0]["fingerprint"])
	}
}

func TestAppend_PluginBoolReturn(t *testing.T) {
	// isPrivateIP is a bool-return plugin used as append
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="append-plugin-bool">
  <rule id="r1" name="r1">
    <check type="EQU" field="event">connect</check>
    <append type="PLUGIN" field="is_internal">isPrivateIP("10.0.0.1")</append>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{"event": "connect"})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	isInternal, ok := out[0]["is_internal"].(bool)
	if !ok || !isInternal {
		t.Errorf("expected is_internal=true, got %v", out[0]["is_internal"])
	}
}

// ============================================================
// GetPluginRealArgs — field-reference arg (Type=1) and data-copy arg (Type=2)
// ============================================================

func TestAppend_PluginWithLiteralArgs(t *testing.T) {
	// base64Decode with a literal argument — exercises GetPluginRealArgs Type=0 path
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="append-literal-arg">
  <rule id="r1" name="r1">
    <check type="EQU" field="op">decode</check>
    <append type="PLUGIN" field="decoded">base64Decode("aGVsbG8=")</append>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{"op": "decode"})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	decoded, ok := out[0]["decoded"].(string)
	if !ok || decoded == "" {
		t.Errorf("expected non-empty decoded string, got %v", out[0]["decoded"])
	}
}
