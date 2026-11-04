package selfplay

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/AltairaLabs/promptarena/v2/arena/arenaconfig"
)

// pcmFullScale is the largest 16-bit sample magnitude.
const pcmFullScale = 32767

// pcmChunk is how many bytes a PCM filter reads from its source at a time.
const pcmChunk = 4096

// ApplyAudioTransforms wraps a 16-bit little-endian mono PCM stream with the
// turn's audio transforms, in order. The result streams: each transform
// works on the audio as it arrives, never on the whole utterance at once.
// seed makes noise repeatable; the message text is a good one. Closing the
// returned reader closes src.
func ApplyAudioTransforms(
	ctx context.Context, transforms []arenaconfig.AudioTransform, configDir string,
	src io.ReadCloser, sampleRate int, seed string,
) (io.ReadCloser, error) {
	out := src
	for i := range transforms {
		t := &transforms[i]
		switch {
		case t.GainDB != nil:
			factor := math.Pow(10, *t.GainDB/20) //nolint:mnd // decibels to amplitude
			out = newPCMFilter(out, func(s float64) float64 { return s * factor })
		case t.NoiseDBFS != nil:
			out = newNoiseFilter(out, *t.NoiseDBFS, fmt.Sprintf("%s#%d", seed, i))
		case t.Exec != nil:
			r, err := newExecAudioFilter(ctx, t.Exec, configDir, out, sampleRate)
			if err != nil {
				_ = out.Close()
				return nil, fmt.Errorf("audio_transforms[%d]: %w", i, err)
			}
			out = r
		}
	}
	return out, nil
}

// pcmFilter applies a function to every sample of a PCM16 stream.
type pcmFilter struct {
	src     io.ReadCloser
	fn      func(float64) float64
	buf     []byte
	carry   []byte // a trailing odd byte, held until its pair arrives
	pending []byte
	err     error
}

func newPCMFilter(src io.ReadCloser, fn func(float64) float64) *pcmFilter {
	return &pcmFilter{src: src, fn: fn, buf: make([]byte, pcmChunk)}
}

func (f *pcmFilter) Read(p []byte) (int, error) {
	for len(f.pending) == 0 {
		if f.err != nil {
			return 0, f.err
		}
		n, err := f.src.Read(f.buf)
		data := append(f.carry, f.buf[:n]...) //nolint:gocritic // carry is at most one byte
		even := len(data) &^ 1
		for i := 0; i < even; i += 2 {
			//nolint:gosec // reinterpreting the PCM16 sample bits, both ways
			s := float64(int16(binary.LittleEndian.Uint16(data[i:])))
			binary.LittleEndian.PutUint16(data[i:], uint16(clampSample(f.fn(s)))) //nolint:gosec // as above
		}
		f.carry = append([]byte(nil), data[even:]...)
		f.pending = data[:even]
		if err != nil {
			// An odd byte at the very end can't be a sample; pass it on.
			f.pending = append(f.pending, f.carry...)
			f.carry = nil
			f.err = err
		}
	}
	n := copy(p, f.pending)
	f.pending = f.pending[n:]
	return n, nil
}

// Close closes the source.
func (f *pcmFilter) Close() error { return f.src.Close() }

func clampSample(v float64) int16 {
	if math.IsNaN(v) {
		return 0
	}
	v = math.Round(v)
	switch {
	case v > pcmFullScale:
		return pcmFullScale
	case v < -pcmFullScale-1:
		return -pcmFullScale - 1
	}
	return int16(v)
}

// newNoiseFilter adds uniform white noise at level dBFS, seeded from seed.
func newNoiseFilter(src io.ReadCloser, dbfs float64, seed string) *pcmFilter {
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	rng := rand.New(rand.NewPCG(h.Sum64(), 0)) //nolint:gosec // repeatable test noise, not security
	// Uniform noise on [-a, a] has RMS a/√3; scale so its RMS is the level.
	amplitude := math.Pow(10, dbfs/20) * pcmFullScale * math.Sqrt(3) //nolint:mnd // dB to amplitude, uniform RMS
	return newPCMFilter(src, func(s float64) float64 { return s + (rng.Float64()*2-1)*amplitude })
}

// execAudioFilter pipes a PCM stream through a program: the source is
// copied to its stdin as it arrives, and reads come from its stdout.
type execAudioFilter struct {
	src    io.ReadCloser
	stdout io.ReadCloser
	run    *transformRun
	// fed receives the source copy's error once the copy ends.
	fed     chan error
	copyErr error
	once    sync.Once
	err     error
}

func newExecAudioFilter(
	ctx context.Context, cfg *arenaconfig.TransformExec, configDir string, src io.ReadCloser, sampleRate int,
) (*execAudioFilter, error) {
	run := newTransformRun(ctx, cfg, configDir, time.Duration(cfg.TimeoutMs)*time.Millisecond, []string{
		"ARENA_SAMPLE_RATE=" + strconv.Itoa(sampleRate), "ARENA_AUDIO_FORMAT=pcm_s16le", "ARENA_AUDIO_CHANNELS=1",
	})
	stdin, err := run.cmd.StdinPipe()
	if err != nil {
		run.cancel()
		return nil, err
	}
	stdout, err := run.cmd.StdoutPipe()
	if err != nil {
		run.cancel()
		return nil, err
	}
	if err := run.cmd.Start(); err != nil {
		run.cancel()
		return nil, fmt.Errorf("%s: %w", cfg.Command, err)
	}
	f := &execAudioFilter{src: src, stdout: stdout, run: run, fed: make(chan error, 1)}
	go func() {
		_, copyErr := io.Copy(stdin, src)
		_ = stdin.Close()
		f.fed <- copyErr
	}()
	return f, nil
}

// Read returns the program's output. At its end, a program that failed, or
// a source that failed partway, is an error rather than a clean end of the
// audio.
func (f *execAudioFilter) Read(p []byte) (int, error) {
	n, err := f.stdout.Read(p)
	if errors.Is(err, io.EOF) {
		if ferr := f.finish(); ferr != nil {
			return n, ferr
		}
	}
	return n, err
}

// finish waits for the program and the source copy, once.
func (f *execAudioFilter) finish() error {
	f.once.Do(func() {
		if waitErr := f.run.cmd.Wait(); waitErr != nil {
			f.err = f.run.failure(waitErr)
		}
		f.run.cancel()
		f.copyErr = f.awaitCopy()
		// A program that exits before reading all its input closes the pipe;
		// that isn't the source failing.
		if f.err == nil && f.copyErr != nil && !errors.Is(f.copyErr, io.ErrClosedPipe) &&
			!errors.Is(f.copyErr, syscall.EPIPE) && !errors.Is(f.copyErr, os.ErrClosed) {
			f.err = fmt.Errorf("reading the audio for %s: %w", f.run.name, f.copyErr)
		}
	})
	return f.err
}

// awaitCopy waits for the source copy to end, for a short while: once the
// program has exited its stdin is closed, so a copy still waiting on the
// source ends when the source is closed.
func (f *execAudioFilter) awaitCopy() error {
	select {
	case err := <-f.fed:
		f.fed <- err
		return err
	case <-time.After(transformWaitDelay):
		return nil
	}
}

// Close stops the program, waits for it, and closes the source.
func (f *execAudioFilter) Close() error {
	f.run.cancel()
	_ = f.finish()
	return f.src.Close()
}
