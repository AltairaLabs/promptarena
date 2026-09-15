package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// newSkillsTestExecutor discovers one skill and returns the registry-resident
// skills executor plus a tool registry with the skill__ tools registered.
func newSkillsTestExecutor(t *testing.T, packTools []string) (*skillsToolExecutor, *tools.Registry) {
	t.Helper()
	dir := writeSkill(t, billingSkillDoc)

	reg := skills.NewRegistry()
	require.NoError(t, reg.Discover([]skills.SkillSource{{Dir: dir}}))

	ste := newSkillsToolExecutor(skills.ExecutorConfig{
		Registry:  reg,
		PackTools: packTools,
		ConfigDir: dir,
	}, reg.PreloadedSkills())

	toolReg := tools.NewRegistry()
	_ = toolReg.Register(skills.BuildSkillActivateDescriptor())
	_ = toolReg.Register(skills.BuildSkillDeactivateDescriptor())
	toolReg.RegisterExecutor(ste)
	return ste, toolReg
}

// activateBilling calls skill__activate as the named run and fails the test if
// the tool reported an error.
func activateBilling(t *testing.T, toolReg *tools.Registry, runID string) {
	t.Helper()
	args, err := json.Marshal(map[string]any{"name": "billing"})
	require.NoError(t, err)
	res, err := toolReg.Execute(withRunID(t.Context(), runID), skills.SkillActivateTool, args)
	require.NoError(t, err)
	require.Empty(t, res.Error)
}

func TestSkillsToolExecutor_ActivationIsVisibleToTheActivatingRun(t *testing.T) {
	ste, toolReg := newSkillsTestExecutor(t, []string{"issue_refund", "lookup_order"})
	ste.RegisterRun("run-a")

	activateBilling(t, toolReg, "run-a")

	assert.Equal(t, []string{"issue_refund"}, ste.GrantsFor("run-a")())
}

func TestSkillsToolExecutor_ActivationDoesNotLeakToAnotherRun(t *testing.T) {
	ste, toolReg := newSkillsTestExecutor(t, []string{"issue_refund", "lookup_order"})
	ste.RegisterRun("run-a")
	ste.RegisterRun("run-b")

	activateBilling(t, toolReg, "run-a")

	assert.Empty(t, ste.GrantsFor("run-b")(),
		"a skill activated in run A must not grant its tools in run B")
}

func TestSkillsToolExecutor_UnknownRunIsAnError(t *testing.T) {
	_, toolReg := newSkillsTestExecutor(t, []string{"issue_refund"})

	args, err := json.Marshal(map[string]any{"name": "billing"})
	require.NoError(t, err)
	res, err := toolReg.Execute(withRunID(t.Context(), "nobody"), skills.SkillActivateTool, args)
	require.NoError(t, err)
	assert.Contains(t, res.Error, `no registered run "nobody"`)
}

func TestSkillsToolExecutor_NoRunIdentityIsAnError(t *testing.T) {
	ste, toolReg := newSkillsTestExecutor(t, []string{"issue_refund"})
	ste.RegisterRun("run-a")

	args, err := json.Marshal(map[string]any{"name": "billing"})
	require.NoError(t, err)
	res, err := toolReg.Execute(t.Context(), skills.SkillActivateTool, args)
	require.NoError(t, err)
	assert.Contains(t, res.Error, "no run identity on context")
}

func TestSkillsToolExecutor_GrantsForUnknownRunIsEmpty(t *testing.T) {
	ste, _ := newSkillsTestExecutor(t, []string{"issue_refund"})
	assert.Empty(t, ste.GrantsFor("nobody")())
}

func TestSkillsToolExecutor_UnregisterRunDropsActiveSkills(t *testing.T) {
	ste, toolReg := newSkillsTestExecutor(t, []string{"issue_refund"})
	ste.RegisterRun("run-a")
	activateBilling(t, toolReg, "run-a")

	ste.UnregisterRun("run-a")
	assert.Empty(t, ste.GrantsFor("run-a")())
}

func TestSkillsToolExecutor_GrantsAreCappedByPackTools(t *testing.T) {
	// The skill asks for a tool the pack does not declare.
	dir := writeSkill(t, `---
name: billing
description: Billing operations.
allowed-tools: [issue_refund, drop_database]
---

Body.
`)

	reg := skills.NewRegistry()
	require.NoError(t, reg.Discover([]skills.SkillSource{{Dir: dir}}))

	ste := newSkillsToolExecutor(skills.ExecutorConfig{
		Registry:  reg,
		PackTools: []string{"issue_refund"},
		ConfigDir: dir,
	}, reg.PreloadedSkills())

	toolReg := tools.NewRegistry()
	_ = toolReg.Register(skills.BuildSkillActivateDescriptor())
	toolReg.RegisterExecutor(ste)
	ste.RegisterRun("run-a")

	activateBilling(t, toolReg, "run-a")

	assert.Equal(t, []string{"issue_refund"}, ste.GrantsFor("run-a")(),
		"a skill must not grant a tool the pack does not declare")
}
