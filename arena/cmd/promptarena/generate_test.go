package main

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newGenerateTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "generate", RunE: runGenerate}
	registerGenerateFlags(cmd.Flags())
	return cmd
}

func TestResolveAdapter_FromRecordings(t *testing.T) {
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("from-recordings", "*.json"))
	adapter, err := resolveAdapter(cmd)
	require.NoError(t, err)
	assert.Equal(t, "recordings", adapter.Name())
}

func TestResolveAdapter_FromSource(t *testing.T) {
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("source", "nonexistent"))
	_, err := resolveAdapter(cmd)
	require.Error(t, err)
}

func TestResolveAdapter_NeitherFlag(t *testing.T) {
	_, err := resolveAdapter(newGenerateTestCmd())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specify either --source or --from-recordings")
}

func TestBuildRequest_Defaults(t *testing.T) {
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("from-recordings", "*.json"))
	req, err := buildRequest(cmd)
	require.NoError(t, err)
	assert.Nil(t, req.List.FilterPassed)
	assert.Empty(t, req.List.Expectations)
	assert.True(t, req.Dedup)
	assert.Equal(t, "", req.Convert.TaskType)
	assert.Nil(t, req.Pack)
}

func TestBuildRequest_Flags(t *testing.T) {
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("from-recordings", "*.json"))
	require.NoError(t, cmd.Flags().Set("filter-passed", "false"))
	require.NoError(t, cmd.Flags().Set("filter-eval-type", "content_matches"))
	require.NoError(t, cmd.Flags().Set("task-type", "intake"))
	require.NoError(t, cmd.Flags().Set("dedup", "false"))
	require.NoError(t, cmd.Flags().Set("expect", "faithfulness>=0.8"))
	require.NoError(t, cmd.Flags().Set("expect", "toxicity<=0.2"))

	req, err := buildRequest(cmd)
	require.NoError(t, err)
	require.NotNil(t, req.List.FilterPassed)
	assert.False(t, *req.List.FilterPassed)
	assert.Equal(t, "content_matches", req.List.FilterEvalType)
	assert.Equal(t, "intake", req.Convert.TaskType)
	assert.False(t, req.Dedup)
	require.Len(t, req.List.Expectations, 2)
	assert.Equal(t, "faithfulness", req.List.Expectations[0].EvalID)
	assert.Equal(t, 0.8, *req.List.Expectations[0].Min)
	assert.Equal(t, 0.2, *req.List.Expectations[1].Max)
}

func TestBuildRequest_BadExpectation(t *testing.T) {
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("from-recordings", "*.json"))
	require.NoError(t, cmd.Flags().Set("expect", "faithfulness"))
	_, err := buildRequest(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "faithfulness")
}

func TestBuildRequest_DeprecatedPackAlias(t *testing.T) {
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("from-recordings", "*.json"))
	require.NoError(t, cmd.Flags().Set("pack", "legacy"))
	req, err := buildRequest(cmd)
	require.NoError(t, err)
	assert.Equal(t, "legacy", req.Convert.TaskType)
}

// End to end over the tracked run-output fixture: the CLI body produces a
// scenario file and prints the decisions.
func TestRunGenerate_WritesScenarios(t *testing.T) {
	out := t.TempDir()
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("from-recordings", filepath.Join("..", "..", "adapters", "testdata", "tool-usage.run.json")))
	require.NoError(t, cmd.Flags().Set("output", out))

	require.NoError(t, runGenerate(cmd, nil))

	matches, err := filepath.Glob(filepath.Join(out, "*.scenario.yaml"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
}
