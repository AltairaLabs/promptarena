package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/turnexecutors"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/memory"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

// TestConcurrentRuns_MemoryIsIsolatedPerRun is the end-to-end guard for the
// bug this file exists for: arena fans out runs as goroutines over one engine
// and one tool registry, so anything holding per-run state in that registry
// leaks between runs. Memory did — registerMemoryForRun used to register a
// per-run executor into the registry's single "memory" slot, so the last run
// to start owned the memory tool for every run in flight.
//
// The two runs are held at a barrier so they genuinely overlap; a sequential
// version of this test passes even against the broken wiring.
func TestConcurrentRuns_MemoryIsIsolatedPerRun(t *testing.T) {
	const runCount = 2

	cfg := &arenaconfig.Config{
		Memory: map[string]any{"enabled": true},
		StateStore: &config.StateStoreConfig{
			Type: "memory",
		},
		LoadedScenarios: map[string]*arenaconfig.Scenario{
			"scenario-a": scenarioNamed("scenario-a"),
			"scenario-b": scenarioNamed("scenario-b"),
		},
		LoadedProviders: map[string]*config.Provider{
			"test-provider": {ID: "test-provider", Type: "test", Model: "test-model"},
		},
	}

	providerRegistry := providers.NewRegistry()
	providerRegistry.Register(&testProvider{id: "test-provider"})

	// Both runs write, then wait for the other to have written, then read.
	// Without the barrier the first run could finish before the second
	// registered, which is exactly the interleaving that hides the bug.
	var wrote sync.WaitGroup
	wrote.Add(runCount)

	var mu sync.Mutex
	seen := map[string][]string{} // scenarioID -> memory contents it could read

	var eng *Engine
	mockExecutor := &mockTurnExecutor{
		executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
			secret := fmt.Sprintf("secret for %s", req.Scenario.ID)

			args, err := json.Marshal(map[string]any{"content": secret})
			if err != nil {
				return err
			}
			res, err := req.ToolRegistry.Execute(ctx, memory.RememberToolName, args)
			if err != nil {
				return err
			}
			if res.Error != "" {
				return fmt.Errorf("remember failed: %s", res.Error)
			}

			wrote.Done()
			wrote.Wait()

			listArgs, err := json.Marshal(map[string]any{"limit": 10})
			if err != nil {
				return err
			}
			listRes, err := req.ToolRegistry.Execute(ctx, memory.ListToolName, listArgs)
			if err != nil {
				return err
			}
			if listRes.Error != "" {
				return fmt.Errorf("list failed: %s", listRes.Error)
			}

			mu.Lock()
			seen[req.Scenario.ID] = contentsOf(listRes.Result)
			mu.Unlock()
			return nil
		},
	}

	conversationExecutor := &DefaultConversationExecutor{scriptedExecutor: mockExecutor}

	var err error
	eng, err = NewEngine(cfg, providerRegistry, nil, nil, conversationExecutor, nil, tools.NewRegistry())
	require.NoError(t, err)

	// NewEngineFromConfig runs this as part of construction; NewEngine does not,
	// so call the same initializer the production path does.
	require.NoError(t, eng.initMemory())
	require.NotNil(t, eng.memoryStore, "initMemory must build the store")

	plan := &RunPlan{
		Combinations: []RunCombination{
			{Region: "default", ScenarioID: "scenario-a", ProviderID: "test-provider"},
			{Region: "default", ScenarioID: "scenario-b", ProviderID: "test-provider"},
		},
	}

	_, err = eng.ExecuteRuns(context.Background(), plan, runCount)
	require.NoError(t, err)

	require.Len(t, seen, runCount, "both runs must have completed")
	for _, scenarioID := range []string{"scenario-a", "scenario-b"} {
		assert.Equal(t, []string{"secret for " + scenarioID}, seen[scenarioID],
			"%s must see its own memory and nothing from the other run", scenarioID)
	}
}

func scenarioNamed(id string) *arenaconfig.Scenario {
	return &arenaconfig.Scenario{
		ID:       id,
		TaskType: "assistance",
		Turns:    []arenaconfig.TurnDefinition{{Role: "user", Content: "Hello"}},
	}
}

// contentsOf pulls the memory contents out of a memory__list result, which is
// an object with a "memories" array.
func contentsOf(raw json.RawMessage) []string {
	var payload struct {
		Memories []struct {
			Content string `json:"content"`
		} `json:"memories"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return []string{fmt.Sprintf("unparseable: %v", err)}
	}
	out := make([]string, 0, len(payload.Memories))
	for _, m := range payload.Memories {
		out = append(out, m.Content)
	}
	return out
}
