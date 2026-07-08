package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/AltairaLabs/promptarena/arena/tui/theme"
	"github.com/AltairaLabs/promptarena/arena/tui/viewmodels"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

var planGroupOrder = []deploy.Action{
	deploy.ActionCreate, deploy.ActionUpdate, deploy.ActionDrift, deploy.ActionDelete,
}

func planRowColor(a deploy.Action) lipgloss.Color {
	switch a { //nolint:exhaustive // ActionNoChange and unknown actions fall through to default
	case deploy.ActionCreate:
		return lipgloss.Color(theme.ColorSuccess)
	case deploy.ActionUpdate:
		return lipgloss.Color(theme.ColorWarning)
	case deploy.ActionDelete:
		return lipgloss.Color(theme.ColorError)
	case deploy.ActionDrift:
		return lipgloss.Color(theme.ColorYellow)
	default:
		return lipgloss.Color(theme.ColorGray)
	}
}

// RenderPlanDiff renders a plan as a grouped, colored diff. When collapseNoChange
// is true, unchanged resources are folded into a single dimmed summary line.
func RenderPlanDiff(d viewmodels.PlanDiffData, width int, collapseNoChange bool) string {
	var b strings.Builder
	if d.Summary != "" {
		b.WriteString(theme.TitleStyle.Render(d.Summary))
		b.WriteString("\n\n")
	}
	writePlanWarnings(&b, d.Warnings)

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))
	writeGroupedPlanRows(&b, d.Rows, dim)
	writeNoChangeRows(&b, d, dim, collapseNoChange)
	writePlanSummary(&b, d)
	return b.String()
}

// writePlanWarnings writes each plan-level warning, followed by a blank line
// if there were any, mirroring RenderPlanDiff's original inline warnings block.
func writePlanWarnings(b *strings.Builder, warnings []string) {
	for _, w := range warnings {
		b.WriteString(theme.WarningStyle.Render("⚠ " + w))
		b.WriteString("\n")
	}
	if len(warnings) > 0 {
		b.WriteString("\n")
	}
}

// writeGroupedPlanRows writes each changed resource row, grouped by action in
// planGroupOrder — create, update, drift, delete — skipping NO_CHANGE rows
// (handled separately by writeNoChangeRows).
func writeGroupedPlanRows(b *strings.Builder, rows []viewmodels.PlanDiffRow, dim lipgloss.Style) {
	for _, action := range planGroupOrder {
		for _, r := range rows {
			if r.Action != action {
				continue
			}
			line := fmt.Sprintf("%s %s.%s", r.Symbol, r.Type, r.Name)
			styled := lipgloss.NewStyle().Foreground(planRowColor(r.Action)).Render(line)
			if r.Detail != "" {
				styled += dim.Render("  (" + r.Detail + ")")
			}
			b.WriteString("  " + styled + "\n")
		}
	}
}

// writeNoChangeRows writes the unchanged-resource section: a single collapsed
// summary line when collapseNoChange is true, or every NO_CHANGE row
// individually otherwise. A no-op when the plan has no unchanged resources.
func writeNoChangeRows(b *strings.Builder, d viewmodels.PlanDiffData, dim lipgloss.Style, collapseNoChange bool) {
	if d.NoChanges == 0 {
		return
	}
	if collapseNoChange {
		b.WriteString("  " + dim.Render(fmt.Sprintf("… %d unchanged (press [space] to expand)", d.NoChanges)) + "\n")
		return
	}
	for _, r := range d.Rows {
		if !r.NoChange {
			continue
		}
		b.WriteString("  " + dim.Render(fmt.Sprintf("%s %s.%s", r.Symbol, r.Type, r.Name)) + "\n")
	}
}

// writePlanSummary writes the trailing "Plan: N to add, ..." line and, when
// the plan has drifted resources, the "⚠ N drifted" suffix.
func writePlanSummary(b *strings.Builder, d viewmodels.PlanDiffData) {
	b.WriteString("\n")
	b.WriteString(theme.TitleStyle.Render(fmt.Sprintf("Plan: %d to add, %d to change, %d to destroy",
		d.Adds, d.Changes, d.Destroys)))
	if d.Drifts > 0 {
		b.WriteString(theme.WarningStyle.Render(fmt.Sprintf("  ⚠ %d drifted", d.Drifts)))
	}
}
