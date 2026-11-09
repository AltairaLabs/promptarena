package logging

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInterceptor_SendFunc_DeliversLog verifies a log emitted through the
// interceptor is delivered to the injected send func as a logging.Msg.
func TestInterceptor_SendFunc_DeliversLog(t *testing.T) {
	var mu sync.Mutex
	var got []Msg
	send := func(m tea.Msg) {
		if lm, ok := m.(Msg); ok {
			mu.Lock()
			got = append(got, lm)
			mu.Unlock()
		}
	}
	ic, err := NewInterceptor(slog.NewTextHandler(io.Discard, nil), send, "", true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ic.Close() })

	slog.New(ic).Info("hello-tui")

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 1 && got[0].Message == "hello-tui"
	}, time.Second, 5*time.Millisecond)
}

func TestNewInterceptor(t *testing.T) {
	t.Run("without log file", func(t *testing.T) {
		// Not suppressed: each record goes straight to the original handler
		// (stderr) and to the TUI, and no file is opened.
		var stderr bytes.Buffer
		got := make(chan Msg, 1)
		interceptor, err := NewInterceptor(slog.NewTextHandler(&stderr, nil), func(m tea.Msg) {
			if lm, ok := m.(Msg); ok {
				got <- lm
			}
		}, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { _ = interceptor.Close() })

		slog.New(interceptor).Warn("to-stderr")
		assert.Contains(t, stderr.String(), "level=WARN msg=to-stderr")
		select {
		case m := <-got:
			assert.Equal(t, "WARN", m.Level)
			assert.Equal(t, "to-stderr", m.Message)
		case <-time.After(time.Second):
			t.Fatal("log never reached the TUI send func")
		}
		assert.Nil(t, interceptor.logFile)
	})

	t.Run("with log file", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "test.log")

		interceptor, err := NewInterceptor(slog.NewTextHandler(io.Discard, nil), nil, logPath, false)
		require.NoError(t, err)
		slog.New(interceptor).Info("to-file")
		require.NoError(t, interceptor.Close())

		// The file is created owner-only and receives the record.
		info, err := os.Stat(logPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(logFilePermissions), info.Mode().Perm())
		data, err := os.ReadFile(logPath)
		require.NoError(t, err)
		assert.Contains(t, string(data), "level=INFO msg=to-file")
	})

	t.Run("invalid log file path", func(t *testing.T) {
		interceptor, err := NewInterceptor(slog.NewTextHandler(io.Discard, nil), nil, "/nonexistent/dir/test.log", false)
		assert.ErrorContains(t, err, "failed to open log file")
		assert.Nil(t, interceptor)
	})
}

func TestInterceptor_Close(t *testing.T) {
	t.Run("with no log file", func(t *testing.T) {
		var sent []Msg
		var mu sync.Mutex
		interceptor, err := NewInterceptor(slog.NewTextHandler(io.Discard, nil), func(m tea.Msg) {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, m.(Msg))
		}, "", false)
		require.NoError(t, err)

		require.NoError(t, interceptor.Close())
		// Close is idempotent, and a log after Close is dropped rather than
		// sent on the closed worker channel.
		require.NoError(t, interceptor.Close())
		require.NoError(t, slog.New(interceptor).Handler().Handle(context.Background(),
			slog.NewRecord(time.Now(), slog.LevelInfo, "after-close", 0)))
		mu.Lock()
		defer mu.Unlock()
		assert.Empty(t, sent)
	})

	t.Run("with log file", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "test.log")
		interceptor, err := NewInterceptor(slog.NewTextHandler(io.Discard, nil), nil, logPath, false)
		require.NoError(t, err)

		require.NoError(t, interceptor.Close())
		_, err = interceptor.logFile.WriteString("x")
		assert.ErrorIs(t, err, os.ErrClosed, "Close must close the log file")
	})
}

func TestInterceptor_Enabled(t *testing.T) {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	send := func(tea.Msg) {}

	interceptor, err := NewInterceptor(handler, send, "", false)
	require.NoError(t, err)

	ctx := context.Background()

	// Should be enabled for INFO and above
	assert.True(t, interceptor.Enabled(ctx, slog.LevelInfo))
	assert.True(t, interceptor.Enabled(ctx, slog.LevelWarn))
	assert.True(t, interceptor.Enabled(ctx, slog.LevelError))

	// Should be disabled for DEBUG (below INFO)
	assert.False(t, interceptor.Enabled(ctx, slog.LevelDebug))
}

func TestInterceptor_Handle(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	handler := slog.NewTextHandler(os.Stderr, nil)
	// Don't pass a program for this test - we're testing file writing
	interceptor, err := NewInterceptor(handler, nil, logPath, false)
	require.NoError(t, err)
	defer interceptor.Close()

	ctx := context.Background()

	// Create a log record
	now := time.Now()
	record := slog.Record{
		Time:    now,
		Message: "test message",
		Level:   slog.LevelInfo,
	}

	// Handle the record
	err = interceptor.Handle(ctx, record)
	assert.NoError(t, err)

	// Check log file was written
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)

	// Check for timestamp format (new slog format)
	assert.Contains(t, string(content), "level=INFO")
	assert.Contains(t, string(content), "test message")
}

func TestInterceptor_WithAttrs(t *testing.T) {
	handler := slog.NewTextHandler(os.Stderr, nil)
	send := func(tea.Msg) {}

	interceptor, err := NewInterceptor(handler, send, "", false)
	require.NoError(t, err)

	attrs := []slog.Attr{slog.String("key", "value")}
	newHandler := interceptor.WithAttrs(attrs)

	assert.NotNil(t, newHandler)
	// WithAttrs returns a sub-handler that shares the parent's TUI worker
	// and file output but applies its own attribute scoping.
	sub, ok := newHandler.(*subInterceptor)
	require.True(t, ok)
	assert.Same(t, interceptor, sub.parent)
}

func TestInterceptor_WithGroup(t *testing.T) {
	handler := slog.NewTextHandler(os.Stderr, nil)
	send := func(tea.Msg) {}

	interceptor, err := NewInterceptor(handler, send, "", false)
	require.NoError(t, err)

	newHandler := interceptor.WithGroup("test-group")

	assert.NotNil(t, newHandler)
	sub, ok := newHandler.(*subInterceptor)
	require.True(t, ok)
	assert.Same(t, interceptor, sub.parent)
}

func TestLevelToString(t *testing.T) {
	tests := []struct {
		level    slog.Level
		expected string
	}{
		{slog.LevelDebug, "DEBUG"},
		{slog.LevelInfo, "INFO"},
		{slog.LevelWarn, "WARN"},
		{slog.LevelError, "ERROR"},
		{slog.Level(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := levelToString(tt.level)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMsg(t *testing.T) {
	now := time.Now()

	msg := Msg{
		Timestamp: now,
		Level:     "INFO",
		Message:   "test message",
	}

	assert.Equal(t, now, msg.Timestamp)
	assert.Equal(t, "INFO", msg.Level)
	assert.Equal(t, "test message", msg.Message)
}

type capturingHandler struct {
	records []slog.Record
}

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingHandler) Handle(ctx context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}

func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *capturingHandler) WithGroup(string) slog.Handler { return h }

func TestInterceptor_FlushBuffer(t *testing.T) {
	handler := &capturingHandler{}

	interceptor, err := NewInterceptor(handler, nil, "", true)
	require.NoError(t, err)

	record := slog.Record{
		Time:    time.Now(),
		Message: "buffered",
		Level:   slog.LevelDebug,
	}

	err = interceptor.Handle(context.Background(), record)
	require.NoError(t, err)

	// Should be buffered, not forwarded yet
	assert.Len(t, handler.records, 0)

	interceptor.FlushBuffer()

	// Second flush should be no-op
	assert.Len(t, handler.records, 1)
	assert.Equal(t, "buffered", handler.records[0].Message)

	interceptor.FlushBuffer()
	assert.Len(t, handler.records, 1)
}

// TestSubInterceptor_ScopedAttrsSurviveFileAndFlush pins that a logger built
// with slog.With / WithGroup keeps its scoped attributes on every output, not
// only on the live stderr path: the verbose log file and the stderr replay of
// logs buffered while the TUI owned the screen must carry them too.
func TestSubInterceptor_ScopedAttrsSurviveFileAndFlush(t *testing.T) {
	t.Run("log file", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "scoped.log")
		ic, err := NewInterceptor(slog.NewTextHandler(io.Discard, nil), nil, logPath, false)
		require.NoError(t, err)

		slog.New(ic).With("run", "run-7").WithGroup("stage").Info("file-bound", "name", "provider")
		require.NoError(t, ic.Close())

		data, err := os.ReadFile(logPath)
		require.NoError(t, err)
		assert.Contains(t, string(data), "run=run-7")
		assert.Contains(t, string(data), "stage.name=provider")
	})

	t.Run("flushed buffer", func(t *testing.T) {
		var stderr bytes.Buffer
		ic, err := NewInterceptor(slog.NewTextHandler(&stderr, nil), nil, "", true)
		require.NoError(t, err)
		t.Cleanup(func() { _ = ic.Close() })

		slog.New(ic).With("run", "run-7").Info("buffered")
		assert.Empty(t, stderr.String(), "suppressed logs must not reach stderr before the flush")

		ic.FlushBuffer()
		assert.Contains(t, stderr.String(), "msg=buffered")
		assert.Contains(t, stderr.String(), "run=run-7")
	})
}

// TestSubInterceptor exercises the WithAttrs / WithGroup paths on the
// parent Interceptor and the resulting subInterceptor's Enabled / Handle /
// WithAttrs / WithGroup methods. Subinterceptors share the parent's TUI
// worker + log file but apply scoped slog attributes.
func TestSubInterceptor(t *testing.T) {
	handler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})

	t.Run("WithAttrs keeps the original handler's level", func(t *testing.T) {
		infoOnly, err := NewInterceptor(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}),
			nil, "", false)
		require.NoError(t, err)
		defer infoOnly.Close()

		sub := infoOnly.WithAttrs([]slog.Attr{slog.String("k", "v")})
		assert.True(t, sub.Enabled(context.Background(), slog.LevelInfo))
		assert.False(t, sub.Enabled(context.Background(), slog.LevelDebug))
	})

	t.Run("subInterceptor.Handle writes scoped attrs to stderr and the TUI", func(t *testing.T) {
		var stderr bytes.Buffer
		got := make(chan Msg, 1)
		ic, err := NewInterceptor(slog.NewTextHandler(&stderr, nil), func(m tea.Msg) { got <- m.(Msg) }, "", false)
		require.NoError(t, err)
		defer ic.Close()

		sub := ic.WithAttrs([]slog.Attr{slog.String("scope", "test")})
		require.NoError(t, sub.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "scoped log", 0)))
		assert.Contains(t, stderr.String(), `msg="scoped log" scope=test`)
		select {
		case m := <-got:
			assert.Equal(t, "scoped log", m.Message)
		case <-time.After(time.Second):
			t.Fatal("scoped log never reached the TUI send func")
		}
	})

	t.Run("subInterceptor.Handle buffers when suppressStderr", func(t *testing.T) {
		// New interceptor with suppressStderr=true to exercise the
		// buffering branch of subInterceptor.Handle.
		sup, err := NewInterceptor(handler, nil, "", true)
		require.NoError(t, err)
		defer sup.Close()
		sub := sup.WithAttrs([]slog.Attr{slog.String("k", "v")})
		rec := slog.NewRecord(time.Now(), slog.LevelInfo, "buffered", 0)
		require.NoError(t, sub.Handle(context.Background(), rec))
		assert.Len(t, sup.logBuffer, 1, "suppressed log should land in parent buffer")
	})

	t.Run("subInterceptor.Handle writes to log file when present", func(t *testing.T) {
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "sub.log")
		withFile, err := NewInterceptor(handler, nil, logPath, false)
		require.NoError(t, err)
		defer withFile.Close()
		sub := withFile.WithAttrs([]slog.Attr{slog.String("k", "v")})
		rec := slog.NewRecord(time.Now(), slog.LevelInfo, "file-bound", 0)
		require.NoError(t, sub.Handle(context.Background(), rec))
		// Force flush/close to ensure buffered file writes hit disk.
		withFile.Close()
		data, err := os.ReadFile(logPath)
		require.NoError(t, err)
		assert.Contains(t, string(data), "file-bound")
	})

	t.Run("subInterceptor.WithAttrs / WithGroup chain", func(t *testing.T) {
		var stderr bytes.Buffer
		ic, err := NewInterceptor(slog.NewTextHandler(&stderr, nil), nil, "", false)
		require.NoError(t, err)
		defer ic.Close()

		sub := ic.WithAttrs([]slog.Attr{slog.String("a", "1")})
		nested := sub.WithAttrs([]slog.Attr{slog.String("b", "2")})
		require.NoError(t, nested.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "nested", 0)))
		assert.Contains(t, stderr.String(), "msg=nested a=1 b=2")

		stderr.Reset()
		grouped := ic.WithGroup("outer").WithGroup("inner").WithAttrs([]slog.Attr{slog.String("c", "3")})
		require.NoError(t, grouped.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "grouped", 0)))
		assert.Contains(t, stderr.String(), "outer.inner.c=3")
	})
}
