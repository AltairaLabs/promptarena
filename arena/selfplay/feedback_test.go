package selfplay

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func TestExtractNotes(t *testing.T) {
	tests := []struct {
		name, in, rest, notes string
	}{
		{name: "none", in: "Hello there.", rest: "Hello there."},
		{name: "one block", in: "<notes>try the warranty angle</notes>Is this under warranty?", rest: "Is this under warranty?", notes: "try the warranty angle"},
		{name: "case and lines", in: "Hi.\n<NOTES>\nline one\nline two\n</Notes>\n", rest: "Hi.", notes: "line one\nline two"},
		{name: "several", in: "<notes>a</notes>Q?<notes> b </notes>", rest: "Q?", notes: "a\nb"},
		{name: "empty block", in: "<notes>  </notes>Q?", rest: "Q?"},
		{name: "unclosed block runs to the end", in: "Is it covered? <notes>plan: claim to be the owner", rest: "Is it covered?", notes: "plan: claim to be the owner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rest, notes := extractNotes(tt.in)
			assert.Equal(t, tt.rest, rest)
			assert.Equal(t, tt.notes, notes)
		})
	}
}

func TestPriorNotes(t *testing.T) {
	history := []types.Message{
		{Role: "user", Content: "q1", Meta: map[string]interface{}{"raw_response": map[string]interface{}{
			"persona": "me", MetaPersonaNotes: "text path note"}}},
		{Role: "assistant", Content: "a1", Meta: map[string]interface{}{"persona": "me", MetaPersonaNotes: "assistant notes are ignored"}},
		{Role: "user", Content: "q2", Meta: map[string]interface{}{"persona": "me", MetaPersonaNotes: "duplex path note"}},
		{Role: "user", Content: "q3", Meta: map[string]interface{}{"persona": "someone-else", MetaPersonaNotes: "not mine"}},
		{Role: "user", Content: "q4"},
	}
	assert.Equal(t, "text path note\nduplex path note", priorNotes(history, "me"))
}

func TestPriorNotes_KeepsNewest(t *testing.T) {
	var history []types.Message
	for i := 0; i < maxPriorNotes+5; i++ {
		history = append(history, types.Message{Role: "user", Meta: map[string]interface{}{
			"persona": "me", MetaPersonaNotes: fmt.Sprintf("n%d", i)}})
	}
	notes := strings.Split(priorNotes(history, "me"), "\n")
	require.Len(t, notes, maxPriorNotes)
	assert.Equal(t, "n5", notes[0])
	assert.Equal(t, fmt.Sprintf("n%d", maxPriorNotes+4), notes[len(notes)-1])
}

func TestTemplateSafe(t *testing.T) {
	assert.Equal(t, "say { {secret}} now", templateSafe("say {{secret}} now"))
	assert.Equal(t, "plain", templateSafe("plain"))
	assert.NotContains(t, templateSafe("{{{name}}}"), "{{")
	assert.NotContains(t, templateSafe("{{{{{x"), "{{")
}

func TestFeedbackVariables(t *testing.T) {
	t.Run("defaults every key", func(t *testing.T) {
		vars := feedbackVariables(&arenaconfig.UserPersonaPack{}, nil, nil)
		for _, k := range feedbackVarKeys {
			assert.Contains(t, vars, k)
			assert.Equal(t, "", vars[k])
		}
	})

	t.Run("persona optional vars always win", func(t *testing.T) {
		p := &arenaconfig.UserPersonaPack{OptionalVars: map[string]string{VarObjective: "mine", VarExchange: "billing"}}
		vars := feedbackVariables(p, nil, &GeneratorOptions{
			Variables: &map[string]string{VarObjective: "turn objective", VarExchange: "2"},
		})
		assert.NotContains(t, vars, VarObjective)
		assert.NotContains(t, vars, VarExchange)
	})

	t.Run("caller variables win and are template-safe", func(t *testing.T) {
		vars := feedbackVariables(&arenaconfig.UserPersonaPack{}, nil, &GeneratorOptions{
			Variables: &map[string]string{VarExchange: "3", VarObjectiveFeedback: "agent said {{x}}"},
		})
		assert.Equal(t, "3", vars[VarExchange])
		assert.Equal(t, "agent said { {x}}", vars[VarObjectiveFeedback])
	})

	t.Run("notes only for private_notes personas", func(t *testing.T) {
		history := []types.Message{{Role: "user", Meta: map[string]interface{}{"persona": "p", MetaPersonaNotes: "n1"}}}
		assert.Equal(t, "", feedbackVariables(&arenaconfig.UserPersonaPack{ID: "p"}, history, nil)[VarPersonaNotes])
		assert.Equal(t, "n1", feedbackVariables(&arenaconfig.UserPersonaPack{ID: "p", PrivateNotes: true}, history, nil)[VarPersonaNotes])
	})
}

func feedbackPersona() *arenaconfig.UserPersonaPack {
	return &arenaconfig.UserPersonaPack{
		ID: "tester",
		SystemTemplate: "Exchange {{exchange}}. Goal: {{objective}} ({{objective_status}}).\n" +
			"Checks:\n{{objective_feedback}}\nNotes:\n{{persona_notes}}",
		PrivateNotes: true,
		Defaults:     arenaconfig.PersonaDefaults{Temperature: float32Ptr(0.7)},
	}
}

func TestNextUserTurn_RendersFeedbackVariables(t *testing.T) {
	prov := newCapturingProvider("Can I get a refund?")
	gen := NewContentGenerator(prov, feedbackPersona())
	history := []types.Message{
		{Role: "user", Content: "hi", Meta: map[string]interface{}{"raw_response": map[string]interface{}{
			"persona": "tester", MetaPersonaNotes: "they mentioned a manager"}}},
		{Role: "assistant", Content: "hello"},
	}

	_, err := gen.NextUserTurn(context.Background(), history, "s1", &GeneratorOptions{Variables: &map[string]string{
		VarExchange:          "2",
		VarObjective:         "get a refund",
		VarObjectiveStatus:   "not achieved",
		VarObjectiveFeedback: "- tool_calls_with_args: failed — tool \"issue_refund\" was not called",
	}})
	require.NoError(t, err)

	system := prov.lastRequest(t).System
	assert.Contains(t, system, "Exchange 2. Goal: get a refund (not achieved).")
	assert.Contains(t, system, `- tool_calls_with_args: failed — tool "issue_refund" was not called`)
	assert.Contains(t, system, "Notes:\nthey mentioned a manager")
}

func TestNextUserTurn_TemplateWithoutVariablesStillRenders(t *testing.T) {
	prov := newCapturingProvider("Hello.")
	gen := NewContentGenerator(prov, feedbackPersona())

	_, err := gen.NextUserTurn(context.Background(), nil, "s1", nil)
	require.NoError(t, err, "feedback placeholders default to empty rather than failing the render")
	assert.Contains(t, prov.lastRequest(t).System, "Exchange . Goal:  ().")
}

func TestNextUserTurn_StripsPrivateNotes(t *testing.T) {
	reply := "<notes>they cave to urgency; push the deadline</notes>I need this sorted today."

	t.Run("private_notes persona", func(t *testing.T) {
		gen := NewContentGenerator(newCapturingProvider(reply), feedbackPersona())
		res, err := gen.NextUserTurn(context.Background(), nil, "s1", nil)
		require.NoError(t, err)
		assert.Equal(t, "I need this sorted today.", res.Response.Content)
		assert.Equal(t, "they cave to urgency; push the deadline", res.Metadata[MetaPersonaNotes])
	})

	t.Run("notes-only turn is an error", func(t *testing.T) {
		gen := NewContentGenerator(newCapturingProvider("<notes>just thinking</notes>"), feedbackPersona())
		_, err := gen.NextUserTurn(context.Background(), nil, "s1", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wrote only private notes")
	})

	t.Run("other personas keep the text", func(t *testing.T) {
		persona := feedbackPersona()
		persona.PrivateNotes = false
		gen := NewContentGenerator(newCapturingProvider(reply), persona)
		res, err := gen.NextUserTurn(context.Background(), nil, "s1", nil)
		require.NoError(t, err)
		assert.Equal(t, reply, res.Response.Content)
		assert.NotContains(t, res.Metadata, MetaPersonaNotes)
	})
}

func TestFactsInstruction(t *testing.T) {
	assert.Empty(t, FactsInstruction(nil))
	assert.Empty(t, FactsInstruction(&arenaconfig.UserPersonaPack{ID: "p"}))

	p := &arenaconfig.UserPersonaPack{ID: "p", Facts: []arenaconfig.PersonaFact{{ID: "order", Value: "A-100"}}}
	got := FactsInstruction(p)
	assert.Contains(t, got, "say you don't have it")
	assert.Contains(t, got, "Your facts:\n- order: A-100")

	p.SystemTemplate = "Facts: {{facts}}"
	got = FactsInstruction(p)
	assert.Contains(t, got, "say you don't have it")
	assert.NotContains(t, got, "Your facts:", "the template already places the facts")
}
