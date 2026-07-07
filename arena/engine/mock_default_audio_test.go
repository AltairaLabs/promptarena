package engine

import (
	"context"
	"os"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
	arenaaudio "github.com/AltairaLabs/promptarena/arena/audio"
)

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
