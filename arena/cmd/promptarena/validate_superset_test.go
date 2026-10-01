package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/packc/compiler"
)

const supersetPromptYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: greeting
spec:
  task_type: greeting
  version: v1.0.0
  description: Greets the user
  system_template: You are a friendly assistant.
`

// writeSupersetKit writes a prompt and an arena config whose spec ends with
// extra (indented under spec:), returning the arena config's path.
func writeSupersetKit(t *testing.T, extra string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "prompts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prompts", "greeting.yaml"), []byte(supersetPromptYAML), 0o600))
	cfg := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: kit
spec:
  prompt_configs:
    - id: greeting
      file: prompts/greeting.yaml
  providers: []
  defaults:
    temperature: 0.7
    max_tokens: 100
` + extra
	path := filepath.Join(dir, "config.arena.yaml")
	require.NoError(t, os.WriteFile(path, []byte(cfg), 0o600))
	return path
}

func validateJSONFor(t *testing.T, path string) validateJSONReport {
	t.Helper()
	var buf bytes.Buffer
	_ = writeValidateJSON(&buf, path, "auto", false)
	var report validateJSONReport
	require.NoError(t, json.Unmarshal(buf.Bytes(), &report), buf.String())
	return report
}

// Before validate ran packc's checks, both of these passed `validate` and
// failed `packc compile` — the gap a coding agent fell into. They come from
// different packc stages, and both are reported in one pass.
func TestValidateJSON_ReportsEveryPackProblem(t *testing.T) {
	path := writeSupersetKit(t, `  agents:
    entry: triage
    members:
      triage:
        description: Routes requests
  compositions:
    loop:
      version: 1
      steps:
        - id: a
          kind: prompt
          prompt_task: greeting
          depends_on: [b]
        - id: b
          kind: prompt
          prompt_task: greeting
          depends_on: [a]
`)
	_, compileErr := compiler.Compile(path)
	require.Error(t, compileErr, "packc rejects this config")

	report := validateJSONFor(t, path)
	assert.False(t, report.Valid)
	assert.Contains(t, report.Stages, stagePack)

	stages := map[string]validateFinding{}
	for _, f := range report.Errors {
		stages[f.Stage] = f
	}
	require.Contains(t, stages, compiler.StageAgents, "errors: %+v", report.Errors)
	assert.Contains(t, stages[compiler.StageAgents].Description, `member "triage"`)
	require.Contains(t, stages, compiler.StageCompositions, "errors: %+v", report.Errors)
	assert.Contains(t, stages[compiler.StageCompositions].Description, "cycle")
	assert.Empty(t, stages[compiler.StageAgents].Source,
		"source is named only when the file to edit is not the one being validated")
}

func TestValidateJSON_ValidKitRunsPackStage(t *testing.T) {
	report := validateJSONFor(t, writeSupersetKit(t, ""))
	assert.True(t, report.Valid, "errors: %+v", report.Errors)
	assert.Equal(t, []string{stageSchema, stageLoad, stageConfig, stagePack}, report.Stages)
	assert.Empty(t, report.Notes)
}

// A config that only runs scenarios against a pack built elsewhere has no
// prompts; there is no pack to build, so the pack stage is skipped, and says so.
func TestValidateJSON_ScenarioOnlyConfigSkipsPackStage(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "s.yaml"), []byte(`apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: s
spec:
  turns:
    - role: user
      content: hi
`), 0o600))
	path := filepath.Join(dir, "config.arena.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: scenarios-only
spec:
  scenarios:
    - file: s.yaml
`), 0o600))

	report := validateJSONFor(t, path)
	assert.True(t, report.Valid, "errors: %+v", report.Errors)
	assert.NotContains(t, report.Stages, stagePack)
	require.Len(t, report.Notes, 1)
	assert.Contains(t, report.Notes[0], "pack checks skipped")
}

// The guard that keeps validate a superset of packc: for every arena config in
// the repo's examples, if packc cannot compile it, validate must not pass it.
// The one exception is a config with no prompts, which validate deliberately
// does not build — and then packc's reason must be exactly that.
func TestValidate_NeverPassesWhatPackcRejects(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples")
	var configs []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".arena.yaml") {
			configs = append(configs, path)
		}
		return nil
	}))
	require.NotEmpty(t, configs)

	rejected := 0
	for _, cfg := range configs {
		_, compileErr := compiler.Compile(cfg)
		if compileErr == nil {
			continue
		}
		rejected++
		report, err := collectValidation(cfg, "auto", false)
		if err != nil || !report.Valid {
			continue // validate rejected it (or could not check it): not a pass
		}
		if !slices.Contains(report.Stages, stagePack) {
			assert.Contains(t, compileErr.Error(), "no prompts",
				"%s: validate skipped the pack stage, so packc may only reject it for having no prompts", cfg)
			continue
		}
		assert.Fail(t, "packc rejects it but validate passes it", "%s: %v", cfg, compileErr)
	}
	t.Logf("%d example configs, %d rejected by packc", len(configs), rejected)
}
