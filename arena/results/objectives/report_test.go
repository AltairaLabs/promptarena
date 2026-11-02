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
