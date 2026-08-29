package voice

import (
	"sync"
	"sync/atomic"
)

// EchoGuardOptions configures the adaptive half-duplex echo gate.
type EchoGuardOptions struct {
	// Threshold is the minimum/base RMS floor (0..1) below which mic audio is gated.
	Threshold float32
	// CouplingFactor scales the current playback RMS to compute the dynamic echo floor.
	CouplingFactor float32
	// DecayRate controls the EMA smoothing for playback level tracking (0..1].
	DecayRate float32
}

// EchoGuard implements an adaptive half-duplex mic gate: while the agent is
// speaking, mic frames quieter than the dynamic threshold are dropped (they are
// most likely the agent's own audio bleeding from open laptop speakers into the
// mic). Louder frames still pass so a deliberate barge-in interrupts. Off
// (always-allow) unless enabled by Run.
type EchoGuard struct {
	threshold      float32
	couplingFactor float32
	decayRate      float32
	agentSpeaking  atomic.Bool
	mu             sync.Mutex
	playbackRMS    float32
}

// NewEchoGuard builds a guard with the given RMS threshold (0..1).
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
	if opts.CouplingFactor <= 0 {
		opts.CouplingFactor = 0.5
	}
	if opts.DecayRate <= 0 || opts.DecayRate > 1.0 {
		opts.DecayRate = 0.8
	}
	return &EchoGuard{
		threshold:      opts.Threshold,
		couplingFactor: opts.CouplingFactor,
		decayRate:      opts.DecayRate,
	}
}

// SetAgentSpeaking marks whether agent audio is currently playing.
func (g *EchoGuard) SetAgentSpeaking(v bool) {
	g.agentSpeaking.Store(v)
	if !v {
		g.mu.Lock()
		g.playbackRMS = 0
		g.mu.Unlock()
	}
}

// RecordPlayback updates the estimated playback level with an outgoing audio frame.
func (g *EchoGuard) RecordPlayback(frame []byte) {
	frameRMS := rms(frame)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.playbackRMS == 0 {
		g.playbackRMS = frameRMS
	} else {
		g.playbackRMS = g.decayRate*g.playbackRMS + (1-g.decayRate)*frameRMS
	}
}

// Allow reports whether a mic frame should be forwarded.
func (g *EchoGuard) Allow(frame []byte) bool {
	if !g.agentSpeaking.Load() {
		return true
	}
	g.mu.Lock()
	effectiveThreshold := g.threshold
	dynamicFloor := g.playbackRMS * g.couplingFactor
	if dynamicFloor > effectiveThreshold {
		effectiveThreshold = dynamicFloor
	}
	g.mu.Unlock()

	return rms(frame) >= effectiveThreshold
}
