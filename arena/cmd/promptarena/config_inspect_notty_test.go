package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/inspect"
)

// writeInspectKit writes a minimal valid kit (the shape `promptarena init`
// produces) and returns its config path. scenarioTaskType lets a test break
// the scenario → prompt cross-reference.
func writeInspectKit(t *testing.T, scenarioTaskType string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"config.arena.yaml": `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: notty
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.prompt.yaml
  providers:
    - file: providers/mock.provider.yaml
  scenarios:
    - file: scenarios/basic.scenario.yaml
`,
		"prompts/assistant.prompt.yaml": `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: assistant
spec:
  task_type: general
  version: "1.0.0"
  system_template: You are a helpful assistant.
`,
		"providers/mock.provider.yaml": `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: mock
spec:
  id: mock-provider
  type: mock
  model: mock-model
`,
		"scenarios/basic.scenario.yaml": `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: basic
spec:
  id: basic
  task_type: ` + scenarioTaskType + `
  turns:
    - role: user
      content: Hello
`,
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	return filepath.Join(dir, "config.arena.yaml")
}

// runInspectWithoutTTY runs config-inspect down the non-interactive path, as
// it runs in CI or behind a pipe, and returns what it printed and its error.
func runInspectWithoutTTY(t *testing.T, configPath string, short bool) (string, error) {
	t.Helper()
	origInteractive, origShort, origSection, origFormat := inspectIsInteractive, inspectShort, inspectSection, inspectFormat
	t.Cleanup(func() {
		inspectIsInteractive, inspectShort, inspectSection, inspectFormat = origInteractive, origShort, origSection, origFormat
	})
	inspectIsInteractive = func() bool { return false }
	inspectShort, inspectSection, inspectFormat = short, "", inspectFormatText

	cmd := &cobra.Command{}
	cmd.Flags().StringP("config", "c", configPath, "")
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := runConfigInspect(cmd, nil)
	return out.String(), err
}

func TestConfigInspect_WithoutTTY_PrintsInspectionAndSucceeds(t *testing.T) {
	out, err := runInspectWithoutTTY(t, writeInspectKit(t, "general"), false)

	require.NoError(t, err)
	assert.Contains(t, out, "Configuration: ")
	assert.Contains(t, out, "assistant", "the prompt config section should list the prompt")
	assert.Contains(t, out, "mock-provider", "the providers section should list the provider")
	assert.Contains(t, out, "Configuration is valid")
	assert.NotContains(t, out, "\x1b[", "output without a terminal should carry no ANSI escapes")
}

func TestConfigInspect_WithoutTTY_InvalidConfigFailsWithErrorsPrinted(t *testing.T) {
	out, err := runInspectWithoutTTY(t, writeInspectKit(t, "no-such-task"), false)

	require.ErrorIs(t, err, errConfigInspectInvalid)
	assert.Contains(t, out, "unknown task_type: no-such-task")
}

func TestInspectIsInteractive_FalseWhenStdinIsAPipe(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	assert.False(t, inspectIsInteractive(), "a pipe on stdin is not a terminal, so the TUI must not start")
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestPrintInspectText_ReportsWriteFailure(t *testing.T) {
	err := printInspectText(failingWriter{}, &InspectionData{ConfigFile: "x.yaml", ValidationPassed: true},
		inspect.RenderOptions{})

	require.EqualError(t, err, "disk full")
}

func TestConfigInspect_WithoutTTY_ShortShowsOnlyValidation(t *testing.T) {
	out, err := runInspectWithoutTTY(t, writeInspectKit(t, "general"), true)

	require.NoError(t, err)
	assert.Contains(t, out, "Configuration is valid")
	assert.NotContains(t, out, "Prompt Configs", "--short should render the validation section alone")
	assert.False(t, strings.Contains(out, "File: prompts/"), "--short should not list prompt files")
}
