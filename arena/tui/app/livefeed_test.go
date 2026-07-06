package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/statestore"
	"github.com/AltairaLabs/promptarena/arena/tui"
	"github.com/AltairaLabs/promptarena/arena/tui/panels"
)

// fakeConvStore returns a canned transcript per conversation, standing in for
// the arena state store the live feed reconciles against.
type fakeConvStore struct {
	msgs map[string][]types.Message
}

func (f *fakeConvStore) set(convID string, msgs ...types.Message) {
	if f.msgs == nil {
		f.msgs = map[string][]types.Message{}
	}
	f.msgs[convID] = msgs
}

func (f *fakeConvStore) GetArenaState(_ context.Context, id string) (*statestore.ArenaConversationState, error) {
	m, ok := f.msgs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	st := &statestore.ArenaConversationState{}
	st.Messages = m
	return st, nil
}

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

// TestLiveFeed_ReconcilesFullTranscript is the core guard: a message event makes
// the feed pull the WHOLE transcript from the store, so every turn — including
// the user turns that the old index-math dropped — appears, in order.
func TestLiveFeed_ReconcilesFullTranscript(t *testing.T) {
	panel := seededPanel(t, "conv-1", 0) // drilled in at start
	store := &fakeConvStore{}
	f := newLiveFeed("conv-1", store)

	// The authoritative transcript, exactly as the completed view would show it:
	// system + interleaved user/assistant/tool.
	store.set("conv-1",
		types.Message{Role: "system", Content: "you are an agent"},
		types.Message{Role: "user", Content: "I want a refund"},
		types.Message{Role: "assistant", Content: "let me look"},
		types.Message{Role: "tool", Content: "{result}"},
		types.Message{Role: "assistant", Content: "found it"},
		types.Message{Role: "user", Content: "thanks so much"},
		types.Message{Role: "assistant", Content: "you are welcome"},
	)

	// Any message event for this conversation triggers a reconcile.
	require.True(t, f.Apply(panel, tui.MessageCreatedMsg{ConversationID: "conv-1", Index: 6, Role: "assistant"}))

	require.Equal(t, 7, panel.MessageCount(), "live view holds the full transcript, no dropped turns")
	panel.SetDimensions(160, 40)
	out := panel.View()
	require.Contains(t, out, "I want a refund", "first user turn shows live")
	require.Contains(t, out, "thanks so much", "later user turn shows live")

	// An event for another conversation is not consumed.
	require.False(t, f.Apply(panel, tui.MessageCreatedMsg{ConversationID: "conv-2", Index: 0}))
}

// TestLiveFeed_ReconcilePicksUpMetadataUpdate verifies MessageUpdated also
// reconciles (cost/latency landing on an existing turn).
func TestLiveFeed_ReconcilePicksUpMetadataUpdate(t *testing.T) {
	panel := seededPanel(t, "conv-1", 0)
	store := &fakeConvStore{}
	f := newLiveFeed("conv-1", store)

	store.set("conv-1", types.Message{Role: "user", Content: "hello"})
	require.True(t, f.Apply(panel, tui.MessageUpdatedMsg{ConversationID: "conv-1", Index: 0}))
	require.Equal(t, 1, panel.MessageCount())
}

// TestLiveFeed_CompletedTurnShowsReasoning proves reasoning attached to a stored
// turn renders in the detail view after a reconcile.
func TestLiveFeed_CompletedTurnShowsReasoning(t *testing.T) {
	panel := seededPanel(t, "conv-1", 0)
	store := &fakeConvStore{}
	f := newLiveFeed("conv-1", store)

	store.set("conv-1",
		types.Message{Role: "user", Content: "q"},
		types.Message{Role: "assistant", Content: "ANSWER: 16", Reasoning: &types.ReasoningTrace{Text: "THOUGHTMARKER"}},
	)
	require.True(t, f.Apply(panel, tui.MessageCreatedMsg{ConversationID: "conv-1", Index: 1, Role: "assistant"}))
	require.Equal(t, 2, panel.MessageCount())

	panel.SelectLast()
	view := panel.View()
	require.Contains(t, view, "Reasoning", "detail view shows the reasoning section")
	require.Contains(t, view, "THOUGHTMARKER", "reasoning text rendered in the completed turn")
}

// TestLiveFeed_AudioSystemPromptAndOtherConv covers audio frames, the system
// prompt fast-path, and conversation filtering.
func TestLiveFeed_AudioSystemPromptAndOtherConv(t *testing.T) {
	panel := seededPanel(t, "conv-1", 1)
	store := &fakeConvStore{}
	f := newLiveFeed("conv-1", store)

	// Audio frames are always consumed.
	require.True(t, f.Apply(panel, tui.AudioLevelMsg{UserLevel: 0.5, AgentLevel: 0.2}))

	// System prompt prepended once, before the store has one.
	require.True(t, f.Apply(panel, tui.ConversationStartedMsg{ConversationID: "conv-1", SystemPrompt: "You are X"}))
	require.True(t, panel.HasSystemPrompt())
	require.True(t, f.Apply(panel, tui.ConversationStartedMsg{ConversationID: "conv-1", SystemPrompt: "other"}))

	// Events for another conversation / unknown types are not consumed.
	require.False(t, f.Apply(panel, tui.ConversationStartedMsg{ConversationID: "x"}))
	require.False(t, f.Apply(panel, struct{}{}))
}

// TestLiveFeed_ReasoningStreamsThenClears covers the live reasoning path:
// ReasoningDeltaMsg accumulates transient thinking; the turn's message clears it.
func TestLiveFeed_ReasoningStreamsThenClears(t *testing.T) {
	panel := seededPanel(t, "conv-1", 1)
	store := &fakeConvStore{}
	store.set("conv-1", types.Message{Role: "user", Content: "seed"}, types.Message{Role: "assistant", Content: "answer"})
	f := newLiveFeed("conv-1", store)

	require.True(t, f.Apply(panel, tui.ReasoningDeltaMsg{Text: "thinking "}))
	require.True(t, f.Apply(panel, tui.ReasoningDeltaMsg{Text: "hard"}))
	require.Equal(t, "thinking hard", panel.LiveReasoning())

	// The turn's message arriving clears the transient reasoning.
	require.True(t, f.Apply(panel, tui.MessageCreatedMsg{ConversationID: "conv-1", Index: 1, Role: "assistant"}))
	require.Empty(t, panel.LiveReasoning(), "reasoning cleared when the turn message arrives")
}

// TestLiveFeed_PreservesSystemPromptOnSystemlessSync guards SyncMessages: a
// store transcript without a leading system row must not blank a system prompt
// already shown via ConversationStarted.
func TestLiveFeed_PreservesSystemPromptOnSystemlessSync(t *testing.T) {
	panel := seededPanel(t, "conv-1", 0)
	store := &fakeConvStore{}
	f := newLiveFeed("conv-1", store)

	require.True(t, f.Apply(panel, tui.ConversationStartedMsg{ConversationID: "conv-1", SystemPrompt: "You are X"}))
	require.True(t, panel.HasSystemPrompt())

	// Store transcript arrives WITHOUT a system row.
	store.set("conv-1", types.Message{Role: "user", Content: "hi"})
	require.True(t, f.Apply(panel, tui.MessageCreatedMsg{ConversationID: "conv-1", Index: 0, Role: "user"}))

	require.True(t, panel.HasSystemPrompt(), "system prompt preserved across a system-less sync")
	require.Equal(t, 2, panel.MessageCount(), "system + user")
}
