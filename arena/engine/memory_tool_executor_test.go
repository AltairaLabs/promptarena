package engine

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/memory"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

// newMemoryTestEngine builds the minimum Engine needed to exercise memory tool
// dispatch: a store, a tool registry with the memory tools registered, and the
// shared memory executor wired in exactly as initMemory does.
func newMemoryTestEngine(t *testing.T) (*Engine, *memory.InMemoryStore) {
	t.Helper()
	store := memory.NewInMemoryStore()
	eng := &Engine{
		memoryStore:  store,
		toolRegistry: tools.NewRegistry(),
	}
	eng.memoryToolExec = newMemoryToolExecutor(store)
	eng.toolRegistry.RegisterExecutor(eng.memoryToolExec)
	memory.RegisterMemoryTools(eng.toolRegistry)
	return eng, store
}

func TestMemoryToolExecutor_WritesToTheCallingRunsScope(t *testing.T) {
	eng, store := newMemoryTestEngine(t)

	scopeA := eng.registerMemoryForRun("scenario-a", "run-a")
	// A second, later run must not take ownership of the memory tool.
	eng.registerMemoryForRun("scenario-b", "run-b")

	args, err := json.Marshal(map[string]any{"content": "A's secret"})
	require.NoError(t, err)

	ctx := withRunID(t.Context(), "run-a")
	_, err = eng.toolRegistry.Execute(ctx, memory.RememberToolName, args)
	require.NoError(t, err)

	inA, err := store.List(t.Context(), scopeA, memory.ListOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, inA, 1, "run A's write must land in run A's scope")
	assert.Equal(t, "A's secret", inA[0].Content)
}

func TestMemoryToolExecutor_DoesNotLeakIntoAnotherRunsScope(t *testing.T) {
	eng, store := newMemoryTestEngine(t)

	eng.registerMemoryForRun("scenario-a", "run-a")
	scopeB := eng.registerMemoryForRun("scenario-b", "run-b")

	args, err := json.Marshal(map[string]any{"content": "A's secret"})
	require.NoError(t, err)

	ctx := withRunID(t.Context(), "run-a")
	_, err = eng.toolRegistry.Execute(ctx, memory.RememberToolName, args)
	require.NoError(t, err)

	inB, err := store.List(t.Context(), scopeB, memory.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, inB, "run A's write must not be visible in run B's scope")
}

func TestMemoryToolExecutor_UnknownRunIsAnError(t *testing.T) {
	eng, _ := newMemoryTestEngine(t)

	args, err := json.Marshal(map[string]any{"content": "orphan"})
	require.NoError(t, err)

	res, err := eng.toolRegistry.Execute(withRunID(t.Context(), "nobody"), memory.RememberToolName, args)
	require.NoError(t, err)
	assert.Contains(t, res.Error, `no registered scope for run "nobody"`,
		"an unscoped memory write must fail, never fall back to a shared scope")
}

func TestMemoryToolExecutor_NoRunIdentityIsAnError(t *testing.T) {
	eng, _ := newMemoryTestEngine(t)
	eng.registerMemoryForRun("scenario-a", "run-a")

	args, err := json.Marshal(map[string]any{"content": "orphan"})
	require.NoError(t, err)

	res, err := eng.toolRegistry.Execute(t.Context(), memory.RememberToolName, args)
	require.NoError(t, err)
	assert.Contains(t, res.Error, "no run identity on context")
}

func TestMemoryToolExecutor_UnregisterRunDropsTheScope(t *testing.T) {
	eng, _ := newMemoryTestEngine(t)
	eng.registerMemoryForRun("scenario-a", "run-a")
	eng.memoryToolExec.unregisterRun("run-a")

	args, err := json.Marshal(map[string]any{"content": "after teardown"})
	require.NoError(t, err)

	res, err := eng.toolRegistry.Execute(withRunID(t.Context(), "run-a"), memory.RememberToolName, args)
	require.NoError(t, err)
	assert.Contains(t, res.Error, `no registered scope for run "run-a"`)
}
