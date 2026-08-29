package voice

import "testing"

func TestEchoGuard_AllowsWhenAgentSilent(t *testing.T) {
	g := NewEchoGuard(0.5)
	if !g.Allow(make([]byte, 64)) {
		t.Fatal("expected mic allowed when agent not speaking")
	}
}

func TestEchoGuard_GatesQuietMicWhileAgentSpeaks(t *testing.T) {
	g := NewEchoGuard(0.5)
	g.SetAgentSpeaking(true)
	if g.Allow(make([]byte, 64)) { // silence < threshold
		t.Fatal("expected quiet mic gated while agent speaks")
	}
}

func TestEchoGuard_AllowsLoudBargeInWhileAgentSpeaks(t *testing.T) {
	g := NewEchoGuard(0.001) // very low threshold
	g.SetAgentSpeaking(true)
	loud := []byte{0xff, 0x7f, 0xff, 0x7f} // near max amplitude
	if !g.Allow(loud) {
		t.Fatal("expected loud barge-in to pass the gate")
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
	g.SetAgentSpeaking(true)

	loudPlayback := []byte{0xff, 0x7f, 0xff, 0x7f}
	g.RecordPlayback(loudPlayback)

	// A moderate mic frame (approx amplitude 0.2) should now be gated
	// because it's quieter than the dynamic echo floor (0.5).
	mediumMic := []byte{0x00, 0x20, 0x00, 0x20}
	if g.Allow(mediumMic) {
		t.Fatal("expected medium mic to be gated during loud playback")
	}

	// But a very loud barge-in (approx amplitude 0.9) must still pass.
	shoutingUser := []byte{0x00, 0x75, 0x00, 0x75}
	if !g.Allow(shoutingUser) {
		t.Fatal("expected loud shouting barge-in to pass dynamic gate")
	}
}
