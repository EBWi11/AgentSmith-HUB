package rules_engine

import (
	"AgentSmith-HUB/common"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto/v2"
)

// ============================================================================
// Shared Test Helpers
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

func buildTestRuleset(tb testing.TB, xmlContent string) *Ruleset {
	tb.Helper()
	ruleset, err := ParseRuleset([]byte(xmlContent))
	if err != nil {
		tb.Fatalf("ParseRuleset failed: %v", err)
	}
	ruleset.RulesetID = "test_ruleset"
	ruleset.IsDetection = true

	err = RulesetBuild(ruleset)
	if err != nil {
		tb.Fatalf("RulesetBuild failed: %v", err)
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

	tb.Cleanup(func() {
		if ruleset != nil {
			ruleset.cleanup()
		}
	})

	return ruleset
}

func getFirstSequenceFromRuleset(t *testing.T, rs *Ruleset) Sequence {
	t.Helper()
	if rs == nil || len(rs.Rules) == 0 {
		t.Fatal("ruleset is empty")
	}
	for _, seq := range rs.Rules[0].SequenceMap {
		return seq
	}
	t.Fatal("sequence not found")
	return Sequence{}
}

func assertStage(t *testing.T, stage CEPStage, expectedAbsent bool, expectedEventIDs []string) {
	t.Helper()
	if stage.IsAbsent != expectedAbsent {
		t.Errorf("expected IsAbsent=%v, got %v", expectedAbsent, stage.IsAbsent)
	}
	if len(stage.EventIDs) != len(expectedEventIDs) {
		t.Errorf("expected %d event IDs %v, got %d: %v", len(expectedEventIDs), expectedEventIDs, len(stage.EventIDs), stage.EventIDs)
		return
	}
	eventMap := make(map[string]bool)
	for _, id := range stage.EventIDs {
		eventMap[id] = true
	}
	for _, expected := range expectedEventIDs {
		if !eventMap[expected] {
			t.Errorf("missing expected event ID: %s (got %v)", expected, stage.EventIDs)
		}
	}
}

func assertIntSlice(t *testing.T, got, expected []int) {
	t.Helper()
	if len(got) != len(expected) {
		t.Errorf("expected %v, got %v", expected, got)
		return
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("at index %d: expected %d, got %d (full: expected %v, got %v)", i, expected[i], got[i], expected, got)
			return
		}
	}
}

// ############################################################################
//
//  PART 1: Condition — Tokenizer / Parser / Evaluator / Completion
//
// ############################################################################

// ============================================================================
// Tokenizer Tests
// ============================================================================

func TestTokenizeCEPCondition_Simple(t *testing.T) {
	tokens, literals, err := tokenizeCEPCondition("a -> b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
	if tokens[0].Data != "a" || tokens[0].Type != CEPTokenLiteral {
		t.Errorf("token 0: expected literal 'a', got %+v", tokens[0])
	}
	if tokens[1].Data != CEPOpSequence || tokens[1].Type != CEPTokenOperator {
		t.Errorf("token 1: expected '->', got %+v", tokens[1])
	}
	if tokens[2].Data != "b" || tokens[2].Type != CEPTokenLiteral {
		t.Errorf("token 2: expected literal 'b', got %+v", tokens[2])
	}
	if !literals["a"] || !literals["b"] {
		t.Errorf("expected literals {a, b}, got %v", literals)
	}
}

func TestTokenizeCEPCondition_Complex(t *testing.T) {
	tokens, _, err := tokenizeCEPCondition("(a and b) -> !c -> (d or e)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ( a & b ) -> ! c -> ( d | e )
	expected := []struct {
		data string
		typ  int
	}{
		{"(", CEPTokenOperator},
		{"a", CEPTokenLiteral},
		{CEPOpAnd, CEPTokenOperator},
		{"b", CEPTokenLiteral},
		{")", CEPTokenOperator},
		{CEPOpSequence, CEPTokenOperator},
		{CEPOpNot, CEPTokenOperator},
		{"c", CEPTokenLiteral},
		{CEPOpSequence, CEPTokenOperator},
		{"(", CEPTokenOperator},
		{"d", CEPTokenLiteral},
		{CEPOpOr, CEPTokenOperator},
		{"e", CEPTokenLiteral},
		{")", CEPTokenOperator},
	}
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %+v", len(expected), len(tokens), tokens)
	}
	for i, exp := range expected {
		if tokens[i].Data != exp.data || tokens[i].Type != exp.typ {
			t.Errorf("token %d: expected {%s, %d}, got {%s, %d}", i, exp.data, exp.typ, tokens[i].Data, tokens[i].Type)
		}
	}
}

func TestTokenizeCEPCondition_NotKeyword(t *testing.T) {
	tokens, _, err := tokenizeCEPCondition("a -> not b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 4 {
		t.Fatalf("expected 4 tokens, got %d", len(tokens))
	}
	if tokens[2].Data != CEPOpNot {
		t.Errorf("expected '!' for 'not', got %s", tokens[2].Data)
	}
}

func TestTokenizeCEPCondition_Empty(t *testing.T) {
	_, _, err := tokenizeCEPCondition("")
	if err == nil {
		t.Fatal("expected error for empty expression")
	}
}

func TestTokenizeCEPCondition_InvalidEventID(t *testing.T) {
	_, _, err := tokenizeCEPCondition("a -> b.c")
	if err == nil {
		t.Fatal("expected error for invalid event ID 'b.c'")
	}
}

// ============================================================================
// Parser Tests
// ============================================================================

func TestParseCEPCondition_SimpleSequence(t *testing.T) {
	cond, err := ParseCEPCondition("a -> b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(cond.Stages))
	}
	assertStage(t, cond.Stages[0], false, []string{"a"})
	assertStage(t, cond.Stages[1], false, []string{"b"})
}

func TestParseCEPCondition_ThreeStepSequence(t *testing.T) {
	cond, err := ParseCEPCondition("a -> b -> c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.Stages) != 3 {
		t.Fatalf("expected 3 stages, got %d", len(cond.Stages))
	}
	assertStage(t, cond.Stages[0], false, []string{"a"})
	assertStage(t, cond.Stages[1], false, []string{"b"})
	assertStage(t, cond.Stages[2], false, []string{"c"})
}

func TestParseCEPCondition_BranchOr(t *testing.T) {
	cond, err := ParseCEPCondition("a -> (b or c)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(cond.Stages))
	}
	assertStage(t, cond.Stages[0], false, []string{"a"})
	assertStage(t, cond.Stages[1], false, []string{"b", "c"})
}

func TestParseCEPCondition_ComplexStage(t *testing.T) {
	cond, err := ParseCEPCondition("(a and b) -> c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(cond.Stages))
	}
	assertStage(t, cond.Stages[0], false, []string{"a", "b"})
	assertStage(t, cond.Stages[1], false, []string{"c"})
}

func TestParseCEPCondition_Absence(t *testing.T) {
	cond, err := ParseCEPCondition("a -> !b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(cond.Stages))
	}
	assertStage(t, cond.Stages[0], false, []string{"a"})
	assertStage(t, cond.Stages[1], true, []string{"b"})
}

func TestParseCEPCondition_AbsenceThenPresence(t *testing.T) {
	cond, err := ParseCEPCondition("a -> !b -> c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.Stages) != 3 {
		t.Fatalf("expected 3 stages, got %d", len(cond.Stages))
	}
	assertStage(t, cond.Stages[0], false, []string{"a"})
	assertStage(t, cond.Stages[1], true, []string{"b"})
	assertStage(t, cond.Stages[2], false, []string{"c"})
}

func TestParseCEPCondition_AbsenceWithGroup(t *testing.T) {
	cond, err := ParseCEPCondition("a -> !(b and c)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(cond.Stages))
	}
	assertStage(t, cond.Stages[0], false, []string{"a"})
	assertStage(t, cond.Stages[1], true, []string{"b", "c"})
}

func TestParseCEPCondition_NestedOrAnd(t *testing.T) {
	cond, err := ParseCEPCondition("a -> (b or (c and d))")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(cond.Stages))
	}
	assertStage(t, cond.Stages[0], false, []string{"a"})
	assertStage(t, cond.Stages[1], false, []string{"b", "c", "d"})
}

func TestParseCEPCondition_AllEventIDs(t *testing.T) {
	cond, err := ParseCEPCondition("login -> (priv_esc or data_access) -> !cleanup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := map[string]bool{"login": true, "priv_esc": true, "data_access": true, "cleanup": true}
	if len(cond.AllEvents) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(cond.AllEvents), cond.AllEvents)
	}
	for k := range expected {
		if !cond.AllEvents[k] {
			t.Errorf("missing event ID: %s", k)
		}
	}
}

func TestParseCEPCondition_HasAbsenceStages(t *testing.T) {
	cond1, _ := ParseCEPCondition("a -> b")
	if cond1.HasAbsenceStages() {
		t.Error("a -> b should not have absence stages")
	}

	cond2, _ := ParseCEPCondition("a -> !b")
	if !cond2.HasAbsenceStages() {
		t.Error("a -> !b should have absence stages")
	}
}

// Parser error cases

func TestParseCEPCondition_SingleEvent(t *testing.T) {
	_, err := ParseCEPCondition("a")
	if err == nil {
		t.Fatal("expected error for single event (no -> operator)")
	}
}

func TestParseCEPCondition_AbsenceFirst(t *testing.T) {
	_, err := ParseCEPCondition("!a -> b")
	if err == nil {
		t.Fatal("expected error when first stage is absence")
	}
}

func TestParseCEPCondition_EmptyExpr(t *testing.T) {
	_, err := ParseCEPCondition("")
	if err == nil {
		t.Fatal("expected error for empty expression")
	}
}

func TestParseCEPCondition_TrailingArrow(t *testing.T) {
	_, err := ParseCEPCondition("a -> b ->")
	if err == nil {
		t.Fatal("expected error for trailing ->")
	}
}

func TestParseCEPCondition_LeadingArrow(t *testing.T) {
	_, err := ParseCEPCondition("-> a -> b")
	if err == nil {
		t.Fatal("expected error for leading ->")
	}
}

func TestParseCEPCondition_UnbalancedParen(t *testing.T) {
	_, err := ParseCEPCondition("a -> (b or c")
	if err == nil {
		t.Fatal("expected error for unbalanced parentheses")
	}
}

func TestParseCEPCondition_DoubleArrow(t *testing.T) {
	_, err := ParseCEPCondition("a -> -> b")
	if err == nil {
		t.Fatal("expected error for double ->")
	}
}

// ============================================================================
// Stage Evaluation Tests (same-event and/or/! logic)
// ============================================================================

func TestEvaluateEvent_SimpleLiteral(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b")
	// Event matches 'a' only
	stages := cond.EvaluateEvent(map[string]bool{"a": true, "b": false})
	assertIntSlice(t, stages, []int{0})

	// Event matches 'b' only
	stages = cond.EvaluateEvent(map[string]bool{"a": false, "b": true})
	assertIntSlice(t, stages, []int{1})

	// Event matches both
	stages = cond.EvaluateEvent(map[string]bool{"a": true, "b": true})
	assertIntSlice(t, stages, []int{0, 1})

	// Event matches neither
	stages = cond.EvaluateEvent(map[string]bool{"a": false, "b": false})
	assertIntSlice(t, stages, []int{})
}

func TestEvaluateEvent_AndStage(t *testing.T) {
	cond, _ := ParseCEPCondition("(a and b) -> c")

	// Both a and b match -> stage 0 satisfied
	stages := cond.EvaluateEvent(map[string]bool{"a": true, "b": true, "c": false})
	assertIntSlice(t, stages, []int{0})

	// Only a matches -> stage 0 NOT satisfied
	stages = cond.EvaluateEvent(map[string]bool{"a": true, "b": false, "c": false})
	assertIntSlice(t, stages, []int{})

	// c matches -> stage 1 satisfied
	stages = cond.EvaluateEvent(map[string]bool{"a": false, "b": false, "c": true})
	assertIntSlice(t, stages, []int{1})
}

func TestEvaluateEvent_OrStage(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> (b or c)")

	// Only b matches -> stage 1 satisfied
	stages := cond.EvaluateEvent(map[string]bool{"a": false, "b": true, "c": false})
	assertIntSlice(t, stages, []int{1})

	// Only c matches -> stage 1 satisfied
	stages = cond.EvaluateEvent(map[string]bool{"a": false, "b": false, "c": true})
	assertIntSlice(t, stages, []int{1})

	// Neither b nor c -> stage 1 NOT satisfied
	stages = cond.EvaluateEvent(map[string]bool{"a": false, "b": false, "c": false})
	assertIntSlice(t, stages, []int{})
}

func TestEvaluateEvent_AbsenceStage(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b")

	// 'b' matches -> absence stage IS evaluated as true (the event matches 'b')
	// Note: EvaluateEvent reports raw match, absence logic is handled in CheckComplete
	stages := cond.EvaluateEvent(map[string]bool{"a": false, "b": true})
	assertIntSlice(t, stages, []int{1})

	// 'b' does not match -> absence stage NOT matched (no event to record)
	stages = cond.EvaluateEvent(map[string]bool{"a": false, "b": false})
	assertIntSlice(t, stages, []int{})
}

func TestEvaluateEvent_NestedOrAnd(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> (b or (c and d))")

	// b matches -> stage 1 satisfied
	stages := cond.EvaluateEvent(map[string]bool{"a": false, "b": true, "c": false, "d": false})
	assertIntSlice(t, stages, []int{1})

	// c and d both match -> stage 1 satisfied
	stages = cond.EvaluateEvent(map[string]bool{"a": false, "b": false, "c": true, "d": true})
	assertIntSlice(t, stages, []int{1})

	// only c matches -> stage 1 NOT satisfied
	stages = cond.EvaluateEvent(map[string]bool{"a": false, "b": false, "c": true, "d": false})
	assertIntSlice(t, stages, []int{})
}

// ============================================================================
// Sequence Completion Tests ("match first, check time later")
// ============================================================================

func TestCheckComplete_SimpleSequence(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b")

	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(1, StageMatch{Timestamp: 200})
	if !cond.CheckComplete(state) {
		t.Error("expected sequence to be complete (a@100, b@200)")
	}
}

func TestCheckComplete_WrongOrder(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b")

	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 200})
	state.AddMatch(1, StageMatch{Timestamp: 100})
	if cond.CheckComplete(state) {
		t.Error("expected sequence NOT complete (a@200, b@100 - wrong order)")
	}
}

func TestCheckComplete_OutOfOrderArrival(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b -> c")

	state := NewSequenceState(100, 1000)
	state.AddMatch(2, StageMatch{Timestamp: 300}) // c arrives first
	state.AddMatch(0, StageMatch{Timestamp: 100}) // a arrives second
	state.AddMatch(1, StageMatch{Timestamp: 200}) // b arrives third
	if !cond.CheckComplete(state) {
		t.Error("expected sequence complete (out-of-order arrival but valid timestamps)")
	}
}

func TestCheckComplete_Incomplete(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b -> c")

	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(2, StageMatch{Timestamp: 300})
	if cond.CheckComplete(state) {
		t.Error("expected sequence NOT complete (b not matched)")
	}
}

func TestCheckComplete_NilState(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b")
	if cond.CheckComplete(nil) {
		t.Error("expected false for nil state")
	}
}

func TestCheckComplete_EmptyState(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b")
	state := NewSequenceState(100, 1000)
	if cond.CheckComplete(state) {
		t.Error("expected false for empty state")
	}
}

func TestCheckComplete_SameTimestamp(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b")

	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(1, StageMatch{Timestamp: 100})
	if cond.CheckComplete(state) {
		t.Error("expected NOT complete (same timestamp, not strictly increasing)")
	}
}

func TestCheckComplete_MultipleMatchesPerStage(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b")

	state := NewSequenceState(50, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 50})
	state.AddMatch(0, StageMatch{Timestamp: 150})
	state.AddMatch(1, StageMatch{Timestamp: 100})
	state.AddMatch(1, StageMatch{Timestamp: 200})
	if !cond.CheckComplete(state) {
		t.Error("expected complete (multiple valid orderings exist)")
	}
}

func TestCheckComplete_MultipleMatchesNoValidOrder(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b -> c")

	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 300})
	state.AddMatch(1, StageMatch{Timestamp: 200})
	state.AddMatch(2, StageMatch{Timestamp: 100})
	if cond.CheckComplete(state) {
		t.Error("expected NOT complete (no valid temporal ordering)")
	}
}

// ============================================================================
// Absence Detection Tests
// ============================================================================

func TestCheckComplete_AbsenceNotObserved(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b")

	// Trailing absence requires timeout — CheckComplete must return false.
	state := NewSequenceState(100, 500)
	state.AddMatch(0, StageMatch{Timestamp: 100})

	if cond.CheckComplete(state) {
		t.Error("expected CheckComplete false for trailing absence (needs timeout)")
	}

	// Before timeout
	if cond.CheckAbsenceTimeout(state, 400) {
		t.Error("expected NOT triggered before timeout")
	}

	// After timeout — absence confirmed
	if !cond.CheckAbsenceTimeout(state, 600) {
		t.Error("expected triggered after timeout (b never appeared)")
	}
}

func TestCheckComplete_AbsenceObserved(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b")

	state := NewSequenceState(100, 500)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(1, StageMatch{Timestamp: 200}) // b was observed

	if cond.CheckAbsenceTimeout(state, 600) {
		t.Error("expected NOT triggered (b was observed)")
	}
}

func TestCheckComplete_AbsenceObservedBeforePrev(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b")

	// b@100 before a@200 — absence after a is still valid
	state := NewSequenceState(100, 500)
	state.AddMatch(0, StageMatch{Timestamp: 200})
	state.AddMatch(1, StageMatch{Timestamp: 100})

	if !cond.CheckAbsenceTimeout(state, 600) {
		t.Error("expected triggered (b@100 was before a@200, so absence after a is valid)")
	}
}

func TestCheckComplete_AbsenceThenPresence(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b -> c")

	// Middle absence bounded by c's arrival — should complete immediately.
	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(2, StageMatch{Timestamp: 300})

	if !cond.CheckComplete(state) {
		t.Error("expected complete (a -> [no b] -> c, absence bounded by c)")
	}
}

func TestCheckComplete_AbsenceThenPresenceViolated(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b -> c")

	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(1, StageMatch{Timestamp: 200}) // b observed
	state.AddMatch(2, StageMatch{Timestamp: 300})

	if cond.CheckComplete(state) {
		t.Error("expected NOT complete (b@200 observed between a@100 and c@300)")
	}
}

// ============================================================================
// Trailing Absence Fix Tests
// ============================================================================

func TestCheckComplete_TrailingAbsence_DoesNotTriggerImmediately(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b")

	state := NewSequenceState(100, 500)
	state.AddMatch(0, StageMatch{Timestamp: 100})

	if cond.CheckComplete(state) {
		t.Error("expected NOT complete: trailing absence must wait for timeout")
	}
}

func TestCheckComplete_TrailingDoubleAbsence_DoesNotTriggerImmediately(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b -> !c")

	state := NewSequenceState(100, 500)
	state.AddMatch(0, StageMatch{Timestamp: 100})

	if cond.CheckComplete(state) {
		t.Error("expected NOT complete: trailing absence chain must wait for timeout")
	}
}

func TestCheckComplete_PresenceThenTrailingAbsence(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b -> !c")

	state := NewSequenceState(100, 500)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(1, StageMatch{Timestamp: 200})

	if cond.CheckComplete(state) {
		t.Error("expected NOT complete: trailing !c must wait for timeout")
	}

	if !cond.CheckAbsenceTimeout(state, 600) {
		t.Error("expected triggered after timeout (c never appeared)")
	}
}

func TestCheckComplete_MiddleAbsence_StillCompletes(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b -> c")

	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(2, StageMatch{Timestamp: 300})

	if !cond.CheckComplete(state) {
		t.Error("expected complete: middle absence bounded by c")
	}
}

func TestCheckComplete_MiddleAbsenceThenTrailingAbsence(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b -> c -> !d")

	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(2, StageMatch{Timestamp: 300})

	if cond.CheckComplete(state) {
		t.Error("expected NOT complete: trailing !d needs timeout")
	}

	if !cond.CheckAbsenceTimeout(state, 1100) {
		t.Error("expected triggered after timeout")
	}
}

// ============================================================================
// FindCompletionPath Tests
// ============================================================================

func TestFindCompletionPath_SimpleSequence(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b")

	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(1, StageMatch{Timestamp: 200})

	path := cond.FindCompletionPath(state)
	if path == nil {
		t.Fatal("expected non-nil path for complete sequence")
	}
	if path.MatchIndices[0] != 0 {
		t.Errorf("expected stage 0 match index 0, got %d", path.MatchIndices[0])
	}
	if path.MatchIndices[1] != 0 {
		t.Errorf("expected stage 1 match index 0, got %d", path.MatchIndices[1])
	}
}

func TestFindCompletionPath_SkipsEarlyMatch(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b -> c")

	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(0, StageMatch{Timestamp: 300})
	state.AddMatch(1, StageMatch{Timestamp: 200})
	state.AddMatch(2, StageMatch{Timestamp: 150})
	state.AddMatch(2, StageMatch{Timestamp: 400})

	path := cond.FindCompletionPath(state)
	if path == nil {
		t.Fatal("expected non-nil path")
	}
	if path.MatchIndices[0] != 0 {
		t.Errorf("expected stage 0 index 0, got %d", path.MatchIndices[0])
	}
	if path.MatchIndices[1] != 0 {
		t.Errorf("expected stage 1 index 0, got %d", path.MatchIndices[1])
	}
	if path.MatchIndices[2] != 1 {
		t.Errorf("expected stage 2 index 1 (T=400), got %d", path.MatchIndices[2])
	}
}

func TestFindCompletionPath_TrailingAbsence_ReturnsNil(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b")

	state := NewSequenceState(100, 500)
	state.AddMatch(0, StageMatch{Timestamp: 100})

	path := cond.FindCompletionPath(state)
	if path != nil {
		t.Error("expected nil path for trailing absence (needs timeout)")
	}
}

func TestFindAbsenceCompletionPath_Basic(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b")

	state := NewSequenceState(100, 500)
	state.AddMatch(0, StageMatch{Timestamp: 100})

	// Before timeout
	path := cond.FindAbsenceCompletionPath(state, 400)
	if path != nil {
		t.Error("expected nil path before timeout")
	}

	// After timeout
	path = cond.FindAbsenceCompletionPath(state, 600)
	if path == nil {
		t.Fatal("expected non-nil path after timeout")
	}
	if path.MatchIndices[0] != 0 {
		t.Errorf("expected stage 0 index 0, got %d", path.MatchIndices[0])
	}
}

// ############################################################################
//
//  PART 2: State Manager — Cache / Absence Tracking / Serialization
//
// ############################################################################

// ============================================================================
// State Manager Local Cache Tests
// ============================================================================

func TestCEPStateManager_LocalCache_SetGet(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	state := NewSequenceState(1000, 2000)
	state.AddMatch(0, StageMatch{Timestamp: 1000, Data: map[string]interface{}{"a": "1"}})

	mgr.SetState("test_key", state, 60)
	time.Sleep(10 * time.Millisecond)

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
	mgr.SetState("delete_key", state, 60)
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

	state1 := mgr.GetOrCreateState("gor_key", 5000, 5)
	if state1 == nil {
		t.Fatal("expected state, got nil")
	}
	if state1.ExpiresAt-state1.CreatedAt != 5000 {
		t.Errorf("expected 5000ms window, got %d", state1.ExpiresAt-state1.CreatedAt)
	}

	state1.AddMatch(0, StageMatch{Timestamp: 100})
	mgr.UpdateState("gor_key", state1, 5)
	time.Sleep(10 * time.Millisecond)

	state2 := mgr.GetOrCreateState("gor_key", 5000, 5)
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

	expired := mgr.GetExpiredAbsenceKeys(1500)
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired key at t=1500, got %d", len(expired))
	}
	if _, ok := expired["key1"]; !ok {
		t.Error("expected key1 to be expired")
	}

	expired = mgr.GetExpiredAbsenceKeys(2500)
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired key at t=2500, got %d", len(expired))
	}
	if _, ok := expired["key2"]; !ok {
		t.Error("expected key2 to be expired")
	}

	mgr.UntrackAbsenceKey("key3")
	expired = mgr.GetExpiredAbsenceKeys(5000)
	if len(expired) != 0 {
		t.Errorf("expected 0 expired keys after untrack, got %d", len(expired))
	}
}

func TestCEPStateManager_AbsenceWheel_KeyReuseNoStaleTrigger(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	mgr.TrackAbsenceKey("reused_key", absenceKeyInfo{ExpiresAt: 10000, RuleID: "r1", SeqID: 1})
	mgr.UntrackAbsenceKey("reused_key")
	mgr.TrackAbsenceKey("reused_key", absenceKeyInfo{ExpiresAt: 20000, RuleID: "r2", SeqID: 2})

	expired := mgr.GetExpiredAbsenceKeys(10000)
	if len(expired) != 0 {
		t.Fatalf("expected no expired keys at old expiry, got %d", len(expired))
	}

	expired = mgr.GetExpiredAbsenceKeys(20000)
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired key at new expiry, got %d", len(expired))
	}
	info, ok := expired["reused_key"]
	if !ok {
		t.Fatal("expected reused_key to be expired at new expiry")
	}
	if info.RuleID != "r2" || info.SeqID != 2 {
		t.Fatalf("expected latest absence info (r2,2), got (%s,%d)", info.RuleID, info.SeqID)
	}
}

func TestCEPStateManager_AbsenceWheel_ReTrackUpdatesEntry(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	mgr.TrackAbsenceKey("k", absenceKeyInfo{ExpiresAt: 30000, RuleID: "r1", SeqID: 1})
	mgr.TrackAbsenceKey("k", absenceKeyInfo{ExpiresAt: 30000, RuleID: "r1", SeqID: 99})

	expired := mgr.GetExpiredAbsenceKeys(30000)
	if len(expired) != 1 {
		t.Fatalf("expected exactly one expired entry, got %d", len(expired))
	}
	info := expired["k"]
	if info.SeqID != 99 {
		t.Fatalf("expected updated SeqID=99, got %d", info.SeqID)
	}
}

func TestCEPStateManager_AbsenceWheel_CatchUpMissedSlots(t *testing.T) {
	cache := newTestLocalCache()
	defer cache.Close()
	mgr := NewCEPStateManager(true, cache)

	mgr.TrackAbsenceKey("k", absenceKeyInfo{ExpiresAt: 2000, RuleID: "r1", SeqID: 1})

	expired := mgr.GetExpiredAbsenceKeys(1000)
	if len(expired) != 0 {
		t.Fatalf("expected 0 expired keys at t=1000, got %d", len(expired))
	}

	expired = mgr.GetExpiredAbsenceKeys(4000)
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired key at t=4000 after slot catch-up, got %d", len(expired))
	}
	if _, ok := expired["k"]; !ok {
		t.Fatal("expected key k to expire during catch-up scan")
	}

	expired = mgr.GetExpiredAbsenceKeys(5000)
	if len(expired) != 0 {
		t.Fatalf("expected no duplicate expiry at t=5000, got %d", len(expired))
	}
}

// ============================================================================
// BuildStateKey Tests
// ============================================================================

func TestBuildStateKey(t *testing.T) {
	key1 := BuildStateKey("ruleset1rule1", "192.168.1.1|")
	key2 := BuildStateKey("ruleset1rule1", "192.168.1.2|")
	key3 := BuildStateKey("ruleset1rule1", "192.168.1.1|")

	if key1 != key3 {
		t.Errorf("expected same keys for same input, got %s vs %s", key1, key3)
	}
	if key1 == key2 {
		t.Errorf("expected different keys for different input, got same: %s", key1)
	}
	if key1[:4] != CEPStateKeyPrefix {
		t.Errorf("expected key to start with %s, got %s", CEPStateKeyPrefix, key1[:4])
	}
}

// ============================================================================
// Memory Control Tests
// ============================================================================

func TestTrimStageMatches(t *testing.T) {
	state := NewSequenceState(1000, 2000)
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

func TestSerializeState_CompressedAndBackwardCompatibleDecode(t *testing.T) {
	state := NewSequenceState(1000, 2000)
	state.AddMatch(0, StageMatch{Timestamp: 100, Data: map[string]interface{}{"field": "value", "ip": "1.2.3.4"}})

	data, err := serializeState(state)
	if err != nil {
		t.Fatalf("serializeState failed: %v", err)
	}

	if len(data) < 4 || data[0] != 0x28 || data[1] != 0xB5 {
		t.Errorf("expected zstd magic header, got 0x%x 0x%x", data[0], data[1])
	}

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

	legacyPlain := `{"sm":{"0":[{"Timestamp":100,"Data":{"field":"legacy"}}]},"ca":1000,"ea":2000}`
	legacyRestored, err := decompressState(legacyPlain)
	if err != nil {
		t.Fatalf("decompressState legacy plain json failed: %v", err)
	}
	if legacyRestored.CreatedAt != 1000 || legacyRestored.ExpiresAt != 2000 {
		t.Errorf("unexpected legacy restored state: %+v", legacyRestored)
	}
}

// ============================================================================
// Timestamp Parsing Tests
// ============================================================================

func TestParseTimestampToNs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"unix seconds", "1700000000", 1700000000 * int64(time.Second)},
		{"unix milliseconds", "1700000000000", 1700000000000 * int64(time.Millisecond)},
		{"unix microseconds", "1700000000000000", 1700000000000000 * int64(time.Microsecond)},
		{"float seconds", "1700000000.123", int64(1700000000.123 * float64(time.Second))},
		{"RFC3339", "2023-11-14T22:13:20Z", 1700000000 * int64(time.Second)},
		{"ISO 8601 with T", "2023-11-14T22:13:20", 1700000000 * int64(time.Second)},
		{"ISO 8601 space", "2023-11-14 22:13:20", 1700000000 * int64(time.Second)},
		{"invalid", "not_a_timestamp", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimestampToNs(tt.input)
			if tt.expected == 0 {
				if result != 0 {
					t.Errorf("expected 0 for invalid input, got %d", result)
				}
				return
			}
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > int64(time.Second) {
				t.Errorf("expected ~%d, got %d (diff=%d)", tt.expected, result, diff)
			}
		})
	}
}

// ############################################################################
//
//  PART 3: Integration — XML Parsing / Validation / Execution / Context
//
// ############################################################################

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
	if seq.Events["fw_block"].GroupBy != "src_ip" {
		t.Errorf("expected fw_block group_by='src_ip', got '%s'", seq.Events["fw_block"].GroupBy)
	}
	if seq.Events["auth_success"].GroupBy != "client_ip" {
		t.Errorf("expected auth_success group_by='client_ip', got '%s'", seq.Events["auth_success"].GroupBy)
	}
}

func TestCEP_Execute_MultiSourceSequence_DifferentEventGroupByFields(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="multi source execute">
			<sequence within="10m">
				<event id="fw_block" group_by="src_ip" event_time="ts">
					<check type="EQU" field="event_type">fw_block</check>
				</event>
				<event id="auth_success" group_by="client_ip" event_time="ts">
					<check type="EQU" field="event_type">auth_success</check>
				</event>
				<condition>fw_block -> auth_success</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)
	var seq Sequence
	foundSeq := false
	for _, s := range rs.Rules[0].SequenceMap {
		seq = s
		foundSeq = true
		break
	}
	if !foundSeq {
		t.Fatal("expected sequence definition in rule")
	}

	stage1 := map[string]interface{}{"event_type": "fw_block", "src_ip": "10.0.0.8", "ts": "1000"}
	preMatchStage1 := make([]string, 0, len(seq.EventOrder))
	preMatchCacheStage1 := map[string]common.CheckCoreCache{}
	for _, eventID := range seq.EventOrder {
		if rs.evaluateEventDef(seq.Events[eventID], stage1, preMatchCacheStage1) {
			preMatchStage1 = append(preMatchStage1, eventID)
		}
	}
	if len(preMatchStage1) != 1 || preMatchStage1[0] != "fw_block" {
		t.Fatalf("unexpected stage1 pre-match event IDs: %+v", preMatchStage1)
	}
	key1Vals := rs.extractCorrelateValuesForStateLookup(&seq, preMatchStage1, stage1, map[string]common.CheckCoreCache{})
	key1 := BuildStateKey(seq.GroupByID, key1Vals)
	if key1Vals == "" {
		t.Fatal("expected non-empty group_by values for stage1")
	}

	stage2 := map[string]interface{}{"event_type": "auth_success", "client_ip": "10.0.0.8", "ts": "1001"}
	preMatchStage2 := make([]string, 0, len(seq.EventOrder))
	preMatchCacheStage2 := map[string]common.CheckCoreCache{}
	for _, eventID := range seq.EventOrder {
		if rs.evaluateEventDef(seq.Events[eventID], stage2, preMatchCacheStage2) {
			preMatchStage2 = append(preMatchStage2, eventID)
		}
	}
	if len(preMatchStage2) != 1 || preMatchStage2[0] != "auth_success" {
		t.Fatalf("unexpected stage2 pre-match event IDs: %+v", preMatchStage2)
	}
	key2Vals := rs.extractCorrelateValuesForStateLookup(&seq, preMatchStage2, stage2, map[string]common.CheckCoreCache{})
	key2 := BuildStateKey(seq.GroupByID, key2Vals)
	if key2Vals == "" {
		t.Fatal("expected non-empty group_by values for stage2")
	}
	if key1 != key2 {
		t.Fatalf("expected same state key across per-event group_by mapping, key1=%s key2=%s", key1, key2)
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
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="test"><sequence group_by="ip" local_cache="true"><event id="a"><check type="EQU" field="f">v</check></event><event id="b"><check type="EQU" field="f">v</check></event><condition>a -> b</condition></sequence></rule></root>`
	if _, err := ParseRuleset([]byte(xml)); err == nil {
		t.Fatal("expected error for missing 'within' attribute")
	}
}

func TestCEP_ParseError_TooFewEvents(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="test"><sequence within="5m" group_by="ip" local_cache="true"><event id="a"><check type="EQU" field="f">v</check></event><condition>a -> b</condition></sequence></rule></root>`
	if _, err := ParseRuleset([]byte(xml)); err == nil {
		t.Fatal("expected error for too few events")
	}
}

func TestCEP_ParseError_MissingCondition(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="test"><sequence within="5m" group_by="ip" local_cache="true"><event id="a"><check type="EQU" field="f">v</check></event><event id="b"><check type="EQU" field="f">v</check></event></sequence></rule></root>`
	if _, err := ParseRuleset([]byte(xml)); err == nil {
		t.Fatal("expected error for missing condition")
	}
}

func TestCEP_ParseError_DuplicateEventID(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="test"><sequence within="5m" group_by="ip" local_cache="true"><event id="a"><check type="EQU" field="f">v1</check></event><event id="a"><check type="EQU" field="f">v2</check></event><condition>a -> a</condition></sequence></rule></root>`
	if _, err := ParseRuleset([]byte(xml)); err == nil {
		t.Fatal("expected error for duplicate event ID")
	}
}

func TestCEP_BuildError_UndefinedEventInCondition(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="test"><sequence within="5m" group_by="ip" local_cache="true"><event id="a"><check type="EQU" field="f">v</check></event><event id="b"><check type="EQU" field="f">v</check></event><condition>a -> c</condition></sequence></rule></root>`
	rs, err := ParseRuleset([]byte(xml))
	if err != nil {
		t.Fatalf("parse should succeed: %v", err)
	}
	rs.RulesetID = "test"
	if err = RulesetBuild(rs); err == nil {
		t.Fatal("expected build error for undefined event 'c' in condition")
	}
}

func TestCEP_BuildError_UnreferencedEvent(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="test"><sequence within="5m" group_by="ip" local_cache="true"><event id="a"><check type="EQU" field="f">v</check></event><event id="b"><check type="EQU" field="f">v</check></event><event id="c"><check type="EQU" field="f">v</check></event><condition>a -> b</condition></sequence></rule></root>`
	rs, err := ParseRuleset([]byte(xml))
	if err != nil {
		t.Fatalf("parse should succeed: %v", err)
	}
	rs.RulesetID = "test"
	if err = RulesetBuild(rs); err == nil {
		t.Fatal("expected build error for unreferenced event 'c'")
	}
}

func TestCEP_BuildError_NoGroupBy(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="test"><sequence within="5m" local_cache="true"><event id="a"><check type="EQU" field="f">v</check></event><event id="b"><check type="EQU" field="f">v</check></event><condition>a -> b</condition></sequence></rule></root>`
	rs, err := ParseRuleset([]byte(xml))
	if err != nil {
		t.Fatalf("parse should succeed: %v", err)
	}
	rs.RulesetID = "test"
	if err = RulesetBuild(rs); err == nil {
		t.Fatal("expected build error for missing group_by")
	}
}

// ============================================================================
// Sequence Execution Tests
// ============================================================================

func TestCEP_Execute_BasicSequence(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="login then exfil"><sequence within="10m" group_by="source_ip" local_cache="true"><event id="login"><check type="EQU" field="event_type">login</check></event><event id="exfil"><check type="EQU" field="event_type">file_transfer</check></event><condition>login -> exfil</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	results := rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.1"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results after login, got %d", len(results))
	}
	rs.EngineCheck(map[string]interface{}{"event_type": "dns_query", "source_ip": "10.0.0.1"})
	time.Sleep(20 * time.Millisecond)

	results = rs.EngineCheck(map[string]interface{}{"event_type": "file_transfer", "source_ip": "10.0.0.1"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result after sequence completion, got %d", len(results))
	}
	if results[0][HitRuleIdFieldName] == nil {
		t.Error("expected hit rule ID in result")
	}
}

func TestCEP_Execute_DifferentCorrelationKeys(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="test"><sequence within="10m" group_by="source_ip" local_cache="true"><event id="a"><check type="EQU" field="event_type">login</check></event><event id="b"><check type="EQU" field="event_type">exfil</check></event><condition>a -> b</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.1"})
	time.Sleep(20 * time.Millisecond)
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.2"})
	time.Sleep(20 * time.Millisecond)

	results := rs.EngineCheck(map[string]interface{}{"event_type": "exfil", "source_ip": "10.0.0.3"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results for unmatched IP, got %d", len(results))
	}
	results = rs.EngineCheck(map[string]interface{}{"event_type": "exfil", "source_ip": "10.0.0.1"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for IP1 sequence, got %d", len(results))
	}
}

func TestCEP_Execute_ThreeStageSequence(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="three stage"><sequence within="10m" group_by="ip" local_cache="true"><event id="scan"><check type="EQU" field="type">scan</check></event><event id="exploit"><check type="EQU" field="type">exploit</check></event><event id="persist"><check type="EQU" field="type">persist</check></event><condition>scan -> exploit -> persist</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "scan", "ip": "1.2.3.4"})
	time.Sleep(20 * time.Millisecond)
	rs.EngineCheck(map[string]interface{}{"type": "exploit", "ip": "1.2.3.4"})
	time.Sleep(20 * time.Millisecond)
	results := rs.EngineCheck(map[string]interface{}{"type": "persist", "ip": "1.2.3.4"})
	if len(results) != 1 {
		t.Fatalf("expected 1 after stage 3, got %d", len(results))
	}
}

func TestCEP_Execute_OrBranch(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="or branch"><sequence within="10m" group_by="user" local_cache="true"><event id="login"><check type="EQU" field="type">login</check></event><event id="priv"><check type="EQU" field="type">priv_esc</check></event><event id="data"><check type="EQU" field="type">data_access</check></event><condition>login -> (priv or data)</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "login", "user": "alice"})
	time.Sleep(20 * time.Millisecond)
	results := rs.EngineCheck(map[string]interface{}{"type": "data_access", "user": "alice"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for OR branch match, got %d", len(results))
	}
}

func TestCEP_Execute_WithPreFilter(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="pre-filter test"><check type="EQU" field="source">internal</check><sequence within="10m" group_by="ip" local_cache="true"><event id="a"><check type="EQU" field="type">scan</check></event><event id="b"><check type="EQU" field="type">exploit</check></event><condition>a -> b</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "scan", "ip": "1.1.1.1", "source": "external"})
	time.Sleep(20 * time.Millisecond)
	results := rs.EngineCheck(map[string]interface{}{"type": "exploit", "ip": "1.1.1.1", "source": "external"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results for external source, got %d", len(results))
	}

	rs.EngineCheck(map[string]interface{}{"type": "scan", "ip": "1.1.1.1", "source": "internal"})
	time.Sleep(20 * time.Millisecond)
	results = rs.EngineCheck(map[string]interface{}{"type": "exploit", "ip": "1.1.1.1", "source": "internal"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for internal source, got %d", len(results))
	}
}

func TestCEP_Execute_CrossEventFieldReference(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="cross-event ref"><sequence within="10m" group_by="user" local_cache="true"><event id="login"><check type="EQU" field="type">login</check></event><event id="exfil"><check type="EQU" field="type">exfil</check></event><condition>login -> exfil</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "login", "user": "bob", "source_ip": "192.168.1.100"})
	time.Sleep(20 * time.Millisecond)

	results := rs.EngineCheck(map[string]interface{}{"type": "exfil", "user": "bob", "dest": "evil.com"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if _, exists := results[0]["#login"]; exists {
		t.Fatal("expected #login to be removed from output payload")
	}
	seqEvents, ok := results[0]["_sequence_events"]
	if !ok {
		t.Fatal("expected _sequence_events in result")
	}
	seqEventsMap := seqEvents.(map[string]interface{})
	if len(seqEventsMap) != 2 {
		t.Errorf("expected 2 events in _sequence_events, got %d", len(seqEventsMap))
	}
	loginEvt := seqEventsMap["login"].(map[string]interface{})
	if loginEvt["source_ip"] != "192.168.1.100" {
		t.Errorf("expected login source_ip=192.168.1.100, got %v", loginEvt["source_ip"])
	}
	exfilEvt := seqEventsMap["exfil"].(map[string]interface{})
	if exfilEvt["dest"] != "evil.com" {
		t.Errorf("expected exfil dest=evil.com, got %v", exfilEvt["dest"])
	}
	condMap := results[0]["_sequence_condition"].(map[string]interface{})
	if condMap["content"] != "login -> exfil" {
		t.Errorf("expected content 'login -> exfil', got %v", condMap["content"])
	}
}

func TestCEP_Execute_WithEventTimestamp(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="event time test"><sequence within="10m" group_by="ip" local_cache="true"><event id="a" event_time="ts"><check type="EQU" field="type">a</check></event><event id="b" event_time="ts"><check type="EQU" field="type">b</check></event><condition>a -> b</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "b", "ip": "1.1.1.1", "ts": "1700000002"})
	time.Sleep(20 * time.Millisecond)
	results := rs.EngineCheck(map[string]interface{}{"type": "a", "ip": "1.1.1.1", "ts": "1700000001"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for out-of-order events, got %d", len(results))
	}
}

func TestCEP_Execute_SequenceNotTriggeredOutOfOrder(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="order check"><sequence within="10m" group_by="ip" local_cache="true"><event id="a" event_time="ts"><check type="EQU" field="type">a</check></event><event id="b" event_time="ts"><check type="EQU" field="type">b</check></event><condition>a -> b</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "a", "ip": "1.1.1.1", "ts": "1700000002"})
	time.Sleep(20 * time.Millisecond)
	results := rs.EngineCheck(map[string]interface{}{"type": "b", "ip": "1.1.1.1", "ts": "1700000001"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results when b.ts < a.ts (wrong order), got %d", len(results))
	}
}

func TestCEP_Execute_SequenceWithAppend(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="seq with append"><sequence within="10m" group_by="ip" local_cache="true"><event id="a"><check type="EQU" field="type">a</check></event><event id="b"><check type="EQU" field="type">b</check></event><condition>a -> b</condition></sequence><append field="alert_type">sequence_detected</append></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "a", "ip": "1.1.1.1"})
	time.Sleep(20 * time.Millisecond)
	results := rs.EngineCheck(map[string]interface{}{"type": "b", "ip": "1.1.1.1"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["alert_type"] != "sequence_detected" {
		t.Errorf("expected alert_type='sequence_detected', got '%v'", results[0]["alert_type"])
	}
}

// ============================================================================
// OR Branch Binding Tests
// ============================================================================

func TestCEP_Execute_OrBranch_OverlappingEvents_BindsSingleBranch(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="test" name="test"><sequence within="5m" group_by="agent_id" local_cache="true"><event id="login" event_time="timestamp"><check type="EQU" field="event_type">login</check></event><event id="exfil1" event_time="timestamp"><check type="EQU" field="event_type">file_transfer</check></event><event id="exfil2" event_time="timestamp"><check type="EQU" field="event_type">file_transfer</check></event><condition>login -> (exfil1 or exfil2)</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"agent_id": "a1b2", "event_type": "login", "timestamp": "1770868609"})
	res := rs.EngineCheck(map[string]interface{}{"agent_id": "a1b2", "event_type": "file_transfer", "timestamp": "1770868619"})
	if len(res) != 1 {
		t.Fatalf("expected 1 result after exfil event, got %d", len(res))
	}
	seqEvents := res[0]["_sequence_events"].(map[string]interface{})
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
	xml := `<root name="test" type="DETECTION"><rule id="test" name="test"><sequence within="5m" group_by="agent_id" local_cache="true"><event id="login"><check type="EQU" field="event_type">login</check></event><event id="exfil1"><check type="EQU" field="event_type">file_transfer</check><append field="_@branch">left</append></event><event id="exfil2"><check type="EQU" field="event_type">file_transfer</check><append field="_@branch">right</append></event><event id="exec"><check type="EQU" field="event_type">exec</check><check type="EQU" field="branch">_@branch</check></event><condition>login -> (exfil1 or exfil2) -> exec</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"agent_id": "a1b2", "event_type": "login"})
	rs.EngineCheck(map[string]interface{}{"agent_id": "a1b2", "event_type": "file_transfer"})
	res := rs.EngineCheck(map[string]interface{}{"agent_id": "a1b2", "event_type": "exec", "branch": "left"})
	if len(res) != 1 {
		t.Fatalf("expected 1 result after exec, got %d", len(res))
	}
}

func TestCEP_Execute_OrBranch_OverlappingEvents_EventTimeUsesBoundBranch(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="test" name="test"><sequence within="5m" group_by="agent_id" local_cache="true"><event id="login" event_time="login_ts"><check type="EQU" field="event_type">login</check></event><event id="exfil1" event_time="left_ts"><check type="EQU" field="event_type">file_transfer</check></event><event id="exfil2" event_time="right_ts"><check type="EQU" field="event_type">file_transfer</check></event><event id="exec"><check type="EQU" field="event_type">exec</check></event><condition>login -> (exfil1 or exfil2) -> exec</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"agent_id": "a1b2", "event_type": "login", "login_ts": "2"})
	rs.EngineCheck(map[string]interface{}{"agent_id": "a1b2", "event_type": "file_transfer", "right_ts": "1"})
	res := rs.EngineCheck(map[string]interface{}{"agent_id": "a1b2", "event_type": "exec"})
	if len(res) != 1 {
		t.Fatalf("expected 1 result after exec, got %d", len(res))
	}
}

// ============================================================================
// Sequence Context (_@) Tests
// ============================================================================

func TestCEP_Parse_EventAppendForSequenceContext(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="ctx parse"><sequence within="10m" group_by="user_id" local_cache="true"><event id="download"><check type="EQU" field="event_type">download</check><append field="_@file.current">_$file_path</append></event><event id="exec"><check type="EQU" field="event_type">exec</check></event><condition>download -> exec</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)
	seq := getFirstSequenceFromRuleset(t, rs)
	download := seq.Events["download"]
	if download == nil {
		t.Fatal("download event missing")
	}
	if len(download.Appends) != 1 {
		t.Fatalf("expected 1 append in download event, got %d", len(download.Appends))
	}
	if download.Appends[0].FieldName != "_@file.current" {
		t.Fatalf("expected append field _@file.current, got %s", download.Appends[0].FieldName)
	}
}

func TestCEP_SequenceContext_BasicWriteRead(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="ctx basic"><sequence within="10m" group_by="user_id" local_cache="true"><event id="download"><check type="EQU" field="event_type">download</check><append field="_@file.current">_$file_path</append></event><event id="exec"><check type="EQU" field="event_type">exec</check><check type="EQU" field="file_path">_@file.current</check></event><condition>download -> exec</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"user_id": "u-1", "event_type": "download", "file_path": "/tmp/a"})
	out := rs.EngineCheck(map[string]interface{}{"user_id": "u-1", "event_type": "exec", "file_path": "/tmp/a"})
	if len(out) != 1 {
		t.Fatalf("expected 1 hit when _@ matches, got %d", len(out))
	}
}

func TestCEP_SequenceContext_UpdateAcrossStages(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="ctx update"><sequence within="10m" group_by="user_id" local_cache="true"><event id="download"><check type="EQU" field="event_type">download</check><append field="_@file.current">_$name</append></event><event id="mv"><check type="EQU" field="event_type">rename</check><check type="EQU" field="src">_@file.current</check><append field="_@file.current">_$dst</append></event><event id="exec"><check type="EQU" field="event_type">exec</check><check type="EQU" field="file_path">_@file.current</check></event><condition>download -> mv -> exec</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"user_id": "u-2", "event_type": "download", "name": "a"})
	rs.EngineCheck(map[string]interface{}{"user_id": "u-2", "event_type": "rename", "src": "a", "dst": "b"})
	out := rs.EngineCheck(map[string]interface{}{"user_id": "u-2", "event_type": "exec", "file_path": "b"})
	if len(out) != 1 {
		t.Fatalf("expected 1 hit after context update chain, got %d", len(out))
	}
}

func TestCEP_SequenceContext_IsolatedByGroupByKey(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="ctx isolate"><sequence within="10m" group_by="user_id" local_cache="true"><event id="download"><check type="EQU" field="event_type">download</check><append field="_@file.current">_$file_path</append></event><event id="exec"><check type="EQU" field="event_type">exec</check><check type="EQU" field="file_path">_@file.current</check></event><condition>download -> exec</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"user_id": "u-A", "event_type": "download", "file_path": "/tmp/a"})
	out := rs.EngineCheck(map[string]interface{}{"user_id": "u-B", "event_type": "exec", "file_path": "/tmp/a"})
	if len(out) != 0 {
		t.Fatalf("expected 0 hits for isolated key, got %d", len(out))
	}
}

func TestCEP_SequenceContext_MissingKeyDoesNotMatch(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="ctx missing"><sequence within="10m" group_by="user_id" local_cache="true"><event id="a"><check type="EQU" field="event_type">a</check></event><event id="b"><check type="EQU" field="event_type">b</check><check type="EQU" field="k">_@not.exists</check></event><condition>a -> b</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"user_id": "u-3", "event_type": "a"})
	out := rs.EngineCheck(map[string]interface{}{"user_id": "u-3", "event_type": "b", "k": "whatever"})
	if len(out) != 0 {
		t.Fatalf("expected 0 hits when context key missing, got %d", len(out))
	}
}

func TestCEP_Execute_SequenceContextReference_WithAtPrefix(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="ctx ref test"><sequence within="10m" group_by="user_id" local_cache="true"><event id="download"><check type="EQU" field="event_type">download</check><append field="_@file.current">_$file_path</append></event><event id="exec"><check type="EQU" field="event_type">exec</check><check type="EQU" field="file_path">_@file.current</check></event><condition>download -> exec</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"user_id": "u-1", "event_type": "download", "file_path": "/tmp/b"})
	results := rs.EngineCheck(map[string]interface{}{"user_id": "u-1", "event_type": "exec", "file_path": "/tmp/b"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result with _@ context match, got %d", len(results))
	}
}

func TestCEP_SequenceContext_PluginAppendWriteRead(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="ctx plugin append"><sequence within="10m" group_by="user_id" local_cache="true"><event id="download"><check type="EQU" field="event_type">download</check><append type="PLUGIN" field="_@file.encoded">base64Encode(file_path)</append></event><event id="exec"><check type="EQU" field="event_type">exec</check><check type="EQU" field="encoded_path">_@file.encoded</check></event><condition>download -> exec</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	if out := rs.EngineCheck(map[string]interface{}{"user_id": "u-p1", "event_type": "download", "file_path": "/tmp/a"}); len(out) != 0 {
		t.Fatalf("expected no hit after stage1, got %d", len(out))
	}
	out := rs.EngineCheck(map[string]interface{}{"user_id": "u-p1", "event_type": "exec", "encoded_path": "L3RtcC9h"})
	if len(out) != 1 {
		t.Fatalf("expected 1 hit when plugin-written _@ matches, got %d", len(out))
	}
}

func TestCEP_SequenceContext_GroupByLookupWhenFirstPassHasNoMatches(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="ctx first pass empty"><sequence within="10m" group_by="user_id" local_cache="true"><event id="a"><check type="EQU" field="event_type">a</check><append field="_@token">_$token</append></event><event id="b"><check type="EQU" field="token_in">_@token</check></event><condition>a -> b</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"user_id": "u-fallback-1", "event_type": "a", "token": "tok-1"})
	out := rs.EngineCheck(map[string]interface{}{"user_id": "u-fallback-1", "token_in": "tok-1"})
	if len(out) != 1 {
		t.Fatalf("expected 1 hit when state is recovered by group_by lookup, got %d", len(out))
	}
}

func TestCEP_SequenceContext_ClearedAfterSequenceCompletion(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="ctx cleanup"><sequence within="10m" group_by="user_id" local_cache="true"><event id="download"><check type="EQU" field="event_type">download</check><append field="_@file.current">_$file_path</append></event><event id="exec"><check type="EQU" field="event_type">exec</check><check type="EQU" field="file_path">_@file.current</check></event><condition>download -> exec</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"user_id": "u-clean-1", "event_type": "download", "file_path": "/tmp/clean-a"})
	firstHit := rs.EngineCheck(map[string]interface{}{"user_id": "u-clean-1", "event_type": "exec", "file_path": "/tmp/clean-a"})
	if len(firstHit) != 1 {
		t.Fatalf("expected 1 hit for first completion, got %d", len(firstHit))
	}

	secondHit := rs.EngineCheck(map[string]interface{}{"user_id": "u-clean-1", "event_type": "exec", "file_path": "/tmp/clean-a"})
	if len(secondHit) != 0 {
		t.Fatalf("expected 0 hits after completion cleanup, got %d", len(secondHit))
	}
}

// ############################################################################
//
//  PART 4: Value Store (Pebble)
//
// ############################################################################

func TestPebbleCEPValueStore_PutGetDelete(t *testing.T) {
	store, err := NewPebbleCEPValueStore("test-put-get-delete")
	if err != nil {
		t.Fatalf("NewPebbleCEPValueStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	ref, err := store.PutSnapshot(map[string]interface{}{"event_type": "login", "user": "alice"}, time.Now().Add(2*time.Minute).UnixNano())
	if err != nil {
		t.Fatalf("PutSnapshot failed: %v", err)
	}
	if ref == "" {
		t.Fatal("expected non-empty ref")
	}
	data, err := store.GetSnapshot(ref)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if data["event_type"] != "login" || data["user"] != "alice" {
		t.Fatalf("unexpected snapshot data: %+v", data)
	}
	if err := store.DeleteSnapshot(ref); err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}
	if _, err := store.GetSnapshot(ref); err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestPebbleCEPValueStore_CleanupExpired(t *testing.T) {
	store, err := NewPebbleCEPValueStore("test-cleanup-expired")
	if err != nil {
		t.Fatalf("NewPebbleCEPValueStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	pastRef, err := store.PutSnapshot(map[string]interface{}{"k": "expired"}, time.Now().Add(-1*time.Minute).UnixNano())
	if err != nil {
		t.Fatalf("PutSnapshot expired failed: %v", err)
	}
	futureRef, err := store.PutSnapshot(map[string]interface{}{"k": "active"}, time.Now().Add(5*time.Minute).UnixNano())
	if err != nil {
		t.Fatalf("PutSnapshot active failed: %v", err)
	}
	if err := store.cleanupExpired(time.Now().UnixNano(), 1000); err != nil {
		t.Fatalf("cleanupExpired failed: %v", err)
	}
	if _, err := store.GetSnapshot(pastRef); err == nil {
		t.Fatal("expected expired ref to be cleaned up")
	}
	active, err := store.GetSnapshot(futureRef)
	if err != nil {
		t.Fatalf("expected active ref to remain, got error: %v", err)
	}
	if active["k"] != "active" {
		t.Fatalf("unexpected active snapshot data: %+v", active)
	}
}

func TestPebbleCEPValueStore_ConcurrentPutAndGet(t *testing.T) {
	store, err := NewPebbleCEPValueStore("test-concurrent-put-get")
	if err != nil {
		t.Fatalf("NewPebbleCEPValueStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	const n = 200
	type item struct {
		ref string
		val string
	}
	items := make([]item, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			val := fmt.Sprintf("v-%d", i)
			ref, putErr := store.PutSnapshot(map[string]interface{}{"v": val}, time.Now().Add(2*time.Minute).UnixNano())
			if putErr != nil {
				t.Errorf("PutSnapshot(%d) failed: %v", i, putErr)
				return
			}
			items[i] = item{ref: ref, val: val}
		}()
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		if items[i].ref == "" {
			t.Fatalf("missing ref for index %d", i)
		}
		data, getErr := store.GetSnapshot(items[i].ref)
		if getErr != nil {
			t.Fatalf("GetSnapshot(%d) failed: %v", i, getErr)
		}
		if data["v"] != items[i].val {
			t.Fatalf("value mismatch at %d: want=%s got=%v", i, items[i].val, data["v"])
		}
	}
}

func TestPebbleCEPValueStore_CloseIdempotent(t *testing.T) {
	store, err := NewPebbleCEPValueStore("test-close-idempotent")
	if err != nil {
		t.Fatalf("NewPebbleCEPValueStore failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close should be idempotent, got: %v", err)
	}
}

func TestPebbleCEPValueStore_GetSnapshot_InlineExpiry(t *testing.T) {
	store, err := NewPebbleCEPValueStore("test-inline-expiry")
	if err != nil {
		t.Fatalf("NewPebbleCEPValueStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Put snapshot that expires almost immediately
	ref, err := store.PutSnapshot(
		map[string]interface{}{"k": "ephemeral"},
		time.Now().Add(50*time.Millisecond).UnixNano(),
	)
	if err != nil {
		t.Fatalf("PutSnapshot failed: %v", err)
	}

	// Should be readable immediately
	data, err := store.GetSnapshot(ref)
	if err != nil {
		t.Fatalf("expected snapshot to be readable, got error: %v", err)
	}
	if data["k"] != "ephemeral" {
		t.Fatalf("unexpected data: %+v", data)
	}

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// GetSnapshot should detect inline expiry and auto-delete
	_, err = store.GetSnapshot(ref)
	if err == nil {
		t.Fatal("expected error for expired snapshot, got nil")
	}

	// Verify the snapshot was actually deleted (not just soft-expired)
	_, err = store.GetSnapshot(ref)
	if err == nil {
		t.Fatal("expected snapshot to be permanently deleted after inline expiry")
	}
}

func TestSanitizePathComponent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"alphanumeric", "abc123", "abc123"},
		{"with dashes and dots", "my-store.v1", "my-store.v1"},
		{"uppercase to lower", "MyStore", "mystore"},
		{"special chars replaced", "store@#$%", "store____"},
		{"spaces replaced", " my store ", "my_store"},
		{"empty string", "", ""},
		{"path traversal", "../../../etc", ".._.._.._etc"},
		{"unicode chars", "存储test", "__test"},
		{"underscores kept", "my_store_v2", "my_store_v2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizePathComponent(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizePathComponent(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ############################################################################
//
//  PART 5: Absence Timeout End-to-End Integration
//
// ############################################################################

func TestCEP_Execute_AbsenceTimeout_EndToEnd(t *testing.T) {
	// End-to-end test: a -> !b with append after sequence.
	// Tests the core absence pipeline: buildAbsenceResult + executePostSequenceOps + enrichment.
	//
	// Strategy: Use a long "within" so the cache TTL does not expire,
	// then manually simulate the expired window by rewriting ExpiresAt to the past
	// and directly calling the scanner's inner logic (avoiding timing wheel second-boundary flakiness).
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="absence e2e">
			<sequence within="10m" group_by="user" local_cache="true">
				<event id="login">
					<check type="EQU" field="event_type">login</check>
				</event>
				<event id="mfa">
					<check type="EQU" field="event_type">mfa_verify</check>
				</event>
				<condition>login -> !mfa</condition>
			</sequence>
			<append field="alert_type">no_mfa</append>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// Stage 1: login event (creates partial state + absence tracking)
	results := rs.EngineCheck(map[string]interface{}{
		"event_type": "login",
		"user":       "alice",
		"source_ip":  "10.0.0.1",
	})
	if len(results) != 0 {
		t.Fatalf("expected 0 results after login, got %d", len(results))
	}
	time.Sleep(20 * time.Millisecond) // Ristretto async

	// Compute the stateKey that executeSequence used.
	stateKey := BuildStateKey("test_ruleset:r1", "alice|")

	// Read the existing state, rewrite ExpiresAt to simulate window expiry.
	state := rs.seqStateManager.GetState(stateKey)
	if state == nil {
		t.Fatal("expected state to exist after login event")
	}
	pastMs := time.Now().UnixMilli() - 1000
	state.ExpiresAt = pastMs
	rs.seqStateManager.UpdateState(stateKey, state, 600)
	time.Sleep(20 * time.Millisecond)

	// Directly invoke the absence result pipeline (mirrors scanAbsenceTimeouts inner logic)
	var seq Sequence
	var seqID int
	for id, s := range rs.Rules[0].SequenceMap {
		seq = s
		seqID = id
		_ = seqID
		break
	}

	nowMs := time.Now().UnixMilli()
	absencePath := seq.Condition.FindAbsenceCompletionPath(state, nowMs)
	if absencePath == nil {
		t.Fatal("expected FindAbsenceCompletionPath to return non-nil path")
	}

	resultData := rs.buildAbsenceResult(state, &seq, absencePath)
	if resultData == nil {
		t.Fatal("expected buildAbsenceResult to return non-nil data")
	}

	resultData = rs.executePostSequenceOps(&rs.Rules[0], seqID, resultData)

	// Verify the result has correct enrichment
	// Verify post-sequence append was applied
	if resultData["alert_type"] != "no_mfa" {
		t.Errorf("expected alert_type='no_mfa', got %v", resultData["alert_type"])
	}
	// Verify _sequence_events contains login data
	seqEvents, ok := resultData["_sequence_events"]
	if !ok {
		t.Fatal("expected _sequence_events in absence result")
	}
	seqEventsMap := seqEvents.(map[string]interface{})
	if _, ok := seqEventsMap["login"]; !ok {
		t.Fatal("expected 'login' key in _sequence_events")
	}
	// Verify _sequence_condition
	condRaw, ok := resultData["_sequence_condition"]
	if !ok {
		t.Fatal("expected _sequence_condition in absence result")
	}
	condMap := condRaw.(map[string]interface{})
	if condMap["content"] != "login -> !mfa" {
		t.Errorf("expected condition content 'login -> !mfa', got %v", condMap["content"])
	}
}

func TestCEP_Execute_AbsenceTimeout_ViolatedNoTrigger(t *testing.T) {
	// When the absent event IS observed, FindAbsenceCompletionPath should return nil.
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="absence violated">
			<sequence within="10m" group_by="user" local_cache="true">
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

	// Login
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "user": "bob"})
	time.Sleep(20 * time.Millisecond)

	// MFA verify (violates absence)
	rs.EngineCheck(map[string]interface{}{"event_type": "mfa_verify", "user": "bob"})
	time.Sleep(20 * time.Millisecond)

	stateKey := BuildStateKey("test_ruleset:r1", "bob|")
	state := rs.seqStateManager.GetState(stateKey)
	if state == nil {
		t.Fatal("expected state to exist")
	}

	// Simulate window expiry
	state.ExpiresAt = time.Now().UnixMilli() - 1000

	var seq Sequence
	for _, s := range rs.Rules[0].SequenceMap {
		seq = s
		break
	}

	nowMs := time.Now().UnixMilli()
	absencePath := seq.Condition.FindAbsenceCompletionPath(state, nowMs)
	if absencePath != nil {
		t.Fatal("expected nil path when absence is violated (mfa was observed)")
	}
}

func TestCEP_Execute_AbsenceTimeout_ThreeStage(t *testing.T) {
	// Test "a -> !b -> c -> !d": middle absence bounded by c, trailing !d needs timeout.
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="mixed absence">
			<sequence within="10m" group_by="user" local_cache="true">
				<event id="a">
					<check type="EQU" field="event_type">a</check>
				</event>
				<event id="b">
					<check type="EQU" field="event_type">b</check>
				</event>
				<event id="c">
					<check type="EQU" field="event_type">c</check>
				</event>
				<event id="d">
					<check type="EQU" field="event_type">d</check>
				</event>
				<condition>a -> !b -> c -> !d</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	// a and c matched, b and d absent
	rs.EngineCheck(map[string]interface{}{"event_type": "a", "user": "carol"})
	time.Sleep(20 * time.Millisecond)
	// c should not trigger immediate completion (trailing !d)
	results := rs.EngineCheck(map[string]interface{}{"event_type": "c", "user": "carol"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results (trailing !d needs timeout), got %d", len(results))
	}
	time.Sleep(20 * time.Millisecond)

	stateKey := BuildStateKey("test_ruleset:r1", "carol|")
	state := rs.seqStateManager.GetState(stateKey)
	if state == nil {
		t.Fatal("expected state to exist")
	}

	// Simulate window expiry
	state.ExpiresAt = time.Now().UnixMilli() - 1000

	var seq Sequence
	var seqID int
	for id, s := range rs.Rules[0].SequenceMap {
		seq = s
		seqID = id
		_ = seqID
		break
	}

	nowMs := time.Now().UnixMilli()
	absencePath := seq.Condition.FindAbsenceCompletionPath(state, nowMs)
	if absencePath == nil {
		t.Fatal("expected FindAbsenceCompletionPath to return non-nil path")
	}

	resultData := rs.buildAbsenceResult(state, &seq, absencePath)
	if resultData == nil {
		t.Fatal("expected buildAbsenceResult to return non-nil data")
	}

	seqEvents := resultData["_sequence_events"].(map[string]interface{})
	if _, ok := seqEvents["a"]; !ok {
		t.Error("expected 'a' in _sequence_events")
	}
	if _, ok := seqEvents["c"]; !ok {
		t.Error("expected 'c' in _sequence_events")
	}
}

// ############################################################################
//
//  PART 6: Checklist Execution in Sequence Events
//
// ############################################################################

func TestCEP_Execute_ChecklistBasedEvent(t *testing.T) {
	// Verify that an event with a checklist (condition-based) correctly
	// evaluates and progresses the sequence.
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="checklist exec">
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

	// Event matching brute (login + failure) — matches checklist "a and b"
	results := rs.EngineCheck(map[string]interface{}{
		"event_type": "login",
		"result":     "failure",
		"source_ip":  "10.0.0.5",
	})
	if len(results) != 0 {
		t.Fatalf("expected 0 results after brute event, got %d", len(results))
	}
	time.Sleep(20 * time.Millisecond)

	// Event matching success (login + success) — should complete sequence
	results = rs.EngineCheck(map[string]interface{}{
		"event_type": "login",
		"result":     "success",
		"source_ip":  "10.0.0.5",
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result after sequence completion, got %d", len(results))
	}
}

func TestCEP_Execute_ChecklistConditionNotMet(t *testing.T) {
	// Verify that a partial checklist match does NOT satisfy the event.
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="checklist partial">
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

	// Event with login but result=success does NOT satisfy "a and b" (b requires failure)
	rs.EngineCheck(map[string]interface{}{
		"event_type": "login",
		"result":     "success",
		"source_ip":  "10.0.0.6",
	})
	time.Sleep(20 * time.Millisecond)

	// Another success event — should NOT trigger because brute stage was never satisfied
	results := rs.EngineCheck(map[string]interface{}{
		"event_type": "login",
		"result":     "success",
		"source_ip":  "10.0.0.6",
	})
	// We might get 0 or we might get nothing — the key point is the brute stage was not satisfied
	// so only the success stage would be populated. Without a brute match, sequence can't complete.
	if len(results) != 0 {
		t.Fatalf("expected 0 results (brute stage never satisfied), got %d", len(results))
	}
}

// ############################################################################
//
//  PART 7: Parser Edge Cases
//
// ############################################################################

func TestParseCEPCondition_NestedSequenceInNot(t *testing.T) {
	// "!(a -> b)" — sequence operator inside a NOT is not allowed.
	// This exercises the containsSequenceOp validation.
	_, err := ParseCEPCondition("a -> !(b -> c)")
	if err == nil {
		t.Fatal("expected error for nested sequence inside NOT operator")
	}
}

func TestParseCEPCondition_FlattenedNestedSequence(t *testing.T) {
	// "(a -> b) -> c" — parser flattens nested sequences into a single 3-stage sequence.
	cond, err := ParseCEPCondition("(a -> b) -> c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.Stages) != 3 {
		t.Fatalf("expected 3 stages after flatten, got %d", len(cond.Stages))
	}
	assertStage(t, cond.Stages[0], false, []string{"a"})
	assertStage(t, cond.Stages[1], false, []string{"b"})
	assertStage(t, cond.Stages[2], false, []string{"c"})
}

func TestParseCEPCondition_ConsecutiveAbsence(t *testing.T) {
	// "a -> !b -> !c" — consecutive absence stages should parse successfully
	cond, err := ParseCEPCondition("a -> !b -> !c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.Stages) != 3 {
		t.Fatalf("expected 3 stages, got %d", len(cond.Stages))
	}
	assertStage(t, cond.Stages[0], false, []string{"a"})
	assertStage(t, cond.Stages[1], true, []string{"b"})
	assertStage(t, cond.Stages[2], true, []string{"c"})
}

func TestParseCEPCondition_AbsenceOrGroup(t *testing.T) {
	// "a -> !(b or c)" — absence of either b or c
	cond, err := ParseCEPCondition("a -> !(b or c)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cond.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(cond.Stages))
	}
	assertStage(t, cond.Stages[1], true, []string{"b", "c"})
}

func TestGetStageEventIDs(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> (b or c) -> !d")

	// Valid indices
	ids0 := cond.GetStageEventIDs(0)
	if len(ids0) != 1 || ids0[0] != "a" {
		t.Errorf("stage 0: expected [a], got %v", ids0)
	}
	ids1 := cond.GetStageEventIDs(1)
	if len(ids1) != 2 {
		t.Errorf("stage 1: expected 2 IDs, got %v", ids1)
	}
	ids2 := cond.GetStageEventIDs(2)
	if len(ids2) != 1 || ids2[0] != "d" {
		t.Errorf("stage 2: expected [d], got %v", ids2)
	}

	// Out of range
	idsNeg := cond.GetStageEventIDs(-1)
	if idsNeg != nil {
		t.Errorf("expected nil for negative index, got %v", idsNeg)
	}
	idsOver := cond.GetStageEventIDs(99)
	if idsOver != nil {
		t.Errorf("expected nil for out-of-range index, got %v", idsOver)
	}
}

// ############################################################################
//
//  PART 8: Extended Coverage — Threshold, Cross-Event Ref, GroupBy, Within
//
// ############################################################################

// TestCEP_Execute_ThresholdInEvent verifies that a <threshold> inside a
// sequence <event> gates the event match. The login event requires >2
// occurrences before the stage is satisfied.
func TestCEP_Execute_ThresholdInEvent(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="threshold gate"><sequence within="10m" group_by="source_ip" local_cache="true"><event id="login"><check type="EQU" field="event_type">login</check><threshold group_by="source_ip" range="5m" local_cache="true">2</threshold></event><event id="exfil"><check type="EQU" field="event_type">exfil</check></event><condition>login -> exfil</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	// Login 1: threshold count=1, 1>2 false => event does not match stage
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.1"})
	time.Sleep(20 * time.Millisecond)

	// Exfil should not trigger because login stage was never satisfied
	results := rs.EngineCheck(map[string]interface{}{"event_type": "exfil", "source_ip": "10.0.0.1"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results (threshold not met after 1 login), got %d", len(results))
	}

	// Login 2: count=2, 2>2 false => still not met
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.1"})
	time.Sleep(20 * time.Millisecond)

	results = rs.EngineCheck(map[string]interface{}{"event_type": "exfil", "source_ip": "10.0.0.1"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results (threshold not met after 2 logins), got %d", len(results))
	}

	// Login 3: count=3, 3>2 true => threshold met, login stage satisfied
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.1"})
	time.Sleep(20 * time.Millisecond)

	// Now exfil should complete the sequence
	results = rs.EngineCheck(map[string]interface{}{"event_type": "exfil", "source_ip": "10.0.0.1"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result (threshold met after 3 logins), got %d", len(results))
	}
}

// TestCEP_Execute_ThresholdInEvent_DifferentGroups verifies that threshold
// counters are isolated by their group_by field.
func TestCEP_Execute_ThresholdInEvent_DifferentGroups(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="threshold groups"><sequence within="10m" group_by="source_ip" local_cache="true"><event id="login"><check type="EQU" field="event_type">login</check><threshold group_by="source_ip" range="5m" local_cache="true">2</threshold></event><event id="exfil"><check type="EQU" field="event_type">exfil</check></event><condition>login -> exfil</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	// 2 logins from IP1 (threshold not met: count=2, 2>2 false)
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.1"})
	time.Sleep(10 * time.Millisecond)
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.1"})
	time.Sleep(10 * time.Millisecond)

	// 3 logins from IP2 (threshold met: count=3, 3>2 true)
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.2"})
	time.Sleep(10 * time.Millisecond)
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.2"})
	time.Sleep(10 * time.Millisecond)
	rs.EngineCheck(map[string]interface{}{"event_type": "login", "source_ip": "10.0.0.2"})
	time.Sleep(10 * time.Millisecond)

	// Exfil from IP1: 0 results (threshold not met for IP1)
	results := rs.EngineCheck(map[string]interface{}{"event_type": "exfil", "source_ip": "10.0.0.1"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results for IP1 (threshold not met), got %d", len(results))
	}

	// Exfil from IP2: 1 result (threshold met for IP2)
	results = rs.EngineCheck(map[string]interface{}{"event_type": "exfil", "source_ip": "10.0.0.2"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result for IP2 (threshold met), got %d", len(results))
	}
}

// TestCEP_Execute_CrossEventFieldRef_InAppend verifies that post-sequence
// <append> operations can reference fields from earlier events using the
// _$#event_id.field syntax.
func TestCEP_Execute_CrossEventFieldRef_InAppend(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="cross ref append"><sequence within="10m" group_by="user" local_cache="true"><event id="login"><check type="EQU" field="type">login</check></event><event id="exfil"><check type="EQU" field="type">exfil</check></event><condition>login -> exfil</condition></sequence><append field="initial_login_ip">_$#login.source_ip</append><append field="exfil_destination">_$#exfil.dest</append><append field="alert_summary">User logged in from _$#login.source_ip then exfil to _$#exfil.dest</append></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "login", "user": "alice", "source_ip": "192.168.1.100"})
	time.Sleep(20 * time.Millisecond)

	results := rs.EngineCheck(map[string]interface{}{"type": "exfil", "user": "alice", "dest": "evil.com"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]

	// Verify cross-event field references resolved correctly
	if r["initial_login_ip"] != "192.168.1.100" {
		t.Errorf("expected initial_login_ip=192.168.1.100, got %v", r["initial_login_ip"])
	}
	if r["exfil_destination"] != "evil.com" {
		t.Errorf("expected exfil_destination=evil.com, got %v", r["exfil_destination"])
	}

	// Verify mixed template interpolation
	expected := "User logged in from 192.168.1.100 then exfil to evil.com"
	if r["alert_summary"] != expected {
		t.Errorf("expected alert_summary=%q, got %q", expected, r["alert_summary"])
	}
}

// TestCEP_Execute_CrossEventFieldRef_ThreeStage verifies _$# references
// work across a 3-stage sequence.
func TestCEP_Execute_CrossEventFieldRef_ThreeStage(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="three stage ref"><sequence within="10m" group_by="host" local_cache="true"><event id="recon"><check type="EQU" field="type">recon</check></event><event id="exploit"><check type="EQU" field="type">exploit</check></event><event id="exfil"><check type="EQU" field="type">exfil</check></event><condition>recon -> exploit -> exfil</condition></sequence><append field="recon_target">_$#recon.target</append><append field="exploit_cve">_$#exploit.cve</append><append field="exfil_bytes">_$#exfil.bytes_sent</append></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "recon", "host": "srv1", "target": "db-server"})
	time.Sleep(20 * time.Millisecond)
	rs.EngineCheck(map[string]interface{}{"type": "exploit", "host": "srv1", "cve": "CVE-2024-1234"})
	time.Sleep(20 * time.Millisecond)
	results := rs.EngineCheck(map[string]interface{}{"type": "exfil", "host": "srv1", "bytes_sent": "50000"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r["recon_target"] != "db-server" {
		t.Errorf("expected recon_target=db-server, got %v", r["recon_target"])
	}
	if r["exploit_cve"] != "CVE-2024-1234" {
		t.Errorf("expected exploit_cve=CVE-2024-1234, got %v", r["exploit_cve"])
	}
	if r["exfil_bytes"] != "50000" {
		t.Errorf("expected exfil_bytes=50000, got %v", r["exfil_bytes"])
	}
}

// TestCEP_Execute_MultiFieldGroupBy verifies that comma-separated group_by
// fields produce distinct correlation keys per unique field combination.
func TestCEP_Execute_MultiFieldGroupBy(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="multi field group_by"><sequence within="10m" group_by="src_ip,src_port" local_cache="true"><event id="scan"><check type="EQU" field="type">scan</check></event><event id="exploit"><check type="EQU" field="type">exploit</check></event><condition>scan -> exploit</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	// Scan from (1.1.1.1, 80)
	rs.EngineCheck(map[string]interface{}{"type": "scan", "src_ip": "1.1.1.1", "src_port": "80"})
	time.Sleep(20 * time.Millisecond)

	// Exploit from (1.1.1.1, 443) — different port, should NOT match
	results := rs.EngineCheck(map[string]interface{}{"type": "exploit", "src_ip": "1.1.1.1", "src_port": "443"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results (different port), got %d", len(results))
	}

	// Exploit from (1.1.1.1, 80) — same IP+port, should match
	results = rs.EngineCheck(map[string]interface{}{"type": "exploit", "src_ip": "1.1.1.1", "src_port": "80"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result (same IP+port), got %d", len(results))
	}
}

// TestCEP_Execute_MultiFieldGroupBy_ThreeFields verifies 3-field group_by.
func TestCEP_Execute_MultiFieldGroupBy_ThreeFields(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="3 field group_by"><sequence within="10m" group_by="region,host,service" local_cache="true"><event id="err"><check type="EQU" field="type">error</check></event><event id="crash"><check type="EQU" field="type">crash</check></event><condition>err -> crash</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "error", "region": "us-east", "host": "web-1", "service": "api"})
	time.Sleep(20 * time.Millisecond)

	// Different service
	results := rs.EngineCheck(map[string]interface{}{"type": "crash", "region": "us-east", "host": "web-1", "service": "worker"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results (different service), got %d", len(results))
	}

	// Exact match
	results = rs.EngineCheck(map[string]interface{}{"type": "crash", "region": "us-east", "host": "web-1", "service": "api"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result (all 3 fields match), got %d", len(results))
	}
}

// TestCEP_Execute_MixedGroupBy verifies that when sequence-level group_by is
// set, it takes precedence over per-event group_by for all events. The
// per-event group_by is only used when no sequence-level group_by exists
// (tested by TestCEP_Execute_MultiSourceSequence_DifferentEventGroupByFields).
func TestCEP_Execute_MixedGroupBy(t *testing.T) {
	// Sequence-level group_by="source_ip" is set; event "login" also has
	// group_by="client_ip", but the sequence-level one takes precedence.
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="mixed group_by"><sequence within="10m" group_by="source_ip" local_cache="true"><event id="fw_block"><check type="EQU" field="type">block</check></event><event id="login" group_by="client_ip"><check type="EQU" field="type">login</check></event><condition>fw_block -> login</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "block", "source_ip": "10.0.0.1"})
	time.Sleep(20 * time.Millisecond)

	// login has client_ip but NOT source_ip — sequence-level group_by
	// extracts source_ip from the data, which is missing for login event.
	// So the correlation key is empty and no match happens.
	results := rs.EngineCheck(map[string]interface{}{"type": "login", "client_ip": "10.0.0.1"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results (sequence-level group_by takes precedence, source_ip missing), got %d", len(results))
	}

	// login with source_ip set — now sequence-level group_by can extract it
	results = rs.EngineCheck(map[string]interface{}{"type": "login", "source_ip": "10.0.0.1"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result (source_ip matches), got %d", len(results))
	}
}

// TestCEP_Execute_MixedGroupBy_Isolation verifies that different per-event
// group_by values produce isolated state.
func TestCEP_Execute_MixedGroupBy_Isolation(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="mixed group_by isolated"><sequence within="10m" group_by="source_ip" local_cache="true"><event id="fw_block"><check type="EQU" field="type">block</check></event><event id="login" group_by="client_ip"><check type="EQU" field="type">login</check></event><condition>fw_block -> login</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	rs.EngineCheck(map[string]interface{}{"type": "block", "source_ip": "10.0.0.1"})
	time.Sleep(20 * time.Millisecond)

	// login with different client_ip — should NOT match
	results := rs.EngineCheck(map[string]interface{}{"type": "login", "client_ip": "10.0.0.2"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results (different client_ip), got %d", len(results))
	}
}

// TestCEP_Execute_WithinWindowExpiration verifies that events arriving after
// the within window do not complete the sequence.
func TestCEP_Execute_WithinWindowExpiration(t *testing.T) {
	// within must be > 5s (ParseDurationToSecondsInt enforces minimum)
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="within expiry"><sequence within="6s" group_by="ip" local_cache="true"><event id="a"><check type="EQU" field="type">a</check></event><event id="b"><check type="EQU" field="type">b</check></event><condition>a -> b</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	// Send stage 1 event
	rs.EngineCheck(map[string]interface{}{"type": "a", "ip": "1.1.1.1"})

	// Wait for the within window + Ristretto TTL to expire (6s TTL + buffer)
	time.Sleep(8 * time.Second)

	// Stage 2 event should not complete because state expired from Ristretto
	results := rs.EngineCheck(map[string]interface{}{"type": "b", "ip": "1.1.1.1"})
	if len(results) != 0 {
		t.Fatalf("expected 0 results (within window expired), got %d", len(results))
	}

	// Verify sequence still works within a fresh window
	rs.EngineCheck(map[string]interface{}{"type": "a", "ip": "1.1.1.1"})
	time.Sleep(20 * time.Millisecond)
	results = rs.EngineCheck(map[string]interface{}{"type": "b", "ip": "1.1.1.1"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result (fresh window), got %d", len(results))
	}
}

// TestSerializeState_WithContext verifies that SequenceState.Context is
// correctly preserved through serialization/deserialization.
func TestSerializeState_WithContext(t *testing.T) {
	state := NewSequenceState(5000, 10000)
	state.AddMatch(0, StageMatch{Timestamp: 100, Data: map[string]interface{}{"event": "login"}})
	state.Context["file.current"] = "/tmp/malware.exe"
	state.Context["user.name"] = "alice"

	data, err := serializeState(state)
	if err != nil {
		t.Fatalf("serializeState failed: %v", err)
	}

	restored, err := decompressState(data)
	if err != nil {
		t.Fatalf("decompressState failed: %v", err)
	}

	if restored.CreatedAt != 5000 || restored.ExpiresAt != 10000 {
		t.Errorf("timestamps mismatch: CreatedAt=%d ExpiresAt=%d", restored.CreatedAt, restored.ExpiresAt)
	}

	if restored.Context == nil {
		t.Fatal("expected non-nil Context after deserialization")
	}
	if restored.Context["file.current"] != "/tmp/malware.exe" {
		t.Errorf("expected file.current=/tmp/malware.exe, got %v", restored.Context["file.current"])
	}
	if restored.Context["user.name"] != "alice" {
		t.Errorf("expected user.name=alice, got %v", restored.Context["user.name"])
	}

	matches := restored.StageMatches[0]
	if len(matches) != 1 || matches[0].Data["event"] != "login" {
		t.Errorf("unexpected restored match data: %v", matches)
	}
}

// TestCEP_Execute_PebbleValueRef_Integration verifies the full round-trip of
// event snapshots through PebbleCEPValueStore: PutSnapshot on stage match →
// GetSnapshot on sequence completion → correct data in _sequence_events.
func TestCEP_Execute_PebbleValueRef_Integration(t *testing.T) {
	xml := `<root name="test" type="DETECTION"><rule id="r1" name="pebble roundtrip"><sequence within="10m" group_by="user" local_cache="true"><event id="login"><check type="EQU" field="type">login</check></event><event id="transfer"><check type="EQU" field="type">transfer</check></event><condition>login -> transfer</condition></sequence></rule></root>`
	rs := buildTestRuleset(t, xml)

	// Verify cepValueStore is initialized (local_cache=true triggers Pebble)
	if rs.cepValueStore == nil {
		t.Fatal("expected cepValueStore to be initialized for local_cache=true")
	}

	// Send login with rich payload — this data goes through Pebble (PutSnapshot)
	rs.EngineCheck(map[string]interface{}{
		"type":      "login",
		"user":      "bob",
		"source_ip": "172.16.0.55",
		"country":   "US",
		"device":    "laptop-42",
	})
	time.Sleep(20 * time.Millisecond)

	// Complete sequence — login data should be read back from Pebble (GetSnapshot)
	results := rs.EngineCheck(map[string]interface{}{
		"type":    "transfer",
		"user":    "bob",
		"dest":    "external-server",
		"size_mb": 500,
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	seqEvents := results[0]["_sequence_events"]
	if seqEvents == nil {
		t.Fatal("expected _sequence_events in result")
	}
	seqEventsMap := seqEvents.(map[string]interface{})

	// Verify login event data was preserved through Pebble round-trip
	loginEvt, ok := seqEventsMap["login"].(map[string]interface{})
	if !ok {
		t.Fatal("expected login event in _sequence_events")
	}
	if loginEvt["source_ip"] != "172.16.0.55" {
		t.Errorf("expected login source_ip=172.16.0.55, got %v", loginEvt["source_ip"])
	}
	if loginEvt["country"] != "US" {
		t.Errorf("expected login country=US, got %v", loginEvt["country"])
	}
	if loginEvt["device"] != "laptop-42" {
		t.Errorf("expected login device=laptop-42, got %v", loginEvt["device"])
	}

	// Verify transfer event data
	transferEvt, ok := seqEventsMap["transfer"].(map[string]interface{})
	if !ok {
		t.Fatal("expected transfer event in _sequence_events")
	}
	if transferEvt["dest"] != "external-server" {
		t.Errorf("expected transfer dest=external-server, got %v", transferEvt["dest"])
	}
}

// TestCEP_AbsenceScanner_RealTimer_EndToEnd tests the full absence pipeline
// by directly calling EngineCheck + scanAbsenceTimeouts (no goroutines/tickers).
func TestCEP_AbsenceScanner_RealTimer_EndToEnd(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="absence real timer">
			<sequence within="6s" group_by="ip" local_cache="true">
				<event id="test1">
					<check type="EQU" field="city">Chicago</check>
				</event>
				<event id="test2">
					<check type="EQU" field="city">Houston</check>
				</event>
				<condition>test1 -> !test2</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	if !rs.hasAbsenceSequences {
		t.Fatal("expected hasAbsenceSequences=true")
	}
	if rs.seqStateManager == nil {
		t.Fatal("expected seqStateManager to be initialized")
	}
	if rs.ruleByID == nil || rs.ruleByID["r1"] == nil {
		t.Fatal("expected ruleByID to contain r1")
	}

	// Set up downstream channel to capture absence alerts
	outputCh := make(chan map[string]interface{}, 100)
	rs.DownStream = map[string]*chan map[string]interface{}{"test_out": &outputCh}

	// Step 1: Send event directly via EngineCheck
	results := rs.EngineCheck(map[string]interface{}{
		"city": "Chicago",
		"ip":   "10.0.0.1",
	})
	if len(results) != 0 {
		t.Fatalf("expected 0 results after Chicago event, got %d", len(results))
	}
	time.Sleep(50 * time.Millisecond) // ristretto async

	// Initialize the scanner's lastProcessedSecond BEFORE waiting.
	// In production, the scanner runs via ticker from Start(), so
	// lastProcessedSecond gets initialized early. Here we simulate that.
	rs.scanAbsenceTimeouts()

	// Step 2: Verify state was created
	stateKey := BuildStateKey("test_ruleset:r1", "10.0.0.1|")
	state := rs.seqStateManager.GetState(stateKey)
	if state == nil {
		t.Fatal("expected state to exist after Chicago event")
	}
	t.Logf("State ExpiresAt=%d, now=%d, diff=%dms",
		state.ExpiresAt, time.Now().UnixMilli(), state.ExpiresAt-time.Now().UnixMilli())

	// Step 3: Wait for the within window to expire
	waitMs := state.ExpiresAt - time.Now().UnixMilli() + 1500 // 1.5s extra
	if waitMs > 0 {
		t.Logf("Waiting %dms for within window to expire...", waitMs)
		time.Sleep(time.Duration(waitMs) * time.Millisecond)
	}

	nowMs := time.Now().UnixMilli()
	t.Logf("After wait: now=%d, ExpiresAt=%d, expired=%v", nowMs, state.ExpiresAt, nowMs >= state.ExpiresAt)

	// Step 4: Verify state still exists in cache (grace period)
	stateCheck := rs.seqStateManager.GetState(stateKey)
	if stateCheck == nil {
		t.Fatal("State was evicted from cache before scanner could read it (TTL too short)")
	}
	t.Log("State still in cache after within expired (grace period working)")

	// Step 5: Directly call scanAbsenceTimeouts
	t.Log("Calling scanAbsenceTimeouts directly...")
	rs.scanAbsenceTimeouts()

	// Step 6: Check if alert was sent to downstream
	select {
	case res := <-outputCh:
		t.Logf("SUCCESS: Got absence alert: %v", res)
	default:
		// Check if the key was even found by GetExpiredAbsenceKeys
		nowMs2 := time.Now().UnixMilli()
		expired := rs.seqStateManager.GetExpiredAbsenceKeys(nowMs2)
		t.Logf("GetExpiredAbsenceKeys returned %d keys", len(expired))
		stateAfter := rs.seqStateManager.GetState(stateKey)
		t.Logf("State after scan: %v", stateAfter)
		t.Fatal("FAIL: No absence alert received on downstream channel")
	}
}

// TestCEP_AbsenceScanner_FullStart_EndToEnd tests the absence pipeline
// with Start() running the real scanner ticker + goroutines.
func TestCEP_AbsenceScanner_FullStart_EndToEnd(t *testing.T) {
	xml := `<root name="test" type="DETECTION">
		<rule id="r1" name="absence full start">
			<sequence within="6s" group_by="ip" local_cache="true">
				<event id="test1">
					<check type="EQU" field="city">Chicago</check>
				</event>
				<event id="test2">
					<check type="EQU" field="city">Houston</check>
				</event>
				<condition>test1 -> !test2</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildTestRuleset(t, xml)

	inputCh := make(chan map[string]interface{}, 100)
	outputCh := make(chan map[string]interface{}, 100)
	rs.UpStream = map[string]*chan map[string]interface{}{"test_in": &inputCh}
	rs.DownStream = map[string]*chan map[string]interface{}{"test_out": &outputCh}

	err := rs.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rs.Stop()

	// Send event matching stage 1
	inputCh <- map[string]interface{}{
		"city": "Chicago",
		"ip":   "10.0.0.1",
	}

	// Wait for within window + scanner tick + buffer
	timeout := time.After(12 * time.Second)
	select {
	case res := <-outputCh:
		t.Logf("SUCCESS: Got absence alert via full Start(): %v", res)
		if res["city"] != "Chicago" {
			t.Errorf("expected city=Chicago, got %v", res["city"])
		}
	case <-timeout:
		t.Fatal("TIMEOUT: No absence alert received via full Start()")
	}
}

// TestCEP_AbsenceScanner_NewFromExisting_EndToEnd tests the absence pipeline
// using NewFromExisting (the production path for PNS instances).
func TestCEP_AbsenceScanner_NewFromExisting_EndToEnd(t *testing.T) {
	xmlBytes := []byte(`<root name="test" type="DETECTION">
		<rule id="r1" name="absence via NewFromExisting">
			<sequence within="6s" group_by="ip" local_cache="true">
				<event id="test1">
					<check type="EQU" field="city">Chicago</check>
				</event>
				<event id="test2">
					<check type="EQU" field="city">Houston</check>
				</event>
				<condition>test1 -> !test2</condition>
			</sequence>
		</rule>
	</root>`)

	// Step 1: Parse and build the "existing" ruleset (same as production)
	existing, err := ParseRuleset(xmlBytes)
	if err != nil {
		t.Fatalf("ParseRuleset failed: %v", err)
	}
	existing.RulesetID = "test_ruleset"
	existing.IsDetection = true
	existing.RawConfig = string(xmlBytes)

	err = RulesetBuild(existing)
	if err != nil {
		t.Fatalf("RulesetBuild failed: %v", err)
	}
	t.Cleanup(func() { existing.cleanup() })

	// Verify existing ruleset has absence flag
	t.Logf("existing: hasAbsenceSequences=%v, seqStateManager=%v, ruleByID=%v, cepValueStore=%v",
		existing.hasAbsenceSequences, existing.seqStateManager != nil,
		len(existing.ruleByID), existing.cepValueStore != nil)

	// Step 2: Create instance via NewFromExisting (the production path)
	instance, err := NewFromExisting(existing, "TEST.project1")
	if err != nil {
		t.Fatalf("NewFromExisting failed: %v", err)
	}
	t.Cleanup(func() { instance.cleanup() })

	// Verify the instance has all necessary CEP components
	t.Logf("instance: hasAbsenceSequences=%v, seqStateManager=%v, ruleByID=%v, cepValueStore=%v",
		instance.hasAbsenceSequences, instance.seqStateManager != nil,
		len(instance.ruleByID), instance.cepValueStore != nil)

	if !instance.hasAbsenceSequences {
		t.Fatal("instance: hasAbsenceSequences should be true")
	}
	if instance.seqStateManager == nil {
		t.Fatal("instance: seqStateManager should not be nil")
	}
	if instance.ruleByID == nil || instance.ruleByID["r1"] == nil {
		t.Fatal("instance: ruleByID should contain r1")
	}
	if instance.cepValueStore == nil {
		t.Fatal("instance: cepValueStore should not be nil")
	}

	// Verify rule's SequenceMap is accessible
	rule := instance.ruleByID["r1"]
	seqCount := 0
	for seqID, seq := range rule.SequenceMap {
		seqCount++
		t.Logf("instance rule r1: seqID=%d, condition=%q, hasAbsence=%v, localCache=%v, withinMs=%d",
			seqID, seq.Condition.Raw, seq.Condition.HasAbsenceStages(), seq.LocalCache, seq.WithinMs)
	}
	if seqCount == 0 {
		t.Fatal("instance: rule r1 has no sequences in SequenceMap")
	}

	// Step 3: Set up channels and start
	inputCh := make(chan map[string]interface{}, 100)
	outputCh := make(chan map[string]interface{}, 100)
	instance.UpStream = map[string]*chan map[string]interface{}{"test_in": &inputCh}
	instance.DownStream = map[string]*chan map[string]interface{}{"test_out": &outputCh}

	err = instance.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer instance.Stop()

	// Step 4: Send event
	inputCh <- map[string]interface{}{
		"city": "Chicago",
		"ip":   "10.0.0.1",
	}

	// Step 5: Wait for absence alert
	timeout := time.After(12 * time.Second)
	select {
	case res := <-outputCh:
		t.Logf("SUCCESS: Got absence alert via NewFromExisting: %v", res)
		if res["city"] != "Chicago" {
			t.Errorf("expected city=Chicago, got %v", res["city"])
		}
	case <-timeout:
		// Debug: check state
		stateKey := BuildStateKey("test_ruleset:r1", "10.0.0.1|")
		state := instance.seqStateManager.GetState(stateKey)
		if state == nil {
			t.Log("DEBUG: state is nil (evicted or never created)")
		} else {
			t.Logf("DEBUG: state exists, ExpiresAt=%d, now=%d", state.ExpiresAt, time.Now().UnixMilli())
		}
		t.Fatal("TIMEOUT: No absence alert received via NewFromExisting")
	}
}
