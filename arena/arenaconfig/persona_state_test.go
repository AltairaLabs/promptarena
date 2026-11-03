package arenaconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func intPtr(n int) *int { return &n }

const stateYAML = `
id: customer
description: d
system_template: "patience {{state.patience}}"
state:
  patience: { type: int, initial: 3, min: 0 }
  gave_order_number: { type: bool }
  mood: { type: string, initial: calm }
transitions:
  - when: [{ type: content_includes_any, params: { patterns: [verify] } }]
    add: { patience: -1 }
  - when: [{ type: content_includes_any, params: { patterns: [A-100] } }]
    target: persona
    set: { gave_order_number: true, mood: curt }
stop_when:
  - state: patience
    equals: 0
    outcome: abandoned
    fail: true
`

func loadStatePersona(t *testing.T) *UserPersonaPack {
	t.Helper()
	var p UserPersonaPack
	require.NoError(t, yaml.Unmarshal([]byte(stateYAML), &p))
	require.NoError(t, validatePersonaState(&p))
	return &p
}

func TestPersonaState_InitialAndVars(t *testing.T) {
	p := loadStatePersona(t)
	state := p.InitialState()
	assert.Equal(t, map[string]interface{}{"patience": 3, "gave_order_number": false, "mood": "calm"}, state)

	vars := p.StateVars(state)
	assert.Equal(t, "3", vars["state.patience"])
	assert.Equal(t, "false", vars["state.gave_order_number"])
	assert.Equal(t, "- gave_order_number: false\n- mood: calm\n- patience: 3", vars["state"])
	assert.True(t, p.Transitions[1].ChecksPersona())
	assert.False(t, p.Transitions[0].ChecksPersona())
}

func TestPersonaState_ApplyClampsAndSets(t *testing.T) {
	p := loadStatePersona(t)
	state := p.InitialState()
	for i := 0; i < 5; i++ {
		state = p.Apply(&p.Transitions[0], state)
	}
	assert.Equal(t, 0, state["patience"], "min 0 holds")
	state = p.Apply(&p.Transitions[1], state)
	assert.Equal(t, true, state["gave_order_number"])
	assert.Equal(t, "curt", state["mood"])

	stop := p.StopWhen[0]
	assert.True(t, stop.Matches(0))
	assert.False(t, stop.Matches(1))
	assert.Equal(t, "abandoned", stop.OutcomeLabel())
}

func TestPersonaStopCondition_Comparisons(t *testing.T) {
	c := PersonaStopCondition{State: "n", AtMost: intPtr(2), AtLeast: intPtr(1)}
	assert.True(t, c.Matches(1))
	assert.True(t, c.Matches(2))
	assert.False(t, c.Matches(3))
	assert.False(t, c.Matches(0))
	assert.False(t, c.Matches("x"))
	assert.Equal(t, DefaultPersonaStopOutcome, c.OutcomeLabel())
	assert.True(t, (&PersonaStopCondition{Equals: 2.0}).Matches(2), "a JSON number matches an int")
}

func TestPersonaState_InitialDefaults(t *testing.T) {
	p := &UserPersonaPack{State: map[string]PersonaStateVar{
		"m": {Type: PersonaStateInt, Min: intPtr(2)},
		"s": {Type: PersonaStateString},
		"u": {Type: PersonaStateInt, Initial: uint64(4)},
	}}
	assert.Equal(t, map[string]interface{}{"m": 2, "s": "", "u": 4}, p.InitialState())
}

func TestPersonaState_StateStrings(t *testing.T) {
	p := loadStatePersona(t)
	assert.ElementsMatch(t, []string{"calm", "curt"}, p.StateStrings())
}

func TestValidatePersonaState_Errors(t *testing.T) {
	when := []AssertionConfig{{Type: "content_includes_any"}}
	tests := []struct {
		name string
		p    UserPersonaPack
		want string
	}{
		{"transitions without state", UserPersonaPack{Transitions: []PersonaTransition{{When: when}}}, "need a state"},
		{"bad name", UserPersonaPack{State: map[string]PersonaStateVar{"1x": {Type: "int"}}}, "letters, digits"},
		{"bad type", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "float"}}}, "type must be"},
		{"min over max", UserPersonaPack{State: map[string]PersonaStateVar{
			"x": {Type: "int", Min: intPtr(3), Max: intPtr(1)}}}, "greater than max"},
		{"bounds on bool", UserPersonaPack{State: map[string]PersonaStateVar{
			"x": {Type: "bool", Min: intPtr(0)}}}, "only to an int"},
		{"initial out of bounds", UserPersonaPack{State: map[string]PersonaStateVar{
			"x": {Type: "int", Initial: 9, Max: intPtr(5)}}}, "outside min/max"},
		{"initial type", UserPersonaPack{State: map[string]PersonaStateVar{
			"x": {Type: "int", Initial: "three"}}}, "is not a int"},
		{"no when", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "int"}},
			Transitions: []PersonaTransition{{Add: map[string]int{"x": 1}}}}, "when: at least one"},
		{"bad target", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "int"}},
			Transitions: []PersonaTransition{{When: when, Target: "user", Add: map[string]int{"x": 1}}}}, "target must be"},
		{"no change", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "int"}},
			Transitions: []PersonaTransition{{When: when}}}, "set or add"},
		{"set unknown", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "int"}},
			Transitions: []PersonaTransition{{When: when, Set: map[string]interface{}{"y": 1}}}}, "set.y: not a declared"},
		{"set type", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "int"}},
			Transitions: []PersonaTransition{{When: when, Set: map[string]interface{}{"x": true}}}}, "set.x: true is not a int"},
		{"add unknown", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "int"}},
			Transitions: []PersonaTransition{{When: when, Add: map[string]int{"y": 1}}}}, "add.y: not a declared"},
		{"add bool", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "bool"}},
			Transitions: []PersonaTransition{{When: when, Add: map[string]int{"x": 1}}}}, "add applies only"},
		{"stop unknown", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "int"}},
			StopWhen: []PersonaStopCondition{{State: "y", Equals: 1}}}, "not a declared"},
		{"stop no comparison", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "int"}},
			StopWhen: []PersonaStopCondition{{State: "x"}}}, "one of equals"},
		{"stop equals type", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "bool"}},
			StopWhen: []PersonaStopCondition{{State: "x", Equals: "yes"}}}, "is not a bool"},
		{"stop at_most bool", UserPersonaPack{State: map[string]PersonaStateVar{"x": {Type: "bool"}},
			StopWhen: []PersonaStopCondition{{State: "x", AtMost: intPtr(1)}}}, "only to an int"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.p.ID = "p"
			err := validatePersonaState(&tt.p)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestPersonaState_TemplateRendersInitialState(t *testing.T) {
	p := loadStatePersona(t)
	out, err := p.buildTemplatedPrompt("", nil)
	require.NoError(t, err)
	assert.Equal(t, "patience 3", out)

	out, err = p.buildTemplatedPrompt("", map[string]string{"state.patience": "1"})
	require.NoError(t, err)
	assert.Equal(t, "patience 1", out, "the run's state wins")
}
