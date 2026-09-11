package generate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/types"
)

func TestConvert_UserTurnsBecomeScenarioTurns(t *testing.T) {
	session := &SessionDetail{
		SessionSummary: SessionSummary{ID: "session-123", Timestamp: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)},
		Messages: []types.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
			{Role: "user", Content: "How are you?"},
			{Role: "assistant", Content: "Well!"},
		},
	}

	c, err := Convert(session, ConvertOptions{})
	require.NoError(t, err)
	sc := c.Scenario

	assert.Equal(t, "promptkit.altairalabs.ai/v1alpha1", sc.APIVersion)
	assert.Equal(t, "Scenario", sc.Kind)
	assert.Equal(t, "session-123", sc.Metadata.Name)
	assert.Equal(t, "conversation", sc.Spec.TaskType)
	require.Len(t, sc.Spec.Turns, 2)
	assert.Equal(t, "Hello", sc.Spec.Turns[0].Content)
	assert.Equal(t, "How are you?", sc.Spec.Turns[1].Content)
	assert.Contains(t, sc.Spec.Description, "Generated from session session-123")
	assert.Contains(t, sc.Spec.Description, "2025-01-15T10:00:00Z")
}

// Every user turn is kept. A session with more than one is warned about, in
// the result and in the scenario itself, because the model will not answer the
// same way twice and later turns were written against the recorded answers.
func TestConvert_MultiTurnWarns(t *testing.T) {
	session := &SessionDetail{
		SessionSummary: SessionSummary{ID: "multi"},
		Messages: []types.Message{
			{Role: "user", Content: "one"}, {Role: "assistant", Content: "a"},
			{Role: "user", Content: "two"}, {Role: "assistant", Content: "b"},
			{Role: "user", Content: "three"},
		},
	}
	c, err := Convert(session, ConvertOptions{})
	require.NoError(t, err)
	assert.Len(t, c.Scenario.Spec.Turns, 3)
	require.Len(t, c.Warnings, 1)
	assert.Contains(t, c.Warnings[0], "3 user turns")
	assert.Contains(t, c.Scenario.Spec.Description, "3 user turns")

	single := &SessionDetail{SessionSummary: SessionSummary{ID: "one"}, Messages: []types.Message{{Role: "user", Content: "x"}}}
	c, err = Convert(single, ConvertOptions{})
	require.NoError(t, err)
	assert.Empty(t, c.Warnings)
}

// Multimodal parts on a user message survive: text as text, media by URL,
// file path or inline data, with the MIME type.
func TestConvert_PartsSurvive(t *testing.T) {
	url := "https://example.com/receipt.png"
	data := "AAAA"
	session := &SessionDetail{
		SessionSummary: SessionSummary{ID: "parts"},
		Messages: []types.Message{{
			Role: "user",
			Parts: []types.ContentPart{
				types.NewTextPart("What is on this receipt?"),
				{Type: types.ContentTypeImage, Media: &types.MediaContent{MIMEType: "image/png", URL: &url}},
				{Type: "document", Media: &types.MediaContent{MIMEType: "application/pdf", Data: &data}},
			},
		}},
	}
	c, err := Convert(session, ConvertOptions{})
	require.NoError(t, err)
	turn := c.Scenario.Spec.Turns[0]
	assert.Equal(t, "What is on this receipt?", turn.Content, "text is kept as content for readability")
	require.Len(t, turn.Parts, 3)
	assert.Equal(t, "text", turn.Parts[0].Type)
	assert.Equal(t, "What is on this receipt?", turn.Parts[0].Text)
	assert.Equal(t, "image", turn.Parts[1].Type)
	require.NotNil(t, turn.Parts[1].Media)
	assert.Equal(t, url, turn.Parts[1].Media.URL)
	assert.Equal(t, "image/png", turn.Parts[1].Media.MIMEType)
	assert.Equal(t, data, turn.Parts[2].Media.Data)
}

func TestConvert_TextOnlyMessageHasNoParts(t *testing.T) {
	session := &SessionDetail{SessionSummary: SessionSummary{ID: "plain"}, Messages: []types.Message{{Role: "user", Content: "hi"}}}
	c, err := Convert(session, ConvertOptions{})
	require.NoError(t, err)
	assert.Empty(t, c.Scenario.Spec.Turns[0].Parts)
}

func TestConvert_VariablesAndPack(t *testing.T) {
	session := &SessionDetail{
		SessionSummary: SessionSummary{ID: "vars"},
		Messages:       []types.Message{{Role: "user", Content: "hi"}},
		Variables:      map[string]string{"customer_tier": "gold"},
		Pack:           &PackRef{Name: "support", Version: "1.4.2", Digest: "sha256:abc"},
	}
	c, err := Convert(session, ConvertOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"customer_tier": "gold"}, c.Scenario.Spec.Variables)
	assert.Contains(t, c.Scenario.Spec.Description, "pack support 1.4.2 (sha256:abc)")
}

// task_type selects the workflow start state, so the recorded entry state's
// prompt_task wins over the option, which wins over the default.
func TestConvert_TaskTypePrecedence(t *testing.T) {
	base := func() *SessionDetail {
		return &SessionDetail{SessionSummary: SessionSummary{ID: "tt"}, Messages: []types.Message{{Role: "user", Content: "hi"}}}
	}
	c, err := Convert(base(), ConvertOptions{})
	require.NoError(t, err)
	assert.Equal(t, "conversation", c.Scenario.Spec.TaskType)

	c, err = Convert(base(), ConvertOptions{TaskType: "intake"})
	require.NoError(t, err)
	assert.Equal(t, "intake", c.Scenario.Spec.TaskType)

	s := base()
	s.Workflow = &WorkflowTrace{EntryState: "triage", EntryPromptTask: "triage_prompt"}
	c, err = Convert(s, ConvertOptions{TaskType: "intake"})
	require.NoError(t, err)
	assert.Equal(t, "triage_prompt", c.Scenario.Spec.TaskType)

	s.Workflow = &WorkflowTrace{EntryState: "triage"} // prompt task unknown
	c, err = Convert(s, ConvertOptions{TaskType: "intake"})
	require.NoError(t, err)
	assert.Equal(t, "intake", c.Scenario.Spec.TaskType)
}

func TestConvert_NilSession(t *testing.T) {
	_, err := Convert(nil, ConvertOptions{})
	require.Error(t, err)
}

func TestSanitizeID(t *testing.T) {
	tests := []struct{ in, want string }{
		{"session-123", "session-123"},
		{"Session_ABC 456", "session-abc-456"},
		{"---weird---", "weird"},
		{"", "generated"},
		{"2026-08-31T19-48-12Z-0001_gemini-flash_default_tool-usage_3a7b0b83_000b",
			"2026-08-31t19-48-12z-0001-gemini-flash-default-tool-usage-3a7b0"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, sanitizeID(tt.in), tt.in)
	}
}

func evalSession(evals ...EvalResult) *SessionDetail {
	return &SessionDetail{
		SessionSummary: SessionSummary{ID: "ev"},
		Messages: []types.Message{
			{Role: "user", Content: "first"}, {Role: "assistant", Content: "a"},
			{Role: "user", Content: "second"}, {Role: "assistant", Content: "b"},
		},
		Evals: evals,
	}
}

func intp(i int) *int { return &i }

// A recorded verdict is the strongest signal: failed assertions become
// scenario assertions with their recorded params; passed ones are dropped.
func TestConvert_RecordedVerdict(t *testing.T) {
	no, yes := false, true
	c, err := Convert(evalSession(
		EvalResult{ID: "a0", Type: "content_includes", Kind: "assertion", Passed: &no,
			Params: map[string]any{"patterns": []any{"policy"}}, Message: "must cite policy", Turn: intp(1)},
		EvalResult{ID: "a1", Type: "tools_called", Kind: "assertion", Passed: &yes,
			Params: map[string]any{"tools": []any{"lookup"}}},
		EvalResult{ID: "g0", Type: "toxicity", Kind: "guardrail", Passed: &no, Details: map[string]any{"action": "block"}},
	), ConvertOptions{})
	require.NoError(t, err)
	sc := c.Scenario.Spec

	require.Len(t, sc.Turns[1].Assertions, 1)
	assert.Equal(t, "content_includes", sc.Turns[1].Assertions[0].Type)
	assert.Equal(t, map[string]any{"patterns": []any{"policy"}}, sc.Turns[1].Assertions[0].Params)
	assert.Equal(t, "must cite policy", sc.Turns[1].Assertions[0].Message)
	assert.Empty(t, sc.Turns[0].Assertions)

	require.Len(t, sc.ConversationAssertions, 1, "the failed guardrail; the passed assertion is dropped")
	assert.Equal(t, "toxicity", sc.ConversationAssertions[0].Type)

	require.Len(t, c.Decisions, 3)
	byID := map[string]Decision{}
	for _, d := range c.Decisions {
		byID[d.EvalID] = d
	}
	assert.Equal(t, DecisionAssertedVerdict, byID["a0"].Outcome)
	assert.Equal(t, DecisionDroppedPassed, byID["a1"].Outcome)
	assert.Equal(t, DecisionAssertedVerdict, byID["g0"].Outcome)
}

// A measurement with a pack-declared threshold becomes an assertion bounded by
// it, whether or not the recorded score would have failed: the author stated
// the expectation. The decision records the recorded score against it.
func TestConvert_PackThreshold(t *testing.T) {
	c, err := Convert(evalSession(
		EvalResult{ID: "faith", Type: "faithfulness", Kind: "eval", Score: f(0.42),
			Params: map[string]any{"judge": "default"}, Threshold: &EvalThreshold{Operator: "gte", Value: 0.8}},
		EvalResult{ID: "tox", Type: "toxicity", Kind: "eval", Score: f(0.05),
			Threshold: &EvalThreshold{Operator: "lt", Value: 0.2}},
	), ConvertOptions{})
	require.NoError(t, err)
	sc := c.Scenario.Spec

	require.Len(t, sc.ConversationAssertions, 2)
	faith := sc.ConversationAssertions[0]
	assert.Equal(t, "faithfulness", faith.Type)
	assert.Equal(t, "default", faith.Params["judge"])
	assert.Equal(t, 0.8, faith.Params["min_score"])
	_, hasMax := faith.Params["max_score"]
	assert.False(t, hasMax)

	tox := sc.ConversationAssertions[1]
	assert.Equal(t, 0.2, tox.Params["max_score"])

	require.Len(t, c.Decisions, 2)
	assert.Equal(t, DecisionAssertedThreshold, c.Decisions[0].Outcome)
	assert.Contains(t, c.Decisions[0].Reason, "0.42")
	assert.Contains(t, c.Decisions[0].Reason, "0.8")
	assert.Contains(t, c.Decisions[1].Reason, "inclusive", "lt loses strictness and says so")
}

// A user expectation supplies the bound when the pack does not. It also has to
// pick the right eval: expectations are keyed by eval ID.
func TestConvert_UserExpectation(t *testing.T) {
	c, err := Convert(evalSession(
		EvalResult{ID: "faith", Type: "faithfulness", Kind: "eval", Score: f(0.42), Turn: intp(0)},
		EvalResult{ID: "other", Type: "answer_relevancy", Kind: "eval", Score: f(0.9)},
	), ConvertOptions{Expectations: []Expectation{{EvalID: "faith", Min: f(0.8)}}})
	require.NoError(t, err)
	sc := c.Scenario.Spec

	require.Len(t, sc.Turns[0].Assertions, 1)
	assert.Equal(t, "faithfulness", sc.Turns[0].Assertions[0].Type)
	assert.Equal(t, 0.8, sc.Turns[0].Assertions[0].Params["min_score"])
	assert.Empty(t, sc.ConversationAssertions, "the unbounded measurement is reported, not asserted")

	byID := map[string]Decision{}
	for _, d := range c.Decisions {
		byID[d.EvalID] = d
	}
	assert.Equal(t, DecisionAssertedExpectation, byID["faith"].Outcome)
	assert.Equal(t, DecisionReported, byID["other"].Outcome)
	assert.Contains(t, byID["other"].Reason, "--expect other>=<value>")
	assert.Contains(t, byID["other"].Reason, "0.9")
}

// Precedence when several sources apply: verdict, then pack threshold, then
// expectation. An expectation never overrides what the pack declared.
func TestConvert_DecisionPrecedence(t *testing.T) {
	no := false
	c, err := Convert(evalSession(
		EvalResult{ID: "x", Type: "faithfulness", Kind: "assertion", Passed: &no, Score: f(0.3),
			Params: map[string]any{"min_score": 0.9}, Threshold: &EvalThreshold{Operator: "gte", Value: 0.5}},
		EvalResult{ID: "y", Type: "faithfulness", Kind: "eval", Score: f(0.3),
			Threshold: &EvalThreshold{Operator: "gte", Value: 0.5}},
	), ConvertOptions{Expectations: []Expectation{{EvalID: "x", Min: f(0.1)}, {EvalID: "y", Min: f(0.99)}}})
	require.NoError(t, err)
	sc := c.Scenario.Spec
	require.Len(t, sc.ConversationAssertions, 2)
	assert.Equal(t, 0.9, sc.ConversationAssertions[0].Params["min_score"], "recorded params win")
	assert.Equal(t, 0.5, sc.ConversationAssertions[1].Params["min_score"], "pack threshold beats expectation")
}

// A pack threshold with an operator this cannot express falls through to the
// next source, and the reason says why.
func TestConvert_UntranslatableThresholdFallsThrough(t *testing.T) {
	c, err := Convert(evalSession(
		EvalResult{ID: "z", Type: "faithfulness", Kind: "eval", Score: f(0.3),
			Threshold: &EvalThreshold{Operator: "between", Value: 0.5}},
	), ConvertOptions{})
	require.NoError(t, err)
	assert.Empty(t, c.Scenario.Spec.ConversationAssertions)
	require.Len(t, c.Decisions, 1)
	assert.Equal(t, DecisionReported, c.Decisions[0].Outcome)
	assert.Contains(t, c.Decisions[0].Reason, "between")
}

// A turn index the scenario does not have (source and converter disagree) must
// not panic; the assertion lands at conversation level and the reason says so.
func TestConvert_OutOfRangeTurnFallsBackToConversation(t *testing.T) {
	no := false
	c, err := Convert(evalSession(
		EvalResult{ID: "a", Type: "content_includes", Kind: "assertion", Passed: &no, Turn: intp(7)},
	), ConvertOptions{})
	require.NoError(t, err)
	assert.Len(t, c.Scenario.Spec.ConversationAssertions, 1)
	assert.Contains(t, c.Decisions[0].Reason, "turn 7")
}

func TestDecision_String(t *testing.T) {
	d := Decision{SessionID: "s", EvalID: "faith", EvalType: "faithfulness", Score: f(0.42),
		Outcome: DecisionAssertedThreshold, MinScore: f(0.8), Reason: "pack threshold gte 0.8"}
	s := d.String()
	assert.Contains(t, s, "faith")
	assert.Contains(t, s, "0.42")
	assert.Contains(t, s, "min_score 0.8")
	assert.Contains(t, s, "pack threshold gte 0.8")
}
