package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runInitForTest runs `promptarena init kit --quick ...` into dir and restores
// the init flag globals, which cobra binds and never resets, afterwards.
func runInitForTest(t *testing.T, dir string, extra ...string) {
	t.Helper()
	savedTemplate, savedProvider, savedOutput := initTemplate, initProvider, initOutputDir
	savedQuick, savedNoEnv, savedForce, savedNoAgent := initQuick, initNoEnv, initForce, initNoAgent
	t.Cleanup(func() {
		initTemplate, initProvider, initOutputDir = savedTemplate, savedProvider, savedOutput
		initQuick, initNoEnv, initForce, initNoAgent = savedQuick, savedNoEnv, savedForce, savedNoAgent
		_ = initCmd.Flags().Set("force", "false")
		_ = initCmd.Flags().Set("no-env", "false")
		rootCmd.SetArgs(nil)
	})
	args := append([]string{"init", "kit", "--template", "quick-start", "--quick", "--no-agent",
		"--output", dir}, extra...)
	rootCmd.SetArgs(args)
	require.NoError(t, rootCmd.Execute())
}

// init into an existing project used to overwrite hand-edited files silently.
func TestInit_KeepsExistingFilesWithoutForce(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "kit")
	require.NoError(t, os.MkdirAll(project, 0o750))
	readme := filepath.Join(project, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("my notes"), 0o600))

	runInitForTest(t, dir, "--provider", "mock")

	data, err := os.ReadFile(readme)
	require.NoError(t, err)
	assert.Equal(t, "my notes", string(data), "an existing file must not be overwritten without --force")
	assert.FileExists(t, filepath.Join(project, "config.arena.yaml"), "missing files are still created")
}

func TestInit_ForceOverwritesExistingFiles(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "kit")
	require.NoError(t, os.MkdirAll(project, 0o750))
	readme := filepath.Join(project, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("my notes"), 0o600))

	runInitForTest(t, dir, "--provider", "mock", "--force")

	data, err := os.ReadFile(readme)
	require.NoError(t, err)
	assert.NotEqual(t, "my notes", string(data))
}

// promptarena never loads a .env file, so init writes .env.example (which
// says how to export the keys) instead of a .env that silently does nothing.
func TestInit_WritesEnvExampleNotEnv(t *testing.T) {
	dir := t.TempDir()
	runInitForTest(t, dir, "--provider", "openai")

	project := filepath.Join(dir, "kit")
	assert.NoFileExists(t, filepath.Join(project, ".env"))
	data, err := os.ReadFile(filepath.Join(project, ".env.example"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "OPENAI_API_KEY")
	assert.Contains(t, string(data), "does not load this file")
}

// --no-git never did anything. It is kept (so scripts passing it still work)
// but deprecated, which also hides it from --help.
func TestInit_NoGitFlagIsDeprecated(t *testing.T) {
	f := initCmd.Flags().Lookup("no-git")
	require.NotNil(t, f)
	assert.NotEmpty(t, f.Deprecated)
	assert.True(t, f.Hidden)
}
