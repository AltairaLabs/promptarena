package engine

import (
	"context"
	"testing"

	runtimestore "github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"
	"github.com/AltairaLabs/promptarena/v2/arena/turnexecutors"
)

// builderRecordingExecutor records the context builder each turn carries.
type builderRecordingExecutor struct {
	stampingTurnExecutor
	builders []*turnexecutors.ConversationContextBuilder
}

func (e *builderRecordingExecutor) ExecuteTurn(ctx context.Context, req turnexecutors.TurnRequest) error {
	e.builders = append(e.builders, req.ContextBuilder)
	return e.stampingTurnExecutor.ExecuteTurn(ctx, req)
}

func (e *builderRecordingExecutor) ExecuteTurnStream(
	ctx context.Context, req turnexecutors.TurnRequest,
) (<-chan turnexecutors.MessageStreamChunk, error) {
	ch := make(chan turnexecutors.MessageStreamChunk)
	close(ch)
	return ch, e.ExecuteTurn(ctx, req)
}

// Every turn of a conversation shares one context builder, so its relevance
// embedding cache carries over; separate conversations never share one.
func TestExecuteConversation_ShareOneContextBuilderPerConversation(t *testing.T) {
	ctx := context.Background()
	run := func(convID string, streaming bool) []*turnexecutors.ConversationContextBuilder {
		store := statestore.NewArenaStateStore()
		require.NoError(t, store.Save(ctx, &runtimestore.ConversationState{ID: convID}))
		rec := &builderRecordingExecutor{}
		ce := &DefaultConversationExecutor{scriptedExecutor: rec}
		req := ConversationRequest{
			Provider: &MockProvider{id: "mock"},
			Scenario: &arenaconfig.Scenario{
				ID:        "sc",
				Streaming: streaming,
				Turns: []arenaconfig.TurnDefinition{
					{Role: "user", Content: "turn one"},
					{Role: "user", Content: "turn two"},
				},
			},
			Config:           &arenaconfig.Config{},
			ConversationID:   convID,
			RunID:            convID,
			StateStoreConfig: &StateStoreConfig{Store: store},
		}
		res := ce.ExecuteConversation(ctx, req)
		require.False(t, res.Failed, res.Error)
		return rec.builders
	}

	for _, streaming := range []bool{false, true} {
		first := run("conv-a", streaming)
		require.Len(t, first, 2)
		require.NotNil(t, first[0])
		assert.Same(t, first[0], first[1], "streaming=%v: turns share the builder", streaming)

		second := run("conv-b", streaming)
		require.Len(t, second, 2)
		assert.NotSame(t, first[0], second[0], "streaming=%v: conversations do not", streaming)
	}
}

func TestExecuteConversationStream_ShareOneContextBuilder(t *testing.T) {
	rec := &builderRecordingExecutor{}
	ce := &DefaultConversationExecutor{scriptedExecutor: rec}
	req := ConversationRequest{
		Provider: &MockProvider{id: "mock"},
		Scenario: &arenaconfig.Scenario{
			ID:    "sc",
			Turns: []arenaconfig.TurnDefinition{{Role: "user", Content: "one"}, {Role: "user", Content: "two"}},
		},
		Config: &arenaconfig.Config{},
	}
	ch, err := ce.ExecuteConversationStream(context.Background(), req)
	require.NoError(t, err)
	for chunk := range ch {
		require.NoError(t, chunk.Error)
	}
	require.Len(t, rec.builders, 2)
	require.NotNil(t, rec.builders[0])
	assert.Same(t, rec.builders[0], rec.builders[1])
}
