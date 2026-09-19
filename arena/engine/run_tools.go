package engine

import (
	"context"

	"github.com/AltairaLabs/PromptKit/runtime/v2/memory"
	"github.com/AltairaLabs/PromptKit/runtime/v2/skills"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

// Memory scope keys. A run's memories are addressed by scenario and run, so
// one scenario's recall cannot reach another's.
const (
	memoryScopeScenario = "scenario"
	memoryScopeRun      = "run"
)

// runTools is the tool state a single run owns.
//
// Arena executes a matrix — many runs as goroutines over one engine. A
// tools.Registry keys executors by name and holds exactly one per name, so any
// executor carrying per-conversation state is a collision waiting to happen
// when runs share a registry. Arena used to work around that with a run-keyed
// map inside each executor.
//
// PromptKit v2.3.0 (#2011, #2014) removes the need for that, two ways, and this
// type uses whichever fits each subsystem:
//
//   - Skills and memory keep their per-conversation state OUTSIDE the executor
//     — an ActiveSet and a scope — which the run carries on its context. The
//     executors become catalog and policy only, and are shared safely
//     engine-wide.
//   - The HTTP executor's response budget still lives on the instance, so the
//     run gets its own via a child registry: a child shares its parent's tool
//     DESCRIPTORS but owns its EXECUTORS, and lookup falls through to the
//     parent for names the run never claims.
type runTools struct {
	// registry is this run's child registry, handed to the pipeline via
	// TurnRequest.ToolRegistry.
	registry *tools.Registry

	// activeSet is this run's skill activation state, nil when the config
	// declares no skills. It is the caller-owned half of the skills executor.
	activeSet *skills.ActiveSet

	// memoryScope is the scope this run's memories are written under, nil when
	// memory is not configured. Kept so seed memories land in the same place
	// the run's tool calls will.
	memoryScope map[string]string
}

// buildRunTools creates the per-run tool state.
//
// Nothing here touches the engine-wide registry: RegisterExecutor writes to the
// child. That is the property TestExecutorRegistrationIsConfinedToInit protects
// for every other file, and the reason this one is allowed to register during a
// run.
func (e *Engine) buildRunTools(scenarioID, runID string) *runTools {
	rt := &runTools{registry: e.toolRegistry.Child()}

	// Live HTTP tools. tools.HTTPExecutor caps the cumulative size of every
	// response it returns against one counter held on the instance, so the
	// budget is per executor. Built per run, it is per run — which is what the
	// SDK gets for free by building one executor per conversation. Shared, a
	// late run could fail because of traffic from earlier, unrelated runs.
	rt.registry.RegisterExecutor(tools.NewHTTPExecutor())

	if e.memoryStore != nil {
		rt.memoryScope = map[string]string{
			memoryScopeScenario: scenarioID,
			memoryScopeRun:      runID,
		}
	}

	if e.skillsFactory != nil {
		rt.activeSet = skills.NewActiveSet()
		e.skillsFactory.preload(rt.activeSet)
	}

	return rt
}

// bindContext attaches this run's skill and memory state to the run context.
// The engine-wide executors read it from there, so one run's activations and
// memories cannot reach another's.
func (rt *runTools) bindContext(ctx context.Context) context.Context {
	if rt == nil {
		return ctx
	}
	if rt.activeSet != nil {
		ctx = skills.WithActiveSet(ctx, rt.activeSet)
	}
	if rt.memoryScope != nil {
		ctx = memory.WithScope(ctx, rt.memoryScope)
	}
	return ctx
}

// toolGrants returns the live accessor for this run's skill tool grants, or nil
// when the run has no skills. The provider stage re-reads it after every tool
// round, so it must stay live rather than being snapshotted.
func (e *Engine) toolGrants(rt *runTools) func() []string {
	if rt == nil || rt.activeSet == nil || e.skillsFactory == nil {
		return nil
	}
	exec := e.skillsFactory.executor
	set := rt.activeSet
	return func() []string { return exec.ToolsFor(set) }
}
