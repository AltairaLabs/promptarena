package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// httpToolExecutor meters HTTP tool responses per run.
//
// tools.HTTPExecutor caps the cumulative size of every response it has ever
// returned, against one counter. In the SDK that is a per-conversation budget,
// because each Open() builds its own executor. Arena registers one executor for
// the whole engine, so the same counter covers an entire matrix: runs late in a
// sweep could fail with ErrAggregateResponseSizeExceeded because of traffic
// from earlier, unrelated runs, and the failure looked like flake because it
// depended on run order.
//
// So the wrapped executor's own cap is disabled and the budget is kept here,
// per run — the same shape as memoryToolExecutor and skillsToolExecutor. Calls
// arriving without a run identity share a single bucket, which preserves the
// previous behavior outside a run.
//
// Entries are never evicted, deliberately: an int64 per run is nothing next to
// the run records arena already keeps, and unlike a memory scope or an active
// skill set nothing about correctness depends on the entry going away. Adding a
// teardown hook would mean widening BuildEngineComponents' return signature for
// a counter.
type httpToolExecutor struct {
	inner     *tools.HTTPExecutor
	maxPerRun int64

	mu    sync.Mutex
	usage map[string]int64 // runID -> cumulative response bytes
}

func newHTTPToolExecutor(maxPerRun int64) *httpToolExecutor {
	return &httpToolExecutor{
		// The inner cap is disabled (0) because this type owns the budget.
		inner:     tools.NewHTTPExecutorWithMaxAggregate(0),
		maxPerRun: maxPerRun,
		usage:     map[string]int64{},
	}
}

// Name implements tools.Executor, matching the mode the wrapped executor claims
// so the registry routes live HTTP tools here.
func (e *httpToolExecutor) Name() string { return e.inner.Name() }

// checkBudget reports whether this run has already exhausted its budget.
func (e *httpToolExecutor) checkBudget(runID string) error {
	if e.maxPerRun <= 0 {
		return nil
	}
	e.mu.Lock()
	used := e.usage[runID]
	e.mu.Unlock()
	if used >= e.maxPerRun {
		return fmt.Errorf(
			"%w: run has used %d bytes of its %d byte budget",
			tools.ErrAggregateResponseSizeExceeded, used, e.maxPerRun,
		)
	}
	return nil
}

// record adds a response's size to the run's running total.
func (e *httpToolExecutor) record(runID string, n int) {
	if n <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.usage[runID] += int64(n)
}

// Execute implements tools.Executor.
func (e *httpToolExecutor) Execute(
	ctx context.Context, descriptor *tools.ToolDescriptor, args json.RawMessage,
) (json.RawMessage, error) {
	runID := runIDFromContext(ctx)
	if err := e.checkBudget(runID); err != nil {
		return nil, err
	}
	result, err := e.inner.Execute(ctx, descriptor, args)
	e.record(runID, len(result))
	return result, err
}

// ExecuteMultimodal implements tools.MultimodalExecutor. The registry prefers
// this over Execute when an executor offers it, so it must be forwarded — a
// wrapper that only implemented Execute would silently drop the content parts
// binary HTTP responses produce.
func (e *httpToolExecutor) ExecuteMultimodal(
	ctx context.Context, descriptor *tools.ToolDescriptor, args json.RawMessage,
) (json.RawMessage, []types.ContentPart, error) {
	runID := runIDFromContext(ctx)
	if err := e.checkBudget(runID); err != nil {
		return nil, nil, err
	}
	result, parts, err := e.inner.ExecuteMultimodal(ctx, descriptor, args)
	e.record(runID, len(result))
	return result, parts, err
}
