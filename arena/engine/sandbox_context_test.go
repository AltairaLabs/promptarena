package engine

import (
	"context"
	"testing"
)

func TestSandboxContainerIDsContext(t *testing.T) {
	ctx := context.Background()
	if got := sandboxContainerIDsFromContext(ctx); got != nil {
		t.Fatalf("empty context should carry no IDs, got %v", got)
	}

	// nil/empty ids is a no-op (context unchanged).
	if got := sandboxContainerIDsFromContext(withSandboxContainerIDs(ctx, nil)); got != nil {
		t.Fatalf("nil ids should be a no-op, got %v", got)
	}

	ids := map[string]string{"sandbox": "abc123"}
	got := sandboxContainerIDsFromContext(withSandboxContainerIDs(ctx, ids))
	if got["sandbox"] != "abc123" {
		t.Fatalf("round-trip failed: got %v", got)
	}
}

func TestSandboxPortsContext(t *testing.T) {
	ctx := context.Background()
	if got := sandboxPortsFromContext(ctx); got != nil {
		t.Fatalf("empty context should carry no ports, got %v", got)
	}
	if got := sandboxPortsFromContext(withSandboxPorts(ctx, nil)); got != nil {
		t.Fatalf("nil ports should be a no-op, got %v", got)
	}
	ports := map[string]map[string]string{"sandbox": {"3000": "http://localhost:54321"}}
	got := sandboxPortsFromContext(withSandboxPorts(ctx, ports))
	if got["sandbox"]["3000"] != "http://localhost:54321" {
		t.Fatalf("round-trip failed: got %v", got)
	}
}

func TestSessionEventMetadata_IncludesSandboxPorts(t *testing.T) {
	e := &Engine{}
	ports := map[string]map[string]string{"sandbox": {"3000": "http://localhost:54321"}}
	ctx := withSandboxPorts(withSandboxContainerIDs(context.Background(), map[string]string{"sandbox": "cid"}), ports)

	meta := e.sessionEventMetadata(ctx, "")
	if got, _ := meta["sandbox_ports"].(map[string]map[string]string); got["sandbox"]["3000"] != "http://localhost:54321" {
		t.Fatalf("sandbox_ports missing from metadata: %v", meta)
	}

	if _, ok := e.sessionEventMetadata(context.Background(), "")["sandbox_ports"]; ok {
		t.Fatal("sandbox_ports must be absent when no ports are open")
	}
}
