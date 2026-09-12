package engine

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/runtime/v2/memory"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

func TestSeedMemoriesForRun(t *testing.T) {
	store := memory.NewInMemoryStore()

	eng := &Engine{
		memoryStore: store,
	}

	scenario := &arenaconfig.Scenario{
		ID: "test-scenario",
		SeedMemories: []arenaconfig.SeedMemoryEntry{
			{
				Content: "Customer prefers dark mode.",
			},
			{
				Content:    "Order #1023 was delayed 5 days.",
				Type:       "episodic",
				Confidence: 0.95,
				Metadata:   map[string]any{"tags": []string{"order-1023"}},
			},
		},
	}

	scope := map[string]string{"scenario": "test-scenario", "run": "run-1"}
	err := eng.seedMemoriesForRun(scenario, scope)
	require.NoError(t, err)

	// Verify memories were saved with correct scope
	memories, err := store.List(t.Context(), scope, memory.ListOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, memories, 2)

	// First memory — defaults applied
	assert.Equal(t, "Customer prefers dark mode.", memories[0].Content)
	assert.Equal(t, "general", memories[0].Type)
	assert.Equal(t, 0.8, memories[0].Confidence)
	assert.Equal(t, scope, memories[0].Scope)

	// Second memory — explicit values preserved
	assert.Equal(t, "Order #1023 was delayed 5 days.", memories[1].Content)
	assert.Equal(t, "episodic", memories[1].Type)
	assert.Equal(t, 0.95, memories[1].Confidence)
	assert.Contains(t, memories[1].Metadata, "tags")
}

func TestSeedMemoriesForRun_NoMemoryStore(t *testing.T) {
	eng := &Engine{memoryStore: nil}
	scenario := &arenaconfig.Scenario{
		SeedMemories: []arenaconfig.SeedMemoryEntry{{Content: "test"}},
	}
	// Should be a no-op, not an error
	err := eng.seedMemoriesForRun(scenario, nil)
	assert.NoError(t, err)
}

func TestSeedMemoriesForRun_EmptySeeds(t *testing.T) {
	store := memory.NewInMemoryStore()
	eng := &Engine{memoryStore: store}
	scenario := &arenaconfig.Scenario{}

	err := eng.seedMemoriesForRun(scenario, map[string]string{"run": "1"})
	assert.NoError(t, err)

	memories, _ := store.List(t.Context(), map[string]string{"run": "1"}, memory.ListOptions{Limit: 10})
	assert.Empty(t, memories)
}

func TestSeedMemoriesForRun_EmptyContent(t *testing.T) {
	store := memory.NewInMemoryStore()
	eng := &Engine{memoryStore: store}
	scenario := &arenaconfig.Scenario{
		SeedMemories: []arenaconfig.SeedMemoryEntry{{Content: ""}},
	}

	err := eng.seedMemoriesForRun(scenario, map[string]string{"run": "1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty content")
}

// TestInitMemory_RegistersToolsAndStore covers initMemory, which was 22.2%.
// It is the only thing that puts the four memory__* tools in front of the
// model, so a silent no-op here produces a scenario where the assistant
// simply never recalls anything and nothing reports why.
func TestInitMemory_RegistersToolsAndStore(t *testing.T) {
	eng := &Engine{
		config:       &arenaconfig.Config{Memory: map[string]any{"enabled": true}},
		toolRegistry: tools.NewRegistry(),
	}

	require.NoError(t, eng.initMemory())
	require.NotNil(t, eng.memoryStore, "a configured memory section must leave a store behind")

	for _, name := range []string{
		memory.RecallToolName, memory.RememberToolName,
		memory.ListToolName, memory.ForgetToolName,
	} {
		assert.NotNil(t, eng.toolRegistry.Get(name), "tool %s must be registered", name)
	}
}

// TestInitMemory_NoConfigLeavesEverythingAlone pins the opposite branch: with
// no memory section the tools must NOT appear, or every scenario would offer
// the model memory it was never asked to have.
func TestInitMemory_NoConfigLeavesEverythingAlone(t *testing.T) {
	eng := &Engine{
		config:       &arenaconfig.Config{},
		toolRegistry: tools.NewRegistry(),
	}

	require.NoError(t, eng.initMemory())
	assert.Nil(t, eng.memoryStore)
	assert.Nil(t, eng.toolRegistry.Get(memory.RecallToolName))
}

// TestRegisterMemoryForRun_ScopesToScenarioAndRun covers registerMemoryForRun,
// which was 0%. The scope it builds is the isolation boundary between runs —
// get it wrong and one scenario recalls another's memories, which shows up as
// a passing assertion rather than an error.
func TestRegisterMemoryForRun_ScopesToScenarioAndRun(t *testing.T) {
	eng := &Engine{
		config:       &arenaconfig.Config{Memory: map[string]any{"enabled": true}},
		toolRegistry: tools.NewRegistry(),
	}
	require.NoError(t, eng.initMemory())

	scope := eng.registerMemoryForRun("scenario-a", "run-1")
	require.Equal(t, map[string]string{"scenario": "scenario-a", "run": "run-1"}, scope)

	// The executor registered for this run must write under that scope.
	ctx := t.Context()
	desc := eng.toolRegistry.Get(memory.RememberToolName)
	require.NotNil(t, desc)
	_, err := eng.toolRegistry.Execute(ctx, memory.RememberToolName,
		json.RawMessage(`{"content":"scoped to run-1"}`))
	require.NoError(t, err)

	got, err := eng.memoryStore.List(ctx, scope, memory.ListOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "scoped to run-1", got[0].Content)

	// A different run must not see it.
	other := eng.registerMemoryForRun("scenario-a", "run-2")
	got, err = eng.memoryStore.List(ctx, other, memory.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, got, "run-2 must not see run-1's memories")
}

// TestRegisterMemoryForRun_NoStoreReturnsNil pins the guard: without initMemory
// there is no store, and the caller relies on a nil scope to mean "memory off".
func TestRegisterMemoryForRun_NoStoreReturnsNil(t *testing.T) {
	eng := &Engine{toolRegistry: tools.NewRegistry()}
	assert.Nil(t, eng.registerMemoryForRun("s", "r"))
}
