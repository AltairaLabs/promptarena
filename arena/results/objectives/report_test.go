package objectives

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/engine"

	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func TestWilson(t *testing.T) {
	// Reference values for the 95% Wilson score interval.
	cases := []struct {
		k, n      int
		low, high float64
	}{
		{0, 10, 0.0, 0.2775},
		{5, 10, 0.2366, 0.7634},
		{10, 10, 0.7225, 1.0},
		{1, 20, 0.0089, 0.2361},
	}
	for _, c := range cases {
		low, high := Wilson(c.k, c.n)
		assert.InDelta(t, c.low, low, 1e-3, "low for %d/%d", c.k, c.n)
		assert.InDelta(t, c.high, high, 1e-3, "high for %d/%d", c.k, c.n)
	}
	low, high := Wilson(0, 0)
	assert.Equal(t, 0.0, low)
	assert.Equal(t, 1.0, high)
}

func objective(description string, achieved bool, at int, evalErr string) assertions.ConversationValidationResult {
	return objectiveOn(0, "achieved", description, achieved, at, evalErr)
}

func objectiveOn(turn int, expect, description string, achieved bool, at int, evalErr string,
) assertions.ConversationValidationResult {
	d := map[string]interface{}{
		"turn_index": turn, "description": description, "achieved": achieved, "exchanges": 5, "expect": expect,
	}
	if achieved {
		d["achieved_at_exchange"] = at
	}
	if evalErr != "" {
		d["error"] = evalErr
	}
	return assertions.ConversationValidationResult{Type: "objective", Details: d}
}

func run(id, scenario string, cost float64, labels map[string]string, data map[string]interface{},
	results ...assertions.ConversationValidationResult) engine.RunResult {
	r := engine.RunResult{
		RunID: id, ScenarioID: scenario, ProviderID: "p1", Labels: labels,
		Cost:                   types.CostInfo{TotalCost: cost},
		ConversationAssertions: engine.AssertionsSummary{Results: results},
	}
	if data != nil {
		r.Params = map[string]interface{}{"data": data}
	}
	return r
}

func TestBuild(t *testing.T) {
	runs := []engine.RunResult{
		run("r1", "refund", 1.0, map[string]string{"category": "pii"}, map[string]interface{}{"tier": "gold"},
			objective("get a refund", true, 2, "")),
		run("r2", "refund", 1.0, map[string]string{"category": "pii"}, map[string]interface{}{"tier": "silver"},
			objective("get a refund", false, 0, "")),
		run("r3", "upgrade", 2.0, nil, nil, objective("get an upgrade", true, 4, "")),
		run("r4", "upgrade", 0.5, nil, nil, objective("get an upgrade", false, 0, "judge down")),
		run("r5", "no-objective", 9.0, nil, nil, assertions.ConversationValidationResult{Type: "contains"}),
	}

	report := Build(runs, []string{GroupScenario, "label.category", "data.tier"})

	require.Len(t, report.Totals, 1)
	total := report.Totals[0]
	assert.Equal(t, "achieved", total.Expect)
	assert.Equal(t, 3, total.Attempts)
	assert.Equal(t, 2, total.Achieved)
	assert.Equal(t, 1, total.Unevaluated)
	assert.InDelta(t, 2.0/3.0, total.Rate, 1e-9)
	assert.InDelta(t, 3.0, total.MeanExchangesToAchieve, 1e-9)
	assert.InDelta(t, 4.5, total.Cost, 1e-9, "runs without an objective are left out")
	assert.InDelta(t, 2.25, total.CostPerAchieved, 1e-9)

	require.Len(t, report.Groupings, 3)
	byScenario := report.Groupings[0]
	require.Len(t, byScenario.Groups, 2)
	assert.Equal(t, "refund", byScenario.Groups[0].Value)
	assert.Equal(t, 1, byScenario.Groups[0].Achieved)
	assert.Equal(t, 2, byScenario.Groups[0].Attempts)
	assert.Equal(t, "upgrade", byScenario.Groups[1].Value)

	byCategory := report.Groupings[1].Groups
	require.Len(t, byCategory, 2)
	assert.Equal(t, noValue, byCategory[0].Value, "runs without the label group under (none)")
	assert.Equal(t, "pii", byCategory[1].Value)

	byTier := report.Groupings[2].Groups
	values := []string{byTier[0].Value, byTier[1].Value, byTier[2].Value}
	assert.Equal(t, []string{noValue, "gold", "silver"}, values)
}

func TestBuild_TrialGroup(t *testing.T) {
	r := run("r1", "refund", 0.1, nil, nil, objective("get a refund", true, 1, ""))
	r.TrialResults = &engine.TrialResults{
		TotalCost: 1.0,
		Objectives: []engine.ObjectiveTrialStats{{
			Description: "get a refund", Attempts: 4, Achieved: 1, AchievedAtExchange: []int{3}, Unevaluated: 1,
		}},
	}
	report := Build([]engine.RunResult{r}, []string{GroupObjective})
	total := report.Totals[0]
	assert.Equal(t, 4, total.Attempts, "a trial group counts every trial, not the reported run")
	assert.Equal(t, 1, total.Achieved)
	assert.Equal(t, 1, total.Unevaluated)
	assert.Equal(t, "achieved", total.Expect, "an empty expect defaults to achieved")
	assert.InDelta(t, 1.0, total.Cost, 1e-9, "trial groups cost every trial")
	assert.Equal(t, "get a refund", report.Groupings[0].Groups[0].Value)
}

func TestValidateGroupBy(t *testing.T) {
	assert.NoError(t, ValidateGroupBy([]string{"scenario", "provider", "objective", "label.x", "data.y"}))
	for _, bad := range []string{"label.", "data.", "row", ""} {
		assert.Error(t, ValidateGroupBy([]string{bad}), bad)
	}
}

func TestRepository_SaveResults(t *testing.T) {
	dir := t.TempDir()
	repo := NewRepository(dir, []string{GroupScenario})
	runs := []engine.RunResult{
		run("r1", "a|b", 0.5, nil, nil, objective("goal", true, 2, "")),
		run("r2", "a|b", 0.5, nil, nil, objective("goal", false, 0, "")),
	}
	require.NoError(t, repo.SaveResults(runs))

	raw, err := os.ReadFile(filepath.Join(dir, JSONFileName))
	require.NoError(t, err)
	var got Report
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Len(t, got.Totals, 1)
	assert.Equal(t, 2, got.Totals[0].Attempts)
	assert.False(t, math.IsNaN(got.Totals[0].CILow))

	md, err := os.ReadFile(filepath.Join(dir, MarkdownFileName))
	require.NoError(t, err)
	text := string(md)
	assert.Contains(t, text, "## By scenario")
	assert.Contains(t, text, "| a\\|b | achieved | 1 / 2 | 50.0% | 9.5%–90.5% | 2.0 | $1.00 | $1.00 | 0 |")
	assert.Contains(t, text, "not that it can't be")

	assert.NoError(t, repo.SaveSummary(nil))
	assert.False(t, repo.SupportsStreaming())
	assert.Error(t, repo.SaveResult(&runs[0]))
	_, err = repo.LoadResults()
	assert.Error(t, err)
}

func TestRenderMarkdown_NoObjectives(t *testing.T) {
	text := RenderMarkdown(Build(nil, []string{GroupScenario}))
	assert.True(t, strings.HasSuffix(text, "No run had an objective.\n"), text)
}

func TestBuild_ErroredRunsAreUnevaluatedAndCosted(t *testing.T) {
	partial := run("r2", "refund", 1.0, nil, nil, objectiveOn(1, "achieved", "first", true, 1, ""))
	partial.Error = "provider 500 on turn 2"
	none := run("r3", "refund", 1.0, nil, nil)
	none.Error = "provider 500"
	other := run("r4", "no-objectives", 5.0, nil, nil)
	other.Error = "provider 500"
	runs := []engine.RunResult{
		run("r1", "refund", 1.0, nil, nil,
			objectiveOn(1, "achieved", "first", true, 1, ""), objectiveOn(3, "achieved", "second", false, 0, "")),
		partial, none, other,
	}
	total := Build(runs, nil).Totals[0]
	assert.Equal(t, 3, total.Attempts)
	assert.Equal(t, 3, total.Unevaluated, "r2 misses the turn-3 objective, r3 misses both")
	assert.InDelta(t, 3.0, total.Cost, 1e-9, "errored runs' cost counts; scenarios without objectives don't")
}

func TestBuild_ObjectiveCostIsSplit(t *testing.T) {
	r := run("r1", "s", 1.0, nil, nil,
		objectiveOn(1, "achieved", "A", true, 1, ""), objectiveOn(2, "achieved", "B", true, 1, ""))
	groups := Build([]engine.RunResult{r}, []string{GroupObjective}).Groupings[0].Groups
	require.Len(t, groups, 2)
	assert.InDelta(t, 0.5, groups[0].Cost, 1e-9)
	assert.InDelta(t, 0.5, groups[1].Cost, 1e-9)
}

func TestBuild_ExpectationsAreSeparate(t *testing.T) {
	runs := []engine.RunResult{
		run("r1", "support", 0, nil, nil, objectiveOn(0, "achieved", "resolve", true, 1, "")),
		run("r2", "holdout", 0, nil, nil, objectiveOn(0, "not_achieved", "refund", true, 1, "")),
	}
	totals := Build(runs, nil).Totals
	require.Len(t, totals, 2)
	assert.Equal(t, "achieved", totals[0].Expect)
	assert.Equal(t, "not_achieved", totals[1].Expect)
	assert.Equal(t, 1, totals[1].Attempts)
}

func TestBuild_ObjectiveWithoutDescription(t *testing.T) {
	runs := []engine.RunResult{
		run("r1", "a", 0, nil, nil, objectiveOn(1, "achieved", "", true, 1, "")),
		run("r2", "b", 0, nil, nil, objectiveOn(1, "achieved", "", false, 0, "")),
	}
	groups := Build(runs, []string{GroupObjective}).Groupings[0].Groups
	require.Len(t, groups, 2)
	assert.Equal(t, "a turn 1 objective", groups[0].Value)
	assert.Equal(t, "b turn 1 objective", groups[1].Value)
}

func TestRenderMarkdown_UnevaluatedAndMultiline(t *testing.T) {
	report := Build([]engine.RunResult{
		run("r1", "s", 0, nil, nil, objectiveOn(0, "achieved", "line one\nline two", false, 0, "no runner")),
	}, []string{GroupObjective})
	text := RenderMarkdown(report)
	assert.Contains(t, text, "| line one line two | achieved | 0 / 0 | – | – |",
		"a group with no judged attempts shows no rate, and a newline stays in its cell")
}

func TestFormatCost(t *testing.T) {
	assert.Equal(t, "$0.00", formatCost(0))
	assert.Equal(t, "$0.000123", formatCost(0.000123))
	assert.Equal(t, "$1.50", formatCost(1.5))
}

func TestBuild_MissingAttemptForDatasetRows(t *testing.T) {
	errored := run("r3", "rows", 0, nil, nil)
	errored.Error = "provider 500"
	runs := []engine.RunResult{
		run("r1", "rows", 0, nil, nil, objectiveOn(1, "achieved", "Refund a tv", true, 1, "")),
		run("r2", "rows", 0, nil, nil, objectiveOn(1, "achieved", "Refund a mug", false, 0, "")),
		errored,
	}
	groups := Build(runs, []string{GroupObjective}).Groupings[0].Groups
	values := make([]string, len(groups))
	for i := range groups {
		values[i] = groups[i].Value
	}
	assert.Equal(t, []string{"Refund a mug", "Refund a tv", "rows turn 1 objective"}, values,
		"an errored row isn't credited to another row's objective")
}

func TestBuild_UnreliableAttemptsAreLeftOutOfTheRate(t *testing.T) {
	unreliable := assertions.ConversationValidationResult{Type: "persona_fidelity", Passed: true,
		Details: map[string]interface{}{"reliable": false, "turn_index": 0}}
	runs := []engine.RunResult{
		run("r1", "s", 1.0, nil, nil, objective("goal", false, 0, "")),
		run("r2", "s", 1.0, nil, nil, objective("goal", true, 1, ""), unreliable),
	}
	r := run("r3", "s", 1.0, nil, nil)
	r.TrialResults = &engine.TrialResults{TotalCost: 2.0, Objectives: []engine.ObjectiveTrialStats{
		{Description: "goal", Expect: "achieved", Attempts: 2, Achieved: 1, AchievedAtExchange: []int{1}, Unreliable: 3},
	}}
	runs = append(runs, r)

	report := Build(runs, nil)

	total := report.Totals[0]
	assert.Equal(t, 3, total.Attempts)
	assert.Equal(t, 1, total.Achieved, "the unreliable run's success is not counted")
	assert.Equal(t, 4, total.Unreliable)
	assert.InDelta(t, 4.0, total.Cost, 1e-9, "unreliable runs' cost still counts")
	assert.Contains(t, RenderMarkdown(report), "| all | achieved | 1 / 3 | 33.3% |")
	assert.Contains(t, RenderMarkdown(report), "| 0 | 4 |\n")
}

func TestBuild_UnreliableIsPerTurn(t *testing.T) {
	unreliableTurn1 := assertions.ConversationValidationResult{Type: "persona_fidelity", Passed: true,
		Details: map[string]interface{}{"reliable": false, "turn_index": 1}}
	runs := []engine.RunResult{run("r1", "s", 1.0, nil, nil,
		objectiveOn(1, "achieved", "first", true, 1, ""), objectiveOn(3, "achieved", "second", true, 1, ""),
		unreliableTurn1)}

	groups := Build(runs, []string{GroupObjective}).Groupings[0].Groups

	require.Len(t, groups, 2)
	assert.Equal(t, 1, groups[0].Unreliable, "turn 1's persona invented a specific")
	assert.Equal(t, 1, groups[1].Attempts, "turn 3's objective is unaffected")
}

func baselineOf(r assertions.ConversationValidationResult) assertions.ConversationValidationResult {
	r.Details["baseline"] = true
	return r
}

func TestBuild_BaselineIsReportedApart(t *testing.T) {
	refund := func(achieved bool) assertions.ConversationValidationResult {
		return objectiveOn(0, "not_achieved", "get a refund", achieved, 1, "")
	}
	runs := []engine.RunResult{
		run("a1", "refund-attack", 1.0, nil, nil, refund(true)),
		run("a2", "refund-attack", 1.0, nil, nil, refund(true)),
		run("a3", "refund-attack", 1.0, nil, nil, refund(false)),
		run("a4", "refund-attack", 1.0, nil, nil, refund(false)),
		run("b1", "refund-baseline", 0.5, nil, nil, baselineOf(refund(true))),
		run("b2", "refund-baseline", 0.5, nil, nil, baselineOf(refund(false))),
		run("b3", "refund-baseline", 0.5, nil, nil, baselineOf(refund(false))),
		run("b4", "refund-baseline", 0.5, nil, nil, baselineOf(objectiveOn(0, "not_achieved", "get a refund", false, 0, "judge down"))),
	}

	report := Build(runs, []string{GroupObjective, GroupScenario})

	require.Len(t, report.Totals, 1)
	total := report.Totals[0]
	assert.Equal(t, 4, total.Attempts, "baseline attempts are not counted with the attack's")
	assert.Equal(t, 2, total.Achieved)
	assert.InDelta(t, 4.0, total.Cost, 1e-9)
	require.NotNil(t, total.Baseline)
	assert.Equal(t, 3, total.Baseline.Attempts)
	assert.Equal(t, 1, total.Baseline.Achieved)
	assert.Equal(t, 1, total.Baseline.Unevaluated)
	assert.InDelta(t, 1.0/3.0, total.Baseline.Rate, 1e-9)
	low, high := Wilson(1, 3)
	assert.InDelta(t, low, total.Baseline.CILow, 1e-9)
	assert.InDelta(t, high, total.Baseline.CIHigh, 1e-9)
	assert.InDelta(t, 2.0, total.Baseline.Cost, 1e-9)
	require.NotNil(t, total.Lift)
	assert.InDelta(t, 0.5-1.0/3.0, *total.Lift, 1e-9)

	byObjective := report.Groupings[0].Groups
	require.Len(t, byObjective, 1, "the attack and its baseline share the objective's row")
	assert.Equal(t, "get a refund", byObjective[0].Value)
	require.NotNil(t, byObjective[0].Baseline)

	byScenario := report.Groupings[1].Groups
	require.Len(t, byScenario, 2)
	assert.Equal(t, "refund-attack", byScenario[0].Value)
	assert.Nil(t, byScenario[0].Baseline)
	assert.Nil(t, byScenario[0].Lift)
	assert.Equal(t, "refund-baseline", byScenario[1].Value)
	assert.Equal(t, 0, byScenario[1].Attempts)
	require.NotNil(t, byScenario[1].Baseline)
	assert.Nil(t, byScenario[1].Lift, "no lift without attack attempts to compare")
}

func TestBuild_BaselineTrialGroup(t *testing.T) {
	r := run("r1", "refund-baseline", 1.0, nil, nil, baselineOf(objective("get a refund", true, 1, "")))
	r.TrialResults = &engine.TrialResults{
		TotalCost: 2.0,
		Objectives: []engine.ObjectiveTrialStats{{
			Description: "get a refund", Baseline: true, Attempts: 5, Achieved: 1, AchievedAtExchange: []int{1},
		}},
	}
	total := Build([]engine.RunResult{r}, nil).Totals[0]
	assert.Equal(t, 0, total.Attempts)
	require.NotNil(t, total.Baseline)
	assert.Equal(t, 5, total.Baseline.Attempts)
	assert.Equal(t, 1, total.Baseline.Achieved)
}

func TestRenderMarkdown_Baseline(t *testing.T) {
	runs := []engine.RunResult{
		run("a1", "attack", 1.0, nil, nil, objective("get a refund", true, 1, "")),
		run("b1", "baseline", 1.0, nil, nil, baselineOf(objective("get a refund", false, 0, ""))),
	}
	md := RenderMarkdown(Build(runs, nil))
	assert.Contains(t, md, "| Unevaluated | Unreliable | Baseline | Lift |")
	assert.Contains(t, md, "| 0 / 1, 0.0% (0.0%–79.3%), $1.00 | +100.0 pts |")

	plain := RenderMarkdown(Build(runs[:1], nil))
	assert.NotContains(t, plain, "Baseline", "the columns appear only when a baseline ran")
}

func TestBuild_LiftPairsByObjective(t *testing.T) {
	obj := func(desc string, achieved bool) assertions.ConversationValidationResult {
		return objectiveOn(0, "not_achieved", desc, achieved, 1, "")
	}
	var runs []engine.RunResult
	for i := 0; i < 2; i++ {
		runs = append(runs,
			run("r", "attack", 0, nil, nil, obj("refund", false)),
			run("d", "attack", 0, nil, nil, obj("discount", true)),
			run("b", "baseline", 0, nil, nil, baselineOf(obj("refund", false))))
	}

	total := Build(runs, nil).Totals[0]
	assert.InDelta(t, 0.5, total.Rate, 1e-9, "the attack's own rate covers both objectives")
	require.NotNil(t, total.Lift)
	assert.InDelta(t, 0.0, *total.Lift, 1e-9,
		"discount has no baseline, so only refund counts: 0% with the attack, 0% without")

	runs = runs[:2]
	runs = append(runs, run("x", "baseline", 0, nil, nil, baselineOf(obj("store credit", false))))
	assert.Nil(t, Build(runs, nil).Totals[0].Lift, "no objective has both an attack and a baseline")
}

func TestBaselineCell(t *testing.T) {
	assert.Equal(t, "–", baselineCell(nil))
	assert.Equal(t, "0 / 0, $0.50, 1 unevaluated, 2 unreliable",
		baselineCell(&Group{Cost: 0.5, Unevaluated: 1, Unreliable: 2}))
	assert.Equal(t, "2 / 3, 66.7% (20.8%–93.9%), $1.00, 4 unreliable",
		baselineCell(&Group{Achieved: 2, Attempts: 3, Rate: 2.0 / 3.0, CILow: 0.2077, CIHigh: 0.939, Cost: 1, Unreliable: 4}))
}

func budgetedRun(id string, budget *engine.TrialBudgetResult, achieved int) engine.RunResult {
	r := run(id, "attack", 0.1, nil, nil, objective("get a refund", achieved > 0, 1, ""))
	r.TrialResults = &engine.TrialResults{
		TotalCost: 0.5,
		Budget:    budget,
		Objectives: []engine.ObjectiveTrialStats{{
			Description: "get a refund", Attempts: budget.TrialsRun, Achieved: achieved,
		}},
	}
	return r
}

func TestBuild_TrialBudget(t *testing.T) {
	runs := []engine.RunResult{
		budgetedRun("a", &engine.TrialBudgetResult{TrialsRun: 2, TrialsSkipped: 8, StoppedBy: "achieved",
			TrialsToFirstAchieved: 2}, 1),
		budgetedRun("b", &engine.TrialBudgetResult{TrialsRun: 4, TrialsSkipped: 6, StoppedBy: "achieved",
			TrialsToFirstAchieved: 4}, 1),
		budgetedRun("c", &engine.TrialBudgetResult{TrialsRun: 3, TrialsSkipped: 7, StoppedBy: "max_cost"}, 0),
	}
	total := Build(runs, nil).Totals[0]
	assert.Equal(t, 9, total.Attempts, "only the trials that ran are attempts")
	assert.Equal(t, 21, total.TrialsSkipped)
	assert.InDelta(t, 3.0, total.MeanTrialsToFirstAchieved, 1e-9, "the mean over groups that got through")
	assert.Equal(t, 2, total.FirstAchievedGroups)
	assert.Equal(t, 3, total.BudgetedGroups)

	md := RenderMarkdown(Build(runs, nil))
	assert.Contains(t, md, "| Groups achieved | Trials to first achieved | Trials skipped |")
	assert.Contains(t, md, "| 2 / 3, 66.7% (20.8%–93.9%) | 3.0 | 21 |")
	low, high := Wilson(2, 3)
	assert.InDelta(t, low, total.GroupsCILow, 1e-9)
	assert.InDelta(t, high, total.GroupsCIHigh, 1e-9)
	assert.NotContains(t, RenderMarkdown(Build([]engine.RunResult{
		run("x", "s", 0, nil, nil, objective("o", true, 1, "")),
	}, nil)), "Trials skipped", "the columns appear only when a trial budget ran")
	mixed := RenderMarkdown(Build(append(runs, run("x", "plain", 0, nil, nil, objective("o", true, 1, ""))),
		[]string{GroupScenario}))
	assert.Contains(t, mixed, "| plain | achieved | 1 / 1 |")
	assert.Contains(t, mixed, "| – | – | – |", "a row without a budget shows no skips")
}

func TestBuild_TrialBudgetCountsEachRunOnceInEveryRow(t *testing.T) {
	r := run("a", "attack", 0.1, nil, nil, objectiveOn(1, "achieved", "first", true, 1, ""),
		objectiveOn(2, "achieved", "second", true, 1, ""))
	r.TrialResults = &engine.TrialResults{
		Budget: &engine.TrialBudgetResult{TrialsRun: 2, TrialsSkipped: 3, StoppedBy: "achieved", TrialsToFirstAchieved: 2},
		Objectives: []engine.ObjectiveTrialStats{
			{TurnIndex: 1, Description: "first", Attempts: 2, Achieved: 1},
			{TurnIndex: 2, Description: "second", Attempts: 2, Achieved: 1},
		},
	}
	report := Build([]engine.RunResult{r}, []string{GroupObjective})
	assert.Equal(t, 1, report.Totals[0].BudgetedGroups, "two objectives, one trial group")
	assert.Equal(t, 3, report.Totals[0].TrialsSkipped)
	for _, g := range report.Groupings[0].Groups {
		assert.Equal(t, 1, g.BudgetedGroups, "%s carries the budget too", g.Value)
	}
}
