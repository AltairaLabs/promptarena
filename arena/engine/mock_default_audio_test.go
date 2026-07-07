package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
	arenaaudio "github.com/AltairaLabs/promptarena/arena/audio"
)

// writeMockResponses writes a minimal mock-responses.yaml with one agent turn
// lacking audio, one agent turn with its own audio, and a selfplay persona
// turn, then returns its path.
func writeMockResponses(t *testing.T) string {
	t.Helper()
	const yaml = `scenarios:
  demo:
    turns:
      1:
        content: "agent line, no audio of its own"
      2:
        content: "agent line with its own audio"
        audio_file: custom.pcm
selfplay:
  persona-x:
    turns:
      1: "persona line, no audio"
`
	path := filepath.Join(t.TempDir(), "mock-responses.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestBuildDecoratedMockRepo proves the repo that gets injected into a
// config-loaded mock provider (`--provider mock-duplex`) defaults audio-less
// agent turns to the built-in assistant clip, while preserving turns that
// declare their own audio and leaving self-play persona turns silent (their
// audio comes from mock-TTS).
func TestBuildDecoratedMockRepo(t *testing.T) {
	ctx := context.Background()
	repo, baseDir, err := buildDecoratedMockRepo(writeMockResponses(t))
	if err != nil {
		t.Fatal(err)
	}
	if baseDir == "" {
		t.Error("baseDir should be the mock-responses.yaml dir, got empty")
	}

	t.Run("audio-less agent turn defaults to the assistant clip", func(t *testing.T) {
		got, err := repo.GetTurn(ctx, mock.ResponseParams{ScenarioID: "demo", TurnNumber: 1})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(got.AudioFile, "promptarena-mock-assistant-turn.pcm") {
			t.Errorf("AudioFile = %q, want the built-in assistant clip", got.AudioFile)
		}
		if got.AudioSampleRate != arenaaudio.MockClipSampleRate {
			t.Errorf("AudioSampleRate = %d, want %d", got.AudioSampleRate, arenaaudio.MockClipSampleRate)
		}
	})

	t.Run("agent turn with its own audio is preserved", func(t *testing.T) {
		got, err := repo.GetTurn(ctx, mock.ResponseParams{ScenarioID: "demo", TurnNumber: 2})
		if err != nil {
			t.Fatal(err)
		}
		if got.AudioFile != "custom.pcm" {
			t.Errorf("AudioFile = %q, want custom.pcm (not overwritten)", got.AudioFile)
		}
	})

	t.Run("self-play persona turn stays silent", func(t *testing.T) {
		got, err := repo.GetTurn(ctx, mock.ResponseParams{
			ScenarioID: "demo", TurnNumber: 1,
			PersonaID: "persona-x", ArenaRole: selfPlayUserArenaRole,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.AudioFile != "" {
			t.Errorf("self-play persona turn got AudioFile %q, want none", got.AudioFile)
		}
	})
}

// TestConfigMockProviderAcceptsAudioInjection proves the wiring is real: a mock
// provider built the way createProviderImpl builds it actually implements the
// injector interface, so applyDefaultMockAgentAudio is not a silent no-op.
func TestConfigMockProviderAcceptsAudioInjection(t *testing.T) {
	spec := providers.ProviderSpec{
		ID:    "mock-duplex",
		Type:  "mock",
		Model: "mock-duplex-model",
		AdditionalConfig: map[string]any{
			"mock_config":  writeMockResponses(t),
			"auto_respond": true,
		},
	}
	prov, err := providers.CreateProviderFromSpec(spec)
	if err != nil {
		t.Fatalf("CreateProviderFromSpec: %v", err)
	}
	if _, ok := prov.(mockResponseInjector); !ok {
		t.Fatalf("config mock provider %T does not implement mockResponseInjector; "+
			"applyDefaultMockAgentAudio would silently do nothing", prov)
	}
	// Must not panic, and is a no-op-safe operation on a real provider.
	applyDefaultMockAgentAudio(prov, spec.AdditionalConfig)
}

// fakeMockRepo returns a fixed turn, letting us exercise the default-audio
// decorator in isolation.
type fakeMockRepo struct{ turn mock.Turn }

func (f *fakeMockRepo) GetResponse(context.Context, mock.ResponseParams) (string, error) {
	return f.turn.Content, nil
}
func (f *fakeMockRepo) GetTurn(context.Context, mock.ResponseParams) (*mock.Turn, error) {
	t := f.turn // copy so the decorator can't mutate our fixture
	return &t, nil
}

func TestDefaultAudioRepository(t *testing.T) {
	const clip = "/tmp/assistant.pcm"
	ctx := context.Background()

	t.Run("agent turn without audio gets the default clip", func(t *testing.T) {
		r := newDefaultAudioRepository(&fakeMockRepo{turn: mock.Turn{Content: "hi"}}, clip)
		got, err := r.GetTurn(ctx, mock.ResponseParams{})
		if err != nil {
			t.Fatal(err)
		}
		if got.AudioFile != clip {
			t.Errorf("AudioFile = %q, want %q", got.AudioFile, clip)
		}
		if got.AudioSampleRate != arenaaudio.MockClipSampleRate {
			t.Errorf("AudioSampleRate = %d, want %d", got.AudioSampleRate, arenaaudio.MockClipSampleRate)
		}
	})

	t.Run("self-play user turn is left untouched", func(t *testing.T) {
		r := newDefaultAudioRepository(&fakeMockRepo{turn: mock.Turn{Content: "persona"}}, clip)
		got, _ := r.GetTurn(ctx, mock.ResponseParams{ArenaRole: selfPlayUserArenaRole})
		if got.AudioFile != "" {
			t.Errorf("self-play user turn got AudioFile %q, want none", got.AudioFile)
		}
		// Same for a persona-tagged turn.
		got2, _ := r.GetTurn(ctx, mock.ResponseParams{PersonaID: "aggressive"})
		if got2.AudioFile != "" {
			t.Errorf("persona turn got AudioFile %q, want none", got2.AudioFile)
		}
	})

	t.Run("turn with its own audio is preserved", func(t *testing.T) {
		r := newDefaultAudioRepository(&fakeMockRepo{turn: mock.Turn{AudioFile: "custom.pcm"}}, clip)
		got, _ := r.GetTurn(ctx, mock.ResponseParams{})
		if got.AudioFile != "custom.pcm" {
			t.Errorf("AudioFile = %q, want custom.pcm (not overwritten)", got.AudioFile)
		}
	})
}

func TestWriteMockAssistantClip(t *testing.T) {
	path, err := writeMockAssistantClip()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("clip not written: %v", err)
	}
	if len(data) == 0 || len(data) != len(arenaaudio.MockAssistantTurnPCM()) {
		t.Errorf("written clip is %d bytes, want %d", len(data), len(arenaaudio.MockAssistantTurnPCM()))
	}
}
