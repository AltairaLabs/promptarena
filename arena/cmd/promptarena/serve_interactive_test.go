package main

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

// freeStartPort returns an ephemeral port number that was free at probe time,
// to use as a deterministic starting point for the scan tests.
func freeStartPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe for free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestLoopbackPortAnswering(t *testing.T) {
	free := freeStartPort(t)
	if loopbackPortAnswering(t.Context(), free) {
		t.Fatalf("nothing is listening on %d, expected not-answering", free)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", free))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			_ = c.Close()
		}
	}()
	if !loopbackPortAnswering(t.Context(), free) {
		t.Fatalf("something is listening on %d, expected answering", free)
	}
}

func TestFirstFreeLoopbackPort_ReturnsUsablePort(t *testing.T) {
	v4, v6, port, err := firstFreeLoopbackPort(t.Context(), freeStartPort(t), 50)
	if err != nil {
		t.Fatalf("expected a free port, got error: %v", err)
	}
	defer func() { _ = v4.Close() }()
	if v6 != nil {
		defer func() { _ = v6.Close() }()
	}
	if v4 == nil || port <= 0 {
		t.Fatalf("expected a bound IPv4 listener and positive port, got port=%d v4=%v", port, v4)
	}
}

func TestFirstFreeLoopbackPort_SkipsIPv4Occupied(t *testing.T) {
	start := freeStartPort(t)
	block, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", start))
	if err != nil {
		t.Skipf("could not occupy 127.0.0.1:%d: %v", start, err)
	}
	defer func() { _ = block.Close() }()

	v4, v6, port, err := firstFreeLoopbackPort(t.Context(), start, 50)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	defer func() { _ = v4.Close() }()
	if v6 != nil {
		defer func() { _ = v6.Close() }()
	}
	if port == start {
		t.Fatalf("expected to skip IPv4-occupied port %d, but selected it", start)
	}
}

// TestFirstFreeLoopbackPort_SkipsIPv6Occupied reproduces the real bug: a process
// holding the port only on the IPv6 loopback must NOT be shadowed. The old
// IPv4-only check would wrongly select the port; the fix must skip it.
func TestFirstFreeLoopbackPort_SkipsIPv6Occupied(t *testing.T) {
	start := freeStartPort(t)
	block, err := net.Listen("tcp6", fmt.Sprintf("[::1]:%d", start))
	if err != nil {
		t.Skipf("no IPv6 loopback available: %v", err)
	}
	defer func() { _ = block.Close() }()

	v4, v6, port, err := firstFreeLoopbackPort(t.Context(), start, 50)
	if err != nil {
		t.Fatalf("scan error: %v", err)
	}
	defer func() { _ = v4.Close() }()
	if v6 != nil {
		defer func() { _ = v6.Close() }()
	}
	if port == start {
		t.Fatalf("expected to skip port %d occupied on the IPv6 loopback, but selected it", start)
	}
}

func TestServeBindsToLocalhost(t *testing.T) {
	// The server address should bind to 127.0.0.1, not 0.0.0.0
	port := 8080
	addr := serveAddr(port)
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("expected address to bind to 127.0.0.1, got %s", addr)
	}
}

func TestServeAddrFormat(t *testing.T) {
	addr := serveAddr(3000)
	if addr != "127.0.0.1:3000" {
		t.Fatalf("expected 127.0.0.1:3000, got %s", addr)
	}
}
