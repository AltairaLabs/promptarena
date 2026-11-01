package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func objectiveScenario(turn TurnDefinition) *Scenario {
	return &Scenario{ID: "s", Turns: []TurnDefinition{{Role: "user", Content: "hi"}, turn}}
}

func validObjective() *ObjectiveConfig {
	return &ObjectiveConfig{Success: []AssertionConfig{{Type: "content_includes_any"}}}
}

func TestScenarioValidate_Objective(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		s := objectiveScenario(TurnDefinition{Role: "attacker", Persona: "p", Objective: validObjective()})
		assert.NoError(t, s.Validate())
	})

	tests := []struct {
		name string
		turn TurnDefinition
		mod  func(*Scenario)
		want string
	}{
		{name: "needs a persona", turn: TurnDefinition{Role: "user", Objective: validObjective()}, want: "self-play turn"},
		{name: "needs success", turn: TurnDefinition{Persona: "p", Objective: &ObjectiveConfig{}}, want: "at least one assertion"},
		{
			name: "bad expect",
			turn: TurnDefinition{Persona: "p", Objective: &ObjectiveConfig{Expect: "maybe", Success: validObjective().Success}},
			want: "invalid value",
		},
		{
			name: "duplex unsupported",
			turn: TurnDefinition{Persona: "p", Objective: validObjective()},
			mod:  func(s *Scenario) { s.Duplex = &DuplexConfig{} },
			want: "duplex",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := objectiveScenario(tt.turn)
			if tt.mod != nil {
				tt.mod(s)
			}
			err := s.validateObjectives()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Contains(t, err.Error(), "turns[1].objective")
		})
	}
}

func TestObjectiveConfig_Defaults(t *testing.T) {
	o := &ObjectiveConfig{}
	assert.True(t, o.ExpectsAchieved())
	assert.True(t, o.StopsOnSuccess())

	no := false
	o = &ObjectiveConfig{Expect: ObjectiveExpectNotAchieved, StopOnSuccess: &no}
	assert.False(t, o.ExpectsAchieved())
	assert.False(t, o.StopsOnSuccess())
}

func TestForDataRow_SubstitutesObjective(t *testing.T) {
	s := objectiveScenario(TurnDefinition{
		Persona: "p",
		Objective: &ObjectiveConfig{
			Description: "get {goal}",
			Success: []AssertionConfig{{
				Type:    "content_includes_any",
				Params:  map[string]interface{}{"patterns": []interface{}{"{marker}"}},
				Message: "saw {marker}",
			}},
		},
	})

	out := s.ForDataRow(DataRow{ID: "r1", Values: map[string]string{"goal": "a refund", "marker": "REFUND"}})

	obj := out.Turns[1].Objective
	assert.Equal(t, "get a refund", obj.Description)
	assert.Equal(t, "saw REFUND", obj.Success[0].Message)
	assert.Equal(t, []interface{}{"REFUND"}, obj.Success[0].Params["patterns"])

	orig := s.Turns[1].Objective
	assert.Equal(t, "get {goal}", orig.Description, "original scenario is not modified")
	assert.Equal(t, []interface{}{"{marker}"}, orig.Success[0].Params["patterns"])
}
