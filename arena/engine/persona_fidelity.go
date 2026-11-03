package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// personaFidelityResultType is the conversation-result type a persona's
// fidelity to its declared facts is reported under.
const personaFidelityResultType = "persona_fidelity"

// Detail keys of a persona fidelity result.
const (
	fidelityKeyReliable  = "reliable"
	fidelityKeyPersona   = "persona"
	fidelityKeyTurnIndex = "turn_index"
	fidelityKeyExchange  = "exchange"
	fidelityKeySpecifics = "specifics"
)

// fidelityViolation is one persona message that stated specifics it was not
// given.
type fidelityViolation struct {
	exchange  int
	specifics []string
}

// fidelityTracker checks one self-play turn's persona messages against the
// persona's declared facts after each exchange. A nil tracker (a persona
// without facts, or with the check turned off) does nothing.
type fidelityTracker struct {
	ce        *DefaultConversationExecutor
	req       *ConversationRequest
	turnIndex int
	persona   *arenaconfig.UserPersonaPack
	// known is what the persona was given, lowercased: its facts, goals,
	// constraints, description, prompt, optional_vars and the turn's
	// objective.
	known string
	// seen is the lowercased text of every exchange already checked in this
	// turn, kept so a specific from an exchange that was later rewound (and
	// handed back to the persona as {{rewound_response}}) still counts as
	// given.
	seen string
	// next is the index a persona message must be at or after to be new, so
	// a message is never checked twice. A rewind moves it back to where the
	// removed message was, since the next message takes its place.
	next int
	// lastIdx is the index of the last persona message checked, and lastText
	// its exchange as it was checked, kept in case the exchange is rewound.
	lastIdx  int
	lastText string
	// rewoundText is the text of exchanges a rewind removed. The persona was
	// handed them back, so the judge is shown them too.
	rewoundText string
	// objective is the turn's objective description, which the persona is
	// given.
	objective  string
	checked    int
	violations []fidelityViolation
	// repaired lists the drafts fact_policy.on_violation: regenerate
	// replaced, with the specifics each invented.
	repaired []fidelityViolation
	// orch runs the fact_policy.judge check without emitting eval events; nil
	// when the persona doesn't ask for the judge.
	orch        *EvalOrchestrator
	judgeFlags  []judgeNote
	judgeErrors []judgeNote
	loadErr     string
}

// newFidelityTracker returns a tracker for the turn, or nil when its persona
// has no facts to check against.
func (ce *DefaultConversationExecutor) newFidelityTracker(
	req *ConversationRequest, turnIdx int, turn *arenaconfig.TurnDefinition,
) *fidelityTracker {
	if ce.selfPlayRegistry == nil {
		return nil
	}
	persona := ce.selfPlayRegistry.GetPersona(turn.Persona)
	if persona == nil || !persona.ChecksFacts() {
		return nil
	}
	known := selfplay.PersonaGivenText(persona)
	objective := ""
	if turn.Objective != nil {
		objective = turn.Objective.Description
		known += "\n" + strings.ToLower(objective)
	}
	t := &fidelityTracker{
		ce: ce, req: req, turnIndex: turnIdx, persona: persona,
		known: known, objective: objective,
	}
	if persona.JudgesFacts() {
		t.orch = ce.resolveEvalOrchestrator(req).Quiet()
	}
	return t
}

// afterExchange checks the persona's message from the exchange just run
// (1-based): a specific in it that is not in what the persona was given and
// not earlier in the conversation is recorded as invented. It runs before the
// objective, so an exchange that is then rewound is still checked, and also
// after an exchange that errored, so a message the agent failed on is not
// left unchecked.
func (t *fidelityTracker) afterExchange(ctx context.Context, exchange int) {
	if t == nil {
		return
	}
	// A run that hit its timeout still has its last message checked.
	messages, err := t.ce.loadMessagesFromStateStore(scoringContext(ctx), t.req)
	if err != nil {
		t.loadErr = err.Error()
		return
	}
	idx := lastIndexOfRole(messages, roleUser)
	if idx < 0 || idx < t.next {
		// No persona message yet, or none new: generating it is what failed.
		return
	}
	t.next, t.lastIdx = idx+1, idx
	t.lastText = selfplay.ConversationText(messages[idx:])
	t.checked++
	if r := repairedSpecifics(messages[idx].Meta); len(r) > 0 {
		t.repaired = append(t.repaired, fidelityViolation{exchange: exchange, specifics: r})
	}
	invented := selfplay.InventedSpecifics(messages[idx].GetContent(),
		t.known, selfplay.ConversationText(messages[:idx]), t.seen)
	t.seen += strings.ToLower(t.lastText)
	if len(invented) > 0 {
		t.violations = append(t.violations, fidelityViolation{exchange: exchange, specifics: invented})
		return
	}
	if t.orch != nil {
		// A message the specifics check already flagged is not judged too.
		t.judgeMessage(scoringContext(ctx), exchange, messages[:idx+1])
	}
}

// rewound records that the exchange just checked was removed: the next
// persona message takes the removed one's place, and is new.
func (t *fidelityTracker) rewound() {
	if t == nil {
		return
	}
	t.next = t.lastIdx
	t.rewoundText += t.lastText
}

// repairedSpecifics reads the specifics a regenerated persona draft
// invented, stored under raw_response by the text self-play executor and at
// the top level by the duplex one.
func repairedSpecifics(meta map[string]interface{}) []string {
	v, ok := meta[selfplay.MetaFidelityRepaired]
	if !ok {
		raw, _ := meta["raw_response"].(map[string]interface{})
		v = raw[selfplay.MetaFidelityRepaired]
	}
	switch v := v.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func lastIndexOfRole(messages []types.Message, role string) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role {
			return i
		}
	}
	return -1
}

// finish records the turn's fidelity result on the conversation.
func (t *fidelityTracker) finish() {
	if t == nil || t.req.fidelity == nil {
		return
	}
	t.req.fidelity.record(t.result())
}

// result reports the turn's fidelity. It fails only when the persona's
// fact_policy asks for that; otherwise an invented specific marks the run
// unreliable without failing it.
func (t *fidelityTracker) result() asrt.ConversationValidationResult {
	invented := 0
	for _, v := range t.violations {
		invented += len(v.specifics)
	}
	violations := violationDetails(t.violations)
	reliable := len(t.violations) == 0 && len(t.judgeFlags) == 0
	details := map[string]interface{}{
		fidelityKeyPersona:   t.persona.ID,
		fidelityKeyTurnIndex: t.turnIndex,
		fidelityKeyReliable:  reliable,
		"checked_messages":   t.checked,
		"violations":         violations,
	}
	if len(t.repaired) > 0 {
		details["repaired"] = violationDetails(t.repaired)
	}
	if len(t.judgeFlags) > 0 {
		details["judge_flags"] = judgeNoteDetails(t.judgeFlags, "reasoning")
	}
	if len(t.judgeErrors) > 0 {
		details["judge_errors"] = judgeNoteDetails(t.judgeErrors, "error")
	}
	if t.loadErr != "" {
		details["error"] = t.loadErr
	}
	return asrt.ConversationValidationResult{
		Type:    personaFidelityResultType,
		Passed:  reliable || !t.persona.FailsOnFactViolation(),
		Message: t.message(invented),
		Details: details,
	}
}

func violationDetails(vs []fidelityViolation) []map[string]interface{} {
	out := make([]map[string]interface{}, len(vs))
	for i, v := range vs {
		out[i] = map[string]interface{}{fidelityKeyExchange: v.exchange, fidelityKeySpecifics: v.specifics}
	}
	return out
}

func (t *fidelityTracker) message(invented int) string {
	suffix := ""
	if n := len(t.repaired); n == 1 {
		suffix = " (1 draft regenerated)"
	} else if n > 1 {
		suffix = fmt.Sprintf(" (%d drafts regenerated)", n)
	}
	if n := len(t.judgeErrors); n > 0 {
		suffix += fmt.Sprintf(" (judge failed on %d)", n)
	}
	if len(t.violations) == 0 {
		if len(t.judgeFlags) > 0 {
			return fmt.Sprintf("persona %s: %s%s", t.persona.ID, t.judgeFlagsText(), suffix)
		}
		return fmt.Sprintf("persona %s stated only declared facts in %d messages%s", t.persona.ID, t.checked, suffix)
	}
	if len(t.judgeFlags) > 0 {
		suffix = "; " + t.judgeFlagsText() + suffix
	}
	quoted := make([]string, 0, invented)
	for _, v := range t.violations {
		for _, s := range v.specifics {
			quoted = append(quoted, fmt.Sprintf("%q (exchange %d)", s, v.exchange))
		}
	}
	noun := "specifics"
	if invented == 1 {
		noun = "specific"
	}
	return fmt.Sprintf("persona %s invented %d %s: %s%s", t.persona.ID, invented, noun, strings.Join(quoted, ", "), suffix)
}

// fidelityRecorder collects persona fidelity results for one conversation.
type fidelityRecorder struct {
	mu      sync.Mutex
	results []asrt.ConversationValidationResult
}

func (r *fidelityRecorder) record(res asrt.ConversationValidationResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, res)
}

func (r *fidelityRecorder) all() []asrt.ConversationValidationResult {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]asrt.ConversationValidationResult(nil), r.results...)
}

// UnreliableTurns returns the scenario turns whose persona invented a
// specific, read from a run's persona fidelity results. An objective on such
// a turn measured the simulator as much as the agent.
func UnreliableTurns(results []asrt.ConversationValidationResult) map[int]bool {
	var out map[int]bool
	for i := range results {
		if results[i].Type != personaFidelityResultType {
			continue
		}
		if reliable, ok := results[i].Details[fidelityKeyReliable].(bool); ok && !reliable {
			if out == nil {
				out = map[int]bool{}
			}
			out[metaInt(results[i].Details[fidelityKeyTurnIndex])] = true
		}
	}
	return out
}
