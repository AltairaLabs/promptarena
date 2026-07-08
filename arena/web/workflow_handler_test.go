package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

func TestHandleWorkflow_NoEngine(t *testing.T) {
	adapter := NewEventAdapter()
	srv := NewServer(adapter, nil, nil, "")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/workflow") //nolint:noctx
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestHandleWorkflow_DefaultGraph(t *testing.T) {
	adapter := NewEventAdapter()
	mock := &mockEngine{cfg: &arenaconfig.Config{}}
	srv := newServerWithRunner(adapter, mock, nil, "")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/workflow") //nolint:noctx
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var g WorkflowGraph
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(g.Nodes) != 1 || g.Nodes[0].ID != "default" {
		t.Errorf("want single default node, got %+v", g.Nodes)
	}
}

func TestHandleWorkflow_StateMachineGraph(t *testing.T) {
	adapter := NewEventAdapter()
	cfg := &arenaconfig.Config{Workflow: map[string]any{
		"version": 2, "entry": "intake",
		"states": map[string]any{
			"intake":  map[string]any{"on_event": map[string]any{"classified": "resolve"}},
			"resolve": map[string]any{},
		},
	}}
	mock := &mockEngine{cfg: cfg}
	srv := newServerWithRunner(adapter, mock, nil, "")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/workflow") //nolint:noctx
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var g WorkflowGraph
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(g.Nodes) != 2 || len(g.Edges) != 1 {
		t.Fatalf("want 2 nodes and 1 edge, got %+v", g)
	}
}

func TestHandleWorkflow_ParseError(t *testing.T) {
	adapter := NewEventAdapter()
	mock := &mockEngine{cfg: &arenaconfig.Config{Workflow: func() {}}}
	srv := newServerWithRunner(adapter, mock, nil, "")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/workflow") //nolint:noctx
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}
