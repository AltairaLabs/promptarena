package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/arena/generate"
)

func newGenerateTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "generate", RunE: runGenerate}
	registerGenerateFlags(cmd.Flags())
	return cmd
}

// buildRequestFromFlags resolves the source the way runGenerate does, then
// builds the request. Tests below only ever use --from-recordings here.
func buildRequestFromFlags(t *testing.T, cmd *cobra.Command) (generate.Request, error) {
	t.Helper()
	source, closer, err := resolveSource(context.Background(), cmd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closer() })
	return buildRequest(cmd, source)
}

func TestResolveSource_FromRecordings(t *testing.T) {
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("from-recordings", "*.json"))
	adapter, closer, err := resolveSource(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, "recordings", adapter.Name())
	assert.NoError(t, closer())
}

// An unknown --source falls through the registry to the deploy adapters, and
// a missing adapter is explained with the install command. The workspace flag
// rides along and must not change that outcome.
func TestResolveSource_UnknownAdapterExplainsInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Chdir(dir)
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("source", "nonexistent"))
	require.NoError(t, cmd.Flags().Set("workspace", "demo"))
	_, _, err := resolveSource(context.Background(), cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "promptarena deploy adapter install nonexistent")
}

type inprocSource struct{}

func (inprocSource) Name() string { return "inproc" }
func (inprocSource) List(context.Context, generate.ListOptions) ([]generate.SessionSummary, error) {
	return nil, nil
}
func (inprocSource) Get(context.Context, string) (*generate.SessionDetail, error) { return nil, nil }

// A source registered in-process by a Go caller wins over the deploy adapters.
func TestResolveSource_RegistryWins(t *testing.T) {
	generateRegistry.Register(inprocSource{})
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("source", "inproc"))
	adapter, closer, err := resolveSource(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, "inproc", adapter.Name())
	assert.NoError(t, closer())
}

func TestResolveSource_NeitherFlag(t *testing.T) {
	_, _, err := resolveSource(context.Background(), newGenerateTestCmd())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specify either --source or --from-recordings")
}

func TestBuildRequest_Defaults(t *testing.T) {
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("from-recordings", "*.json"))
	req, err := buildRequestFromFlags(t, cmd)
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

	req, err := buildRequestFromFlags(t, cmd)
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
	_, err := buildRequestFromFlags(t, cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "faithfulness")
}

func TestBuildRequest_DeprecatedPackAlias(t *testing.T) {
	cmd := newGenerateTestCmd()
	require.NoError(t, cmd.Flags().Set("from-recordings", "*.json"))
	require.NoError(t, cmd.Flags().Set("pack", "legacy"))
	req, err := buildRequestFromFlags(t, cmd)
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
