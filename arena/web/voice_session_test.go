package web

import (
	"context"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/audio"
	"github.com/gorilla/websocket"
)

// fakeConn implements wsConn for tests. reads is a queue of (messageType, data).
type fakeConn struct {
	mu     sync.Mutex
	reads  chan fakeMsg
	writes []fakeMsg
	closed bool
}
type fakeMsg struct {
	mt   int
	data []byte
}

func newFakeConn() *fakeConn { return &fakeConn{reads: make(chan fakeMsg, 16)} }

func (c *fakeConn) ReadMessage() (int, []byte, error) {
	m, ok := <-c.reads
	if !ok {
		return 0, nil, websocket.ErrCloseSent
	}
	return m.mt, m.data, nil
}
func (c *fakeConn) WriteMessage(mt int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := append([]byte(nil), data...)
	c.writes = append(c.writes, fakeMsg{mt, cp})
	return nil
}

// Close marks the conn closed and closes reads so a pending ReadMessage
// unblocks with an error, letting readPump exit. Guarded against double-close.
func (c *fakeConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	close(c.reads)
	return nil
}

func (c *fakeConn) writeSnapshot() []fakeMsg {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]fakeMsg(nil), c.writes...)
}

func pcm16(samples ...int16) []byte {
	b := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(s))
	}
	return b
}

func TestWSSessionSourceForwardsBinaryFrames(t *testing.T) {
	conn := newFakeConn()
	sess := newWSAudioSession(conn)
	if err := sess.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sess.Close()

	frame := pcm16(1, 2, 3)
	conn.reads <- fakeMsg{websocket.BinaryMessage, frame}

	src := sess.Sources()[0]
	select {
	case mf := <-src.Frames():
		if mf.Kind != audio.KindAudio || string(mf.Data) != string(frame) {
			t.Fatalf("bad frame: %+v", mf)
		}
		if mf.Format.SampleRate != audio.SampleRate16kHz {
			t.Fatalf("want 16k capture, got %d", mf.Format.SampleRate)
		}
	case <-time.After(time.Second):
		t.Fatal("no frame forwarded")
	}
}

func TestWSSessionSinkWritesBinaryAndFlush(t *testing.T) {
	conn := newFakeConn()
	sess := newWSAudioSession(conn)
	_ = sess.Start(context.Background())
	defer sess.Close()

	sink := sess.Sinks()[0]
	sink.Write(audio.MediaFrame{Kind: audio.KindAudio, Data: pcm16(9, 9), Format: audio.Format{SampleRate: audio.SampleRate24kHz, Channels: 1}})
	sink.Flush()

	// Expect one binary write (the PCM) and at least one text write containing "flush".
	var sawBinary, sawFlush bool
	for _, w := range conn.writeSnapshot() {
		if w.mt == websocket.BinaryMessage && string(w.data) == string(pcm16(9, 9)) {
			sawBinary = true
		}
		if w.mt == websocket.TextMessage && string(w.data) == string(voiceFlushMsg()) {
			sawFlush = true
		}
	}
	if !sawBinary || !sawFlush {
		t.Fatalf("sawBinary=%v sawFlush=%v", sawBinary, sawFlush)
	}
}

func TestWSSessionCloseClosesFrames(t *testing.T) {
	conn := newFakeConn()
	sess := newWSAudioSession(conn)
	if err := sess.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	frame := pcm16(1, 2, 3)
	conn.reads <- fakeMsg{websocket.BinaryMessage, frame}

	src := sess.Sources()[0]
	select {
	case <-src.Frames():
	case <-time.After(time.Second):
		t.Fatal("no frame forwarded")
	}

	if err := sess.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case _, ok := <-src.Frames():
		if ok {
			t.Fatal("expected frames channel to be closed after Close, got a value")
		}
	case <-time.After(time.Second):
		t.Fatal("frames channel was not closed within timeout")
	}
}

func TestWSSessionMuteDropsFrames(t *testing.T) {
	conn := newFakeConn()
	sess := newWSAudioSession(conn)
	if err := sess.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sess.Close()

	src := sess.Sources()[0]

	conn.reads <- fakeMsg{websocket.TextMessage, []byte(`{"type":"mute","muted":true}`)}
	conn.reads <- fakeMsg{websocket.BinaryMessage, pcm16(1, 2, 3)}

	select {
	case mf := <-src.Frames():
		t.Fatalf("expected no frame while muted, got %+v", mf)
	case <-time.After(200 * time.Millisecond):
	}

	conn.reads <- fakeMsg{websocket.TextMessage, []byte(`{"type":"mute","muted":false}`)}
	frame := pcm16(4, 5, 6)
	conn.reads <- fakeMsg{websocket.BinaryMessage, frame}

	select {
	case mf := <-src.Frames():
		if string(mf.Data) != string(frame) {
			t.Fatalf("bad frame: %+v", mf)
		}
	case <-time.After(time.Second):
		t.Fatal("no frame forwarded after unmute")
	}
}

func TestWSSinkEmitsSpeakingThenListening(t *testing.T) {
	conn := newFakeConn()
	clock := &fakeClock{t: time.Unix(0, 0)}
	sink := newWSSink(conn, clock.now)

	sink.Write(audio.MediaFrame{Data: pcm16(1)}) // first write → speaking
	clock.advance(sinkQuietGap + time.Millisecond)
	sink.tick() // quiet detector → listening

	var sawSpeaking, sawListening bool
	for _, w := range conn.writeSnapshot() {
		if w.mt == websocket.TextMessage {
			switch string(w.data) {
			case string(voiceStateMsg(voiceStateSpeaking)):
				sawSpeaking = true
			case string(voiceStateMsg(voiceStateListening)):
				sawListening = true
			}
		}
	}
	if !sawSpeaking || !sawListening {
		t.Fatalf("sawSpeaking=%v sawListening=%v", sawSpeaking, sawListening)
	}
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time          { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) advance(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func TestWSSourceCloseIsSafe(t *testing.T) {
	conn := newFakeConn()
	sess := newWSAudioSession(conn)
	if err := sess.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer sess.Close()

	src := sess.Sources()[0]
	if err := src.Close(); err != nil {
		t.Fatalf("source close: %v", err)
	}

	select {
	case _, ok := <-src.Frames():
		if ok {
			t.Fatal("expected frames channel to be closed, got a value")
		}
	case <-time.After(time.Second):
		t.Fatal("frames channel was not closed within timeout")
	}
}
