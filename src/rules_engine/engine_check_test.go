package rules_engine

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// LocalCacheFRQSum
// ---------------------------------------------------------------------------

func newTestRuleset() *Ruleset {
	return &Ruleset{
		Cache:            newLocalThresholdCounter(),
		CacheForClassify: newLocalClassifyCounter(),
	}
}

func TestLocalCacheFRQSum_FirstCall(t *testing.T) {
	r := newTestRuleset()
	hit, err := r.LocalCacheFRQSum("key1", 3, 60, 10)
	if err != nil || hit {
		t.Fatalf("expected (false, nil), got (%v, %v)", hit, err)
	}
	v, ok := r.Cache.Get("key1")
	if !ok || v != 3 {
		t.Fatalf("expected stored value 3, got %d ok=%v", v, ok)
	}
}

func TestLocalCacheFRQSum_Accumulate(t *testing.T) {
	r := newTestRuleset()
	for i := 0; i < 3; i++ {
		hit, _ := r.LocalCacheFRQSum("key1", 2, 60, 10)
		if hit {
			t.Fatalf("unexpected hit on iteration %d", i)
		}
	}
	v, _ := r.Cache.Get("key1")
	if v != 6 {
		t.Fatalf("expected accumulated value 6, got %d", v)
	}
}

func TestLocalCacheFRQSum_ExceedsThreshold(t *testing.T) {
	r := newTestRuleset()
	// threshold=5; add 3 then 3 → 6 > 5 → hit
	r.LocalCacheFRQSum("key1", 3, 60, 5)
	hit, err := r.LocalCacheFRQSum("key1", 3, 60, 5)
	if err != nil || !hit {
		t.Fatalf("expected (true, nil), got (%v, %v)", hit, err)
	}
	// Key should be deleted after trigger
	_, ok := r.Cache.Get("key1")
	if ok {
		t.Fatal("key should be deleted after threshold exceeded")
	}
}

func TestLocalCacheFRQSum_ExactThresholdNotTriggered(t *testing.T) {
	r := newTestRuleset()
	// threshold=5; add 5 → 5 is NOT > 5 → no hit
	hit, _ := r.LocalCacheFRQSum("key1", 5, 60, 5)
	if hit {
		t.Fatal("expected no hit when sum equals threshold")
	}
}

func TestLocalCacheFRQSum_ResetAfterExpiry(t *testing.T) {
	r := newTestRuleset()
	r.LocalCacheFRQSum("key1", 4, 0 /* 0s TTL — expires immediately */, 10)
	time.Sleep(5 * time.Millisecond)
	// After expiry the next call should restart from sumData, not accumulate
	hit, _ := r.LocalCacheFRQSum("key1", 4, 60, 10)
	if hit {
		t.Fatal("should not hit after expiry reset")
	}
	v, _ := r.Cache.Get("key1")
	if v != 4 {
		t.Fatalf("expected fresh value 4 after expiry, got %d", v)
	}
}

func TestLocalCacheFRQSum_TTLPreservedOnAccumulate(t *testing.T) {
	r := newTestRuleset()
	r.LocalCacheFRQSum("key1", 1, 60, 100)
	ttlBefore, _ := r.Cache.GetTTL("key1")

	// Small sleep so TTL decreases slightly
	time.Sleep(2 * time.Millisecond)
	r.LocalCacheFRQSum("key1", 1, 60, 100)
	ttlAfter, _ := r.Cache.GetTTL("key1")

	// TTL should be preserved (roughly), not reset to full 60s
	if ttlAfter > ttlBefore {
		t.Fatalf("TTL should not increase on accumulate: before=%v after=%v", ttlBefore, ttlAfter)
	}
}

// ---------------------------------------------------------------------------
// LocalCacheFRQClassify
// ---------------------------------------------------------------------------

func TestLocalCacheFRQClassify_FirstCall(t *testing.T) {
	r := newTestRuleset()
	hit, err := r.LocalCacheFRQClassify("uid:a:evt:login", "uid:a", 60, 3)
	if err != nil || hit {
		t.Fatalf("expected (false, nil), got (%v, %v)", hit, err)
	}
}

func TestLocalCacheFRQClassify_AccumulateDistinct(t *testing.T) {
	r := newTestRuleset()
	// threshold=3; add 3 distinct tmpKeys — should not hit (3 is not > 3)
	for i, ev := range []string{"login", "read", "write"} {
		hit, _ := r.LocalCacheFRQClassify("uid:a:evt:"+ev, "uid:a", 60, 3)
		if hit {
			t.Fatalf("unexpected hit on event %d (%s)", i, ev)
		}
	}
}

func TestLocalCacheFRQClassify_ExceedsThreshold(t *testing.T) {
	r := newTestRuleset()
	// threshold=3; 4th distinct key should trigger
	events := []string{"login", "read", "write", "delete"}
	var hit bool
	for _, ev := range events {
		var err error
		hit, err = r.LocalCacheFRQClassify("uid:b:evt:"+ev, "uid:b", 60, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hit {
			break
		}
	}
	if !hit {
		t.Fatal("expected threshold to be exceeded after 4 distinct events")
	}
	// classify key should be gone after trigger
	_, ok := r.CacheForClassify.Get("uid:b")
	if ok {
		t.Fatal("classify key should be deleted after threshold exceeded")
	}
}

func TestLocalCacheFRQClassify_DuplicateKeyNotCounted(t *testing.T) {
	r := newTestRuleset()
	// Send same tmpKey 5 times — should never exceed threshold=2 because count stays 1
	for i := 0; i < 5; i++ {
		hit, _ := r.LocalCacheFRQClassify("uid:c:evt:login", "uid:c", 60, 2)
		if hit {
			t.Fatalf("duplicate key incorrectly counted on iteration %d", i)
		}
	}
}

// ---------------------------------------------------------------------------
// NCS_EQU / NCS_NEQ
// ---------------------------------------------------------------------------

func TestNCS_EQU_SameCase(t *testing.T) {
	res, _ := NCS_EQU("hello", "hello")
	if !res {
		t.Fatal("NCS_EQU: same-case strings should match")
	}
}

func TestNCS_EQU_DifferentCase(t *testing.T) {
	res, _ := NCS_EQU("Hello", "HELLO")
	if !res {
		t.Fatal("NCS_EQU: case-insensitive should match")
	}
}

func TestNCS_EQU_NoMatch(t *testing.T) {
	res, _ := NCS_EQU("foo", "bar")
	if res {
		t.Fatal("NCS_EQU: different strings should not match")
	}
}

func TestNCS_NEQ_SameCase(t *testing.T) {
	res, _ := NCS_NEQ("hello", "hello")
	if res {
		t.Fatal("NCS_NEQ: equal strings should return false")
	}
}

func TestNCS_NEQ_DifferentCase(t *testing.T) {
	res, _ := NCS_NEQ("Hello", "HELLO")
	if res {
		t.Fatal("NCS_NEQ: case-insensitive equal strings should return false")
	}
}

func TestNCS_NEQ_Different(t *testing.T) {
	res, _ := NCS_NEQ("foo", "bar")
	if !res {
		t.Fatal("NCS_NEQ: different strings should return true")
	}
}

// ---------------------------------------------------------------------------
// ExprASTResult — short-circuit AND / OR
// ---------------------------------------------------------------------------

func makeAST(expr string) *ReCepAST {
	return GetAST(expr)
}

func TestExprASTResult_AND_TrueTrue(t *testing.T) {
	a := makeAST("a and b")
	tok := map[string]bool{"a": true, "b": true}
	if !a.ExprASTResult(a.ExprAST, tok) {
		t.Fatal("T AND T should be true")
	}
}

func TestExprASTResult_AND_TrueFalse(t *testing.T) {
	a := makeAST("a and b")
	tok := map[string]bool{"a": true, "b": false}
	if a.ExprASTResult(a.ExprAST, tok) {
		t.Fatal("T AND F should be false")
	}
}

func TestExprASTResult_AND_FalseTrue(t *testing.T) {
	a := makeAST("a and b")
	tok := map[string]bool{"a": false, "b": true}
	if a.ExprASTResult(a.ExprAST, tok) {
		t.Fatal("F AND T should be false (short-circuit)")
	}
}

func TestExprASTResult_AND_FalseFalse(t *testing.T) {
	a := makeAST("a and b")
	tok := map[string]bool{"a": false, "b": false}
	if a.ExprASTResult(a.ExprAST, tok) {
		t.Fatal("F AND F should be false")
	}
}

func TestExprASTResult_OR_TrueTrue(t *testing.T) {
	a := makeAST("a or b")
	tok := map[string]bool{"a": true, "b": true}
	if !a.ExprASTResult(a.ExprAST, tok) {
		t.Fatal("T OR T should be true")
	}
}

func TestExprASTResult_OR_TrueFalse(t *testing.T) {
	a := makeAST("a or b")
	tok := map[string]bool{"a": true, "b": false}
	if !a.ExprASTResult(a.ExprAST, tok) {
		t.Fatal("T OR F should be true (short-circuit)")
	}
}

func TestExprASTResult_OR_FalseTrue(t *testing.T) {
	a := makeAST("a or b")
	tok := map[string]bool{"a": false, "b": true}
	if !a.ExprASTResult(a.ExprAST, tok) {
		t.Fatal("F OR T should be true")
	}
}

func TestExprASTResult_OR_FalseFalse(t *testing.T) {
	a := makeAST("a or b")
	tok := map[string]bool{"a": false, "b": false}
	if a.ExprASTResult(a.ExprAST, tok) {
		t.Fatal("F OR F should be false")
	}
}

func TestExprASTResult_NOT_True(t *testing.T) {
	a := makeAST("not a")
	tok := map[string]bool{"a": true}
	if a.ExprASTResult(a.ExprAST, tok) {
		t.Fatal("NOT T should be false")
	}
}

func TestExprASTResult_NOT_False(t *testing.T) {
	a := makeAST("not a")
	tok := map[string]bool{"a": false}
	if !a.ExprASTResult(a.ExprAST, tok) {
		t.Fatal("NOT F should be true")
	}
}

func TestExprASTResult_Complex(t *testing.T) {
	// (a AND b) OR (c AND d)
	tests := []struct {
		a, b, c, d bool
		want       bool
	}{
		{true, true, false, false, true},
		{false, false, true, true, true},
		{false, false, false, false, false},
		{true, false, false, true, false},
		{true, true, true, true, true},
	}
	ast := makeAST("(a and b) or (c and d)")
	for _, tc := range tests {
		tok := map[string]bool{"a": tc.a, "b": tc.b, "c": tc.c, "d": tc.d}
		got := ast.ExprASTResult(ast.ExprAST, tok)
		if got != tc.want {
			t.Errorf("(a=%v AND b=%v) OR (c=%v AND d=%v): want %v got %v",
				tc.a, tc.b, tc.c, tc.d, tc.want, got)
		}
	}
}

func TestExprASTResult_NestedNot(t *testing.T) {
	// NOT (a AND b)
	ast := makeAST("not (a and b)")
	tests := []struct {
		a, b bool
		want bool
	}{
		{true, true, false},
		{true, false, true},
		{false, true, true},
		{false, false, true},
	}
	for _, tc := range tests {
		tok := map[string]bool{"a": tc.a, "b": tc.b}
		got := ast.ExprASTResult(ast.ExprAST, tok)
		if got != tc.want {
			t.Errorf("NOT(a=%v AND b=%v): want %v got %v", tc.a, tc.b, tc.want, got)
		}
	}
}
