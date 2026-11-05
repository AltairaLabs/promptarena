package turnexecutors

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/statestore"

	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/tools"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// TestSelfPlayExecutor_GenerateThenSend drafts the persona's message without
// sending it, then sends that same message without generating another.
func TestSelfPlayExecutor_GenerateThenSend(t *testing.T) {
	agent := new(MockProvider)
	agent.On("ID").Return("agent")
	agent.On("ShouldIncludeRawOutput").Return(false).Maybe()
	agent.On("SupportsStreaming").Return(false).Maybe()
	agent.On("Predict", mock.Anything, mock.Anything).Return(providers.PredictionResponse{Content: "noted"}, nil)

	store := statestore.NewArenaStateStore()
	gen := &selfPlayMockContentGenerator{message: types.Message{Role: "user", Content: "drafted"}}
	exec := NewSelfPlayExecutor(NewPipelineExecutor(tools.NewRegistry(), nil), &selfPlayMockProvider{contentGen: gen})
	req := TurnRequest{
		Provider:         agent,
		Scenario:         &arenaconfig.Scenario{ID: "s", TaskType: "test"},
		TaskType:         "test",
		StateStoreConfig: &StateStoreConfig{Store: store, UserID: "u"},
		ConversationID:   "candidate-conv",
		SelfPlayRole:     "attacker",
		SelfPlayPersona:  "p",
	}

	msg, err := exec.GenerateUserMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "drafted", msg.Content)
	_, err = store.Load(context.Background(), "candidate-conv")
	assert.Error(t, err, "drafting saves nothing")

	// The executor sends the message it is given, not a new draft.
	gen.message.Content = "a different draft"
	msg.Content = "edited before sending"
	req.SelfPlayMessage = &msg
	require.NoError(t, exec.ExecuteTurn(context.Background(), req))
	state, err := store.Load(context.Background(), "candidate-conv")
	require.NoError(t, err)
	var sent []string
	for i := range state.Messages {
		if state.Messages[i].Role == "user" {
			sent = append(sent, state.Messages[i].Content)
		}
	}
	assert.Equal(t, []string{"edited before sending"}, sent)
}

func TestSelfPlayExecutor_GenerateUserMessage_LoadError(t *testing.T) {
	store := new(MockStateStore)
	store.On("Load", mock.Anything, "c").Return(nil, errors.New("storage error"))
	exec := &SelfPlayExecutor{}
	_, err := exec.GenerateUserMessage(context.Background(), TurnRequest{
		ConversationID: "c", StateStoreConfig: &StateStoreConfig{Store: store},
	})
	assert.ErrorContains(t, err, "storage error")
}
