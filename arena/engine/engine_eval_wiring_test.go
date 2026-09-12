package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
)

// assertedScenarioConfig returns a minimal config with one mock provider and
// one scenario that carries a turn assertion, so a run exercises the eval
// orchestrator end to end.
func assertedScenarioConfig() *arenaconfig.Config {
	return &arenaconfig.Config{
		Defaults: arenaconfig.Defaults{
			Temperature: 0.1,
			MaxTokens:   64,
		},
		LoadedProviders: map[string]*config.Provider{
			"mock-assistant": {
				ID:    "mock-assistant",
				Type:  "mock",
				Model: "mock-model",
				Defaults: config.ProviderDefaults{
					Temperature: 0.1,
					MaxTokens:   64,
					TopP:        1.0,
				},
			},
		},
		LoadedScenarios: map[string]*arenaconfig.Scenario{
			"asserted": {
				ID:       "asserted",
				TaskType: "assistance",
				Turns: []arenaconfig.TurnDefinition{
					{
						Role:    "user",
						Content: "Say something",
						Assertions: []arenaconfig.AssertionConfig{
							{Type: "contains", Params: map[string]interface{}{"patterns": []string{"Mock"}}},
						},
					},
				},
			},
		},
	}
}

// buildDIEngine builds the engine the way an embedding caller does:
// BuildEngineComponents followed by NewEngine (#190).
func buildDIEngine(t *testing.T, cfg *arenaconfig.Config) (*Engine, ConversationExecutor) {
	t.Helper()
	pr, pm, mcpReg, conv, ad, cleanup, toolReg, _, err := BuildEngineComponents(cfg, nil)
	require.NoError(t, err)
	if cleanup != nil {
		t.Cleanup(cleanup)
	}
	eng, err := NewEngine(cfg, pr, pm, mcpReg, conv, ad, toolReg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close() })
	return eng, conv
}

// TestNewEngine_WiresEvalOrchestrator pins the root cause of #190: the DI
// constructor must adopt the orchestrator that BuildEngineComponents already
// injected into the conversation executor, exactly as NewEngineFromConfig does.
func TestNewEngine_WiresEvalOrchestrator(t *testing.T) {
	eng, conv := buildDIEngine(t, assertedScenarioConfig())

	require.NotNil(t, eng.evalOrchestrator, "NewEngine left evalOrchestrator nil")
	require.Same(t, evalOrchestratorFrom(conv), eng.evalOrchestrator,
		"engine must share the executor's orchestrator so SetEventBus reaches it")
}

// TestNewEngine_DIBuiltEnginePublishesEvalEvents is the behavioural guard from
// #190: an engine built with BuildEngineComponents + NewEngine, with a bus
// attached, must deliver eval.completed for an asserted scenario.
func TestNewEngine_DIBuiltEnginePublishesEvalEvents(t *testing.T) {
	eng, _ := buildDIEngine(t, assertedScenarioConfig())

	bus := events.NewEventBus()
	t.Cleanup(func() { bus.Close() })

	var mu sync.Mutex
	var got []*events.Event
	bus.Subscribe(events.EventEvalCompleted, func(e *events.Event) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, e)
	})
	eng.SetEventBus(bus)

	plan := &RunPlan{Combinations: []RunCombination{
		{Region: "default", ScenarioID: "asserted", ProviderID: "mock-assistant"},
	}}
	runIDs, err := eng.ExecuteRuns(context.Background(), plan, 1)
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) > 0
	}, 2*time.Second, 10*time.Millisecond,
		"no eval.completed reached the bus from a DI-built engine")
}
