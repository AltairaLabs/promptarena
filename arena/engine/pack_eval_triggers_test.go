package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"
)

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
