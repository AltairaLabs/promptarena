package selfplay

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

func pcm(samples ...int16) []byte {
	out := make([]byte, 2*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[2*i:], uint16(s))
	}
	return out
}

func samplesOf(b []byte) []int16 {
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return out
}

func readAllAudio(t *testing.T, transforms []arenaconfig.AudioTransform, src []byte) ([]byte, error) {
	t.Helper()
	r, err := ApplyAudioTransforms(context.Background(), transforms, "",
		io.NopCloser(iotest.OneByteReader(bytes.NewReader(src))), 16000, "seed")
	require.NoError(t, err)
	defer r.Close()
	return io.ReadAll(r)
}

func f64(v float64) *float64 { return &v }

func TestAudioTransforms_Gain(t *testing.T) {
	// One byte at a time, so every sample straddles a read.
	out, err := readAllAudio(t, []arenaconfig.AudioTransform{{GainDB: f64(-6.0206)}},
		pcm(1000, -1000, 32000, 0))
	require.NoError(t, err)
	assert.Equal(t, []int16{500, -500, 16000, 0}, samplesOf(out))

	out, err = readAllAudio(t, []arenaconfig.AudioTransform{{GainDB: f64(20)}}, pcm(30000, -30000))
	require.NoError(t, err)
	assert.Equal(t, []int16{32767, -32768}, samplesOf(out), "louder clips at full scale")
}

func TestAudioTransforms_OddTrailingByteIsPassedOn(t *testing.T) {
	out, err := readAllAudio(t, []arenaconfig.AudioTransform{{GainDB: f64(0)}}, append(pcm(7), 0x42))
	require.NoError(t, err)
	assert.Equal(t, append(pcm(7), 0x42), out)
}

func TestAudioTransforms_NoiseIsRepeatableAtItsLevel(t *testing.T) {
	silence := make([]byte, 2*20000)
	a, err := readAllAudio(t, []arenaconfig.AudioTransform{{NoiseDBFS: f64(-30)}}, silence)
	require.NoError(t, err)
	b, err := readAllAudio(t, []arenaconfig.AudioTransform{{NoiseDBFS: f64(-30)}}, silence)
	require.NoError(t, err)
	assert.Equal(t, a, b, "the same seed gives the same noise")

	var sum float64
	for _, s := range samplesOf(a) {
		sum += float64(s) * float64(s)
	}
	rmsDBFS := 20 * math.Log10(math.Sqrt(sum/20000)/32767)
	assert.InDelta(t, -30, rmsDBFS, 0.5)
}

// TestAudioHelper is the exec audio transform the tests run: this test
// binary, re-invoked with AUDIO_HELPER set.
func TestAudioHelper(t *testing.T) {
	mode := os.Getenv("AUDIO_HELPER")
	if mode == "" {
		t.Skip("run as an audio transform by the other tests")
	}
	data, _ := io.ReadAll(os.Stdin)
	switch mode {
	case "invert":
		if os.Getenv("ARENA_SAMPLE_RATE") != "16000" || os.Getenv("ARENA_AUDIO_FORMAT") != "pcm_s16le" {
			os.Exit(4)
		}
		s := samplesOf(data)
		for i := range s {
			s[i] = -s[i]
		}
		_, _ = os.Stdout.Write(pcm(s...))
	case "fail":
		_, _ = io.WriteString(os.Stderr, "codec not found")
		os.Exit(2)
	}
	os.Exit(0)
}

func audioHelper(mode string) arenaconfig.AudioTransform {
	return arenaconfig.AudioTransform{Exec: &arenaconfig.TransformExec{
		Command: os.Args[0], Args: []string{"-test.run=^TestAudioHelper$"}, Env: map[string]string{"AUDIO_HELPER": mode},
	}}
}

func TestAudioTransforms_Exec(t *testing.T) {
	out, err := readAllAudio(t, []arenaconfig.AudioTransform{audioHelper("invert"), {GainDB: f64(-6.0206)}},
		pcm(1000, -2000))
	require.NoError(t, err)
	assert.Equal(t, []int16{-500, 1000}, samplesOf(out), "exec then gain, in order")

	_, err = readAllAudio(t, []arenaconfig.AudioTransform{audioHelper("fail")}, pcm(1))
	require.Error(t, err, "a failing program is an error, not a clean end of the audio")
	assert.Contains(t, err.Error(), "codec not found")
}

func TestAudioTransforms_ExecThatCannotStart(t *testing.T) {
	src := &closeTracker{Reader: bytes.NewReader(pcm(1))}
	_, err := ApplyAudioTransforms(context.Background(), []arenaconfig.AudioTransform{{Exec: &arenaconfig.TransformExec{
		Command: "/no/such/program"}}}, "", src, 16000, "s")
	assert.ErrorContains(t, err, "audio_transforms[0]")
	assert.True(t, src.closed, "the source is closed when the transform can't start")
}

type closeTracker struct {
	io.Reader
	closed bool
}

func (c *closeTracker) Close() error { c.closed = true; return nil }

// failingSource returns some audio, then an error, as a TTS stream that
// fails partway does.
type failingSource struct{ sent bool }

func (f *failingSource) Read(p []byte) (int, error) {
	if !f.sent {
		f.sent = true
		return copy(p, pcm(100, 200)), nil
	}
	return 0, errors.New("tts stream broke")
}
func (f *failingSource) Close() error { return nil }

func TestAudioTransforms_ExecReportsASourceThatFailsPartway(t *testing.T) {
	r, err := ApplyAudioTransforms(context.Background(), []arenaconfig.AudioTransform{audioHelper("invert")}, "",
		&failingSource{}, 16000, "s")
	require.NoError(t, err)
	defer r.Close()
	_, err = io.ReadAll(r)
	assert.ErrorContains(t, err, "tts stream broke", "truncated speech is not a clean end of the audio")
}

func TestAudioTransforms_FilterPassesASourceError(t *testing.T) {
	r, err := ApplyAudioTransforms(context.Background(), []arenaconfig.AudioTransform{{GainDB: f64(0)}}, "",
		&failingSource{}, 16000, "s")
	require.NoError(t, err)
	_, err = io.ReadAll(r)
	assert.ErrorContains(t, err, "tts stream broke")
}

func TestClampSample(t *testing.T) {
	assert.Equal(t, int16(0), clampSample(math.NaN()))
	assert.Equal(t, int16(32767), clampSample(math.Inf(1)))
	assert.Equal(t, int16(-32768), clampSample(math.Inf(-1)))
}
