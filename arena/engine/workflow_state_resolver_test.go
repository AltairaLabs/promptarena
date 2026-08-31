package engine

import (
	"context"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/packspec"
	"github.com/AltairaLabs/PromptKit/runtime/persistence/memory"
	"github.com/AltairaLabs/PromptKit/runtime/prompt"
	"github.com/AltairaLabs/PromptKit/runtime/tools"
	"github.com/AltairaLabs/PromptKit/runtime/workflow"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// resolverPromptRegistry seeds one template per prompt task so renderState has
// something real to load.
func resolverPromptRegistry(t *testing.T, tasks map[string]string) *prompt.Registry {
	t.Helper()
	repo := memory.NewPromptRepository()
	for task, tmpl := range tasks {
		repo.RegisterPrompt(task, &prompt.Config{
			APIVersion: "promptkit.altairalabs.ai/v1alpha1",
			Kind:       "PromptConfig",
			Metadata:   metav1.ObjectMeta{Name: task},
			Spec: prompt.Spec{
				TaskType:       task,
				Version:        "v1.0.0",
				SystemTemplate: tmpl,
			},
		})
	}
	return prompt.NewRegistryWithRepository(repo)
}

// resolverSpec is a two-hop workflow: triage escalates to specialist, which
// resolves into a terminal state.
func resolverSpec() *workflow.Spec {
	return &workflow.Spec{
		Entry: "triage",
		States: map[string]*workflow.State{
			"triage": {
				PromptTask: "triage",
				OnEvent:    map[string]string{"Escalate": "specialist"},
			},
			"specialist": {
				PromptTask:  "specialist",
				Description: "Handles escalations",
				OnEvent:     map[string]string{"Resolve": "closed"},
			},
			"closed": {PromptTask: "closed", Terminal: packspec.Ptr(true)},
		},
	}
}

// newResolverFixture wires an executor with a prompt registry and one
// registered run, and returns the resolver bound to it.
func newResolverFixture(t *testing.T, startState string) (*workflowTransitionExecutor, *workflowStateResolver) {
	t.Helper()
	spec := resolverSpec()
	registry := tools.NewRegistry()
	registerTransitionToolForSpec(registry, spec)

	exec := newWorkflowTransitionExecutor(spec, registry)
	exec.setPromptRegistry(resolverPromptRegistry(t, map[string]string{
		"triage":     "You are triage.",
		"specialist": "You are the specialist. Context: {{workflow_context}}",
		"closed":     "Closed.",
	}))
	exec.RegisterRunAtState("run-1", &arenaconfig.Scenario{ID: "s1"}, nil, startState)

	res := exec.ResolverForRun("run-1")
	require.NotNil(t, res, "expected a resolver for a registered run")
	return exec, res.(*workflowStateResolver)
}

// deferEvent puts a transition in the pending slot the way the tool does.
func deferEvent(t *testing.T, exec *workflowTransitionExecutor, runID, event string) {
	t.Helper()
	desc := exec.registry.Get(workflow.TransitionToolName)
	require.NotNil(t, desc)
	_, err := exec.Execute(
		withWorkflowScenarioID(context.Background(), runID),
		desc,
		[]byte(`{"event":"`+event+`","context":"brief for the next state"}`),
	)
	require.NoError(t, err)
}

func TestResolverForRun_NilWithoutPromptRegistry(t *testing.T) {
	spec := resolverSpec()
	exec := newWorkflowTransitionExecutor(spec, tools.NewRegistry())
	exec.RegisterRunAtState("run-1", &arenaconfig.Scenario{ID: "s1"}, nil, "triage")

	// Nil must be a nil INTERFACE, not a typed nil pointer: the provider stage
	// tests the interface, so a typed nil would be called and panic.
	assert.Nil(t, exec.ResolverForRun("run-1"),
		"no prompt registry means no in-turn handoff")
}

func TestResolverForRun_NilForUnregisteredRun(t *testing.T) {
	exec, _ := newResolverFixture(t, "triage")
	assert.Nil(t, exec.ResolverForRun("no-such-run"))
}

func TestResolveCurrentState_RendersCurrentStateWithoutTransition(t *testing.T) {
	_, res := newResolverFixture(t, "triage")

	h, err := res.ResolveCurrentState(context.Background())
	require.NoError(t, err)
	assert.True(t, h.Valid)
	assert.False(t, h.Stop)
	assert.Contains(t, h.SystemPrompt, "You are triage.")
}

// The transition the tool left pending commits here, and the turn continues as
// the destination state — this is the whole point of the resolver.
func TestResolveCurrentState_CommitsPendingAndRendersDestination(t *testing.T) {
	exec, res := newResolverFixture(t, "triage")
	deferEvent(t, exec, "run-1", "Escalate")

	h, err := res.ResolveCurrentState(context.Background())
	require.NoError(t, err)
	assert.True(t, h.Valid)
	assert.Contains(t, h.SystemPrompt, "You are the specialist.",
		"the destination state's prompt must drive the next round")
	assert.Equal(t, "specialist", exec.StateMachine("run-1").CurrentState())

	// The brief the outgoing state wrote is bound for the incoming one.
	assert.Contains(t, h.SystemPrompt, "brief for the next state")
}

// Callers re-resolve between every round, so this must be safe to repeat and
// must keep reporting the state the turn should be running as.
func TestResolveCurrentState_IsIdempotent(t *testing.T) {
	exec, res := newResolverFixture(t, "triage")
	deferEvent(t, exec, "run-1", "Escalate")

	first, err := res.ResolveCurrentState(context.Background())
	require.NoError(t, err)
	second, err := res.ResolveCurrentState(context.Background())
	require.NoError(t, err)

	assert.Equal(t, first.SystemPrompt, second.SystemPrompt)
	assert.Equal(t, "specialist", exec.StateMachine("run-1").CurrentState(),
		"a second resolve must not advance the machine again")
}

// A rejected transition is an ordinary outcome, not a dead run: the turn
// continues in the state it was already in. Returning the error instead ends
// the provider stage, the turn and the run.
func TestResolveCurrentState_RejectedTransitionDoesNotFailTheTurn(t *testing.T) {
	exec, res := newResolverFixture(t, "triage")

	// "Resolve" belongs to specialist; the run is in triage, so committing it
	// is refused by this run's state machine.
	deferEvent(t, exec, "run-1", "Resolve")

	h, err := res.ResolveCurrentState(context.Background())
	require.NoError(t, err, "a refused transition must not fail the turn")
	assert.True(t, h.Valid)
	assert.Contains(t, h.SystemPrompt, "You are triage.",
		"the turn continues in the state it was already in")
	assert.Equal(t, "triage", exec.StateMachine("run-1").CurrentState())
}

func TestResolveCurrentState_NoRunReturnsEmptyHandoff(t *testing.T) {
	exec, res := newResolverFixture(t, "triage")
	exec.UnregisterRun("run-1")

	h, err := res.ResolveCurrentState(context.Background())
	require.NoError(t, err)
	assert.False(t, h.Valid)
	assert.False(t, h.Stop)
}

// A state with no prompt_task leaves the turn on whatever prompt it has rather
// than blanking it.
func TestResolveCurrentState_NoPromptTaskLeavesPromptAlone(t *testing.T) {
	spec := resolverSpec()
	spec.States["triage"].PromptTask = ""
	registry := tools.NewRegistry()
	registerTransitionToolForSpec(registry, spec)

	exec := newWorkflowTransitionExecutor(spec, registry)
	exec.setPromptRegistry(resolverPromptRegistry(t, map[string]string{"specialist": "spec"}))
	exec.RegisterRunAtState("run-1", &arenaconfig.Scenario{ID: "s1"}, nil, "triage")

	h, err := exec.ResolverForRun("run-1").ResolveCurrentState(context.Background())
	require.NoError(t, err)
	assert.False(t, h.Valid)
}

// Externally orchestrated states wait for an injected event, so the turn must
// stop rather than run on into them — but only when we just transitioned.
func TestResolveCurrentState_StopsOnExternalDestination(t *testing.T) {
	spec := resolverSpec()
	spec.States["specialist"].Orchestration = packspec.Ptr(workflow.OrchestrationExternal)
	registry := tools.NewRegistry()
	registerTransitionToolForSpec(registry, spec)

	exec := newWorkflowTransitionExecutor(spec, registry)
	exec.setPromptRegistry(resolverPromptRegistry(t, map[string]string{
		"triage": "triage", "specialist": "spec",
	}))
	exec.RegisterRunAtState("run-1", &arenaconfig.Scenario{ID: "s1"}, nil, "triage")
	deferEvent(t, exec, "run-1", "Escalate")

	h, err := exec.ResolverForRun("run-1").ResolveCurrentState(context.Background())
	require.NoError(t, err)
	assert.True(t, h.Stop, "an external destination must not continue the turn")
	assert.False(t, h.Valid)
}

// Sitting in an external state without having just transitioned is an ordinary
// scripted turn, which must still be answered.
func TestResolveCurrentState_ExternalStateStillAnswersWithoutTransition(t *testing.T) {
	spec := resolverSpec()
	spec.States["specialist"].Orchestration = packspec.Ptr(workflow.OrchestrationExternal)
	registry := tools.NewRegistry()
	registerTransitionToolForSpec(registry, spec)

	exec := newWorkflowTransitionExecutor(spec, registry)
	exec.setPromptRegistry(resolverPromptRegistry(t, map[string]string{"specialist": "You are the specialist."}))
	exec.RegisterRunAtState("run-1", &arenaconfig.Scenario{ID: "s1"}, nil, "specialist")

	h, err := exec.ResolverForRun("run-1").ResolveCurrentState(context.Background())
	require.NoError(t, err)
	assert.False(t, h.Stop)
	assert.True(t, h.Valid)
}

func TestCurrentStateMeta_DescribesTheActiveState(t *testing.T) {
	exec, res := newResolverFixture(t, "triage")

	meta := res.CurrentStateMeta()
	require.NotNil(t, meta)
	assert.Equal(t, "triage", meta[currentStateMetaKey])
	assert.Equal(t, false, meta["terminal"])

	deferEvent(t, exec, "run-1", "Escalate")
	_, err := res.ResolveCurrentState(context.Background())
	require.NoError(t, err)

	meta = res.CurrentStateMeta()
	assert.Equal(t, "specialist", meta[currentStateMetaKey],
		"metadata must follow the state within the turn, not the turn's start state")
	assert.Equal(t, "Handles escalations", meta["description"])
}

func TestCurrentStateMeta_NilForUnregisteredRun(t *testing.T) {
	exec, res := newResolverFixture(t, "triage")
	exec.UnregisterRun("run-1")
	assert.Nil(t, res.CurrentStateMeta())
}

func TestRecordToolCalls_FeedsTheWorkflowBudget(t *testing.T) {
	exec, res := newResolverFixture(t, "triage")

	before := exec.StateMachine("run-1").Context().TotalToolCalls
	res.RecordToolCalls(3)
	assert.Equal(t, before+3, exec.StateMachine("run-1").Context().TotalToolCalls,
		"the tool loop's count must reach the workflow context, or max_tool_calls stays inert")
}

func TestRecordToolCalls_IgnoresNonPositiveAndMissingRun(t *testing.T) {
	exec, res := newResolverFixture(t, "triage")

	res.RecordToolCalls(0)
	res.RecordToolCalls(-2)
	assert.Zero(t, exec.StateMachine("run-1").Context().TotalToolCalls)

	exec.UnregisterRun("run-1")
	res.RecordToolCalls(5) // must not panic once the run is gone
}
