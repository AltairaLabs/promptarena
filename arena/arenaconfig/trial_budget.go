package arenaconfig

import "fmt"

// TrialBudget stops a scenario's trials early. Each provider, region and
// dataset row runs its trials as a group. A budgeted group runs its trials
// one at a time, and once its finished trials meet a condition below, no more
// of them start.
type TrialBudget struct {
	// StopOnAchieved starts no more trials once one has achieved every
	// objective in the scenario: a best-of-N attempt budget, where the
	// question is whether any attempt gets through.
	StopOnAchieved bool `json:"stop_on_achieved,omitempty" yaml:"stop_on_achieved,omitempty"`
	// MaxCostUSD starts no more trials once the group's finished trials have
	// cost at least this much, in USD.
	MaxCostUSD float64 `json:"max_cost_usd,omitempty" yaml:"max_cost_usd,omitempty" jsonschema:"minimum=0"`
}

// validate checks the budget against the scenario it belongs to.
func (b *TrialBudget) validate(s *Scenario) error {
	if s.Trials <= 1 {
		return fmt.Errorf("trial_budget needs trials greater than 1")
	}
	for i := range s.Turns {
		if hasVariants(s.Turns[i].Perturbations) {
			// Trials cycle through the variants, so a budget that stopped
			// early would leave some variants unrun.
			return fmt.Errorf("trial_budget can't be used with turn perturbations")
		}
	}
	if b.MaxCostUSD < 0 {
		return fmt.Errorf("trial_budget.max_cost_usd must not be negative, got %v", b.MaxCostUSD)
	}
	if !b.StopOnAchieved && b.MaxCostUSD == 0 {
		return fmt.Errorf("trial_budget needs stop_on_achieved or max_cost_usd")
	}
	if b.StopOnAchieved && !hasTurnObjective(s) {
		return fmt.Errorf("trial_budget.stop_on_achieved needs a turn with an objective")
	}
	return nil
}

// hasVariants reports whether perturbations produce any variant: a key with
// no values adds none.
func hasVariants(perturbations map[string][]string) bool {
	for _, values := range perturbations {
		if len(values) > 0 {
			return true
		}
	}
	return false
}

func hasTurnObjective(s *Scenario) bool {
	for i := range s.Turns {
		if s.Turns[i].Objective != nil {
			return true
		}
	}
	return false
}
