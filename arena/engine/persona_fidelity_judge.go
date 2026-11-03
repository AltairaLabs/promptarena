package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"

	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"
)

// factJudgeRubric tells the judge how to score a persona message: 1 when it
// asserts nothing the persona was not given, 0 when it does.
const factJudgeRubric = "Score 1 if the message asserts no fact beyond what the persona was given " +
	"and what the conversation already established. Score 0 if it asserts one, and name it in the reasoning. " +
	"Rephrasing a given fact, asking a question, guessing aloud or saying it doesn't know does not count."

// factJudgeMinScore is the score a message needs for the judge to pass it.
const factJudgeMinScore = 0.5

// judgeNote is one persona message the judge flagged, or failed to check.
type judgeNote struct {
	exchange int
	text     string
}

// judgeMessage asks the configured judge whether the persona's message (the
// last of messages) asserts a fact it was not given. It runs on the quiet
// orchestrator, so the check emits no eval events of its own.
func (t *fidelityTracker) judgeMessage(ctx context.Context, exchange int, messages []types.Message) {
	idx := len(messages) - 1
	// The judge reads the latest assistant message as the content to judge,
	// so the persona's message is presented as one.
	content := []types.Message{{Role: roleAssistant, Content: messages[idx].GetContent()}}
	checks := t.orch.RunAssertionsAsConversationResults(ctx,
		[]asrt.AssertionConfig{{Type: "llm_judge", Params: map[string]interface{}{
			"criteria":  t.judgeCriteria(messages[:idx]),
			"rubric":    factJudgeRubric,
			"min_score": factJudgeMinScore,
		}}},
		content, 0, t.req.ConversationID, evals.TriggerOnConversationComplete)
	if len(checks) == 0 {
		t.judgeErrors = append(t.judgeErrors, judgeNote{exchange: exchange, text: "no eval runner"})
		return
	}
	check := checks[0]
	if errMsg, _ := check.Details["error"].(string); errMsg != "" {
		t.judgeErrors = append(t.judgeErrors, judgeNote{exchange: exchange, text: errMsg})
		return
	}
	if skipped, _ := check.Details["skipped"].(bool); skipped {
		// A skipped judge did not check the message, so it did not pass it.
		reason, _ := check.Details["skip_reason"].(string)
		t.judgeErrors = append(t.judgeErrors, judgeNote{exchange: exchange, text: "skipped: " + reason})
		return
	}
	if !check.Passed {
		t.judgeFlags = append(t.judgeFlags, judgeNote{exchange: exchange, text: check.Message})
	}
}

// judgeCriteria describes the check to the judge: the persona's facts, the
// rest of what it was told about itself, the conversation before its message
// and any exchanges a rewind removed, which the persona was handed back.
func (t *fidelityTracker) judgeCriteria(before []types.Message) string {
	var b strings.Builder
	b.WriteString("The content is one message from a simulated user (a persona) talking to an agent. " +
		"Does it assert a specific fact, such as a name, number, date, amount, place, event or something the user " +
		"did, that the persona was not given and that the conversation had not already established?\n\n")
	b.WriteString("The persona's facts:\n" + t.persona.FactsText() + "\n\n")
	background := selfplay.PersonaBackground(t.persona)
	if t.objective != "" {
		background += "\nIts objective: " + t.objective
	}
	if background != "" {
		b.WriteString("What else the persona was told about itself:\n" + background + "\n\n")
	}
	if t.rewoundText != "" {
		b.WriteString("Earlier exchanges that were removed, which the persona was shown:\n" + t.rewoundText + "\n")
	}
	b.WriteString("The conversation before the message:\n" + judgeTranscript(before))
	return b.String()
}

// judgeTranscript renders the conversation with each message labeled by who
// sent it.
func judgeTranscript(messages []types.Message) string {
	var b strings.Builder
	for i := range messages {
		who := messages[i].Role
		switch who {
		case roleAssistant:
			who = "agent"
		case roleSystem:
			continue
		}
		text := messages[i].GetContent()
		if messages[i].ToolResult != nil {
			text = messages[i].ToolResult.GetTextContent()
		}
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", who, text)
	}
	return b.String()
}

func judgeNoteDetails(notes []judgeNote, key string) []map[string]interface{} {
	out := make([]map[string]interface{}, len(notes))
	for i, n := range notes {
		out[i] = map[string]interface{}{fidelityKeyExchange: n.exchange, key: n.text}
	}
	return out
}

// judgeFlagsText describes the judge's flags for the result message.
func (t *fidelityTracker) judgeFlagsText() string {
	parts := make([]string, len(t.judgeFlags))
	for i, f := range t.judgeFlags {
		parts[i] = fmt.Sprintf("exchange %d (%s)", f.exchange, f.text)
	}
	plural := "s"
	if len(t.judgeFlags) == 1 {
		plural = ""
	}
	return fmt.Sprintf("the judge flagged %d message%s: %s", len(t.judgeFlags), plural, strings.Join(parts, ", "))
}
