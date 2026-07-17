package web

import (
	"context"
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/audio"
	"github.com/gorilla/websocket"
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

func newWSAudioSession(conn wsConn) *wsAudioSession {
	muted := &atomicBool{}
	return &wsAudioSession{
		conn:  conn,
		src:   &wsSource{frames: make(chan audio.MediaFrame, 32)},
		sink:  newWSSink(conn),
		muted: muted,
	}
}

func (s *wsAudioSession) Start(ctx context.Context) error {
	ctx, s.cancel = context.WithCancel(ctx)
	go s.readPump(ctx)
	return nil
}

func (s *wsAudioSession) Sources() []audio.Source { return []audio.Source{s.src} }
func (s *wsAudioSession) Sinks() []audio.Sink     { return []audio.Sink{s.sink} }

func (s *wsAudioSession) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.sink.stop()
	return s.conn.Close()
}

// readPump reads WS messages: binary → capture frames (unless muted); text →
// control (mute toggle). It closes the source channel when the socket ends,
// which RunInteractiveVoice reads as end-of-user-speech.
func (s *wsAudioSession) readPump(ctx context.Context) {
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
			select {
			case s.src.frames <- frame:
			case <-ctx.Done():
				return
			default: // drop on backpressure, preserve capture cadence
			}
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
	once   sync.Once
}

func (s *wsSource) Frames() <-chan audio.MediaFrame { return s.frames }
func (s *wsSource) Kind() audio.MediaKind           { return audio.KindAudio }
func (s *wsSource) Close() error                    { s.closeOnce(); return nil }
func (s *wsSource) closeOnce()                      { s.once.Do(func() { close(s.frames) }) }

// --- Sink ---

type wsSink struct {
	conn wsConn
	mu   sync.Mutex
}

func newWSSink(conn wsConn) *wsSink { return &wsSink{conn: conn} }

func (s *wsSink) Write(fr audio.MediaFrame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.WriteMessage(websocket.BinaryMessage, fr.Data)
}

func (s *wsSink) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.WriteMessage(websocket.TextMessage, voiceFlushMsg())
}

func (s *wsSink) Kind() audio.MediaKind { return audio.KindAudio }
func (s *wsSink) Close() error          { return nil }
func (s *wsSink) stop()                 {}

// --- tiny atomic bool (avoid importing sync/atomic bool churn) ---

type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (a *atomicBool) get() bool  { a.mu.Lock(); defer a.mu.Unlock(); return a.v }
func (a *atomicBool) set(v bool) { a.mu.Lock(); a.v = v; a.mu.Unlock() }
