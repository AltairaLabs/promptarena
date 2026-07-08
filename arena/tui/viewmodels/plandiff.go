package viewmodels

import (
	"github.com/AltairaLabs/promptarena/arena/deploy/flow"

	"github.com/AltairaLabs/PromptKit/runtime/deploy"
)

// PlanDiffRow is one resource change, presentation-ready.
type PlanDiffRow struct {
	Symbol   string
	Type     string
	Name     string
	Detail   string
	Action   deploy.Action
	NoChange bool
}

// PlanDiffData is the full plan, grouped and counted for rendering.
type PlanDiffData struct {
	Summary   string
	Warnings  []string
	Rows      []PlanDiffRow
	Adds      int
	Changes   int
	Destroys  int
	Drifts    int
	NoChanges int
}

// BuildPlanDiff converts a PlanResponse into a PlanDiffData.
func BuildPlanDiff(plan *deploy.PlanResponse) PlanDiffData {
	d := PlanDiffData{Summary: plan.Summary, Warnings: plan.Warnings}
	for _, c := range plan.Changes {
		row := PlanDiffRow{
			Symbol: flow.ActionSymbol(c.Action), Type: c.Type, Name: c.Name,
			Detail: c.Detail, Action: c.Action, NoChange: c.Action == deploy.ActionNoChange,
		}
		d.Rows = append(d.Rows, row)
		switch c.Action { //nolint:exhaustive // ActionNoChange and unknown actions fall through to default
		case deploy.ActionCreate:
			d.Adds++
		case deploy.ActionUpdate:
			d.Changes++
		case deploy.ActionDelete:
			d.Destroys++
		case deploy.ActionDrift:
			d.Drifts++
		default:
			d.NoChanges++
		}
	}
	return d
}
