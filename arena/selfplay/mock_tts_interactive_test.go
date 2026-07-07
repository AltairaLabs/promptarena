package selfplay

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/tts"
	arenaaudio "github.com/AltairaLabs/promptarena/arena/audio"
)

// TestMockTTS_DefaultsToBuiltInUserClip verifies that with no fixture audio
// configured, Synthesize returns the built-in "mock user turn" clip (not the
// old silence), so mock self-play personas are audible and self-labeling.
func TestMockTTS_DefaultsToBuiltInUserClip(t *testing.T) {
	m := &MockTTSService{SampleRate: mockTTSDefaultSampleRate} // no cachedAudio

	rc, err := m.Synthesize(context.Background(), "some persona line", tts.SynthesisConfig{})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	_ = rc.Close()

	if !bytes.Equal(got, arenaaudio.MockUserTurnPCM()) {
		t.Errorf("fallback audio = %d bytes, want the built-in user clip (%d bytes)",
			len(got), len(arenaaudio.MockUserTurnPCM()))
	}
}

// TestMockTTS_ConcurrentSynthesizeIsRaceSafe exercises the mutex added
// to MockTTSService. Without the lock, currentFileIndex + cachedAudio
// would race under `go test -race` when Synthesize is called from
// multiple goroutines concurrently — a realistic scenario when many
// arena scenarios share one mock TTS instance.
func TestMockTTS_ConcurrentSynthesizeIsRaceSafe(t *testing.T) {
	m := &MockTTSService{
		SampleRate:  mockTTSDefaultSampleRate,
		cachedAudio: [][]byte{{0x01, 0x02}, {0x03, 0x04}, {0x05, 0x06}},
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				rc, err := m.Synthesize(context.Background(), "x", tts.SynthesisConfig{})
				if err != nil {
					t.Errorf("Synthesize: %v", err)
					return
				}
				if _, err := io.ReadAll(rc); err != nil {
					t.Errorf("ReadAll: %v", err)
				}
				_ = rc.Close()
			}
		}()
	}
	wg.Wait()
}
