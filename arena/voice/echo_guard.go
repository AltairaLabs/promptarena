package voice

import (
	"sync"
	"time"
)

// defaultHangover is how long the gate stays engaged after the last recorded
// playback frame ends, covering the tail of speaker output the mic may still
// be picking up once the frame itself has stopped sounding.
const defaultHangover = 200 * time.Millisecond

// EchoGuardOptions configures the adaptive half-duplex echo gate.
type EchoGuardOptions struct {
	// Threshold is the minimum/base RMS floor (0..1) below which mic audio is gated.
	Threshold float32
	// CouplingFactor scales the current playback RMS to compute the dynamic echo floor.
	CouplingFactor float32
	// DecayRate controls the EMA smoothing for playback level tracking. Must be
	// in (0, 1); 1.0 would freeze the estimate at the first frame forever, so
	// it is rejected the same as an out-of-range value and replaced with the
	// default.
	DecayRate float32
	// Hangover is how long, after the last played frame ends, the gate stays
	// engaged before Allow reverts to always-allow. Defaults to 200ms.
	Hangover time.Duration
}

// EchoGuard implements an adaptive half-duplex mic gate: while the agent is
// speaking, mic frames quieter than the dynamic threshold are dropped (they are
// most likely the agent's own audio bleeding from open laptop speakers into the
// mic). Louder frames still pass so a deliberate barge-in interrupts.
//
// "Speaking" is tracked as a deadline rather than a boolean. RecordPlayback
// extends the deadline by the audible duration of the frame it was just given,
// from whichever is later of "now" and the current deadline -- so a backlog of
// queued frames accumulates the way a hardware playback buffer does, and the
// window stays open for the actual audible lifetime of buffered audio instead
// of only the (near-instant) duration of the call that enqueued it. See
// Driver.Run for how frames are recorded and Reset for how barge-in clears the
// window early.
type EchoGuard struct {
	threshold      float32
	couplingFactor float32
	decayRate      float32
	hangover       time.Duration

	mu            sync.Mutex
	playbackRMS   float32
	speakingUntil time.Time
}

// NewEchoGuard builds a guard with the given RMS threshold (0..1) and the
// default adaptive coupling (CouplingFactor 0.5, DecayRate 0.8, Hangover
// 200ms). Use NewEchoGuardWithOptions to choose those explicitly.
func NewEchoGuard(threshold float32) *EchoGuard {
	return NewEchoGuardWithOptions(EchoGuardOptions{
		Threshold:      threshold,
		CouplingFactor: 0.5,
		DecayRate:      0.8,
	})
}

// NewEchoGuardWithOptions constructs an EchoGuard with custom adaptive parameters.
func NewEchoGuardWithOptions(opts EchoGuardOptions) *EchoGuard {
	if opts.Threshold <= 0 {
		opts.Threshold = 0.02
	}
	if opts.CouplingFactor < 0 {
		opts.CouplingFactor = 0.5
	}
	if opts.DecayRate <= 0 || opts.DecayRate >= 1.0 {
		opts.DecayRate = 0.8
	}
	if opts.Hangover <= 0 {
		opts.Hangover = defaultHangover
	}
	return &EchoGuard{
		threshold:      opts.Threshold,
		couplingFactor: opts.CouplingFactor,
		decayRate:      opts.DecayRate,
		hangover:       opts.Hangover,
	}
}

// RecordPlayback folds a PCM16 frame (PlaybackSampleRate, mono) that is being
// handed to the speaker into the playback RMS estimate, and extends the
// speaking window by the frame's own audible duration. Call it once per frame
// at (or just before) the point the frame is enqueued for playback -- it does
// not need Play to block for the audible duration, which is what makes it
// correct against buffered hardware drivers that return from Play early.
func (g *EchoGuard) RecordPlayback(frame []byte) {
	g.recordPlaybackAt(frame, time.Now())
}

func (g *EchoGuard) recordPlaybackAt(frame []byte, now time.Time) {
	frameRMS := rms(frame)
	dur := playbackFrameDuration(frame)

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.playbackRMS == 0 {
		g.playbackRMS = frameRMS
	} else {
		g.playbackRMS = g.decayRate*g.playbackRMS + (1-g.decayRate)*frameRMS
	}
	base := g.speakingUntil
	if base.Before(now) {
		base = now
	}
	g.speakingUntil = base.Add(dur)
}

// Reset clears the speaking window immediately. Callers should invoke it on
// barge-in (when queued playback is flushed): without it, frames already
// recorded before the flush would keep gating the mic for their full
// now-irrelevant duration even though the speaker has gone silent.
func (g *EchoGuard) Reset() {
	g.mu.Lock()
	g.speakingUntil = time.Time{}
	g.mu.Unlock()
}

// Allow reports whether a mic frame should be forwarded.
func (g *EchoGuard) Allow(frame []byte) bool {
	return g.allowAt(frame, time.Now())
}

func (g *EchoGuard) allowAt(frame []byte, now time.Time) bool {
	g.mu.Lock()
	deadline := g.speakingUntil
	playbackRMS := g.playbackRMS
	g.mu.Unlock()

	if deadline.IsZero() || !now.Before(deadline.Add(g.hangover)) {
		return true
	}

	effectiveThreshold := g.threshold
	dynamicFloor := playbackRMS * g.couplingFactor
	if dynamicFloor > effectiveThreshold {
		effectiveThreshold = dynamicFloor
	}

	return rms(frame) >= effectiveThreshold
}

// playbackFrameDuration returns how long a PCM16 mono frame at
// PlaybackSampleRate takes to sound, so the speaking window can be extended by
// exactly the audio a frame represents rather than by how long the call that
// enqueued it happened to take.
func playbackFrameDuration(frame []byte) time.Duration {
	samples := len(frame) / pcm16BytesPerSample
	return time.Duration(samples) * time.Second / time.Duration(PlaybackSampleRate)
}
