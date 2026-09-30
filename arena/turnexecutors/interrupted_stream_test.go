package turnexecutors

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"

	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	runtimestore "github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

var errStreamDropped = errors.New("upstream dropped the stream")

// cancelMidStreamProvider streams a partial reply, then waits for the turn to
// be canceled and closes without an error chunk, as a provider that just stops
// reading does. sent is closed once the partial reply is out.
type cancelMidStreamProvider struct {
	MockStreamingProvider
	sent chan struct{}
}

func (p *cancelMidStreamProvider) PredictStream(
	ctx context.Context, _ providers.PredictionRequest,
) (<-chan providers.StreamChunk, error) {
	ch := make(chan providers.StreamChunk)
	go func() {
		defer close(ch)
		ch <- providers.StreamChunk{Content: "I will sea", Delta: "I will sea"}
		close(p.sent)
		<-ctx.Done()
	}()
	return ch, nil
}

func newInterruptedArenaStore(t *testing.T) (*statestore.ArenaStateStore, *StateStoreConfig) {
	t.Helper()
	store := statestore.NewArenaStateStore()
	require.NoError(t, store.Save(context.Background(), &runtimestore.ConversationState{
		ID: "conv", UserID: "u", Messages: []types.Message{}, Metadata: map[string]interface{}{},
	}))
	return store, &StateStoreConfig{Store: store, UserID: "u"}
}

func interruptedReq(provider providers.Provider, cfg *StateStoreConfig, asserts []assertions.AssertionConfig) TurnRequest {
	return TurnRequest{
		Provider:         provider,
		Scenario:         &arenaconfig.Scenario{TaskType: "test"},
		TaskType:         "test",
		StateStoreConfig: cfg,
		ConversationID:   "conv",
		TurnEvalRunner:   &contentIncludesTurnEvalRunner{},
		Assertions:       asserts,
	}
}

func lastSaved(t *testing.T, store *statestore.ArenaStateStore) []types.Message {
	t.Helper()
	state, err := store.GetArenaState(context.Background(), "conv")
	require.NoError(t, err)
	return state.Messages
}

var searchAssertion = []assertions.AssertionConfig{{
	Type:    "content_includes",
	Params:  map[string]interface{}{"patterns": []string{"search"}},
	Message: "Expected content to include search",
}}

func TestArena_InterruptedStream_SavesPartialReply(t *testing.T) {
	for name, asserts := range map[string][]assertions.AssertionConfig{
		"no assertions":   nil,
		"with assertions": searchAssertion,
	} {
		t.Run(name, func(t *testing.T) {
			provider := &MockStreamingProvider{streamChunks: []providers.StreamChunk{
				{Content: "I will sea", Delta: "I will sea"},
				{Error: errStreamDropped},
			}}
			store, cfg := newInterruptedArenaStore(t)
			req := interruptedReq(provider, cfg, asserts)
			user := types.Message{Role: "user", Content: "Search for something"}

			err := NewPipelineExecutor(tools.NewRegistry(), nil).Execute(context.Background(), &req, &user)
			require.ErrorIs(t, err, errStreamDropped, "the stream failure must be the turn's error")

			msgs := lastSaved(t, store)
			require.NotEmpty(t, msgs)
			last := msgs[len(msgs)-1]
			require.Equal(t, "user", msgs[len(msgs)-2].Role, "the user message is saved before the reply")
			assert.Equal(t, "I will sea", last.Content)
			assert.True(t, last.IsInterrupted(), "the saved reply must be marked interrupted")
			assert.Nil(t, last.Meta["assertions"], "assertions must not be judged against a fragment")
		})
	}
}

func TestArena_CanceledTurn_SavesPartialReply(t *testing.T) {
	for name, asserts := range map[string][]assertions.AssertionConfig{
		"no assertions":   nil,
		"with assertions": searchAssertion,
	} {
		t.Run(name, func(t *testing.T) {
			provider := &cancelMidStreamProvider{sent: make(chan struct{})}
			store, cfg := newInterruptedArenaStore(t)
			req := interruptedReq(provider, cfg, asserts)
			user := types.Message{Role: "user", Content: "Search for something"}

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				<-provider.sent
				cancel()
			}()
			err := NewPipelineExecutor(tools.NewRegistry(), nil).Execute(ctx, &req, &user)
			require.ErrorIs(t, err, context.Canceled)

			msgs := lastSaved(t, store)
			require.NotEmpty(t, msgs)
			last := msgs[len(msgs)-1]
			assert.Equal(t, "I will sea", last.Content)
			assert.True(t, last.IsInterrupted(), "a canceled turn must still save its partial reply")
		})
	}
}
