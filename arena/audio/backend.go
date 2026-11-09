package audio

import "sync"

// PlaybackBackend opens the host audio device for a stereo s16le stream at
// rate. LocalSink calls it when it is not headless.
type PlaybackBackend func(rate, channels int) (PlaybackContext, error)

var (
	playbackMu      sync.RWMutex
	playbackBackend PlaybackBackend
)

// RegisterPlaybackBackend sets the backend LocalSink uses for host playback.
//
// This package does not link an audio library itself: the oto backend lives in
// arena/audio/otobackend, which registers itself when a binary imports it. oto
// depends on purego, and purego makes a CGO_ENABLED=0 Linux binary dynamically
// linked against glibc, so a binary that imports arena/engine without it stays
// static (distroless/static, Alpine, scratch). Without a backend, LocalSink is
// a no-op, as it is when no audio device is available.
func RegisterPlaybackBackend(b PlaybackBackend) {
	playbackMu.Lock()
	defer playbackMu.Unlock()
	playbackBackend = b
}

func registeredPlaybackBackend() PlaybackBackend {
	playbackMu.RLock()
	defer playbackMu.RUnlock()
	return playbackBackend
}
