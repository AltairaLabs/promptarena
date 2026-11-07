package turnexecutors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline"
)

func TestSelfPlayExecutor_GenerateUserMessage_PassesTurnOverrides(t *testing.T) {
	temp := float32(1.3)
	seed := 11

	gen := new(MockContentGen)
	gen.On("NextUserTurn", mock.Anything, mock.Anything, "s1", mock.MatchedBy(func(o *selfplay.GeneratorOptions) bool {
		return o != nil && o.Temperature != nil && *o.Temperature == temp && o.Seed != nil && *o.Seed == seed &&
			o.Variables != nil && (*o.Variables)["exchange"] == "3"
	})).Return(&pipeline.ExecutionResult{Response: &pipeline.Response{Content: "hi"}}, nil)

	content := new(MockSelfPlayContentProvider)
	content.On("GetContentGenerator", "attacker", "p").Return(gen, nil)

	executor := &SelfPlayExecutor{contentProvider: content}
	req := &TurnRequest{
		Scenario:         &arenaconfig.Scenario{ID: "s1"},
		SelfPlayRole:     "attacker",
		SelfPlayPersona:  "p",
		SelfPlayUserTemp: &temp,
		SelfPlaySeed:     &seed,
		SelfPlayVars:     map[string]string{"exchange": "3"},
	}

	msg, _, err := executor.generateUserMessage(context.Background(), req, nil)
	require.NoError(t, err)
	assert.Equal(t, "hi", msg.Content)
	gen.AssertExpectations(t)
}
