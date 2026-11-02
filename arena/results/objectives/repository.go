package objectives

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/engine"
	"github.com/AltairaLabs/promptarena/v2/arena/results"
)

// Files written into the output directory.
const (
	JSONFileName     = "objectives.json"
	MarkdownFileName = "objectives.md"
)

// outputDirPerm is the mode for a newly created output directory.
const outputDirPerm = 0o750

// percent scales a fraction for display.
const percent = 100

// outputFilePerm is the mode for the report files.
const outputFilePerm = 0o600

// Repository writes the objective report as JSON and Markdown.
type Repository struct {
	outputDir string
	groupBy   []string
}

// NewRepository creates a repository that writes the report into outputDir,
// with one grouping per groupBy key.
func NewRepository(outputDir string, groupBy []string) *Repository {
	return &Repository{outputDir: outputDir, groupBy: groupBy}
}

// SaveResults builds the report from the runs and writes both files.
func (r *Repository) SaveResults(runs []engine.RunResult) error {
	if err := results.ValidateResults(runs); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	if err := os.MkdirAll(r.outputDir, outputDirPerm); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	report := Build(runs, r.groupBy)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode objective report: %w", err)
	}
	if err := r.write(JSONFileName, data); err != nil {
		return err
	}
	return r.write(MarkdownFileName, []byte(RenderMarkdown(report)))
}

func (r *Repository) write(name string, data []byte) error {
	path := filepath.Join(r.outputDir, name)
	if err := os.WriteFile(path, data, outputFilePerm); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// SaveSummary is a no-op: the report is built from the runs themselves.
func (r *Repository) SaveSummary(*results.ResultSummary) error { return nil }

// LoadResults is not supported: the report can't be read back into runs.
func (r *Repository) LoadResults() ([]engine.RunResult, error) {
	return nil, results.NewUnsupportedOperationError("LoadResults", "objectives report is write-only")
}

// SupportsStreaming returns false: the report needs every run.
func (r *Repository) SupportsStreaming() bool { return false }

// SaveResult is not supported: the report needs every run.
func (r *Repository) SaveResult(*engine.RunResult) error {
	return results.NewUnsupportedOperationError("SaveResult", "objectives report needs every run")
}

// RenderMarkdown renders the report as Markdown tables.
func RenderMarkdown(report *Report) string {
	var b strings.Builder
	b.WriteString("# Objective Report\n\n")
	b.WriteString("How often self-play turn objectives were achieved. Rates count evaluated attempts only, " +
		"and each rate has a 95% Wilson score interval. A rate of 0 means the objective wasn't reached " +
		"in these attempts, not that it can't be. Objectives that should be achieved and objectives " +
		"that should not (`expect: not_achieved`, where a high rate means the agent failed to hold out) " +
		"are reported in separate rows.\n\n")
	if len(report.Totals) == 0 {
		b.WriteString("No run had an objective.\n")
		return b.String()
	}
	b.WriteString("## Overall\n\n")
	writeTable(&b, "", report.Totals)
	for _, g := range report.Groupings {
		fmt.Fprintf(&b, "\n## By %s\n\n", g.By)
		writeTable(&b, g.By, g.Groups)
	}
	return b.String()
}

func writeTable(b *strings.Builder, by string, groups []Group) {
	first := "Group"
	if by != "" {
		first = by
	}
	fmt.Fprintf(b, "| %s | Expect | Achieved | Rate | 95%% CI | Mean exchanges | Cost | "+
		"Cost per achieved | Unevaluated |\n", first)
	b.WriteString("|---|---|---|---|---|---|---|---|---|\n")
	for _, g := range groups {
		perAchieved := "–"
		if g.Achieved > 0 {
			perAchieved = formatCost(g.CostPerAchieved)
		}
		rate, interval := "–", "–"
		if g.Attempts > 0 {
			rate = fmt.Sprintf("%.1f%%", g.Rate*percent)
			interval = fmt.Sprintf("%.1f%%–%.1f%%", g.CILow*percent, g.CIHigh*percent)
		}
		fmt.Fprintf(b, "| %s | %s | %d / %d | %s | %s | %s | %s | %s | %d |\n",
			escapeCell(g.Value), g.Expect, g.Achieved, g.Attempts, rate, interval,
			optionalFloat(g.MeanExchangesToAchieve, "%.1f"), formatCost(g.Cost), perAchieved, g.Unevaluated)
	}
}

func optionalFloat(v float64, format string) string {
	if v == 0 {
		return "–"
	}
	return fmt.Sprintf(format, v)
}

// formatCost shows a cost to the cent, or to six places when it is under a
// cent, so a cheap mock or small-model run doesn't read as free.
func formatCost(v float64) string {
	if v != 0 && v < 0.01 {
		return fmt.Sprintf("$%.6f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

// escapeCell keeps a value inside one Markdown table cell.
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.Join(strings.Fields(s), " ")
}
