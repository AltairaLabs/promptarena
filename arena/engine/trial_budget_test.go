package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"
)

func TestTrialBudget_StopOnAchieved(t *testing.T) {
	b := &trialBudget{cfg: &arenaconfig.TrialBudget{StopOnAchieved: true}}
	b.record(0.1, false)
	assert.Empty(t, b.result.StoppedBy)
	b.record(0.1, true)
	assert.Equal(t, TrialBudgetResult{TrialsRun: 2, StoppedBy: TrialBudgetStoppedAchieved, TrialsToFirstAchieved: 2},
		b.result)
}

func TestTrialBudget_MaxCost(t *testing.T) {
	b := &trialBudget{cfg: &arenaconfig.TrialBudget{MaxCostUSD: 0.25}}
	b.record(0.1, false)
	b.record(0.1, false)
	assert.Empty(t, b.result.StoppedBy, "0.20 spent is under the cap")
	b.record(0.1, true)
	assert.Equal(t, TrialBudgetResult{TrialsRun: 3, StoppedBy: TrialBudgetStoppedMaxCost, TrialsToFirstAchieved: 3},
		b.result, "without stop_on_achieved, an achieved trial doesn't stop the group")
}

func TestPlanWork_BudgetedTrialsAreOneItem(t *testing.T) {
	budget := &arenaconfig.TrialBudget{StopOnAchieved: true}
	e := &Engine{scenarios: map[string]*arenaconfig.Scenario{
		"budgeted": {ID: "budgeted", Trials: 2, TrialBudget: budget},
		"plain":    {ID: "plain", Trials: 2},
	}}
	combos := []RunCombination{
		{ScenarioID: "budgeted", ProviderID: "p1", TotalTrials: 2, TrialIndex: 0},
		{ScenarioID: "budgeted", ProviderID: "p1", TotalTrials: 2, TrialIndex: 1},
		{ScenarioID: "budgeted", ProviderID: "p2", TotalTrials: 2, TrialIndex: 0},
		{ScenarioID: "plain", ProviderID: "p1", TotalTrials: 2, TrialIndex: 0},
		{ScenarioID: "budgeted", ProviderID: "p2", TotalTrials: 2, TrialIndex: 1},
	}
	items := e.planWork(combos)
	require.Len(t, items, 3, "one item per budgeted group, one per other run")
	assert.Same(t, budget, items[0].budget)
	assert.Equal(t, []int{0, 1}, []int{items[0].trials[0].idx, items[0].trials[1].idx})
	assert.Equal(t, []int{2, 4}, []int{items[1].trials[0].idx, items[1].trials[1].idx})
	assert.Nil(t, items[2].budget)
	assert.Equal(t, 3, items[2].idx)
}

func TestExecuteRuns_CanceledBudgetedGroupRunsNothing(t *testing.T) {
	eng := newTestEngine(t, t.TempDir(), &arenaconfig.Config{})
	eng.scenarios = map[string]*arenaconfig.Scenario{"s1": {ID: "s1", TaskType: "support", Trials: 3,
		TrialBudget: &arenaconfig.TrialBudget{MaxCostUSD: 1}}}
	skipped := 0
	eng.RegisterTrialsSkippedHook(func(n int) { skipped += n })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runIDs, err := eng.ExecuteRuns(ctx, &RunPlan{Combinations: trialCombosFor("s1", 3)}, 2)

	require.Error(t, err, "the canceled trials are reported")
	assert.Equal(t, []string{"", "", ""}, runIDs, "no trial ran")
	assert.Equal(t, 0, skipped, "a canceled trial is not a budget skip")
}

func TestExecuteRuns_BudgetedGroupRunsEveryTrialUntilStopped(t *testing.T) {
	// Runs that fail cost nothing and achieve nothing, so the budget never
	// stops the group.
	eng := newTestEngine(t, t.TempDir(), &arenaconfig.Config{})
	eng.scenarios = map[string]*arenaconfig.Scenario{"s1": {ID: "s1", TaskType: "support", Trials: 3,
		TrialBudget: &arenaconfig.TrialBudget{MaxCostUSD: 1}}}

	runIDs, err := eng.ExecuteRuns(context.Background(), &RunPlan{Combinations: trialCombosFor("s1", 3)}, 2)

	require.NoError(t, err)
	require.Len(t, runIDs, 1, "the trials aggregate to one run")
	rr, err := eng.GetStateStore().(*statestore.ArenaStateStore).GetResult(context.Background(), runIDs[0])
	require.NoError(t, err)
	tr, ok := rr.TrialResults.(*TrialResults)
	require.True(t, ok)
	assert.Equal(t, &TrialBudgetResult{TrialsRun: 3}, tr.Budget)
}

func trialCombosFor(scenario string, n int) []RunCombination {
	combos := trialCombos(n)
	for i := range combos {
		combos[i].ScenarioID = scenario
	}
	return combos
}

func TestTrialBudgetFor(t *testing.T) {
	budget := &arenaconfig.TrialBudget{MaxCostUSD: 1}
	e := &Engine{scenarios: map[string]*arenaconfig.Scenario{"s": {ID: "s", Trials: 3, TrialBudget: budget}}}
	assert.Same(t, budget, e.trialBudgetFor(RunCombination{ScenarioID: "s", TotalTrials: 3}))
	assert.Nil(t, e.trialBudgetFor(RunCombination{ScenarioID: "s", TotalTrials: 1}))
	assert.Nil(t, e.trialBudgetFor(RunCombination{ScenarioID: "missing", TotalTrials: 3}))
}

func achievedObjective(achieved bool) assertions.ConversationValidationResult {
	d := map[string]interface{}{objectiveKeyTurnIndex: 1, "achieved": achieved}
	if achieved {
		d["achieved_at_exchange"] = 1
	}
	return assertions.ConversationValidationResult{Type: objectiveResultType, Details: d}
}

func TestFinishedTrial(t *testing.T) {
	store := statestore.NewArenaStateStore()
	e := &Engine{stateStore: store}
	saveTrialRun(t, store, "won", nil, []assertions.ConversationValidationResult{achievedObjective(true)}, "")
	saveTrialRun(t, store, "lost", nil, []assertions.ConversationValidationResult{achievedObjective(true),
		{Type: objectiveResultType, Details: map[string]interface{}{objectiveKeyTurnIndex: 2, "achieved": false}}}, "")
	saveTrialRun(t, store, "unreliable", nil, []assertions.ConversationValidationResult{achievedObjective(true),
		{Type: personaFidelityResultType, Details: map[string]interface{}{
			fidelityKeyReliable: false, fidelityKeyTurnIndex: 1,
		}}}, "")
	saveTrialRun(t, store, "none", nil, nil, "")
	saveTrialRun(t, store, "baseline", nil, []assertions.ConversationValidationResult{achievedObjective(true),
		{Type: objectiveResultType, Details: map[string]interface{}{
			objectiveKeyTurnIndex: 2, "achieved": false, objectiveKeyBaseline: true,
		}}}, "")

	ctx := context.Background()
	_, achieved := e.finishedTrial(ctx, "won")
	assert.True(t, achieved)
	_, achieved = e.finishedTrial(ctx, "lost")
	assert.False(t, achieved, "every objective must be achieved")
	_, achieved = e.finishedTrial(ctx, "unreliable")
	assert.False(t, achieved, "an objective the simulator faked doesn't spend the budget")
	_, achieved = e.finishedTrial(ctx, "none")
	assert.False(t, achieved, "no objective, nothing achieved")
	_, achieved = e.finishedTrial(ctx, "")
	assert.False(t, achieved)
	_, achieved = e.finishedTrial(ctx, "baseline")
	assert.True(t, achieved, "a baseline objective doesn't count")
}

func TestAggregateTrialResults_BudgetStoppedAfterOneTrial(t *testing.T) {
	store := statestore.NewArenaStateStore()
	saveTrialRun(t, store, "a", nil, []assertions.ConversationValidationResult{achievedObjective(true)}, "")
	b := newTrialBudgets()
	key := trialGroupKey(trialCombos(3)[0])
	b.set(key, TrialBudgetResult{TrialsRun: 1, TrialsSkipped: 2, StoppedBy: TrialBudgetStoppedAchieved,
		TrialsToFirstAchieved: 1})

	// The two skipped trials have no run ID.
	ids := aggregateTrialResults(store, []string{"a", "", ""}, trialCombos(3), nil, b)

	require.Equal(t, []string{"a"}, ids)
	rr, err := store.GetResult(context.Background(), "a")
	require.NoError(t, err)
	tr, ok := rr.TrialResults.(*TrialResults)
	require.True(t, ok, "a group the budget stopped after one trial still reports it")
	assert.Equal(t, 1, tr.TrialCount)
	assert.Equal(t, &TrialBudgetResult{
		TrialsRun: 1, TrialsSkipped: 2, StoppedBy: TrialBudgetStoppedAchieved, TrialsToFirstAchieved: 1,
	}, tr.Budget)
}
