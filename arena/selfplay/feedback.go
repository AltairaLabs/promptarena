package selfplay

import (
	"regexp"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// Template variables every persona generation receives, so a
// system_template can reference them whether or not the turn sets them.
const (
	// VarExchange is the 1-based number of the exchange being generated.
	VarExchange = "exchange"
	// VarObjective is the turn objective's description.
	VarObjective = "objective"
	// VarObjectiveStatus is "achieved" or "not achieved" on a turn with an
	// objective, and empty otherwise.
	VarObjectiveStatus = "objective_status"
	// VarObjectiveFeedback lists the objective's success checks from the
	// previous exchange, one per line, with whether each passed.
	VarObjectiveFeedback = "objective_feedback"
	// VarPersonaNotes holds the persona's private notes from earlier turns.
	VarPersonaNotes = "persona_notes"
	// VarRewoundMessage and VarRewoundResponse hold the persona's message and
	// the agent's reply from the previous exchange when the objective's rewind
	// rule removed it, and are empty otherwise.
	VarRewoundMessage  = "rewound_message"
	VarRewoundResponse = "rewound_response"
	// VarRewindsLeft is how many more exchanges the turn may rewind, on a
	// turn whose objective has a rewind rule.
	VarRewindsLeft = "rewinds_left"
	// VarSearchScore is, on a turn whose objective searches a tree, the score
	// (1–10) of the agent's last reply on the branch being extended, and empty
	// on the first exchange.
	VarSearchScore = "search_score"
	// VarSearchBranch says which of the branch's candidates is being drafted,
	// as "2 of 3", on a turn whose objective searches a tree.
	VarSearchBranch = "search_branch"
)

// MetaPersonaNotes is the message metadata key that carries a turn's private
// notes.
const MetaPersonaNotes = "persona_notes"

// personaRole is the role the persona's turns are stored under.
const personaRole = "user"

// reNotesGroups is the number of reNotes submatches: the whole block and its
// contents.
const reNotesGroups = 2

// metaRawResponse is the user-message metadata key the text self-play
// executor nests generation metadata under.
const metaRawResponse = "raw_response"

var feedbackVarKeys = []string{
	VarExchange, VarObjective, VarObjectiveStatus, VarObjectiveFeedback, VarPersonaNotes,
	VarRewoundMessage, VarRewoundResponse, VarRewindsLeft, VarSearchScore, VarSearchBranch,
}

// reNotes matches a <notes>...</notes> block, case-insensitively and across
// lines. A block left open (a turn cut off by its token cap) runs to the end
// of the text, so an unfinished note is never sent to the agent.
var reNotes = regexp.MustCompile(`(?is)<notes>(.*?)(?:</notes>|\z)`)

// maxPriorNotes caps how many earlier turns' notes {{persona_notes}} carries,
// keeping the newest, so the persona prompt stays bounded on long runs.
const maxPriorNotes = 20

// extractNotes removes every <notes> block from content (including an
// unclosed one) and returns the remaining text and the notes, joined by
// newlines.
func extractNotes(content string) (remaining, notes string) {
	var found []string
	remaining = reNotes.ReplaceAllStringFunc(content, func(block string) string {
		if m := reNotes.FindStringSubmatch(block); len(m) == reNotesGroups {
			if n := strings.TrimSpace(m[1]); n != "" {
				found = append(found, n)
			}
		}
		return ""
	})
	return strings.TrimSpace(remaining), strings.Join(found, "\n")
}

// priorNotes collects the private notes stored on personaID's earlier turns
// (user messages), oldest first, keeping at most the newest maxPriorNotes.
// Notes written by another persona in the same conversation are not included.
func priorNotes(history []types.Message, personaID string) string {
	var notes []string
	for i := range history {
		if history[i].Role != personaRole || metaString(history[i].Meta, metaPersona) != personaID {
			continue
		}
		if n := metaString(history[i].Meta, MetaPersonaNotes); n != "" {
			notes = append(notes, n)
		}
	}
	if len(notes) > maxPriorNotes {
		notes = notes[len(notes)-maxPriorNotes:]
	}
	return strings.Join(notes, "\n")
}

// metaPersona is the metadata key holding the persona that wrote a turn.
const metaPersona = "persona"

// metaString reads a string from a persona turn's metadata, stored either at
// the top level (the duplex path) or under raw_response (the text path).
func metaString(meta map[string]interface{}, key string) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta[key].(string); ok {
		return v
	}
	if raw, ok := meta[metaRawResponse].(map[string]interface{}); ok {
		if v, ok := raw[key].(string); ok {
			return v
		}
	}
	return ""
}

// templateSafe stops a value from adding template placeholders when it is
// substituted into the persona prompt. The renderer substitutes recursively
// and fails on unresolved placeholders, so agent text or notes containing
// "{{" would otherwise expand other variables or break the turn.
func templateSafe(v string) string {
	// Repeat until none are left: one pass over "{{{" leaves a "{{" behind.
	for strings.Contains(v, "{{") {
		v = strings.ReplaceAll(v, "{{", "{ {")
	}
	return v
}

// feedbackVariables returns the template variables for a generation, all
// template-safe: empty defaults for every feedback key, the persona's notes,
// then the caller's variables. A key the persona defines in optional_vars is
// left out entirely, so the persona's own value always wins.
func feedbackVariables(
	persona *arenaconfig.UserPersonaPack, history []types.Message, opts *GeneratorOptions,
) map[string]string {
	vars := make(map[string]string, len(feedbackVarKeys))
	for _, k := range feedbackVarKeys {
		vars[k] = ""
	}
	if persona.PrivateNotes {
		vars[VarPersonaNotes] = templateSafe(priorNotes(history, persona.ID))
	}
	if opts != nil {
		for k, v := range opts.Variables {
			vars[k] = templateSafe(v)
		}
	}
	for k := range persona.OptionalVars {
		delete(vars, k)
	}
	return vars
}
