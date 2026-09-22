package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/templates"
)

// remoteURL matches a URL a scenario would fetch at run time. The
// yaml-language-server $schema modeline is editor metadata, never fetched.
var remoteURL = regexp.MustCompile(`(?m)^[^#]*https?://`)

// TestBuiltInTemplates_RunGreenAgainstMock scaffolds every built-in template with
// the mock provider and runs it headlessly. `init` then `run` is the first thing
// a new user or coding agent does, so every template must come up green, offline,
// with no API key. Templates are discovered from the embedded set, so a new one
// is covered without touching this test.
func TestBuiltInTemplates_RunGreenAgainstMock(t *testing.T) {
	// init binds these package globals via cobra StringVar/BoolVar; Execute mutates
	// them, so save and restore to keep the test hermetic for sibling tests.
	savedTemplate, savedProvider, savedOutput := initTemplate, initProvider, initOutputDir
	savedQuick, savedNoGit, savedNoEnv := initQuick, initNoGit, initNoEnv
	t.Cleanup(func() {
		initTemplate, initProvider, initOutputDir = savedTemplate, savedProvider, savedOutput
		initQuick, initNoGit, initNoEnv = savedQuick, savedNoGit, savedNoEnv
		rootCmd.SetArgs(nil)
	})

	// A template that quietly wires a real provider would pass on a machine with
	// keys set. Clear them so it fails here instead.
	for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "CLAUDE_API_KEY",
		"GEMINI_API_KEY", "GOOGLE_API_KEY"} {
		t.Setenv(key, "")
	}

	builtIns, err := templates.NewLoader(t.TempDir()).ListBuiltIn()
	require.NoError(t, err)
	require.NotEmpty(t, builtIns)

	for _, info := range builtIns {
		t.Run(info.Name, func(t *testing.T) {
			dir := t.TempDir()
			rootCmd.SetArgs([]string{"init", "kit",
				"--template", info.Name, "--provider", "mock", "--quick", "--no-env", "--no-git",
				"--output", dir})
			require.NoError(t, rootCmd.Execute(), "init must scaffold the kit")

			projectDir := filepath.Join(dir, "kit")
			cfg := filepath.Join(projectDir, "config.arena.yaml")
			require.FileExists(t, cfg)

			scenarios, err := filepath.Glob(filepath.Join(projectDir, "scenarios", "*.yaml"))
			require.NoError(t, err)
			for _, s := range scenarios {
				data, err := os.ReadFile(s)
				require.NoError(t, err)
				assert.False(t, remoteURL.Match(data),
					"%s fetches a remote URL at run time; ship the fixture in the template instead", filepath.Base(s))
			}

			outDir := filepath.Join(projectDir, "out")
			rootCmd.SetArgs([]string{"run", "--config", cfg, "--ci", "--out", outDir, "--formats", "json"})
			require.NoError(t, rootCmd.Execute(), "a fresh %s kit must pass against its mock", info.Name)
		})
	}
}
