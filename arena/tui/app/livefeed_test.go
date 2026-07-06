package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/statestore"
	"github.com/AltairaLabs/promptarena/arena/tui"
	"github.com/AltairaLabs/promptarena/arena/tui/panels"
)

func seededPanel(t *testing.T, runID string, n int) *panels.ConversationPanel {
	t.Helper()
	panel := panels.NewConversationPanel()
	panel.SetDimensions(80, 20)
	msgs := make([]types.Message, n)
	for i := range msgs {
		msgs[i] = types.Message{Role: "user", Content: "seed"}
	}
	panel.SetData(runID, "scn", "prov", &statestore.RunResult{Messages: msgs})
	return panel
}

// TestLiveFeed_AppendsNewDedupsOld verifies new turns append while turns
// already covered by the seed are ignored, and other conversations pass through.
func TestLiveFeed_AppendsNewDedupsOld(t *testing.T) {
	panel := seededPanel(t, "conv-1", 2)
	require.Equal(t, 2, panel.MessageCount())

	f := newLiveFeed("conv-1", 2)

	require.True(t, f.Apply(panel, tui.MessageCreatedMsg{
		ConversationID: "conv-1", Index: 2, Role: "assistant", Content: "new",
	}))
	require.Equal(t, 3, panel.MessageCount(), "new turn appended")

	// Older index for the same conversation: consumed, but not appended.
	require.True(t, f.Apply(panel, tui.MessageCreatedMsg{
		ConversationID: "conv-1", Index: 1, Role: "user", Content: "old",
	}))
	require.Equal(t, 3, panel.MessageCount(), "duplicate/old index ignored")

	// Different conversation: not consumed.
	require.False(t, f.Apply(panel, tui.MessageCreatedMsg{ConversationID: "conv-2", Index: 0}))
	require.Equal(t, 3, panel.MessageCount())
}

// TestLiveFeed_SystemMessageDoesNotDropUserTurns reproduces the "only assistant
// turns show live" bug: the system prompt is broadcast as a MessageCreated at
// index 0. If it goes through the transcript dedup it advances the append
// counter by one, shifting every subsequent odd index so user turns get
// skipped. It must be routed to the prompt path instead.
func TestLiveFeed_SystemMessageDoesNotDropUserTurns(t *testing.T) {
	panel := seededPanel(t, "conv-1", 0) // drilled in at start: empty seed
	f := newLiveFeed("conv-1", 0)

	// The exact broadcast sequence: system at transcript index 0, then
	// interleaved user / assistant / tool turns at sequential indices.
	seq := []tui.MessageCreatedMsg{
		{ConversationID: "conv-1", Index: 0, Role: "system", Content: "you are an agent"},
		{ConversationID: "conv-1", Index: 1, Role: "user", Content: "I want a refund"},
		{ConversationID: "conv-1", Index: 2, Role: "assistant", Content: "let me look"},
		{ConversationID: "conv-1", Index: 3, Role: "tool", Content: "{result}"},
		{ConversationID: "conv-1", Index: 4, Role: "assistant", Content: "found it"},
		{ConversationID: "conv-1", Index: 5, Role: "user", Content: "thanks so much"},
		{ConversationID: "conv-1", Index: 6, Role: "assistant", Content: "you are welcome"},
	}
	for i := range seq {
		require.True(t, f.Apply(panel, seq[i]))
	}

	// system (prepended) + 6 turns = 7 messages; before the fix the user turns
	// (and tool) were dropped, leaving only system + 3 assistants = 4.
	require.Equal(t, 7, panel.MessageCount(), "all turns present, no user turns dropped")

	panel.SetDimensions(160, 40)
	out := panel.View()
	require.Contains(t, out, "I want a refund", "first user turn must show live")
	require.Contains(t, out, "thanks so much", "later user turn must show live")
}

// TestLiveFeed_CompletedTurnShowsReasoning proves the live interactive path:
// a MessageCreatedMsg carrying Reasoning is appended to the panel and the
// completed turn's detail view renders the reasoning (not just the transient
// streaming pane). Guards the event->tui-message->panel reasoning seam.
func TestLiveFeed_CompletedTurnShowsReasoning(t *testing.T) {
	panel := seededPanel(t, "conv-1", 1)
	f := newLiveFeed("conv-1", 1)

	// Single non-wrapping token: the detail pane line-wraps and ANSI-styles
	// prose, so a multi-word phrase won't survive as a contiguous substring.
	require.True(t, f.Apply(panel, tui.MessageCreatedMsg{
		ConversationID: "conv-1", Index: 1, Role: "assistant", Content: "ANSWER: 16",
		Reasoning: &types.ReasoningTrace{Text: "THOUGHTMARKER"},
	}))
	require.Equal(t, 2, panel.MessageCount())

	panel.SelectLast()
	view := panel.View()
	require.Contains(t, view, "Reasoning", "detail view shows the reasoning section")
	require.Contains(t, view, "THOUGHTMARKER", "reasoning text rendered in the completed turn")
}

// TestLiveFeed_AudioSystemPromptAndMetadata covers the audio, conversation
// started (system prompt), and message updated paths.
func TestLiveFeed_AudioSystemPromptAndMetadata(t *testing.T) {
	panel := seededPanel(t, "conv-1", 1)
	f := newLiveFeed("conv-1", 1)

	// Audio frames are always consumed.
	require.True(t, f.Apply(panel, tui.AudioLevelMsg{UserLevel: 0.5, AgentLevel: 0.2}))

	// System prompt prepended once.
	require.True(t, f.Apply(panel, tui.ConversationStartedMsg{
		ConversationID: "conv-1", SystemPrompt: "You are X",
	}))
	require.True(t, panel.HasSystemPrompt())
	require.Equal(t, 2, panel.MessageCount(), "system + seeded user")

	// A second started event is consumed but does not duplicate the prompt.
	require.True(t, f.Apply(panel, tui.ConversationStartedMsg{
		ConversationID: "conv-1", SystemPrompt: "other",
	}))
	require.Equal(t, 2, panel.MessageCount())

	// Metadata update for an existing index: consumed, no panic.
	require.True(t, f.Apply(panel, tui.MessageUpdatedMsg{
		ConversationID: "conv-1", Index: 1, LatencyMs: 100, TotalCost: 0.01,
	}))

	// Started for another conversation is not consumed.
	require.False(t, f.Apply(panel, tui.ConversationStartedMsg{ConversationID: "x"}))

	// Unknown message types are not consumed.
	require.False(t, f.Apply(panel, struct{}{}))
}

// TestLiveFeed_ReasoningStreamsThenClears covers the live reasoning path:
// ReasoningDeltaMsg accumulates transient thinking; the turn's message clears it.
func TestLiveFeed_ReasoningStreamsThenClears(t *testing.T) {
	panel := seededPanel(t, "conv-1", 1)
	f := newLiveFeed("conv-1", 1)

	require.True(t, f.Apply(panel, tui.ReasoningDeltaMsg{Text: "thinking "}))
	require.True(t, f.Apply(panel, tui.ReasoningDeltaMsg{Text: "hard"}))
	require.Equal(t, "thinking hard", panel.LiveReasoning())

	// The turn's message arriving clears the transient reasoning.
	require.True(t, f.Apply(panel, tui.MessageCreatedMsg{
		ConversationID: "conv-1", Index: 1, Role: "assistant", Content: "answer",
	}))
	require.Empty(t, panel.LiveReasoning(), "reasoning cleared when the turn message arrives")
}
