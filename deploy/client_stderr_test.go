//go:build !windows

package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// syncBuffer is a goroutine-safe strings.Builder.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "adapter.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil { //nolint:gosec // test adapter must be executable
		t.Fatal(err)
	}
	return path
}

// TestAdapterClient_StderrIsForwardedAndReportedOnCrash is the regression for
// #407 item 1: an adapter's stderr used to go to the null device, so a
// crashing adapter left only "adapter closed connection unexpectedly".
func TestAdapterClient_StderrIsForwardedAndReportedOnCrash(t *testing.T) {
	script := writeScript(t, "echo 'debug: starting' >&2\necho 'panic: boom' >&2\nexit 2\n")
	var stderr syncBuffer

	client, err := NewAdapterClientWithStderr(context.Background(), script, &stderr)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.GetProviderInfo(context.Background())
	if err == nil {
		t.Fatal("expected the crashed adapter to fail the call")
	}
	if !strings.Contains(err.Error(), "panic: boom") {
		t.Errorf("crash error should carry the adapter's stderr, got %q", err)
	}
	_ = client.Close()
	if got := stderr.String(); !strings.Contains(got, "debug: starting") {
		t.Errorf("stderr not forwarded, got %q", got)
	}
}

// TestAdapterClient_NilStderrStillReportsCrash covers the discard branch: the
// tail is kept even when nothing is forwarded.
func TestAdapterClient_NilStderrStillReportsCrash(t *testing.T) {
	script := writeScript(t, "echo 'fatal: no config' >&2\nexit 1\n")
	client, err := NewAdapterClientWithStderr(context.Background(), script, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = client.Close() }()
	_, err = client.GetProviderInfo(context.Background())
	if err == nil || !strings.Contains(err.Error(), "fatal: no config") {
		t.Fatalf("err = %v, want the stderr tail", err)
	}
}

// TestAdapterClient_SilentCrashKeepsPlainMessage pins the no-stderr branch.
func TestAdapterClient_SilentCrashKeepsPlainMessage(t *testing.T) {
	script := writeScript(t, "exit 1\n")
	client, err := NewAdapterClientWithStderr(context.Background(), script, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = client.Close() }()
	_, err = client.GetProviderInfo(context.Background())
	// Depending on timing the call sees EOF or a broken pipe; either way there
	// is no stderr to append.
	if err == nil || strings.Contains(err.Error(), "adapter stderr") {
		t.Fatalf("err = %v", err)
	}
}

func TestTailBuffer_KeepsOnlyTheEnd(t *testing.T) {
	tb := &tailBuffer{limit: 4}
	_, _ = tb.Write([]byte("abc"))
	_, _ = tb.Write([]byte("defg"))
	if got := tb.String(); got != "defg" {
		t.Fatalf("tail = %q, want defg", got)
	}
}
