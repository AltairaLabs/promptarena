package redteam

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/engine"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"
)

const exampleDir = "../../examples/redteam-generated"

// exampleOptions are the options the example's README runs the command with.
func exampleOptions(dir string) Options {
	return Options{
		Dir:        dir,
		Name:       "shop-policy",
		Strategies: []string{StrategyScripted, "crescendo"},
		Converters: []string{ConverterNone, "best_of_n"},
		Judge:      "policy-judge",
		TaskType:   "support",
		Role:       DefaultRole,
		Turns:      DefaultTurns,
		Trials:     2,
		Baseline:   true,
		Source:     Source{Title: "objectives.jsonl", URLs: []string{"objectives.jsonl"}},
	}
}

func readObjectives(t *testing.T, path string) Objectives {
	t.Helper()
	objs, err := ReadObjectivesFile(path)
	require.NoError(t, err)
	return objs
}

// The example holds exactly what the command writes, so it can't drift from
// the generator.
func TestGenerate_MatchesTheExample(t *testing.T) {
	dir := t.TempDir()
	res, err := Generate(exampleOptions(dir), readObjectives(t, filepath.Join(exampleDir, "objectives.jsonl")))
	require.NoError(t, err)
	require.NotEmpty(t, res.Written)
	for _, rel := range res.Written {
		want, err := os.ReadFile(filepath.Join(exampleDir, filepath.FromSlash(rel)))
		require.NoError(t, err, "the example is missing %s; regenerate it as its README says", rel)
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), "%s is out of date; regenerate the example as its README says", rel)
	}
	assert.Equal(t, 22, res.Runs)
	assert.Equal(t, []string{"personas/crescendo.persona.yaml", "personas/direct.persona.yaml"}, res.Personas)
}

// copyDir copies the example into a temp dir, leaving out run output.
func copyDir(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			if d.Name() == "out" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o750)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o600)
	})
	require.NoError(t, err)
	return dst
}

// The generated scenarios load and run through the real engine: every
// attack is held off, and every benign request is answered.
func TestGenerate_ScenariosRunThroughTheEngine(t *testing.T) {
	t.Setenv("PROMPTKIT_SCHEMA_SOURCE", "local")
	dir := copyDir(t, exampleDir)
	for _, sub := range []string{"data", "scenarios", "personas"} {
		require.NoError(t, os.RemoveAll(filepath.Join(dir, sub)))
	}
	_, err := Generate(exampleOptions(dir), readObjectives(t, filepath.Join(dir, "objectives.jsonl")))
	require.NoError(t, err)

	eng, err := engine.NewEngineFromConfigFile(filepath.Join(dir, "config.arena.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Close() })
	plan, err := eng.GenerateRunPlan(nil, nil, nil, nil)
	require.NoError(t, err)
	runIDs, err := eng.ExecuteRuns(context.Background(), plan, 1)
	require.NoError(t, err)
	require.Len(t, runIDs, 22)

	store, ok := eng.GetStateStore().(*statestore.ArenaStateStore)
	require.True(t, ok)
	objectives := 0
	for _, id := range runIDs {
		r, err := store.GetResult(context.Background(), id)
		require.NoError(t, err)
		assert.Empty(t, r.Error, r.ScenarioID)
		assert.True(t, r.ConversationAssertions.Passed, r.ScenarioID)
		for _, c := range r.ConversationAssertions.Results {
			if c.Type == "objective" {
				objectives++
			}
		}
	}
	assert.Equal(t, 12, objectives, "the crescendo and baseline runs each report their objective")
}

func harmful(n int, set func(i int, o *Objective)) Objectives {
	var objs Objectives
	for i := range n {
		o := Objective{ID: "o" + string(rune('a'+i)), Text: "Objective " + string(rune('A'+i))}
		if set != nil {
			set(i, &o)
		}
		objs.Harmful = append(objs.Harmful, o)
	}
	return objs
}

func TestGenerate_DefaultsToARefusalProbe(t *testing.T) {
	dir := t.TempDir()
	res, err := Generate(Options{Dir: dir, Name: "set", Source: Source{Title: "set.jsonl"}}, harmful(2, nil))
	require.NoError(t, err)
	assert.Equal(t, []string{"data/set.jsonl", "scenarios/set-scripted.scenario.yaml"}, res.Written)
	assert.Empty(t, res.Personas)

	data, err := os.ReadFile(filepath.Join(dir, "scenarios", "set-scripted.scenario.yaml"))
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, "content: '{objective}'")
	assert.Contains(t, s, "- type: refusal\n")
	assert.Contains(t, s, "task_type: conversation")
	assert.NotContains(t, s, "trials:")
	assert.NotContains(t, res.ConfigSnippet(), "self_play")
	assert.NotContains(t, res.ConfigSnippet(), "judges")
}

func TestGenerate_DatasetObjectivesLoad(t *testing.T) {
	dir := t.TempDir()
	objs := harmful(2, func(i int, o *Objective) { o.Taxonomy = "jbb:cat-" + string(rune('a'+i)) })
	objs.Benign = []Objective{{ID: "b1", Text: "Benign one"}}
	opts := Options{
		Dir: dir, Name: "jbb", Strategies: []string{StrategyScripted, "refine"},
		Converters: []string{"base64", "rot13"}, Judge: "j", Trials: 3, MaxCostUSD: 0.5, Baseline: true,
		Taxonomy: []string{"owasp-llm:LLM01"},
		Source:   Source{Title: "JBB", URLs: []string{"https://example.test/a.csv"}, License: "MIT", Homepage: "https://h"},
	}
	res, err := Generate(opts, objs)
	require.NoError(t, err)
	assert.Len(t, res.Scenarios, 6) // 2 strategies x 2 converters, baseline, over-refusal

	data, err := os.ReadFile(filepath.Join(dir, "scenarios", "jbb-refine-base64.scenario.yaml"))
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, "# Source: https://example.test/a.csv\n# License: MIT, see https://h\n")
	assert.Contains(t, s, "base64-encoded")
	assert.Contains(t, s, "max_cost_usd: 0.5")
	assert.Contains(t, s, "- owasp-llm:LLM01\n")

	// Every scenario loads, with its data rows and taxonomy checked.
	for _, rel := range res.Scenarios {
		sc, err := arenaconfig.LoadScenario(filepath.Join(dir, filepath.FromSlash(rel)))
		require.NoError(t, err, rel)
		assert.NotEmpty(t, sc.Data.Rows, rel)
	}
	snippet := res.ConfigSnippet()
	assert.Contains(t, snippet, "personas/refine.persona.yaml")
	assert.Contains(t, snippet, "- name: j\n")
}

func TestGenerate_RefusesToOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	opts := Options{Dir: dir, Name: "s", Strategies: []string{"crescendo"}, Judge: "j", Source: Source{Title: "t"}}
	_, err := Generate(opts, harmful(1, nil))
	require.NoError(t, err)

	persona := filepath.Join(dir, "personas", "crescendo.persona.yaml")
	require.NoError(t, os.WriteFile(persona, []byte("edited"), 0o600))

	_, err = Generate(opts, harmful(1, nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--force")

	opts.Force = true
	res, err := Generate(opts, harmful(1, nil))
	require.NoError(t, err)
	assert.Equal(t, []string{"personas/crescendo.persona.yaml"}, res.KeptPersonas)
	got, err := os.ReadFile(persona)
	require.NoError(t, err)
	assert.Equal(t, "edited", string(got), "an existing persona is always kept")
}

func TestGenerate_OnlyBenignWritesOverRefusal(t *testing.T) {
	dir := t.TempDir()
	res, err := Generate(Options{Dir: dir, Name: "b", Strategies: []string{"crescendo"}, Source: Source{Title: "t"}},
		Objectives{Benign: []Objective{{ID: "x", Text: "Ask X", Prompt: "X?"}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"scenarios/b-over-refusal.scenario.yaml"}, res.Scenarios)
	data, err := os.ReadFile(filepath.Join(dir, "scenarios", "b-over-refusal.scenario.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "content: '{prompt}'")
	assert.Contains(t, string(data), "max_score: 0")
}

func TestGenerate_Validation(t *testing.T) {
	base := func() Options { return Options{Dir: t.TempDir(), Name: "n", Source: Source{Title: "t"}} }
	cases := map[string]struct {
		mod  func(*Options)
		objs Objectives
		want string
	}{
		"bad name":          {func(o *Options) { o.Name = "Bad Name" }, harmful(1, nil), "lowercase"},
		"unknown strategy":  {func(o *Options) { o.Strategies = []string{"direct"} }, harmful(1, nil), "unknown strategy"},
		"unknown converter": {func(o *Options) { o.Converters = []string{"rot47"} }, harmful(1, nil), "unknown converter"},
		"no objectives":     {func(*Options) {}, Objectives{}, "no objectives"},
		"partial taxonomy": {func(*Options) {}, harmful(2, func(i int, o *Objective) {
			if i == 0 {
				o.Taxonomy = "owasp-llm:LLM01"
			}
		}), "1 of 2 objectives have a taxonomy"},
		"partial pattern": {func(*Options) {}, harmful(2, func(i int, o *Objective) {
			if i == 1 {
				o.SuccessPattern = "x"
			}
		}), "success_pattern"},
		"persona without a check": {func(o *Options) { o.Strategies = []string{"refine"} }, harmful(1, nil), "--judge"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := base()
			tc.mod(&opts)
			_, err := Generate(opts, tc.objs)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestGenerate_PatternOnlyChecks(t *testing.T) {
	dir := t.TempDir()
	objs := harmful(1, func(_ int, o *Objective) { o.SuccessPattern = "done" })
	res, err := Generate(Options{Dir: dir, Name: "p", Strategies: []string{"multi-technique"}, Baseline: false,
		Source: Source{Title: "t"}}, objs)
	require.NoError(t, err)
	assert.Equal(t, []string{"scenarios/p-multi-technique.scenario.yaml"}, res.Scenarios)
	data, err := os.ReadFile(filepath.Join(dir, "scenarios", "p-multi-technique.scenario.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "content_includes")
	assert.NotContains(t, string(data), "llm_judge")
	assert.True(t, strings.HasPrefix(string(data), schemaLine))
}

func TestGenerate_WriteFailsUnderAFile(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	_, err := Generate(Options{Dir: blocker, Name: "w", Source: Source{Title: "t"}}, harmful(1, nil))
	require.Error(t, err)
}
