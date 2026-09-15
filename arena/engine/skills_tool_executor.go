package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/v2/skills"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

// SkillsExecutor is the engine's handle on skill activation state. Arena shares
// one tool registry across every concurrent run, so the object registered in it
// is shared; the per-run lifecycle below is how a run claims its own
// active-skill set and its own tool grants.
//
// It is returned by BuildEngineComponents so programmatically built engines get
// the same per-run isolation as file-configured ones.
type SkillsExecutor interface {
	tools.Executor

	// RegisterRun creates this run's skill state, replaying preloaded skills.
	RegisterRun(runID string)
	// UnregisterRun drops it when the run finishes.
	UnregisterRun(runID string)
	// GrantsFor returns the live tool-grant accessor for a run, suitable for
	// stage.ProviderConfig.ToolGrants.
	GrantsFor(runID string) func() []string
}

// skillsToolExecutor is the single skills executor registered in the
// engine-wide tool registry. PromptKit's skills.Executor holds its active-skill
// set in one unkeyed map, which is correct for the SDK (one executor per
// conversation) and wrong for arena (one engine, many concurrent runs). Sharing
// it would mean a skill activated in one run granted its tools in every other —
// a permission surface, so worse than the inert behavior it replaces.
//
// So each run gets its own skills.Executor. What stays shared is everything
// immutable: the discovered skill registry, the pack's tool ceiling and the
// config dir, all carried on the ExecutorConfig template.
type skillsToolExecutor struct {
	cfg       skills.ExecutorConfig
	preloaded []*skills.Skill

	mu   sync.RWMutex
	runs map[string]*skills.Executor
}

func newSkillsToolExecutor(cfg skills.ExecutorConfig, preloaded []*skills.Skill) *skillsToolExecutor {
	return &skillsToolExecutor{
		cfg:       cfg,
		preloaded: preloaded,
		runs:      map[string]*skills.Executor{},
	}
}

// Name implements tools.Executor. It must equal skills.SkillExecutorName so the
// registry routes skill__ tools here.
func (e *skillsToolExecutor) Name() string { return skills.SkillExecutorName }

// RegisterRun creates this run's executor and replays the preloaded skills into
// it. Preloading is best-effort, matching PromptKit's SDK capability: a skill
// that fails to preload can still be activated on demand.
func (e *skillsToolExecutor) RegisterRun(runID string) {
	exec := skills.NewExecutor(e.cfg)
	for _, sk := range e.preloaded {
		_, _, _ = exec.Activate(sk.Name)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runs[runID] = exec
}

// UnregisterRun drops a run's executor once the run has finished, so a long
// matrix does not accumulate executors for the life of the engine.
func (e *skillsToolExecutor) UnregisterRun(runID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.runs, runID)
}

// executorFor returns the run's executor, or nil when the run is unknown.
func (e *skillsToolExecutor) executorFor(runID string) *skills.Executor {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.runs[runID]
}

// GrantsFor returns the live tool-grant accessor for a run, suitable for
// stage.ProviderConfig.ToolGrants. The provider stage re-reads it after every
// tool round, so it must reflect activations made mid-turn. An unknown run
// grants nothing.
func (e *skillsToolExecutor) GrantsFor(runID string) func() []string {
	return func() []string {
		exec := e.executorFor(runID)
		if exec == nil {
			return nil
		}
		return exec.ActiveTools()
	}
}

// Execute implements tools.Executor by delegating to a PromptKit skills tool
// executor bound to the calling run. An unknown run is an error rather than a
// fallback to some shared executor: a silent fallback is exactly the cross-run
// grant leak this type exists to prevent.
func (e *skillsToolExecutor) Execute(
	ctx context.Context, desc *tools.ToolDescriptor, args json.RawMessage,
) (json.RawMessage, error) {
	runID := runIDFromContext(ctx)
	if runID == "" {
		return nil, fmt.Errorf("skills executor: no run identity on context")
	}
	exec := e.executorFor(runID)
	if exec == nil {
		return nil, fmt.Errorf("skills executor: no registered run %q", runID)
	}
	return skills.NewToolExecutor(exec).Execute(ctx, desc, args)
}
