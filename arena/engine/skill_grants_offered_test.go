package engine

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"
)

// runWorkflowSkillsScenario runs one scenario of the examples/workflow-skills
// kit rooted at dir through the real engine and returns its stored result.
func runWorkflowSkillsScenario(t *testing.T, dir, scenario string) *statestore.RunResult {
	t.Helper()
	eng, err := NewEngineFromConfigFile(filepath.Join(dir, "config.arena.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close() })

	plan := &RunPlan{Combinations: []RunCombination{
		{Region: "default", ScenarioID: scenario, ProviderID: "mock-shopco"},
	}}
	runIDs, err := eng.ExecuteRuns(context.Background(), plan, 1)
	require.NoError(t, err)
	require.Len(t, runIDs, 1)

	store, ok := eng.GetStateStore().(*statestore.ArenaStateStore)
	require.True(t, ok)
	result, err := store.GetResult(context.Background(), runIDs[0])
	require.NoError(t, err)
	return result
}

// offeredPerAssistantMessage returns, for each assistant message, whether it
// recorded being offered tool.
func offeredPerAssistantMessage(msgs []types.Message, tool string) []bool {
	var out []bool
	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		offered, _ := m.Meta[types.MetaToolsOffered].([]string)
		out = append(out, slices.Contains(offered, tool))
	}
	return out
}

// A skill grant followed through a live run into the provider's tools array:
// refund is not offered until pci-compliance is activated, stays offered while
// the conversation is in billing, and goes when it moves to closed, whose
// `skills: none` leaves no skill active. The scenario's tools_offered
// assertions pass.
func TestSkillGrants_OfferedOnlyWhileGranted(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "workflow-skills")
	result := runWorkflowSkillsScenario(t, dir, "billing-flow")
	require.Empty(t, result.Error, "the example's tools_offered assertions must pass")

	offered := offeredPerAssistantMessage(result.Messages, "refund")
	first := slices.Index(offered, true)
	require.Positive(t, first, "refund must be absent at first and offered later: %v", offered)
	last := first
	for i := range offered {
		if offered[i] {
			last = i
		}
	}
	assert.NotContains(t, offered[first:last+1], false, "the grant must hold without gaps: %v", offered)
	assert.False(t, offered[len(offered)-1], "closed has no skills, so refund must be gone: %v", offered)
}

// The same run with the grant removed fails on the scenario's assertion, which
// is what tools_called could never do against a mock: the mock still emits the
// scripted calls, but the tools array no longer holds refund.
func TestSkillGrants_MissingGrantFailsToolsOffered(t *testing.T) {
	dir := copyExample(t, filepath.Join("..", "..", "examples", "workflow-skills"))
	skill := filepath.Join(dir, "skills", "billing", "pci-compliance", "SKILL.md")
	b, err := os.ReadFile(skill)
	require.NoError(t, err)
	stripped := strings.Replace(string(b), "allowed-tools:\n  - refund\n", "", 1)
	require.NotEqual(t, string(b), stripped, "fixture drifted: pci-compliance no longer grants refund")
	require.NoError(t, os.WriteFile(skill, []byte(stripped), 0o600))

	result := runWorkflowSkillsScenario(t, dir, "billing-flow")
	assert.Contains(t, result.Error, "tools not offered: refund")
}

// copyExample copies an example directory into a temp dir, leaving out run output.
func copyExample(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "out" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o750)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o600)
	})
	require.NoError(t, err)
	return dst
}
