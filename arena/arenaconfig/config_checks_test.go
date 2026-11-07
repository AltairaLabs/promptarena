package arenaconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const twoScenarioDocs = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: first
spec:
  task_type: support
  description: first
  turns:
    - role: user
      content: hi
---
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: second
spec:
  task_type: support
  description: second
  turns:
    - role: user
      content: hi
`

// A second scenario after `---` used to be dropped without a word.
func TestLoadScenario_RejectsSeveralDocuments(t *testing.T) {
	_, err := LoadScenario(writeLoaderSchemaFile(t, twoScenarioDocs))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "holds 2 YAML documents")
}

func TestCheckSingleDocument(t *testing.T) {
	for name, doc := range map[string]string{
		"one":                 "a: 1\n",
		"leading separator":   "---\na: 1\n",
		"trailing separator":  "a: 1\n---\n",
		"empty second doc":    "a: 1\n---\n~\n",
		"unparseable is left": "a: [1\n---\nb: 2\n",
		"empty file":          "",
	} {
		assert.NoError(t, checkSingleDocument([]byte(doc), "f.yaml"), name)
	}
	err := checkSingleDocument([]byte("a: 1\n---\nb: 2\n---\nc: 3\n"), "f.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "f.yaml holds 3 YAML documents")
}

func TestLoadPersona_RejectsSeveralDocumentsAndUnknownKeys(t *testing.T) {
	persona := `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Persona
metadata:
  name: p
spec:
  id: p
  system_template: "You are a customer."
  style:
    verbosity: short
    challenge_level: low
    friction_tags: []
`
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}

	_, err := LoadPersona(write("ok.persona.yaml", persona))
	require.NoError(t, err)

	_, err = LoadPersona(write("two.persona.yaml", persona+"---\n"+persona))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "holds 2 YAML documents")

	_, err = LoadPersona(write("typo.persona.yaml", persona+"    formality: casual\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown field "formality"`)
	assert.Contains(t, err.Error(), "accepts only the keys in the persona schema")

	_, err = LoadPersona(write("typo.persona.json", `{"id":"p","system_template":"x","goalz":[]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown field "goalz"`)
}

func mockToolData(spec string) config.ToolData {
	return config.ToolData{FilePath: "tools/t.yaml", Data: []byte(
		"apiVersion: promptkit.altairalabs.ai/v1alpha1\nkind: Tool\nmetadata:\n  name: t\nspec:\n" + spec)}
}

// mock_template silently won over mock_result.
func TestValidateToolMocks(t *testing.T) {
	for name, spec := range map[string]string{
		"result":        "  mock_result: {ok: true}\n",
		"template":      "  mock_template: '{}'\n",
		"template file": "  mock_template_file: t.tmpl\n",
	} {
		cfg := &Config{LoadedTools: []config.ToolData{mockToolData(spec)}}
		assert.NoError(t, cfg.validateToolMocks(), name)
	}
	for name, spec := range map[string]string{
		"result and template":      "  mock_result: {ok: true}\n  mock_template: '{}'\n",
		"result file and template": "  mock_result_file: r.json\n  mock_template: '{}'\n",
		"result and template file": "  mock_result: 1\n  mock_template_file: t.tmpl\n",
	} {
		cfg := &Config{LoadedTools: []config.ToolData{mockToolData(spec)}}
		err := cfg.validateToolMocks()
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "tool t (tools/t.yaml)")
		assert.Contains(t, err.Error(), "not both")
	}
	unparseable := &Config{LoadedTools: []config.ToolData{{FilePath: "x.yaml", Data: []byte("a: [")}}}
	assert.NoError(t, unparseable.validateToolMocks())
}

func TestValidateConsentOverrides(t *testing.T) {
	cfg := &Config{LoadedScenarios: map[string]*Scenario{
		"ok":  {Turns: []TurnDefinition{{ConsentOverrides: map[string]string{"a": "grant", "b": "deny", "c": "timeout"}}}},
		"nil": nil,
	}}
	require.NoError(t, cfg.validateConsentOverrides())

	cfg.LoadedScenarios["bad"] = &Scenario{Turns: []TurnDefinition{{}, {ConsentOverrides: map[string]string{"loc": "denied"}}}}
	err := cfg.validateConsentOverrides()
	require.Error(t, err)
	assert.Equal(t, `scenario bad: turns[1].consent_overrides.loc: unknown value "denied" (want grant, deny, timeout)`,
		err.Error())
}

func TestIgnoredFieldWarnings_ProjectStage(t *testing.T) {
	cfg := &Config{LoadedScenarios: map[string]*Scenario{
		"a":   {ContextMetadata: &ContextMetadata{Domain: "d", ProjectStage: "production"}},
		"b":   {ContextMetadata: &ContextMetadata{Domain: "d"}},
		"c":   {},
		"nil": nil,
	}}
	assert.Equal(t, []string{"scenario a: context_metadata.project_stage is ignored and will be removed"},
		cfg.ignoredFieldWarnings())
}

func writeMockFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mock.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestCheckMockConfigFile(t *testing.T) {
	valid := `defaultResponse: hi
scenarios:
  s:
    defaultResponse: x
    turns:
      1: plain text
      2:
        response: calling
        tool_calls:
          - name: lookup
            arguments: {q: 1}
      3:
        type: multimodal
        parts:
          - type: image
            image_url: {url: "mock://x.png"}
    steps:
      step-a: {content: done}
selfplay:
  persona:
    turns:
      1: hello
`
	assert.NoError(t, CheckMockConfigFile(writeMockFile(t, valid)))
	assert.NoError(t, CheckMockConfigFile(writeMockFile(t, "")))
	assert.NoError(t, CheckMockConfigFile(filepath.Join(t.TempDir(), "missing.yaml")),
		"an unreadable file is left to the provider")

	err := CheckMockConfigFile(writeMockFile(t, "default_response: hi\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field default_response not found")

	err = CheckMockConfigFile(writeMockFile(t, `scenarios:
  s:
    turns:
      1: {respnse: typo}
      2:
        tool_calls:
          - {name: x, args: {}}
        parts:
          - {type: text, txt: x}
    steps:
      a: {contnt: y}
`))
	require.Error(t, err)
	for _, want := range []string{
		`scenarios.s.turns.1: unknown key "respnse"`,
		`scenarios.s.turns.2.tool_calls[0]: unknown key "args"`,
		`scenarios.s.turns.2.parts[0]: unknown key "txt"`,
		`scenarios.s.steps.a: unknown key "contnt"`,
	} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestValidateMockConfigs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mock.yaml"), []byte("default_response: x\n"), 0o600))
	cfg := &Config{
		ConfigDir: dir,
		LoadedProviders: map[string]*config.Provider{
			"real":    {Type: "openai", AdditionalConfig: map[string]interface{}{"mock_config": "mock.yaml"}},
			"no-file": {Type: "mock"},
			"nil":     nil,
		},
	}
	require.NoError(t, cfg.validateMockConfigs())

	cfg.LoadedProviders["mock"] = &config.Provider{Type: "mock", AdditionalConfig: map[string]interface{}{
		"mock_config": "mock.yaml",
	}}
	err := cfg.validateMockConfigs()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider mock: mock config "+filepath.Join(dir, "mock.yaml"))
}
