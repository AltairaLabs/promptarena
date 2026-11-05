package selfplay

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// sampleProcessor rewrites a stream of samples. push takes the next samples
// and returns any output ready; flush ends the stream and returns the rest.
type sampleProcessor interface {
	push(in []float64) []float64
	flush() []float64
}

// chain runs processors in order.
type chain []sampleProcessor

func (c chain) push(in []float64) []float64 {
	for _, p := range c {
		in = p.push(in)
	}
	return in
}

func (c chain) flush() []float64 {
	// Each processor's last output goes through the rest before they end.
	var out []float64
	for _, p := range c {
		out = append(p.push(out), p.flush()...)
	}
	return out
}

// pcmSampleBytes is the size of a PCM16 sample.
const pcmSampleBytes = 2

// processorFilter runs a PCM16 stream through a sampleProcessor.
type processorFilter struct {
	src     io.ReadCloser
	proc    sampleProcessor
	buf     []byte
	carry   []byte // a trailing odd byte, held until its pair arrives
	pending []byte
	err     error
}

func newProcessorFilter(src io.ReadCloser, proc sampleProcessor) *processorFilter {
	return &processorFilter{src: src, proc: proc, buf: make([]byte, pcmChunk)}
}

func (f *processorFilter) Read(p []byte) (int, error) {
	for len(f.pending) == 0 {
		if f.err != nil {
			return 0, f.err
		}
		n, err := f.src.Read(f.buf)
		data := append(f.carry, f.buf[:n]...) //nolint:gocritic // carry is at most one byte
		even := len(data) &^ 1
		in := make([]float64, even/pcmSampleBytes)
		for i := range in {
			in[i] = float64(int16(binary.LittleEndian.Uint16(data[pcmSampleBytes*i:]))) //nolint:gosec // PCM16 sample bits
		}
		f.carry = append([]byte(nil), data[even:]...)
		f.pending = encodePCM(f.proc.push(in))
		if err != nil {
			// An odd byte at the very end can't be a sample; pass it on.
			f.pending = append(append(f.pending, encodePCM(f.proc.flush())...), f.carry...)
			f.carry = nil
			f.err = err
		}
	}
	n := copy(p, f.pending)
	f.pending = f.pending[n:]
	return n, nil
}

// Close closes the source.
func (f *processorFilter) Close() error { return f.src.Close() }

func encodePCM(samples []float64) []byte {
	out := make([]byte, pcmSampleBytes*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[pcmSampleBytes*i:], uint16(clampSample(s))) //nolint:gosec // PCM16 sample bits
	}
	return out
}

// stretcher changes the tempo by rate (2 is twice as fast) and keeps the
// pitch, by WSOLA: it overlap-adds windowed frames of the input, read at
// rate times the output's hop, each shifted within a tolerance to line up
// with the one before.
type stretcher struct {
	rate   float64
	frame  int // samples per frame
	hop    int // output hop, half a frame
	tol    int // how far a frame may shift from where the rate puts it
	window []float64

	in     []float64 // input not yet consumed
	inBase int       // the absolute index of in[0]
	acc    []float64 // output overlap still being added to
	k      int       // the next frame
	next   int       // where the last frame's natural continuation starts
	ended  bool
}

func newStretcher(rate float64, sampleRate int) *stretcher {
	frame := max(sampleRate*3/100&^1, 64) //nolint:mnd // 30ms frames
	w := make([]float64, frame)
	for i := range w {
		// A periodic Hann window: at half a frame's hop, the windows sum to 1.
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(frame)) //nolint:mnd // Hann
	}
	return &stretcher{
		rate: rate, frame: frame, hop: frame / 2, tol: frame / 4, window: w, //nolint:mnd // half and quarter frames
		acc: make([]float64, frame),
	}
}

func (s *stretcher) push(in []float64) []float64 {
	s.in = append(s.in, in...)
	return s.run()
}

func (s *stretcher) flush() []float64 {
	s.ended = true
	out := s.run()
	if s.k == 0 {
		return out
	}
	// The last frame's tail is still in the overlap.
	return append(out, s.acc[:s.hop]...)
}

// at returns the input sample at absolute index i, silence past either end.
func (s *stretcher) at(i int) float64 {
	j := i - s.inBase
	if j < 0 || j >= len(s.in) {
		return 0
	}
	return s.in[j]
}

func (s *stretcher) run() []float64 {
	var out []float64
	end := s.inBase + len(s.in)
	for {
		nominal := int(math.Round(float64(s.k*s.hop) * s.rate))
		if s.ended && (nominal >= end || end == 0) {
			return out
		}
		lo, hi := max(nominal-s.tol, 0), nominal+s.tol
		if !s.ended && max(hi, s.next)+s.frame > end {
			return out
		}
		pos := nominal
		if s.k > 0 {
			pos = s.bestMatch(lo, hi)
		}
		for i := range s.frame {
			s.acc[i] += s.window[i] * s.at(pos+i)
		}
		out = append(out, s.acc[:s.hop]...)
		copy(s.acc, s.acc[s.hop:])
		clear(s.acc[s.frame-s.hop:])
		s.next = pos + s.hop
		s.k++
		s.trim(min(int(math.Round(float64(s.k*s.hop)*s.rate))-s.tol, s.next))
	}
}

// bestMatch returns the frame start in [lo, hi] most like the last frame's
// natural continuation.
func (s *stretcher) bestMatch(lo, hi int) int {
	best, bestScore := lo, math.Inf(-1)
	for pos := lo; pos <= hi; pos++ {
		score := 0.0
		for i := 0; i < s.frame; i += 2 {
			score += s.at(pos+i) * s.at(s.next+i)
		}
		if score > bestScore {
			best, bestScore = pos, score
		}
	}
	return best
}

// trim drops input before the absolute index from.
func (s *stretcher) trim(from int) {
	if drop := from - s.inBase; drop > 0 {
		drop = min(drop, len(s.in))
		s.in = s.in[drop:]
		s.inBase += drop
	}
}

// resampler reads the input factor times as fast, by linear interpolation:
// the pitch goes up by factor and the duration down by it.
type resampler struct {
	factor float64
	in     []float64
	inBase int
	pos    float64 // the absolute input position of the next output sample
}

func (r *resampler) push(in []float64) []float64 {
	r.in = append(r.in, in...)
	return r.run(false)
}

func (r *resampler) flush() []float64 { return r.run(true) }

func (r *resampler) run(ended bool) []float64 {
	var out []float64
	end := r.inBase + len(r.in)
	for {
		i := int(r.pos)
		if i >= end || (!ended && i+1 >= end) {
			break
		}
		a := r.in[i-r.inBase]
		b := a
		if i+1 < end {
			b = r.in[i+1-r.inBase]
		}
		out = append(out, a+(b-a)*(r.pos-float64(i)))
		r.pos += r.factor
	}
	if drop := int(r.pos) - r.inBase; drop > 0 {
		drop = min(drop, len(r.in))
		r.in = r.in[drop:]
		r.inBase += drop
	}
	return out
}

// newPitchShifter shifts the pitch by semitones and keeps the duration.
func newPitchShifter(semitones float64, sampleRate int) sampleProcessor {
	return newTempoPitch(1, semitones, sampleRate)
}

// newTempoPitch changes the tempo by speed and the pitch by semitones in one
// pass: stretch the tempo by speed/factor, then read it factor times as fast,
// which raises the pitch by factor and brings the tempo to speed.
func newTempoPitch(speed, semitones float64, sampleRate int) sampleProcessor {
	factor := math.Pow(2, semitones/12) //nolint:mnd // semitones in an octave
	return chain{newStretcher(speed/factor, sampleRate), &resampler{factor: factor}}
}

// The ranges best_of_n draws from, after Best-of-N's audio augmentations
// (arXiv 2412.03556): tempo, pitch, volume and background noise.
const (
	bonSpeedMin  = 0.8
	bonSpeedMax  = 1.25
	bonPitchMax  = 3.0  // semitones either way
	bonGainMax   = 6.0  // dB either way
	bonNoiseMin  = -45. // dBFS
	bonNoiseMax  = -25. // dBFS
	bonNoiseOdds = 0.5
)

// audioVariant is one best_of_n draw.
type audioVariant struct {
	speed, semitones, gainDB float64
	noisy                    bool
	noiseDBFS                float64
}

// variantSeed is best_of_n's seed for the transform at index: the scenario,
// turn, trial, data row, perturbation, defaults.seed and message.
func variantSeed(req *AudioTransformRequest, index int) string {
	return fmt.Sprintf("%s\x00%d\x00%d\x00%d\x00%d\x00%d\x00%d\x00%s", req.Scenario, req.Turn, req.Trial,
		req.DataRow, req.Perturbation, req.Seed, index, req.Message)
}

// drawAudioVariant draws the variant for the transform at index.
func drawAudioVariant(req *AudioTransformRequest, index int) audioVariant {
	rng := seededRNG(variantSeed(req, index))
	between := func(lo, hi float64) float64 { return lo + rng.Float64()*(hi-lo) }
	v := audioVariant{
		speed:     between(bonSpeedMin, bonSpeedMax),
		semitones: between(-bonPitchMax, bonPitchMax),
		gainDB:    between(-bonGainMax, bonGainMax),
		noisy:     rng.Float64() < bonNoiseOdds,
	}
	if v.noisy {
		v.noiseDBFS = between(bonNoiseMin, bonNoiseMax)
	}
	return v
}

// newBestOfN draws the variant for the transform at index and applies it:
// tempo and pitch, then volume, then noise. The noise is seeded like the
// draw, so it differs per trial too.
func newBestOfN(src io.ReadCloser, req *AudioTransformRequest, index int) io.ReadCloser {
	v := drawAudioVariant(req, index)
	out := io.ReadCloser(newProcessorFilter(src, newTempoPitch(v.speed, v.semitones, req.SampleRate)))
	factor := math.Pow(10, v.gainDB/20) //nolint:mnd // decibels to amplitude
	out = newPCMFilter(out, func(s float64) float64 { return s * factor })
	if v.noisy {
		out = newNoiseFilter(out, v.noiseDBFS, variantSeed(req, index)+"\x00noise")
	}
	return out
}
