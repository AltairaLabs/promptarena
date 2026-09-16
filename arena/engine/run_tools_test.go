package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/memory"
	"github.com/AltairaLabs/PromptKit/runtime/v2/skills"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
)

const billingSkillDoc = `---
name: billing
description: Billing operations.
allowed-tools: [issue_refund]
---

Use issue_refund to refund a customer.
`

// writeSkill writes a single SKILL.md under a fresh temp dir and returns the
// dir to discover from.
func writeSkill(t *testing.T, doc string) string {
	t.Helper()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "billing")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(doc), 0o644))
	return dir
}

// newSkillsEngine builds an engine whose skills factory can serve runs, with
// the skill__ descriptors registered engine-wide as the builder does.
func newSkillsEngine(t *testing.T, doc string, packTools []string) *Engine {
	t.Helper()
	dir := writeSkill(t, doc)

	reg := skills.NewRegistry()
	require.NoError(t, reg.Discover([]skills.SkillSource{{Dir: dir}}))

	parent := tools.NewRegistry()
	_ = parent.Register(skills.BuildSkillActivateDescriptor())
	_ = parent.Register(skills.BuildSkillDeactivateDescriptor())

	exec := skills.NewExecutor(skills.ExecutorConfig{
		Registry:  reg,
		PackTools: packTools,
		ConfigDir: dir,
	})
	parent.RegisterExecutor(skills.NewToolExecutor(exec))

	return &Engine{
		toolRegistry:  parent,
		skillsFactory: &SkillsFactory{executor: exec, catalog: reg, preloaded: reg.PreloadedSkills()},
	}
}

// activateBilling calls skill__activate through the run's registry, with the
// run's own ActiveSet on the context — the live dispatch path.
func activateBilling(t *testing.T, rt *runTools) {
	t.Helper()
	args, err := json.Marshal(map[string]any{"name": "billing"})
	require.NoError(t, err)
	res, err := rt.registry.Execute(rt.bindContext(t.Context()), skills.SkillActivateTool, args)
	require.NoError(t, err)
	require.Empty(t, res.Error)
}

// The property this whole change exists for: a skill activated in one run must
// not grant its tools in another. Each run owns its ActiveSet and the executor
// holds no conversation state, so this is structural rather than something a
// run-keyed lookup has to get right.
func TestRunTools_SkillActivationDoesNotLeakBetweenRuns(t *testing.T) {
	eng := newSkillsEngine(t, billingSkillDoc, []string{"issue_refund", "lookup_order"})

	runA := eng.buildRunTools("scenario-a", "run-a")
	runB := eng.buildRunTools("scenario-b", "run-b")

	activateBilling(t, runA)

	assert.Equal(t, []string{"issue_refund"}, eng.toolGrants(runA)(),
		"the activating run must see its grant")
	assert.Empty(t, eng.toolGrants(runB)(),
		"a skill activated in run A must not grant its tools in run B")
}

func TestRunTools_GrantsAreCappedByTheCeiling(t *testing.T) {
	// The skill asks for a tool the config never declares.
	eng := newSkillsEngine(t, `---
name: billing
description: Billing operations.
allowed-tools: [issue_refund, drop_database]
---

Body.
`, []string{"issue_refund"})

	rt := eng.buildRunTools("scenario-a", "run-a")
	activateBilling(t, rt)

	assert.Equal(t, []string{"issue_refund"}, eng.toolGrants(rt)(),
		"a skill must not grant a tool outside the ceiling")
}

func TestRunTools_NoSkillsMeansNoGrantsAccessor(t *testing.T) {
	eng := &Engine{toolRegistry: tools.NewRegistry()}
	rt := eng.buildRunTools("scenario-a", "run-a")
	assert.Nil(t, eng.toolGrants(rt), "runs without skills must not install an accessor")
}

// Memory is scoped the same way: the scope is the run's, carried on its
// context, rather than captured on an executor a shared registry holds one of.
func TestRunTools_MemoryIsScopedPerRun(t *testing.T) {
	store := memory.NewInMemoryStore()
	parent := tools.NewRegistry()
	// Mirrors initMemory: one executor engine-wide, no scope of its own.
	parent.RegisterExecutor(memory.NewExecutor(store, nil))
	memory.RegisterMemoryTools(parent)
	eng := &Engine{toolRegistry: parent, memoryStore: store}

	runA := eng.buildRunTools("scenario-a", "run-a")
	runB := eng.buildRunTools("scenario-b", "run-b")

	args, err := json.Marshal(map[string]any{"content": "A's secret"})
	require.NoError(t, err)
	res, err := runA.registry.Execute(runA.bindContext(t.Context()), memory.RememberToolName, args)
	require.NoError(t, err)
	require.Empty(t, res.Error)

	inA, err := store.List(t.Context(), runA.memoryScope, memory.ListOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, inA, 1)
	assert.Equal(t, "A's secret", inA[0].Content)

	inB, err := store.List(t.Context(), runB.memoryScope, memory.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, inB, "run A's write must not be visible in run B's scope")
}

func TestRunTools_NoMemoryMeansNoScope(t *testing.T) {
	eng := &Engine{toolRegistry: tools.NewRegistry()}
	rt := eng.buildRunTools("scenario-a", "run-a")
	assert.Nil(t, rt.memoryScope)
}

// A run's registry must still see the engine's descriptors and fall through to
// its executors — that is what makes the child safe to use for the whole turn.
func TestRunTools_ChildSeesParentToolsAndExecutors(t *testing.T) {
	parent := tools.NewRegistry()
	memory.RegisterMemoryTools(parent)
	eng := &Engine{toolRegistry: parent}

	rt := eng.buildRunTools("scenario-a", "run-a")

	assert.Contains(t, rt.registry.GetTools(), memory.RememberToolName,
		"the run must see tools registered on the engine registry")
}

// The HTTP response budget lives on the run's own executor, so one run cannot
// spend another's. Registering it per run is the whole mechanism.
func TestRunTools_EachRunGetsItsOwnHTTPExecutor(t *testing.T) {
	eng := &Engine{toolRegistry: tools.NewRegistry()}

	runA := eng.buildRunTools("scenario-a", "run-a")
	runB := eng.buildRunTools("scenario-b", "run-b")

	assert.NotSame(t, runA.registry, runB.registry)
}

// preload: true skills must be active from turn 1 in every run, and replaying
// them into each run's own set is what keeps that from being shared state.
func TestRunTools_PreloadedSkillsAreActivePerRun(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "billing")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: billing
description: Billing operations.
allowed-tools: [issue_refund]
---

Body.
`), 0o644))

	reg := skills.NewRegistry()
	require.NoError(t, reg.Discover([]skills.SkillSource{{Dir: dir, Preload: true}}))
	require.NotEmpty(t, reg.PreloadedSkills(), "fixture must declare a preloaded skill")

	exec := skills.NewExecutor(skills.ExecutorConfig{
		Registry:  reg,
		PackTools: []string{"issue_refund"},
		ConfigDir: dir,
	})
	eng := &Engine{
		toolRegistry:  tools.NewRegistry(),
		skillsFactory: &SkillsFactory{executor: exec, catalog: reg, preloaded: reg.PreloadedSkills()},
	}

	runA := eng.buildRunTools("scenario-a", "run-a")
	runB := eng.buildRunTools("scenario-b", "run-b")

	assert.Equal(t, []string{"issue_refund"}, eng.toolGrants(runA)(),
		"a preloaded skill grants its tools without an explicit activation")
	assert.Equal(t, []string{"issue_refund"}, eng.toolGrants(runB)(),
		"every run gets the preload replayed into its own set")

	// The sets are distinct: deactivating in one must not affect the other.
	_, err := exec.DeactivateIn(runA.activeSet, "billing")
	require.NoError(t, err)
	assert.Empty(t, eng.toolGrants(runA)())
	assert.Equal(t, []string{"issue_refund"}, eng.toolGrants(runB)(),
		"run B keeps its preloaded skill after run A drops its own")
}
