package engine

import (
	"context"
	"fmt"
	"maps"
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/template"
	"github.com/AltairaLabs/PromptKit/runtime/workflow"
)

// workflowContextVar is the template variable carrying the brief the outgoing
// state wrote for the incoming one. Matches the SDK so a pack's state prompts
// render identically under arena and under sdk.OpenWorkflow.
const workflowContextVar = "workflow_context"

// currentStateMetaKey names the active workflow state in the metadata stamped
// onto each assistant message.
const currentStateMetaKey = "current_state"

// workflowStateResolver implements stage.WorkflowStateResolver for a single
// arena run.
//
// Arena differs from the SDK in one way that shapes this type: the SDK has one
// state machine per conversation, while arena runs many scenarios concurrently
// against one shared executor, keyed by run ID. stage.WorkflowStateResolver's
// CurrentStateMeta takes no context, so the run cannot be recovered at call
// time — the binding has to happen when the resolver is built. Arena builds a
// fresh pipeline per turn, so one resolver per run, handed to that run's
// ProviderStage, is the natural fit.
type workflowStateResolver struct {
	exec     *workflowTransitionExecutor
	runID    string
	renderer *template.Renderer

	// mu guards contextSummary. ResolveCurrentState is called between provider
	// rounds on the turn's goroutine, but the run's pipeline and the engine's
	// bookkeeping are not guaranteed to share one, and the executor's own map
	// is mutex-guarded for the same reason.
	mu sync.Mutex
	// contextSummary is the brief the outgoing state wrote for the incoming
	// one. Retained because ResolveCurrentState re-renders on every call,
	// including ones long after the transition committed and cleared the
	// pending record.
	contextSummary string
}

// ResolverForRun returns a stage.WorkflowStateResolver bound to one run, or nil
// when the run is not registered or the engine has no prompt registry to render
// destination prompts with.
//
// The nil is returned as a nil interface rather than a typed nil pointer: a
// typed nil boxed into an interface is not == nil, and the provider stage tests
// the interface, so returning one would turn "no workflow here" into a nil
// dereference on the first round.
func (e *workflowTransitionExecutor) ResolverForRun(runID string) stage.WorkflowStateResolver {
	if e == nil || e.promptRegistry == nil {
		return nil
	}

	e.mu.Lock()
	_, ok := e.runs[runID]
	e.mu.Unlock()
	if !ok {
		return nil
	}

	return &workflowStateResolver{
		exec:     e,
		runID:    runID,
		renderer: template.NewRenderer(),
	}
}

// run returns this resolver's run state, or nil once it has been unregistered.
func (r *workflowStateResolver) run() *workflowRunState {
	r.exec.mu.Lock()
	defer r.exec.mu.Unlock()
	return r.exec.runs[r.runID]
}

// RecordToolCalls implements stage.ToolCallRecorder.
//
// The runtime tool loop is the only place that observes every executed call on
// every path, so it counts and forwards the number here; the count feeds
// engine.budget.max_tool_calls, which stays inert without it.
func (r *workflowStateResolver) RecordToolCalls(n int) {
	if n <= 0 {
		return
	}
	run := r.run()
	if run == nil || run.transExec == nil {
		return
	}
	if machine := run.transExec.StateMachine(); machine != nil {
		machine.IncrementToolCalls(n)
	}
}

// ResolveCurrentState implements stage.WorkflowStateResolver.
//
// It commits whatever transition the workflow__transition tool left pending and
// reports the prompt and tools for the state the run is now in, so the tool
// loop's next round speaks as the destination state. Without it the machine
// advances and the destination never speaks until some later scripted turn
// happens to arrive.
//
// It reports what the turn should be running rather than what just changed, so
// a re-executed pipeline — which re-runs prompt assembly and resets the prompt
// to whatever the pipeline was built for — is corrected on the next call.
// Callers rely on it being safe to call repeatedly.
func (r *workflowStateResolver) ResolveCurrentState(_ context.Context) (stage.Handoff, error) {
	run := r.run()
	if run == nil || run.transExec == nil {
		return stage.Handoff{}, nil
	}

	// Capture the brief before committing: CommitPending clears the pending
	// record, and the context argument is what the outgoing state wrote for
	// the incoming one.
	justTransitioned := false
	if pending := run.transExec.Pending(); pending != nil {
		r.mu.Lock()
		r.contextSummary = pending.ContextSummary
		r.mu.Unlock()
		// CommitPending fires the executor's OnCommit hook, which records the
		// transition, updates scenario.TaskType, re-registers the transition
		// tool for the new state and emits the observability events. Committing
		// here therefore keeps all of that, rather than duplicating it.
		if _, err := run.transExec.CommitPending(); err != nil {
			return stage.Handoff{}, fmt.Errorf("transition commit failed: %w", err)
		}
		justTransitioned = true
	}

	machine := run.transExec.StateMachine()
	if machine == nil {
		return stage.Handoff{}, nil
	}
	name := machine.CurrentState()
	current := r.exec.wfSpec.States[name]
	if current == nil {
		return stage.Handoff{}, fmt.Errorf("current state %q not found in workflow spec", name)
	}

	// States the turn must not run on into, and only when we arrived by
	// committing during this turn — an externally orchestrated state still
	// handles an ordinary scripted turn normally. Treating it as "never run"
	// would end the turn with no rounds and an empty response.
	//
	//	external    — waits for an injected event rather than continuing.
	//	composition — CompositionStage runs the state itself.
	if justTransitioned {
		switch current.Orchestration {
		case workflow.OrchestrationExternal, workflow.OrchestrationComposition:
			return stage.Handoff{Stop: true}, nil
		case workflow.OrchestrationInternal, workflow.OrchestrationHybrid:
			// Runtime-driven; continue the turn as this state. The zero value
			// also lands here, which means internal.
		}
	}

	// Nothing to render: leave the turn on the prompt it already has rather
	// than blanking it.
	if current.PromptTask == "" {
		return stage.Handoff{}, nil
	}

	r.mu.Lock()
	summary := r.contextSummary
	r.mu.Unlock()

	systemPrompt, allowedTools, err := r.renderState(machine, name, current, summary)
	if err != nil {
		return stage.Handoff{}, err
	}
	return stage.Handoff{
		Valid:        true,
		SystemPrompt: systemPrompt,
		AllowedTools: allowedTools,
	}, nil
}

// renderState renders the destination state's prompt with the carry-forward
// context and the run's current artifact values bound.
func (r *workflowStateResolver) renderState(
	machine *workflow.StateMachine, stateName string, dest *workflow.State, contextSummary string,
) (systemPrompt string, allowedTools []string, err error) {
	vars := map[string]string{}
	for name, value := range machine.Artifacts() {
		vars["artifacts."+name] = value
	}
	if contextSummary != "" {
		vars[workflowContextVar] = contextSummary
	}

	tmpl, err := r.exec.promptRegistry.LoadTemplate(dest.PromptTask, vars, "")
	if err != nil {
		return "", nil, fmt.Errorf("load prompt %q for state %q: %w", dest.PromptTask, stateName, err)
	}

	// Same precedence as the template stage: template defaults, then
	// fragments, then the values bound for this handoff.
	merged := make(map[string]string, len(tmpl.DefaultVars)+len(tmpl.FragmentVars)+len(vars))
	maps.Copy(merged, tmpl.DefaultVars)
	maps.Copy(merged, tmpl.FragmentVars)
	maps.Copy(merged, vars)

	rendered, err := r.renderer.RenderDetailed(tmpl.RawTemplate, merged)
	if err != nil {
		// Degrade to the raw template rather than failing the turn, matching
		// what the template stage does: a template that renders badly should
		// still hand off.
		return tmpl.RawTemplate, tmpl.AllowedTools, nil //nolint:nilerr // deliberate degrade
	}
	return rendered.Text, tmpl.AllowedTools, nil
}

// CurrentStateMeta implements stage.WorkflowStateResolver. The map is stamped
// onto each assistant message the turn produces, so output can be attributed to
// the state that generated it — a turn may now span states, which per-turn
// attribution cannot express.
func (r *workflowStateResolver) CurrentStateMeta() map[string]any {
	run := r.run()
	if run == nil || run.transExec == nil {
		return nil
	}
	machine := run.transExec.StateMachine()
	if machine == nil {
		return nil
	}

	name := machine.CurrentState()
	meta := map[string]any{currentStateMetaKey: name}
	if state := r.exec.wfSpec.States[name]; state != nil {
		if state.Description != "" {
			meta["description"] = state.Description
		}
		meta["terminal"] = state.Terminal || len(state.OnEvent) == 0
	}
	return meta
}

// compile-time checks that this satisfies both runtime interfaces. The tool
// loop type-asserts for ToolCallRecorder separately, so losing it would be
// silent — the budget would simply stop counting.
var (
	_ stage.WorkflowStateResolver = (*workflowStateResolver)(nil)
	_ stage.ToolCallRecorder      = (*workflowStateResolver)(nil)
)
