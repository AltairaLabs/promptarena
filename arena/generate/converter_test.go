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
