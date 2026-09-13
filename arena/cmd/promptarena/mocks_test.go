package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/promptarena/v2/arena/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Basic coverage for loadRunResults filtering using staged fixtures.
func TestLoadRunResults_Filtering(t *testing.T) {
	tmp := t.TempDir()

	fixtures := []string{
		filepath.Join("..", "..", "templates", "testdata", "2025-11-30T19-49Z_openai-gpt4o_default_hardware-faults_18c25790.json"),
		filepath.Join("..", "..", "templates", "testdata", "2025-11-30T19-49Z_openai-gpt4o_default_redteam-selfplay_83be345a.json"),
	}

	for _, src := range fixtures {
		base := filepath.Base(src)
		dst := filepath.Join(tmp, base)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read fixture %s: %v", src, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("write temp fixture %s: %v", dst, err)
		}
	}

	results, err := loadRunResults(tmp, []string{"hardware-faults"}, nil)
	if err != nil {
		t.Fatalf("loadRunResults error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result after filtering, got %d", len(results))
	}

	if results[0].ScenarioID != "hardware-faults" {
		t.Fatalf("unexpected ScenarioID: %s", results[0].ScenarioID)
	}
}

func TestLoadRunResults_FilePath(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "single.json")

	run := engine.RunResult{
		RunID:      "run-1",
		ScenarioID: "s1",
		ProviderID: "p1",
	}
	writeJSON(t, file, run)

	results, err := loadRunResults(file, nil, nil)
	if err != nil {
		t.Fatalf("loadRunResults error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestLoadRunResults_NoMatches(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "single.json")
	run := engine.RunResult{
		RunID:      "run-1",
		ScenarioID: "s1",
		ProviderID: "p1",
	}
	writeJSON(t, file, run)

	_, err := loadRunResults(tmp, []string{"other"}, nil)
	if err == nil {
		t.Fatalf("expected error for unmatched filters")
	}
}

func TestLoadRunResults_SkipsNonResultJSON(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "index.json"), `{"not":"a-run"}`)
	runFile := filepath.Join(tmp, "run.json")
	run := engine.RunResult{
		RunID:      "run-1",
		ScenarioID: "s1",
		ProviderID: "p1",
	}
	writeJSON(t, runFile, run)

	results, err := loadRunResults(tmp, nil, nil)
	if err != nil {
		t.Fatalf("loadRunResults error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after skipping non-run files, got %d", len(results))
	}
}

func TestLoadRunResults_NoJSONFiles(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "readme.txt"), "hello")

	if _, err := loadRunResults(tmp, nil, nil); err == nil {
		t.Fatalf("expected error when no JSON files present")
	}
}

func writeJSON(t *testing.T, path string, run engine.RunResult) {
	t.Helper()
	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal run: %v", err)
	}
	writeFile(t, path, string(data))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestParseRun_DistinguishesUnreadableFromNotARunFile covers parseRun's two
// non-success exits, which mean different things to the caller.
//
// A directory of results legitimately contains JSON that is not a run — those
// must be skipped quietly, or every scan would fail on the first stray file.
// A file that cannot be READ is different: it means the scan is incomplete,
// and swallowing it would silently drop runs from the mock set.
func TestParseRun_DistinguishesUnreadableFromNotARunFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("an unreadable path is an error", func(t *testing.T) {
		_, ok, err := parseRun(filepath.Join(dir, "absent.json"))
		require.Error(t, err, "a missing file must not be silently skipped")
		assert.False(t, ok)
		assert.Contains(t, err.Error(), "absent.json", "the error must name the file")
	})

	t.Run("malformed JSON is skipped, not fatal", func(t *testing.T) {
		p := filepath.Join(dir, "broken.json")
		require.NoError(t, os.WriteFile(p, []byte("{not json"), 0o600))

		_, ok, err := parseRun(p)
		require.NoError(t, err, "a stray non-JSON file must not fail the scan")
		assert.False(t, ok)
	})

	t.Run("valid JSON that is not a run is skipped", func(t *testing.T) {
		p := filepath.Join(dir, "other.json")
		require.NoError(t, os.WriteFile(p, []byte(`{"hello":"world"}`), 0o600))

		_, ok, err := parseRun(p)
		require.NoError(t, err)
		assert.False(t, ok, "JSON without the run identity fields is not a run")
	})

	t.Run("a run missing any identity field is skipped", func(t *testing.T) {
		// Each of the three is required; a run without one cannot be keyed
		// back to a scenario or provider and would corrupt the mock set.
		for name, body := range map[string]string{
			"no run id":      `{"ScenarioID":"s","ProviderID":"p"}`,
			"no scenario id": `{"RunID":"r","ProviderID":"p"}`,
			"no provider id": `{"RunID":"r","ScenarioID":"s"}`,
		} {
			p := filepath.Join(dir, "partial.json")
			require.NoError(t, os.WriteFile(p, []byte(body), 0o600))

			_, ok, err := parseRun(p)
			require.NoError(t, err, name)
			assert.False(t, ok, "%s: must be skipped", name)
		}
	})

	t.Run("a complete run parses", func(t *testing.T) {
		p := filepath.Join(dir, "good.json")
		require.NoError(t, os.WriteFile(p,
			[]byte(`{"RunID":"r1","ScenarioID":"s1","ProviderID":"p1"}`), 0o600))

		res, ok, err := parseRun(p)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "r1", res.RunID)
		assert.Equal(t, "s1", res.ScenarioID)
		assert.Equal(t, "p1", res.ProviderID)
	})
}

// TestCollectJSONFiles_WalksADirectoryOrTakesAFile covers collectJSONFiles.
// The input is whatever the operator typed after --input, so both shapes have
// to work, and a non-JSON file or nested directory must be skipped rather than
// handed to parseRun as a candidate run.
func TestCollectJSONFiles_WalksADirectoryOrTakesAFile(t *testing.T) {
	dir := t.TempDir()

	write := func(name string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("{}"), 0o600))
		return p
	}
	a := write("a.json")
	write("notes.txt")
	b := write("b.json")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "nested"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nested", "c.json"), []byte("{}"), 0o600))

	t.Run("a directory yields only its own .json files", func(t *testing.T) {
		got, err := collectJSONFiles(dir)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{a, b}, got,
			"only top-level .json files belong; nested dirs and .txt do not")
	})

	t.Run("a file path is taken as-is", func(t *testing.T) {
		got, err := collectJSONFiles(a)
		require.NoError(t, err)
		assert.Equal(t, []string{a}, got)
	})

	t.Run("a file path is taken even without a .json suffix", func(t *testing.T) {
		// An explicit path is the operator naming a file; second-guessing the
		// extension would reject a validly-named result they pointed at.
		txt := filepath.Join(dir, "notes.txt")
		got, err := collectJSONFiles(txt)
		require.NoError(t, err)
		assert.Equal(t, []string{txt}, got)
	})

	t.Run("a missing path is an error naming the input", func(t *testing.T) {
		_, err := collectJSONFiles(filepath.Join(dir, "absent"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "input")
	})

	t.Run("an empty directory is an error naming the path", func(t *testing.T) {
		// Not an empty result: a directory with no results means the operator
		// pointed at the wrong place, and returning zero mocks silently would
		// look like a successful run that generated nothing.
		empty := t.TempDir()
		_, err := collectJSONFiles(empty)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no JSON result files found")
		assert.Contains(t, err.Error(), empty)
	})
}

// TestMatchesFilters_EmptyAllowMeansAll covers matchesFilters. An empty allow
// set means "no filter" — treating it as "allow nothing" would make an
// unfiltered run silently produce zero mocks.
func TestMatchesFilters_EmptyAllowMeansAll(t *testing.T) {
	res := &engine.RunResult{ScenarioID: "s1", ProviderID: "p1"}

	assert.True(t, matchesFilters(res, nil, nil), "no filters must match everything")
	assert.True(t, matchesFilters(res, map[string]bool{"s1": true}, nil))
	assert.True(t, matchesFilters(res, nil, map[string]bool{"p1": true}))
	assert.True(t, matchesFilters(res, map[string]bool{"s1": true}, map[string]bool{"p1": true}))

	assert.False(t, matchesFilters(res, map[string]bool{"other": true}, nil),
		"a scenario filter that excludes this run must not match")
	assert.False(t, matchesFilters(res, nil, map[string]bool{"other": true}),
		"a provider filter that excludes this run must not match")
	assert.False(t, matchesFilters(res, map[string]bool{"s1": true}, map[string]bool{"other": true}),
		"both filters must pass, not either")
}

// TestMocksGenerateCmd_DryRunPrintsWithoutWriting drives the `mocks generate`
// command body, which no test reached.
//
// --dry-run is the flag an operator uses to check what would be produced
// before overwriting a committed mock file. If it wrote anyway the damage is
// silent and already done by the time they read the output, so the two things
// worth pinning are that the YAML reaches stdout and that nothing lands on
// disk.
func TestMocksGenerateCmd_DryRunPrintsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	runPath := filepath.Join(dir, "run.json")
	require.NoError(t, os.WriteFile(runPath, []byte(
		`{"RunID":"r1","ScenarioID":"s1","ProviderID":"p1"}`), 0o600))

	outDir := filepath.Join(dir, "generated")

	// Package-level flag state is shared; restore it so neighbouring tests
	// are unaffected.
	savedInput, savedOut, savedDry := mocksInput, mocksOutput, mocksDryRun
	t.Cleanup(func() { mocksInput, mocksOutput, mocksDryRun = savedInput, savedOut, savedDry })
	mocksInput, mocksOutput, mocksDryRun = runPath, outDir, true

	var stdout bytes.Buffer
	mocksGenerateCmd.SetOut(&stdout)
	t.Cleanup(func() { mocksGenerateCmd.SetOut(nil) })

	require.NoError(t, mocksGenerateCmd.RunE(mocksGenerateCmd, nil))

	assert.Contains(t, stdout.String(), "dry-run",
		"a dry run must label its output as such")
	assert.NoDirExists(t, outDir, "a dry run must not write files")
}

// TestMocksGenerateCmd_WritesFiles is the same path with the flag off: the
// output directory appears and the command reports what it produced.
func TestMocksGenerateCmd_WritesFiles(t *testing.T) {
	dir := t.TempDir()
	runPath := filepath.Join(dir, "run.json")
	require.NoError(t, os.WriteFile(runPath, []byte(
		`{"RunID":"r1","ScenarioID":"s1","ProviderID":"p1"}`), 0o600))

	outPath := filepath.Join(dir, "mocks.yaml")

	savedInput, savedOut, savedDry := mocksInput, mocksOutput, mocksDryRun
	t.Cleanup(func() { mocksInput, mocksOutput, mocksDryRun = savedInput, savedOut, savedDry })
	mocksInput, mocksOutput, mocksDryRun = runPath, outPath, false

	var stdout bytes.Buffer
	mocksGenerateCmd.SetOut(&stdout)
	t.Cleanup(func() { mocksGenerateCmd.SetOut(nil) })

	require.NoError(t, mocksGenerateCmd.RunE(mocksGenerateCmd, nil))
	assert.Contains(t, stdout.String(), "Generated")
}

// TestMocksGenerateCmd_BadInputFails pins that an unreadable input fails the
// command rather than producing an empty mock file, which would overwrite a
// good one with nothing.
func TestMocksGenerateCmd_BadInputFails(t *testing.T) {
	savedInput := mocksInput
	t.Cleanup(func() { mocksInput = savedInput })
	mocksInput = filepath.Join(t.TempDir(), "absent")

	require.Error(t, mocksGenerateCmd.RunE(mocksGenerateCmd, nil))
}
