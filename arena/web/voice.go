package web

import (
	"fmt"
	"net/http"

	pkgconfig "github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/arena/engine"
	"github.com/gorilla/websocket"
)

// voiceUpgrader upgrades /api/interactive/voice connections. CheckOrigin
// returns true unconditionally: same-origin in prod, and the Vite dev proxy
// forwards Origin so cross-origin dev requests need to pass too.
var voiceUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

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

// handleInteractiveVoice upgrades the connection to a WebSocket and drives a
// live duplex voice conversation over it for an existing interactive session.
// Query params: session (required), stt (optional), voice (optional
// output-voice), bargein (optional "1" to enable barge-in).
//
// The pre-upgrade checks (engine present, session known, request buildable)
// run before any WS upgrade so failures surface as plain HTTP status codes.
// Once upgraded, protocol errors are relayed as a "state"/"error" text frame
// rather than an HTTP status, since the response is already a WS handshake.
func (s *Server) handleInteractiveVoice(w http.ResponseWriter, r *http.Request) {
	if s.interactiveEngine == nil {
		http.Error(w, msgEngineNotConfigured, http.StatusServiceUnavailable)
		return
	}
	sessID := r.URL.Query().Get("session")
	sess, ok := s.interactive.get(sessID)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	req, err := buildVoiceRequest(
		s.interactiveEngine, sess,
		r.URL.Query().Get("stt"),
		r.URL.Query().Get("voice"),
		r.URL.Query().Get("bargein") == "1",
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	conn, err := voiceUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error response.
	}

	wsSess := newWSAudioSession(conn)
	// Written before RunRealtimeSession starts the session, so it can never
	// race the sink's own writes (see task brief's concurrency note).
	_ = conn.WriteMessage(websocket.TextMessage, voiceStateMsg(voiceStateLive))
	de := s.interactiveEngine.GetDuplexExecutor()
	if err := de.RunRealtimeSession(r.Context(), req, wsSess); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, voiceErrorMsg(err.Error()))
	}
	_ = conn.Close()
}
