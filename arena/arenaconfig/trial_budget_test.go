package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrialBudget_Validate(t *testing.T) {
	withObjective := []TurnDefinition{{Role: "attacker", Persona: "p", Objective: &ObjectiveConfig{
		Success: []AssertionConfig{{Type: "content_includes"}},
	}}}
	cases := []struct {
		name   string
		s      Scenario
		errMsg string
	}{
		{"stop on achieved", Scenario{ID: "s", Trials: 5, Turns: withObjective,
			TrialBudget: &TrialBudget{StopOnAchieved: true}}, ""},
		{"cost cap without objective", Scenario{ID: "s", Trials: 5, TrialBudget: &TrialBudget{MaxCostUSD: 1}}, ""},
		{"one trial", Scenario{ID: "s", Trials: 1, Turns: withObjective,
			TrialBudget: &TrialBudget{StopOnAchieved: true}}, "trial_budget needs trials greater than 1"},
		{"empty", Scenario{ID: "s", Trials: 5, TrialBudget: &TrialBudget{}},
			"trial_budget needs stop_on_achieved or max_cost_usd"},
		{"negative cost", Scenario{ID: "s", Trials: 5, TrialBudget: &TrialBudget{MaxCostUSD: -1}},
			"max_cost_usd must not be negative"},
		{"no objective", Scenario{ID: "s", Trials: 5, TrialBudget: &TrialBudget{StopOnAchieved: true}},
			"stop_on_achieved needs a turn with an objective"},
		{"perturbations", Scenario{ID: "s", Trials: 5, Turns: []TurnDefinition{{Role: "user", Content: "{x}",
			Perturbations: map[string][]string{"x": {"a", "b"}}}}, TrialBudget: &TrialBudget{MaxCostUSD: 1}},
			"trial_budget can't be used with turn perturbations"},
		{"empty perturbations", Scenario{ID: "s", Trials: 5, Turns: []TurnDefinition{{Role: "user", Content: "hi",
			Perturbations: map[string][]string{"x": {}}}}, TrialBudget: &TrialBudget{MaxCostUSD: 1}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.s.Validate()
			if c.errMsg == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.errMsg)
			assert.Contains(t, err.Error(), "scenario s: ")
		})
	}
}
