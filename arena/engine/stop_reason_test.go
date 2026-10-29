package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	arenaassertions "github.com/AltairaLabs/promptarena/v2/arena/assertions"
	"github.com/AltairaLabs/promptarena/v2/arena/turnexecutors"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline/stage"
	"github.com/AltairaLabs/PromptKit/runtime/v2/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func TestStopReasonFor(t *testing.T) {
	wrapped := fmt.Errorf("pipeline execution failed: %w", &stage.MaxRoundsError{Rounds: 2})
	assert.Equal(t, StopReasonMaxRounds, stopReasonFor(wrapped))
	assert.Empty(t, stopReasonFor(errors.New("validation failed")))
	assert.Empty(t, stopReasonFor(nil))
}

func TestConversationExecutor_HandleTurnExecutionError_MaxRoundsSetsStopReason(t *testing.T) {
	ce := &DefaultConversationExecutor{}
	req := ConversationRequest{Scenario: &arenaconfig.Scenario{}}
	err := fmt.Errorf("pipeline execution failed: %w", &stage.MaxRoundsError{Rounds: 3})

	result := ce.handleTurnExecutionError(context.Background(), &req, err, 0, arenaconfig.TurnDefinition{Role: "user"})

	require.True(t, result.Failed)
	assert.Equal(t, StopReasonMaxRounds, result.StopReason)
}

// TestExecuteConversation_MaxRoundsStillEvaluatesConversationAssertions guards
// #332: a run whose tool loop hits max_rounds must still be scored. The
// conversation assertions run on the partial transcript and the result says
// why the run stopped.
func TestExecuteConversation_MaxRoundsStillEvaluatesConversationAssertions(t *testing.T) {
	// The tool loop persists each round before the cap ends the turn, so the
	// store holds a partial transcript when the error comes back.
	scriptedExecutor := &MockTurnExecutor{
		executeFunc: func(ctx context.Context, req turnexecutors.TurnRequest) error {
			appender := req.StateStoreConfig.Store.(statestore.MessageAppender)
			partial := []types.Message{
				{Role: "user", Content: "Build it"},
				{Role: "assistant", Content: "Working on it"},
			}
			if err := appender.AppendMessages(ctx, req.ConversationID, partial); err != nil {
				return err
			}
			return fmt.Errorf("pipeline execution failed: %w", &stage.MaxRoundsError{Rounds: 2})
		},
	}
	executor := NewDefaultConversationExecutor(scriptedExecutor, nil, nil, createTestPromptRegistry(t), nil)

	req := ConversationRequest{
		Region:           "us",
		ConversationID:   "capped-run",
		StateStoreConfig: &StateStoreConfig{Store: createTestStateStore(), UserID: "test-user"},
		Scenario: &arenaconfig.Scenario{
			ID:       "capped",
			TaskType: "support",
			Turns:    []arenaconfig.TurnDefinition{{Role: "user", Content: "Build it"}},
			ConversationAssertions: []arenaassertions.AssertionConfig{
				{Type: "content_not_includes", Params: map[string]interface{}{"patterns": []string{"forbidden"}}},
			},
		},
		Provider: &MockProvider{id: "test"},
		Config:   &arenaconfig.Config{},
	}

	result := executor.ExecuteConversation(context.Background(), req)

	require.True(t, result.Failed)
	assert.Equal(t, StopReasonMaxRounds, result.StopReason)
	assert.Len(t, result.ConversationAssertionResults, 1, "conversation assertions must still be evaluated")
}

func TestExecuteConversation_OtherErrorLeavesStopReasonEmpty(t *testing.T) {
	scriptedExecutor := &MockTurnExecutor{
		executeFunc: func(context.Context, turnexecutors.TurnRequest) error {
			return errors.New("executor failed")
		},
	}
	executor := NewDefaultConversationExecutor(scriptedExecutor, nil, nil, createTestPromptRegistry(t), nil)

	result := executor.ExecuteConversation(context.Background(), ConversationRequest{
		Scenario: &arenaconfig.Scenario{
			ID: "broken", TaskType: "support",
			Turns: []arenaconfig.TurnDefinition{{Role: "user", Content: "Hello"}},
		},
		Provider: &MockProvider{id: "test"},
		Config:   &arenaconfig.Config{},
	})

	require.True(t, result.Failed)
	assert.Empty(t, result.StopReason)
}
