package engine

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/AltairaLabs/promptarena/v2/arena/turnexecutors"

	"github.com/AltairaLabs/PromptKit/runtime/v2/logger"
	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/template"
	"github.com/AltairaLabs/PromptKit/runtime/v2/workflow"
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
	// taskPromptConfigVars returns the arena.yaml prompt-config vars for a
	// prompt task. A scripted turn renders with its task's vars, so a handoff
	// swaps the origin task's for the destination's. Nil means none.
	taskPromptConfigVars func(task string) map[string]string

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
func (r *workflowStateResolver) ResolveCurrentState(ctx context.Context) (stage.Handoff, error) {
	return r.resolveWith(ctx, func(string) map[string]string { return nil })
}

// ForTurn returns this resolver bound to one turn, so the handoff renders
// with the variables that turn renders its own prompt with: the scenario's,
// the prompt config's and the variable providers', which VariableProviderStage
// publishes on turnState. Implements turnexecutors.TurnBindableResolver.
//
// The bound resolver shares this one's state (the retained brief), and only
// the rendering differs, so ToolCallRecorder and CurrentStateMeta are this
// resolver's own.
//
// It is called while the turn's pipeline is built, before anything in the
// turn can commit a transition, so the run's task is the one the turn's
// variables were resolved for.
func (r *workflowStateResolver) ForTurn(turnState *stage.TurnState) stage.WorkflowStateResolver {
	t := &turnWorkflowStateResolver{workflowStateResolver: r, turnState: turnState}
	if run := r.run(); run != nil && run.scenario != nil {
		t.turnTask = run.scenario.TaskType
		t.scenarioVars = scenarioVariables(run.scenario)
	}
	return t
}

// turnWorkflowStateResolver is a workflowStateResolver bound to one turn's
// TurnState; see ForTurn.
type turnWorkflowStateResolver struct {
	*workflowStateResolver
	turnState *stage.TurnState
	// turnTask is the prompt task the turn's variables were resolved for,
	// and scenarioVars the scenario's own variables, which outrank any
	// prompt config's.
	turnTask     string
	scenarioVars map[string]string
}

// ResolveCurrentState implements stage.WorkflowStateResolver, rendering with
// the turn's variables.
func (t *turnWorkflowStateResolver) ResolveCurrentState(ctx context.Context) (stage.Handoff, error) {
	var turnVars map[string]string
	if t.turnState != nil {
		turnVars = t.turnState.Variables
	}
	return t.resolveWith(ctx, func(task string) map[string]string {
		return t.varsForTask(turnVars, task)
	})
}

// varsForTask returns the turn's variables as a scripted turn on task would
// have them: the turn's own task's prompt-config vars swapped for task's. The
// scenario's variables outrank both, as they do on a scripted turn, so a key
// the scenario sets is left alone.
func (t *turnWorkflowStateResolver) varsForTask(turnVars map[string]string, task string) map[string]string {
	if t.taskPromptConfigVars == nil || task == t.turnTask {
		return turnVars
	}
	out := maps.Clone(turnVars)
	if out == nil {
		out = map[string]string{}
	}
	for k, v := range t.taskPromptConfigVars(t.turnTask) {
		if _, set := t.scenarioVars[k]; !set && out[k] == v {
			delete(out, k)
		}
	}
	for k, v := range t.taskPromptConfigVars(task) {
		if _, set := t.scenarioVars[k]; !set {
			out[k] = v
		}
	}
	return out
}

// resolveWith is ResolveCurrentState, rendering the state's prompt with
// varsFor(its prompt task) beneath the values the handoff binds.
func (r *workflowStateResolver) resolveWith(
	_ context.Context, varsFor func(task string) map[string]string,
) (stage.Handoff, error) {
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
		// transition, updates scenario.TaskType and emits the observability
		// events. Committing here therefore keeps all of that, rather than
		// duplicating it.
		//
		// A rejected commit must NOT fail the turn. The workflow declines
		// transitions for ordinary reasons — an event invalid in this state, a
		// max-visits redirect, an exhausted budget — and the run simply stays
		// where it is, which the model can see and respond to. Returning the
		// error here instead ends the provider stage, the turn and the run:
		// before this resolver existed the same condition was reported by the
		// post-turn hook and the run continued, and that remains the right
		// behavior. OnCommitError has already emitted the failure event.
		if _, err := run.transExec.CommitPending(); err != nil {
			logger.Warn("workflow transition rejected; continuing in current state",
				"run_id", r.runID, "error", err)
		} else {
			justTransitioned = true
		}
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
		switch workflow.OrchestrationOf(current) {
		case workflow.OrchestrationExternal, workflow.OrchestrationComposition:
			return stage.Handoff{Stop: true}, nil
		case workflow.OrchestrationInternal, workflow.OrchestrationHybrid:
			// Runtime-driven; continue the turn as this state. An unset
			// orchestration also lands here — OrchestrationOf defaults it
			// to internal.
		}
		// RFC 0014: the destination may hand the turn straight back to the
		// user instead of speaking. Checked after orchestration because those
		// two stop for structural reasons that outrank an author's preference,
		// and `control` is inert on an externally orchestrated state anyway —
		// there is no floor to hold when the runtime is not mid-turn.
		//
		// HoldsFloor, not a direct read of current.Control: an absent value
		// deliberately runs on rather than resolving to the RFC's `user`
		// default, and that decision lives in one place shared with the SDK
		// resolver so the two cannot drift (#175).
		if !workflow.HoldsFloor(current) {
			return stage.Handoff{Stop: true}, nil
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

	systemPrompt, allowedTools, err := r.renderState(machine, name, current, summary, varsFor(current.PromptTask))
	if err != nil {
		return stage.Handoff{}, err
	}
	return stage.Handoff{
		Valid:        true,
		SystemPrompt: systemPrompt,
		AllowedTools: allowedTools,
	}, nil
}

// renderState renders the destination state's prompt with the turn's
// variables, the carry-forward context and the run's current artifact values,
// as PromptKit's SDK resolver renders a handoff.
//
// A render that cannot complete fails the round with
// stage.ErrTemplateUnresolved, as TemplateStage fails a turn: the raw template
// would put literal {{...}} in front of the model.
func (r *workflowStateResolver) renderState(
	machine *workflow.StateMachine, stateName string, dest *workflow.State, contextSummary string,
	turnVars map[string]string,
) (systemPrompt string, allowedTools []string, err error) {
	vars := r.workflowVars(machine, contextSummary, turnVars)

	// Required variables are checked after every source is merged, as the
	// template stage checks them, so one a fragment or a default supplies
	// counts.
	tmpl, err := r.exec.promptRegistry.LoadTemplateDeferRequired(dest.PromptTask, vars, "")
	if err != nil {
		return "", nil, fmt.Errorf("load prompt %q for state %q: %w", dest.PromptTask, stateName, err)
	}

	// Same precedence as the template stage: template defaults, then
	// fragments, then the variables bound above.
	merged := make(map[string]string, len(tmpl.DefaultVars)+len(tmpl.FragmentVars)+len(vars))
	maps.Copy(merged, tmpl.DefaultVars)
	maps.Copy(merged, tmpl.FragmentVars)
	maps.Copy(merged, vars)

	if missing := missingRequiredVars(tmpl.RequiredVars, merged); len(missing) > 0 {
		return "", nil, fmt.Errorf("%w: %s: missing required variables: %s",
			stage.ErrTemplateUnresolved, dest.PromptTask, strings.Join(missing, ", "))
	}

	rendered, err := r.renderer.RenderDetailed(tmpl.RawTemplate, merged)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %s: %w", stage.ErrTemplateUnresolved, dest.PromptTask, err)
	}
	return rendered.Text, tmpl.AllowedTools, nil
}

// workflowVars returns vars with the workflow's own variables layered on.
// Lowest to highest: every artifact a state declares, bound empty so one
// nothing has set yet renders empty; vars; then the artifacts set so far and
// the brief the outgoing state wrote, as PromptKit's SDK binds them.
func (r *workflowStateResolver) workflowVars(
	machine *workflow.StateMachine, contextSummary string, vars map[string]string,
) map[string]string {
	out := declaredArtifactVars(r.exec.wfSpec)
	maps.Copy(out, vars)
	if machine != nil {
		for name, value := range machine.Artifacts() {
			out["artifacts."+name] = value
		}
	}
	if contextSummary != "" {
		out[workflowContextVar] = contextSummary
	}
	return out
}

// turnPromptVars layers the workflow's variables over a scripted turn's prompt
// variables, so the template stage renders the run's current state as the
// handoff into it rendered it. Without them a state prompt reading an artifact
// or the brief failed its first scripted turn.
func (r *workflowStateResolver) turnPromptVars(vars map[string]string) map[string]string {
	var machine *workflow.StateMachine
	if run := r.run(); run != nil && run.transExec != nil {
		machine = run.transExec.StateMachine()
	}
	r.mu.Lock()
	summary := r.contextSummary
	r.mu.Unlock()
	return r.workflowVars(machine, summary, vars)
}

// missingRequiredVars returns the required names with no non-empty value in
// vars, in declaration order, as the template stage computes them.
func missingRequiredVars(required []string, vars map[string]string) []string {
	var missing []string
	for _, name := range required {
		if vars[name] == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

// declaredArtifactVars returns "artifacts.<name>" bound to "" for every
// artifact a state of spec declares. Bound beneath every other value, it makes
// a prompt that reads an artifact nothing has set yet render it empty, as a
// missing optional fragment renders empty. An artifact no state declares stays
// unbound, so reading one still fails the render.
func declaredArtifactVars(spec *workflow.Spec) map[string]string {
	vars := map[string]string{}
	if spec == nil {
		return vars
	}
	for _, state := range spec.States {
		if state == nil {
			continue
		}
		for name := range state.Artifacts {
			vars["artifacts."+name] = ""
		}
	}
	return vars
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
		meta["terminal"] = workflow.IsTerminal(state)
	}
	return meta
}

// compile-time checks that this satisfies both runtime interfaces. The tool
// loop type-asserts for ToolCallRecorder separately, so losing it would be
// silent — the budget would simply stop counting.
var (
	_ stage.WorkflowStateResolver        = (*workflowStateResolver)(nil)
	_ stage.ToolCallRecorder             = (*workflowStateResolver)(nil)
	_ turnexecutors.TurnBindableResolver = (*workflowStateResolver)(nil)
	_ stage.WorkflowStateResolver        = (*turnWorkflowStateResolver)(nil)
	_ stage.ToolCallRecorder             = (*turnWorkflowStateResolver)(nil)
)
