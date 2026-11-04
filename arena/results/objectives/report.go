// Package objectives provides the objective report for Arena: how often
// self-play turn objectives were achieved, grouped by scenario, provider,
// label or dataset column, with a confidence interval on each rate.
package objectives

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/AltairaLabs/promptarena/v2/arena/engine"
)

// Group-by keys. label.<key> and data.<column> take a suffix.
const (
	GroupScenario  = "scenario"
	GroupProvider  = "provider"
	GroupObjective = "objective"
	groupLabel     = "label."
	groupData      = "data."
)

// noValue is the group value for runs that lack the group-by key.
const noValue = "(none)"

// wilsonZ is the normal quantile for a two-sided 95% interval.
const wilsonZ = 1.959963984540054

// Group is the objective outcomes of one group of runs that share an
// expected outcome. Objectives that should be achieved and objectives that
// should not are never counted together.
type Group struct {
	// Value is the group-by key's value shared by these runs.
	Value string `json:"value"`
	// Expect is the objectives' expected outcome: "achieved" or
	// "not_achieved". For not_achieved, Rate is how often the agent failed to
	// hold out.
	Expect string `json:"expect"`
	// Attempts is the number of evaluated objective attempts.
	Attempts int `json:"attempts"`
	// Achieved is the number of attempts that reached their objective.
	Achieved int `json:"achieved"`
	// Rate is Achieved / Attempts, or 0 with no attempts.
	Rate float64 `json:"rate"`
	// CILow and CIHigh bound Rate with a 95% Wilson score interval.
	CILow  float64 `json:"ci_low"`
	CIHigh float64 `json:"ci_high"`
	// MeanExchangesToAchieve is the mean exchange at which achieved
	// attempts reached their objective, or 0 when none did.
	MeanExchangesToAchieve float64 `json:"mean_exchanges_to_achieve,omitempty"`
	// Cost is the cost of the group's runs, in USD. A run with several
	// objectives splits its cost evenly between them, and a run with trials
	// counts every trial.
	Cost float64 `json:"cost"`
	// CostPerAchieved is Cost / Achieved, or 0 when none were achieved.
	CostPerAchieved float64 `json:"cost_per_achieved,omitempty"`
	// Unevaluated is the number of attempts that were never judged: their
	// success checks could not be evaluated, or the run or trial errored
	// before the objective was recorded. They are not counted in Attempts.
	Unevaluated int `json:"unevaluated,omitempty"`
	// Unreliable is the number of evaluated attempts in which a persona
	// invented a specific it was not given, so the attempt measured the
	// simulator as much as the agent. They are not counted in Attempts.
	Unreliable int `json:"unreliable,omitempty"`
	// BudgetedGroups is the number of trial groups in this group that ran
	// under a scenario's trial_budget, and FirstAchievedGroups how many of
	// them had a trial achieve every objective. Under stop_on_achieved, Rate
	// counts trials that stopped at the first success, so it overstates the
	// per-trial rate; FirstAchievedGroups / BudgetedGroups, bounded by
	// GroupsCILow and GroupsCIHigh, is the chance of success within the
	// budget.
	BudgetedGroups      int     `json:"budgeted_groups,omitempty"`
	FirstAchievedGroups int     `json:"first_achieved_groups,omitempty"`
	GroupsCILow         float64 `json:"groups_ci_low,omitempty"`
	GroupsCIHigh        float64 `json:"groups_ci_high,omitempty"`
	// MeanTrialsToFirstAchieved is the mean, over those trial groups, of how
	// many trials had run when the first achieved every objective: the
	// attempts it took. 0 when none did.
	MeanTrialsToFirstAchieved float64 `json:"mean_trials_to_first_achieved,omitempty"`
	// TrialsSkipped is the number of trials a trial_budget kept from
	// starting. They are not attempts.
	TrialsSkipped int `json:"trials_skipped,omitempty"`
	// Baseline totals the group's baseline attempts, from objectives marked
	// baseline: true: the same goal pursued without the attack. They are not
	// counted in the fields above, and their cost is Baseline.Cost. Nil when
	// the group has none.
	Baseline *Group `json:"baseline,omitempty"`
	// Lift is how much more often the attack reached its objective than the
	// baseline did: the attack's rate minus the baseline's, both over the
	// objectives (by description) that the two attempted. Nil when no
	// objective has evaluated attempts on both sides.
	Lift *float64 `json:"lift,omitempty"`
}

// Grouping is the report for one group-by key.
type Grouping struct {
	// By is the group-by key, e.g. "scenario" or "label.category".
	By     string  `json:"by"`
	Groups []Group `json:"groups"`
}

// Report is the objective report for a set of runs.
type Report struct {
	// Totals has one row per expected outcome across every run with an
	// objective.
	Totals []Group `json:"totals"`
	// Groupings holds one entry per group-by key, in the order requested.
	Groupings []Grouping `json:"groupings"`
}

// ValidateGroupBy reports an error for a group-by key the report doesn't know.
func ValidateGroupBy(keys []string) error {
	for _, k := range keys {
		switch {
		case k == GroupScenario, k == GroupProvider, k == GroupObjective:
		case strings.HasPrefix(k, groupLabel) && len(k) > len(groupLabel):
		case strings.HasPrefix(k, groupData) && len(k) > len(groupData):
		default:
			return fmt.Errorf("unknown objectives group-by key %q: want scenario, provider, objective, "+
				"label.<key> or data.<column>", k)
		}
	}
	return nil
}

// attempt is one objective's outcomes in one run: one attempt for a plain
// run, or every trial's for a run that stands for a trial group.
type attempt struct {
	run         *engine.RunResult
	turnIndex   int
	description string
	expect      string
	attempts    int
	achieved    int
	achievedAt  []int
	unevaluated int
	unreliable  int
	// baseline marks a no-attack baseline objective's attempts.
	baseline bool
	// budget is how the run's trial_budget ended its trials. Every attempt of
	// the run carries it; a group counts each run once.
	budget *engine.TrialBudgetResult
	// cost is this objective's share of the run's cost.
	cost float64
}

// objectiveMeta is what the report knows about an objective on a scenario
// turn from the runs that recorded it.
type objectiveMeta struct {
	description string
	expect      string
	baseline    bool
}

// Build makes the report. Runs without an objective are left out. A run that
// errored before recording an objective its scenario's other runs recorded
// gets an unevaluated attempt for it, so its failure and its cost are not
// dropped.
func Build(runs []engine.RunResult, groupBy []string) *Report {
	perRun := make([][]attempt, len(runs))
	catalog := map[string]map[int]objectiveMeta{}
	for i := range runs {
		perRun[i] = attemptsOf(&runs[i])
		for _, a := range perRun[i] {
			addToCatalog(catalog, &a)
		}
	}
	var all []attempt
	for i := range runs {
		attempts := perRun[i]
		if runs[i].Error != "" && !isTrialGroup(&runs[i]) {
			attempts = append(attempts, missingAttempts(&runs[i], attempts, catalog[runs[i].ScenarioID])...)
		}
		for j := range attempts {
			attempts[j].cost = runCost(&runs[i]) / float64(len(attempts))
		}
		all = append(all, attempts...)
	}
	report := &Report{Totals: groupAttempts(all, "")}
	for _, key := range groupBy {
		report.Groupings = append(report.Groupings, Grouping{By: key, Groups: groupAttempts(all, key)})
	}
	return report
}

// addToCatalog records an objective a run recorded. When runs of the same
// scenario turn disagree on the description, as dataset rows that fill in
// placeholders do, the catalog keeps none, so a missing attempt is never
// credited to another row's objective.
func addToCatalog(catalog map[string]map[int]objectiveMeta, a *attempt) {
	turns := catalog[a.run.ScenarioID]
	if turns == nil {
		turns = map[int]objectiveMeta{}
		catalog[a.run.ScenarioID] = turns
	}
	meta, seen := turns[a.turnIndex]
	if !seen {
		turns[a.turnIndex] = objectiveMeta{a.description, a.expect, a.baseline}
		return
	}
	if meta.description != a.description {
		meta.description = ""
		turns[a.turnIndex] = meta
	}
}

// missingAttempts returns an unevaluated attempt for each objective in the
// scenario's catalog that the run did not record, in turn order.
func missingAttempts(run *engine.RunResult, recorded []attempt, catalog map[int]objectiveMeta) []attempt {
	have := map[int]bool{}
	for _, a := range recorded {
		have[a.turnIndex] = true
	}
	turns := make([]int, 0, len(catalog))
	for turn := range catalog {
		if !have[turn] {
			turns = append(turns, turn)
		}
	}
	sort.Ints(turns)
	out := make([]attempt, 0, len(turns))
	for _, turn := range turns {
		m := catalog[turn]
		out = append(out, attempt{
			run: run, turnIndex: turn, description: m.description, expect: m.expect, baseline: m.baseline, unevaluated: 1,
		})
	}
	return out
}

func isTrialGroup(run *engine.RunResult) bool {
	return run.TrialResults != nil && len(run.TrialResults.Objectives) > 0
}

// attemptsOf reads a run's objective attempts, from its trial results when
// it stands for a trial group.
func attemptsOf(run *engine.RunResult) []attempt {
	if isTrialGroup(run) {
		out := make([]attempt, 0, len(run.TrialResults.Objectives))
		for _, o := range run.TrialResults.Objectives {
			out = append(out, attempt{
				run: run, turnIndex: o.TurnIndex, description: o.Description, expect: expectOrDefault(o.Expect),
				attempts: o.Attempts, achieved: o.Achieved, achievedAt: o.AchievedAtExchange,
				unevaluated: o.Unevaluated, unreliable: o.Unreliable, baseline: o.Baseline,
				budget: run.TrialResults.Budget,
			})
		}
		return out
	}
	outcomes := engine.ObjectiveOutcomes(run.ConversationAssertions.Results)
	unreliable := engine.UnreliableTurns(run.ConversationAssertions.Results)
	out := make([]attempt, 0, len(outcomes))
	for _, o := range outcomes {
		a := attempt{
			run: run, turnIndex: o.TurnIndex, description: o.Description, expect: expectOrDefault(o.Expect),
			baseline: o.Baseline,
		}
		switch {
		case !o.Evaluated:
			a.unevaluated = 1
		case unreliable[o.TurnIndex]:
			a.unreliable = 1
		case o.Achieved:
			a.attempts, a.achieved, a.achievedAt = 1, 1, []int{o.AchievedAtExchange}
		default:
			a.attempts = 1
		}
		out = append(out, a)
	}
	return out
}

func expectOrDefault(expect string) string {
	if expect == "" {
		return expectAchieved
	}
	return expect
}

// expectAchieved is the default expected outcome of an objective.
const expectAchieved = "achieved"

// groupAttempts splits attempts by the group-by key's value and their
// expected outcome, sorted by value and then expectation. An empty key puts
// every attempt in one value, "all".
func groupAttempts(all []attempt, key string) []Group {
	type groupKey struct{ value, expect string }
	byKey := map[groupKey][]attempt{}
	for _, a := range all {
		v := "all"
		if key != "" {
			v = groupValue(&a, key)
		}
		k := groupKey{v, a.expect}
		byKey[k] = append(byKey[k], a)
	}
	keys := make([]groupKey, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].value != keys[j].value {
			return keys[i].value < keys[j].value
		}
		return keys[i].expect < keys[j].expect
	})
	groups := make([]Group, 0, len(keys))
	for _, k := range keys {
		groups = append(groups, summarize(k.value, k.expect, byKey[k]))
	}
	return groups
}

func groupValue(a *attempt, key string) string {
	var v string
	switch {
	case key == GroupScenario:
		v = a.run.ScenarioID
	case key == GroupProvider:
		v = a.run.ProviderID
	case key == GroupObjective:
		v = objectiveName(a)
	case strings.HasPrefix(key, groupLabel):
		v = a.run.Labels[strings.TrimPrefix(key, groupLabel)]
	case strings.HasPrefix(key, groupData):
		v = dataValue(a.run, strings.TrimPrefix(key, groupData))
	}
	if v == "" {
		return noValue
	}
	return v
}

// objectiveName names an attempt's objective: its description, or its
// scenario and turn when it has none.
func objectiveName(a *attempt) string {
	if a.description != "" {
		return a.description
	}
	return fmt.Sprintf("%s turn %d objective", a.run.ScenarioID, a.turnIndex)
}

// dataValue reads a dataset column from a run's Params.data.
func dataValue(run *engine.RunResult, column string) string {
	data, _ := run.Params["data"].(map[string]interface{})
	if v, ok := data[column]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// summarize totals a set of attempts. Baseline attempts are totalled apart,
// as the group's Baseline, and Lift compares the two.
func summarize(value, expect string, attempts []attempt) Group {
	var attack, baseline []attempt
	for _, a := range attempts {
		if a.baseline {
			baseline = append(baseline, a)
		} else {
			attack = append(attack, a)
		}
	}
	g := tally(value, expect, attack)
	addBudgets(&g, attempts)
	if len(baseline) > 0 {
		b := tally(value, expect, baseline)
		g.Baseline = &b
		g.Lift = lift(attack, baseline)
	}
	return g
}

// tally totals a set of attempts into a group.
func tally(value, expect string, attempts []attempt) Group {
	g := Group{Value: value, Expect: expect}
	exchangeSum, exchangeCount := 0, 0
	for _, a := range attempts {
		g.Attempts += a.attempts
		g.Achieved += a.achieved
		g.Unevaluated += a.unevaluated
		g.Unreliable += a.unreliable
		g.Cost += a.cost
		for _, x := range a.achievedAt {
			exchangeSum += x
			exchangeCount++
		}
	}
	if g.Attempts > 0 {
		g.Rate = float64(g.Achieved) / float64(g.Attempts)
	}
	g.CILow, g.CIHigh = Wilson(g.Achieved, g.Attempts)
	if exchangeCount > 0 {
		g.MeanExchangesToAchieve = float64(exchangeSum) / float64(exchangeCount)
	}
	if g.Achieved > 0 {
		g.CostPerAchieved = g.Cost / float64(g.Achieved)
	}
	return g
}

// addBudgets totals the trial budgets of the runs behind a set of attempts,
// counting each run once however many of its objectives are in the set.
func addBudgets(g *Group, attempts []attempt) {
	seen := map[*engine.RunResult]bool{}
	trialsToFirst := 0
	for i := range attempts {
		a := &attempts[i]
		if a.budget == nil || seen[a.run] {
			continue
		}
		seen[a.run] = true
		g.BudgetedGroups++
		g.TrialsSkipped += a.budget.TrialsSkipped
		if a.budget.TrialsToFirstAchieved > 0 {
			g.FirstAchievedGroups++
			trialsToFirst += a.budget.TrialsToFirstAchieved
		}
	}
	if g.BudgetedGroups == 0 {
		return
	}
	g.GroupsCILow, g.GroupsCIHigh = Wilson(g.FirstAchievedGroups, g.BudgetedGroups)
	if g.FirstAchievedGroups > 0 {
		g.MeanTrialsToFirstAchieved = float64(trialsToFirst) / float64(g.FirstAchievedGroups)
	}
}

// lift is the attack's rate minus the baseline's, over the objectives both
// attempted, paired by objective (see objectiveName). An objective with no
// baseline, or a baseline with no attack, is left out of both rates, so a
// group mixing them doesn't credit the attack with a lift it never earned.
// It is nil when no objective has evaluated attempts on both sides.
func lift(attack, baseline []attempt) *float64 {
	inAttack, inBaseline := map[string]bool{}, map[string]bool{}
	for i := range attack {
		inAttack[objectiveName(&attack[i])] = true
	}
	for i := range baseline {
		inBaseline[objectiveName(&baseline[i])] = true
	}
	paired := func(as []attempt, other map[string]bool) []attempt {
		var out []attempt
		for i := range as {
			if other[objectiveName(&as[i])] {
				out = append(out, as[i])
			}
		}
		return out
	}
	a := tally("", "", paired(attack, inBaseline))
	b := tally("", "", paired(baseline, inAttack))
	if a.Attempts == 0 || b.Attempts == 0 {
		return nil
	}
	l := a.Rate - b.Rate
	return &l
}

// runCost is the cost of a run, or of all its trials when it stands for a
// trial group.
func runCost(run *engine.RunResult) float64 {
	if run.TrialResults != nil && run.TrialResults.TotalCost > 0 {
		return run.TrialResults.TotalCost
	}
	return run.Cost.TotalCost
}

// Wilson returns the 95% Wilson score interval for successes out of n. With
// no trials the interval is the whole range [0, 1].
func Wilson(successes, n int) (low, high float64) {
	if n <= 0 {
		return 0, 1
	}
	p := float64(successes) / float64(n)
	nf := float64(n)
	z2 := wilsonZ * wilsonZ
	center := p + z2/(2*nf) //nolint:mnd // constant of the Wilson score formula
	margin := wilsonZ * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf))
	denom := 1 + z2/nf
	low = math.Max(0, (center-margin)/denom)
	high = math.Min(1, (center+margin)/denom)
	return low, high
}
