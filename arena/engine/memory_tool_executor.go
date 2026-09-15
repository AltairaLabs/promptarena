package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/v2/memory"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

// memoryToolExecutor is the single memory executor registered in the
// engine-wide tool registry. The registry keys executors by name, so
// registering a per-run executor — as arena used to — means the last run to
// start owns the memory tool for every run in flight, and writes land in the
// wrong run's scope.
//
// Instead this object is registered once at init and holds one scope per run.
// Execute resolves the caller's run from the context and delegates to a
// PromptKit memory executor bound to that run's scope.
type memoryToolExecutor struct {
	store memory.Store

	mu     sync.RWMutex
	scopes map[string]map[string]string // runID -> memory scope
}

func newMemoryToolExecutor(store memory.Store) *memoryToolExecutor {
	return &memoryToolExecutor{
		store:  store,
		scopes: map[string]map[string]string{},
	}
}

// Name implements tools.Executor. It must equal memory.ExecutorMode so the
// registry routes memory__ tools here.
func (e *memoryToolExecutor) Name() string { return memory.ExecutorMode }

// registerRun binds a scope to a run. Called once per run, before its first
// turn.
func (e *memoryToolExecutor) registerRun(runID string, scope map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scopes[runID] = scope
}

// unregisterRun drops a run's scope. Called when the run finishes, so a long
// matrix does not accumulate scopes for the life of the engine.
func (e *memoryToolExecutor) unregisterRun(runID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.scopes, runID)
}

func (e *memoryToolExecutor) scopeFor(runID string) (map[string]string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	scope, ok := e.scopes[runID]
	return scope, ok
}

// Execute implements tools.Executor by delegating to a scope-bound PromptKit
// executor. An unknown or absent run is an error: falling back to an unscoped
// executor would silently share memory between runs, which is the failure this
// type exists to prevent.
func (e *memoryToolExecutor) Execute(
	ctx context.Context, desc *tools.ToolDescriptor, args json.RawMessage,
) (json.RawMessage, error) {
	runID := runIDFromContext(ctx)
	if runID == "" {
		return nil, fmt.Errorf("memory executor: no run identity on context")
	}
	scope, ok := e.scopeFor(runID)
	if !ok {
		return nil, fmt.Errorf("memory executor: no registered scope for run %q", runID)
	}
	return memory.NewExecutor(e.store, scope).Execute(ctx, desc, args)
}
