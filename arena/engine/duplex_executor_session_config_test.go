package engine

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/tools"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

func newStreamingCfg() *providers.StreamingInputConfig {
	return &providers.StreamingInputConfig{Metadata: map[string]interface{}{}}
}

func TestApplyScenarioVADConfig(t *testing.T) {
	de := &DuplexConversationExecutor{}

	t.Run("no-op without duplex VAD", func(t *testing.T) {
		cfg := newStreamingCfg()
		de.applyScenarioVADConfig(cfg, &ConversationRequest{Scenario: &arenaconfig.Scenario{}})
		assert.NotContains(t, cfg.Metadata, "vad_config")
	})

	t.Run("no-op when all thresholds are zero", func(t *testing.T) {
		cfg := newStreamingCfg()
		req := &ConversationRequest{Scenario: &arenaconfig.Scenario{
			Duplex: &arenaconfig.DuplexConfig{
				TurnDetection: &arenaconfig.TurnDetectionConfig{VAD: &arenaconfig.VADConfig{}},
			},
		}}
		de.applyScenarioVADConfig(cfg, req)
		assert.NotContains(t, cfg.Metadata, "vad_config")
	})

	t.Run("populates vad_config from set thresholds", func(t *testing.T) {
		cfg := newStreamingCfg()
		req := &ConversationRequest{Scenario: &arenaconfig.Scenario{
			Duplex: &arenaconfig.DuplexConfig{
				TurnDetection: &arenaconfig.TurnDetectionConfig{VAD: &arenaconfig.VADConfig{
					SilenceThresholdMs: 300,
					MinSpeechMs:        800,
					MaxTurnDurationS:   30,
				}},
			},
		}}
		de.applyScenarioVADConfig(cfg, req)
		vc, ok := cfg.Metadata["vad_config"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, 300, vc["silence_threshold_ms"])
		assert.Equal(t, 800, vc["min_speech_ms"])
		assert.Equal(t, 30, vc["max_turn_duration_s"])
	})
}

func TestApplyToolsConfig(t *testing.T) {
	t.Run("no-op with nil registry", func(t *testing.T) {
		de := &DuplexConversationExecutor{}
		cfg := newStreamingCfg()
		de.applyToolsConfig(cfg)
		assert.Nil(t, cfg.Tools)
	})

	t.Run("no-op with empty registry", func(t *testing.T) {
		de := &DuplexConversationExecutor{toolRegistry: tools.NewRegistry()}
		cfg := newStreamingCfg()
		de.applyToolsConfig(cfg)
		assert.Nil(t, cfg.Tools)
	})

	t.Run("populates tools from registry", func(t *testing.T) {
		reg := tools.NewRegistry()
		require.NoError(t, reg.Register(&tools.ToolDescriptor{
			Name:        "search",
			Description: "search things",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}))
		de := &DuplexConversationExecutor{toolRegistry: reg}
		cfg := newStreamingCfg()
		de.applyToolsConfig(cfg)
		require.Len(t, cfg.Tools, 1)
		assert.Equal(t, "search", cfg.Tools[0].Name)
	})
}
