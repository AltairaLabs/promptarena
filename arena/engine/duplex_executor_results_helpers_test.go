package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runtimestore "github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/arena/statestore"
)

func TestGetConversationHistory(t *testing.T) {
	de := &DuplexConversationExecutor{}
	ctx := context.Background()

	t.Run("nil without a state store", func(t *testing.T) {
		assert.Nil(t, de.getConversationHistory(ctx, &ConversationRequest{}))
	})

	t.Run("nil when store is not a statestore.Store", func(t *testing.T) {
		req := &ConversationRequest{StateStoreConfig: &StateStoreConfig{Store: "not a store"}}
		assert.Nil(t, de.getConversationHistory(ctx, req))
	})

	t.Run("returns messages from the store", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		convID := "conv-hist"
		require.NoError(t, store.Save(ctx, &runtimestore.ConversationState{
			ID:       convID,
			Messages: []types.Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "yo"}},
		}))
		req := &ConversationRequest{
			ConversationID:   convID,
			StateStoreConfig: &StateStoreConfig{Store: store},
		}
		got := de.getConversationHistory(ctx, req)
		require.Len(t, got, 2)
		assert.Equal(t, "yo", got[1].Content)
	})

	t.Run("nil for an unknown conversation", func(t *testing.T) {
		req := &ConversationRequest{
			ConversationID:   "missing",
			StateStoreConfig: &StateStoreConfig{Store: statestore.NewArenaStateStore()},
		}
		assert.Nil(t, de.getConversationHistory(ctx, req))
	})
}

func TestDuplexBuildConversationContext(t *testing.T) {
	de := &DuplexConversationExecutor{}
	req := &ConversationRequest{
		Scenario: &arenaconfig.Scenario{ID: "scenario-1"},
	}
	messages := []types.Message{{Role: "user", Content: "hello"}}
	convCtx := de.buildConversationContext(req, messages)
	require.NotNil(t, convCtx)
	assert.Equal(t, "scenario-1", convCtx.Metadata.ScenarioID)
	assert.Empty(t, convCtx.Metadata.ProviderID, "nil provider yields empty provider id")
}
