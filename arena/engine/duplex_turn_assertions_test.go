package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	arenaassertions "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	runtimestore "github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func includesTurn(pattern string) *arenaconfig.TurnDefinition {
	return &arenaconfig.TurnDefinition{
		Role: "user",
		Assertions: []arenaconfig.AssertionConfig{{
			Type:   "content_includes",
			Params: map[string]interface{}{"patterns": []interface{}{pattern}},
		}},
	}
}

// TestDuplexTurnAssertions_SurviveTheSaveStageRewrite reproduces #446 item 1:
// a failing duplex turn assertion was written into the state store, then lost
// when the duplex save stage rewrote the transcript, so the run reported
// Failed: 0. The outcome must reach the final result's messages.
func TestDuplexTurnAssertions_SurviveTheSaveStageRewrite(t *testing.T) {
	ctx := context.Background()
	store := statestore.NewArenaStateStore()
	convID := "conv-duplex-turns"
	save := func(msgs ...types.Message) {
		// The duplex save stage writes the transcript it collected off the
		// stream, which never carries assertion metadata.
		require.NoError(t, store.Save(ctx, &runtimestore.ConversationState{ID: convID, Messages: msgs}))
	}

	de := &DuplexConversationExecutor{
		evalOrchestrator: NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, ""),
	}
	req := &ConversationRequest{
		ConversationID:       convID,
		Scenario:             &arenaconfig.Scenario{ID: "basic-duplex"},
		StateStoreConfig:     &StateStoreConfig{Store: store},
		duplexTurnAssertions: &duplexTurnAssertionRecorder{},
	}

	user1 := types.Message{Role: "user", Content: "[audio]"}
	reply1 := types.Message{Role: roleAssistant, Content: "Hello, I'm Nova."}
	user2 := types.Message{Role: "user", Content: "[audio]"}
	reply2 := types.Message{Role: roleAssistant, Content: "My name is Nova."}

	save(user1, reply1)
	de.evaluateTurnAssertions(ctx, req, includesTurn("ZZZNOMATCH"), 0)
	save(user1, reply1, user2, reply2)
	de.evaluateTurnAssertions(ctx, req, includesTurn("Nova"), 1)
	save(user1, reply1, user2, reply2) // final save

	result := de.buildResultFromStateStore(ctx, req)
	require.Len(t, result.Messages, 4)
	// A failing turn assertion fails the run, as it does on the text path.
	assert.True(t, result.Failed)
	assert.Equal(t, "turn assertions failed: 1 of 2", result.Error)

	first, ok := arenaassertions.TurnSummaryFromMeta(result.Messages[1].Meta)
	require.True(t, ok, "first reply must carry its turn assertions")
	assert.Equal(t, arenaassertions.TurnSummary{Total: 1, Failed: 1, Passed: false}, first)

	second, ok := arenaassertions.TurnSummaryFromMeta(result.Messages[3].Meta)
	require.True(t, ok, "second reply must carry its turn assertions")
	assert.True(t, second.Passed)

	total, failed := arenaassertions.CountTurnAssertions(result.Messages)
	assert.Equal(t, 2, total)
	assert.Equal(t, 1, failed)

	// The run result is read back from the store, so the store must agree.
	stored, err := store.Load(ctx, convID)
	require.NoError(t, err)
	total, failed = arenaassertions.CountTurnAssertions(stored.Messages)
	assert.Equal(t, 2, total)
	assert.Equal(t, 1, failed)
}

// failingSaveStore is an ArenaStateStore whose Save always fails.
type failingSaveStore struct {
	*statestore.ArenaStateStore
}

func (failingSaveStore) Save(context.Context, *runtimestore.ConversationState) error {
	return errors.New("disk full")
}

// TestDuplexTurnAssertions_SaveFailureFailsTheRun pins that a turn outcome the
// store would not take fails the run, rather than reading as a pass.
func TestDuplexTurnAssertions_SaveFailureFailsTheRun(t *testing.T) {
	ctx := context.Background()
	inner := statestore.NewArenaStateStore()
	require.NoError(t, inner.Save(ctx, &runtimestore.ConversationState{
		ID:       "conv",
		Messages: []types.Message{{Role: "user"}, {Role: roleAssistant, Content: "hi"}},
	}))
	recorder := &duplexTurnAssertionRecorder{}
	recorder.record(0, map[string]interface{}{"total": 1, "failed": 1, "all_passed": false})
	req := &ConversationRequest{
		ConversationID:       "conv",
		Scenario:             &arenaconfig.Scenario{ID: "s"},
		StateStoreConfig:     &StateStoreConfig{Store: failingSaveStore{inner}},
		duplexTurnAssertions: recorder,
	}

	result := (&DuplexConversationExecutor{}).buildResultFromStateStore(ctx, req)
	assert.True(t, result.Failed)
	assert.Contains(t, result.Error, "disk full")
}

func TestDuplexTurnAssertions_AllPassingLeavesTheRunClean(t *testing.T) {
	ctx := context.Background()
	store := statestore.NewArenaStateStore()
	require.NoError(t, store.Save(ctx, &runtimestore.ConversationState{
		ID:       "conv-pass",
		Messages: []types.Message{{Role: "user"}, {Role: roleAssistant, Content: "hi"}},
	}))
	recorder := &duplexTurnAssertionRecorder{}
	recorder.record(0, duplexAssertionSummary([]arenaassertions.AssertionResult{{Passed: true}}))
	req := &ConversationRequest{
		ConversationID:       "conv-pass",
		Scenario:             &arenaconfig.Scenario{ID: "s"},
		StateStoreConfig:     &StateStoreConfig{Store: store},
		duplexTurnAssertions: recorder,
	}

	result := (&DuplexConversationExecutor{}).buildResultFromStateStore(ctx, req)
	assert.False(t, result.Failed)
	assert.Empty(t, result.Error)
	turn, ok := arenaassertions.TurnSummaryFromMeta(result.Messages[1].Meta)
	require.True(t, ok)
	assert.True(t, turn.Passed)
}

func TestDuplexTurnAssertionRecorder(t *testing.T) {
	t.Run("nil recorder is a no-op", func(t *testing.T) {
		var r *duplexTurnAssertionRecorder
		r.record(0, map[string]interface{}{"total": 1})
		msgs := []types.Message{{Role: roleAssistant}}
		assert.False(t, r.apply(msgs))
		assert.Nil(t, msgs[0].Meta)
	})

	t.Run("negative ordinal is ignored", func(t *testing.T) {
		r := &duplexTurnAssertionRecorder{}
		r.record(-1, map[string]interface{}{"total": 1})
		assert.Empty(t, r.byReply)
	})

	t.Run("empty recorder leaves messages alone", func(t *testing.T) {
		r := &duplexTurnAssertionRecorder{}
		msgs := []types.Message{{Role: roleAssistant}}
		assert.False(t, r.apply(msgs))
		assert.Nil(t, msgs[0].Meta)
	})

	t.Run("apply copies Meta rather than mutating a shared map", func(t *testing.T) {
		shared := map[string]interface{}{"finish_reason": "stop"}
		msgs := []types.Message{
			{Role: "system"},
			{Role: roleAssistant, Meta: shared},
			{Role: roleAssistant},
		}
		r := &duplexTurnAssertionRecorder{}
		summary := map[string]interface{}{"total": 1, "failed": 0, "all_passed": true}
		r.record(0, summary)
		r.apply(msgs)

		assert.NotContains(t, shared, "assertions")
		assert.Equal(t, summary, msgs[1].Meta["assertions"])
		assert.Nil(t, msgs[2].Meta)
	})

	t.Run("apply keeps the reply's existing metadata", func(t *testing.T) {
		msgs := []types.Message{{Role: roleAssistant, Meta: map[string]interface{}{"finish_reason": "stop"}}}
		r := &duplexTurnAssertionRecorder{}
		r.record(0, map[string]interface{}{"total": 1})
		r.apply(msgs)
		assert.Equal(t, "stop", msgs[0].Meta["finish_reason"])
		assert.NotNil(t, msgs[0].Meta["assertions"])
	})
}

func TestLastAssistantOrdinal(t *testing.T) {
	assert.Equal(t, -1, lastAssistantOrdinal(nil))
	assert.Equal(t, -1, lastAssistantOrdinal([]types.Message{{Role: "user"}}))
	assert.Equal(t, 1, lastAssistantOrdinal([]types.Message{
		{Role: "system"}, {Role: roleAssistant}, {Role: "user"}, {Role: roleAssistant}, {Role: "user"},
	}))
}
