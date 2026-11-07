package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHubRunWritesResultFiles reproduces #428: a run started from the hub's
// Run page kept its results only in memory, while `promptarena run` writes the
// configured report formats. The hub context's SaveRunResults must write the
// same files `run` would for a config with no output settings (json, under
// the config's out/ directory).
func TestHubRunWritesResultFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "tui", "app", "testdata", "run-config"))))

	// The engine's media store writes under the working directory; keep it in
	// the temp dir rather than the package source tree.
	t.Chdir(t.TempDir())

	var warn bytes.Buffer
	ctx := buildHubContext(dir, &warn)
	require.Empty(t, warn.String())
	require.NotNil(t, ctx.SaveRunResults, "the hub must give its Run page a results writer")

	eng, err := ctx.EnsureEngine()
	require.NoError(t, err)
	require.NoError(t, eng.EnableMockProviderMode(""))
	plan, err := eng.GenerateRunPlan(nil, nil, nil, nil)
	require.NoError(t, err)
	runIDs, err := eng.ExecuteRuns(context.Background(), plan, 1)
	require.NoError(t, err)
	require.NotEmpty(t, runIDs)

	require.NoError(t, ctx.SaveRunResults(eng, runIDs))

	assert.FileExists(t, filepath.Join(dir, "out", "index.json"))
	for _, id := range runIDs {
		assert.FileExists(t, filepath.Join(dir, "out", id+".json"))
	}
}
