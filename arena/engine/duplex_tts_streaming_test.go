package engine

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
	"github.com/AltairaLabs/promptarena/v2/arena/selfplay"

	"github.com/AltairaLabs/PromptKit/pkg/v2/config"
)

func TestTurnAudioMirror_WriteFinalizeCleanup(t *testing.T) {
	m, err := newTurnAudioMirror("turn-abc")
	require.NoError(t, err)
	require.NotEmpty(t, m.path)

	require.NoError(t, m.write([]byte("chunk1")))
	require.NoError(t, m.write([]byte("chunk2")))

	abs, err := m.finalize()
	require.NoError(t, err)
	assert.True(t, m.finalized)
	assert.NotEmpty(t, abs)

	contents, err := os.ReadFile(abs)
	require.NoError(t, err)
	assert.Equal(t, "chunk1chunk2", string(contents))

	m.cleanup()
	_, err = os.Stat(abs)
	assert.True(t, os.IsNotExist(err), "cleanup should remove the file")

	// idempotent
	m.cleanup()
}

func TestTurnAudioMirror_NilSafe(t *testing.T) {
	var m *turnAudioMirror
	assert.NoError(t, m.write([]byte("ignored")))
	abs, err := m.finalize()
	assert.NoError(t, err)
	assert.Empty(t, abs)
	m.cleanup()
}

func TestTurnAudioMirror_WriteAfterFinalizeIsNoop(t *testing.T) {
	m, err := newTurnAudioMirror("turn-finalized")
	require.NoError(t, err)
	defer m.cleanup()

	require.NoError(t, m.write([]byte("early")))
	abs, err := m.finalize()
	require.NoError(t, err)

	// After finalize the file has been handed off; a late chunk must succeed
	// silently and must not reach it.
	assert.NoError(t, m.write([]byte("late")))
	contents, err := os.ReadFile(abs)
	require.NoError(t, err)
	assert.Equal(t, "early", string(contents))
}

func TestOpenTextSynthesisStream_HappyPath(t *testing.T) {
	// A provider with Type=mock and Capability=tts — GetForProvider will
	// return the default MockTTSService (no audio files → empty stream).
	p := &config.Provider{
		ID:    "mock-tts",
		Type:  "mock",
		Role:  config.RoleTTS,
		Voice: "v1",
	}
	reg := selfplay.NewRegistryWithTTS(
		nil,
		map[string]string{},
		map[string]*arenaconfig.UserPersonaPack{},
		[]arenaconfig.SelfPlayRoleGroup{},
		selfplay.NewTTSRegistry(),
	)
	de := &DuplexConversationExecutor{selfPlayRegistry: reg}

	result, err := de.openTextSynthesisStream(context.Background(), "hello", p)
	require.NoError(t, err)
	require.NotNil(t, result.Reader)
	defer result.Reader.Close()
	assert.Equal(t, "hello", result.Text, "the scripted text is what gets synthesized")
	assert.Nil(t, result.TextResult, "no LLM ran for a scripted turn")
}

func TestOpenTextSynthesisStream_WrongCapabilityErrors(t *testing.T) {
	// A provider with non-TTS capability should be rejected by GetForProvider.
	p := &config.Provider{
		ID:    "llm-provider",
		Type:  "openai",
		Role:  config.RoleLLM,
		Voice: "v1",
	}
	reg := selfplay.NewRegistryWithTTS(
		nil,
		map[string]string{},
		map[string]*arenaconfig.UserPersonaPack{},
		[]arenaconfig.SelfPlayRoleGroup{},
		selfplay.NewTTSRegistry(),
	)
	de := &DuplexConversationExecutor{selfPlayRegistry: reg}

	_, err := de.openTextSynthesisStream(context.Background(), "hello", p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get TTS service for provider")
}
