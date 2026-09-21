package voice

import "context"

// LiveRunner consumes mic frames and emits playback frames, returning when mic
// closes or ctx ends. engine.(*DuplexConversationExecutor).RunInteractiveVoice
// is adapted to this signature by the chat command. flush is called to drop
// in-flight speaker audio on barge-in (Interrupt element); the driver passes
// AudioIO.Flush so the hardware buffer is cleared immediately.
type LiveRunner func(ctx context.Context, mic <-chan []byte, play func([]byte), flush func()) error

// Driver wires hardware AudioIO to a LiveRunner and (optionally) reports RMS
// levels for the TUI meter. It owns no LLM or pipeline knowledge.
type Driver struct {
	io      AudioIO
	run     LiveRunner
	onLevel func(user, agent float32)
	guard   *EchoGuard
}

// NewDriver constructs a Driver. onLevel may be nil.
func NewDriver(io AudioIO, run LiveRunner, onLevel func(user, agent float32)) *Driver {
	return &Driver{io: io, run: run, onLevel: onLevel}
}

// NewDriverWithGuard constructs a Driver with an optional half-duplex echo guard.
// When guard is non-nil, each played frame is recorded with guard.RecordPlayback
// before it reaches AudioIO.Play, and mic frames are gated by guard.Allow before
// reaching the runner. RecordPlayback tracks a speaking deadline extended by
// each frame's own audible duration, so the gate stays correct against
// buffered hardware drivers whose Play call returns long before the audio is
// actually audible. The runner's flush callback is wrapped to also call
// guard.Reset, so a barge-in that drops queued playback also closes the gate
// immediately instead of leaving it open for audio that will now never sound.
func NewDriverWithGuard(io AudioIO, run LiveRunner, onLevel func(user, agent float32), guard *EchoGuard) *Driver {
	return &Driver{io: io, run: run, onLevel: onLevel, guard: guard}
}

// Run starts audio I/O and drives the conversation until ctx ends or mic closes.
func (d *Driver) Run(ctx context.Context) error {
	if err := d.io.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = d.io.Close() }()

	play := func(frame []byte) {
		if d.guard != nil {
			d.guard.RecordPlayback(frame)
		}
		d.io.Play(frame)
		if d.onLevel != nil {
			d.onLevel(0, rms(frame))
		}
	}

	flush := d.io.Flush
	if d.guard != nil {
		flush = func() {
			d.io.Flush()
			d.guard.Reset()
		}
	}

	mic := d.io.CaptureChunks()
	if d.onLevel != nil || d.guard != nil {
		mic = d.tapLevels(ctx, mic)
	}
	return d.run(ctx, mic, play, flush)
}

// tapLevels forwards mic frames while reporting their RMS as the user level.
// When a guard is configured, frames that fail Allow are dropped silently.
// The send to out is ctx-guarded so the goroutine exits promptly when ctx is
// canceled instead of blocking on a full or closed downstream channel.
func (d *Driver) tapLevels(ctx context.Context, in <-chan []byte) <-chan []byte {
	out := make(chan []byte)
	go func() {
		defer close(out)
		for f := range in {
			if d.guard != nil && !d.guard.Allow(f) {
				continue
			}
			if d.onLevel != nil {
				d.onLevel(rms(f), 0)
			}
			select {
			case out <- f:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
