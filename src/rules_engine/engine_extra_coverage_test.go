package rules_engine

import (
	"testing"
)

// ============================================================
// regex_result_cache.go coverage
// ============================================================

func TestRegexResultCache_GetPut(t *testing.T) {
	cache := NewRegexResultCache(10)

	// Miss on empty cache
	_, found := cache.Get(`^\d+$`, "123")
	if found {
		t.Fatal("expected cache miss on empty cache")
	}

	// Put and then get
	cache.Put(`^\d+$`, "123", RegexMatchResult{matched: true})
	result, found := cache.Get(`^\d+$`, "123")
	if !found {
		t.Fatal("expected cache hit after Put")
	}
	if !result.matched {
		t.Error("expected matched=true")
	}
}

func TestRegexResultCache_UpdateExisting(t *testing.T) {
	cache := NewRegexResultCache(10)
	cache.Put(`^a$`, "a", RegexMatchResult{matched: true})
	// Update with different result
	cache.Put(`^a$`, "a", RegexMatchResult{matched: false})
	result, found := cache.Get(`^a$`, "a")
	if !found || result.matched {
		t.Error("expected updated entry matched=false")
	}
}

func TestRegexResultCache_LRUEviction(t *testing.T) {
	cache := NewRegexResultCache(2)
	cache.Put(`^a$`, "a", RegexMatchResult{matched: true})
	cache.Put(`^b$`, "b", RegexMatchResult{matched: true})
	// Access a to make it MRU
	cache.Get(`^a$`, "a")
	// Add c — should evict b (LRU)
	cache.Put(`^c$`, "c", RegexMatchResult{matched: true})

	if cache.Size() != 2 {
		t.Errorf("expected size 2 after eviction, got %d", cache.Size())
	}
}

func TestRegexResultCache_Size(t *testing.T) {
	cache := NewRegexResultCache(5)
	if cache.Size() != 0 {
		t.Error("expected empty cache size 0")
	}
	cache.Put(`^x$`, "x", RegexMatchResult{matched: false})
	if cache.Size() != 1 {
		t.Errorf("expected size 1, got %d", cache.Size())
	}
}

func TestRegexResultCache_Clear(t *testing.T) {
	cache := NewRegexResultCache(5)
	cache.Put(`^x$`, "x", RegexMatchResult{matched: true})
	cache.Clear()
	if cache.Size() != 0 {
		t.Errorf("expected 0 after clear, got %d", cache.Size())
	}
}

func TestCachedRegexMatch_FromRawSkipsCache(t *testing.T) {
	cache := NewRegexResultCache(10)
	// isFromRaw=true should bypass cache
	matched, err := CachedRegexMatch(cache, `^\d+$`, "123", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Error("expected match for digits-only string")
	}
	// Cache should still be empty since isFromRaw=true
	if cache.Size() != 0 {
		t.Error("expected cache to remain empty for raw values")
	}
}

func TestCachedRegexMatch_StaticCaches(t *testing.T) {
	cache := NewRegexResultCache(10)
	// First call — cache miss, compiles and caches
	matched, err := CachedRegexMatch(cache, `^hello$`, "hello", false)
	if err != nil || !matched {
		t.Fatalf("expected match: %v %v", matched, err)
	}
	// Second call — should hit cache
	matched2, err2 := CachedRegexMatch(cache, `^hello$`, "hello", false)
	if err2 != nil || !matched2 {
		t.Fatalf("expected cache hit match: %v %v", matched2, err2)
	}
}

func TestCachedRegexMatch_NoMatch(t *testing.T) {
	cache := NewRegexResultCache(10)
	matched, err := CachedRegexMatch(cache, `^\d+$`, "abc", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched {
		t.Error("expected no match for non-digit string")
	}
}

func TestCachedRegexMatch_InvalidPattern(t *testing.T) {
	cache := NewRegexResultCache(10)
	_, err := CachedRegexMatch(cache, `[invalid`, "test", false)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestCachedRegexMatch_NilCache(t *testing.T) {
	// Should still work with nil cache (no caching)
	matched, err := CachedRegexMatch(nil, `^\w+$`, "hello", false)
	if err != nil {
		t.Fatalf("unexpected error with nil cache: %v", err)
	}
	if !matched {
		t.Error("expected match with nil cache")
	}
}

func TestCachedRegexMatchWithPrecompiled_Matches(t *testing.T) {
	cache := NewRegexResultCache(10)
	re, err := GetCompiledRegex(`^\d{3}$`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	matched := CachedRegexMatchWithPrecompiled(cache, re, `^\d{3}$`, "123")
	if !matched {
		t.Error("expected match for 3-digit string")
	}
}

func TestCachedRegexMatchWithPrecompiled_NoMatch(t *testing.T) {
	cache := NewRegexResultCache(10)
	re, err := GetCompiledRegex(`^\d{3}$`)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	matched := CachedRegexMatchWithPrecompiled(cache, re, `^\d{3}$`, "12345")
	if matched {
		t.Error("expected no match for 5-digit string against 3-digit pattern")
	}
}

func TestGetRegexResultCacheStats_Nil(t *testing.T) {
	stats := GetRegexResultCacheStats(nil)
	if stats["size"] != 0 {
		t.Errorf("expected size=0 for nil cache, got %v", stats["size"])
	}
}

func TestGetRegexResultCacheStats_NonNil(t *testing.T) {
	cache := NewRegexResultCache(5)
	cache.Put(`^x$`, "x", RegexMatchResult{matched: true})
	stats := GetRegexResultCacheStats(cache)
	if stats["size"] != 1 {
		t.Errorf("expected size=1, got %v", stats["size"])
	}
	if stats["capacity"] != 5 {
		t.Errorf("expected capacity=5, got %v", stats["capacity"])
	}
}

func TestSetRegexResultCacheCapacity(t *testing.T) {
	cache := NewRegexResultCache(5)
	SetRegexResultCacheCapacity(cache, 20)
	stats := GetRegexResultCacheStats(cache)
	if stats["capacity"] != 20 {
		t.Errorf("expected capacity=20, got %v", stats["capacity"])
	}
}

func TestSetRegexResultCacheCapacity_Nil(t *testing.T) {
	// Should not panic
	SetRegexResultCacheCapacity(nil, 20)
}

func TestClearGlobalRegexResultCache(t *testing.T) {
	// Just verify it doesn't panic
	ClearGlobalRegexResultCache()
}

// ============================================================
// engine_utils.go — ISNULL and NOTNULL direct unit tests
// ============================================================

func TestISNULL_EmptyString(t *testing.T) {
	res, _ := ISNULL("", "")
	if !res {
		t.Error("expected ISNULL(\"\") = true")
	}
}

func TestISNULL_NonEmpty(t *testing.T) {
	res, _ := ISNULL("value", "")
	if res {
		t.Error("expected ISNULL(\"value\") = false")
	}
}

func TestNOTNULL_NonEmpty(t *testing.T) {
	res, _ := NOTNULL("value", "")
	if !res {
		t.Error("expected NOTNULL(\"value\") = true")
	}
}

func TestNOTNULL_Empty(t *testing.T) {
	res, _ := NOTNULL("", "")
	if res {
		t.Error("expected NOTNULL(\"\") = false")
	}
}

func TestNOTNULL_Whitespace(t *testing.T) {
	res, _ := NOTNULL("   ", "")
	if res {
		t.Error("expected NOTNULL(\"   \") = false (whitespace-only)")
	}
}

// ============================================================
// engine_condition.go — toStr methods on ExprAST types
// ============================================================

func TestExprASTToStr_Number(t *testing.T) {
	n := NumberExprAST{Val: "abc"}
	s := n.toStr()
	if s == "" {
		t.Error("expected non-empty toStr for NumberExprAST")
	}
}

func TestExprASTToStr_Binary(t *testing.T) {
	b := BinaryExprAST{
		Op:  "&",
		Lhs: NumberExprAST{Val: "a"},
		Rhs: NumberExprAST{Val: "b"},
	}
	s := b.toStr()
	if s == "" {
		t.Error("expected non-empty toStr for BinaryExprAST")
	}
}

func TestExprASTToStr_Unary(t *testing.T) {
	u := UnaryExprAST{
		Op:      "!",
		Operand: NumberExprAST{Val: "c"},
	}
	s := u.toStr()
	if s == "" {
		t.Error("expected non-empty toStr for UnaryExprAST")
	}
}

// ============================================================
// engine_utils.go — convertPluginArgument and GetCheckDataWithTypeFromCache
// via plugin append with field-reference argument
// ============================================================

func TestAppend_PluginWithFieldReference(t *testing.T) {
	// Use unquoted field name (not _$ prefix) to trigger Type=1 PluginArg
	// which calls GetCheckDataWithTypeFromCache and convertPluginArgument
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="append-fieldref2">
  <rule id="r1" name="r1">
    <check type="EQU" field="op">hash</check>
    <append type="PLUGIN" field="hash_result">hashMD5(payload)</append>
  </rule>
</root>`)

	out := rs.EngineCheck(map[string]interface{}{
		"op":      "hash",
		"payload": "hello world",
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	hash, ok := out[0]["hash_result"].(string)
	if !ok || hash == "" {
		t.Errorf("expected non-empty hash string, got %v", out[0]["hash_result"])
	}
}

// ============================================================
// engine_core.go — executeIteratorThreshold via EngineCheck
// ============================================================

func TestIterator_WithThreshold_FiresAfterExceed(t *testing.T) {
	rs := buildRulesetFromXML(t, `
<root type="DETECTION" name="iter-threshold-fire">
  <rule id="r1" name="r1">
    <iterator type="ANY" field="ports" variable="port">
      <check type="MT" field="port">1024</check>
      <threshold group_by="port" range="10s" local_cache="true">2</threshold>
    </iterator>
  </rule>
</root>`)
	t.Cleanup(func() { rs.cleanup() })

	// First call — threshold not exceeded (count=1, need >2)
	out1 := rs.EngineCheck(map[string]interface{}{
		"ports": []interface{}{"8080"},
	})
	if len(out1) != 0 {
		t.Fatalf("expected 0 results on first call, got %d", len(out1))
	}

	// Second call — threshold not exceeded (count=2, need >2)
	out2 := rs.EngineCheck(map[string]interface{}{
		"ports": []interface{}{"8080"},
	})
	if len(out2) != 0 {
		t.Fatalf("expected 0 results on second call, got %d", len(out2))
	}

	// Third call — threshold exceeded (count=3, > 2)
	out3 := rs.EngineCheck(map[string]interface{}{
		"ports": []interface{}{"8080"},
	})
	if len(out3) != 1 {
		t.Fatalf("expected 1 result on third call (threshold exceeded), got %d", len(out3))
	}
}

// ============================================================
// generateCacheKey — direct test
// ============================================================

func TestGenerateCacheKey_Deterministic(t *testing.T) {
	k1 := generateCacheKey(`^\d+$`, "123")
	k2 := generateCacheKey(`^\d+$`, "123")
	if k1 != k2 {
		t.Error("expected same key for same inputs")
	}
	k3 := generateCacheKey(`^\d+$`, "456")
	if k1 == k3 {
		t.Error("expected different keys for different inputs")
	}
}
