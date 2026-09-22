package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuickStartScaffold_RunsAgainstMock scaffolds the quick-start template with
// the mock provider and runs it headlessly. The mock provider never contacts a
// real LLM, so this is safe for CI at zero API cost. It proves the scaffold
// executes end-to-end, produces a report, and passes: the scaffold ships scripted
// mock replies, so a fresh `init` followed by `run` must be green.
func TestQuickStartScaffold_RunsAgainstMock(t *testing.T) {
	// init binds these package globals via cobra StringVar/BoolVar; Execute mutates
	// them, so save and restore to keep the test hermetic for sibling tests.
	savedTemplate, savedProvider, savedOutput := initTemplate, initProvider, initOutputDir
	savedQuick, savedNoGit, savedNoEnv := initQuick, initNoGit, initNoEnv
	t.Cleanup(func() {
		initTemplate, initProvider, initOutputDir = savedTemplate, savedProvider, savedOutput
		initQuick, initNoGit, initNoEnv = savedQuick, savedNoGit, savedNoEnv
		rootCmd.SetArgs(nil)
	})

	dir := t.TempDir()

	rootCmd.SetArgs([]string{"init", "mockkit",
		"--template", "quick-start", "--provider", "mock", "--quick", "--no-env", "--no-git",
		"--output", dir})
	require.NoError(t, rootCmd.Execute(), "init must scaffold the kit")

	projectDir := filepath.Join(dir, "mockkit")
	cfg := filepath.Join(projectDir, "config.arena.yaml")
	require.FileExists(t, cfg)

	outDir := filepath.Join(projectDir, "out")
	rootCmd.SetArgs([]string{"run", "--config", cfg, "--ci", "--out", outDir, "--formats", "json"})
	require.NoError(t, rootCmd.Execute(), "a fresh quick-start kit must pass against its mock")

	require.DirExists(t, outDir)
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	var hasJSON bool
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			hasJSON = true
		}
	}
	assert.True(t, hasJSON, "run should produce a JSON report in %s", outDir)
}
