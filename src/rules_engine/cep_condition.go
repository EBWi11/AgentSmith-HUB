package rules_engine

import (
	"errors"
	"fmt"
	"strings"
)

// ============================================================================
// CEP Condition Parser and Evaluator
//
// Parses condition expressions like "a -> b -> (c or d)" into a structured
// representation of ordered stages. Evaluates event matches against stages
// and checks temporal ordering for sequence completion.
//
// Operator precedence (lowest to highest):
//   -> (sequence)  <  or  <  and  <  ! (not)  <  ()
//
// Design principle: "Match first, check time later"
//   1. Match: check incoming event against all event definitions
//   2. Stage eval: determine which stages the event satisfies
//   3. Time check: verify temporal ordering across accumulated matches
// ============================================================================

// --- Token Types ---

const (
	CEPTokenLiteral  = iota // Event ID (e.g., "login", "exfil")
	CEPTokenOperator        // Operator (->  &  |  !  (  ))
)

// CEPToken represents a single token in the condition expression.
type CEPToken struct {
	Data  string
	Type  int
	Index int // Position in the original token list (for error reporting)
}

// --- AST Node Types ---
// Reuses the ExprAST interface from engine_condition.go:
//   - NumberExprAST  (literal event ID)
//   - BinaryExprAST  (and/or/->)
//   - UnaryExprAST   (!)

// Internal operator symbols
const (
	CEPOpSequence = "->" // Temporal sequence
	CEPOpAnd      = "&"  // Logical AND (same event)
	CEPOpOr       = "|"  // Logical OR (same event)
	CEPOpNot      = "!"  // Absence (after ->) or negation (within stage)
)

// --- CEP Condition (parsed result) ---

// CEPStage represents one stage in the temporal sequence (separated by ->).
type CEPStage struct {
	Expr     ExprAST  // AST for within-stage matching (and/or/!/literal)
	IsAbsent bool     // true if this stage is an absence check (preceded by !)
	EventIDs []string // All event IDs referenced in this stage
}

// CEPCondition represents a fully parsed and flattened CEP condition.
type CEPCondition struct {
	Raw       string          // Original expression string
	Stages    []CEPStage      // Ordered stages extracted from the -> chain
	AllEvents map[string]bool // All event IDs referenced in the entire condition
}

// --- State Types (for sequence tracking) ---

// StageMatch records that a stage was satisfied by an event at a given time.
type StageMatch struct {
	Timestamp int64                  // Unix nanoseconds (from event_time or detection time)
	Data      map[string]interface{} // Optional inline snapshot (used by Redis mode/fallback)
	ValueRef  string                 // Optional external value-store reference (local cache mode)
	// MatchedEventIDsSet indicates MatchedEventIDs has been intentionally resolved
	// for this match (including the case of an empty binding).
	MatchedEventIDsSet bool
	// MatchedEventIDs records which event IDs in the stage expression actually
	// satisfied this stage for this match (important for OR branches).
	MatchedEventIDs []string
}

// SequenceState tracks the accumulated matches for one correlation key.
type SequenceState struct {
	StageMatches map[int][]StageMatch   // stageIndex -> list of matches (sorted by timestamp)
	Context      map[string]interface{} // Sequence-scoped context for dynamic cross-event constraints
	CreatedAt    int64                  // When the first match was recorded (unix ms)
	ExpiresAt    int64                  // Deadline for sequence completion (unix ms)
}

// NewSequenceState creates a new SequenceState with the given expiration.
func NewSequenceState(createdAt, expiresAt int64) *SequenceState {
	return &SequenceState{
		StageMatches: make(map[int][]StageMatch),
		Context:      make(map[string]interface{}),
		CreatedAt:    createdAt,
		ExpiresAt:    expiresAt,
	}
}

// AddMatch records a stage match. Maintains timestamp-sorted order.
func (s *SequenceState) AddMatch(stageIdx int, match StageMatch) {
	matches := s.StageMatches[stageIdx]

	// Insert in sorted order by timestamp
	insertIdx := len(matches)
	for i, m := range matches {
		if match.Timestamp < m.Timestamp {
			insertIdx = i
			break
		}
	}

	// Insert at position
	matches = append(matches, StageMatch{})
	copy(matches[insertIdx+1:], matches[insertIdx:])
	matches[insertIdx] = match
	s.StageMatches[stageIdx] = matches
}

// ============================================================================
// Tokenizer
// ============================================================================

// tokenizeCEPCondition converts a condition string into a list of tokens.
func tokenizeCEPCondition(expr string) ([]CEPToken, map[string]bool, error) {
	if strings.TrimSpace(expr) == "" {
		return nil, nil, errors.New("condition expression cannot be empty")
	}

	allLiterals := make(map[string]bool)

	// Pad operators with spaces for easy splitting
	expr = strings.ReplaceAll(expr, "->", " -> ")
	expr = strings.ReplaceAll(expr, "(", " ( ")
	expr = strings.ReplaceAll(expr, ")", " ) ")
	// Handle ! carefully: only pad if not part of ->
	// Since we already replaced -> with " -> ", any remaining ! is a not operator
	expr = strings.ReplaceAll(expr, "!", " ! ")

	parts := strings.Fields(expr) // Split by whitespace, ignoring empty parts

	tokens := make([]CEPToken, 0, len(parts))

	for i, part := range parts {
		lower := strings.ToLower(part)

		switch {
		case part == "->":
			tokens = append(tokens, CEPToken{Data: CEPOpSequence, Type: CEPTokenOperator, Index: i})
		case lower == "and":
			tokens = append(tokens, CEPToken{Data: CEPOpAnd, Type: CEPTokenOperator, Index: i})
		case lower == "or":
			tokens = append(tokens, CEPToken{Data: CEPOpOr, Type: CEPTokenOperator, Index: i})
		case lower == "not" || part == "!":
			tokens = append(tokens, CEPToken{Data: CEPOpNot, Type: CEPTokenOperator, Index: i})
		case part == "(":
			tokens = append(tokens, CEPToken{Data: "(", Type: CEPTokenOperator, Index: i})
		case part == ")":
			tokens = append(tokens, CEPToken{Data: ")", Type: CEPTokenOperator, Index: i})
		default:
			// Validate literal: must be alphanumeric/underscore/hyphen
			if !isValidCEPEventID(part) {
				return nil, nil, fmt.Errorf("invalid event ID '%s' at position %d", part, i)
			}
			allLiterals[part] = true
			tokens = append(tokens, CEPToken{Data: part, Type: CEPTokenLiteral, Index: i})
		}
	}

	if len(tokens) == 0 {
		return nil, nil, errors.New("condition expression produced no tokens")
	}

	return tokens, allLiterals, nil
}

// isValidCEPEventID checks if a string is a valid event ID (alphanumeric, underscore, hyphen).
func isValidCEPEventID(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// ============================================================================
// Parser (Recursive Descent with Precedence Climbing)
// ============================================================================

// cepParser holds the parser state.
type cepParser struct {
	tokens  []CEPToken
	pos     int
	current CEPToken
}

func newCEPParser(tokens []CEPToken) *cepParser {
	return &cepParser{
		tokens:  tokens,
		pos:     0,
		current: tokens[0],
	}
}

// advance moves to the next token. Returns false if at end.
func (p *cepParser) advance() bool {
	p.pos++
	if p.pos < len(p.tokens) {
		p.current = p.tokens[p.pos]
		return true
	}
	return false
}

// peek returns the current token without advancing.
func (p *cepParser) peek() CEPToken {
	return p.current
}

// atEnd returns true if all tokens have been consumed.
func (p *cepParser) atEnd() bool {
	return p.pos >= len(p.tokens)
}

// parseSequenceExpr handles the lowest precedence: -> operator
// sequence_expr := or_expr ( '->' or_expr )*
func (p *cepParser) parseSequenceExpr() (ExprAST, error) {
	lhs, err := p.parseOrExpr()
	if err != nil {
		return nil, err
	}

	for !p.atEnd() && p.peek().Type == CEPTokenOperator && p.peek().Data == CEPOpSequence {
		p.advance() // consume ->
		if p.atEnd() {
			return nil, errors.New("unexpected end of expression after '->'")
		}
		rhs, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		lhs = BinaryExprAST{Op: CEPOpSequence, Lhs: lhs, Rhs: rhs}
	}

	return lhs, nil
}

// parseOrExpr handles: or_expr := and_expr ( 'or' and_expr )*
func (p *cepParser) parseOrExpr() (ExprAST, error) {
	lhs, err := p.parseAndExpr()
	if err != nil {
		return nil, err
	}

	for !p.atEnd() && p.peek().Type == CEPTokenOperator && p.peek().Data == CEPOpOr {
		p.advance() // consume |
		if p.atEnd() {
			return nil, errors.New("unexpected end of expression after 'or'")
		}
		rhs, err := p.parseAndExpr()
		if err != nil {
			return nil, err
		}
		lhs = BinaryExprAST{Op: CEPOpOr, Lhs: lhs, Rhs: rhs}
	}

	return lhs, nil
}

// parseAndExpr handles: and_expr := unary_expr ( 'and' unary_expr )*
func (p *cepParser) parseAndExpr() (ExprAST, error) {
	lhs, err := p.parseUnaryExpr()
	if err != nil {
		return nil, err
	}

	for !p.atEnd() && p.peek().Type == CEPTokenOperator && p.peek().Data == CEPOpAnd {
		p.advance() // consume &
		if p.atEnd() {
			return nil, errors.New("unexpected end of expression after 'and'")
		}
		rhs, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		lhs = BinaryExprAST{Op: CEPOpAnd, Lhs: lhs, Rhs: rhs}
	}

	return lhs, nil
}

// parseUnaryExpr handles: unary_expr := '!' unary_expr | primary
func (p *cepParser) parseUnaryExpr() (ExprAST, error) {
	if !p.atEnd() && p.peek().Type == CEPTokenOperator && p.peek().Data == CEPOpNot {
		p.advance() // consume !
		if p.atEnd() {
			return nil, errors.New("unexpected end of expression after '!'")
		}
		operand, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		return UnaryExprAST{Op: CEPOpNot, Operand: operand}, nil
	}

	return p.parsePrimary()
}

// parsePrimary handles: primary := '(' sequence_expr ')' | literal
func (p *cepParser) parsePrimary() (ExprAST, error) {
	if p.atEnd() {
		return nil, errors.New("unexpected end of expression")
	}

	tok := p.peek()

	if tok.Type == CEPTokenOperator && tok.Data == "(" {
		p.advance() // consume (
		if p.atEnd() {
			return nil, errors.New("unexpected end of expression after '('")
		}
		expr, err := p.parseSequenceExpr()
		if err != nil {
			return nil, err
		}
		if p.atEnd() || p.peek().Data != ")" {
			return nil, errors.New("expected ')' but not found")
		}
		p.advance() // consume )
		return expr, nil
	}

	if tok.Type == CEPTokenLiteral {
		p.advance() // consume literal
		return NumberExprAST{Val: tok.Data}, nil
	}

	return nil, fmt.Errorf("unexpected token '%s' at position %d", tok.Data, tok.Index)
}

// ============================================================================
// AST Flattener: Extract stages from -> chain
// ============================================================================

// flattenSequence walks the AST and extracts ordered stages from the -> chain.
// For each stage, it determines if it's an absence stage (top-level !) and
// collects all referenced event IDs.
func flattenSequence(ast ExprAST) []CEPStage {
	stages := make([]CEPStage, 0, 4)
	flattenSequenceRecursive(ast, &stages)
	return stages
}

func flattenSequenceRecursive(ast ExprAST, stages *[]CEPStage) {
	switch node := ast.(type) {
	case BinaryExprAST:
		if node.Op == CEPOpSequence {
			// Split by ->: flatten left and right
			flattenSequenceRecursive(node.Lhs, stages)
			flattenSequenceRecursive(node.Rhs, stages)
			return
		}
	}

	// This is a single stage (not a -> node)
	stage := CEPStage{}

	// Check if top-level is ! (absence)
	if unary, ok := ast.(UnaryExprAST); ok && unary.Op == CEPOpNot {
		stage.IsAbsent = true
		stage.Expr = unary.Operand
	} else {
		stage.Expr = ast
	}

	stage.EventIDs = collectEventIDs(stage.Expr)
	*stages = append(*stages, stage)
}

// containsSequenceOp checks if an AST subtree contains the -> operator.
// Used to reject expressions like !(a -> b) where -> appears inside a stage.
func containsSequenceOp(ast ExprAST) bool {
	switch node := ast.(type) {
	case BinaryExprAST:
		if node.Op == CEPOpSequence {
			return true
		}
		return containsSequenceOp(node.Lhs) || containsSequenceOp(node.Rhs)
	case UnaryExprAST:
		return containsSequenceOp(node.Operand)
	default:
		return false
	}
}

// collectEventIDs extracts all literal event IDs from an AST subtree.
func collectEventIDs(ast ExprAST) []string {
	ids := make([]string, 0, 4)
	collectEventIDsRecursive(ast, &ids)
	return ids
}

func collectEventIDsRecursive(ast ExprAST, ids *[]string) {
	switch node := ast.(type) {
	case NumberExprAST:
		*ids = append(*ids, node.Val)
	case BinaryExprAST:
		collectEventIDsRecursive(node.Lhs, ids)
		collectEventIDsRecursive(node.Rhs, ids)
	case UnaryExprAST:
		collectEventIDsRecursive(node.Operand, ids)
	}
}

// ============================================================================
// Top-Level Parse Function
// ============================================================================

// ParseCEPCondition parses a CEP condition expression string into a CEPCondition.
// Returns an error if the expression is syntactically invalid.
//
// Examples:
//
//	"a -> b"               -> 2 stages: [a], [b]
//	"a -> b -> c"          -> 3 stages: [a], [b], [c]
//	"a -> (b or c)"        -> 2 stages: [a], [b or c]
//	"(a and b) -> c"       -> 2 stages: [a and b], [c]
//	"a -> !b"              -> 2 stages: [a], [!b (absent)]
//	"a -> !b -> c"         -> 3 stages: [a], [!b (absent)], [c]
func ParseCEPCondition(expr string) (*CEPCondition, error) {
	tokens, allLiterals, err := tokenizeCEPCondition(expr)
	if err != nil {
		return nil, fmt.Errorf("tokenize error: %w", err)
	}

	parser := newCEPParser(tokens)
	ast, err := parser.parseSequenceExpr()
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Ensure all tokens were consumed
	if !parser.atEnd() {
		return nil, fmt.Errorf("unexpected token '%s' at position %d", parser.peek().Data, parser.peek().Index)
	}

	// Flatten AST into stages
	stages := flattenSequence(ast)

	// Validate: must have at least 2 stages (otherwise it's not a sequence)
	if len(stages) < 2 {
		return nil, fmt.Errorf("sequence condition must have at least 2 stages separated by '->', got %d", len(stages))
	}

	// Validate: first stage cannot be absent
	if stages[0].IsAbsent {
		return nil, errors.New("first stage in sequence cannot be an absence check (!)")
	}

	// Validate: no stage expression may contain the -> operator (nested sequences are not supported)
	for i, stage := range stages {
		if containsSequenceOp(stage.Expr) {
			return nil, fmt.Errorf("nested sequence operator '->' is not allowed inside stage %d; use only 'and'/'or' within a stage", i)
		}
	}

	return &CEPCondition{
		Raw:       expr,
		Stages:    stages,
		AllEvents: allLiterals,
	}, nil
}

// ============================================================================
// Stage Evaluation (Phase 1+2: match event against stage expressions)
// ============================================================================

// EvaluateEvent determines which stages the current event satisfies.
// matchMap contains the result of checking the event against all event definitions:
//
//	{"login": true, "exfil": false, "scan": true}
//
// Returns a list of stage indices that this event satisfies.
func (c *CEPCondition) EvaluateEvent(matchMap map[string]bool) []int {
	matched := make([]int, 0, len(c.Stages))

	for i, stage := range c.Stages {
		if evaluateStageExpr(stage.Expr, matchMap) {
			matched = append(matched, i)
		}
	}

	return matched
}

// evaluateStageExpr evaluates a within-stage expression against a boolean match map.
// Handles and/or/! logic on a single event's match results.
func evaluateStageExpr(expr ExprAST, matchMap map[string]bool) bool {
	switch node := expr.(type) {
	case NumberExprAST:
		return matchMap[node.Val]
	case BinaryExprAST:
		l := evaluateStageExpr(node.Lhs, matchMap)
		r := evaluateStageExpr(node.Rhs, matchMap)
		switch node.Op {
		case CEPOpAnd:
			return l && r
		case CEPOpOr:
			return l || r
		}
	case UnaryExprAST:
		if node.Op == CEPOpNot {
			return !evaluateStageExpr(node.Operand, matchMap)
		}
	}
	return false
}

// EvaluateEventBindings returns stage -> matched event IDs for stages satisfied by this event.
// For OR expressions, the first satisfied branch (left-first) is selected.
func (c *CEPCondition) EvaluateEventBindings(matchMap map[string]bool) map[int][]string {
	bindings := make(map[int][]string, len(c.Stages))
	for i, stage := range c.Stages {
		ids, ok := matchedEventIDsForStageExpr(stage.Expr, matchMap)
		if ok {
			bindings[i] = ids
		}
	}
	return bindings
}

func matchedEventIDsForStageExpr(expr ExprAST, matchMap map[string]bool) ([]string, bool) {
	switch node := expr.(type) {
	case NumberExprAST:
		if matchMap[node.Val] {
			return []string{node.Val}, true
		}
		return nil, false
	case BinaryExprAST:
		switch node.Op {
		case CEPOpAnd:
			leftIDs, leftOK := matchedEventIDsForStageExpr(node.Lhs, matchMap)
			if !leftOK {
				return nil, false
			}
			rightIDs, rightOK := matchedEventIDsForStageExpr(node.Rhs, matchMap)
			if !rightOK {
				return nil, false
			}
			return mergeEventIDs(leftIDs, rightIDs), true
		case CEPOpOr:
			// OR binding policy: first satisfied branch wins (left-first).
			if leftIDs, leftOK := matchedEventIDsForStageExpr(node.Lhs, matchMap); leftOK {
				return leftIDs, true
			}
			return matchedEventIDsForStageExpr(node.Rhs, matchMap)
		default:
			// "->" is not expected inside a single stage expression.
			return nil, false
		}
	case UnaryExprAST:
		if node.Op == CEPOpNot {
			operandIDs, operandOK := matchedEventIDsForStageExpr(node.Operand, matchMap)
			if !operandOK {
				// "!expr" satisfied -> no positive event binding.
				return nil, true
			}
			// Operand matched means "!expr" not satisfied.
			_ = operandIDs
			return nil, false
		}
	}
	return nil, false
}

func mergeEventIDs(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	seen := make(map[string]bool, len(a)+len(b))
	for _, v := range a {
		if !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	for _, v := range b {
		if !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	return out
}

// ============================================================================
// Sequence Completion Check (Phase 3: verify temporal ordering)
// ============================================================================

// CheckComplete checks if the sequence is fully satisfied given the current state.
// For each stage (in order), find a match whose timestamp is strictly after the
// previous stage's match timestamp, verifying temporal ordering.
//
// Absence stages are handled specially:
//   - An absence stage is satisfied when NO match exists for it with timestamp > prevTimestamp.
//   - If an absence stage has any match with timestamp > prevTimestamp, the sequence
//     is NOT complete (the absent event was observed).
//
// Uses memoization keyed on (stageIdx, prevTimestamp) to avoid exponential backtracking
// in sequences with absence stages. Worst-case complexity: O(N * M) where N = stages, M = max matches per stage.
//
// Returns true if the entire sequence is satisfied.
func (c *CEPCondition) CheckComplete(state *SequenceState) bool {
	if state == nil || len(state.StageMatches) == 0 {
		return false
	}

	memo := make(map[[2]int64]bool)
	return c.checkCompleteFromStage(state, 0, 0, memo)
}

// checkCompleteFromStage recursively checks completion starting from stageIdx
// with the constraint that all matches must have timestamp > prevTimestamp.
// Uses memoization to prevent exponential backtracking in pathological cases.
func (c *CEPCondition) checkCompleteFromStage(state *SequenceState, stageIdx int, prevTimestamp int64, memo map[[2]int64]bool) bool {
	if stageIdx >= len(c.Stages) {
		// All stages satisfied
		return true
	}

	memoKey := [2]int64{int64(stageIdx), prevTimestamp}
	if cached, found := memo[memoKey]; found {
		return cached
	}

	stage := c.Stages[stageIdx]
	var result bool

	if stage.IsAbsent {
		// Absence stage: check that NO match exists after prevTimestamp
		matches, exists := state.StageMatches[stageIdx]
		if exists {
			for _, m := range matches {
				if m.Timestamp > prevTimestamp {
					// Absent event was observed -> sequence NOT complete
					memo[memoKey] = false
					return false
				}
			}
		}
		// No match found after prevTimestamp -> absence satisfied
		// Continue to next stage with the same prevTimestamp
		// (absence doesn't advance the time cursor)
		result = c.checkCompleteFromStage(state, stageIdx+1, prevTimestamp, memo)
	} else {
		// Normal stage: find a match with timestamp > prevTimestamp that allows completion
		matches, exists := state.StageMatches[stageIdx]
		if !exists || len(matches) == 0 {
			memo[memoKey] = false
			return false
		}

		for _, m := range matches {
			if m.Timestamp > prevTimestamp {
				// Try this match and continue to next stage
				if c.checkCompleteFromStage(state, stageIdx+1, m.Timestamp, memo) {
					result = true
					break
				}
			}
		}
	}

	memo[memoKey] = result
	return result
}

// CheckAbsenceTimeout checks if any absence stages have timed out (for deferred triggering).
// This is called when the sequence has not completed normally but the time window has expired.
// Returns true if the sequence should trigger due to absence timeout.
//
// Logic: find the furthest stage that has been reached. If the next stage is an
// absence stage and the time window has expired, the absence is confirmed.
// Uses memoization to prevent exponential backtracking.
func (c *CEPCondition) CheckAbsenceTimeout(state *SequenceState, nowMs int64) bool {
	if state == nil || nowMs < state.ExpiresAt {
		return false
	}

	// Check if we can complete the sequence with absence timeouts
	memo := make(map[[2]int64]bool)
	return c.checkAbsenceFromStage(state, 0, 0, nowMs, memo)
}

// checkAbsenceFromStage recursively checks if the sequence can complete
// considering that the time window has expired (absence stages auto-satisfy).
// Uses memoization to prevent exponential backtracking.
func (c *CEPCondition) checkAbsenceFromStage(state *SequenceState, stageIdx int, prevTimestamp int64, nowMs int64, memo map[[2]int64]bool) bool {
	if stageIdx >= len(c.Stages) {
		return true
	}

	memoKey := [2]int64{int64(stageIdx), prevTimestamp}
	if cached, found := memo[memoKey]; found {
		return cached
	}

	stage := c.Stages[stageIdx]
	var result bool

	if stage.IsAbsent {
		// Absence stage at timeout: check that no match exists after prevTimestamp
		matches, exists := state.StageMatches[stageIdx]
		if exists {
			for _, m := range matches {
				if m.Timestamp > prevTimestamp {
					// Absent event was observed -> cannot trigger
					memo[memoKey] = false
					return false
				}
			}
		}
		// Absence confirmed (timed out, event never appeared)
		result = c.checkAbsenceFromStage(state, stageIdx+1, prevTimestamp, nowMs, memo)
	} else {
		// Normal stage: must have a match
		matches, exists := state.StageMatches[stageIdx]
		if !exists || len(matches) == 0 {
			memo[memoKey] = false
			return false
		}

		for _, m := range matches {
			if m.Timestamp > prevTimestamp {
				if c.checkAbsenceFromStage(state, stageIdx+1, m.Timestamp, nowMs, memo) {
					result = true
					break
				}
			}
		}
	}

	memo[memoKey] = result
	return result
}

// HasAbsenceStages returns true if any stage in the condition is an absence stage.
func (c *CEPCondition) HasAbsenceStages() bool {
	for _, stage := range c.Stages {
		if stage.IsAbsent {
			return true
		}
	}
	return false
}

// StageCount returns the number of stages in the condition.
func (c *CEPCondition) StageCount() int {
	return len(c.Stages)
}

// GetStageEventIDs returns the event IDs referenced in a specific stage.
func (c *CEPCondition) GetStageEventIDs(stageIdx int) []string {
	if stageIdx < 0 || stageIdx >= len(c.Stages) {
		return nil
	}
	return c.Stages[stageIdx].EventIDs
}
