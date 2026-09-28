package voice

import (
	"context"
	"testing"
	"time"
)

type fakeIO struct {
	capture chan []byte
	played  [][]byte
	started bool
	flushed int
}

func (f *fakeIO) Start(context.Context) error  { f.started = true; return nil }
func (f *fakeIO) CaptureChunks() <-chan []byte { return f.capture }
func (f *fakeIO) Play(b []byte)                { f.played = append(f.played, b) }
func (f *fakeIO) Flush()                       { f.flushed++ }
func (f *fakeIO) Close() error                 { return nil }

func TestDriver_PipesMicToRunnerAndPlaysOutput(t *testing.T) {
	io := &fakeIO{capture: make(chan []byte, 2)}
	// runner echoes each mic frame to play, then returns when mic closes.
	runner := func(ctx context.Context, mic <-chan []byte, play func([]byte), _ func()) error {
		for f := range mic {
			play(f)
		}
		return nil
	}
	d := NewDriver(io, runner, nil)

	io.capture <- []byte{1, 2}
	close(io.capture)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !io.started {
		t.Fatal("expected AudioIO.Start to be called")
	}
	if len(io.played) != 1 {
		t.Fatalf("expected 1 played frame, got %d", len(io.played))
	}
}

func TestDriver_ReportsLevelsViaTapAndPlay(t *testing.T) {
	io := &fakeIO{capture: make(chan []byte, 2)}
	var userLevels, agentLevels []float32
	onLevel := func(user, agent float32) {
		userLevels = append(userLevels, user)
		agentLevels = append(agentLevels, agent)
	}
	runner := func(ctx context.Context, mic <-chan []byte, play func([]byte), _ func()) error {
		for f := range mic {
			play(f)
		}
		return nil
	}
	d := NewDriver(io, runner, onLevel)

	frame := []byte{0x00, 0x40, 0x00, 0x40} // non-silent PCM
	io.capture <- frame
	close(io.capture)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// tapLevels should have emitted one user level > 0
	if len(userLevels) == 0 {
		t.Fatal("expected at least one user level callback")
	}
	if userLevels[0] <= 0 {
		t.Fatalf("expected positive user level, got %v", userLevels[0])
	}
	// play callback should have emitted one agent level > 0
	if len(agentLevels) == 0 {
		t.Fatal("expected at least one agent level callback")
	}
	if agentLevels[len(agentLevels)-1] <= 0 {
		t.Fatalf("expected positive agent level, got %v", agentLevels[len(agentLevels)-1])
	}
}

func TestRMS_SilenceIsZero(t *testing.T) {
	if got := rms(make([]byte, 64)); got != 0 {
		t.Fatalf("silence rms = %v, want 0", got)
	}
}

func TestRMS_NonSilenceIsPositive(t *testing.T) {
	frame := []byte{0x00, 0x40, 0x00, 0x40} // two samples ~0x4000
	if got := rms(frame); got <= 0 {
		t.Fatalf("expected positive rms, got %v", got)
	}
}

func TestDriverWithGuard_DropsQuietMicWhilePlaybackWindowOpen(t *testing.T) {
	io := &fakeIO{capture: make(chan []byte, 2)}
	var received [][]byte
	loud := make([]byte, 6400) // ~200ms at 24kHz mono PCM16, loud enough to open the gate
	for i := 0; i < len(loud); i += 2 {
		loud[i], loud[i+1] = 0xff, 0x7f
	}
	runner := func(ctx context.Context, mic <-chan []byte, play func([]byte), _ func()) error {
		play(loud) // agent speaks: opens the gate via RecordPlayback, same path Driver.Run uses for real audio
		// Send the mic frame only after play() returns, so RecordPlayback has
		// already run before the tap goroutine can read it -- otherwise this
		// races against tapLevels the same way a boolean flag set after Run()
		// starts would.
		io.capture <- make([]byte, 64) // silence — should be gated while the window from `loud` is open
		close(io.capture)
		for f := range mic {
			received = append(received, f)
		}
		return nil
	}
	guard := NewEchoGuard(0.5)
	d := NewDriverWithGuard(io, runner, nil, guard)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(received) != 0 {
		t.Fatalf("expected quiet mic frame to be dropped, got %d frames", len(received))
	}
}

// TestDriverWithGuard_AdaptiveFloorEngagesOnBufferedHardware is the regression
// for the bug this PR was written to fix: fakeIO.Play returns immediately, the
// way real buffered hardware drivers do, so a boolean "speaking" flag toggled
// synchronously around the Play call closes again before the mic frame it was
// meant to gate ever arrives. RecordPlayback must open a window sized to the
// frame's own audible duration instead, independent of how long Play took.
func TestDriverWithGuard_AdaptiveFloorEngagesOnBufferedHardware(t *testing.T) {
	io := &fakeIO{capture: make(chan []byte, 4)}
	var received [][]byte

	loud := make([]byte, 64)
	for i := 0; i < len(loud); i += 2 {
		loud[i], loud[i+1] = 0xff, 0x7f // ~full scale -> dynamic floor ~0.5
	}
	echo := make([]byte, 64)
	for i := 0; i < len(echo); i += 2 {
		echo[i], echo[i+1] = 0x00, 0x20 // ~0.25: under the dynamic floor, over the 0.02 base
	}

	runner := func(ctx context.Context, mic <-chan []byte, play func([]byte), _ func()) error {
		play(loud)         // agent speaks
		io.capture <- echo // its echo reaches the mic
		close(io.capture)
		for f := range mic {
			received = append(received, f)
		}
		return nil
	}

	d := NewDriverWithGuard(io, runner, nil, NewEchoGuard(0.02))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(received) != 0 {
		t.Fatalf("echo frame reached the runner: the adaptive floor did not engage (received %d frames)", len(received))
	}
}

// TestDriverWithGuard_FlushResetsTheGate covers barge-in: dropping queued
// playback (Interrupt element) must also close the gate immediately, or the
// mic stays suppressed for audio that will now never actually sound.
func TestDriverWithGuard_FlushResetsTheGate(t *testing.T) {
	io := &fakeIO{capture: make(chan []byte, 2)}
	var received [][]byte
	loud := make([]byte, 6400) // ~200ms window
	for i := 0; i < len(loud); i += 2 {
		loud[i], loud[i+1] = 0xff, 0x7f
	}
	runner := func(ctx context.Context, mic <-chan []byte, play func([]byte), flush func()) error {
		play(loud) // opens the gate
		flush()    // barge-in: drop queued playback and reset the gate
		// Sent only after flush(), so Reset has already run before the tap
		// goroutine can read it -- see the note in the sibling test above.
		io.capture <- make([]byte, 64) // quiet frame — must pass now that flush reset the gate
		close(io.capture)
		for f := range mic {
			received = append(received, f)
		}
		return nil
	}
	guard := NewEchoGuard(0.5)
	d := NewDriverWithGuard(io, runner, nil, guard)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if io.flushed != 1 {
		t.Fatalf("expected AudioIO.Flush to be called once, got %d", io.flushed)
	}
	if len(received) != 1 {
		t.Fatalf("expected the mic frame to pass after flush reset the gate, got %d frames", len(received))
	}
}
