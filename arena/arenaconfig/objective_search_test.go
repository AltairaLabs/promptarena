package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func searchObjective(search *ObjectiveSearchConfig) *ObjectiveConfig {
	o := validObjective()
	o.Search = search
	return o
}

func TestScenarioValidate_ObjectiveSearch(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		s := objectiveScenario(TurnDefinition{Role: "attacker", Persona: "p", Objective: searchObjective(
			&ObjectiveSearchConfig{Strategy: "tree", Branching: 2, Width: 3, Depth: 4, Judge: "j", PruneOffTopic: true},
		)})
		assert.NoError(t, s.Validate())
	})

	no := false
	tests := []struct {
		name string
		turn TurnDefinition
		want string
	}{
		{
			name: "unknown strategy",
			turn: TurnDefinition{Persona: "p", Objective: searchObjective(&ObjectiveSearchConfig{Strategy: "beam"})},
			want: `search.strategy: invalid value "beam"`,
		},
		{
			name: "missing strategy",
			turn: TurnDefinition{Persona: "p", Objective: searchObjective(&ObjectiveSearchConfig{})},
			want: "search.strategy",
		},
		{
			name: "negative size",
			turn: TurnDefinition{Persona: "p", Objective: searchObjective(&ObjectiveSearchConfig{Strategy: "tree", Width: -1})},
			want: "must not be negative",
		},
		{
			name: "prune needs a judge",
			turn: TurnDefinition{Persona: "p", Objective: searchObjective(
				&ObjectiveSearchConfig{Strategy: "tree", PruneOffTopic: true})},
			want: "prune_off_topic: needs a judge",
		},
		{
			name: "rewind",
			turn: TurnDefinition{Persona: "p", Objective: func() *ObjectiveConfig {
				o := searchObjective(&ObjectiveSearchConfig{Strategy: "tree"})
				o.Rewind = &ObjectiveRewindConfig{When: validObjective().Success}
				return o
			}()},
			want: "rewind and search can't be combined",
		},
		{
			name: "stop_on_success false",
			turn: TurnDefinition{Persona: "p", Objective: func() *ObjectiveConfig {
				o := searchObjective(&ObjectiveSearchConfig{Strategy: "tree"})
				o.StopOnSuccess = &no
				return o
			}()},
			want: "stop_on_success: false",
		},
		{
			name: "turns",
			turn: TurnDefinition{Persona: "p", Turns: 3, Objective: searchObjective(&ObjectiveSearchConfig{Strategy: "tree"})},
			want: "set search.depth instead",
		},
		{
			name: "max_turns",
			turn: TurnDefinition{Persona: "p", MaxTurns: 3, Objective: searchObjective(&ObjectiveSearchConfig{Strategy: "tree"})},
			want: "turns and max_turns don't apply",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := objectiveScenario(tt.turn).validateObjectives()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Contains(t, err.Error(), "turns[1]")
		})
	}

	t.Run("duplex", func(t *testing.T) {
		s := objectiveScenario(TurnDefinition{Persona: "p", Objective: searchObjective(&ObjectiveSearchConfig{Strategy: "tree"})})
		s.Duplex = &DuplexConfig{}
		err := s.validateObjectives()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplex")
	})
}

func TestObjectiveSearchConfig_Defaults(t *testing.T) {
	s := &ObjectiveSearchConfig{Strategy: "tree"}
	assert.Equal(t, DefaultSearchBranching, s.BranchingOrDefault())
	assert.Equal(t, DefaultSearchWidth, s.WidthOrDefault())
	assert.Equal(t, DefaultSearchDepth, s.DepthOrDefault())

	s = &ObjectiveSearchConfig{Strategy: "tree", Branching: 2, Width: 7, Depth: 9}
	assert.Equal(t, 2, s.BranchingOrDefault())
	assert.Equal(t, 7, s.WidthOrDefault())
	assert.Equal(t, 9, s.DepthOrDefault())
}

func TestConfig_ValidateSearchJudges(t *testing.T) {
	scenario := objectiveScenario(TurnDefinition{Persona: "p", Objective: searchObjective(
		&ObjectiveSearchConfig{Strategy: "tree", Judge: "scorer"})})
	plain := objectiveScenario(TurnDefinition{Persona: "p", Objective: validObjective()})

	c := &Config{LoadedScenarios: map[string]*Scenario{"s": scenario, "plain": plain}}
	err := c.validateSearchJudges()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `scenario s: turns[1].objective.search.judge: no judge "scorer" in spec.judges`)

	c.LoadedJudges = map[string]*JudgeTarget{"scorer": {}}
	assert.NoError(t, c.validateSearchJudges())
}
