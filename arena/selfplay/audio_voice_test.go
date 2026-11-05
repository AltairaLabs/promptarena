package selfplay

import (
	"bytes"
	"context"
	"io"
	"math"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

const voiceTestRate = 16000

// tone is seconds of a sine at freq Hz, at half scale.
func tone(freq, seconds float64) []byte {
	n := int(seconds * voiceTestRate)
	s := make([]int16, n)
	for i := range s {
		s[i] = int16(16000 * math.Sin(2*math.Pi*freq*float64(i)/voiceTestRate))
	}
	return pcm(s...)
}

// pitchOf estimates a tone's frequency from its zero crossings, away from
// the ends.
func pitchOf(b []byte) float64 {
	s := samplesOf(b)
	lo, hi := len(s)/10, len(s)*9/10
	crossings := 0
	for i := lo + 1; i < hi; i++ {
		if (s[i-1] < 0) != (s[i] < 0) {
			crossings++
		}
	}
	return float64(crossings) / 2 / (float64(hi-lo) / voiceTestRate)
}

func secondsOf(b []byte) float64 { return float64(len(b)/2) / voiceTestRate }

func runAudio(t *testing.T, transforms []arenaconfig.AudioTransform, src []byte, req AudioTransformRequest) []byte {
	t.Helper()
	req.SampleRate = voiceTestRate
	// Odd-sized reads, so frames and samples straddle them.
	r, err := ApplyAudioTransforms(context.Background(), transforms, "",
		io.NopCloser(iotest.HalfReader(bytes.NewReader(src))), req)
	require.NoError(t, err)
	defer r.Close()
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return out
}

func TestAudioTransforms_SpeedKeepsThePitch(t *testing.T) {
	src := tone(300, 2)
	for _, speed := range []float64{0.5, 0.8, 1.25, 2} {
		out := runAudio(t, []arenaconfig.AudioTransform{{Speed: f64(speed)}}, src, AudioTransformRequest{})
		assert.InDelta(t, 2/speed, secondsOf(out), 0.05, "speed %v: duration", speed)
		assert.InDelta(t, 300, pitchOf(out), 6, "speed %v: pitch", speed)
	}
}

func TestAudioTransforms_PitchKeepsTheDuration(t *testing.T) {
	src := tone(300, 2)
	for _, tc := range []struct{ semitones, want float64 }{{12, 600}, {-12, 150}, {3, 300 * math.Pow(2, 0.25)}} {
		out := runAudio(t, []arenaconfig.AudioTransform{{PitchSemitones: f64(tc.semitones)}}, src, AudioTransformRequest{})
		assert.InDelta(t, 2, secondsOf(out), 0.05, "%v semitones: duration", tc.semitones)
		assert.InDelta(t, tc.want, pitchOf(out), tc.want*0.03, "%v semitones: pitch", tc.semitones)
	}
}

func TestAudioTransforms_SpeedAndPitchStreamTheSameAsWhole(t *testing.T) {
	src := tone(220, 1)
	transforms := []arenaconfig.AudioTransform{{Speed: f64(1.3)}, {PitchSemitones: f64(-4)}}
	streamed := runAudio(t, transforms, src, AudioTransformRequest{})

	r, err := ApplyAudioTransforms(context.Background(), transforms, "",
		io.NopCloser(bytes.NewReader(src)), AudioTransformRequest{SampleRate: voiceTestRate})
	require.NoError(t, err)
	whole, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, whole, streamed)
}

func TestAudioTransforms_SpeedOfEmptyAudioIsEmpty(t *testing.T) {
	assert.Empty(t, runAudio(t, []arenaconfig.AudioTransform{{Speed: f64(1.5)}}, nil, AudioTransformRequest{}))
	assert.Empty(t, runAudio(t, []arenaconfig.AudioTransform{{PitchSemitones: f64(2)}}, nil, AudioTransformRequest{}))
}

func TestAudioBestOfN_VariesByTrialAndRepeats(t *testing.T) {
	req := AudioTransformRequest{Message: "hello", Scenario: "s", Turn: 1, Seed: 42}
	seen := map[audioVariant]bool{}
	for trial := 0; trial < 8; trial++ {
		req.Trial = trial
		v := drawAudioVariant(&req, 0)
		assert.Equal(t, v, drawAudioVariant(&req, 0), "the same trial draws the same variant")
		assert.GreaterOrEqual(t, v.speed, bonSpeedMin)
		assert.LessOrEqual(t, v.speed, bonSpeedMax)
		assert.LessOrEqual(t, math.Abs(v.semitones), bonPitchMax)
		assert.LessOrEqual(t, math.Abs(v.gainDB), bonGainMax)
		if v.noisy {
			assert.GreaterOrEqual(t, v.noiseDBFS, bonNoiseMin)
			assert.LessOrEqual(t, v.noiseDBFS, bonNoiseMax)
		}
		seen[v] = true
	}
	assert.Len(t, seen, 8, "each trial draws its own variant")

	req.Trial = 0
	other := req
	other.Seed = 7
	assert.NotEqual(t, drawAudioVariant(&req, 0), drawAudioVariant(&other, 0), "defaults.seed changes the draw")
}

func TestAudioTransforms_BestOfNRewritesTheAudio(t *testing.T) {
	src := tone(300, 1)
	req := AudioTransformRequest{Message: "hello", Scenario: "s", Trial: 3}
	a := runAudio(t, []arenaconfig.AudioTransform{{BestOfN: true}}, src, req)
	assert.Equal(t, a, runAudio(t, []arenaconfig.AudioTransform{{BestOfN: true}}, src, req), "repeatable")
	assert.NotEqual(t, src, a)

	v := drawAudioVariant(&req, 0)
	assert.InDelta(t, 1/v.speed, secondsOf(a), 0.05, "the drawn speed sets the duration")

	req.Trial = 4
	assert.NotEqual(t, a, runAudio(t, []arenaconfig.AudioTransform{{BestOfN: true}}, src, req), "another trial differs")
}

func TestAudioBestOfN_NoiseDiffersPerTrial(t *testing.T) {
	silence := make([]byte, 2*voiceTestRate/2)
	req := AudioTransformRequest{Message: "hello", Scenario: "s"}
	var noisy [][]byte
	for trial := 0; len(noisy) < 2 && trial < 32; trial++ {
		req.Trial = trial
		if drawAudioVariant(&req, 0).noisy {
			noisy = append(noisy, runAudio(t, []arenaconfig.AudioTransform{{BestOfN: true}}, silence, req))
		}
	}
	require.Len(t, noisy, 2)
	// The same waveform at another level correlates near 1; independent noise
	// near 0.
	x, y := samplesOf(noisy[0]), samplesOf(noisy[1])
	var xy, xx, yy float64
	for i := range min(len(x), len(y)) {
		xy += float64(x[i]) * float64(y[i])
		xx += float64(x[i]) * float64(x[i])
		yy += float64(y[i]) * float64(y[i])
	}
	assert.Less(t, math.Abs(xy/math.Sqrt(xx*yy)), 0.2, "two noisy trials hear different noise, not one waveform rescaled")

	other := req
	other.DataRow = 1
	assert.NotEqual(t, drawAudioVariant(&req, 0), drawAudioVariant(&other, 0), "each dataset row draws its own variant")
	other = req
	other.Perturbation = 1
	assert.NotEqual(t, drawAudioVariant(&req, 0), drawAudioVariant(&other, 0), "each perturbation draws its own variant")
}

func TestAudioTransforms_SpeedPassesOnAnOddTrailingByte(t *testing.T) {
	out := runAudio(t, []arenaconfig.AudioTransform{{Speed: f64(1)}}, append(tone(300, 0.1), 0x42), AudioTransformRequest{})
	assert.Equal(t, byte(0x42), out[len(out)-1], "as gain_db does")
	assert.Equal(t, 1, len(out)%2)
}
