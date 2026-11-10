package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"

	runtimestore "github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// Both conversation executors deliver when: eval files once the last turn is
// done and before scoring, and a failed delivery fails the run rather than
// letting evals run against a sandbox that is missing their files.
func TestBuildResult_DeliversEvalFilesBeforeScoring(t *testing.T) {
	builders := map[string]func(context.Context, *ConversationRequest) *ConversationResult{
		"text":   NewDefaultConversationExecutor(nil, nil, nil, nil, nil).buildResultFromStateStore,
		"duplex": NewDuplexConversationExecutor(nil, nil, nil, nil, nil).buildResultFromStateStore,
	}
	for name, build := range builders {
		store := statestore.NewArenaStateStore()
		require.NoError(t, store.Save(context.Background(), &runtimestore.ConversationState{
			ID:       "conv",
			Messages: []types.Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}},
		}))
		req := &ConversationRequest{
			ConversationID:   "conv",
			Scenario:         &arenaconfig.Scenario{ID: "s"},
			StateStoreConfig: &StateStoreConfig{Store: store},
		}

		calls := 0
		ok := build(withEvalDelivery(context.Background(), func(context.Context) error { calls++; return nil }), req)
		assert.False(t, ok.Failed, "%s: %s", name, ok.Error)
		assert.Equal(t, 1, calls, "%s: delivery runs once per result", name)
		assert.Len(t, ok.Messages, 2, name)

		failed := build(withEvalDelivery(context.Background(), func(context.Context) error {
			return errors.New("docker cp: no space left")
		}), req)
		assert.True(t, failed.Failed, name)
		assert.Contains(t, failed.Error, "deliver eval files: docker cp: no space left", name)
	}
}
