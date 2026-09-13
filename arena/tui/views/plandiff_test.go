package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/AltairaLabs/promptarena/v2/arena/tui/theme"
	"github.com/AltairaLabs/promptarena/v2/arena/tui/viewmodels"
	"github.com/AltairaLabs/promptarena/v2/deploy"
)

func TestRenderPlanDiff_CollapsesNoChange(t *testing.T) {
	d := viewmodels.PlanDiffData{
		Summary: "1 to add",
		Rows: []viewmodels.PlanDiffRow{
			{Symbol: "+", Type: "agent_runtime", Name: "bot", Action: deploy.ActionCreate},
			{Symbol: " ", Type: "secret", Name: "s", Action: deploy.ActionNoChange, NoChange: true},
		},
		Adds: 1, NoChanges: 1,
	}
	out := stripANSIForTest(RenderPlanDiff(d, 80, true))
	if !strings.Contains(out, "1 to add") {
		t.Fatal("missing summary")
	}
	if !strings.Contains(out, "+ agent_runtime.bot") {
		t.Fatalf("missing create row:\n%s", out)
	}
	if !strings.Contains(out, "1 unchanged") {
		t.Fatalf("no-change not collapsed:\n%s", out)
	}
	if strings.Contains(out, "secret.s") {
		t.Fatalf("collapsed row should be hidden:\n%s", out)
	}
}

func TestRenderPlanDiff_ExpandedShowsNoChange(t *testing.T) {
	d := viewmodels.PlanDiffData{
		Rows:      []viewmodels.PlanDiffRow{{Symbol: " ", Type: "secret", Name: "s", NoChange: true}},
		NoChanges: 1,
	}
	out := stripANSIForTest(RenderPlanDiff(d, 80, false))
	if !strings.Contains(out, "secret.s") {
		t.Fatalf("expanded should show no-change row:\n%s", out)
	}
}

// TestRenderPlanDiff_GroupsAndColorsByAction covers planRowColor and
// writeGroupedPlanRows. The colour is how an operator tells a delete from a
// create at a glance before approving a plan, and the grouping order is what
// puts destructive changes where they will be read — a plan that renders in
// arbitrary order is a plan someone approves without seeing the deletes.
func TestRenderPlanDiff_GroupsAndColorsByAction(t *testing.T) {
	d := viewmodels.PlanDiffData{
		Rows: []viewmodels.PlanDiffRow{
			{Symbol: "-", Type: "secret", Name: "old", Action: deploy.ActionDelete},
			{Symbol: "+", Type: "agent", Name: "new", Action: deploy.ActionCreate},
			{Symbol: "~", Type: "config", Name: "cfg", Action: deploy.ActionUpdate, Detail: "image changed"},
			{Symbol: "!", Type: "role", Name: "r", Action: deploy.ActionDrift},
		},
		Adds: 1, Changes: 1, Destroys: 1, Drifts: 1,
	}
	out := stripANSIForTest(RenderPlanDiff(d, 80, true))

	// Grouped create → update → drift → delete, whatever order the rows came in.
	idx := func(s string) int { return strings.Index(out, s) }
	for _, pair := range [][2]string{
		{"agent.new", "config.cfg"},
		{"config.cfg", "role.r"},
		{"role.r", "secret.old"},
	} {
		if idx(pair[0]) == -1 || idx(pair[1]) == -1 {
			t.Fatalf("missing row %q or %q:\n%s", pair[0], pair[1], out)
		}
		if idx(pair[0]) > idx(pair[1]) {
			t.Errorf("%q must render before %q:\n%s", pair[0], pair[1], out)
		}
	}

	if !strings.Contains(out, "(image changed)") {
		t.Errorf("row detail dropped:\n%s", out)
	}
	if !strings.Contains(out, "⚠ 1 drifted") {
		t.Errorf("drift count missing from the summary:\n%s", out)
	}

	// Each action must get its own colour; a shared one defeats the point.
	seen := map[lipgloss.TerminalColor]deploy.Action{}
	for _, a := range []deploy.Action{
		deploy.ActionCreate, deploy.ActionUpdate, deploy.ActionDelete, deploy.ActionDrift,
	} {
		c := planRowColor(a)
		if prev, dup := seen[c]; dup {
			t.Errorf("%v and %v render the same colour", prev, a)
		}
		seen[c] = a
	}
	// An unknown action must fall back to muted rather than borrowing a
	// meaning it does not have.
	if planRowColor(deploy.ActionNoChange) != theme.Colors().TextMuted {
		t.Error("an unrecognised action must render muted")
	}
}

// TestRenderPlanDiff_WarningsPrecedeTheRows covers writePlanWarnings. A warning
// printed after the plan is a warning read after the decision.
func TestRenderPlanDiff_WarningsPrecedeTheRows(t *testing.T) {
	d := viewmodels.PlanDiffData{
		Warnings: []string{"credentials expire in 2 days", "region mismatch"},
		Rows: []viewmodels.PlanDiffRow{
			{Symbol: "+", Type: "agent", Name: "bot", Action: deploy.ActionCreate},
		},
		Adds: 1,
	}
	out := stripANSIForTest(RenderPlanDiff(d, 80, true))

	for _, w := range d.Warnings {
		if !strings.Contains(out, w) {
			t.Fatalf("warning %q missing:\n%s", w, out)
		}
	}
	if strings.Index(out, "credentials expire") > strings.Index(out, "agent.bot") {
		t.Errorf("warnings must render above the rows:\n%s", out)
	}
}

func TestRenderPlanDiff_NoWarningsAddsNoBlankBlock(t *testing.T) {
	d := viewmodels.PlanDiffData{
		Rows: []viewmodels.PlanDiffRow{
			{Symbol: "+", Type: "agent", Name: "bot", Action: deploy.ActionCreate},
		},
		Adds: 1,
	}
	out := stripANSIForTest(RenderPlanDiff(d, 80, true))
	if strings.Contains(out, "⚠") {
		t.Errorf("no warnings configured, but a warning marker rendered:\n%s", out)
	}
}

// TestRenderPlanDiff_EmptyPlanStillReportsTotals covers writePlanSummary's
// zero case: a plan with nothing in it must still say so, rather than
// rendering blank and leaving the operator unsure it ran.
func TestRenderPlanDiff_EmptyPlanStillReportsTotals(t *testing.T) {
	out := stripANSIForTest(RenderPlanDiff(viewmodels.PlanDiffData{}, 80, true))
	if !strings.Contains(out, "Plan: 0 to add, 0 to change, 0 to destroy") {
		t.Errorf("empty plan must still print totals:\n%s", out)
	}
	if strings.Contains(out, "drifted") {
		t.Errorf("no drift should mean no drift suffix:\n%s", out)
	}
}
