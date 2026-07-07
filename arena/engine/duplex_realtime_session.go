package engine

import (
	"context"
	"fmt"

	"github.com/AltairaLabs/PromptKit/runtime/audio"
)

// realtimeSessionPlaybackRate is the sample rate stamped on the MediaFrames
// handed to the Session's Sink. The duplex pipeline emits provider-rate PCM16
// (24 kHz for the realtime/TTS providers). Per the audio.Sink contract the rate
// is informational — a hardware or transport sink fixes its own playback rate —
// but stamping the true rate lets a resampling sink do the right thing.
const realtimeSessionPlaybackRate = audio.SampleRate24kHz

// realtimeSessionCaptureBuffer bounds the mic-forwarding channel, matching the
// runtime capture path so backpressure behaves identically.
const realtimeSessionCaptureBuffer = 32

// RunRealtimeSession drives a live duplex (voice) conversation using an
// externally supplied audio.Session for capture and playback, instead of the
// process-local microphone and speaker. It lets a library consumer bridge any
// audio transport — e.g. a WebRTC track — into a headless realtime conversation:
// implement audio.Session (one Source for capture, one Sink for playback) and
// pass it here.
//
// Contract:
//   - The Session's first Source must emit PCM16 mono @ 16 kHz capture frames.
//     Its Frames channel closing (or ctx being canceled) ends the conversation,
//     signaling end-of-user-speech to the provider.
//   - Response audio is written to the Session's first Sink as PCM16 mono frames
//     stamped at 24 kHz; Flush is called on barge-in to drop in-flight playback.
//   - Session.Start must begin capture/playback and return (non-blocking), like
//     PortAudio's; the Session is Closed before this method returns.
//
// This entrypoint pulls in only the pure runtime/audio interfaces — it does not
// import the PortAudio device binding — so a binary that uses it (and avoids the
// voice/TUI packages) stays CGO_ENABLED=0 / statically linkable for scratch
// containers. See RunInteractiveVoice for the underlying channel-based seam.
func (de *DuplexConversationExecutor) RunRealtimeSession(
	ctx context.Context,
	req *ConversationRequest,
	sess audio.Session,
) error {
	mic, play, flush, err := startSessionBridge(ctx, sess)
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close() }()
	return de.RunInteractiveVoice(ctx, req, mic, play, flush)
}

// startSessionBridge validates the Session, starts it, and adapts its
// Source/Sink to the (mic channel, play, flush) triple RunInteractiveVoice
// consumes. Split out from RunRealtimeSession so the adapter is unit-testable
// with a fake Session, independent of the full duplex pipeline.
//
// The caller owns closing the Session (RunRealtimeSession defers it).
func startSessionBridge(
	ctx context.Context,
	sess audio.Session,
) (mic <-chan []byte, play func([]byte), flush func(), err error) {
	if sess == nil {
		return nil, nil, nil, fmt.Errorf("realtime session: audio.Session is nil")
	}
	sources := sess.Sources()
	if len(sources) == 0 {
		return nil, nil, nil, fmt.Errorf("realtime session: audio.Session has no capture Source")
	}
	sinks := sess.Sinks()
	if len(sinks) == 0 {
		return nil, nil, nil, fmt.Errorf("realtime session: audio.Session has no playback Sink")
	}
	if serr := sess.Start(ctx); serr != nil {
		return nil, nil, nil, fmt.Errorf("realtime session: start: %w", serr)
	}
	source, sink := sources[0], sinks[0]

	// Bridge the Source's MediaFrames to the []byte mic channel. Drop on
	// backpressure to preserve capture cadence (matching the hardware observer
	// path); the channel closes when Frames does, which RunInteractiveVoice reads
	// as end-of-user-speech.
	micCh := make(chan []byte, realtimeSessionCaptureBuffer)
	go func() {
		defer close(micCh)
		for frame := range source.Frames() {
			select {
			case micCh <- frame.Data:
			default: // drop on backpressure — preserve capture cadence
			}
		}
	}()

	play = func(frame []byte) {
		sink.Write(audio.MediaFrame{
			Kind:   audio.KindAudio,
			Data:   frame,
			Format: audio.Format{SampleRate: realtimeSessionPlaybackRate, Channels: 1},
		})
	}

	return micCh, play, sink.Flush, nil
}
