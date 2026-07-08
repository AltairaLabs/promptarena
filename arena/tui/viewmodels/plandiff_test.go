package viewmodels

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

func TestBuildPlanDiff_CountsAndSymbols(t *testing.T) {
	plan := &deploy.PlanResponse{
		Summary:  "2 to add, 1 to change",
		Warnings: []string{"token expires soon"},
		Changes: []deploy.ResourceChange{
			{Type: "agent_runtime", Name: "bot", Action: deploy.ActionCreate},
			{Type: "a2a_endpoint", Name: "ep", Action: deploy.ActionCreate},
			{Type: "agent_runtime", Name: "old", Action: deploy.ActionUpdate, Detail: "image bumped"},
			{Type: "secret", Name: "unused", Action: deploy.ActionNoChange},
		},
	}
	d := BuildPlanDiff(plan)
	if d.Adds != 2 || d.Changes != 1 || d.NoChanges != 1 {
		t.Fatalf("counts add=%d chg=%d nochg=%d", d.Adds, d.Changes, d.NoChanges)
	}
	if d.Rows[0].Symbol != "+" {
		t.Fatalf("row0 symbol = %q", d.Rows[0].Symbol)
	}
	if !d.Rows[3].NoChange {
		t.Fatal("row3 should be flagged NoChange")
	}
	if d.Warnings[0] != "token expires soon" {
		t.Fatal("warning not carried")
	}
}
