package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// personaStateResultType is the conversation-result type a self-play turn's
// persona state is reported under.
const personaStateResultType = "persona_state"

// Detail keys of a persona state result.
const (
	stateKeyOutcome    = "outcome"
	stateKeyExchange   = "exchange"
	stateKeyStoppedAt  = "stopped_at_exchange"
	stateKeyTransition = "transition"
	// keyExchanges is how many exchanges a self-play turn ran, in objective
	// and persona state results.
	keyExchanges = "exchanges"
)

// personaStateRecorder holds each persona's state for one conversation, so a
// later self-play turn with the same persona carries on from where the last
// one left it, and collects each turn's state result.
type personaStateRecorder struct {
	mu      sync.Mutex
	current map[string]map[string]interface{}
	results []asrt.ConversationValidationResult
}

// load returns the persona's current state, starting it from its initial
// values on first use.
func (r *personaStateRecorder) load(p *arenaconfig.UserPersonaPack) map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.current[p.ID]; ok {
		return s
	}
	return p.InitialState()
}

func (r *personaStateRecorder) save(id string, state map[string]interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		r.current = map[string]map[string]interface{}{}
	}
	r.current[id] = state
}

func (r *personaStateRecorder) record(res asrt.ConversationValidationResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, res)
}

func (r *personaStateRecorder) all() []asrt.ConversationValidationResult {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]asrt.ConversationValidationResult(nil), r.results...)
}

// stateChange is one transition applied after an exchange.
type stateChange struct {
	exchange   int
	transition int
	before     map[string]interface{}
	after      map[string]interface{}
}

// stateTracker applies a persona's state transitions after each exchange of
// one self-play turn and checks its stop conditions. A nil tracker (a persona
// without transitions or stop conditions) does nothing.
type stateTracker struct {
	ce        *DefaultConversationExecutor
	req       *ConversationRequest
	turnIndex int
	persona   *arenaconfig.UserPersonaPack
	// orch evaluates the transitions' checks without emitting eval events.
	orch      *EvalOrchestrator
	initial   map[string]interface{}
	state     map[string]interface{}
	exchanges int
	changes   []stateChange
	// stop is the condition that ended the turn, at exchange stoppedAt.
	stop      *arenaconfig.PersonaStopCondition
	stoppedAt int
	evalErr   string
}

// newStateTracker returns a tracker for the turn, or nil when its persona
// has no transitions or stop conditions.
func (ce *DefaultConversationExecutor) newStateTracker(
	req *ConversationRequest, turnIdx int, turn *arenaconfig.TurnDefinition,
) *stateTracker {
	if ce.selfPlayRegistry == nil || req.personaState == nil {
		return nil
	}
	persona := ce.selfPlayRegistry.GetPersona(turn.Persona)
	if persona == nil || (len(persona.Transitions) == 0 && len(persona.StopWhen) == 0) {
		return nil
	}
	state := req.personaState.load(persona)
	return &stateTracker{
		ce: ce, req: req, turnIndex: turnIdx, persona: persona,
		orch:    ce.resolveEvalOrchestrator(req).Quiet(),
		initial: state, state: state,
	}
}

// addVariables adds the current state to the persona's template variables.
func (t *stateTracker) addVariables(vars map[string]string) {
	if t == nil {
		return
	}
	for k, v := range t.persona.StateVars(t.state) {
		vars[k] = v
	}
}

// afterExchange applies every transition whose checks all pass on the
// exchange just run (1-based), then reports whether a stop condition ends the
// turn; with checkStop false the conditions are not checked. It runs only for
// an exchange that is kept: a rewound exchange never happened, so it changes
// nothing.
func (t *stateTracker) afterExchange(ctx context.Context, exchange int, checkStop bool) (stop bool) {
	if t == nil {
		return false
	}
	t.exchanges = exchange
	// A run that hit its timeout still has its last exchange applied.
	ctx = scoringContext(ctx)
	if len(t.persona.Transitions) > 0 {
		t.applyTransitions(ctx, exchange)
	}
	t.req.personaState.save(t.persona.ID, t.state)
	if !checkStop {
		return false
	}
	return t.checkStop(exchange)
}

// stoppedAtStart reports whether the state the turn starts with already
// meets a stop condition, as when an earlier turn left the persona
// abandoned; the turn then runs no exchanges.
func (t *stateTracker) stoppedAtStart() bool {
	return t != nil && t.checkStop(0)
}

// checkStop records the first stop condition the state meets, after exchange
// (0 before the first), and reports whether there was one.
func (t *stateTracker) checkStop(exchange int) bool {
	for i := range t.persona.StopWhen {
		c := &t.persona.StopWhen[i]
		if c.Matches(t.state[c.State]) {
			t.stop, t.stoppedAt = c, exchange
			logger.Debug("Self-play persona state stopped the turn",
				"turn", t.turnIndex, "exchange", exchange, "outcome", c.OutcomeLabel())
			return true
		}
	}
	return false
}

func (t *stateTracker) applyTransitions(ctx context.Context, exchange int) {
	if t.orch == nil {
		t.evalErr = objectiveErrNoRunner
		return
	}
	messages, err := t.ce.loadMessagesFromStateStore(ctx, t.req)
	if err != nil {
		t.evalErr = err.Error()
		return
	}
	start := lastIndexOfRole(messages, roleUser)
	if start < 0 {
		return
	}
	// The checks see only the exchange just run, as rewind checks do: a
	// conversation-level check reads every reply, so one matching an earlier
	// reply would otherwise fire the transition on every exchange after it.
	agentView := messages[start:]
	personaView := []types.Message{{Role: roleAssistant, Content: selfplay.PersonaText(&messages[start])}}
	for i := range t.persona.Transitions {
		tr := &t.persona.Transitions[i]
		view := agentView
		if tr.ChecksPersona() {
			view = personaView
		}
		checks := t.orch.RunAssertionsAsConversationResults(
			ctx, tr.When, view, len(view)-1, t.req.ConversationID,
			evals.TriggerOnConversationComplete,
		)
		if !allPassed(checks, len(tr.When)) {
			continue
		}
		after := t.persona.Apply(tr, t.state)
		t.changes = append(t.changes, stateChange{exchange: exchange, transition: i, before: t.state, after: after})
		t.state = after
	}
}

// finish records the turn's state result on the conversation.
func (t *stateTracker) finish() {
	if t == nil {
		return
	}
	t.req.personaState.record(t.result())
}

// result reports the turn's state: where it started and ended, each
// transition applied, and the stop condition that ended the turn, if any. It
// fails when that condition says to, or when the transitions could not be
// evaluated, since the state then says nothing about the run.
func (t *stateTracker) result() asrt.ConversationValidationResult {
	changes := make([]map[string]interface{}, len(t.changes))
	for i, c := range t.changes {
		changes[i] = map[string]interface{}{
			stateKeyExchange: c.exchange, stateKeyTransition: c.transition,
			"before": c.before, "after": c.after,
		}
	}
	details := map[string]interface{}{
		fidelityKeyPersona:   t.persona.ID,
		fidelityKeyTurnIndex: t.turnIndex,
		"initial":            t.initial,
		"final":              t.state,
		keyExchanges:         t.exchanges,
		"changes":            changes,
	}
	if t.stop != nil {
		details[stateKeyOutcome] = t.stop.OutcomeLabel()
		details[stateKeyStoppedAt] = t.stoppedAt
	}
	if t.evalErr != "" {
		details["error"] = t.evalErr
	}
	return asrt.ConversationValidationResult{
		Type:    personaStateResultType,
		Passed:  t.evalErr == "" && (t.stop == nil || !t.stop.Fail),
		Message: t.message(),
		Details: details,
	}
}

func (t *stateTracker) message() string {
	msg := fmt.Sprintf("persona %s state %s", t.persona.ID, stateDiff(t.initial, t.state))
	switch {
	case t.stop != nil && t.stoppedAt == 0:
		msg += fmt.Sprintf("; %s before the turn began", t.stop.OutcomeLabel())
	case t.stop != nil:
		msg += fmt.Sprintf("; %s at exchange %d", t.stop.OutcomeLabel(), t.stoppedAt)
	}
	if t.evalErr != "" {
		msg += "; transitions could not be evaluated: " + t.evalErr
	}
	return msg
}

// stateDiff describes what changed between two states, e.g.
// "patience 3→0, gave_order_number false→true", or "unchanged".
func stateDiff(before, after map[string]interface{}) string {
	names := make([]string, 0, len(after))
	for name := range after {
		if before[name] != after[name] {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "unchanged"
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = fmt.Sprintf("%s %s→%s", name,
			arenaconfig.FormatStateValue(before[name]), arenaconfig.FormatStateValue(after[name]))
	}
	return strings.Join(parts, ", ")
}
