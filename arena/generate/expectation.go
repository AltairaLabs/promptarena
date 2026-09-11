package generate

import (
	"fmt"
	"strconv"
	"strings"
)

// Expectation is a user-stated range for an eval's score: "faithfulness>=0.8".
// It does two jobs with one input: it selects sessions whose measurement fell
// outside the range (ListOptions.Expectations) and it supplies the threshold
// that turns that measurement into a scenario assertion (min_score/max_score).
type Expectation struct {
	EvalID string
	Min    *float64
	Max    *float64
}

// Symbolic operators accepted in an expectation, and the spec's word forms
// (schema guide: "gte", "lte", "gt", "lt", "eq") accepted in a pack threshold.
const (
	opGTE, opGT, opLTE, opLT, opEQ, opAssign = ">=", ">", "<=", "<", "==", "="
	wordGTE, wordGT, wordLTE, wordLT, wordEQ = "gte", "gt", "lte", "lt", "eq"
)

// expectationOperators are every symbolic operator ParseExpectation accepts.
var expectationOperators = []string{opGTE, opLTE, opEQ, opGT, opLT, opAssign}

// ParseExpectation parses "<eval-id><op><number>" where op is one of
// >=, <=, ==, =, >, <. Whitespace around the operator is ignored.
func ParseExpectation(s string) (Expectation, error) {
	idx, op := firstOperator(s)
	if idx < 0 {
		return Expectation{}, fmt.Errorf("expectation %q: want <eval-id><op><number>, e.g. faithfulness>=0.8", s)
	}
	id := strings.TrimSpace(s[:idx])
	rest := strings.TrimSpace(s[idx+len(op):])
	if id == "" || rest == "" {
		return Expectation{}, fmt.Errorf("expectation %q: want <eval-id><op><number>", s)
	}
	value, err := strconv.ParseFloat(rest, 64)
	if err != nil {
		return Expectation{}, fmt.Errorf("expectation %q: %q is not a number", s, rest)
	}
	return expectationFor(id, op, value)
}

// firstOperator finds the leftmost operator in s, preferring the longer form
// when two start at the same position (">=" over ">"). Returns -1 when none.
func firstOperator(s string) (int, string) {
	best, bestOp := -1, ""
	for _, op := range expectationOperators {
		idx := strings.Index(s, op)
		if idx < 0 {
			continue
		}
		if best < 0 || idx < best || (idx == best && len(op) > len(bestOp)) {
			best, bestOp = idx, op
		}
	}
	return best, bestOp
}

func expectationFor(id, op string, value float64) (Expectation, error) {
	e := Expectation{EvalID: id}
	switch op {
	case opGTE, opGT, wordGTE, wordGT:
		e.Min = &value
	case opLTE, opLT, wordLTE, wordLT:
		e.Max = &value
	case opEQ, opAssign, wordEQ:
		e.Min, e.Max = &value, &value
	default:
		return Expectation{}, fmt.Errorf("expectation for %q: unknown operator %q", id, op)
	}
	return e, nil
}

// Violated reports whether a score falls outside the range. A nil score is a
// violation: a measurement that never produced a value cannot satisfy an
// expectation, and hiding that would hide the very session the user asked for.
func (e Expectation) Violated(score *float64) bool {
	if score == nil {
		return true
	}
	if e.Min != nil && *score < *e.Min {
		return true
	}
	if e.Max != nil && *score > *e.Max {
		return true
	}
	return false
}

// Bounds returns the min_score/max_score the expectation becomes.
func (e Expectation) Bounds() (minScore, maxScore *float64) {
	return e.Min, e.Max
}

// BoundsFromThreshold translates a pack threshold into min_score/max_score.
// min_score and max_score are inclusive, so a strict operator loses its
// strictness; note says so, and is empty when nothing was lost. An operator
// this cannot translate yields nil bounds and a note naming it.
func BoundsFromThreshold(t *EvalThreshold) (minScore, maxScore *float64, note string) {
	if t == nil {
		return nil, nil, ""
	}
	v := t.Value
	switch strings.ToLower(strings.TrimSpace(t.Operator)) {
	case wordGTE, opGTE:
		return &v, nil, ""
	case wordGT, opGT:
		return &v, nil, fmt.Sprintf("pack threshold is > %v but min_score is inclusive; asserting >= %v", v, v)
	case wordLTE, opLTE:
		return nil, &v, ""
	case wordLT, opLT:
		return nil, &v, fmt.Sprintf("pack threshold is < %v but max_score is inclusive; asserting <= %v", v, v)
	case wordEQ, opEQ, opAssign:
		return &v, &v, ""
	}
	return nil, nil, fmt.Sprintf(
		"pack threshold operator %q is not one this can express as min_score/max_score", t.Operator)
}
