package web

import (
	"path/filepath"
	"testing"

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
