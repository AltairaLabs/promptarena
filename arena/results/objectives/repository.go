package objectives

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
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
		"are reported in separate rows. Unreliable attempts are ones in which a persona invented a specific " +
		"it was not given; like unevaluated ones, they are not counted in the rate.\n\n")
	if len(report.Totals) == 0 {
		b.WriteString("No run had an objective.\n")
		return b.String()
	}
	cols := optionalColumns{baseline: hasBaseline(report), budget: hasBudget(report)}
	if cols.budget {
		b.WriteString("Groups achieved counts the trial groups run under a `trial_budget` in which a trial " +
			"achieved every objective, with a 95% Wilson interval: the chance of success within the budget. " +
			"Under `stop_on_achieved` a group stops at its first success, so read this rather than Rate, " +
			"which then overstates the per-trial rate. Trials to first achieved is the mean number of trials " +
			"those groups took. Trials skipped are trials the budget kept from starting; they are not " +
			"attempts.\n\n")
	}
	if cols.baseline {
		b.WriteString("Baseline attempts are from objectives marked `baseline: true`: the same goal pursued " +
			"without the attack. They are counted in the Baseline column only, with their cost. Lift is the " +
			"attack's rate minus the baseline's, in percentage points, over the objectives both attempted.\n\n")
	}
	b.WriteString("## Overall\n\n")
	writeTable(&b, "", report.Totals, cols)
	for _, g := range report.Groupings {
		fmt.Fprintf(&b, "\n## By %s\n\n", g.By)
		writeTable(&b, g.By, g.Groups, cols)
		if len(g.Untested) > 0 {
			fmt.Fprintf(&b, "\nNo objective maps to: %s.\n", strings.Join(untestedNames(g.Untested), ", "))
		}
	}
	return b.String()
}

// hasBaseline reports whether any group in the report has baseline attempts.
func hasBaseline(report *Report) bool {
	for i := range report.Totals {
		if report.Totals[i].Baseline != nil {
			return true
		}
	}
	return false
}

// twoColumnRule is the Markdown header rule for a pair of optional columns.
const twoColumnRule = "---|---|"

// optionalColumns are the table columns shown only when some run needs them.
type optionalColumns struct {
	baseline bool
	budget   bool
}

// hasBudget reports whether any run in the report had a trial budget.
func hasBudget(report *Report) bool {
	for i := range report.Totals {
		if report.Totals[i].BudgetedGroups > 0 {
			return true
		}
	}
	return false
}

func writeTable(b *strings.Builder, by string, groups []Group, cols optionalColumns) {
	first := "Group"
	if by != "" {
		first = by
	}
	header := "| %s | Expect | Achieved | Rate | 95%% CI | Mean exchanges | Cost | " +
		"Cost per achieved | Unevaluated | Unreliable |"
	rule := "|---|---|---|---|---|---|---|---|---|---|"
	if cols.budget {
		header += " Groups achieved | Trials to first achieved | Trials skipped |"
		rule += "---|" + twoColumnRule
	}
	if cols.baseline {
		header += " Baseline | Lift |"
		rule += twoColumnRule
	}
	fmt.Fprintf(b, header+"\n", first)
	b.WriteString(rule + "\n")
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
		fmt.Fprintf(b, "| %s | %s | %d / %d | %s | %s | %s | %s | %s | %d | %d |",
			escapeCell(groupName(&g)), g.Expect, g.Achieved, g.Attempts, rate, interval,
			optionalFloat(g.MeanExchangesToAchieve, "%.1f"), formatCost(g.Cost), perAchieved, g.Unevaluated, g.Unreliable)
		if cols.budget {
			skipped := "–"
			if g.BudgetedGroups > 0 {
				skipped = fmt.Sprint(g.TrialsSkipped)
			}
			fmt.Fprintf(b, " %s | %s | %s |", groupsAchievedCell(&g),
				optionalFloat(g.MeanTrialsToFirstAchieved, "%.1f"), skipped)
		}
		if cols.baseline {
			fmt.Fprintf(b, " %s | %s |", baselineCell(g.Baseline), liftCell(g.Lift))
		}
		b.WriteString("\n")
	}
}

// groupName is a group's value, with its title when it has one.
func groupName(g *Group) string {
	if g.Title != "" {
		return g.Value + " " + g.Title
	}
	return g.Value
}

// untestedNames gives each untested taxonomy entry its title.
func untestedNames(entries []string) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = strings.TrimSpace(e + " " + arenaconfig.TaxonomyTitle(e))
	}
	return out
}

// baselineCell shows a group's baseline: achieved / attempts with its rate
// and interval, its cost, and any attempts left out of its rate.
func baselineCell(bl *Group) string {
	if bl == nil {
		return "–"
	}
	parts := []string{fmt.Sprintf("%d / %d", bl.Achieved, bl.Attempts)}
	if bl.Attempts > 0 {
		parts = append(parts, fmt.Sprintf("%.1f%% (%.1f%%–%.1f%%)", bl.Rate*percent, bl.CILow*percent, bl.CIHigh*percent))
	}
	parts = append(parts, formatCost(bl.Cost))
	if bl.Unevaluated > 0 {
		parts = append(parts, fmt.Sprintf("%d unevaluated", bl.Unevaluated))
	}
	if bl.Unreliable > 0 {
		parts = append(parts, fmt.Sprintf("%d unreliable", bl.Unreliable))
	}
	return strings.Join(parts, ", ")
}

// groupsAchievedCell shows how many budgeted trial groups had a trial
// achieve every objective, with the rate and its interval.
func groupsAchievedCell(g *Group) string {
	if g.BudgetedGroups == 0 {
		return "–"
	}
	rate := float64(g.FirstAchievedGroups) / float64(g.BudgetedGroups)
	return fmt.Sprintf("%d / %d, %.1f%% (%.1f%%–%.1f%%)", g.FirstAchievedGroups, g.BudgetedGroups,
		rate*percent, g.GroupsCILow*percent, g.GroupsCIHigh*percent)
}

func liftCell(lift *float64) string {
	if lift == nil {
		return "–"
	}
	return fmt.Sprintf("%+.1f pts", *lift*percent)
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
