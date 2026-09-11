package generate

import (
	"fmt"
	"strings"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/arena/assertions"
)

// boundParamCount is how many keys a bound can add to an assertion's params
// (min_score and max_score).
const boundParamCount = 2

// DecisionOutcome is what a recorded eval became in the scenario.
type DecisionOutcome string

const (
	// DecisionAssertedVerdict means the eval carried a failed verdict and
	// becomes an assertion with its recorded params.
	DecisionAssertedVerdict DecisionOutcome = "asserted-verdict"
	// DecisionAssertedThreshold means a measurement was bounded by the pack's
	// declared threshold.
	DecisionAssertedThreshold DecisionOutcome = "asserted-threshold"
	// DecisionAssertedExpectation means a measurement was bounded by a user
	// expectation.
	DecisionAssertedExpectation DecisionOutcome = "asserted-expectation"
	// DecisionDroppedPassed means the eval carried a verdict and it passed;
	// there is nothing to regress against.
	DecisionDroppedPassed DecisionOutcome = "dropped-passed"
	// DecisionReported means a measurement had no known range. It is reported
	// so the user can supply one, never asserted against an invented bound.
	DecisionReported DecisionOutcome = "reported"
)

// Decision explains what one recorded eval became and why. The CLI prints
// them; a library caller can inspect them.
type Decision struct {
	SessionID string
	EvalID    string
	EvalType  string
	Turn      *int
	Score     *float64
	Outcome   DecisionOutcome
	MinScore  *float64
	MaxScore  *float64
	Reason    string
}

// String renders one line for a log.
func (d Decision) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "session %s eval %s (%s)", d.SessionID, d.EvalID, d.EvalType)
	if d.Turn != nil {
		fmt.Fprintf(&b, " turn %d", *d.Turn)
	}
	if d.Score != nil {
		fmt.Fprintf(&b, " scored %v", *d.Score)
	}
	fmt.Fprintf(&b, ": %s", d.Outcome)
	if d.MinScore != nil {
		fmt.Fprintf(&b, " min_score %v", *d.MinScore)
	}
	if d.MaxScore != nil {
		fmt.Fprintf(&b, " max_score %v", *d.MaxScore)
	}
	if d.Reason != "" {
		fmt.Fprintf(&b, " — %s", d.Reason)
	}
	return b.String()
}

// applyEvalDecisions walks the session's recorded evals, decides what each
// becomes, attaches the resulting assertions to the scenario, and records
// every decision on the conversion.
//
// Precedence: a recorded verdict; then the pack's declared threshold; then a
// user expectation; else report. Nothing invents a bound.
func applyEvalDecisions(scenario *arenaconfig.Scenario, session *SessionDetail, opts ConvertOptions, conv *Conversion) {
	expectations := make(map[string]Expectation, len(opts.Expectations))
	for _, e := range opts.Expectations {
		expectations[e.EvalID] = e
	}

	for i := range session.Evals {
		ev := &session.Evals[i]
		d, ac := decideEval(session.ID, ev, expectations)
		if ac != nil {
			place(scenario, ev.Turn, *ac, &d)
		}
		conv.Decisions = append(conv.Decisions, d)
	}
}

// decideEval returns the decision and, when the eval becomes an assertion, the
// assertion config to attach.
func decideEval(
	sessionID string, ev *EvalResult, expectations map[string]Expectation,
) (Decision, *assertions.AssertionConfig) {
	d := Decision{SessionID: sessionID, EvalID: ev.ID, EvalType: ev.Type, Turn: ev.Turn, Score: ev.Score}

	// 1. A recorded verdict.
	if ev.Passed != nil {
		if *ev.Passed {
			d.Outcome = DecisionDroppedPassed
			d.Reason = "recorded verdict passed; nothing to regress against"
			return d, nil
		}
		d.Outcome = DecisionAssertedVerdict
		d.Reason = "recorded verdict failed; asserting with the recorded params"
		return d, assertionFrom(ev, nil, nil)
	}

	// 2. The pack's declared threshold.
	if ev.Threshold != nil {
		minS, maxS, note := BoundsFromThreshold(ev.Threshold)
		if minS != nil || maxS != nil {
			d.Outcome = DecisionAssertedThreshold
			d.MinScore, d.MaxScore = minS, maxS
			d.Reason = fmt.Sprintf("pack threshold %s %v (recorded score %s)",
				ev.Threshold.Operator, ev.Threshold.Value, scoreString(ev.Score))
			if note != "" {
				d.Reason += "; " + note
			}
			return d, assertionFrom(ev, minS, maxS)
		}
		d.Reason = note + "; "
	}

	// 3. A user expectation.
	if e, ok := expectations[ev.ID]; ok {
		minS, maxS := e.Bounds()
		d.Outcome = DecisionAssertedExpectation
		d.MinScore, d.MaxScore = minS, maxS
		d.Reason += fmt.Sprintf("user expectation (recorded score %s)", scoreString(ev.Score))
		return d, assertionFrom(ev, minS, maxS)
	}

	// 4. Nothing known.
	d.Outcome = DecisionReported
	d.Reason += fmt.Sprintf("scored %s in session %s; no threshold known — pass --expect %s>=<value> "+
		"(or <=) or declare threshold on the eval in the pack", scoreString(ev.Score), sessionID, ev.ID)
	return d, nil
}

// assertionFrom builds the scenario assertion: the eval's own params plus the
// bound. Recorded params are copied, never mutated.
func assertionFrom(ev *EvalResult, minScore, maxScore *float64) *assertions.AssertionConfig {
	params := make(map[string]interface{}, len(ev.Params)+boundParamCount)
	for k, v := range ev.Params {
		params[k] = v
	}
	if minScore != nil {
		params["min_score"] = *minScore
	}
	if maxScore != nil {
		params["max_score"] = *maxScore
	}
	if len(params) == 0 {
		params = nil
	}
	return &assertions.AssertionConfig{Type: ev.Type, Params: params, Message: ev.Message}
}

// place attaches the assertion to the turn it answers, or to the conversation
// when the eval has no turn or names one the scenario does not have.
func place(scenario *arenaconfig.Scenario, turn *int, ac assertions.AssertionConfig, d *Decision) {
	if turn != nil {
		if *turn >= 0 && *turn < len(scenario.Turns) {
			scenario.Turns[*turn].Assertions = append(scenario.Turns[*turn].Assertions, ac)
			return
		}
		d.Reason += fmt.Sprintf("; turn %d is not in the scenario, attached at conversation level", *turn)
	}
	scenario.ConversationAssertions = append(scenario.ConversationAssertions, ac)
}

func scoreString(s *float64) string {
	if s == nil {
		return "none"
	}
	return fmt.Sprintf("%v", *s)
}
