package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/arena/engine"
)

const (
	voiceFixtureConfig     = "testdata/voice-config.yaml"
	voiceFixtureProviderID = "voice-mock"
	voiceFixtureSTTID      = "voice-stt"
	voiceFixtureTaskType   = "voice"
)

// newVoiceTestEngine builds an *engine.Engine from the voice-capable fixture
// with mock provider mode enabled, mirroring newTestServer in
// interactive_test.go.
func newVoiceTestEngine(t *testing.T) *engine.Engine {
	t.Helper()
	eng, err := engine.NewEngineFromConfigFile(filepath.Clean(voiceFixtureConfig))
	if err != nil {
		t.Fatalf("NewEngineFromConfigFile: %v", err)
	}
	if err := eng.EnableMockProviderMode(""); err != nil {
		t.Fatalf("EnableMockProviderMode: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng
}

// newTestServerWithVoiceEngine builds a *Server via the same constructor the
// other web tests use, with interactiveEngine set to the Task 4 voice fixture
// engine so handlers gated on it (e.g. handleInteractiveVoice) can run.
func newTestServerWithVoiceEngine(t *testing.T) *Server {
	t.Helper()
	eng := newVoiceTestEngine(t)
	s := newServerWithRunner(nil, eng, nil, "")
	s.interactiveEngine = eng
	return s
}

func newVoiceTestSession(t *testing.T, eng *engine.Engine) *engine.InteractiveSession {
	t.Helper()
	sess, err := eng.NewInteractiveSession(engine.InteractiveSessionOptions{
		ProviderID: voiceFixtureProviderID,
		TaskType:   voiceFixtureTaskType,
	})
	if err != nil {
		t.Fatalf("NewInteractiveSession: %v", err)
	}
	return sess
}

// TestBuildVoiceRequest_WiresEventBus pins the fix for the empty-message-window
// bug: the voice request must carry the engine's event bus so voice turns
// publish message/transcript/tool events to the SSE stream the console renders.
func TestBuildVoiceRequest_WiresEventBus(t *testing.T) {
	eng := newVoiceTestEngine(t)
	bus := events.NewEventBus()
	eng.SetEventBus(bus, engine.WithMessageEvents())
	sess := newVoiceTestSession(t, eng)

	req, err := buildVoiceRequest(eng, sess, "", "", true)
	if err != nil {
		t.Fatalf("buildVoiceRequest: %v", err)
	}
	if req.EventBus == nil {
		t.Fatal("EventBus not wired onto the voice request — message window would stay empty")
	}
	if req.EventBus != bus {
		t.Fatal("EventBus should be the engine's configured bus")
	}
}

func TestVoiceProviderIDs_OnlyRealtime(t *testing.T) {
	eng := newVoiceTestEngine(t)
	got := eng.VoiceProviderIDs()
	if len(got) != 1 || got[0] != voiceFixtureProviderID {
		t.Fatalf("want [%s] (only the realtime provider, not the stt provider), got %v", voiceFixtureProviderID, got)
	}
}

// TestVoiceProviderIDs_DiscriminatesRealProviders is the authoritative test for
// the capability check: with REAL providers (no mock mode, constructed
// credential-free), a Gemini Live provider and an OpenAI realtime provider are
// detected, while an OpenAI text model is excluded. This is the case the old
// config-flag heuristic got wrong for Gemini.
func TestVoiceProviderIDs_DiscriminatesRealProviders(t *testing.T) {
	eng, err := engine.NewEngineFromConfigFile(filepath.Clean("testdata/capability-config.yaml"))
	if err != nil {
		t.Fatalf("NewEngineFromConfigFile: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	got := eng.VoiceProviderIDs()
	want := []string{"cap-gemini", "cap-openai-realtime"} // sorted; cap-openai-text excluded
	if len(got) != len(want) {
		t.Fatalf("VoiceProviderIDs: want %v (gemini + openai-realtime, excluding the text model), got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("VoiceProviderIDs: want %v, got %v", want, got)
		}
	}
}

func TestOptionsIncludesVoiceProviders(t *testing.T) {
	s := newTestServerWithVoiceEngine(t)
	rec := httptest.NewRecorder()
	s.handleInteractiveOptions(rec, httptest.NewRequest(http.MethodGet, "/api/interactive/options", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var out struct {
		VoiceProviders []string `json:"voiceProviders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	found := false
	for _, id := range out.VoiceProviders {
		if id == voiceFixtureProviderID {
			found = true
		}
	}
	if !found {
		t.Fatalf("voiceProviders should include %q, got %v", voiceFixtureProviderID, out.VoiceProviders)
	}
}

func TestBuildVoiceRequest_RealtimeWhenNoSTT(t *testing.T) {
	eng := newVoiceTestEngine(t)
	sess := newVoiceTestSession(t, eng)

	req, err := buildVoiceRequest(eng, sess, "", "", true)
	if err != nil {
		t.Fatalf("buildVoiceRequest: %v", err)
	}
	if req.VoiceSTT != nil {
		t.Fatalf("want nil VoiceSTT for realtime mode, got %+v", req.VoiceSTT)
	}
	if req.Provider != sess.Provider() {
		t.Fatal("provider not carried from session")
	}
	if !req.VoiceBargeIn {
		t.Fatal("barge-in flag not set")
	}
	if req.ConversationID != sess.ConversationID() {
		t.Fatal("conversation id mismatch")
	}
	if req.RunID != sess.ConversationID() {
		t.Fatal("run id should match conversation id")
	}
	if req.Scenario == nil {
		t.Fatal("scenario must not be nil")
	}
	if req.Scenario.TaskType != voiceFixtureTaskType {
		t.Fatalf("want scenario task type %q, got %q", voiceFixtureTaskType, req.Scenario.TaskType)
	}
	if req.Scenario.Duplex == nil || req.Scenario.Duplex.TurnDetection == nil {
		t.Fatal("want duplex + turn detection configured")
	}
	if req.Scenario.Duplex.TurnDetection.Mode != arenaconfig.TurnDetectionModeASM {
		t.Fatalf("want ASM turn detection mode, got %q", req.Scenario.Duplex.TurnDetection.Mode)
	}
	if req.Config == nil {
		t.Fatal("config must be carried onto the request")
	}
	if req.StateStoreConfig == nil || req.StateStoreConfig.Store == nil {
		t.Fatal("state store must be wired")
	}
}

func TestBuildVoiceRequest_ComposedWhenSTTSet(t *testing.T) {
	eng := newVoiceTestEngine(t)
	sess := newVoiceTestSession(t, eng)

	req, err := buildVoiceRequest(eng, sess, voiceFixtureSTTID, "some-voice", false)
	if err != nil {
		t.Fatalf("buildVoiceRequest: %v", err)
	}
	if req.VoiceSTT == nil {
		t.Fatal("want non-nil VoiceSTT for composed VAD mode")
	}
	if req.VoiceSTT.ID != voiceFixtureSTTID {
		t.Fatalf("want VoiceSTT id %q, got %q", voiceFixtureSTTID, req.VoiceSTT.ID)
	}
	if req.VoiceOutputVoice != "some-voice" {
		t.Fatalf("want output voice carried through, got %q", req.VoiceOutputVoice)
	}
	if req.VoiceBargeIn {
		t.Fatal("barge-in flag should be false")
	}
	if req.Scenario.Duplex.TurnDetection.Mode != arenaconfig.TurnDetectionModeVAD {
		t.Fatalf("want VAD turn detection mode, got %q", req.Scenario.Duplex.TurnDetection.Mode)
	}
}

func TestBuildVoiceRequest_UnknownSTTErrors(t *testing.T) {
	eng := newVoiceTestEngine(t)
	sess := newVoiceTestSession(t, eng)

	if _, err := buildVoiceRequest(eng, sess, "no-such-stt", "", false); err == nil {
		t.Fatal("want error for unknown stt provider id, got nil")
	}
}

func TestHandleVoiceUnknownSession(t *testing.T) {
	s := newTestServerWithVoiceEngine(t)
	req := httptest.NewRequest(http.MethodGet, "/api/interactive/voice?session=nope", nil)
	rec := httptest.NewRecorder()
	s.handleInteractiveVoice(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}
