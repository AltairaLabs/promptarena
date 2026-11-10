package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

// writeFragmentProject writes an arena config whose prompt, in prompts/,
// references ../fragments/tone.yaml, and returns the arena config's path.
func writeFragmentProject(t *testing.T, required bool, withFragment bool) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	req := "false"
	if required {
		req = "true"
	}
	write("prompts/support.yaml", `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: support
spec:
  task_type: support
  version: v1.0.0
  description: Support prompt
  system_template: "You help customers. {{tone}}"
  fragments:
    - name: tone
      path: ../fragments/tone.yaml
      required: `+req+`
`)
	if withFragment {
		write("fragments/tone.yaml", `fragment_type: guidelines
version: v1.0.0
description: House tone
content: "Answer in plain, friendly English."
`)
	}
	write("config.arena.yaml", `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
    - id: support
      file: prompts/support.yaml
  providers: []
  defaults:
    temperature: 0.7
    max_tokens: 100
`)
	return filepath.Join(dir, "config.arena.yaml")
}

// promptarena run resolves a relative fragment path against the prompt
// config's directory, as packc compile does.
func TestBuildPromptRegistry_ResolvesFragmentFromPromptDir(t *testing.T) {
	cfg, err := arenaconfig.LoadConfig(writeFragmentProject(t, true, true))
	require.NoError(t, err)

	registry, err := buildPromptRegistry(cfg)
	require.NoError(t, err)
	require.NotNil(t, registry)

	assembled := registry.Load("support")
	require.NotNil(t, assembled)
	assert.Equal(t, "You help customers. Answer in plain, friendly English.", assembled.SystemPrompt)
}

func TestBuildPromptRegistry_RequiredFragmentMissing(t *testing.T) {
	cfg, err := arenaconfig.LoadConfig(writeFragmentProject(t, true, false))
	require.NoError(t, err)

	_, err = buildPromptRegistry(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `required fragment "tone"`)
	assert.Contains(t, err.Error(), filepath.Join("fragments", "tone.yaml"))
}

func TestBuildPromptRegistry_OptionalFragmentMissingStillBuilds(t *testing.T) {
	cfg, err := arenaconfig.LoadConfig(writeFragmentProject(t, false, false))
	require.NoError(t, err)

	registry, err := buildPromptRegistry(cfg)
	require.NoError(t, err)

	// A missing optional fragment renders empty (PromptKit#2523), so the
	// prompt still loads and its placeholder is gone.
	assembled := registry.Load("support")
	require.NotNil(t, assembled, "the prompt did not load without its optional fragment")
	assert.Equal(t, "You help customers. ", assembled.SystemPrompt)
}
