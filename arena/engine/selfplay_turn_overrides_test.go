package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"

	"github.com/AltairaLabs/PromptKit/runtime/v2/pipeline"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

func TestResolveTurnTemperature_AssistantTemp(t *testing.T) {
	cfg := &arenaconfig.Config{Defaults: arenaconfig.Defaults{Temperature: 0.3}}

	t.Run("assistant_temp beats config default", func(t *testing.T) {
		req := ConversationRequest{Config: cfg}
		turn := &arenaconfig.TurnDefinition{AssistantTemp: 0.9}
		assert.InDelta(t, 0.9, resolveTurnTemperature(req, turn), 1e-6)
	})

	t.Run("request override beats assistant_temp", func(t *testing.T) {
		override := 0.1
		req := ConversationRequest{Config: cfg, Temperature: &override}
		turn := &arenaconfig.TurnDefinition{AssistantTemp: 0.9}
		assert.InDelta(t, 0.1, resolveTurnTemperature(req, turn), 1e-6)
	})

	t.Run("unset assistant_temp falls back", func(t *testing.T) {
		req := ConversationRequest{Config: cfg}
		assert.InDelta(t, 0.3, resolveTurnTemperature(req, &arenaconfig.TurnDefinition{}), 1e-6)
	})
}

func TestBuildTurnRequest_SelfPlayOverrides(t *testing.T) {
	ce := &DefaultConversationExecutor{}
	req := ConversationRequest{
		Config:     &arenaconfig.Config{Defaults: arenaconfig.Defaults{Temperature: 0.3}},
		Scenario:   &arenaconfig.Scenario{TaskType: "task"},
		TrialIndex: 3,
	}

	t.Run("set", func(t *testing.T) {
		tr := ce.buildTurnRequest(req, arenaconfig.TurnDefinition{
			Role: "attacker", Persona: "p", UserTemp: 1.2, AssistantTemp: 0.5, Seed: 9,
		})
		require.NotNil(t, tr.SelfPlayUserTemp)
		assert.InDelta(t, 1.2, *tr.SelfPlayUserTemp, 1e-6)
		require.NotNil(t, tr.SelfPlaySeed)
		assert.Equal(t, 9, *tr.SelfPlaySeed)
		assert.InDelta(t, 0.5, tr.Temperature, 1e-6)
		assert.Equal(t, 3, tr.SelfPlayTrial, "the trial seeds the random converters")
	})

	t.Run("unset", func(t *testing.T) {
		tr := ce.buildTurnRequest(req, arenaconfig.TurnDefinition{Role: "attacker", Persona: "p"})
		assert.Nil(t, tr.SelfPlayUserTemp)
		assert.Nil(t, tr.SelfPlaySeed)
		assert.InDelta(t, 0.3, tr.Temperature, 1e-6)
	})
}

// optsCapturingGenerator records the options it was called with.
type optsCapturingGenerator struct {
	opts *selfplay.GeneratorOptions
}

func (g *optsCapturingGenerator) NextUserTurn(
	_ context.Context, _ []types.Message, _ string, opts *selfplay.GeneratorOptions,
) (*pipeline.ExecutionResult, error) {
	g.opts = opts
	return &pipeline.ExecutionResult{Response: &pipeline.Response{Content: "hi"}}, nil
}

func TestGenerateSelfplayText_PassesTurnOverrides(t *testing.T) {
	gen := &optsCapturingGenerator{}
	turn := &arenaconfig.TurnDefinition{UserTemp: 0.95, Seed: 3}

	_, _, err := generateSelfplayText(context.Background(), gen, nil, "s1", 2, 0, turn)
	require.NoError(t, err)

	require.NotNil(t, gen.opts)
	assert.Equal(t, 2, gen.opts.SelfplayTurnIndex)
	require.NotNil(t, gen.opts.Temperature)
	assert.InDelta(t, 0.95, *gen.opts.Temperature, 1e-6)
	require.NotNil(t, gen.opts.Seed)
	assert.Equal(t, 3, *gen.opts.Seed)
	assert.Equal(t, "2", gen.opts.Variables["exchange"], "duplex turns get the exchange number too")
}
