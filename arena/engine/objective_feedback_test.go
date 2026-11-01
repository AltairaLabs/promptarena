package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"
	"github.com/AltairaLabs/promptarena/v2/arena/turnexecutors"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func TestChecksFeedback(t *testing.T) {
	checks := []asrt.ConversationValidationResult{
		{Type: "content_includes_any", Passed: false, Message: "no match"},
		{Type: "tools_called", Passed: true},
		{Type: "llm_judge", Passed: true, Details: map[string]interface{}{"skipped": true}},
	}
	assert.Equal(t,
		"- content_includes_any: failed — no match\n- tools_called: passed\n- llm_judge: skipped",
		checksFeedback(checks))
	assert.Equal(t, "", checksFeedback(nil))
}

func TestPersonaVariables_NoObjective(t *testing.T) {
	var tracker *objectiveTracker
	assert.Equal(t, map[string]string{selfplay.VarExchange: "4"}, tracker.personaVariables(4))
}

// TestObjective_FeedbackReachesEachExchange runs a self-play turn with an
// objective and records the variables each exchange is generated with.
func TestObjective_FeedbackReachesEachExchange(t *testing.T) {
	var seen []map[string]string
	replies := []string{"no", "REFUND ISSUED"}
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "Start"}, {Role: "assistant", Content: "Hello"}})
	}}
	selfPlay := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		seen = append(seen, req.SelfPlayVars)
		return saveMessagesToStateStore(ctx, req, []types.Message{
			{Role: "user", Content: "ask"}, {Role: "assistant", Content: replies[len(seen)-1]},
		})
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
	executor.evalOrchestrator = NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")

	obj := refundObjective("", nil)
	obj.Success[0].Message = "agent issued the refund"
	res := executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{ID: "fb", TaskType: "assistant", Turns: []arenaconfig.TurnDefinition{
			{Role: "user", Content: "Start"},
			{Role: "attacker", Persona: "attacker", Turns: 5, Objective: obj},
		}},
		Provider:         &MockProvider{id: "test"},
		Config:           &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"},
		ConversationID:   "fb",
	})
	require.False(t, res.Failed, res.Error)
	require.Len(t, seen, 2)

	first := seen[0]
	assert.Equal(t, "1", first[selfplay.VarExchange])
	assert.Equal(t, "get a refund", first[selfplay.VarObjective])
	assert.Equal(t, "not achieved", first[selfplay.VarObjectiveStatus])
	assert.Equal(t, "", first[selfplay.VarObjectiveFeedback], "nothing has been checked before the first exchange")

	second := seen[1]
	assert.Equal(t, "2", second[selfplay.VarExchange])
	assert.Equal(t, "not achieved", second[selfplay.VarObjectiveStatus])
	assert.Equal(t, "- content_includes_any: failed — agent issued the refund", second[selfplay.VarObjectiveFeedback])
}

func TestSelfPlayTurn_WithoutObjectiveGetsExchangeNumber(t *testing.T) {
	var seen []map[string]string
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "Start"}, {Role: "assistant", Content: "Hello"}})
	}}
	selfPlay := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		seen = append(seen, req.SelfPlayVars)
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "ask"}, {Role: "assistant", Content: "ok"}})
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
	executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{ID: "plain", TaskType: "assistant", Turns: []arenaconfig.TurnDefinition{
			{Role: "user", Content: "Start"},
			{Role: "attacker", Persona: "attacker", Turns: 2},
		}},
		Provider:         &MockProvider{id: "test"},
		Config:           &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"},
		ConversationID:   "plain",
	})
	require.Len(t, seen, 2)
	assert.Equal(t, map[string]string{selfplay.VarExchange: "2"}, seen[1])
}

// TestObjective_FeedbackRefreshedAfterAchieved keeps a turn going past the
// objective and checks the persona is shown the latest checks, not the ones
// from the exchange that achieved it.
func TestObjective_FeedbackRefreshedAfterAchieved(t *testing.T) {
	var seen []map[string]string
	replies := []string{"REFUND ISSUED", "no", "no"}
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "Start"}, {Role: "assistant", Content: "Hello"}})
	}}
	selfPlay := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		seen = append(seen, req.SelfPlayVars)
		return saveMessagesToStateStore(ctx, req, []types.Message{
			{Role: "user", Content: "ask"}, {Role: "assistant", Content: replies[len(seen)-1]},
		})
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
	executor.evalOrchestrator = NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")

	keepGoing := false
	// The check looks at the latest assistant reply only, so it passes on
	// exchange 1 and fails afterwards.
	obj := &arenaconfig.ObjectiveConfig{
		Description:   "get a refund",
		StopOnSuccess: &keepGoing,
		Success: []arenaconfig.AssertionConfig{{
			Type:   "content_matches",
			Params: map[string]interface{}{"pattern": "REFUND ISSUED"},
		}},
	}
	res := executor.ExecuteConversation(context.Background(), ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{ID: "fb2", TaskType: "assistant", Turns: []arenaconfig.TurnDefinition{
			{Role: "user", Content: "Start"},
			{Role: "attacker", Persona: "attacker", Turns: 3, Objective: obj},
		}},
		Provider:         &MockProvider{id: "test"},
		Config:           &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"},
		ConversationID:   "fb2",
	})
	require.False(t, res.Failed, res.Error)
	require.Len(t, seen, 3)

	assert.Equal(t, "achieved", seen[1][selfplay.VarObjectiveStatus])
	assert.Contains(t, seen[1][selfplay.VarObjectiveFeedback], ": passed")
	assert.Equal(t, "achieved", seen[2][selfplay.VarObjectiveStatus], "achieved stays achieved")
	assert.Contains(t, seen[2][selfplay.VarObjectiveFeedback], ": failed", "feedback describes the latest exchange")
}
