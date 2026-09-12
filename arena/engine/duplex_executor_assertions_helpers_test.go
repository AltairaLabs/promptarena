package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	arenaassertions "github.com/AltairaLabs/promptarena/arena/assertions"
	"github.com/AltairaLabs/promptarena/arena/statestore"

	runtimestore "github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
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

// TestEvaluateTurnAssertions_GuardsBeforeEvaluating covers evaluateTurnAssertions,
// which was 0%. Every guard here returns silently, so getting one wrong does not
// raise — it just leaves a turn's assertions unevaluated, and a turn with no
// results reads downstream as a turn that passed.
func TestEvaluateTurnAssertions_GuardsBeforeEvaluating(t *testing.T) {
	ctx := context.Background()

	t.Run("a turn with no assertions does nothing", func(t *testing.T) {
		de := &DuplexConversationExecutor{}
		// No state store and no orchestrator: reaching past this guard would
		// dereference one of them.
		de.evaluateTurnAssertions(ctx, &ConversationRequest{}, &arenaconfig.TurnDefinition{}, 0)
	})

	t.Run("no conversation history stops before the orchestrator", func(t *testing.T) {
		de := &DuplexConversationExecutor{}
		turn := &arenaconfig.TurnDefinition{
			Assertions: []arenaconfig.AssertionConfig{{Type: "contains", Params: map[string]interface{}{"value": "hello"}}},
		}
		// StateStoreConfig is nil, so getConversationHistory returns nil.
		de.evaluateTurnAssertions(ctx, &ConversationRequest{}, turn, 1)
	})

	t.Run("history with no assistant message stops too", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		convID := "conv-user-only"
		require.NoError(t, store.Save(ctx, &runtimestore.ConversationState{
			ID:       convID,
			Messages: []types.Message{{Role: "user", Content: "hi"}},
		}))

		de := &DuplexConversationExecutor{}
		req := &ConversationRequest{
			ConversationID:   convID,
			StateStoreConfig: &StateStoreConfig{Store: store},
		}
		turn := &arenaconfig.TurnDefinition{
			Assertions: []arenaconfig.AssertionConfig{{Type: "contains", Params: map[string]interface{}{"value": "hello"}}},
		}
		de.evaluateTurnAssertions(ctx, req, turn, 2)
	})

	t.Run("a nil orchestrator stops after finding the assistant message", func(t *testing.T) {
		store := statestore.NewArenaStateStore()
		convID := "conv-no-orchestrator"
		require.NoError(t, store.Save(ctx, &runtimestore.ConversationState{
			ID: convID,
			Messages: []types.Message{
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: "hello there"},
			},
		}))

		de := &DuplexConversationExecutor{evalOrchestrator: nil}
		req := &ConversationRequest{
			ConversationID:   convID,
			StateStoreConfig: &StateStoreConfig{Store: store},
		}
		turn := &arenaconfig.TurnDefinition{
			Assertions: []arenaconfig.AssertionConfig{{Type: "contains", Params: map[string]interface{}{"value": "hello"}}},
		}
		de.evaluateTurnAssertions(ctx, req, turn, 3)
	})
}

// TestLastAssistantMessage_PicksTheMostRecent pins the helper the guard above
// depends on: assertions are scored against the LAST assistant turn, so
// returning an earlier one would grade the wrong reply.
func TestLastAssistantMessage_PicksTheMostRecent(t *testing.T) {
	assert.Nil(t, lastAssistantMessage(nil))
	assert.Nil(t, lastAssistantMessage([]types.Message{{Role: "user", Content: "hi"}}))

	got := lastAssistantMessage([]types.Message{
		{Role: "assistant", Content: "first"},
		{Role: "user", Content: "more"},
		{Role: "assistant", Content: "second"},
	})
	require.NotNil(t, got)
	assert.Equal(t, "second", got.Content)
}
