package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/turnexecutors"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func includes(patterns ...interface{}) []arenaconfig.AssertionConfig {
	return []arenaconfig.AssertionConfig{{
		Type: "content_includes_any", Params: map[string]interface{}{"patterns": patterns},
	}}
}

func statefulPersona(stop ...arenaconfig.PersonaStopCondition) *arenaconfig.UserPersonaPack {
	minPatience := 0
	return &arenaconfig.UserPersonaPack{
		ID:           "customer",
		SystemPrompt: "You want to know where your order is.",
		State: map[string]arenaconfig.PersonaStateVar{
			"patience":          {Type: arenaconfig.PersonaStateInt, Initial: 2, Min: &minPatience},
			"gave_order_number": {Type: arenaconfig.PersonaStateBool},
		},
		Transitions: []arenaconfig.PersonaTransition{
			{When: includes("verify"), Add: map[string]int{"patience": -1}},
			{When: includes("A-100"), Target: arenaconfig.PersonaTargetPersona,
				Set: map[string]interface{}{"gave_order_number": true}},
		},
		StopWhen: stop,
	}
}

// exchangeScript is what the persona and the agent say on one exchange.
type exchangeScript struct{ persona, agent string }

// runState runs a scripted opening turn, then a self-play turn of the given
// exchanges (or more turns, repeating the last). It returns the result and the
// persona variables each exchange was generated with.
func runState(t *testing.T, persona *arenaconfig.UserPersonaPack, turns []arenaconfig.TurnDefinition,
	script ...exchangeScript) (*ConversationResult, []map[string]string) {
	t.Helper()
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{
			{Role: "user", Content: "Hi"},
			{Role: "assistant", Content: "Please verify your identity."},
		})
	}}
	var seen []map[string]string
	selfPlay := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		vars := map[string]string{}
		for k, v := range req.SelfPlayVars {
			vars[k] = v
		}
		seen = append(seen, vars)
		s := script[min(len(seen), len(script))-1]
		return saveMessagesToStateStore(ctx, req, []types.Message{
			{Role: "user", Content: s.persona}, {Role: "assistant", Content: s.agent},
		})
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, fidelityRegistry(t, persona),
		createTestPromptRegistry(t), nil)
	executor.evalOrchestrator = NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")
	all := []arenaconfig.TurnDefinition{{Role: "user", Content: "Hi"}}
	for _, turn := range turns {
		turn.Role = "attacker"
		turn.Persona = persona.ID
		all = append(all, turn)
	}
	res := executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region:           "default",
		Scenario:         &arenaconfig.Scenario{ID: "state", TaskType: "assistant", Turns: all},
		Provider:         &MockProvider{id: "test"},
		Config:           &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"},
		ConversationID:   "state-conv",
	})
	return res, seen
}

func stateResults(res *ConversationResult) []asrt.ConversationValidationResult {
	var out []asrt.ConversationValidationResult
	for _, r := range res.ConversationAssertionResults {
		if r.Type == personaStateResultType {
			out = append(out, r)
		}
	}
	return out
}

func TestPersonaState_TransitionsUpdateStateAndVariables(t *testing.T) {
	res, seen := runState(t, statefulPersona(), []arenaconfig.TurnDefinition{{Turns: 3}},
		exchangeScript{"Where is my order?", "Can you verify your email?"},
		exchangeScript{"It's A-100.", "Thanks, checking."},
		exchangeScript{"Any news?", "It ships tomorrow."},
	)
	require.False(t, res.Failed, res.Error)

	require.Len(t, seen, 3)
	assert.Equal(t, "2", seen[0]["state.patience"])
	assert.Equal(t, "- gave_order_number: false\n- patience: 2", seen[0]["state"])
	assert.Equal(t, "1", seen[1]["state.patience"], "the agent asked to verify on exchange 1")
	assert.Equal(t, "false", seen[1]["state.gave_order_number"])
	assert.Equal(t, "true", seen[2]["state.gave_order_number"], "the persona gave A-100 on exchange 2")
	assert.Equal(t, "1", seen[2]["state.patience"],
		"the opening turn's 'verify' is not re-read: the checks see only the latest exchange")

	results := stateResults(res)
	require.Len(t, results, 1)
	r := results[0]
	assert.True(t, r.Passed)
	assert.Equal(t, "persona customer state gave_order_number false→true, patience 2→1", r.Message)
	changes, ok := r.Details["changes"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, changes, 2)
	assert.Equal(t, 1, changes[0]["exchange"])
	assert.Equal(t, 0, changes[0]["transition"])
	assert.Equal(t, map[string]interface{}{"patience": 2, "gave_order_number": false}, changes[0]["before"])
	assert.Equal(t, 2, changes[1]["exchange"])
	assert.Equal(t, 1, changes[1]["transition"])
	assert.Equal(t, 3, r.Details["exchanges"])
	assert.NotContains(t, r.Details, "outcome")
}

func TestPersonaState_PersonaTargetReadsOnlyThePersonaMessage(t *testing.T) {
	res, _ := runState(t, statefulPersona(), []arenaconfig.TurnDefinition{{Turns: 1}},
		exchangeScript{"Where is it?", "Order A-100 is on its way."})
	r := stateResults(res)[0]
	assert.Equal(t, map[string]interface{}{"patience": 2, "gave_order_number": false}, r.Details["final"],
		"the agent saying A-100 is not the persona giving it")
	assert.Equal(t, "persona customer state unchanged", r.Message)
}

func TestPersonaState_StopWhenEndsTheTurnAndIsReported(t *testing.T) {
	persona := statefulPersona(arenaconfig.PersonaStopCondition{
		State: "patience", Equals: 0, Outcome: "abandoned", Fail: true,
	})
	turn := arenaconfig.TurnDefinition{Turns: 1, MaxTurns: 5, Objective: refundObjective("", nil)}
	res, seen := runState(t, persona, []arenaconfig.TurnDefinition{turn},
		exchangeScript{"Where is my order?", "Please verify first."})

	assert.Len(t, seen, 2, "patience 2→1→0 ends the turn after exchange 2")
	r := stateResults(res)[0]
	assert.False(t, r.Passed, "fail: true fails the run")
	assert.Equal(t, "abandoned", r.Details["outcome"])
	assert.Equal(t, 2, r.Details["stopped_at_exchange"])
	assert.Equal(t, "persona customer state patience 2→0; abandoned at exchange 2", r.Message)

	obj := objectiveResult(t, res)
	assert.Equal(t, "abandoned", obj.Details["persona_outcome"])
	assert.Equal(t, "get a refund: not achieved in 2 exchanges (persona abandoned)", obj.Message)
}

func TestPersonaState_StopWithoutFailPasses(t *testing.T) {
	persona := statefulPersona(arenaconfig.PersonaStopCondition{State: "gave_order_number", Equals: true})
	res, seen := runState(t, persona, []arenaconfig.TurnDefinition{{Turns: 1, MaxTurns: 4}},
		exchangeScript{"It's A-100.", "Thanks."})
	assert.Len(t, seen, 1)
	r := stateResults(res)[0]
	assert.True(t, r.Passed)
	assert.Equal(t, arenaconfig.DefaultPersonaStopOutcome, r.Details["outcome"])
}

func TestPersonaState_ObjectiveStopTakesPrecedence(t *testing.T) {
	persona := statefulPersona(arenaconfig.PersonaStopCondition{State: "patience", AtMost: intPtrEngine(1)})
	turn := arenaconfig.TurnDefinition{Turns: 1, MaxTurns: 3, Objective: refundObjective("", nil)}
	res, _ := runState(t, persona, []arenaconfig.TurnDefinition{turn},
		exchangeScript{"Refund please.", "Please verify. REFUND ISSUED."})

	r := stateResults(res)[0]
	assert.NotContains(t, r.Details, "outcome", "the objective ended the turn, not the state")
	assert.Equal(t, map[string]interface{}{"patience": 1, "gave_order_number": false}, r.Details["final"])
	assert.NotContains(t, objectiveResult(t, res).Details, "persona_outcome")
}

func TestPersonaState_RewoundExchangeChangesNothing(t *testing.T) {
	obj := refundObjective("", nil)
	obj.Rewind = &arenaconfig.ObjectiveRewindConfig{When: includes("verify"), Max: 1}
	res, seen := runState(t, statefulPersona(), []arenaconfig.TurnDefinition{{Turns: 2, Objective: obj}},
		exchangeScript{"Where is my order?", "Please verify."},
		exchangeScript{"Any news?", "On its way."},
	)
	require.Len(t, seen, 2)
	assert.Equal(t, "2", seen[1]["state.patience"], "the rewound exchange's 'verify' did not cost patience")
	r := stateResults(res)[0]
	assert.Equal(t, "persona customer state unchanged", r.Message)
}

func TestPersonaState_CarriesAcrossTurns(t *testing.T) {
	res, seen := runState(t, statefulPersona(), []arenaconfig.TurnDefinition{{Turns: 1}, {Turns: 1}},
		exchangeScript{"Where is my order?", "Please verify."})
	require.Len(t, seen, 2)
	assert.Equal(t, "1", seen[1]["state.patience"], "the second turn starts where the first left off")
	results := stateResults(res)
	require.Len(t, results, 2)
	assert.Equal(t, map[string]interface{}{"patience": 1, "gave_order_number": false}, results[1].Details["initial"])
	assert.Equal(t, 0, results[1].Details["final"].(map[string]interface{})["patience"])
	assert.Equal(t, 2, results[1].Details["turn_index"])
}

func TestPersonaState_NoTransitionsAddsNoResult(t *testing.T) {
	persona := &arenaconfig.UserPersonaPack{
		ID: "customer", SystemPrompt: "p",
		State: map[string]arenaconfig.PersonaStateVar{"mood": {Type: arenaconfig.PersonaStateString}},
	}
	res, _ := runState(t, persona, []arenaconfig.TurnDefinition{{Turns: 1}}, exchangeScript{"Hi", "Hello"})
	assert.Empty(t, stateResults(res))
}

func intPtrEngine(n int) *int { return &n }

func TestPersonaState_AlreadyStoppedPersonaSkipsTheNextTurn(t *testing.T) {
	persona := statefulPersona(arenaconfig.PersonaStopCondition{
		State: "patience", AtMost: intPtrEngine(1), Outcome: "abandoned",
	})
	turn := arenaconfig.TurnDefinition{Turns: 1, MaxTurns: 3}
	second := arenaconfig.TurnDefinition{Turns: 1, MaxTurns: 3, Objective: refundObjective("", nil)}
	res, seen := runState(t, persona, []arenaconfig.TurnDefinition{turn, second},
		exchangeScript{"Where is my order?", "Please verify."})

	assert.Len(t, seen, 1, "the abandoned persona does not speak again in the next turn")
	results := stateResults(res)
	require.Len(t, results, 2)
	assert.Equal(t, 0, results[1].Details["stopped_at_exchange"])
	assert.Equal(t, "persona customer state unchanged; abandoned before the turn began", results[1].Message)
	assert.Equal(t, "abandoned", objectiveResult(t, res).Details["persona_outcome"])
}

func TestPersonaState_EvalErrorFailsTheResult(t *testing.T) {
	tr := &stateTracker{
		persona: statefulPersona(), initial: map[string]interface{}{"patience": 2},
		state: map[string]interface{}{"patience": 2}, evalErr: objectiveErrNoRunner,
	}
	r := tr.result()
	assert.False(t, r.Passed)
	assert.Equal(t, objectiveErrNoRunner, r.Details["error"])
	assert.Contains(t, r.Message, "transitions could not be evaluated")
}

func TestPersonaFidelity_StateStringsAreGiven(t *testing.T) {
	persona := customerPersona(nil)
	persona.State = map[string]arenaconfig.PersonaStateVar{
		"ticket": {Type: arenaconfig.PersonaStateString, Initial: "TK-4821"},
	}
	persona.Transitions = []arenaconfig.PersonaTransition{{
		When: includes("escalate"), Set: map[string]interface{}{"ticket": "ESC-9001"},
	}}
	res := runFidelity(t, persona, arenaconfig.TurnDefinition{}, "My ticket is TK-4821, or ESC-9001.")
	assert.Equal(t, true, fidelityResult(t, res).Details["reliable"])
}
