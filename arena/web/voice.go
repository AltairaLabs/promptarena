package web

import (
	"fmt"

	pkgconfig "github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/arena/engine"
)

// duplexVoiceTimeout is the idle ceiling for a web voice run. Matches the TUI's
// interactive voice scenario (arena/tui/app/chatpage_voice.go).
const duplexVoiceTimeout = "30m"

// interactiveVoiceScenarioID is the synthetic scenario ID used for web voice
// runs, mirroring the TUI's interactive-voice scenario.
const interactiveVoiceScenarioID = "interactive-voice"

// buildVoiceRequest assembles the duplex ConversationRequest for a web voice
// run, reusing the provider already resolved by the text InteractiveSession.
// It mirrors the TUI's request build (chatpage_voice.go runVoice, steps 3-6).
//
// sttID == "" selects the realtime/ASM path (VoiceSTT nil, provider-native
// turn detection). A non-empty sttID must resolve via cfg.LoadedSTTProviders
// and selects the composed VAD path (client-side turn detection + STT).
func buildVoiceRequest(
	eng *engine.Engine,
	sess *engine.InteractiveSession,
	sttID, outputVoice string,
	bargeIn bool,
) (*engine.ConversationRequest, error) {
	cfg := eng.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("engine has no config")
	}

	var voiceSTT *pkgconfig.Provider
	turnMode := arenaconfig.TurnDetectionModeASM
	if sttID != "" {
		prov, ok := cfg.LoadedSTTProviders[sttID]
		if !ok {
			return nil, fmt.Errorf("stt provider not found: %s", sttID)
		}
		voiceSTT = prov
		turnMode = arenaconfig.TurnDetectionModeVAD
	}

	scenario := &arenaconfig.Scenario{
		ID:       interactiveVoiceScenarioID,
		TaskType: sess.TaskType(),
		Duplex: &arenaconfig.DuplexConfig{
			Timeout:       duplexVoiceTimeout,
			TurnDetection: &arenaconfig.TurnDetectionConfig{Mode: turnMode},
		},
	}

	conversationID := sess.ConversationID()
	return &engine.ConversationRequest{
		Provider:       sess.Provider(),
		Scenario:       scenario,
		Config:         cfg,
		RunID:          conversationID,
		ConversationID: conversationID,
		StateStoreConfig: &engine.StateStoreConfig{
			Store: eng.GetStateStore(),
		},
		VoiceSTT:         voiceSTT,
		VoiceOutputVoice: outputVoice,
		VoiceBargeIn:     bargeIn,
	}, nil
}
