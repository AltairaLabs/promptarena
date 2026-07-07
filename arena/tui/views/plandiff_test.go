package views

import (
	"strings"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
	"github.com/AltairaLabs/promptarena/arena/tui/viewmodels"
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
