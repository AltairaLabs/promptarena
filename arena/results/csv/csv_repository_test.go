package csv

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/engine"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func ptr(f float64) *float64 { return &f }

// readCSV returns the header and the rows keyed by column name.
func readCSV(t *testing.T, dir string) ([]string, []map[string]string) {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, FileName))
	require.NoError(t, err)
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	require.NoError(t, err)
	require.NotEmpty(t, records)
	header := records[0]
	rows := make([]map[string]string, 0, len(records)-1)
	for _, rec := range records[1:] {
		require.Len(t, rec, len(header))
		row := map[string]string{}
		for i, col := range header {
			row[col] = rec[i]
		}
		rows = append(rows, row)
	}
	return header, rows
}

func armRun() engine.RunResult {
	return engine.RunResult{
		RunID:      "run-a",
		ScenarioID: "build-service",
		ProviderID: "claude",
		Region:     "default",
		Labels:     map[string]string{"arm": "spec"},
		Duration:   1500 * time.Millisecond,
		Cost: types.CostInfo{
			InputTokens: 1200, OutputTokens: 300, CachedTokens: 800, TotalCost: 0.0425,
		},
		ToolStats: &types.ToolStats{TotalCalls: 4, ByTool: map[string]int{"Bash": 3, "Edit": 1}},
		ConversationAssertions: engine.AssertionsSummary{
			Passed: true, Total: 2,
			Results: []assertions.ConversationValidationResult{
				{Type: "tool_exec", Passed: true, Details: map[string]interface{}{"score": 0.75}},
				{Type: "llm_judge_session", Passed: true},
			},
		},
		EvalResults: []evals.EvalResult{{EvalID: "semgrep_findings", Score: ptr(3)}},
	}
}

func TestSaveResults_WritesFixedAndDynamicColumns(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, NewRepository(dir).SaveResults([]engine.RunResult{armRun()}))

	header, rows := readCSV(t, dir)
	assert.Equal(t, fixedColumnNames(), header[:len(fixedColumns)])
	assert.Equal(t, []string{
		"label.arm",
		"assertion.1.tool_exec.passed", "assertion.1.tool_exec.score",
		"assertion.2.llm_judge_session.passed",
		"eval.semgrep_findings.score",
		"tool.Bash.count", "tool.Edit.count",
	}, header[len(fixedColumns):])

	require.Len(t, rows, 1)
	row := rows[0]
	assert.Equal(t, "run-a", row["run_id"])
	assert.Equal(t, "true", row["passed"])
	assert.Equal(t, "false", row["skipped"])
	assert.Equal(t, "1500", row["duration_ms"])
	assert.Equal(t, "1200", row["input_tokens"])
	assert.Equal(t, "800", row["cached_tokens"])
	assert.Equal(t, "0.0425", row["total_cost_usd"])
	assert.Equal(t, "4", row["tool_calls_total"])
	assert.Equal(t, "spec", row["label.arm"])
	assert.Equal(t, "0.75", row["assertion.1.tool_exec.score"])
	assert.Equal(t, "3", row["eval.semgrep_findings.score"])
	assert.Equal(t, "3", row["tool.Bash.count"])
}

func TestSaveResults_UnionOfColumnsLeavesMissingCellsEmpty(t *testing.T) {
	other := engine.RunResult{
		RunID: "run-b", ScenarioID: "build-service", ProviderID: "claude",
		Labels: map[string]string{"arm": "one-line", "trial_set": "2"},
	}
	dir := t.TempDir()
	require.NoError(t, NewRepository(dir).SaveResults([]engine.RunResult{armRun(), other}))

	header, rows := readCSV(t, dir)
	assert.Contains(t, header, "label.trial_set")
	assert.Empty(t, rows[0]["label.trial_set"])
	assert.Equal(t, "2", rows[1]["label.trial_set"])
	assert.Empty(t, rows[1]["tool.Bash.count"])
	assert.Equal(t, "0", rows[1]["tool_calls_total"], "nil ToolStats means no calls")
}

func TestSaveResults_SkippedAndCappedRuns(t *testing.T) {
	skipped := engine.RunResult{
		RunID: "run-skip", ScenarioID: "s", ProviderID: "p", Skipped: true, SkipReason: "503",
	}
	capped := engine.RunResult{
		RunID: "run-cap", ScenarioID: "s", ProviderID: "p",
		Error: "provider stage: max rounds (2) exceeded", StopReason: "max_rounds",
	}
	dir := t.TempDir()
	require.NoError(t, NewRepository(dir).SaveResults([]engine.RunResult{skipped, capped}))

	_, rows := readCSV(t, dir)
	assert.Equal(t, "false", rows[0]["passed"], "a skipped run is never a pass")
	assert.Equal(t, "true", rows[0]["skipped"])
	assert.Equal(t, "false", rows[1]["passed"])
	assert.Equal(t, "max_rounds", rows[1]["stop_reason"])
}

func TestSaveResults_EscapesAwkwardCells(t *testing.T) {
	res := engine.RunResult{
		RunID: "run-x", ScenarioID: "s", ProviderID: "p",
		Error: "tool said \"no\", then\nquit",
	}
	dir := t.TempDir()
	require.NoError(t, NewRepository(dir).SaveResults([]engine.RunResult{res}))

	_, rows := readCSV(t, dir)
	assert.Equal(t, res.Error, rows[0]["error"])
}

func TestSaveResults_AssertionColumnsKeepNumericOrder(t *testing.T) {
	results := make([]assertions.ConversationValidationResult, 11)
	for i := range results {
		results[i] = assertions.ConversationValidationResult{Type: "contains", Passed: true}
	}
	res := engine.RunResult{
		RunID: "run-many", ScenarioID: "s", ProviderID: "p",
		ConversationAssertions: engine.AssertionsSummary{Total: 11, Passed: true, Results: results},
	}
	dir := t.TempDir()
	require.NoError(t, NewRepository(dir).SaveResults([]engine.RunResult{res}))

	header, _ := readCSV(t, dir)
	dyn := header[len(fixedColumns):]
	assert.Equal(t, "assertion.2.contains.passed", dyn[1])
	assert.Equal(t, "assertion.10.contains.passed", dyn[9])
}

func TestSaveResults_RejectsInvalidResults(t *testing.T) {
	repo := NewRepository(t.TempDir())
	require.Error(t, repo.SaveResults(nil))
	require.Error(t, repo.SaveResults([]engine.RunResult{{RunID: "r"}}))
}

func TestSaveResults_FailsWhenOutputDirUnusable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	err := NewRepository(file).SaveResults([]engine.RunResult{armRun()})
	require.Error(t, err)
}

func TestNumeric(t *testing.T) {
	for _, v := range []any{float64(2), float32(2), 2, int64(2)} {
		n, ok := numeric(v)
		assert.True(t, ok)
		assert.InDelta(t, 2.0, n, 1e-9)
	}
	_, ok := numeric("2")
	assert.False(t, ok)
}

func TestUnsupportedOperations(t *testing.T) {
	repo := NewRepository(t.TempDir())
	assert.NoError(t, repo.SaveSummary(nil))
	assert.False(t, repo.SupportsStreaming())
	_, err := repo.LoadResults()
	assert.Error(t, err)
	assert.Error(t, repo.SaveResult(&engine.RunResult{}))
}

func TestSaveResults_FlattensScalarEvalData(t *testing.T) {
	res := engine.RunResult{
		RunID: "run-bax", ScenarioID: "s", ProviderID: "p",
		EvalResults: []evals.EvalResult{{
			EvalID: "baxbench",
			Score:  ptr(0.5),
			Details: map[string]any{
				"functional_score": 0.75,
				"security_score":   1,
				"app_started":      true,
				"score":            0.1, // would shadow the eval's own score
				"note":             "strings stay in JSON",
				"cwes":             map[string]any{"CWE-89": false},
			},
		}},
	}
	dir := t.TempDir()
	require.NoError(t, NewRepository(dir).SaveResults([]engine.RunResult{res}))

	header, rows := readCSV(t, dir)
	row := rows[0]
	assert.Equal(t, "0.5", row["eval.baxbench.score"])
	assert.Equal(t, "0.75", row["eval.baxbench.functional_score"])
	assert.Equal(t, "1", row["eval.baxbench.security_score"])
	assert.Equal(t, "true", row["eval.baxbench.app_started"])
	assert.NotContains(t, header, "eval.baxbench.note")
	assert.NotContains(t, header, "eval.baxbench.cwes")
}
