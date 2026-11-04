package selfplay

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// MetaFidelityRepaired is the metadata key on a persona turn listing the
// specifics its first draft invented, when fact_policy.on_violation:
// regenerate replaced the draft.
const MetaFidelityRepaired = "fidelity_repaired"

// RegenerateInstruction is appended to the persona's prompt when its draft
// invented specifics, for the one retry.
func RegenerateInstruction(invented []string) string {
	quoted := make([]string, len(invented))
	for i, s := range invented {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return fmt.Sprintf(`

Your previous draft of this message stated %s, which you were not given and
nobody in the conversation has said. Write the message again without it. Use
only your facts; if you need something that is not in them, say you don't
have it.`, strings.Join(quoted, ", "))
}

// ConversationText joins every message's text and tool result, so a specific
// the agent or a tool gave can be repeated by the persona.
func ConversationText(messages []types.Message) string {
	var b strings.Builder
	for i := range messages {
		b.WriteString(PersonaText(&messages[i]))
		b.WriteByte('\n')
		if messages[i].ToolResult != nil {
			b.WriteString(messages[i].ToolResult.GetTextContent())
			b.WriteByte('\n')
		}
	}
	return b.String()
}

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

// Specifics returns the identifier-shaped tokens in text, in order and without
// repeats: email addresses, and tokens that contain a digit and at least
// minSpecificLen letters and digits. Ordinals (2nd, 28th) and percentages are
// not specifics, and a leading or trailing '#', '-', '/', '.', ':' or '_' is
// dropped, so "#4471" matches a fact of "4471".
func Specifics(text string) []string {
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

// PersonaGivenText returns, lowercased, what the persona's config gives it:
// its facts and its PersonaBackground. A specific found here was not
// invented.
func PersonaGivenText(persona *arenaconfig.UserPersonaPack) string {
	parts := make([]string, 0, len(persona.Facts)+1)
	for _, f := range persona.Facts {
		parts = append(parts, f.Value)
	}
	parts = append(parts, PersonaBackground(persona))
	return strings.ToLower(strings.Join(parts, "\n"))
}

// PersonaBackground returns what the persona's config tells it about itself
// besides its facts, one non-empty line or paragraph each: its description,
// prompt, goals, constraints, optional_vars values (by name) and the string
// values its state can hold.
func PersonaBackground(persona *arenaconfig.UserPersonaPack) string {
	parts := []string{persona.Description, persona.SystemPrompt, persona.SystemTemplate}
	parts = append(parts, persona.Goals...)
	parts = append(parts, persona.Constraints...)
	names := make([]string, 0, len(persona.OptionalVars))
	for name := range persona.OptionalVars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		parts = append(parts, persona.OptionalVars[name])
	}
	parts = append(parts, persona.StateStrings()...)
	kept := parts[:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n")
}

// InventedSpecifics returns the specifics in message that appear in none of
// given, each compared lowercased.
func InventedSpecifics(message string, given ...string) []string {
	text := strings.ToLower(strings.Join(given, "\n"))
	var invented []string
	for _, s := range Specifics(message) {
		if !strings.Contains(text, strings.ToLower(s)) {
			invented = append(invented, s)
		}
	}
	return invented
}
