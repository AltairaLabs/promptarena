package engine

import (
	"context"
	"os"
	"path/filepath"

	"github.com/AltairaLabs/PromptKit/runtime/logger"
	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
	arenaaudio "github.com/AltairaLabs/promptarena/arena/audio"
)

// selfPlayUserArenaRole is the ArenaRole the runtime tags a self-play user
// turn with. Those turns' audio is synthesized by the mock TTS, so the default
// agent clip must not be applied to them.
const selfPlayUserArenaRole = "self_play_user"

// defaultAudioRepository wraps a mock ResponseRepository and fills in the
// built-in "mock assistant turn" clip for agent turns that carry no audio of
// their own — so a mock duplex run has clear, self-labeling agent audio without
// any per-config setup. Turns that already declare audio, and self-play user
// turns (which get their audio from the mock TTS), are passed through untouched.
type defaultAudioRepository struct {
	inner              mock.ResponseRepository
	assistantAudioFile string
}

func newDefaultAudioRepository(inner mock.ResponseRepository, assistantAudioFile string) *defaultAudioRepository {
	return &defaultAudioRepository{inner: inner, assistantAudioFile: assistantAudioFile}
}

func (r *defaultAudioRepository) GetResponse(ctx context.Context, params mock.ResponseParams) (string, error) {
	return r.inner.GetResponse(ctx, params)
}

func (r *defaultAudioRepository) GetTurn(ctx context.Context, params mock.ResponseParams) (*mock.Turn, error) {
	turn, err := r.inner.GetTurn(ctx, params)
	if err != nil || turn == nil {
		return turn, err
	}
	if turn.AudioFile != "" || r.assistantAudioFile == "" {
		return turn, err
	}
	if params.ArenaRole == selfPlayUserArenaRole || params.PersonaID != "" {
		return turn, err // self-play user turn — audio comes from the mock TTS
	}
	// Copy before mutating so we never alter the inner repository's own state.
	clone := *turn
	clone.AudioFile = r.assistantAudioFile
	clone.AudioSampleRate = arenaaudio.MockClipSampleRate
	return &clone, nil
}

// writeMockAssistantClip writes the embedded "mock assistant turn" clip to a
// stable temp file and returns its absolute path (the runtime mock provider
// loads audio_file from disk; it honors absolute paths). Overwriting the same
// path each run is harmless — the bytes are identical.
func writeMockAssistantClip() (string, error) {
	path := filepath.Join(os.TempDir(), "promptarena-mock-assistant-turn.pcm")
	if err := os.WriteFile(path, arenaaudio.MockAssistantTurnPCM(), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// withDefaultMockAudio wraps repo so agent turns without audio default to the
// built-in assistant clip. Best-effort: if the clip can't be materialized it
// returns repo unchanged rather than failing mock mode.
func withDefaultMockAudio(repo mock.ResponseRepository) mock.ResponseRepository {
	clipPath, err := writeMockAssistantClip()
	if err != nil {
		logger.Warn("mock provider: default agent audio unavailable, agent turns will be silent", "error", err)
		return repo
	}
	return newDefaultAudioRepository(repo, clipPath)
}
