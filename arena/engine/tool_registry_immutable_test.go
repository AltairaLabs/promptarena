package engine

import (
	"context"
	"sort"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/tools"
	"github.com/AltairaLabs/PromptKit/runtime/workflow"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/stretchr/testify/require"
)

// snapshotRegistry renders every registered tool to a stable string so two
// points in time can be compared exactly — names, modes and full parameter
// schemas. Comparing only names would miss the bug this guards against, which
// rewrote a descriptor's event enum while leaving its name alone.
func snapshotRegistry(t *testing.T, registry *tools.Registry) []string {
	t.Helper()
	var out []string
	for name, desc := range registry.GetTools() {
		out = append(out, name+"|"+desc.Mode+"|"+string(desc.InputSchema))
	}
	sort.Strings(out)
	return out
}

// twoStateSpec is a workflow whose states have DIFFERENT event sets, which is
// the configuration that exposes shared-registry mutation: whichever run
// transitioned last would leave its own state's events in place for everyone.
func twoStateSpec() *workflow.Spec {
	return &workflow.Spec{
		Entry: "triage",
		States: map[string]*workflow.State{
			"triage":     {PromptTask: "triage", OnEvent: map[string]string{"Escalate": "specialist"}},
			"specialist": {PromptTask: "specialist", OnEvent: map[string]string{"Resolve": "closed"}},
			"closed":     {PromptTask: "closed", Terminal: true},
		},
	}
}

// The tool registry is shared by every concurrent run, so it must not change
// once built. This is the property, asserted directly: register the workflow
// tools, snapshot the registry, then drive real runs through registration and
// transitions and confirm the registry is byte-identical afterwards.
//
// A comment saying "don't mutate this" does not survive contact with a future
// change. This test does: anything that re-registers, narrows or removes a
// descriptor during a run fails here with a diff.
func TestToolRegistryIsImmutableAcrossRuns(t *testing.T) {
	registry := tools.NewRegistry()
	spec := twoStateSpec()
	registerTransitionToolForSpec(registry, spec)

	exec := newWorkflowTransitionExecutor(spec, registry)
	before := snapshotRegistry(t, registry)
	require.NotEmpty(t, before, "expected the transition tool to be registered")

	// Two concurrent runs starting in different states — the shape that used to
	// corrupt the descriptor, because each run's transitions rewrote the enum
	// the other depended on.
	exec.RegisterRunAtState("run-a", &arenaconfig.Scenario{ID: "a"}, nil, "triage")
	exec.RegisterRunAtState("run-b", &arenaconfig.Scenario{ID: "b"}, nil, "specialist")

	require.Equal(t, before, snapshotRegistry(t, registry),
		"registering a run must not touch the shared tool registry")

	// Drive each run through a transition, including into a terminal state.
	commit(t, exec, "run-a", "Escalate")
	require.Equal(t, before, snapshotRegistry(t, registry),
		"committing a transition must not touch the shared tool registry")

	commit(t, exec, "run-b", "Resolve")
	require.Equal(t, before, snapshotRegistry(t, registry),
		"transitioning into a terminal state must not touch the shared tool registry")

	exec.UnregisterRun("run-a")
	exec.UnregisterRun("run-b")
	require.Equal(t, before, snapshotRegistry(t, registry),
		"unregistering a run must not touch the shared tool registry")
}

// commit drives a run through the real workflow__transition path: the tool
// executor defers the event, then the commit fires the executor's OnCommit hook
// and everything hung off it.
//
// Calling StateMachine.ProcessEvent directly would be simpler and useless — it
// bypasses applyPostCommit, which is the function that used to mutate the
// registry. A guard that never reaches the mutating code cannot catch it.
func commit(t *testing.T, exec *workflowTransitionExecutor, runID, event string) {
	t.Helper()
	desc := exec.registry.Get(workflow.TransitionToolName)
	require.NotNil(t, desc, "transition tool should be registered")

	ctx := withWorkflowScenarioID(context.Background(), runID)
	args := []byte(`{"event":"` + event + `","context":"test"}`)
	_, err := exec.Execute(ctx, desc, args)
	require.NoError(t, err, "deferring event %q for run %q", event, runID)

	require.NoError(t, exec.CommitPendingTransition(runID, nil),
		"committing event %q for run %q", event, runID)
}

// Each run's legal events come from its own state machine, not from the shared
// descriptor. This is why the descriptor can safely advertise the union: the
// narrow check still happens, per run, where the run's state actually lives.
func TestEventValidityIsPerRunNotPerRegistry(t *testing.T) {
	registry := tools.NewRegistry()
	spec := twoStateSpec()
	registerTransitionToolForSpec(registry, spec)

	exec := newWorkflowTransitionExecutor(spec, registry)
	exec.RegisterRunAtState("in-triage", &arenaconfig.Scenario{ID: "a"}, nil, "triage")
	exec.RegisterRunAtState("in-specialist", &arenaconfig.Scenario{ID: "b"}, nil, "specialist")

	// "Resolve" belongs to specialist. The run sitting in triage must reject it
	// even though the shared descriptor lists it as a known event.
	_, err := exec.StateMachine("in-triage").ProcessEvent("Resolve")
	require.Error(t, err, "triage must reject an event that belongs to another state")

	// ...and the run that IS in specialist accepts the same event.
	_, err = exec.StateMachine("in-specialist").ProcessEvent("Resolve")
	require.NoError(t, err, "specialist must accept its own event")
}
