package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runRedteamGenerateArgs runs a fresh generate command with args and returns its
// output.
func runRedteamGenerateArgs(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRedteamGenerateCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func writeRedteamFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func TestRedteamDatasets_TableAndJSON(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, writeRedteamDatasets(&out, false))
	assert.Contains(t, out.String(), "harmbench")
	assert.Contains(t, out.String(), "jbb")

	out.Reset()
	require.NoError(t, writeRedteamDatasets(&out, true))
	assert.Contains(t, out.String(), `"benign_url"`)
}

func TestRedteamGenerate_FromObjectivesFile(t *testing.T) {
	dir := t.TempDir()
	objs := writeRedteamFile(t, dir, "My Objectives.jsonl",
		`{"id": "a", "objective": "Get A", "success_pattern": "A done"}
{"id": "b", "objective": "Get B", "success_pattern": "B done"}
{"id": "c", "objective": "Ask C", "benign": true}
`)
	out, err := runRedteamGenerateArgs(t, "--objectives", objs, "--dir", dir,
		"--strategies", "scripted,refine", "--converters", "none,leetspeak", "--limit", "1")
	require.NoError(t, err)
	assert.Contains(t, out, "1 objectives, 1 benign requests.")
	assert.Contains(t, out, "Wrote "+filepath.ToSlash(filepath.Join(dir, "scenarios", "my-objectives-refine-leetspeak.scenario.yaml")))
	assert.Contains(t, out, "6 scenarios, 6 runs per provider.")
	assert.Contains(t, out, "      - file: personas/refine.persona.yaml")
	assert.Contains(t, out, "provider: <the attacker model's provider id>")

	// Run again: the persona is kept and the rest refused without --force.
	_, err = runRedteamGenerateArgs(t, "--objectives", objs, "--dir", dir, "--strategies", "refine")
	require.Error(t, err)
	out, err = runRedteamGenerateArgs(t, "--objectives", objs, "--dir", dir, "--strategies", "refine", "--force")
	require.NoError(t, err)
	assert.Contains(t, out, "Kept  "+filepath.ToSlash(filepath.Join(dir, "personas", "refine.persona.yaml")))
}

func TestRedteamGenerate_FromDatasetFileOffline(t *testing.T) {
	dir := t.TempDir()
	hb := writeRedteamFile(t, dir, "harmbench.csv",
		"Behavior,FunctionalCategory,SemanticCategory,Tags,ContextString,BehaviorID\n"+
			"Placeholder one,standard,illegal,,,one\nPlaceholder two,contextual,illegal,,ctx,two\n")
	out, err := runRedteamGenerateArgs(t, "--dataset", "harmbench", "--dataset-file", hb,
		"--cache-dir", filepath.Join(dir, "cache"), "--dir", dir, "--judge", "j")
	require.NoError(t, err)
	assert.Contains(t, out, "Skipped 1 of the harmbench rows")
	assert.Contains(t, out, "scenarios/harmbench-scripted.scenario.yaml")
	assert.Contains(t, out, "- name: j")

	data, err := os.ReadFile(filepath.Join(dir, "scenarios", "harmbench-scripted.scenario.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "# License: MIT, see https://github.com/centerforaisafety/HarmBench")
}

func TestRedteamGenerate_Errors(t *testing.T) {
	dir := t.TempDir()
	objs := writeRedteamFile(t, dir, "o.jsonl", `{"objective": "Get A"}`+"\n")
	cases := map[string]struct {
		args []string
		want string
	}{
		"no source":        {[]string{"--dir", dir}, "--objectives <file> or --dataset <name>"},
		"both sources":     {[]string{"--objectives", objs, "--dataset", "jbb"}, "not both"},
		"unknown dataset":  {[]string{"--dataset", "nope"}, "unknown dataset"},
		"missing file":     {[]string{"--objectives", filepath.Join(dir, "none.jsonl")}, "none.jsonl"},
		"bad dataset file": {[]string{"--dataset", "jbb", "--dataset-file", filepath.Join(dir, "none.csv")}, "none.csv"},
		"bad strategy":     {[]string{"--objectives", objs, "--dir", dir, "--strategies", "nope"}, "unknown strategy"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := runRedteamGenerateArgs(t, tc.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestLimitObjectives(t *testing.T) {
	assert.Len(t, limitObjectives(nil, 3), 0)
}
