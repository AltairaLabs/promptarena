package arenaconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeDataTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

const dataScenarioYAML = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: capitals
spec:
  id: capitals
  task_type: support
  description: capitals
  data:
    file: data/capitals.jsonl
  turns:
    - role: user
      content: "What is the capital of {country}?"
      assertions:
        - type: content_includes
          params:
            patterns: ["{answer}"]
`

func TestLoadScenario_ReadsDataRelativeToScenarioFile(t *testing.T) {
	dir := t.TempDir()
	writeDataTestFile(t, dir, "data/capitals.jsonl",
		`{"id": "fr", "country": "France", "answer": "Paris"}`+"\n\n"+
			`{"country": "Japan", "answer": "Tokyo"}`+"\n")
	path := writeDataTestFile(t, dir, "capitals.scenario.yaml", dataScenarioYAML)

	s, err := LoadScenario(path)
	require.NoError(t, err)
	require.Len(t, s.Data.Rows, 2)
	assert.Equal(t, "fr", s.Data.Rows[0].ID)
	assert.Equal(t, "2", s.Data.Rows[1].ID, "rows without an id are named by position")
	assert.Equal(t, "Tokyo", s.Data.Rows[1].Values["answer"])
}

func TestLoadScenario_DataRejectsMissingColumn(t *testing.T) {
	dir := t.TempDir()
	writeDataTestFile(t, dir, "data/capitals.jsonl", `{"country": "France", "anser": "Paris"}`+"\n")
	path := writeDataTestFile(t, dir, "capitals.scenario.yaml", dataScenarioYAML)

	_, err := LoadScenario(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "row 1 has no column for {answer}")
}

func TestLoadScenario_DataAndPerturbationsAreExclusive(t *testing.T) {
	dir := t.TempDir()
	writeDataTestFile(t, dir, "data/capitals.jsonl", `{"country": "France", "answer": "Paris"}`+"\n")
	path := writeDataTestFile(t, dir, "capitals.scenario.yaml", dataScenarioYAML+`    - role: user
      content: "And {city}?"
      perturbations:
        city: [Lyon]
`)
	_, err := LoadScenario(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data and perturbations are mutually exclusive")
}

func TestLoadConfig_InlineScenarioDataResolvesAgainstConfig(t *testing.T) {
	dir := t.TempDir()
	writeDataTestFile(t, dir, "rows.csv", "country,answer\nFrance,Paris\n")
	configPath := writeArenaConfig(t, dir, `  providers: []
  scenario_specs:
    capitals:
      task_type: support
      description: capitals
      data:
        file: rows.csv
      turns:
        - role: user
          content: "Capital of {country}?"
  defaults:
    concurrency: 1
`)
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)
	require.Len(t, cfg.LoadedScenarios["capitals"].Data.Rows, 1)
	assert.Equal(t, "France", cfg.LoadedScenarios["capitals"].Data.Rows[0].Values["country"])
}

func TestReadDataFile_JSONLValues(t *testing.T) {
	dir := t.TempDir()
	path := writeDataTestFile(t, dir, "rows.jsonl",
		`{"item": {"n": 3, "f": 1.50, "ok": true, "none": null, "tags": ["a", "b"]}}`+"\n")

	rows, err := readDataFile(path, "item")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, map[string]string{
		"n": "3", "f": "1.50", "ok": "true", "none": "", "tags": `["a","b"]`,
	}, rows[0].Values)
}

func TestReadDataFile_Errors(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]struct {
		name, content, field, want string
	}{
		"not an object": {name: "a.jsonl", content: "[1]\n", want: "line 1: not a JSON object"},
		"field missing": {name: "b.jsonl", content: `{"x": 1}` + "\n", field: "item", want: `field "item" is not an object`},
		"empty":         {name: "c.jsonl", content: "\n", want: "no rows"},
		"header only":   {name: "d.csv", content: "a,b\n", want: "no rows"},
		"ragged csv":    {name: "e.csv", content: "a,b\n1\n", want: "wrong number of fields"},
		"duplicate ids": {name: "f.csv", content: "id,a\nx,1\nx,2\n", want: `rows 1 and 2 share id "x"`},
		"extension":     {name: "g.json", content: "{}", want: "unsupported extension"},
		"two on a line": {name: "h.jsonl", content: `{"a": 1}{"a": 2}` + "\n", want: "line 1: more than one JSON value"},
		"repeated col":  {name: "i.csv", content: "id,q,q\n1,a,b\n", want: `header repeats column "q"`},
		"unnamed col":   {name: "j.csv", content: "id,q,\n1,a,b\n", want: "header column 3 has no name"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := readDataFile(writeDataTestFile(t, dir, tc.name, tc.content), tc.field)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestReadDataFile_CSVStripsBOM(t *testing.T) {
	path := writeDataTestFile(t, t.TempDir(), "rows.csv", "\ufeffid, q\nr1,\"a, b\"\n")
	rows, err := readDataFile(path, "")
	require.NoError(t, err)
	assert.Equal(t, "r1", rows[0].ID)
	assert.Equal(t, "a, b", rows[0].Values["q"])
}

func TestScenarioDataValidate(t *testing.T) {
	assert.ErrorContains(t, (&Scenario{Data: &ScenarioData{}}).Validate(), "data.file is required")
	assert.ErrorContains(t,
		(&Scenario{Data: &ScenarioData{File: "rows.csv", Field: "item"}}).Validate(),
		"data.field applies to JSONL files only")
	assert.NoError(t, (&Scenario{Data: &ScenarioData{File: "rows.jsonl", Field: "item"}}).Validate())
}

func TestSubstituteDataPlaceholders(t *testing.T) {
	values := map[string]string{"a": "1", "b.c": "2", "loop": "{a}"}
	cases := map[string]string{
		"{a} and {b.c}":       "1 and 2",
		"{missing} stays":     "{missing} stays",
		"{{a}} is a template": "{{a}} is a template",
		"{a}} is not":         "{a}} is not",
		"{1a} {} { a } {":     "{1a} {} { a } {",
		`{"a": 1}`:            `{"a": 1}`,
		"{loop}":              "{a}",
		"a{a}b":               "a1b",
	}
	for in, want := range cases {
		assert.Equal(t, want, SubstituteDataPlaceholders(in, values), in)
	}
	assert.Equal(t, []string{"a", "b.c", "x"}, DataPlaceholders("{a} {{t}} {b.c} {x}"))
}

func TestForDataRow_SubstitutesWithoutTouchingOriginal(t *testing.T) {
	original := &Scenario{
		Variables: map[string]string{"topic": "{country}"},
		Turns: []TurnDefinition{{
			Role:    "user",
			Content: "Capital of {country}?",
			Parts:   []TurnContentPart{{Type: "text", Text: "{country}"}},
			Assertions: []AssertionConfig{{
				Type:    "content_includes",
				Message: "expected {answer}",
				Params: map[string]interface{}{
					"patterns": []interface{}{"{answer}"},
					"nested":   map[string]interface{}{"x": "{answer}", "n": 3},
				},
			}},
		}},
		ConversationAssertions: []AssertionConfig{{
			Type:   "llm_judge",
			Params: map[string]interface{}{"criteria": "Says {answer}"},
		}},
		SeedMemories: []SeedMemoryEntry{{Content: "User asked about {country}"}},
	}
	row := DataRow{ID: "fr", Values: map[string]string{"country": "France", "answer": "Paris"}}

	got := original.ForDataRow(row)

	assert.Equal(t, "Capital of France?", got.Turns[0].Content)
	assert.Equal(t, "France", got.Turns[0].Parts[0].Text)
	assert.Equal(t, "France", got.Variables["topic"])
	a := got.Turns[0].Assertions[0]
	assert.Equal(t, "expected Paris", a.Message)
	assert.Equal(t, []interface{}{"Paris"}, a.Params["patterns"])
	assert.Equal(t, map[string]interface{}{"x": "Paris", "n": 3}, a.Params["nested"])
	assert.Equal(t, "Says Paris", got.ConversationAssertions[0].Params["criteria"])
	assert.Equal(t, "User asked about France", got.SeedMemories[0].Content)

	assert.Equal(t, "Capital of {country}?", original.Turns[0].Content)
	assert.Equal(t, "{country}", original.Turns[0].Parts[0].Text)
	assert.Equal(t, "{country}", original.Variables["topic"])
	assert.Equal(t, "expected {answer}", original.Turns[0].Assertions[0].Message)
	assert.Equal(t, []interface{}{"{answer}"}, original.Turns[0].Assertions[0].Params["patterns"])
	assert.Equal(t, "Says {answer}", original.ConversationAssertions[0].Params["criteria"])
	assert.Equal(t, "User asked about {country}", original.SeedMemories[0].Content)
}
