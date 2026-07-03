package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runtimestore "github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	arenaassertions "github.com/AltairaLabs/promptarena/arena/assertions"
	"github.com/AltairaLabs/promptarena/arena/statestore"
)

func TestCountPassedFailedAssertions(t *testing.T) {
	de := &DuplexConversationExecutor{}
	results := []arenaassertions.AssertionResult{
		{Passed: true},
		{Passed: false},
		{Passed: true},
	}
	assert.Equal(t, 2, de.countPassedAssertions(results))
	assert.Equal(t, 1, de.countFailedAssertions(results))
	assert.Equal(t, 0, de.countPassedAssertions(nil))
	assert.Equal(t, 0, de.countFailedAssertions(nil))
}

func TestStoreAssertionResults(t *testing.T) {
	de := &DuplexConversationExecutor{}
	results := []arenaassertions.AssertionResult{
		{Passed: true, Message: "ok"},
		{Passed: false},
	}

	t.Run("no-op without a state store", func(t *testing.T) {
		msg := &types.Message{Role: roleAssistant}
		assert.NotPanics(t, func() {
			de.storeAssertionResults(&ConversationRequest{}, msg, results)
		})
		assert.Nil(t, msg.Meta["assertions"])
	})

	t.Run("writes summary metadata and persists to store", func(t *testing.T) {
		ctx := context.Background()
		store := statestore.NewArenaStateStore()
		convID := "conv-assert"
		require.NoError(t, store.Save(ctx, &runtimestore.ConversationState{
			ID: convID,
			Messages: []types.Message{
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: "reply"},
			},
		}))

		req := &ConversationRequest{
			ConversationID:   convID,
			StateStoreConfig: &StateStoreConfig{Store: store},
		}
		msg := &types.Message{Role: roleAssistant, Content: "reply"}
		de.storeAssertionResults(req, msg, results)

		summary, ok := msg.Meta["assertions"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, 2, summary["total"])
		assert.Equal(t, 1, summary["failed"])
		assert.Equal(t, false, summary["all_passed"])
		list, ok := summary["results"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, list, 2)
		assert.Equal(t, "ok", list[0]["message"])
	})
}

func TestDuplexEvaluateConversationAssertions_NoOrchestrator(t *testing.T) {
	de := &DuplexConversationExecutor{}
	req := &ConversationRequest{
		Scenario: &arenaconfig.Scenario{
			ConversationAssertions: []arenaconfig.AssertionConfig{
				{Type: "contains", Message: "must contain X"},
			},
		},
	}
	results := de.evaluateConversationAssertions(context.Background(), req, nil)
	require.Len(t, results, 1)
	assert.False(t, results[0].Passed)
	assert.Equal(t, "contains", results[0].Type)
	assert.Equal(t, "eval runner not configured", results[0].Details["error"])
}

func TestDuplexEvaluateConversationAssertions_NoAssertions(t *testing.T) {
	de := &DuplexConversationExecutor{}
	req := &ConversationRequest{Scenario: &arenaconfig.Scenario{}}
	assert.Nil(t, de.evaluateConversationAssertions(context.Background(), req, nil))
}
