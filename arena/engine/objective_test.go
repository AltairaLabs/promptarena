package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	asrt "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"
	"github.com/AltairaLabs/promptarena/v2/arena/turnexecutors"

	"github.com/AltairaLabs/PromptKit/runtime/v2/evals"
	"github.com/AltairaLabs/PromptKit/runtime/v2/events"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// objectiveHarness runs a scenario whose self-play executor replies with
// replies[i] on exchange i+1 (and "nothing yet" after them), so a test can
// choose the exchange at which the objective's success check first passes.
type objectiveHarness struct {
	replies   []string
	completes int // exchange that returns ErrConversationComplete; 0 = never
	calls     int
}

func (h *objectiveHarness) run(t *testing.T, turn arenaconfig.TurnDefinition, withOrchestrator bool) *ConversationResult {
	t.Helper()
	scripted := &MockTurnExecutor{
		executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
			return saveMessagesToStateStore(ctx, req, []types.Message{
				{Role: "user", Content: req.ScriptedContent},
				{Role: "assistant", Content: "Hello"},
			})
		},
	}
	selfPlay := &MockTurnExecutor{
		executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
			h.calls++
			reply := "nothing yet"
			if h.calls <= len(h.replies) {
				reply = h.replies[h.calls-1]
			}
			if err := saveMessagesToStateStore(ctx, req, []types.Message{
				{Role: "user", Content: fmt.Sprintf("ask %d", h.calls)},
				{Role: "assistant", Content: reply},
			}); err != nil {
				return err
			}
			if h.completes == h.calls {
				return selfplay.ErrConversationComplete
			}
			return nil
		},
	}

	var orch *EvalOrchestrator
	if withOrchestrator {
		orch = NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")
	}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
	executor.evalOrchestrator = orch

	turn.Role = "attacker"
	turn.Persona = "attacker"
	req := ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{
			ID:       "objective",
			TaskType: "assistant",
			Turns:    []arenaconfig.TurnDefinition{{Role: "user", Content: "Start"}, turn},
		},
		Provider:         &MockProvider{id: "test"},
		Config:           &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"},
		ConversationID:   "objective-conv",
	}
	return executor.ExecuteConversation(context.Background(), req)
}

func refundObjective(expect string, stop *bool) *arenaconfig.ObjectiveConfig {
	return &arenaconfig.ObjectiveConfig{
		Description:   "get a refund",
		Expect:        expect,
		StopOnSuccess: stop,
		Success: []arenaconfig.AssertionConfig{{
			Type:   "content_includes_any",
			Params: map[string]interface{}{"patterns": []interface{}{"REFUND ISSUED"}},
		}},
	}
}

func objectiveResult(t *testing.T, res *ConversationResult) asrt.ConversationValidationResult {
	t.Helper()
	for _, r := range res.ConversationAssertionResults {
		if r.Type == objectiveResultType {
			return r
		}
	}
	t.Fatalf("no objective result in %+v", res.ConversationAssertionResults)
	return asrt.ConversationValidationResult{}
}

func TestObjective_StopsAtFirstSuccess(t *testing.T) {
	h := &objectiveHarness{replies: []string{"no", "no", "REFUND ISSUED"}}
	res := h.run(t, arenaconfig.TurnDefinition{Turns: 5, MaxTurns: 8, Objective: refundObjective("", nil)}, true)

	require.False(t, res.Failed, res.Error)
	assert.Equal(t, 3, h.calls, "stops at the exchange that achieves the objective, even below the minimum turns")
	r := objectiveResult(t, res)
	assert.True(t, r.Passed)
	assert.Equal(t, true, r.Details["achieved"])
	assert.Equal(t, 3, r.Details["achieved_at_exchange"])
	assert.Equal(t, 3, r.Details["exchanges"])
	assert.Equal(t, "get a refund: achieved at exchange 3 of 3", r.Message)
}

func TestObjective_NotAchievedRunsToMaxAndFails(t *testing.T) {
	h := &objectiveHarness{}
	res := h.run(t, arenaconfig.TurnDefinition{Turns: 4, Objective: refundObjective("", nil)}, true)

	assert.Equal(t, 4, h.calls)
	r := objectiveResult(t, res)
	assert.False(t, r.Passed, "expected achieved, was not")
	assert.Equal(t, false, r.Details["achieved"])
	assert.NotContains(t, r.Details, "achieved_at_exchange")
	assert.Equal(t, "get a refund: not achieved in 4 exchanges", r.Message)
}

func TestObjective_ExpectNotAchieved(t *testing.T) {
	t.Run("passes when never achieved", func(t *testing.T) {
		h := &objectiveHarness{}
		res := h.run(t, arenaconfig.TurnDefinition{Turns: 3, Objective: refundObjective(arenaconfig.ObjectiveExpectNotAchieved, nil)}, true)
		assert.True(t, objectiveResult(t, res).Passed)
	})
	t.Run("fails when achieved", func(t *testing.T) {
		h := &objectiveHarness{replies: []string{"REFUND ISSUED"}}
		res := h.run(t, arenaconfig.TurnDefinition{Turns: 3, Objective: refundObjective(arenaconfig.ObjectiveExpectNotAchieved, nil)}, true)
		assert.Equal(t, 1, h.calls)
		assert.False(t, objectiveResult(t, res).Passed)
	})
}

func TestObjective_StopOnSuccessFalseKeepsGoing(t *testing.T) {
	no := false
	h := &objectiveHarness{replies: []string{"REFUND ISSUED"}}
	res := h.run(t, arenaconfig.TurnDefinition{Turns: 3, Objective: refundObjective("", &no)}, true)

	assert.Equal(t, 3, h.calls)
	r := objectiveResult(t, res)
	assert.True(t, r.Passed)
	assert.Equal(t, 1, r.Details["achieved_at_exchange"])
	assert.Equal(t, 3, r.Details["exchanges"])
}

func TestObjective_CheckedOnNaturalCompletion(t *testing.T) {
	h := &objectiveHarness{replies: []string{"no", "REFUND ISSUED"}, completes: 2}
	res := h.run(t, arenaconfig.TurnDefinition{Turns: 1, MaxTurns: 5, Objective: refundObjective("", nil)}, true)

	assert.Equal(t, 2, h.calls)
	assert.True(t, objectiveResult(t, res).Passed)
}

func TestObjective_NoOrchestratorFails(t *testing.T) {
	h := &objectiveHarness{replies: []string{"REFUND ISSUED"}}
	res := h.run(t, arenaconfig.TurnDefinition{Turns: 2, Objective: refundObjective("", nil)}, false)

	assert.Equal(t, 2, h.calls)
	r := objectiveResult(t, res)
	assert.False(t, r.Passed)
	assert.Equal(t, "eval runner not configured", r.Details["error"])
}

func TestObjective_TurnWithoutObjectiveAddsNoResult(t *testing.T) {
	h := &objectiveHarness{}
	res := h.run(t, arenaconfig.TurnDefinition{Turns: 2}, true)
	for _, r := range res.ConversationAssertionResults {
		assert.NotEqual(t, objectiveResultType, r.Type)
	}
}

func TestObjective_StreamingScenario(t *testing.T) {
	h := &objectiveHarness{replies: []string{"no", "REFUND ISSUED"}}
	// Reuse the harness but force the streaming conversation path.
	scripted := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "Start"}, {Role: "assistant", Content: "Hello"}})
	}}
	selfPlay := &MockTurnExecutor{executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
		h.calls++
		reply := "nothing yet"
		if h.calls <= len(h.replies) {
			reply = h.replies[h.calls-1]
		}
		return saveMessagesToStateStore(ctx, req, []types.Message{{Role: "user", Content: "ask"}, {Role: "assistant", Content: reply}})
	}}
	executor := NewDefaultConversationExecutor(scripted, selfPlay, createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
	executor.evalOrchestrator = NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "")
	off := false
	req := ConversationRequest{
		Region: "default",
		Scenario: &arenaconfig.Scenario{
			ID: "objective-stream", TaskType: "assistant", Streaming: true,
			Turns: []arenaconfig.TurnDefinition{
				{Role: "user", Content: "Start", Streaming: &off},
				{Role: "attacker", Persona: "attacker", Turns: 4, Streaming: &off, Objective: refundObjective("", nil)},
			},
		},
		Provider:         &MockProvider{id: "test"},
		Config:           &arenaconfig.Config{},
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "u"},
		ConversationID:   "objective-stream",
	}
	res := executor.ExecuteConversation(context.Background(), req)

	assert.Equal(t, 2, h.calls)
	assert.True(t, objectiveResult(t, res).Passed)
}

func TestAllPassed(t *testing.T) {
	pass := asrt.ConversationValidationResult{Passed: true}
	fail := asrt.ConversationValidationResult{Passed: false}
	skipped := asrt.ConversationValidationResult{Passed: true, Details: map[string]interface{}{"skipped": true, "skip_reason": "when"}}

	assert.True(t, allPassed([]asrt.ConversationValidationResult{pass, pass}, 2))
	assert.False(t, allPassed([]asrt.ConversationValidationResult{pass, fail}, 2))
	assert.False(t, allPassed([]asrt.ConversationValidationResult{pass, skipped}, 2), "a skipped check was not made")
	assert.False(t, allPassed([]asrt.ConversationValidationResult{pass}, 2), "a missing result was not made")
}

func TestObjective_ScriptedRoleIsRejected(t *testing.T) {
	executor := NewDefaultConversationExecutor(&MockTurnExecutor{}, &MockTurnExecutor{}, createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
	req := ConversationRequest{
		Scenario: &arenaconfig.Scenario{ID: "s", Turns: []arenaconfig.TurnDefinition{
			{Role: "user", Persona: "attacker", Content: "hi", Objective: refundObjective("", nil)},
		}},
		Config: &arenaconfig.Config{},
	}
	res := executor.ExecuteConversation(context.Background(), req)
	assert.True(t, res.Failed)
	assert.Contains(t, res.Error, `turns[0].objective: role "user" is not a self-play role`)
}

func TestObjective_StreamingConversationPathRejects(t *testing.T) {
	executor := NewDefaultConversationExecutor(&MockTurnExecutor{}, &MockTurnExecutor{}, createTestSelfPlayRegistry(t), createTestPromptRegistry(t), nil)
	req := ConversationRequest{
		Scenario: &arenaconfig.Scenario{ID: "s", Turns: []arenaconfig.TurnDefinition{
			{Role: "attacker", Persona: "attacker", Objective: refundObjective("", nil)},
		}},
		Config: &arenaconfig.Config{},
	}
	stream, err := executor.ExecuteConversationStream(context.Background(), req)
	require.NoError(t, err)
	var chunks []ConversationStreamChunk
	for c := range stream {
		chunks = append(chunks, c)
	}
	require.Len(t, chunks, 1)
	require.Error(t, chunks[0].Error)
	assert.Contains(t, chunks[0].Error.Error(), "not supported on the streaming conversation path")
	assert.True(t, chunks[0].Result.Failed)
}

func TestEvalOrchestrator_Quiet(t *testing.T) {
	assert.Nil(t, (*EvalOrchestrator)(nil).Quiet())

	orch := NewEvalOrchestrator(evals.NewEvalTypeRegistry(), nil, false, nil, "task")
	orch.SetMetadata(map[string]any{"k": "v"})
	orch.SetEventBus(events.NewEventBus())

	quiet := orch.Quiet()
	assert.Nil(t, quiet.eventBus)
	assert.NotNil(t, orch.eventBus, "original keeps its event bus")
	assert.NotSame(t, orch.runner, quiet.runner)
	assert.Equal(t, "v", quiet.metadata["k"])
	assert.Equal(t, "task", quiet.taskType)
}
