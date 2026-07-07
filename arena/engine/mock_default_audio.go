package engine

import (
	"context"
	"os"
	"path/filepath"

	"github.com/AltairaLabs/PromptKit/runtime/logger"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
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

// mockResponseInjector is satisfied by the runtime mock streaming provider (via
// its embedded *StreamingProvider). It lets arena swap in a decorated
// repository after the runtime factory has already built the provider.
type mockResponseInjector interface {
	WithMockResponses(repo mock.ResponseRepository, scenarioID, fixtureBaseDir string) *mock.StreamingProvider
}

// applyDefaultMockAgentAudio re-wires a config-built mock provider so agent
// turns that declare no audio default to the built-in assistant clip. This is
// the `--provider mock-duplex` counterpart of withDefaultMockAudio (which only
// runs under --mock-provider). No-op unless the provider is a mock streaming
// provider backed by a mock_config file.
func applyDefaultMockAgentAudio(prov providers.Provider, additionalConfig map[string]any) {
	injector, ok := prov.(mockResponseInjector)
	if !ok {
		return
	}
	mockCfg, _ := additionalConfig["mock_config"].(string)
	if mockCfg == "" {
		return
	}
	repo, baseDir, err := buildDecoratedMockRepo(mockCfg)
	if err != nil {
		logger.Warn("mock provider: default agent audio unavailable", "error", err)
		return
	}
	scenarioID, _ := additionalConfig["duplex_scenario"].(string)
	injector.WithMockResponses(repo, scenarioID, baseDir)
}

// buildDecoratedMockRepo loads a FileMockRepository from mockCfg and wraps it so
// audio-less agent turns default to the built-in assistant clip. Returns the
// decorated repo and the repo's fixture base dir (relative audio_file paths
// resolve against it).
func buildDecoratedMockRepo(mockCfg string) (mock.ResponseRepository, string, error) {
	fileRepo, err := mock.NewFileMockRepository(mockCfg)
	if err != nil {
		return nil, "", err
	}
	return withDefaultMockAudio(fileRepo), fileRepo.BaseDir(), nil
}
