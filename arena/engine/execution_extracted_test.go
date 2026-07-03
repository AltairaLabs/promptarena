package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

func TestPerturbedScenario(t *testing.T) {
	base := &arenaconfig.Scenario{
		Turns: []arenaconfig.TurnDefinition{
			{Role: "user", Content: "Go to {city}", Perturbations: map[string][]string{
				"city": {"NYC", "LA"},
			}},
		},
	}
	e := &Engine{}

	t.Run("negative index returns original", func(t *testing.T) {
		got := e.perturbedScenario(base, -1)
		assert.Same(t, base, got)
	})

	t.Run("out-of-range index returns original", func(t *testing.T) {
		got := e.perturbedScenario(base, 99)
		assert.Same(t, base, got)
	})

	t.Run("valid index returns substituted copy", func(t *testing.T) {
		got := e.perturbedScenario(base, 0)
		require.NotSame(t, base, got)
		require.Len(t, got.Turns, 1)
		// One of the two variants must have been substituted in.
		assert.Contains(t, []string{"Go to NYC", "Go to LA"}, got.Turns[0].Content)
		// Original scenario must remain untouched.
		assert.Equal(t, "Go to {city}", base.Turns[0].Content)
	})

	t.Run("scenario without perturbations returns original", func(t *testing.T) {
		plain := &arenaconfig.Scenario{
			Turns: []arenaconfig.TurnDefinition{{Role: "user", Content: "hi"}},
		}
		got := e.perturbedScenario(plain, 0)
		assert.Same(t, plain, got)
	})
}

func TestResolveRunOrchestrator(t *testing.T) {
	t.Run("workflow orchestrator takes precedence", func(t *testing.T) {
		wf := &EvalOrchestrator{taskType: "workflow"}
		e := &Engine{evalOrchestrator: &EvalOrchestrator{taskType: "shared"}}
		got := e.resolveRunOrchestrator(wf)
		assert.Same(t, wf, got)
	})

	t.Run("nil workflow with no engine orchestrator returns nil", func(t *testing.T) {
		e := &Engine{}
		assert.Nil(t, e.resolveRunOrchestrator(nil))
	})

	t.Run("nil workflow clones engine orchestrator", func(t *testing.T) {
		shared := &EvalOrchestrator{taskType: "shared"}
		e := &Engine{evalOrchestrator: shared}
		got := e.resolveRunOrchestrator(nil)
		require.NotNil(t, got)
		assert.NotSame(t, shared, got)
		assert.Equal(t, "shared", got.taskType)
	})
}

func TestStampTimeoutError(t *testing.T) {
	e := &Engine{}
	timeout := 42 * time.Second

	t.Run("no timeout is a no-op", func(t *testing.T) {
		res := &ConversationResult{}
		bail, msg := e.stampTimeoutError(context.Background(), res, timeout)
		assert.False(t, bail)
		assert.Empty(t, msg)
		assert.False(t, res.Failed)
	})

	t.Run("timeout with nil result bails with synthetic error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		bail, msg := e.stampTimeoutError(ctx, nil, timeout)
		assert.True(t, bail)
		assert.Contains(t, msg, "run timed out after 42s")
	})

	t.Run("timeout with no partial messages appends existing error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res := &ConversationResult{Error: "boom"}
		bail, msg := e.stampTimeoutError(ctx, res, timeout)
		assert.True(t, bail)
		assert.True(t, strings.HasPrefix(msg, "run timed out after 42s"))
		assert.Contains(t, msg, "boom")
	})

	t.Run("timeout with partial messages stamps result and keeps it", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res := &ConversationResult{Messages: []types.Message{{Role: "user"}}}
		bail, msg := e.stampTimeoutError(ctx, res, timeout)
		assert.False(t, bail)
		assert.Empty(t, msg)
		assert.True(t, res.Failed)
		assert.Contains(t, res.Error, "run timed out after 42s")
	})

	t.Run("timeout with partial messages preserves prior error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res := &ConversationResult{
			Messages: []types.Message{{Role: "user"}},
			Error:    "prior failure",
		}
		bail, _ := e.stampTimeoutError(ctx, res, timeout)
		assert.False(t, bail)
		assert.True(t, res.Failed)
		assert.Contains(t, res.Error, "run timed out after 42s")
		assert.Contains(t, res.Error, "prior failure")
	})
}

func TestBuildConversationRequest(t *testing.T) {
	e := &Engine{}
	scenario := &arenaconfig.Scenario{ID: "s1"}
	combo := RunCombination{Region: "us", ProviderID: "openai", ScenarioID: "s1"}
	start := time.Now()

	req := e.buildConversationRequest(combo, scenario, nil, nil, nil, "run-1", start)

	assert.Same(t, scenario, req.Scenario)
	assert.Equal(t, "us", req.Region)
	assert.Equal(t, "run-1", req.RunID)
	assert.Equal(t, "run-1", req.ConversationID)
	require.NotNil(t, req.StateStoreConfig)
	assert.Equal(t, "openai", req.StateStoreConfig.Metadata["provider"])
	assert.Equal(t, "s1", req.StateStoreConfig.Metadata["scenario"])
	assert.Equal(t, "us", req.StateStoreConfig.Metadata["region"])
}
