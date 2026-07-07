package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/audio"
)

// fakeSource is an in-memory audio.Source whose frames the test drives.
type fakeSource struct{ ch chan audio.MediaFrame }

func (f *fakeSource) Frames() <-chan audio.MediaFrame { return f.ch }
func (f *fakeSource) Kind() audio.MediaKind           { return audio.KindAudio }
func (f *fakeSource) Close() error                    { close(f.ch); return nil }

// fakeSink records what the bridge writes and flushes.
type fakeSink struct {
	mu      sync.Mutex
	writes  []audio.MediaFrame
	flushes int
}

func (f *fakeSink) Write(fr audio.MediaFrame) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, fr)
}
func (f *fakeSink) Flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flushes++
}
func (f *fakeSink) Kind() audio.MediaKind { return audio.KindAudio }
func (f *fakeSink) Close() error          { return nil }

func (f *fakeSink) snapshot() ([]audio.MediaFrame, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]audio.MediaFrame(nil), f.writes...), f.flushes
}

// fakeSession is a minimal audio.Session with configurable Source/Sink lists and
// Start error, for exercising startSessionBridge without real hardware.
type fakeSession struct {
	sources  []audio.Source
	sinks    []audio.Sink
	startErr error
	started  bool
}

func (s *fakeSession) Start(context.Context) error { s.started = true; return s.startErr }
func (s *fakeSession) Sources() []audio.Source     { return s.sources }
func (s *fakeSession) Sinks() []audio.Sink         { return s.sinks }
func (s *fakeSession) Close() error                { return nil }

func TestStartSessionBridge_Validation(t *testing.T) {
	ctx := context.Background()
	src := &fakeSource{ch: make(chan audio.MediaFrame)}
	snk := &fakeSink{}

	tests := []struct {
		name string
		sess audio.Session
	}{
		{"nil session", nil},
		{"no source", &fakeSession{sources: nil, sinks: []audio.Sink{snk}}},
		{"no sink", &fakeSession{sources: []audio.Source{src}, sinks: nil}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, err := startSessionBridge(ctx, tc.sess); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestStartSessionBridge_StartErrorPropagates(t *testing.T) {
	sess := &fakeSession{
		sources:  []audio.Source{&fakeSource{ch: make(chan audio.MediaFrame)}},
		sinks:    []audio.Sink{&fakeSink{}},
		startErr: context.DeadlineExceeded,
	}
	if _, _, _, err := startSessionBridge(context.Background(), sess); err == nil {
		t.Fatal("expected Start error to propagate, got nil")
	}
}

func TestStartSessionBridge_BridgesCaptureAndPlayback(t *testing.T) {
	ctx := context.Background()
	src := &fakeSource{ch: make(chan audio.MediaFrame, 1)}
	snk := &fakeSink{}
	sess := &fakeSession{sources: []audio.Source{src}, sinks: []audio.Sink{snk}}

	mic, play, flush, err := startSessionBridge(ctx, sess)
	if err != nil {
		t.Fatal(err)
	}
	if !sess.started {
		t.Error("Start was not called")
	}

	// Capture: a Source frame surfaces on the mic channel as its raw bytes.
	want := []byte{1, 2, 3, 4}
	src.ch <- audio.MediaFrame{Kind: audio.KindAudio, Data: want}
	select {
	case got := <-mic:
		if string(got) != string(want) {
			t.Errorf("mic frame = %v, want %v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mic frame")
	}

	// Playback: play() writes to the Sink stamped at the 24 kHz playback rate.
	play([]byte{9, 9})
	flush()
	writes, flushes := snk.snapshot()
	if len(writes) != 1 {
		t.Fatalf("sink writes = %d, want 1", len(writes))
	}
	if writes[0].Format.SampleRate != realtimeSessionPlaybackRate {
		t.Errorf("write rate = %d, want %d", writes[0].Format.SampleRate, realtimeSessionPlaybackRate)
	}
	if writes[0].Format.Channels != 1 {
		t.Errorf("write channels = %d, want 1", writes[0].Format.Channels)
	}
	if flushes != 1 {
		t.Errorf("flushes = %d, want 1", flushes)
	}

	// Closing the Source closes the mic channel (end-of-user-speech).
	_ = src.Close()
	select {
	case _, open := <-mic:
		if open {
			// drain any late frame, then confirm closed
			<-mic
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mic channel not closed after Source closed")
	}
}
