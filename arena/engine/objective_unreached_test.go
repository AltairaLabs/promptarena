package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"
	"github.com/AltairaLabs/promptarena/v2/arena/turnexecutors"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// unreachedScenario has a scripted opening turn and objectives on turns 1
// and 2.
func unreachedScenario() *arenaconfig.Scenario {
	return &arenaconfig.Scenario{ID: "unreached", TaskType: "assistant", Turns: []arenaconfig.TurnDefinition{
		{Role: "user", Content: "Start"},
		{Role: "attacker", Persona: "attacker", Turns: 1, Objective: refundObjective("", nil)},
		{Role: "attacker", Persona: "attacker", Turns: 1, Objective: refundObjective(arenaconfig.ObjectiveExpectNotAchieved, nil)},
	}}
}

func runFailingOpeningTurn(t *testing.T, failWith error) *ConversationResult {
	t.Helper()
	scripted := &MockTurnExecutor{executeFunc: func(context.Context, turnexecutors.TurnRequest) error { return failWith }}
	selfPlay := &MockTurnExecutor{executeFunc: func(context.Context, turnexecutors.TurnRequest) error {
		t.Fatal("self-play turn should not run")
		return nil
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
	executor.evalOrchestrator = NewEvalOrchestrator(nil, nil, false, nil, "")
	return executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region:           "default",
		Scenario:         unreachedScenario(),
		Provider:         &MockProvider{id: "test"},
		Config:           &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"},
		ConversationID:   "unreached-conv",
	})
}

func TestObjective_UnreachedWhenTheRunFailsFirst(t *testing.T) {
	res := runFailingOpeningTurn(t, errors.New("provider returned 400"))

	require.True(t, res.Failed)
	outcomes := ObjectiveOutcomes(res.ConversationAssertionResults)
	require.Len(t, outcomes, 2, "every objective the run never reached is reported")
	assert.Equal(t, 1, outcomes[0].TurnIndex)
	assert.Equal(t, 2, outcomes[1].TurnIndex)
	assert.Equal(t, arenaconfig.ObjectiveExpectNotAchieved, outcomes[1].Expect)
	for _, o := range outcomes {
		assert.False(t, o.Evaluated)
		assert.False(t, o.Achieved)
	}
	r := objectiveResult(t, res)
	assert.False(t, r.Passed)
	assert.Equal(t, "get a refund: could not be evaluated: the run ended before this turn", r.Message)
}

func TestObjective_TransientSkipReportsNoUnreachedObjectives(t *testing.T) {
	ce := &DefaultConversationExecutor{}
	req := ConversationRequest{Scenario: unreachedScenario(), objectives: &objectiveRecorder{}}
	transient := &providers.ProviderHTTPError{StatusCode: 503, Body: "unavailable", Provider: "test"}

	res := ce.handleTurnExecutionError(context.Background(), &req, transient, 0, req.Scenario.Turns[0])

	require.True(t, res.Skipped)
	assert.Empty(t, ObjectiveOutcomes(res.ConversationAssertionResults), "a skipped run is left out, not counted")
}

func TestObjectiveRecorder_RecordUnreachedKeepsRecordedOutcomes(t *testing.T) {
	sc := unreachedScenario()
	r := &objectiveRecorder{}
	r.record(objectiveOutcome{turnIndex: 1, cfg: sc.Turns[1].Objective, exchanges: 1})

	r.recordUnreached(sc)
	r.recordUnreached(nil)
	var nilRecorder *objectiveRecorder
	nilRecorder.recordUnreached(sc)

	results := r.results()
	require.Len(t, results, 2)
	assert.NotContains(t, results[0].Details, "error", "turn 1 was reached and keeps its outcome")
	assert.Equal(t, "the run ended before this turn", results[1].Details["error"])
	assert.Equal(t, true, results[1].Details["ended_early"])
	assert.False(t, endedEarly(&results[0]))
	assert.True(t, endedEarly(&results[1]))
}

func TestObjective_TurnThatErrorsIsNotEvaluated(t *testing.T) {
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "Start"}, {Role: "assistant", Content: "Hello"}})
	}}
	calls := 0
	selfPlay := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		calls++
		if calls == 2 {
			return errors.New("provider returned 400")
		}
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "ask"}, {Role: "assistant", Content: "no"}})
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
	executor.evalOrchestrator = NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")
	sc := unreachedScenario()
	sc.Turns[1].Turns = 3
	sc.Turns[1].Objective = refundObjective(arenaconfig.ObjectiveExpectNotAchieved, nil)

	res := executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region: "default", Scenario: sc, Provider: &MockProvider{id: "test"}, Config: &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"}, ConversationID: "interrupted",
	})

	require.True(t, res.Failed)
	outcomes := ObjectiveOutcomes(res.ConversationAssertionResults)
	require.Len(t, outcomes, 2)
	assert.False(t, outcomes[0].Evaluated, "an objective cut short by the error is not a fair attempt")
	r := objectiveResult(t, res)
	assert.False(t, r.Passed, "an expect: not_achieved objective does not pass because the turn errored")
	assert.Equal(t, true, r.Details["ended_early"])
	assert.Equal(t, "get a refund: could not be evaluated: the turn ended with an error before the objective was decided",
		r.Message)
}

func TestObjective_NonStreamingTransientErrorStillReportsUnreached(t *testing.T) {
	res := runFailingOpeningTurn(t, &providers.ProviderHTTPError{StatusCode: 503, Body: "unavailable", Provider: "test"})

	require.True(t, res.Failed, "the non-streaming path fails the run on any turn error")
	assert.Len(t, ObjectiveOutcomes(res.ConversationAssertionResults), 2)
}

func TestLoadTrialResults_EndedEarlyObjectiveIsNotAnAssertionResult(t *testing.T) {
	store := statestore.NewArenaStateStore()
	ctx := context.Background()
	reached := (&objectiveOutcome{turnIndex: 1, cfg: refundObjective("", nil), exchanges: 1, achievedAt: 1}).result()
	notReached := (&objectiveOutcome{turnIndex: 1, cfg: refundObjective("", nil), evalErr: objectiveErrNotReached,
		endedEarly: true}).result()
	setupTrialRun(t, store, ctx, "r1", true, []assertions.ConversationValidationResult{reached})
	setupTrialRun(t, store, ctx, "r2", false, []assertions.ConversationValidationResult{notReached})

	trials := loadTrialResults(store, []string{"r1", "r2"}, nil)

	key := conversationAssertionKey(0, objectiveResultType)
	require.Len(t, trials, 2)
	assert.Contains(t, trials[0].assertionResults, key)
	assert.NotContains(t, trials[1].assertionResults, key, "counted by the missing-result rule instead")
	require.Len(t, trials[1].objectives, 1, "still reported to the objectives stats")
	assert.False(t, trials[1].objectives[0].Evaluated)
}
