package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/statestore"
	"github.com/AltairaLabs/promptarena/arena/tui"
	"github.com/AltairaLabs/promptarena/arena/tui/panels"
)

// roleSystemMessage is the message role for the system prompt.
const roleSystemMessage = "system"

// conversationStore is the slice of the arena state store the live feed needs
// to read the in-progress transcript for a conversation.
type conversationStore interface {
	GetArenaState(ctx context.Context, id string) (*statestore.ArenaConversationState, error)
}

// liveFeed keeps a ConversationPanel in sync with an in-progress run during a
// drill-in. Rather than reconstruct the transcript from individual message
// events — which is fragile to the system-prompt offset, seed alignment, and
// gaps when persistence lags the broadcast — it treats a message event merely
// as a signal to RECONCILE the panel against the store, which is the same
// source of truth the completed (static) view uses. So the live view always
// matches the final one; it can only ever be a beat behind, never wrong.
type liveFeed struct {
	convID string
	store  conversationStore
}

func newLiveFeed(convID string, store conversationStore) *liveFeed {
	return &liveFeed{convID: convID, store: store}
}

// Apply routes msg into panel. It returns true when msg was consumed: an
// audio-level frame (always), or a message/conversation event for this feed's
// conversation. Events for other conversations return false so the host can
// handle them.
func (f *liveFeed) Apply(panel *panels.ConversationPanel, msg tea.Msg) bool {
	switch m := msg.(type) {
	case tui.AudioLevelMsg:
		panel.SetAudioLevels(m.UserLevel, m.AgentLevel, true)
		return true
	case tui.ConversationStartedMsg:
		if m.ConversationID != f.convID {
			return false
		}
		// Fast-path the system prompt before the first turn is persisted; once
		// it is, reconcile keeps it (the store transcript leads with system).
		if !panel.HasSystemPrompt() {
			panel.PrependSystemPrompt(&types.Message{
				Role:      roleSystemMessage,
				Content:   m.SystemPrompt,
				Timestamp: m.Time,
			})
		}
		return true
	case tui.ReasoningDeltaMsg:
		// Non-content live thinking — accumulate transiently for this turn.
		panel.AppendLiveReasoning(m.Text)
		return true
	case tui.MessageCreatedMsg:
		if m.ConversationID != f.convID {
			return false
		}
		// The turn's message arrived — clear the transient thinking display and
		// pull the authoritative transcript.
		panel.ClearLiveReasoning()
		f.reconcile(panel)
		return true
	case tui.MessageUpdatedMsg:
		if m.ConversationID != f.convID {
			return false
		}
		// Cost/latency landed on an existing message — reconcile picks it up.
		f.reconcile(panel)
		return true
	}
	return false
}

// reconcile replaces the panel's transcript with the store's current messages
// for this conversation. A no-op until the store has the conversation (nil
// store, not-found, or empty) so an early drill-in doesn't blank the seed.
func (f *liveFeed) reconcile(panel *panels.ConversationPanel) {
	if f.store == nil {
		return
	}
	st, err := f.store.GetArenaState(context.Background(), f.convID)
	if err != nil || st == nil || len(st.Messages) == 0 {
		return
	}
	panel.SyncMessages(st.Messages)
}
