package compiler

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const toneFragmentYAML = `fragment_type: guidelines
version: v1.0.0
description: House tone
content: "Answer in plain, friendly English."
`

// fragmentPromptYAML is a prompt config whose one fragment reference is given
// by ref, indented as a list item under spec.fragments.
func fragmentPromptYAML(ref string) string {
	return `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: support
spec:
  task_type: support
  version: v1.0.0
  description: Support prompt
  system_template: "You help {{company}} customers. {{tone}}"
  variables:
    - name: company
      type: string
      required: false
      default: Acme
  fragments:
` + ref
}

// fragmentProject writes an arena config at <dir>/config.arena.yaml that
// references prompts/support.yaml, so a fragment path of ../fragments/... is
// relative to the prompt's directory and not to the arena config's.
func fragmentProject(t *testing.T, ref string) (dir, configFile string) {
	t.Helper()
	dir = t.TempDir()
	writeFixture(t, dir, "prompts/support.yaml", fragmentPromptYAML(ref))
	configFile = writeFixture(t, dir, "config.arena.yaml", minimalArenaConfig("prompts/support.yaml"))
	return dir, configFile
}

func TestCompile_FragmentRelativeToPromptDir(t *testing.T) {
	dir, configFile := fragmentProject(t, `    - name: tone
      path: ../fragments/tone.yaml
      required: true
`)
	writeFixture(t, dir, "fragments/tone.yaml", toneFragmentYAML)

	result, err := Compile(configFile, WithPackID("frag-pack"))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"tone": "Answer in plain, friendly English."}, result.Pack.Fragments)
	assert.Contains(t, string(result.JSON), `"tone": "Answer in plain, friendly English."`)
}

func TestCompile_RequiredFragmentMissingNamesTheFile(t *testing.T) {
	dir, configFile := fragmentProject(t, `    - name: tone
      path: ../fragments/tone.yaml
      required: true
`)

	_, err := Compile(configFile, WithPackID("frag-pack"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `required fragment "tone"`)
	assert.Contains(t, err.Error(), filepath.Join(dir, "fragments", "tone.yaml"))
}

func TestCompile_OptionalFragmentMissingWarns(t *testing.T) {
	dir, configFile := fragmentProject(t, `    - name: tone
      path: ../fragments/tone.yaml
      required: false
`)

	result, err := Compile(configFile, WithPackID("frag-pack"))
	require.NoError(t, err)
	assert.Empty(t, result.Pack.Fragments)
	require.Len(t, result.Warnings, 1, result.Warnings)
	assert.Contains(t, result.Warnings[0], `optional fragment "tone" was not loaded`)
	assert.Contains(t, result.Warnings[0], filepath.Join(dir, "fragments", "tone.yaml"))
}

func TestCompile_DynamicFragmentStaysPlaceholder(t *testing.T) {
	_, configFile := fragmentProject(t, `    - name: tone
      path: "../fragments/{{lang}}.yaml"
      required: true
`)

	result, err := Compile(configFile, WithPackID("frag-pack"))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"tone": "{{tone}}"}, result.Pack.Fragments)
	assert.Empty(t, result.Warnings)
}

func TestCompile_FragmentWithoutPathIsAnError(t *testing.T) {
	_, configFile := fragmentProject(t, `    - name: tone
      required: true
`)

	_, err := Compile(configFile, WithPackID("frag-pack"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `fragment "tone" has no path`)
}

func TestCompile_FragmentFileThatDoesNotParse(t *testing.T) {
	dir, configFile := fragmentProject(t, `    - name: tone
      path: ../fragments/tone.yaml
      required: true
`)
	writeFixture(t, dir, "fragments/tone.yaml", "content: [unclosed\n")

	_, err := Compile(configFile, WithPackID("frag-pack"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing fragment file")
	assert.Contains(t, err.Error(), filepath.Join(dir, "fragments", "tone.yaml"))
}

func TestCompilePromptFile_FragmentRelativeToPromptDir(t *testing.T) {
	dir := t.TempDir()
	promptFile := writeFixture(t, dir, "prompts/support.yaml", fragmentPromptYAML(`    - name: tone
      path: ../fragments/tone.yaml
      required: true
`))
	writeFixture(t, dir, "fragments/tone.yaml", toneFragmentYAML)

	r := CompilePromptFile(promptFile)
	require.Empty(t, r.Errors, messages(r.Errors))
	assert.Equal(t, map[string]string{"tone": "Answer in plain, friendly English."}, r.Pack.Fragments)
}

func TestCompilePromptFile_OptionalFragmentMissingWarns(t *testing.T) {
	dir := t.TempDir()
	promptFile := writeFixture(t, dir, "prompts/support.yaml", fragmentPromptYAML(`    - name: tone
      path: ../fragments/tone.yaml
      required: false
`))

	r := CompilePromptFile(promptFile)
	require.Empty(t, r.Errors, messages(r.Errors))
	assert.Contains(t, messages(r.Warnings), `optional fragment "tone" was not loaded`)
}

func TestCompilePromptFile_RequiredFragmentMissing(t *testing.T) {
	dir := t.TempDir()
	promptFile := writeFixture(t, dir, "prompts/support.yaml", fragmentPromptYAML(`    - name: tone
      path: ../fragments/tone.yaml
      required: true
`))

	r := CompilePromptFile(promptFile)
	require.NotEmpty(t, r.Errors)
	assert.Contains(t, messages(r.Errors), filepath.Join(dir, "fragments", "tone.yaml"))
}
