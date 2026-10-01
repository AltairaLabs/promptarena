package compiler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/AltairaLabs/PromptKit/runtime/v2/prompt"
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
				Examples: []*prompt.MultimodalExample{{
					Name: "ex",
					Role: "user",
					Parts: []*prompt.ExampleContentPart{{
						Media: &prompt.ExampleMedia{FilePath: "gone.jpg", MimeType: "image/jpeg"},
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
				Examples: []*prompt.MultimodalExample{{
					Name: "ex",
					Role: "user",
					Parts: []*prompt.ExampleContentPart{{
						Media: &prompt.ExampleMedia{FilePath: "ok.jpg", MimeType: "image/jpeg"},
					}},
				}},
			},
		}}
		assert.Empty(t, validateMediaReferences(cfg, dir))
	})
}

func TestValidateExampleMediaReferences_Integration(t *testing.T) {
	t.Run("text only part skipped", func(t *testing.T) {
		ex := prompt.MultimodalExample{Name: "e", Role: "user", Parts: []*prompt.ExampleContentPart{
			{Text: "hi"},
		}}
		assert.Empty(t, validateExampleMediaReferences(&ex, "/tmp"))
	})

	t.Run("empty file path skipped", func(t *testing.T) {
		ex := prompt.MultimodalExample{Name: "e", Role: "user", Parts: []*prompt.ExampleContentPart{
			{Media: &prompt.ExampleMedia{URL: "https://x/y.jpg", MimeType: "image/jpeg"}},
		}}
		assert.Empty(t, validateExampleMediaReferences(&ex, "/tmp"))
	})

	t.Run("absolute missing path warns", func(t *testing.T) {
		ex := prompt.MultimodalExample{Name: "e", Role: "user", Parts: []*prompt.ExampleContentPart{
			{Media: &prompt.ExampleMedia{FilePath: "/no/such/x.jpg", MimeType: "image/jpeg"}},
		}}
		w := validateExampleMediaReferences(&ex, "/tmp")
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
					Examples: []*prompt.MultimodalExample{{
						Name: "ex", Role: "user",
						Parts: []*prompt.ExampleContentPart{{
							Media: &prompt.ExampleMedia{FilePath: "missing.png", MimeType: "image/png"},
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

	t.Run("passes the agents block through", func(t *testing.T) {
		cfg := &arenaconfig.Config{Agents: &prompt.AgentsConfig{
			Entry: "triage",
			Members: map[string]*prompt.AgentDef{
				"triage": {Description: "Triage agent"},
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
		got, err := parseToolsFromConfig(&arenaconfig.Config{})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("invalid tool bytes fail", func(t *testing.T) {
		cfg := &arenaconfig.Config{LoadedTools: []config.ToolData{
			{FilePath: "broken.yaml", Data: []byte("not: [valid")},
		}}
		got, err := parseToolsFromConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "broken.yaml")
		assert.Nil(t, got)
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
		got, err := parseToolsFromConfig(cfg)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "search", got[0].Name)
		assert.Equal(t, "Search the web", got[0].Description)
	})
}

func TestCheckSchema(t *testing.T) {
	// Build a valid pack JSON to validate against the embedded schema.
	dir := t.TempDir()
	writeFixture(t, dir, "prompts/greeting.yaml", minimalPromptYAML)
	configFile := writeFixture(t, dir, "config.arena.yaml", minimalArenaConfig("prompts/greeting.yaml"))
	res, err := Compile(configFile, WithPackID("schema-test"), WithSkipSchemaValidation())
	require.NoError(t, err)
	src := sourceMap{arenaConfig: "arena.yaml", prompts: map[string]string{"greeting": "prompts/greeting.yaml"}}

	check := func(data []byte) *CheckResult {
		r := &CheckResult{}
		r.checkSchema(data, src)
		return r
	}

	t.Run("valid pack passes", func(t *testing.T) {
		assert.Empty(t, check(res.JSON).Errors)
	})

	t.Run("malformed json errors", func(t *testing.T) {
		r := check([]byte("this is not json"))
		require.Len(t, r.Errors, 1)
		assert.Equal(t, StagePackSchema, r.Errors[0].Stage)
		assert.Contains(t, r.Errors[0].Message, "schema validation failed")
	})

	t.Run("each violation is its own problem, pointed at its source", func(t *testing.T) {
		var pack map[string]any
		require.NoError(t, json.Unmarshal(res.JSON, &pack))
		pack["id"] = 123
		greeting := pack["prompts"].(map[string]any)["greeting"].(map[string]any)
		greeting["not_a_spec_field"] = true
		data, err := json.Marshal(pack)
		require.NoError(t, err)

		r := check(data)
		require.Len(t, r.Errors, 2)
		byPath := map[string]Problem{}
		for _, p := range r.Errors {
			byPath[p.Path] = p
		}
		require.Contains(t, byPath, "id")
		assert.Equal(t, "arena.yaml", byPath["id"].Source)
		require.Contains(t, byPath, "prompts.greeting")
		assert.Equal(t, "prompts/greeting.yaml", byPath["prompts.greeting"].Source,
			"a prompt-level violation points at the prompt config file")
		assert.Contains(t, byPath["prompts.greeting"].Message, "not_a_spec_field")
	})

	t.Run("unreadable schema source errors", func(t *testing.T) {
		t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "/nonexistent/schema/file.json")
		r := check(res.JSON)
		require.Len(t, r.Errors, 1)
		assert.Contains(t, r.Errors[0].Message, "could not be performed")
	})
}
