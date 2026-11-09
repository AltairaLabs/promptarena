package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/engine"
)

// TestCreateResultRepository_WritesEachFormat drives the repository that
// `promptarena run --formats …` builds through SaveResults and checks what lands
// on disk: each format writes its own file at the path its flag names, and a
// multi-format run writes every one of them.
func TestCreateResultRepository_WritesEachFormat(t *testing.T) {
	const run = "scenario-one" // every per-run report must name the run it was given
	cases := []struct {
		name    string
		formats []string
		// want maps each file expected under the temp dir to text it must contain.
		want map[string]string
	}{
		{name: "json writes one file per run into --out", formats: []string{"json"},
			want: map[string]string{"out/run-1.json": run}},
		{name: "junit writes --junit-file", formats: []string{"junit"},
			want: map[string]string{"reports/junit.xml": run}},
		{name: "csv writes results.csv into --out", formats: []string{"csv"},
			want: map[string]string{"out/results.csv": run}},
		{
			// A run without objectives is not listed, but the requested
			// --objectives-group-by must reach the report.
			name: "objectives writes json and markdown into --out", formats: []string{"objectives"},
			want: map[string]string{
				"out/objectives.json": `"by": "scenario"`,
				"out/objectives.md":   "No run had an objective.",
			},
		},
		{name: "markdown writes --markdown-file", formats: []string{"markdown"},
			want: map[string]string{"reports/results.md": run}},
		{name: "html is served by the markdown report", formats: []string{"html"},
			want: map[string]string{"reports/results.md": run}},
		{
			name:    "every requested format is written",
			formats: []string{"json", "junit", "csv", "markdown"},
			want: map[string]string{
				"out/results.csv": run, "out/run-1.json": run, "reports/junit.xml": run, "reports/results.md": run,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			params := &RunParameters{
				OutDir:            filepath.Join(dir, "out"),
				OutputFormats:     tc.formats,
				JUnitFile:         filepath.Join(dir, "reports", "junit.xml"),
				MarkdownFile:      filepath.Join(dir, "reports", "results.md"),
				ObjectivesGroupBy: []string{"scenario"},
			}

			repo, err := createResultRepository(params, "")
			require.NoError(t, err)
			require.NoError(t, repo.SaveResults([]engine.RunResult{
				{RunID: "run-1", ScenarioID: "scenario-one", ProviderID: "provider-one"},
			}))

			wantFiles := make([]string, 0, len(tc.want))
			for rel := range tc.want {
				wantFiles = append(wantFiles, rel)
			}
			sort.Strings(wantFiles)
			assert.Equal(t, wantFiles, filesUnder(t, dir))
			for rel, marker := range tc.want {
				data, err := os.ReadFile(filepath.Join(dir, rel)) //nolint:gosec // test file under t.TempDir
				require.NoError(t, err)
				assert.Contains(t, string(data), marker, rel)
			}
		})
	}
}

// filesUnder lists the regular files beneath dir, relative and slash-separated.
func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	require.NoError(t, filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	}))
	sort.Strings(files)
	return files
}
