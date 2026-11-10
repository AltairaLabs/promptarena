package audio

import (
	"errors"
	"testing"
)

// withPlaybackBackend registers b for the duration of the test.
func withPlaybackBackend(t *testing.T, b PlaybackBackend) {
	t.Helper()
	prev := registeredPlaybackBackend()
	RegisterPlaybackBackend(b)
	t.Cleanup(func() { RegisterPlaybackBackend(prev) })
}

func TestNewLocalSink_UsesRegisteredBackend(t *testing.T) {
	fake := &fakeOtoContext{}
	var gotRate, gotChannels int
	withPlaybackBackend(t, func(rate, channels int) (PlaybackContext, error) {
		gotRate, gotChannels = rate, channels
		return fake, nil
	})

	sink, err := NewLocalSink(LocalSinkConfig{Rate: Rate48k})
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}
	defer sink.Close()

	if sink.noop {
		t.Fatal("sink is a no-op; want it playing through the registered backend")
	}
	if gotRate != Rate48k || gotChannels != channelStereo {
		t.Fatalf("backend opened with rate=%d channels=%d, want %d/%d", gotRate, gotChannels, Rate48k, channelStereo)
	}
	if fake.reader == nil {
		t.Fatal("backend context never got a player")
	}
}

func TestNewLocalSink_NoBackendIsNoOp(t *testing.T) {
	withPlaybackBackend(t, nil)

	sink, err := NewLocalSink(LocalSinkConfig{Rate: Rate24k})
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}
	defer sink.Close()

	if !sink.noop {
		t.Fatal("sink is playing with no backend registered; want a no-op")
	}
}

func TestNewLocalSink_BackendErrorIsNoOp(t *testing.T) {
	withPlaybackBackend(t, func(_, _ int) (PlaybackContext, error) {
		return nil, errors.New("no audio device")
	})

	sink, err := NewLocalSink(LocalSinkConfig{Rate: Rate24k})
	if err != nil {
		t.Fatalf("NewLocalSink: %v", err)
	}
	defer sink.Close()

	if !sink.noop {
		t.Fatal("sink is playing after the backend failed; want a no-op")
	}
}
