package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

// Duplex runs used to drop defaults.temperature and defaults.max_tokens: the
// realtime session config carried no sampling at all.
func TestBuildBaseSessionConfig_CarriesConfigSampling(t *testing.T) {
	executor := NewDuplexConversationExecutor(nil, nil, nil, nil, nil)
	req := &ConversationRequest{
		Scenario: &arenaconfig.Scenario{ID: "s"},
		Config:   &arenaconfig.Config{Defaults: arenaconfig.Defaults{Temperature: 0.3, MaxTokens: 400}},
	}

	cfg := executor.buildBaseSessionConfig(req, 16000)
	require.NotNil(t, cfg.Sampling)
	assert.InDelta(t, 0.3, cfg.Sampling.Temperature, 1e-6)
	assert.Equal(t, 400, cfg.Sampling.MaxTokens)
}

func TestDuplexSampling(t *testing.T) {
	temp := 0.0
	maxTok := 50
	cfg := &arenaconfig.Config{Defaults: arenaconfig.Defaults{Temperature: 0.3, MaxTokens: 400}}

	t.Run("request overrides win, an explicit 0 temperature counts", func(t *testing.T) {
		s := duplexSampling(&ConversationRequest{Config: cfg, Temperature: &temp, MaxTokens: &maxTok})
		require.NotNil(t, s)
		assert.True(t, s.TemperatureSet)
		assert.InDelta(t, 0, s.Temperature, 1e-6)
		assert.Equal(t, 50, s.MaxTokens)
	})
	t.Run("nothing set sends none", func(t *testing.T) {
		assert.Nil(t, duplexSampling(&ConversationRequest{Config: &arenaconfig.Config{}}))
		assert.Nil(t, duplexSampling(&ConversationRequest{}))
	})
	t.Run("max_tokens alone", func(t *testing.T) {
		s := duplexSampling(&ConversationRequest{Config: &arenaconfig.Config{
			Defaults: arenaconfig.Defaults{MaxTokens: 10},
		}})
		require.NotNil(t, s)
		assert.False(t, s.TemperatureSet)
		assert.Equal(t, 10, s.MaxTokens)
	})
}
