package engine

import (
	"context"
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

func TestCountFailedAssertions(t *testing.T) {
	results := []arenaassertions.AssertionResult{
		{Passed: true},
		{Passed: false},
		{Passed: true},
	}
	assert.Equal(t, 1, countFailedAssertions(results))
	assert.Equal(t, 0, countFailedAssertions(nil))
}

func TestDuplexAssertionSummary(t *testing.T) {
	summary := duplexAssertionSummary([]arenaassertions.AssertionResult{
		{Passed: true, Message: "ok"},
		{Passed: false},
	})
	assert.Equal(t, 2, summary["total"])
	assert.Equal(t, 1, summary["failed"])
	assert.Equal(t, false, summary["all_passed"])
	list, ok := summary["results"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, list, 2)
	assert.Equal(t, "ok", list[0]["message"])
	_, hasMessage := list[1]["message"]
	assert.False(t, hasMessage)

	// The run summary and RunPassed read it through TurnSummaryFromMeta.
	turn, ok := arenaassertions.TurnSummaryFromMeta(map[string]interface{}{"assertions": summary})
	require.True(t, ok)
	assert.Equal(t, arenaassertions.TurnSummary{Total: 2, Failed: 1, Passed: false}, turn)
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

// TestEvaluateTurnAssertions_GuardsBeforeEvaluating covers the guards of
// evaluateTurnAssertions. Every guard returns silently, so getting one wrong
// does not raise — it records a summary for a reply that was never graded, or
// grades the wrong one. Each case below must record nothing; the one case that
// passes every guard must record the reply's result, which proves the recorder
// is being watched.
func TestEvaluateTurnAssertions_GuardsBeforeEvaluating(t *testing.T) {
	ctx := context.Background()
	includesHello := []arenaconfig.AssertionConfig{{
		Type:   "content_includes",
		Params: map[string]interface{}{"patterns": []interface{}{"hello"}},
	}}
	userOnly := []types.Message{{Role: "user", Content: "hi"}}
	withReply := []types.Message{{Role: "user", Content: "hi"}, {Role: roleAssistant, Content: "hello there"}}
	realOrchestrator := NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")

	tests := []struct {
		name         string
		assertions   []arenaconfig.AssertionConfig
		history      []types.Message // nil: no state store at all
		orchestrator *EvalOrchestrator
		wantRecorded bool
	}{
		{name: "a turn with no assertions records nothing",
			history: withReply, orchestrator: realOrchestrator},
		{name: "no conversation history records nothing",
			assertions: includesHello, orchestrator: realOrchestrator},
		{name: "history with no assistant message records nothing",
			assertions: includesHello, history: userOnly, orchestrator: realOrchestrator},
		{name: "a nil orchestrator records nothing",
			assertions: includesHello, history: withReply},
		{name: "past every guard the reply's result is recorded",
			assertions: includesHello, history: withReply, orchestrator: realOrchestrator, wantRecorded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &duplexTurnAssertionRecorder{}
			req := &ConversationRequest{ConversationID: "conv-guards", duplexTurnAssertions: recorder}
			if tt.history != nil {
				store := statestore.NewArenaStateStore()
				require.NoError(t, store.Save(ctx, &runtimestore.ConversationState{
					ID: req.ConversationID, Messages: tt.history,
				}))
				req.StateStoreConfig = &StateStoreConfig{Store: store}
			}
			de := &DuplexConversationExecutor{evalOrchestrator: tt.orchestrator}

			de.evaluateTurnAssertions(ctx, req, &arenaconfig.TurnDefinition{Assertions: tt.assertions}, 0)

			if !tt.wantRecorded {
				assert.Empty(t, recorder.byReply)
				return
			}
			require.Contains(t, recorder.byReply, 0, "the first reply's summary must be recorded")
			turn, ok := arenaassertions.TurnSummaryFromMeta(map[string]interface{}{"assertions": recorder.byReply[0]})
			require.True(t, ok)
			assert.Equal(t, arenaassertions.TurnSummary{Total: 1, Failed: 0, Passed: true}, turn)
		})
	}
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
