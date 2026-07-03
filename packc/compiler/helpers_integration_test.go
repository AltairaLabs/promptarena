package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/prompt"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

func TestValidateMediaReferences_Integration(t *testing.T) {
	t.Run("nil media config", func(t *testing.T) {
		cfg := &prompt.Config{Spec: prompt.Spec{TaskType: "t"}}
		assert.Empty(t, validateMediaReferences(cfg, "/tmp"))
	})

	t.Run("disabled media config", func(t *testing.T) {
		cfg := &prompt.Config{Spec: prompt.Spec{
			TaskType:    "t",
			MediaConfig: &prompt.MediaConfig{Enabled: false},
		}}
		assert.Empty(t, validateMediaReferences(cfg, "/tmp"))
	})

	t.Run("missing file yields warning", func(t *testing.T) {
		cfg := &prompt.Config{Spec: prompt.Spec{
			TaskType: "t",
			MediaConfig: &prompt.MediaConfig{
				Enabled: true,
				Examples: []prompt.MultimodalExample{{
					Name: "ex",
					Role: "user",
					Parts: []prompt.ExampleContentPart{{
						Media: &prompt.ExampleMedia{FilePath: "gone.jpg", MIMEType: "image/jpeg"},
					}},
				}},
			},
		}}
		w := validateMediaReferences(cfg, "/tmp")
		require.Len(t, w, 1)
		assert.Contains(t, w[0], "gone.jpg")
	})

	t.Run("existing file yields no warning", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ok.jpg"), []byte("x"), 0o644))
		cfg := &prompt.Config{Spec: prompt.Spec{
			TaskType: "t",
			MediaConfig: &prompt.MediaConfig{
				Enabled: true,
				Examples: []prompt.MultimodalExample{{
					Name: "ex",
					Role: "user",
					Parts: []prompt.ExampleContentPart{{
						Media: &prompt.ExampleMedia{FilePath: "ok.jpg", MIMEType: "image/jpeg"},
					}},
				}},
			},
		}}
		assert.Empty(t, validateMediaReferences(cfg, dir))
	})
}

func TestValidateExampleMediaReferences_Integration(t *testing.T) {
	t.Run("text only part skipped", func(t *testing.T) {
		ex := prompt.MultimodalExample{Name: "e", Role: "user", Parts: []prompt.ExampleContentPart{
			{Text: "hi"},
		}}
		assert.Empty(t, validateExampleMediaReferences(ex, "/tmp"))
	})

	t.Run("empty file path skipped", func(t *testing.T) {
		ex := prompt.MultimodalExample{Name: "e", Role: "user", Parts: []prompt.ExampleContentPart{
			{Media: &prompt.ExampleMedia{URL: "https://x/y.jpg", MIMEType: "image/jpeg"}},
		}}
		assert.Empty(t, validateExampleMediaReferences(ex, "/tmp"))
	})

	t.Run("absolute missing path warns", func(t *testing.T) {
		ex := prompt.MultimodalExample{Name: "e", Role: "user", Parts: []prompt.ExampleContentPart{
			{Media: &prompt.ExampleMedia{FilePath: "/no/such/x.jpg", MIMEType: "image/jpeg"}},
		}}
		w := validateExampleMediaReferences(ex, "/tmp")
		require.Len(t, w, 1)
		assert.Contains(t, w[0], "/no/such/x.jpg")
		assert.Contains(t, w[0], "part 0")
	})
}

func TestCollectMediaWarnings(t *testing.T) {
	t.Run("non-config entries skipped", func(t *testing.T) {
		cfg := &arenaconfig.Config{LoadedPromptConfigs: map[string]*arenaconfig.PromptConfigData{
			"bad": {FilePath: "bad.yaml", Config: "not a prompt config"},
		}}
		assert.Empty(t, collectMediaWarnings(cfg, "/tmp"))
	})

	t.Run("prefixes warnings with task type", func(t *testing.T) {
		cfg := &arenaconfig.Config{LoadedPromptConfigs: map[string]*arenaconfig.PromptConfigData{
			"greet": {FilePath: "greet.yaml", Config: &prompt.Config{Spec: prompt.Spec{
				TaskType: "greet",
				MediaConfig: &prompt.MediaConfig{
					Enabled: true,
					Examples: []prompt.MultimodalExample{{
						Name: "ex", Role: "user",
						Parts: []prompt.ExampleContentPart{{
							Media: &prompt.ExampleMedia{FilePath: "missing.png", MIMEType: "image/png"},
						}},
					}},
				},
			}}},
		}}
		w := collectMediaWarnings(cfg, "/tmp")
		require.Len(t, w, 1)
		assert.Contains(t, w[0], "media(greet):")
		assert.Contains(t, w[0], "missing.png")
	})
}

func TestBuildMemoryRepo_Integration(t *testing.T) {
	t.Run("registers valid config", func(t *testing.T) {
		cfg := &arenaconfig.Config{LoadedPromptConfigs: map[string]*arenaconfig.PromptConfigData{
			"t": {FilePath: "t.yaml", Config: &prompt.Config{
				APIVersion: "promptkit.altairalabs.ai/v1alpha1",
				Kind:       "PromptConfig",
				Spec:       prompt.Spec{TaskType: "t", SystemTemplate: "hi"},
			}},
		}}
		repo, err := buildMemoryRepo(cfg)
		require.NoError(t, err)
		require.NotNil(t, repo)
		got, err := repo.LoadPrompt("t")
		require.NoError(t, err)
		assert.Equal(t, "t", got.Spec.TaskType)
	})

	t.Run("nil config entry skipped", func(t *testing.T) {
		cfg := &arenaconfig.Config{LoadedPromptConfigs: map[string]*arenaconfig.PromptConfigData{
			"t": {FilePath: "t.yaml", Config: nil},
		}}
		repo, err := buildMemoryRepo(cfg)
		require.NoError(t, err)
		assert.NotNil(t, repo)
	})

	t.Run("invalid config type errors", func(t *testing.T) {
		cfg := &arenaconfig.Config{LoadedPromptConfigs: map[string]*arenaconfig.PromptConfigData{
			"t": {FilePath: "bad.yaml", Config: "not a config"},
		}}
		repo, err := buildMemoryRepo(cfg)
		require.Error(t, err)
		assert.Nil(t, repo)
		assert.Contains(t, err.Error(), "invalid type")
	})
}

func TestParseAgentsFromConfig_Integration(t *testing.T) {
	t.Run("nil agents returns nil", func(t *testing.T) {
		ag, err := parseAgentsFromConfig(&arenaconfig.Config{})
		require.NoError(t, err)
		assert.Nil(t, ag)
	})

	t.Run("parses agents block", func(t *testing.T) {
		cfg := &arenaconfig.Config{Agents: map[string]any{
			"entry": "triage",
			"members": map[string]any{
				"triage": map[string]any{"description": "Triage agent"},
			},
		}}
		ag, err := parseAgentsFromConfig(cfg)
		require.NoError(t, err)
		require.NotNil(t, ag)
		assert.Equal(t, "triage", ag.Entry)
		assert.Contains(t, ag.Members, "triage")
	})
}

func TestParseToolsFromConfig_Integration(t *testing.T) {
	t.Run("no tools returns empty", func(t *testing.T) {
		assert.Empty(t, parseToolsFromConfig(&arenaconfig.Config{}))
	})

	t.Run("invalid tool bytes skipped", func(t *testing.T) {
		cfg := &arenaconfig.Config{LoadedTools: []config.ToolData{
			{FilePath: "broken.yaml", Data: []byte("not: [valid")},
		}}
		assert.Empty(t, parseToolsFromConfig(cfg))
	})

	t.Run("valid tool parsed", func(t *testing.T) {
		toolYAML := []byte(`apiVersion: promptkit.altairalabs.ai/v1alpha1
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
`)
		cfg := &arenaconfig.Config{LoadedTools: []config.ToolData{
			{FilePath: "search.yaml", Data: toolYAML},
		}}
		got := parseToolsFromConfig(cfg)
		require.Len(t, got, 1)
		assert.Equal(t, "search", got[0].Name)
		assert.Equal(t, "Search the web", got[0].Description)
	})
}

func TestValidateSchema(t *testing.T) {
	// Build a valid pack JSON to validate against the embedded schema.
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)
	configFile := writeFixture(t, dir, "config.arena.yaml", minimalArenaConfig("prompts/greeting.yaml"))
	res, err := Compile(configFile, WithPackID("schema-test"), WithSkipSchemaValidation())
	require.NoError(t, err)

	t.Run("valid pack passes", func(t *testing.T) {
		assert.NoError(t, validateSchema(res.JSON))
	})

	t.Run("malformed json errors", func(t *testing.T) {
		err := validateSchema([]byte("this is not json"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "schema validation failed")
	})

	t.Run("schema-invalid pack errors", func(t *testing.T) {
		err := validateSchema([]byte(`{"id": 123}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed schema validation")
	})

	t.Run("unreadable schema source errors", func(t *testing.T) {
		t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "/nonexistent/schema/file.json")
		err := validateSchema(res.JSON)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not be performed")
	})
}
