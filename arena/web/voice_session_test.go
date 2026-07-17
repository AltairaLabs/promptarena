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
func (c *fakeConn) Close() error { c.mu.Lock(); c.closed = true; c.mu.Unlock(); return nil }

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
