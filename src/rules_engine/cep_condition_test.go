package rules_engine

import (
	"testing"
)

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
	// Stage 0: a
	assertStage(t, cond.Stages[0], false, []string{"a"})
	// Stage 1: b
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

	// a@T=100, b@T=200 -> complete (correct order)
	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(1, StageMatch{Timestamp: 200})
	if !cond.CheckComplete(state) {
		t.Error("expected sequence to be complete (a@100, b@200)")
	}
}

func TestCheckComplete_WrongOrder(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b")

	// a@T=200, b@T=100 -> NOT complete (b happened before a)
	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 200})
	state.AddMatch(1, StageMatch{Timestamp: 100})
	if cond.CheckComplete(state) {
		t.Error("expected sequence NOT complete (a@200, b@100 - wrong order)")
	}
}

func TestCheckComplete_OutOfOrderArrival(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b -> c")

	// Events arrive out of order: c first, then a, then b
	// But timestamps are valid: a@100, b@200, c@300
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

	// Only a and c matched, b missing
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

	// a@T=100, b@T=100 -> NOT complete (timestamps must be strictly increasing)
	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(1, StageMatch{Timestamp: 100})
	if cond.CheckComplete(state) {
		t.Error("expected NOT complete (same timestamp, not strictly increasing)")
	}
}

func TestCheckComplete_MultipleMatchesPerStage(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> b")

	// Multiple matches for stage 0: a@50, a@150
	// Multiple matches for stage 1: b@100, b@200
	// Valid ordering: a@50 -> b@100, or a@50 -> b@200, or a@150 -> b@200
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

	// a@300, b@200, c@100 -> no valid increasing order
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

	// a matched, b NOT matched -> absence satisfied
	// But CheckComplete requires absence + all other stages to match
	// Since b is absent and has no matches, we need to verify via CheckAbsenceTimeout
	state := NewSequenceState(100, 500)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	// No match for stage 1 (absent)

	// Before timeout
	if cond.CheckAbsenceTimeout(state, 400) {
		t.Error("expected NOT triggered before timeout")
	}

	// After timeout - absence confirmed
	if !cond.CheckAbsenceTimeout(state, 600) {
		t.Error("expected triggered after timeout (b never appeared)")
	}
}

func TestCheckComplete_AbsenceObserved(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b")

	// a matched, b also matched -> absence violated
	state := NewSequenceState(100, 500)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(1, StageMatch{Timestamp: 200}) // b was observed

	// After timeout - but b was observed, so should NOT trigger
	if cond.CheckAbsenceTimeout(state, 600) {
		t.Error("expected NOT triggered (b was observed)")
	}
}

func TestCheckComplete_AbsenceObservedBeforePrev(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b")

	// a@200, b@100 -> b happened BEFORE a, so absence after a is still valid
	state := NewSequenceState(100, 500)
	state.AddMatch(0, StageMatch{Timestamp: 200})
	state.AddMatch(1, StageMatch{Timestamp: 100}) // b before a

	// After timeout - b was before a, so absence after a is satisfied
	if !cond.CheckAbsenceTimeout(state, 600) {
		t.Error("expected triggered (b@100 was before a@200, so absence after a is valid)")
	}
}

func TestCheckComplete_AbsenceThenPresence(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b -> c")

	// a@100, no b, c@300 -> should complete
	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(2, StageMatch{Timestamp: 300})
	// No match for stage 1 (absent)

	// Normal CheckComplete should NOT work (absence needs timeout logic)
	// But since b has no matches at all, absence is trivially satisfied
	if !cond.CheckComplete(state) {
		t.Error("expected complete (a -> [no b] -> c)")
	}
}

func TestCheckComplete_AbsenceThenPresenceViolated(t *testing.T) {
	cond, _ := ParseCEPCondition("a -> !b -> c")

	// a@100, b@200, c@300 -> b was observed between a and c, absence violated
	state := NewSequenceState(100, 1000)
	state.AddMatch(0, StageMatch{Timestamp: 100})
	state.AddMatch(1, StageMatch{Timestamp: 200}) // b observed
	state.AddMatch(2, StageMatch{Timestamp: 300})

	if cond.CheckComplete(state) {
		t.Error("expected NOT complete (b@200 observed between a@100 and c@300)")
	}
}

// ============================================================================
// Helpers
// ============================================================================

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
