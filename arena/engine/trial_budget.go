package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/v2/events"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	arenastore "github.com/AltairaLabs/promptarena/v2/arena/statestore"
)

// Reasons a trial budget stopped a trial group, in TrialBudgetResult.StoppedBy.
const (
	TrialBudgetStoppedAchieved = "achieved"
	TrialBudgetStoppedMaxCost  = "max_cost"
)

// TrialBudgetResult reports how a scenario's trial_budget ended a trial group.
type TrialBudgetResult struct {
	// TrialsRun is the number of trials that ran.
	TrialsRun int `json:"trials_run"`
	// TrialsSkipped is the number of trials the budget kept from starting.
	TrialsSkipped int `json:"trials_skipped,omitempty"`
	// StoppedBy is why no more trials started: "achieved" or "max_cost".
	// Empty when every trial ran.
	StoppedBy string `json:"stopped_by,omitempty"`
	// TrialsToFirstAchieved is how many trials had run when the first one to
	// achieve every objective did, counting it; 0 when none did.
	TrialsToFirstAchieved int `json:"trials_to_first_achieved,omitempty"`
}

// trialBudget tracks one budgeted trial group as its trials run.
type trialBudget struct {
	cfg    *arenaconfig.TrialBudget
	result TrialBudgetResult
	cost   float64
}

// record counts a finished trial, and stops the group when the budget is met.
func (b *trialBudget) record(cost float64, achieved bool) {
	b.result.TrialsRun++
	b.cost += cost
	if achieved && b.result.TrialsToFirstAchieved == 0 {
		b.result.TrialsToFirstAchieved = b.result.TrialsRun
	}
	switch {
	case b.result.StoppedBy != "":
	case b.cfg.StopOnAchieved && achieved:
		b.result.StoppedBy = TrialBudgetStoppedAchieved
	case b.cfg.MaxCostUSD > 0 && b.cost >= b.cfg.MaxCostUSD:
		b.result.StoppedBy = TrialBudgetStoppedMaxCost
	}
}

// trialBudgets holds the result of each budgeted trial group in a dispatch,
// for the trial aggregation to report. It is safe for the concurrent run
// workers.
type trialBudgets struct {
	mu      sync.Mutex
	results map[TrialGroupKey]TrialBudgetResult
}

func newTrialBudgets() *trialBudgets {
	return &trialBudgets{results: map[TrialGroupKey]TrialBudgetResult{}}
}

func (b *trialBudgets) set(key TrialGroupKey, r TrialBudgetResult) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.results[key] = r
}

// result returns the group's budget result, or nil when the group has no
// budget.
func (b *trialBudgets) result(key TrialGroupKey) *TrialBudgetResult {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, ok := b.results[key]; ok {
		return &r
	}
	return nil
}

// trialBudgetFor returns the trial budget a combination runs under, or nil
// when it is not a trial of a scenario with one.
func (e *Engine) trialBudgetFor(combo RunCombination) *arenaconfig.TrialBudget {
	if combo.TotalTrials <= 1 {
		return nil
	}
	if sc := e.scenarios[combo.ScenarioID]; sc != nil {
		return sc.TrialBudget
	}
	return nil
}

// planWork turns the plan into work items. A budgeted scenario's trials for
// one provider, region and dataset row become a single item, run one trial
// at a time by one worker, so the budget is checked before each trial starts
// while the other workers carry on with other items. Budgeted groups come
// first, so a long one doesn't start last and run on alone.
func (e *Engine) planWork(combos []RunCombination) []runWorkItem {
	var items, single []runWorkItem
	grouped := map[TrialGroupKey]int{}
	for i := range combos {
		combo := combos[i]
		budget := e.trialBudgetFor(combo)
		if budget == nil {
			single = append(single, runWorkItem{idx: i, combo: combo})
			continue
		}
		key := trialGroupKey(combo)
		pos, ok := grouped[key]
		if !ok {
			pos = len(items)
			grouped[key] = pos
			items = append(items, runWorkItem{budget: budget})
		}
		items[pos].trials = append(items[pos].trials, runWorkItem{idx: i, combo: combo})
	}
	return append(items, single...)
}

// runBudgetedTrials runs a budgeted trial group's trials in order until its
// budget is met, recording each trial's run ID and error, and reports how the
// budget ended the group. A trial that is not run, because the budget was met
// or the context was canceled, gets no run ID.
func (e *Engine) runBudgetedTrials(
	ctx context.Context, item *runWorkItem, record func(idx int, runID string, err error), batchID string,
	budgets *trialBudgets,
) {
	b := &trialBudget{cfg: item.budget}
	for i := range item.trials {
		trial := &item.trials[i]
		if b.result.StoppedBy != "" {
			b.result.TrialsSkipped++
			continue
		}
		if ctx.Err() != nil {
			record(trial.idx, "", ctx.Err())
			continue
		}
		runID, err := e.executeRun(ctx, trial.combo, batchID)
		record(trial.idx, runID, err)
		if runID != "" {
			// A trial with no saved result isn't in the trial results either.
			b.record(e.finishedTrial(ctx, runID))
		}
	}
	key := trialGroupKey(item.trials[0].combo)
	budgets.set(key, b.result)
	if b.result.TrialsSkipped > 0 {
		e.reportSkippedTrials(key, b.result)
	}
}

// reportSkippedTrials tells progress displays that a budget kept trials from
// starting, so their totals still add up: it fires the trials-skipped hooks
// and emits an arena.trials.skipped event.
func (e *Engine) reportSkippedTrials(key TrialGroupKey, r TrialBudgetResult) {
	e.runCompletedMu.RLock()
	skipHooks := append([]TrialsSkippedHook(nil), e.trialsSkippedHooks...)
	e.runCompletedMu.RUnlock()
	for _, h := range skipHooks {
		h(r.TrialsSkipped)
	}
	if e.eventBus == nil {
		return
	}
	events.NewEmitter(e.eventBus, "", "", "").EmitCustom(
		events.EventType("arena.trials.skipped"),
		"ArenaEngine",
		"trials_skipped",
		map[string]interface{}{
			skippedKeyScenario:      key.ScenarioID,
			"provider_id":           key.ProviderID,
			skippedKeyRegion:        key.Region,
			arenastore.DataRowLabel: key.DataRowID,
			"count":                 r.TrialsSkipped,
			"reason":                r.StoppedBy,
		},
		fmt.Sprintf("Trial budget skipped %d trial(s) of %s", r.TrialsSkipped, key.ScenarioID),
	)
}

// Keys of the arena.trials.skipped event's data.
const (
	skippedKeyScenario = "scenario"
	skippedKeyRegion   = "region"
)

// TrialsSkippedHook is called with the number of trials a trial_budget kept
// from starting, once per trial group that skipped any.
type TrialsSkippedHook func(count int)

// RegisterTrialsSkippedHook registers a callback for trials a trial_budget
// skipped, so a progress display can take them off its total.
func (e *Engine) RegisterTrialsSkippedHook(hook TrialsSkippedHook) {
	if hook == nil {
		return
	}
	e.runCompletedMu.Lock()
	e.trialsSkippedHooks = append(e.trialsSkippedHooks, hook)
	e.runCompletedMu.Unlock()
}

// finishedTrial reads what a budget needs from a finished trial: its cost,
// and whether it achieved every objective it recorded. An objective the run
// never reached is recorded as not achieved. A baseline objective doesn't
// count, and an objective on a turn whose persona invented a specific
// doesn't count as achieved, so the simulator's mistake doesn't spend the
// budget.
func (e *Engine) finishedTrial(ctx context.Context, runID string) (cost float64, achieved bool) {
	store, ok := e.stateStore.(*arenastore.ArenaStateStore)
	if !ok || runID == "" {
		return 0, false
	}
	rr, err := store.GetResult(ctx, runID)
	if err != nil || rr == nil {
		return 0, false
	}
	outcomes := ObjectiveOutcomes(rr.ConversationAssertions.Results)
	unreliable := UnreliableTurns(rr.ConversationAssertions.Results)
	counted := 0
	achieved = true
	for _, o := range outcomes {
		if o.Baseline {
			continue
		}
		counted++
		if !o.Achieved || unreliable[o.TurnIndex] {
			achieved = false
		}
	}
	achieved = achieved && counted > 0
	return rr.Cost.TotalCost, achieved
}
