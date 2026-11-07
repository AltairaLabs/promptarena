package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	runtimestore "github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"
)

// The duplex path runs every pack eval trigger too, not just the session ones.
func TestDuplexResult_RunsEveryPackEvalTrigger(t *testing.T) {
	params := map[string]any{"patterns": []any{"hi"}}
	defs := []evals.EvalDef{
		{ID: "turn_check", Type: "contains", Trigger: "every_turn", Params: params},
		{ID: "session_check", Type: "contains", Trigger: "on_session_complete", Params: params},
		{ID: "conversation_check", Type: "contains", Trigger: "on_conversation_complete", Params: params},
	}
	orch := NewEvalOrchestrator(evals.NewEvalTypeRegistry(), defs, false, nil, "")
	de := NewDuplexConversationExecutor(nil, nil, nil, nil, orch)

	store := statestore.NewArenaStateStore()
	require.NoError(t, store.Save(context.Background(), &runtimestore.ConversationState{
		ID: "conv",
		Messages: []types.Message{
			{Role: "user", Content: "hello"}, {Role: "assistant", Content: "hi one"},
			{Role: "user", Content: "again"}, {Role: "assistant", Content: "hi two"},
		},
	}))
	req := &ConversationRequest{
		ConversationID:   "conv",
		Scenario:         &arenaconfig.Scenario{ID: "duplex"},
		StateStoreConfig: &StateStoreConfig{Store: store},
	}

	byID := map[string]int{}
	for _, r := range de.buildResultFromStateStore(context.Background(), req).EvalResults {
		byID[r.EvalID]++
	}
	assert.Equal(t, map[string]int{"turn_check": 2, "session_check": 1, "conversation_check": 1}, byID)
}

// Every pack eval trigger fires in a normal run through the real engine: the
// turn triggers once per assistant turn, the session and conversation triggers
// once at the end (#432 item 2).
func TestPackEvals_EveryTriggerFiresThroughEngine(t *testing.T) {
	eng, err := NewEngineFromConfigFile(filepath.Join("testdata", "pack_eval_triggers", "config.arena.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close() })

	plan := &RunPlan{Combinations: []RunCombination{
		{Region: "default", ScenarioID: "two-turns", ProviderID: "mock"},
	}}
	runIDs, err := eng.ExecuteRuns(context.Background(), plan, 1)
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	store, ok := eng.GetStateStore().(*statestore.ArenaStateStore)
	require.True(t, ok)
	result, err := store.GetResult(context.Background(), runIDs[0])
	require.NoError(t, err)
	require.Empty(t, result.Error)

	byID := map[string][]int{}
	for _, r := range result.EvalResults {
		assert.Empty(t, r.Error, "eval %s errored", r.EvalID)
		byID[r.EvalID] = append(byID[r.EvalID], r.TurnIndex)
	}
	assert.Len(t, byID["turn_check"], 2, "every_turn must fire once per assistant turn: %v", byID)
	assert.Len(t, byID["sampled_turn_check"], 2, "sample_turns at 100%% must fire once per turn: %v", byID)
	assert.Len(t, byID["session_check"], 1, "on_session_complete must fire once: %v", byID)
	assert.Len(t, byID["conversation_check"], 1, "on_conversation_complete must fire once: %v", byID)
	if turns := byID["turn_check"]; len(turns) == 2 {
		assert.Less(t, turns[0], turns[1], "turn evals must carry distinct, ordered turn indexes")
	}
}

// A tool loop's intermediate assistant message does not close a turn; the
// reply before the next user message, and a trailing reply, do.
func TestTurnEndIndexes(t *testing.T) {
	msgs := []types.Message{
		{Role: "system"},
		{Role: "user"},
		{Role: "assistant"}, // tool call
		{Role: "tool"},
		{Role: "assistant"}, // closes turn 1
		{Role: "user"},
		{Role: "assistant"}, // closes turn 2
	}
	assert.Equal(t, []int{4, 6}, turnEndIndexes(msgs))
	assert.Nil(t, turnEndIndexes(nil))
	assert.Nil(t, turnEndIndexes([]types.Message{{Role: "user"}}))
}

// RunPackEvals is a no-op on a nil orchestrator or one with evals skipped.
func TestRunPackEvals_NoRunner(t *testing.T) {
	var nilOrch *EvalOrchestrator
	assert.Nil(t, nilOrch.RunPackEvals(context.Background(), nil, "s"))

	defs := []evals.EvalDef{{ID: "x", Type: "contains", Trigger: "every_turn"}}
	skipped := NewEvalOrchestrator(evals.NewEvalTypeRegistry(), defs, true, nil, "")
	assert.Nil(t, skipped.RunPackEvals(context.Background(), []types.Message{{Role: "assistant"}}, "s"))
}
