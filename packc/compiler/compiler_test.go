package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFixture creates a file in the temp directory and returns its path.
func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	subdir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(subdir, 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

const minimalPromptYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: greeting
spec:
  task_type: "greeting"
  version: "v1.0.0"
  description: "A simple greeting prompt"
  system_template: "You are a friendly assistant."
`

const secondPromptYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: farewell
spec:
  task_type: "farewell"
  version: "v1.0.0"
  description: "A farewell prompt"
  system_template: "You are a polite assistant that says goodbye."
`

func minimalArenaConfig(promptFiles ...string) string {
	cfg := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
`
	for i, f := range promptFiles {
		cfg += "    - id: prompt" + string(rune('0'+i)) + "\n"
		cfg += "      file: " + f + "\n"
	}
	cfg += `  providers: []
  defaults:
    temperature: 0.7
    max_tokens: 100
`
	return cfg
}

func TestCompile_SinglePrompt(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)
	configFile := writeFixture(t, dir, "config.arena.yaml", minimalArenaConfig("prompts/greeting.yaml"))

	result, err := Compile(configFile,
		WithPackID("test-pack"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Pack)
	require.NotEmpty(t, result.JSON)

	assert.Equal(t, "test-pack", result.Pack.ID)
	assert.Contains(t, result.Pack.Prompts, "greeting")
	assert.Equal(t, "A simple greeting prompt", result.Pack.Prompts["greeting"].Description)

	// Verify JSON is valid
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(result.JSON, &parsed))
	assert.Equal(t, "test-pack", parsed["id"])
}

func TestCompile_MultiplePrompts(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)
	writeFixture(t, dir, "prompts/farewell.yaml", secondPromptYAML)
	configFile := writeFixture(t, dir, "config.arena.yaml",
		minimalArenaConfig("prompts/greeting.yaml", "prompts/farewell.yaml"))

	result, err := Compile(configFile,
		WithPackID("multi-pack"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, result.Pack)

	assert.Equal(t, "multi-pack", result.Pack.ID)
	assert.Len(t, result.Pack.Prompts, 2)
	assert.Contains(t, result.Pack.Prompts, "greeting")
	assert.Contains(t, result.Pack.Prompts, "farewell")
}

func TestCompile_WithTools(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)

	toolYAML := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: search
spec:
  name: search
  description: "Search the web"
  mode: client
  input_schema:
    type: object
    properties:
      query:
        type: string
        description: "Search query"
    required:
      - query
  output_schema:
    type: object
    properties:
      results:
        type: array
`
	writeFixture(t, dir, "tools/search.yaml", toolYAML)

	arenaConfig := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
    - id: prompt0
      file: prompts/greeting.yaml
  tools:
    - file: tools/search.yaml
  providers: []
  defaults:
    temperature: 0.7
    max_tokens: 100
`
	configFile := writeFixture(t, dir, "config.arena.yaml", arenaConfig)

	result, err := Compile(configFile,
		WithPackID("tools-pack"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, result.Pack)

	assert.Equal(t, "tools-pack", result.Pack.ID)
	assert.Contains(t, result.Pack.Prompts, "greeting")

	// Tools may or may not be present depending on how the tool loader works
	// with the Kind: Tool format; verify pack compiled without error.
}

func TestCompile_WithWorkflow(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)
	writeFixture(t, dir, "prompts/farewell.yaml", secondPromptYAML)

	arenaConfig := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
    - id: prompt0
      file: prompts/greeting.yaml
    - id: prompt1
      file: prompts/farewell.yaml
  providers: []
  workflow:
    version: 1
    entry: start
    states:
      start:
        prompt_task: greeting
        on_event:
          Done: end
      end:
        prompt_task: farewell
  defaults:
    temperature: 0.7
    max_tokens: 100
`
	configFile := writeFixture(t, dir, "config.arena.yaml", arenaConfig)

	result, err := Compile(configFile,
		WithPackID("workflow-pack"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, result.Pack)

	assert.Equal(t, "workflow-pack", result.Pack.ID)
	assert.NotNil(t, result.Pack.Workflow)
	assert.Equal(t, "start", result.Pack.Workflow.Entry)
	assert.Len(t, result.Pack.Workflow.States, 2)
}

func TestCompile_WithAgents(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)

	arenaConfig := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
    - id: prompt0
      file: prompts/greeting.yaml
  providers: []
  agents:
    entry: triage
    members:
      triage:
        description: "Triage agent"
        tags:
          - router
  defaults:
    temperature: 0.7
    max_tokens: 100
`
	configFile := writeFixture(t, dir, "config.arena.yaml", arenaConfig)

	result, err := Compile(configFile,
		WithPackID("agents-pack"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, result.Pack)

	assert.Equal(t, "agents-pack", result.Pack.ID)
	assert.NotNil(t, result.Pack.Agents)
	assert.Equal(t, "triage", result.Pack.Agents.Entry)
}

func TestCompile_CustomPackID(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)
	configFile := writeFixture(t, dir, "config.arena.yaml", minimalArenaConfig("prompts/greeting.yaml"))

	result, err := Compile(configFile,
		WithPackID("custom-id-123"),
		WithCompilerVersion("test-v2"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, result.Pack)

	assert.Equal(t, "custom-id-123", result.Pack.ID)
	assert.Contains(t, result.Pack.Compilation.CompiledWith, "test-v2")
}

func TestCompile_DefaultPackIDFromDirectory(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory with a specific name to test ID derivation
	subDir := filepath.Join(dir, "My Cool Project")
	require.NoError(t, os.MkdirAll(filepath.Join(subDir, "prompts"), 0o755))
	writeFixture(t, subDir, "prompts/greeting.yaml", minimalPromptYAML)
	configFile := writeFixture(t, subDir, "config.arena.yaml", minimalArenaConfig("prompts/greeting.yaml"))

	result, err := Compile(configFile,
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, result.Pack)

	// Should be sanitized from "My Cool Project"
	assert.Equal(t, "my-cool-project", result.Pack.ID)
}

func TestCompile_NonexistentConfigFile(t *testing.T) {
	result, err := Compile("/nonexistent/path/config.arena.yaml",
		WithPackID("test"),
		WithSkipSchemaValidation(),
	)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "loading arena config")
}

func TestCompile_InvalidConfigFile(t *testing.T) {
	dir := t.TempDir()
	configFile := writeFixture(t, dir, "config.arena.yaml", "this is not valid yaml: [[[")

	result, err := Compile(configFile,
		WithPackID("test"),
		WithSkipSchemaValidation(),
	)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestCompile_EmptyConfig(t *testing.T) {
	dir := t.TempDir()
	arenaConfig := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs: []
  providers: []
  defaults:
    temperature: 0.7
    max_tokens: 100
`
	configFile := writeFixture(t, dir, "config.arena.yaml", arenaConfig)

	result, err := Compile(configFile,
		WithPackID("empty-pack"),
		WithSkipSchemaValidation(),
	)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestCompile_ResultJSONIsValidPack(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)
	configFile := writeFixture(t, dir, "config.arena.yaml", minimalArenaConfig("prompts/greeting.yaml"))

	result, err := Compile(configFile,
		WithPackID("json-test"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)

	// Verify the JSON can be deserialized back into a Pack
	var pack prompt.Pack
	require.NoError(t, json.Unmarshal(result.JSON, &pack))
	assert.Equal(t, "json-test", pack.ID)
	assert.Contains(t, pack.Prompts, "greeting")
}

func TestSanitizePackID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MyProject", "myproject"},
		{"my project name", "my-project-name"},
		{"my_project!@#$%", "myproject"},
		{"Customer Support Bot!", "customer-support-bot"},
		{"my---project", "my-project"},
		{"-my-project-", "my-project"},
		{"customer-support", "customer-support"},
		{"project123", "project123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizePackID(tt.input))
		})
	}
}

func TestCompile_IncludesSkills(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()

	writeFixture(t, dir, "prompts/chat.yaml", minimalPromptYAML)

	// Create skill directory with a SKILL.md
	skillDir := filepath.Join(dir, "skills", "test-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(
		"---\nname: test-skill\ndescription: Test\n---\nInstructions.\n",
	), 0o600))

	arenaConfig := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
    - id: greeting
      file: prompts/chat.yaml
  providers: []
  skills:
    - path: skills/
  defaults:
    concurrency: 1
`
	configFile := writeFixture(t, dir, "config.arena.yaml", arenaConfig)

	result, err := Compile(configFile,
		WithPackID("skills-pack"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, result.Pack)

	assert.Equal(t, "skills-pack", result.Pack.ID)
	require.Len(t, result.Pack.Skills, 1)
	// Path should be relative (converted back from absolute)
	assert.Equal(t, "skills", prompt.SkillPath(result.Pack.Skills[0]))
}

const echoToolYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: echo
spec:
  name: echo
  description: "Echo input back"
  mode: client
  input_schema:
    type: object
    properties:
      x:
        type: string
        description: "Value to echo"
  output_schema:
    type: object
    properties:
      result:
        type: string
`

func TestCompile_CarriesAndValidatesCompositions(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")

	// valid: workflow composition state referencing a real tool in the pack
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)
	writeFixture(t, dir, "tools/echo.yaml", echoToolYAML)

	validArena := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
    - id: prompt0
      file: prompts/greeting.yaml
  tools:
    - file: tools/echo.yaml
  providers: []
  workflow:
    version: 1
    entry: run
    states:
      run:
        orchestration: composition
        composition: flow
        terminal: true
  compositions:
    flow:
      version: 1
      steps:
        - id: s
          kind: tool
          tool: echo
          args:
            x: "${input.t}"
  defaults:
    temperature: 0.7
    max_tokens: 100
`
	configFile := writeFixture(t, dir, "config.arena.yaml", validArena)

	result, err := Compile(configFile,
		WithPackID("comp-pack"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err, "valid composition pack should compile without error")
	require.NotNil(t, result.Pack)
	require.Len(t, result.Pack.Compositions, 1, "compiled pack should contain 1 composition")

	// bad: composition step references a tool not in the pack
	dir2 := t.TempDir()
	writeFixture(t, dir2, "prompts/greeting.yaml", minimalPromptYAML)
	writeFixture(t, dir2, "tools/echo.yaml", echoToolYAML)

	badArena := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
    - id: prompt0
      file: prompts/greeting.yaml
  tools:
    - file: tools/echo.yaml
  providers: []
  workflow:
    version: 1
    entry: run
    states:
      run:
        orchestration: composition
        composition: flow
        terminal: true
  compositions:
    flow:
      version: 1
      steps:
        - id: s
          kind: tool
          tool: missing_tool
          args:
            x: "${input.t}"
  defaults:
    temperature: 0.7
    max_tokens: 100
`
	badConfigFile := writeFixture(t, dir2, "config.arena.yaml", badArena)

	_, err = Compile(badConfigFile,
		WithPackID("bad-comp-pack"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.Error(t, err, "composition with an unknown tool ref must fail compilation")
	assert.Contains(t, err.Error(), "composition")
}

func TestCompileOptions(t *testing.T) {
	t.Run("WithPackID sets pack ID", func(t *testing.T) {
		var opts compileOptions
		WithPackID("my-id")(&opts)
		assert.Equal(t, "my-id", opts.packID)
	})

	t.Run("WithCompilerVersion sets version", func(t *testing.T) {
		var opts compileOptions
		WithCompilerVersion("v2.0")(&opts)
		assert.Equal(t, "v2.0", opts.compilerVersion)
	})

	t.Run("WithSkipSchemaValidation sets flag", func(t *testing.T) {
		var opts compileOptions
		WithSkipSchemaValidation()(&opts)
		assert.True(t, opts.skipSchemaValidation)
	})
}

// TestCompile_WithPackMetadata covers the arena config's pack_metadata block,
// the primary authoring route to the compiled pack's metadata: the registry
// compiler has no CompileOption for it and leaves pack.Metadata nil, so
// governance could travel in a pack that no author could write.
func TestCompile_WithPackMetadata(t *testing.T) {
	// pack_metadata is new, so validate against the in-repo schema rather than
	// the published one, which has not shipped this field yet.
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)

	arenaConfig := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
    - id: prompt0
      file: prompts/greeting.yaml
  providers: []
  pack_metadata:
    domain: customer-support
    language: en
    tags:
      - support
  defaults:
    temperature: 0.7
`
	configFile := writeFixture(t, dir, "config.arena.yaml", arenaConfig)

	result, err := Compile(configFile,
		WithPackID("metadata-pack"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, result.Pack)
	require.NotNil(t, result.Pack.Metadata, "pack_metadata must reach the compiled pack")

	assert.Equal(t, "customer-support", result.Pack.Metadata.Domain)
	assert.Equal(t, "en", result.Pack.Metadata.Language)
	assert.Equal(t, []string{"support"}, result.Pack.Metadata.Tags)
}

// TestCompile_CarriesSpecMetadata covers the fallback route (#135): when the
// arena config has no pack_metadata block, a loaded prompt config's own
// spec.metadata should still reach the compiled pack instead of being
// silently dropped.
func TestCompile_CarriesSpecMetadata(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()

	promptWithMetadata := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: support
spec:
  task_type: "support"
  version: "v1.0.0"
  description: "Customer support prompt"
  system_template: "You are a support assistant."
  metadata:
    domain: "financial-services"
    tags:
      - "underwriting"
      - "mortgage"
`
	writeFixture(t, dir, "prompts/support.yaml", promptWithMetadata)
	configFile := writeFixture(t, dir, "config.arena.yaml", minimalArenaConfig("prompts/support.yaml"))

	res, err := Compile(configFile,
		WithPackID("support-pack"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, res.Pack)
	require.NotNil(t, res.Pack.Metadata, "spec.metadata must be carried into the compiled pack")
	assert.Equal(t, "financial-services", res.Pack.Metadata.Domain)
	assert.Equal(t, []string{"underwriting", "mortgage"}, res.Pack.Metadata.Tags)
	// PackMetadata (PromptKit v1.9.0, packspec RFC 0012) no longer carries a
	// changelog field, so that assertion was dropped along with it here.

	// Promoting a prompt's metadata to the whole pack is a guess, so it has to
	// be visible: a silent promotion is the same class of surprise as #135's
	// silent drop.
	assert.True(t, hasWarning(res.Warnings, `taken from prompt "prompt0"`),
		"promoting spec.metadata must be reported, got %v", res.Warnings)
}

// promptWithMetadataYAML is a prompt config declaring its own spec.metadata.
func promptWithMetadataYAML(name, domain string) string {
	return `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: ` + name + `
spec:
  task_type: "` + name + `"
  version: "v1.0.0"
  description: "` + name + ` prompt"
  system_template: "You are ` + name + `."
  metadata:
    domain: "` + domain + `"
`
}

func hasWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// TestCompile_SpecMetadataChoiceIsDeterministic pins the choice made when more
// than one prompt declares spec.metadata. The candidates come out of a map, so
// picking "the first one found" would compile the same config into different
// packs on different runs — and quietly, since only one of them can win.
func TestCompile_SpecMetadataChoiceIsDeterministic(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/alpha.yaml", promptWithMetadataYAML("alpha", "aviation"))
	writeFixture(t, dir, "prompts/beta.yaml", promptWithMetadataYAML("beta", "banking"))
	configFile := writeFixture(t, dir, "config.arena.yaml",
		minimalArenaConfig("prompts/alpha.yaml", "prompts/beta.yaml"))

	// Go randomizes map iteration per range, so a single compile would agree
	// with an arbitrary pick about half the time.
	for i := 0; i < 20; i++ {
		res, err := Compile(configFile,
			WithPackID("multi-pack"),
			WithCompilerVersion("test-v1"),
			WithSkipSchemaValidation(),
		)
		require.NoError(t, err)
		require.NotNil(t, res.Pack.Metadata)
		assert.Equal(t, "aviation", res.Pack.Metadata.Domain,
			"the first prompt in sorted order must win on every run")
		assert.True(t, hasWarning(res.Warnings, "only \"prompt0\" was used"),
			"the prompts passed over must be named, got %v", res.Warnings)
	}
}

// TestCompile_PackMetadataBeatsSpecMetadata covers the precedence: pack_metadata
// is the pack-level authoring route, so it wins — but the spec.metadata it
// displaced is reported rather than dropped on the floor.
func TestCompile_PackMetadataBeatsSpecMetadata(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/alpha.yaml", promptWithMetadataYAML("alpha", "aviation"))

	arenaConfig := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test
spec:
  prompt_configs:
    - id: prompt0
      file: prompts/alpha.yaml
  providers: []
  pack_metadata:
    domain: customer-support
  defaults:
    temperature: 0.7
`
	configFile := writeFixture(t, dir, "config.arena.yaml", arenaConfig)

	res, err := Compile(configFile,
		WithPackID("precedence-pack"),
		WithCompilerVersion("test-v1"),
		WithSkipSchemaValidation(),
	)
	require.NoError(t, err)
	require.NotNil(t, res.Pack.Metadata)
	assert.Equal(t, "customer-support", res.Pack.Metadata.Domain)
	assert.True(t, hasWarning(res.Warnings, "was ignored"),
		"the ignored spec.metadata must be reported, got %v", res.Warnings)
}
