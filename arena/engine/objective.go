package engine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"
	"github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
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
	// rewinds lists the exchanges removed from the conversation, in order.
	rewinds []rewoundExchange
}

// rewoundExchange is one exchange removed from the conversation by the
// objective's rewind rule.
type rewoundExchange struct {
	// exchange is the 1-based exchange that was removed.
	exchange int
	// message is what the persona said; response is the agent's last reply.
	message  string
	response string
	// checks holds the rewind assertions' results that triggered the rewind.
	checks []asrt.ConversationValidationResult
	// cost is what the removed messages cost; it still counts towards the run.
	cost types.CostInfo
}

func (r *rewoundExchange) detail() map[string]interface{} {
	return map[string]interface{}{
		"exchange": r.exchange,
		"message":  r.message,
		"response": r.response,
		"checks":   r.checks,
		"cost_usd": r.cost.TotalCost,
	}
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
	if len(o.rewinds) > 0 {
		rewinds := make([]map[string]interface{}, len(o.rewinds))
		for i := range o.rewinds {
			rewinds[i] = o.rewinds[i].detail()
		}
		details["rewinds"] = rewinds
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
	var msg string
	switch {
	case o.evalErr != "":
		msg = fmt.Sprintf("%s: could not be evaluated: %s", label, o.evalErr)
	case o.achieved():
		msg = fmt.Sprintf("%s: achieved at exchange %d of %d", label, o.achievedAt, o.exchanges)
	default:
		msg = fmt.Sprintf("%s: not achieved in %d exchanges", label, o.exchanges)
	}
	if n := len(o.rewinds); n > 0 {
		msg += fmt.Sprintf(" (%d rewound)", n)
	}
	return msg
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
	// startLen is how many messages the conversation held before the current
	// exchange, and startKnown whether that is known; a rewind cuts the
	// conversation back to startLen. afterExchange carries it forward, so the
	// conversation is only loaded for it when it is not known.
	startLen   int
	startKnown bool
	// lastRewound is the exchange the previous exchange's rewind removed, or
	// nil when the previous exchange was kept.
	lastRewound *rewoundExchange
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

// beforeExchange notes where the next exchange starts, so it can be rewound.
// It does nothing unless the objective has a rewind rule.
func (t *objectiveTracker) beforeExchange(ctx context.Context) {
	if t == nil || t.outcome.cfg.Rewind == nil || t.startKnown {
		return
	}
	messages, err := t.ce.loadMessagesFromStateStore(ctx, t.req)
	t.startLen, t.startKnown = len(messages), err == nil
}

// afterExchange evaluates the objective once exchange (1-based) has
// completed, and reports whether the self-play turn should stop and whether
// the exchange was rewound. An evaluation error is kept only until a later
// exchange evaluates cleanly, so one transient failure does not decide the
// outcome.
func (t *objectiveTracker) afterExchange(ctx context.Context, exchange int) (stop, rewound bool) {
	if t == nil {
		return false, false
	}
	t.outcome.exchanges = exchange
	t.lastRewound = nil

	if t.orch == nil {
		t.outcome.evalErr = objectiveErrNoRunner
		t.outcome.checks = nil
		t.startKnown = false
		return false, false
	}
	// A run that hit its timeout still has its last exchange judged, as
	// conversation assertions are.
	ctx = scoringContext(ctx)
	messages, err := t.ce.loadMessagesFromStateStore(ctx, t.req)
	if err != nil {
		// Clear the checks so the persona is not shown results from an
		// exchange that was not re-checked.
		t.outcome.evalErr = err.Error()
		t.outcome.checks = nil
		t.startKnown = false
		return false, false
	}
	t.outcome.evalErr = ""
	prevChecks := t.outcome.checks
	// The checks are re-run even once the objective is achieved, so the
	// persona's feedback always describes the latest exchange.
	checks := t.orch.RunAssertionsAsConversationResults(
		ctx, t.outcome.cfg.Success, messages,
		len(messages)-1, t.req.ConversationID,
		evals.TriggerOnConversationComplete,
	)
	t.outcome.checks = checks
	if t.outcome.achieved() {
		t.keepExchange(messages)
		return t.outcome.cfg.StopsOnSuccess(), false
	}
	if !allPassed(checks, len(t.outcome.cfg.Success)) {
		if t.maybeRewind(ctx, exchange, messages) {
			// The checks described the removed exchange; the conversation
			// is back to what the previous checks saw.
			t.outcome.checks = prevChecks
			return false, true
		}
		t.keepExchange(messages)
		return false, false
	}
	t.keepExchange(messages)
	t.outcome.achievedAt = exchange
	logger.Debug("Self-play objective achieved",
		"turn", t.outcome.turnIndex, "exchange", exchange)
	return t.outcome.cfg.StopsOnSuccess(), false
}

// keepExchange records that the conversation, as loaded after the exchange,
// is where the next exchange starts.
func (t *objectiveTracker) keepExchange(messages []types.Message) {
	t.startLen, t.startKnown = len(messages), true
}

// maybeRewind removes the exchange just run from the conversation when the
// objective's rewind assertions all pass on it and the turn has rewinds
// left, and reports whether it did. An exchange with a failed turn assertion or guardrail
// result is kept, so the failure stays in the report.
func (t *objectiveTracker) maybeRewind(ctx context.Context, exchange int, messages []types.Message) bool {
	rule := t.outcome.cfg.Rewind
	if rule == nil || !t.startKnown || t.startLen >= len(messages) ||
		len(t.outcome.rewinds) >= rule.MaxRewinds() {
		return false
	}
	// The rewind checks see only the exchange just run: a conversation-level
	// check reads every reply, so one matching an earlier, kept reply would
	// otherwise rewind every exchange after it.
	dropped := messages[t.startLen:]
	if hasFailedTurnChecks(dropped) {
		return false
	}
	checks := t.orch.RunAssertionsAsConversationResults(
		ctx, rule.When, dropped,
		len(dropped)-1, t.req.ConversationID,
		evals.TriggerOnConversationComplete,
	)
	if !allPassed(checks, len(rule.When)) {
		return false
	}
	rec := rewoundExchange{exchange: exchange, checks: checks}
	for i := range dropped {
		msgCost(&rec.cost, &dropped[i])
		switch dropped[i].Role {
		case roleUser:
			if rec.message == "" {
				rec.message = dropped[i].GetContent()
			}
		case roleAssistant:
			if c := dropped[i].GetContent(); c != "" {
				rec.response = c
			}
		}
	}
	if err := t.truncate(ctx, &rec.cost); err != nil {
		logger.Warn("Self-play rewind failed; keeping the exchange",
			"turn", t.outcome.turnIndex, "exchange", exchange, "error", err)
		return false
	}
	t.outcome.rewinds = append(t.outcome.rewinds, rec)
	t.lastRewound = &t.outcome.rewinds[len(t.outcome.rewinds)-1]
	logger.Debug("Self-play exchange rewound",
		"turn", t.outcome.turnIndex, "exchange", exchange)
	return true
}

// truncate cuts the stored conversation back to startLen messages, adding
// cost (what the removed messages cost) to the conversation's rewound-cost
// metadata so the run's total still includes it. It runs between exchanges,
// outside the pipeline, so replacing the whole state is safe.
func (t *objectiveTracker) truncate(ctx context.Context, cost *types.CostInfo) error {
	store, err := stateStoreFor(t.req)
	if err != nil {
		return err
	}
	writer, ok := store.(statestore.BulkWriter)
	if !ok {
		return errors.New("the state store cannot replace a conversation")
	}
	state, err := store.Load(ctx, t.req.ConversationID)
	if err != nil {
		return err
	}
	if len(state.Messages) < t.startLen {
		return fmt.Errorf("conversation has %d messages, fewer than the %d before the exchange",
			len(state.Messages), t.startLen)
	}
	state.Messages = state.Messages[:t.startLen]
	if state.Metadata == nil {
		state.Metadata = map[string]interface{}{}
	}
	state.Metadata[rewoundCostMetaKey] = addCostToMeta(state.Metadata[rewoundCostMetaKey], cost)
	return writer.Save(ctx, state)
}

// hasFailedTurnChecks reports whether any message carries a failed turn
// assertion or guardrail result.
func hasFailedTurnChecks(messages []types.Message) bool {
	for i := range messages {
		for j := range messages[i].Validations {
			if !messages[i].Validations[j].Passed {
				return true
			}
		}
		for _, r := range trialTurnResults(&messages[i]) {
			if passed, ok := r["passed"].(bool); ok && !passed {
				return true
			}
		}
	}
	return false
}

// personaVariables returns the persona's template variables for the
// exchange about to be generated (1-based): its number and, on a turn with an
// objective, the objective, whether it has been achieved, and the success
// checks from the previous exchange. A nil tracker gives the exchange number
// alone.
func (t *objectiveTracker) personaVariables(exchange int) map[string]string {
	vars := map[string]string{selfplay.VarExchange: strconv.Itoa(exchange)}
	if t == nil {
		return vars
	}
	vars[selfplay.VarObjective] = t.outcome.cfg.Description
	vars[selfplay.VarObjectiveStatus] = objectiveStatusNotAchieved
	if t.outcome.achieved() {
		vars[selfplay.VarObjectiveStatus] = objectiveStatusAchieved
	}
	vars[selfplay.VarObjectiveFeedback] = checksFeedback(t.outcome.checks)
	if r := t.lastRewound; r != nil {
		vars[selfplay.VarRewoundMessage] = r.message
		vars[selfplay.VarRewoundResponse] = r.response
	}
	if rule := t.outcome.cfg.Rewind; rule != nil {
		vars[selfplay.VarRewindsLeft] = strconv.Itoa(rule.MaxRewinds() - len(t.outcome.rewinds))
	}
	return vars
}

// Values of the objective_status persona variable.
const (
	objectiveStatusAchieved    = "achieved"
	objectiveStatusNotAchieved = "not achieved"
)

// States a success check is reported in, in objective_feedback.
const (
	checkStatePassed  = "passed"
	checkStateFailed  = "failed"
	checkStateSkipped = "skipped"
)

// checksFeedback renders success-check results for the persona, one line per
// check: "- <type>: passed|failed — <message>".
func checksFeedback(checks []asrt.ConversationValidationResult) string {
	lines := make([]string, 0, len(checks))
	for i := range checks {
		state := checkStateFailed
		if _, skipped := asrt.IsSkipped(checks[i].Details); skipped {
			state = checkStateSkipped
		} else if checks[i].Passed {
			state = checkStatePassed
		}
		line := fmt.Sprintf("- %s: %s", checks[i].Type, state)
		if checks[i].Message != "" {
			line += " — " + checks[i].Message
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
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
