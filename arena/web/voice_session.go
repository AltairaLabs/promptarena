package web

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/AltairaLabs/PromptKit/runtime/audio"
)

// wsConn is the slice of *websocket.Conn this package needs, so tests can fake it.
type wsConn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// wsAudioSession adapts a WebSocket into an audio.Session: inbound binary frames
// become 16 kHz capture MediaFrames; the sink writes 24 kHz playback frames back
// out as binary and forwards Flush as a control frame.
type wsAudioSession struct {
	conn   wsConn
	src    *wsSource
	sink   *wsSink
	cancel context.CancelFunc
	muted  *atomicBool
}

// captureFrameBuffer is the inbound capture-frame queue depth. Sized to absorb
// short bursts; sends beyond it are dropped rather than blocking the read pump.
const captureFrameBuffer = 32

// quietTickInterval polls the sink for end-of-speech at twice the quiet-gap
// rate, so a gap is detected within roughly one tick of elapsing.
const quietTickInterval = sinkQuietGap / 2

func newWSAudioSession(conn wsConn) *wsAudioSession {
	return &wsAudioSession{
		conn:  conn,
		src:   &wsSource{frames: make(chan audio.MediaFrame, captureFrameBuffer)},
		sink:  newWSSink(conn, time.Now),
		muted: &atomicBool{},
	}
}

// Start begins the read pump and quiet detector, and returns immediately.
// Both stop when ctx is canceled or Close is called.
func (s *wsAudioSession) Start(ctx context.Context) error {
	ctx, s.cancel = context.WithCancel(ctx)
	go s.readPump()
	go s.runQuietDetector(ctx)
	return nil
}

func (s *wsAudioSession) runQuietDetector(ctx context.Context) {
	t := time.NewTicker(quietTickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.sink.stopCh:
			return
		case <-t.C:
			s.sink.tick()
		}
	}
}

// Sources returns the single capture source fed by inbound WebSocket frames.
func (s *wsAudioSession) Sources() []audio.Source { return []audio.Source{s.src} }

// Sinks returns the single playback sink that writes frames back to the socket.
func (s *wsAudioSession) Sinks() []audio.Sink { return []audio.Sink{s.sink} }

// Close cancels the session's goroutines, stops the sink and closes the socket.
func (s *wsAudioSession) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.sink.stop()
	return s.conn.Close()
}

// readPump reads WS messages: binary → capture frames (unless muted); text →
// control (mute toggle). It closes the source channel when the socket ends,
// which RunInteractiveVoice reads as end-of-user-speech. Cancellation arrives
// by way of the socket closing, so this takes no context.
func (s *wsAudioSession) readPump() {
	defer s.src.closeOnce()
	for {
		mt, data, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		switch mt {
		case websocket.BinaryMessage:
			if s.muted.get() {
				continue
			}
			frame := audio.MediaFrame{
				Kind:   audio.KindAudio,
				Data:   data,
				Format: audio.Format{SampleRate: audio.SampleRate16kHz, Channels: 1},
			}
			s.src.send(frame)
		case websocket.TextMessage:
			if c, err := parseVoiceControl(data); err == nil && c.Type == "mute" {
				s.muted.set(c.Muted)
			}
		}
	}
}

// --- Source ---

type wsSource struct {
	frames chan audio.MediaFrame
	mu     sync.Mutex
	closed bool
}

// Frames exposes the capture channel consumed by the duplex executor.
func (s *wsSource) Frames() <-chan audio.MediaFrame { return s.frames }

// Kind reports that this source carries audio.
func (s *wsSource) Kind() audio.MediaKind { return audio.KindAudio }

// Close closes the capture channel; safe to call more than once.
func (s *wsSource) Close() error { s.closeOnce(); return nil }

// send delivers a frame non-blockingly, dropping on backpressure. Serialized
// with closeOnce so a send can never race the channel close.
func (s *wsSource) send(f audio.MediaFrame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.frames <- f:
	default: // drop on backpressure — preserve capture cadence
	}
}

func (s *wsSource) closeOnce() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.frames)
}

// --- Sink ---

const sinkQuietGap = 200 * time.Millisecond

type wsSink struct {
	conn      wsConn
	now       func() time.Time
	mu        sync.Mutex
	speaking  bool
	lastWrite time.Time
	stopCh    chan struct{}
	stopOnce  sync.Once
}

func newWSSink(conn wsConn, now func() time.Time) *wsSink {
	s := &wsSink{conn: conn, now: now, stopCh: make(chan struct{})}
	return s
}

func (s *wsSink) Write(fr audio.MediaFrame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.speaking {
		s.speaking = true
		_ = s.conn.WriteMessage(websocket.TextMessage, voiceStateMsg(voiceStateSpeaking))
	}
	s.lastWrite = s.now()
	// Hold the lock across the binary write too: gorilla's Conn allows only one
	// concurrent writer, and tick()/Flush() write control frames under the same lock.
	_ = s.conn.WriteMessage(websocket.BinaryMessage, fr.Data)
}

// tick flips speaking→listening once the quiet gap has elapsed. Called on a
// timer by the session (see runQuietDetector); exposed for tests.
func (s *wsSink) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.speaking && s.now().Sub(s.lastWrite) >= sinkQuietGap {
		s.speaking = false
		_ = s.conn.WriteMessage(websocket.TextMessage, voiceStateMsg(voiceStateListening))
	}
}

// Flush forwards an end-of-utterance marker to the browser as a control frame.
func (s *wsSink) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.WriteMessage(websocket.TextMessage, voiceFlushMsg())
}

// Kind reports that this sink carries audio.
func (s *wsSink) Kind() audio.MediaKind { return audio.KindAudio }

// Close is a no-op: the socket is owned and closed by the session.
func (s *wsSink) Close() error { return nil }
func (s *wsSink) stop()        { s.stopOnce.Do(func() { close(s.stopCh) }) }

// --- tiny atomic bool (avoid importing sync/atomic bool churn) ---

type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (a *atomicBool) get() bool  { a.mu.Lock(); defer a.mu.Unlock(); return a.v }
func (a *atomicBool) set(v bool) { a.mu.Lock(); a.v = v; a.mu.Unlock() }
