package compiler

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
)

// compiledPackMap compiles the minimal project and returns the pack as a
// generic map, so a test can break one field and write it back.
func compiledPackMap(t *testing.T) map[string]any {
	t.Helper()
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)
	configFile := writeFixture(t, dir, "config.arena.yaml", minimalArenaConfig("prompts/greeting.yaml"))
	res, err := Compile(configFile, WithPackID("test-pack"), WithCompilerVersion("test-v1"))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(res.JSON, &m))
	return m
}

func writePackMap(t *testing.T, m map[string]any) string {
	t.Helper()
	data, err := json.Marshal(m)
	require.NoError(t, err)
	return writeFixture(t, t.TempDir(), "test.pack.json", string(data))
}

func stages(problems []Problem) []string {
	out := make([]string, 0, len(problems))
	for _, p := range problems {
		out = append(out, p.Stage)
	}
	return out
}

func messages(problems []Problem) string {
	out := make([]string, 0, len(problems))
	for _, p := range problems {
		out = append(out, p.String())
	}
	return strings.Join(out, "\n")
}

func TestValidatePackFile_ValidPack(t *testing.T) {
	r := ValidatePackFile(writePackMap(t, compiledPackMap(t)))
	assert.Empty(t, r.Errors, messages(r.Errors))
	require.NotNil(t, r.Pack)
	assert.Equal(t, "test-pack", r.Pack.ID)
}

func TestValidatePackFile_MissingFile(t *testing.T) {
	r := ValidatePackFile(filepath.Join(t.TempDir(), "nope.pack.json"))
	require.Len(t, r.Errors, 1)
	assert.Equal(t, StageLoad, r.Errors[0].Stage)
	assert.Contains(t, r.Errors[0].Message, "reading pack file")
}

// A JSON syntax error is reported as one, not as schema errors against the
// leading object that happened to parse (#424 item 7).
func TestValidatePackFile_InvalidJSON(t *testing.T) {
	path := writeFixture(t, t.TempDir(), "x.pack.json", "{}}")
	r := ValidatePackFile(path)
	require.Len(t, r.Errors, 1, messages(r.Errors))
	assert.Equal(t, StageLoad, r.Errors[0].Stage)
	assert.Contains(t, r.Errors[0].Message, "invalid JSON")
	assert.NotContains(t, messages(r.Errors), "id is required")
}

func TestValidatePackFile_SchemaErrors(t *testing.T) {
	m := compiledPackMap(t)
	m["version"] = "1.0"
	r := ValidatePackFile(writePackMap(t, m))
	assert.Contains(t, stages(r.Errors), StagePackSchema, messages(r.Errors))
}

// Agent findings compile treats as warnings must not fail validate (#424 item 4).
func TestValidatePackFile_AgentWarningsDoNotBlock(t *testing.T) {
	m := compiledPackMap(t)
	m["agents"] = map[string]any{
		"entry": "greeting",
		"members": map[string]any{
			"greeting": map[string]any{"description": "Greets", "input_modes": []string{"text"}},
		},
	}
	r := ValidatePackFile(writePackMap(t, m))
	assert.Empty(t, r.Errors, messages(r.Errors))
	assert.Contains(t, messages(r.Warnings), "not a valid MIME type")
}

// validate runs the composition check compile runs (#424 item 5).
func TestValidatePackFile_ChecksCompositions(t *testing.T) {
	m := compiledPackMap(t)
	m["workflow"] = map[string]any{
		"version": 1,
		"entry":   "run",
		"states": map[string]any{
			"run": map[string]any{"orchestration": "composition", "composition": "flow", "terminal": true},
		},
	}
	m["compositions"] = map[string]any{
		"flow": map[string]any{
			"version": 1,
			"steps": []any{map[string]any{
				"id": "s", "kind": "tool", "tool": "missing_tool", "args": map[string]any{"x": "${input.t}"},
			}},
		},
	}
	r := ValidatePackFile(writePackMap(t, m))
	assert.Contains(t, stages(r.Errors), StageCompositions, messages(r.Errors))
}

// The PromptPack schema does not require compilation, so a pack written by
// another tool without it validates, with a warning (#424 item 13).
func TestValidatePackFile_MissingCompilationIsAWarning(t *testing.T) {
	m := compiledPackMap(t)
	delete(m, "compilation")
	r := ValidatePackFile(writePackMap(t, m))
	assert.Empty(t, r.Errors, messages(r.Errors))
	assert.Contains(t, messages(r.Warnings), "missing compilation metadata")
}

func TestValidatePackFile_UndeclaredPromptToolIsAWarning(t *testing.T) {
	m := compiledPackMap(t)
	m["prompts"].(map[string]any)["greeting"].(map[string]any)["tools"] = []string{"nonexistent_tool"}
	r := ValidatePackFile(writePackMap(t, m))
	assert.Empty(t, r.Errors, messages(r.Errors))
	assert.Contains(t, messages(r.Warnings), `tool 'nonexistent_tool' is not defined in the pack's tools`)
}

func TestCompilePromptFile_Success(t *testing.T) {
	promptFile := writeFixture(t, t.TempDir(), "greeting.yaml", minimalPromptYAML)
	r := CompilePromptFile(promptFile, WithCompilerVersion("test-v1"))
	require.NoError(t, r.Err())
	require.NotNil(t, r.Pack)
	assert.Equal(t, "v1.0.0", r.Pack.Version, "defaults to the prompt's spec.version")
	require.NotNil(t, r.Pack.Schema, "compile-prompt packs carry $schema (#425 item 11)")
	assert.Equal(t, prompt.PromptPackSchemaURL, *r.Pack.Schema)
	assert.Contains(t, string(r.JSON), `"$schema"`)
}

func TestCompilePromptFile_PackVersion(t *testing.T) {
	promptFile := writeFixture(t, t.TempDir(), "greeting.yaml", minimalPromptYAML)
	r := CompilePromptFile(promptFile, WithPackVersion("2.3.4"), WithSkipSchemaValidation())
	require.NoError(t, r.Err())
	assert.Equal(t, "2.3.4", r.Pack.Version)

	r = CompilePromptFile(promptFile, WithPackVersion("two"))
	require.Len(t, r.Errors, 1)
	assert.Equal(t, StagePackVersion, r.Errors[0].Stage)
}

// compile-prompt rejects a prompt config that fails its schema, as compile
// does, instead of writing a pack validate then rejects (#425 item 4).
func TestCompilePromptFile_SchemaInvalidPrompt(t *testing.T) {
	bad := strings.Replace(minimalPromptYAML, `version: "v1.0.0"`, `version: "1.0"`, 1)
	promptFile := writeFixture(t, t.TempDir(), "bad.yaml", bad)
	r := CompilePromptFile(promptFile)
	require.Len(t, r.Errors, 1, messages(r.Errors))
	assert.Equal(t, StageLoad, r.Errors[0].Stage)
	assert.Contains(t, r.Errors[0].Message, "schema validation")
	assert.Nil(t, r.JSON)
}

func TestCompilePromptFile_MissingFile(t *testing.T) {
	r := CompilePromptFile(filepath.Join(t.TempDir(), "nope.yaml"))
	require.Len(t, r.Errors, 1)
	assert.Contains(t, r.Errors[0].Message, "reading prompt file")
}

func TestCompilePromptFile_MediaWarning(t *testing.T) {
	withMedia := minimalPromptYAML + `  media:
    enabled: true
    supported_types: ["image"]
    examples:
      - name: missing-media
        role: user
        parts:
          - type: image
            media:
              file_path: does-not-exist.jpg
              mime_type: image/jpeg
`
	promptFile := writeFixture(t, t.TempDir(), "greeting.yaml", withMedia)
	r := CompilePromptFile(promptFile, WithSkipSchemaValidation())
	require.NoError(t, r.Err())
	assert.Contains(t, messages(r.Warnings), "does-not-exist.jpg")
}

func TestCompile_PackVersion(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)
	configFile := writeFixture(t, dir, "config.arena.yaml", minimalArenaConfig("prompts/greeting.yaml"))

	res, err := Compile(configFile, WithPackID("p"), WithPackVersion("v3.1.0-rc.1"))
	require.NoError(t, err)
	assert.Equal(t, "v3.1.0-rc.1", res.Pack.Version)

	res, err = Compile(configFile, WithPackID("p"))
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", res.Pack.Version, "PromptKit's default when none is given")

	_, err = Compile(configFile, WithPackID("p"), WithPackVersion("latest"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pack version")
}

func engineYAML(name, version string) string {
	return strings.Replace(
		strings.ReplaceAll(minimalPromptYAML, "greeting", name),
		"  system_template:",
		"  template_engine:\n    version: "+version+"\n    syntax: \"{{variable}}\"\n  system_template:", 1)
}

func TestCompile_TemplateEngine(t *testing.T) {
	t.Run("prompts that agree set the pack engine", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, "prompts/a.yaml", engineYAML("alpha", "v1"))
		writeFixture(t, dir, "prompts/b.yaml", engineYAML("beta", "v1"))
		writeFixture(t, dir, "prompts/c.yaml", strings.ReplaceAll(minimalPromptYAML, "greeting", "gamma"))
		configFile := writeFixture(t, dir, "config.arena.yaml",
			minimalArenaConfig("prompts/a.yaml", "prompts/b.yaml", "prompts/c.yaml"))
		res, err := Compile(configFile, WithPackID("p"))
		require.NoError(t, err)
		require.NotNil(t, res.Pack.TemplateEngine)
		assert.Equal(t, "v1", res.Pack.TemplateEngine.Version)
		assert.Empty(t, res.Pack.TemplateEngine.Features, "the declared engine, not the registry default")
	})

	t.Run("prompts that disagree fail the compile", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, "prompts/a.yaml", engineYAML("alpha", "v1"))
		writeFixture(t, dir, "prompts/b.yaml", engineYAML("beta", "v2"))
		configFile := writeFixture(t, dir, "config.arena.yaml",
			minimalArenaConfig("prompts/a.yaml", "prompts/b.yaml"))
		_, err := Compile(configFile, WithPackID("p"), WithSkipSchemaValidation())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "different template engines")
	})
}

func TestValidatePackVersion(t *testing.T) {
	for _, v := range []string{"1.0.0", "v2.1.3", "1.0.0-alpha", "2.0.0-beta.1+build.123"} {
		assert.NoError(t, validatePackVersion(v), v)
	}
	for _, v := range []string{"", "1.0", "latest", "v1.0.0.0", "01.0.0"} {
		assert.Error(t, validatePackVersion(v), v)
	}
}
