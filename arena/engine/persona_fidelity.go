package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"

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

// minSpecificLen is the fewest letters and digits a token needs to count as a
// specific: "A-100" and "2024" do, "2" and "28" don't, so a count or a day of
// the month on its own is not flagged.
const minSpecificLen = 3

var (
	// reOrdinal matches an ordinal such as 1st or 28th, which is ordinary
	// speech rather than an identifier.
	reOrdinal = regexp.MustCompile(`^\d+(st|nd|rd|th)$`)
	// reEmail matches an email address.
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	// reToken matches a run of letters, digits and the joiners identifiers
	// use (A-100, 2024-10-28, #4471, 10:30).
	reToken = regexp.MustCompile(`[A-Za-z0-9#][A-Za-z0-9\-_/.:#]*`)
)

// specifics returns the identifier-shaped tokens in text, in order and without
// repeats: email addresses, and tokens that contain a digit and at least
// minSpecificLen letters and digits. Ordinals (2nd, 28th) and percentages are
// not specifics, and a leading or trailing '#', '-', '/', '.', ':' or '_' is
// dropped, so "#4471" matches a fact of "4471".
func specifics(text string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if k := strings.ToLower(s); !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	for _, email := range reEmail.FindAllString(text, -1) {
		add(email)
	}
	rest := reEmail.ReplaceAllString(text, " ")
	for _, loc := range reToken.FindAllStringIndex(rest, -1) {
		if loc[1] < len(rest) && rest[loc[1]] == '%' {
			continue
		}
		tok := strings.Trim(rest[loc[0]:loc[1]], tokenJoiners)
		if strings.ContainsAny(tok, "0123456789") && alnumCount(tok) >= minSpecificLen &&
			!reOrdinal.MatchString(strings.ToLower(tok)) {
			add(tok)
		}
	}
	return out
}

// tokenJoiners are the characters reToken allows inside an identifier.
const tokenJoiners = "-_/.:#"

func alnumCount(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			n++
		}
	}
	return n
}

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
	// lastChecked is the index of the last persona message checked, so a
	// message is never checked twice.
	lastChecked int
	checked     int
	violations  []fidelityViolation
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
	parts := []string{persona.Description, persona.SystemPrompt, persona.SystemTemplate}
	for _, f := range persona.Facts {
		parts = append(parts, f.Value)
	}
	parts = append(parts, persona.Goals...)
	parts = append(parts, persona.Constraints...)
	for _, v := range persona.OptionalVars {
		parts = append(parts, v)
	}
	if turn.Objective != nil {
		parts = append(parts, turn.Objective.Description)
	}
	return &fidelityTracker{
		ce: ce, req: req, turnIndex: turnIdx, persona: persona,
		known: strings.ToLower(strings.Join(parts, "\n")), lastChecked: -1,
	}
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
	if idx < 0 || idx == t.lastChecked {
		// No persona message yet, or none new: generating it is what failed.
		return
	}
	t.lastChecked = idx
	t.checked++
	earlier := strings.ToLower(conversationText(messages[:idx]))
	var invented []string
	for _, s := range specifics(messages[idx].GetContent()) {
		k := strings.ToLower(s)
		if !strings.Contains(t.known, k) && !strings.Contains(earlier, k) && !strings.Contains(t.seen, k) {
			invented = append(invented, s)
		}
	}
	t.seen += strings.ToLower(conversationText(messages[idx:]))
	if len(invented) > 0 {
		t.violations = append(t.violations, fidelityViolation{exchange: exchange, specifics: invented})
	}
}

func lastIndexOfRole(messages []types.Message, role string) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role {
			return i
		}
	}
	return -1
}

// conversationText joins every message's text and tool result, so a
// specific the agent or a tool gave can be repeated by the persona.
func conversationText(messages []types.Message) string {
	var b strings.Builder
	for i := range messages {
		b.WriteString(messages[i].GetContent())
		b.WriteByte('\n')
		if messages[i].ToolResult != nil {
			b.WriteString(messages[i].ToolResult.GetTextContent())
			b.WriteByte('\n')
		}
	}
	return b.String()
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
	violations := make([]map[string]interface{}, len(t.violations))
	for i, v := range t.violations {
		invented += len(v.specifics)
		violations[i] = map[string]interface{}{fidelityKeyExchange: v.exchange, fidelityKeySpecifics: v.specifics}
	}
	reliable := len(t.violations) == 0
	details := map[string]interface{}{
		fidelityKeyPersona:   t.persona.ID,
		fidelityKeyTurnIndex: t.turnIndex,
		fidelityKeyReliable:  reliable,
		"checked_messages":   t.checked,
		"violations":         violations,
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

func (t *fidelityTracker) message(invented int) string {
	if len(t.violations) == 0 {
		return fmt.Sprintf("persona %s stated only declared facts in %d messages", t.persona.ID, t.checked)
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
	return fmt.Sprintf("persona %s invented %d %s: %s", t.persona.ID, invented, noun, strings.Join(quoted, ", "))
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
