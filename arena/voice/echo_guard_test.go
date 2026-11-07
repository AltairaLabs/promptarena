package voice

import (
	"testing"
	"time"
)

func TestEchoGuard_AllowsWhenAgentSilent(t *testing.T) {
	g := NewEchoGuard(0.5)
	if !g.Allow(make([]byte, 64)) {
		t.Fatal("expected mic allowed when agent not speaking")
	}
}

func TestEchoGuard_GatesQuietMicWhilePlaybackWindowOpen(t *testing.T) {
	g := NewEchoGuard(0.5)
	now := time.Now()
	g.recordPlaybackAt(make([]byte, 6400), now) // ~200ms of silence at 24kHz mono PCM16
	if g.allowAt(make([]byte, 64), now) {       // silence < threshold
		t.Fatal("expected quiet mic gated while the playback window is open")
	}
}

func TestEchoGuard_AllowsLoudBargeInWhilePlaybackWindowOpen(t *testing.T) {
	g := NewEchoGuard(0.001) // very low threshold
	now := time.Now()
	g.recordPlaybackAt(make([]byte, 6400), now)
	loud := []byte{0xff, 0x7f, 0xff, 0x7f} // near max amplitude
	if !g.allowAt(loud, now) {
		t.Fatal("expected loud barge-in to pass the gate")
	}
}

func TestEchoGuard_AllowsOnceThePlaybackWindowElapses(t *testing.T) {
	g := NewEchoGuard(0.5)
	now := time.Now()
	frame := make([]byte, 64) // 32 samples ~ 1.33ms of audio at 24kHz
	g.recordPlaybackAt(frame, now)

	// Just inside the frame duration + hangover: still gated.
	if g.allowAt(make([]byte, 64), now.Add(playbackFrameDuration(frame)+defaultHangover-time.Millisecond)) {
		t.Fatal("expected mic gated before the window (frame duration + hangover) elapses")
	}
	// Just past it: open again.
	if !g.allowAt(make([]byte, 64), now.Add(playbackFrameDuration(frame)+defaultHangover+time.Millisecond)) {
		t.Fatal("expected mic allowed once the window elapses")
	}
}

func TestEchoGuard_RecordPlaybackAccumulatesABacklog(t *testing.T) {
	// Regression: a boolean "speaking" flag collapses back to false as soon as
	// the last-recorded frame's own duration elapses, even if several frames
	// were queued back-to-back and the hardware buffer is still playing all
	// of them. The deadline must accumulate across frames recorded before the
	// first one's window has elapsed, the way a hardware playback buffer does.
	g := NewEchoGuard(0.5)
	now := time.Now()
	frame := make([]byte, 64)
	dur := playbackFrameDuration(frame)

	g.recordPlaybackAt(frame, now)
	g.recordPlaybackAt(frame, now.Add(dur/2)) // second frame queued mid-first-frame

	// Total window should now cover roughly 1.5 frame-durations from `now`,
	// not just one: the deadline started at now+dur, and the second call
	// (at now+dur/2, itself before that deadline) extends it by another dur.
	if g.allowAt(make([]byte, 64), now.Add(dur+dur/2-time.Millisecond)) {
		t.Fatal("expected the second queued frame to extend the gate past the first frame's own duration")
	}
}

func TestEchoGuard_AdaptivePlaybackFloor(t *testing.T) {
	// Base threshold is 0.01. With loud playback (RMS ~ 1.0) and CouplingFactor 0.5,
	// effective threshold dynamically rises to ~ 0.5.
	g := NewEchoGuardWithOptions(EchoGuardOptions{
		Threshold:      0.01,
		CouplingFactor: 0.5,
		DecayRate:      0.5,
	})
	now := time.Now()

	loudPlayback := []byte{0xff, 0x7f, 0xff, 0x7f}
	g.recordPlaybackAt(loudPlayback, now)

	// A moderate mic frame (approx amplitude 0.2) should now be gated
	// because it's quieter than the dynamic echo floor (0.5).
	mediumMic := []byte{0x00, 0x20, 0x00, 0x20}
	if g.allowAt(mediumMic, now) {
		t.Fatal("expected medium mic to be gated during loud playback")
	}

	// But a very loud barge-in (approx amplitude 0.9) must still pass.
	shoutingUser := []byte{0x00, 0x75, 0x00, 0x75}
	if !g.allowAt(shoutingUser, now) {
		t.Fatal("expected loud shouting barge-in to pass dynamic gate")
	}
}

func TestEchoGuard_DecayRateEMASurvivesAFrameBoundary(t *testing.T) {
	// Regression for the old SetAgentSpeaking(false) reset, which zeroed
	// playbackRMS between every play() call and made DecayRate dead code
	// outside a test that called RecordPlayback twice by hand without ever
	// crossing the false-edge in between.
	g := NewEchoGuard(0.02)
	loud := make([]byte, 64)
	for i := 0; i < len(loud); i += 2 {
		loud[i], loud[i+1] = 0xff, 0x7f
	}
	now := time.Now()
	for i := 0; i < 2; i++ { // two frames, back to back
		g.recordPlaybackAt(loud, now)
		now = now.Add(playbackFrameDuration(loud))
	}
	g.mu.Lock()
	got := g.playbackRMS
	g.mu.Unlock()
	if got == 0 {
		t.Fatal("playback estimate reset to 0 between frames: decayRate never applies")
	}
}

func TestEchoGuard_Reset(t *testing.T) {
	g := NewEchoGuard(0.5)
	now := time.Now()
	loud := make([]byte, 64)
	for i := 0; i < len(loud); i += 2 {
		loud[i], loud[i+1] = 0xff, 0x7f
	}
	g.recordPlaybackAt(loud, now)
	if g.allowAt(make([]byte, 64), now) {
		t.Fatal("expected mic gated before Reset")
	}
	g.Reset()
	if !g.allowAt(make([]byte, 64), now) {
		t.Fatal("expected Reset to close the gate immediately (barge-in flush)")
	}
	g.mu.Lock()
	pRMS := g.playbackRMS
	g.mu.Unlock()
	if pRMS != 0 {
		t.Fatalf("expected Reset to clear playbackRMS, got %v", pRMS)
	}
}

func TestEchoGuardWithOptions_DefaultsZeroCouplingFactor(t *testing.T) {
	g := NewEchoGuardWithOptions(EchoGuardOptions{Threshold: 0.02, CouplingFactor: 0})
	if g.couplingFactor != 0.5 {
		t.Fatalf("CouplingFactor 0 was not defaulted: got %v, want default 0.5", g.couplingFactor)
	}
}

func TestEchoGuardWithOptions_RejectsDecayRateOfOne(t *testing.T) {
	// DecayRate 1.0 would freeze playbackRMS at its first-ever value forever
	// (1*old + 0*new); it must fall back to the default like any other
	// out-of-range value rather than being silently accepted.
	g := NewEchoGuardWithOptions(EchoGuardOptions{Threshold: 0.02, DecayRate: 1.0})
	if g.decayRate != 0.8 {
		t.Fatalf("DecayRate 1.0 was not rejected: got %v, want default 0.8", g.decayRate)
	}
}

// constantFrame returns a PCM16 mono frame of n samples at a constant
// amplitude, so its RMS is amp (0..1).
func constantFrame(amp float32, n int) []byte {
	v := int16(amp * 32767)
	frame := make([]byte, n*pcm16BytesPerSample)
	for i := 0; i < len(frame); i += pcm16BytesPerSample {
		frame[i], frame[i+1] = byte(uint16(v)), byte(uint16(v)>>8)
	}
	return frame
}

// TestEchoGuard_NaturalTurnEndDoesNotLeakPlaybackRMS is the regression for a
// stale playback estimate carrying across turns. A turn that ends naturally
// (no flush, so no Reset) used to leave playbackRMS at the loud turn's level;
// the first frame of the next, quiet turn was then averaged against it, the
// floor jumped to ~0.4 and a genuine 0.25-RMS barge-in was dropped.
func TestEchoGuard_NaturalTurnEndDoesNotLeakPlaybackRMS(t *testing.T) {
	g := NewEchoGuard(0.02)
	now := time.Now()

	// Loud agent turn at full scale.
	loud := constantFrame(1.0, 480) // 20ms at 24kHz
	for i := 0; i < 10; i++ {
		g.recordPlaybackAt(loud, now)
		now = now.Add(playbackFrameDuration(loud))
	}

	// The turn ends naturally; the user pauses well past the hangover.
	now = now.Add(defaultHangover + time.Second)

	// Quiet agent turn: floor should be ~0.1 (0.2 * 0.5), not the loud turn's.
	quiet := constantFrame(0.2, 480)
	g.recordPlaybackAt(quiet, now)

	echo := constantFrame(0.06, 480) // ~0.3x playback: still gated
	if g.allowAt(echo, now) {
		t.Fatal("expected the quiet turn's own echo to be gated")
	}
	bargeIn := constantFrame(0.25, 480)
	if !g.allowAt(bargeIn, now) {
		g.mu.Lock()
		est := g.playbackRMS
		g.mu.Unlock()
		t.Fatalf("0.25-RMS barge-in dropped on the first quiet frame: playback estimate %.3f leaked from the previous turn", est)
	}
}

func TestEchoGuard_SetAgentSpeakingFalseClosesTheWindow(t *testing.T) {
	g := NewEchoGuard(0.5)
	now := time.Now()
	g.recordPlaybackAt(make([]byte, 6400), now)

	g.SetAgentSpeaking(true) // no-op: playback drives the window
	if g.allowAt(make([]byte, 64), now) {
		t.Fatal("expected quiet mic still gated after SetAgentSpeaking(true)")
	}

	g.SetAgentSpeaking(false)
	if !g.allowAt(make([]byte, 64), now) {
		t.Fatal("expected mic allowed after SetAgentSpeaking(false)")
	}
}
