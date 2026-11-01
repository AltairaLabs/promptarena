package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"
)

// objectiveResultType is the conversation-result type an objective outcome
// is reported under.
const objectiveResultType = "objective"

// Detail keys and messages used in objective results.
const (
	objectiveKeyTurnIndex   = "turn_index"
	objectiveKeyDescription = "description"
	objectiveErrNoRunner    = "eval runner not configured"
)

// objectiveRecorder collects objective outcomes for one conversation.
type objectiveRecorder struct {
	mu       sync.Mutex
	outcomes []objectiveOutcome
}

func (r *objectiveRecorder) record(o objectiveOutcome) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outcomes = append(r.outcomes, o)
}

// results returns one conversation result per recorded objective, in the
// order the turns ran.
func (r *objectiveRecorder) results() []asrt.ConversationValidationResult {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]asrt.ConversationValidationResult, len(r.outcomes))
	for i := range r.outcomes {
		out[i] = r.outcomes[i].result()
	}
	return out
}

// objectiveOutcome is what happened to one self-play turn's objective.
type objectiveOutcome struct {
	turnIndex int
	cfg       *arenaconfig.ObjectiveConfig
	// exchanges is how many self-play exchanges ran.
	exchanges int
	// achievedAt is the 1-based exchange at which the objective was first
	// achieved; 0 when it never was.
	achievedAt int
	// checks holds the success assertions' results from the last evaluation.
	checks []asrt.ConversationValidationResult
	// evalErr is set when the success assertions could not be evaluated.
	evalErr string
}

func (o *objectiveOutcome) achieved() bool { return o.achievedAt > 0 }

// result reports the outcome as a conversation result that passes when the
// outcome matches the objective's expectation.
func (o *objectiveOutcome) result() asrt.ConversationValidationResult {
	expect := arenaconfig.ObjectiveExpectAchieved
	if !o.cfg.ExpectsAchieved() {
		expect = arenaconfig.ObjectiveExpectNotAchieved
	}
	details := map[string]interface{}{
		objectiveKeyTurnIndex:   o.turnIndex,
		objectiveKeyDescription: o.cfg.Description,
		"expect":                expect,
		"achieved":              o.achieved(),
		"exchanges":             o.exchanges,
		"success_checks":        o.checks,
	}
	if o.achieved() {
		details["achieved_at_exchange"] = o.achievedAt
	}
	if o.evalErr != "" {
		details["error"] = o.evalErr
	}
	return asrt.ConversationValidationResult{
		Type:    objectiveResultType,
		Passed:  o.evalErr == "" && o.achieved() == o.cfg.ExpectsAchieved(),
		Message: o.message(),
		Details: details,
	}
}

func (o *objectiveOutcome) message() string {
	label := o.cfg.Description
	if label == "" {
		label = fmt.Sprintf("turn %d objective", o.turnIndex)
	}
	switch {
	case o.evalErr != "":
		return fmt.Sprintf("%s: could not be evaluated: %s", label, o.evalErr)
	case o.achieved():
		return fmt.Sprintf("%s: achieved at exchange %d of %d", label, o.achievedAt, o.exchanges)
	default:
		return fmt.Sprintf("%s: not achieved in %d exchanges", label, o.exchanges)
	}
}

// objectiveTracker checks one self-play turn's objective after each exchange.
// A nil tracker (no objective on the turn) does nothing.
type objectiveTracker struct {
	ce  *DefaultConversationExecutor
	req *ConversationRequest
	// orch evaluates the success checks without emitting eval events: the
	// checks are probes, and only the final objective result is reported.
	orch    *EvalOrchestrator
	outcome objectiveOutcome
}

// newObjectiveTracker returns a tracker for the turn, or nil when the turn
// has no objective.
func (ce *DefaultConversationExecutor) newObjectiveTracker(
	req *ConversationRequest, turnIdx int, turn *arenaconfig.TurnDefinition,
) *objectiveTracker {
	if turn.Objective == nil {
		return nil
	}
	return &objectiveTracker{
		ce:      ce,
		req:     req,
		orch:    ce.resolveEvalOrchestrator(req).Quiet(),
		outcome: objectiveOutcome{turnIndex: turnIdx, cfg: turn.Objective},
	}
}

// afterExchange evaluates the objective once exchange (1-based) has
// completed, and reports whether the self-play turn should stop. An
// evaluation error is kept only until a later exchange evaluates cleanly, so
// one transient failure does not decide the outcome.
func (t *objectiveTracker) afterExchange(ctx context.Context, exchange int) bool {
	if t == nil {
		return false
	}
	t.outcome.exchanges = exchange
	if t.outcome.achieved() {
		return t.outcome.cfg.StopsOnSuccess()
	}

	if t.orch == nil {
		t.outcome.evalErr = objectiveErrNoRunner
		return false
	}
	// A run that hit its timeout still has its last exchange judged, as
	// conversation assertions are.
	ctx = scoringContext(ctx)
	_, messages, err := t.ce.loadMessagesFromStateStore(ctx, t.req)
	if err != nil {
		t.outcome.evalErr = err.Error()
		return false
	}
	t.outcome.evalErr = ""
	checks := t.orch.RunAssertionsAsConversationResults(
		ctx, t.outcome.cfg.Success, messages,
		len(messages)-1, t.req.ConversationID,
		evals.TriggerOnConversationComplete,
	)
	t.outcome.checks = checks
	if !allPassed(checks, len(t.outcome.cfg.Success)) {
		return false
	}
	t.outcome.achievedAt = exchange
	logger.Debug("Self-play objective achieved",
		"turn", t.outcome.turnIndex, "exchange", exchange)
	return t.outcome.cfg.StopsOnSuccess()
}

// finish records the outcome on the conversation.
func (t *objectiveTracker) finish() {
	if t == nil {
		return
	}
	t.req.objectives.record(t.outcome)
}

// allPassed reports whether every expected check ran and passed. A short
// result list (an assertion that produced no result) and a skipped check
// (an unmet when condition, no judge configured) count as not passed: the
// objective is achieved only when every check was actually made.
func allPassed(checks []asrt.ConversationValidationResult, want int) bool {
	if len(checks) < want {
		return false
	}
	for i := range checks {
		if _, skipped := asrt.IsSkipped(checks[i].Details); skipped || !checks[i].Passed {
			return false
		}
	}
	return true
}

// objectiveRoleError reports an objective on a turn that does not run
// self-play, which would otherwise never be checked.
func (ce *DefaultConversationExecutor) objectiveRoleError(scenario *arenaconfig.Scenario) error {
	for i := range scenario.Turns {
		turn := &scenario.Turns[i]
		if turn.Objective != nil && !ce.isSelfPlayRole(turn.Role) {
			return fmt.Errorf("turns[%d].objective: role %q is not a self-play role", i, turn.Role)
		}
	}
	return nil
}

// hasObjectives reports whether any turn in the scenario has an objective.
func hasObjectives(scenario *arenaconfig.Scenario) bool {
	for i := range scenario.Turns {
		if scenario.Turns[i].Objective != nil {
			return true
		}
	}
	return false
}
