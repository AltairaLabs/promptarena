// Package csv provides CSV result output for Arena.
// This package implements the ResultRepository interface to write one row
// per run — labels, assertion outcomes, eval scores, tokens, cost, duration
// and tool counts — for charting and analysis outside Arena.
package csv

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/engine"
	"github.com/AltairaLabs/promptarena/v2/arena/results"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
)

// FileName is the CSV file written into the output directory.
const FileName = "results.csv"

// outputDirPerm is the mode for a newly created output directory.
const outputDirPerm = 0o750

// fixedColumn is a column every row carries, with how to read it from a run.
type fixedColumn struct {
	name  string
	value func(*engine.RunResult) string
}

// fixedColumns lead every row, in this order. Per-run dynamic columns
// (labels, assertions, evals, tools) follow, sorted.
var fixedColumns = []fixedColumn{
	{"run_id", func(r *engine.RunResult) string { return r.RunID }},
	{"scenario_id", func(r *engine.RunResult) string { return r.ScenarioID }},
	{"provider_id", func(r *engine.RunResult) string { return r.ProviderID }},
	{"region", func(r *engine.RunResult) string { return r.Region }},
	{"passed", func(r *engine.RunResult) string { return strconv.FormatBool(!r.Skipped && results.RunPassed(r)) }},
	{"skipped", func(r *engine.RunResult) string { return strconv.FormatBool(r.Skipped) }},
	{"stop_reason", func(r *engine.RunResult) string { return r.StopReason }},
	{"error", func(r *engine.RunResult) string { return r.Error }},
	{"duration_ms", func(r *engine.RunResult) string { return strconv.FormatInt(r.Duration.Milliseconds(), 10) }},
	{"input_tokens", func(r *engine.RunResult) string { return strconv.Itoa(r.Cost.InputTokens) }},
	{"output_tokens", func(r *engine.RunResult) string { return strconv.Itoa(r.Cost.OutputTokens) }},
	{"cached_tokens", func(r *engine.RunResult) string { return strconv.Itoa(r.Cost.CachedTokens) }},
	{"total_cost_usd", func(r *engine.RunResult) string { return formatFloat(r.Cost.TotalCost) }},
	{"tool_calls_total", func(r *engine.RunResult) string {
		if r.ToolStats == nil {
			return "0"
		}
		return strconv.Itoa(r.ToolStats.TotalCalls)
	}},
}

// fixedColumnNames returns the fixed columns' names in order.
func fixedColumnNames() []string {
	names := make([]string, len(fixedColumns))
	for i, c := range fixedColumns {
		names[i] = c.name
	}
	return names
}

// Column-group prefixes, in output order.
const (
	prefixLabel     = "label."
	prefixAssertion = "assertion."
	prefixEval      = "eval."
	prefixTool      = "tool."
)

// Repository writes all results to a single CSV file.
type Repository struct {
	outputDir string
}

// NewRepository creates a CSV repository that writes FileName into
// outputDir.
func NewRepository(outputDir string) *Repository {
	return &Repository{outputDir: outputDir}
}

// SaveResults writes one row per run. The column set is the union across
// runs; a run without a given label, assertion, eval or tool leaves that cell
// empty.
func (r *Repository) SaveResults(runResults []engine.RunResult) error {
	if err := results.ValidateResults(runResults); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	if err := os.MkdirAll(r.outputDir, outputDirPerm); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	rows := make([]map[string]string, len(runResults))
	dynamic := map[string]bool{}
	for i := range runResults {
		rows[i] = rowFor(&runResults[i])
		for col := range rows[i] {
			dynamic[col] = true
		}
	}
	for _, c := range fixedColumns {
		delete(dynamic, c.name)
	}
	header := append(fixedColumnNames(), sortedDynamicColumns(dynamic)...)

	path := filepath.Join(r.outputDir, FileName)
	f, err := os.Create(path) // #nosec G304 -- path is the configured output dir + fixed name
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	w := csv.NewWriter(f)
	writeErr := w.Write(header)
	for _, row := range rows {
		if writeErr != nil {
			break
		}
		record := make([]string, len(header))
		for j, col := range header {
			record[j] = row[col]
		}
		writeErr = w.Write(record)
	}
	w.Flush()
	if writeErr == nil {
		writeErr = w.Error()
	}
	if closeErr := f.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return fmt.Errorf("failed to write %s: %w", path, writeErr)
	}
	return nil
}

// rowFor flattens one run into column → cell.
func rowFor(res *engine.RunResult) map[string]string {
	row := make(map[string]string, len(fixedColumns))
	for _, c := range fixedColumns {
		row[c.name] = c.value(res)
	}
	for k, v := range res.Labels {
		row[prefixLabel+k] = v
	}
	for i, a := range res.ConversationAssertions.Results {
		key := fmt.Sprintf("%s%d.%s", prefixAssertion, i+1, a.Type)
		row[key+".passed"] = strconv.FormatBool(a.Passed)
		if score, ok := numeric(a.Details["score"]); ok {
			row[key+".score"] = formatFloat(score)
		}
	}
	// A per-turn eval fires once per turn under the same id; the last
	// result wins, which for session evals is the only one.
	for i := range res.EvalResults {
		ev := &res.EvalResults[i]
		if ev.Score != nil {
			row[prefixEval+ev.EvalID+".score"] = formatFloat(*ev.Score)
		}
		addEvalDataColumns(row, ev)
	}
	if res.ToolStats != nil {
		for name, n := range res.ToolStats.ByTool {
			row[prefixTool+name+".count"] = strconv.Itoa(n)
		}
	}
	return row
}

// addEvalDataColumns flattens an eval's named metrics and its top-level
// numeric and boolean data entries into eval.<id>.<key> columns, so one eval
// can report several scores (e.g. functional_score and security_score).
// Nested values stay in the run JSON; a "score" key would shadow the eval's
// own score and is skipped. A metric wins over a data entry of the same name.
func addEvalDataColumns(row map[string]string, ev *evals.EvalResult) {
	for k, v := range ev.Details {
		if k == "score" {
			continue
		}
		col := prefixEval + ev.EvalID + "." + k
		if b, isBool := v.(bool); isBool {
			row[col] = strconv.FormatBool(b)
		} else if n, isNum := numeric(v); isNum {
			row[col] = formatFloat(n)
		}
	}
	for k, v := range ev.Metrics {
		if k == "score" {
			continue
		}
		row[prefixEval+ev.EvalID+"."+k] = formatFloat(v)
	}
}

// sortedDynamicColumns orders columns by group (labels, assertions, evals,
// tools), then alphabetically — except assertions, which keep their
// declaration order (assertion.2 before assertion.10).
func sortedDynamicColumns(cols map[string]bool) []string {
	out := make([]string, 0, len(cols))
	for c := range cols {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		gi, gj := groupRank(out[i]), groupRank(out[j])
		if gi != gj {
			return gi < gj
		}
		if gi == 1 {
			ni, nj := assertionIndex(out[i]), assertionIndex(out[j])
			if ni != nj {
				return ni < nj
			}
		}
		return out[i] < out[j]
	})
	return out
}

// groupRank orders the dynamic column groups; columns rowFor produces always
// match one of the prefixes.
func groupRank(col string) int {
	for rank, p := range []string{prefixLabel, prefixAssertion, prefixEval, prefixTool} {
		if strings.HasPrefix(col, p) {
			return rank
		}
	}
	return len(fixedColumns)
}

// assertionIndex parses n from "assertion.<n>.<type>.<field>".
func assertionIndex(col string) int {
	index, _, _ := strings.Cut(strings.TrimPrefix(col, prefixAssertion), ".")
	n, _ := strconv.Atoi(index)
	return n
}

func numeric(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// SaveSummary is a no-op: the CSV is per-run rows only.
func (r *Repository) SaveSummary(*results.ResultSummary) error {
	return nil
}

// LoadResults is not supported: CSV is an export format.
func (r *Repository) LoadResults() ([]engine.RunResult, error) {
	return nil, results.NewUnsupportedOperationError("LoadResults", "CSV repository does not support loading results")
}

// SupportsStreaming returns false: the column set is the union of all runs.
func (r *Repository) SupportsStreaming() bool {
	return false
}

// SaveResult is not supported: the header depends on every run.
func (r *Repository) SaveResult(*engine.RunResult) error {
	return results.NewUnsupportedOperationError("SaveResult", "CSV repository does not support streaming writes")
}
