package web

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

func TestBuildWorkflowGraph_NoConfig(t *testing.T) {
	g, err := BuildWorkflowGraph(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 1 || g.Nodes[0].ID != "default" || g.Nodes[0].Kind != "entry" {
		t.Fatalf("want single default entry node, got %+v", g.Nodes)
	}
	if len(g.Edges) != 0 {
		t.Fatalf("want no edges, got %+v", g.Edges)
	}
}

func TestBuildWorkflowGraph_NoConfig_SerializesEdgesAsEmptyArray(t *testing.T) {
	g, err := BuildWorkflowGraph(nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"edges":null`) {
		t.Fatalf("want edges to serialize as [], got null: %s", b)
	}
	if !strings.Contains(string(b), `"edges":[]`) {
		t.Fatalf("want edges to serialize as [], got: %s", b)
	}
}

func TestBuildWorkflowGraph_NoWorkflow(t *testing.T) {
	g, err := BuildWorkflowGraph(&arenaconfig.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 1 || g.Nodes[0].ID != "default" || g.Nodes[0].Kind != "entry" {
		t.Fatalf("want single default entry node, got %+v", g.Nodes)
	}
	if len(g.Edges) != 0 {
		t.Fatalf("want no edges, got %+v", g.Edges)
	}
	if !g.Nodes[0].Terminal {
		t.Fatalf("default node should be terminal: %+v", g.Nodes[0])
	}
}

func TestBuildWorkflowGraph_StateMachine(t *testing.T) {
	cfg := &arenaconfig.Config{Workflow: map[string]any{
		"version": 2, "entry": "intake",
		"states": map[string]any{
			"intake":  map[string]any{"on_event": map[string]any{"classified": "resolve"}},
			"resolve": map[string]any{}, // no on_event => terminal
		},
	}}
	g, err := BuildWorkflowGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]WorkflowGraphNode{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	if byID["intake"].Kind != "entry" || !byID["intake"].Entry {
		t.Fatalf("intake should be entry: %+v", byID["intake"])
	}
	if byID["resolve"].Kind != "output" || !byID["resolve"].Terminal {
		t.Fatalf("resolve should be terminal/output: %+v", byID["resolve"])
	}
	if len(g.Edges) != 1 || g.Edges[0].From != "intake" || g.Edges[0].To != "resolve" || g.Edges[0].Label != "classified" {
		t.Fatalf("want intake->resolve[classified], got %+v", g.Edges)
	}
}

func TestBuildWorkflowGraph_MiddleAgentState(t *testing.T) {
	cfg := &arenaconfig.Config{Workflow: map[string]any{
		"version": 2, "entry": "intake",
		"states": map[string]any{
			"intake": map[string]any{"on_event": map[string]any{"classified": "triage"}},
			"triage": map[string]any{"on_event": map[string]any{"done": "resolve"}},
			"resolve": map[string]any{
				"terminal": true,
			},
		},
	}}
	g, err := BuildWorkflowGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]WorkflowGraphNode{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	if byID["triage"].Kind != "agent" || byID["triage"].Terminal {
		t.Fatalf("triage should be a non-terminal agent state: %+v", byID["triage"])
	}
	if byID["resolve"].Kind != "output" || !byID["resolve"].Terminal {
		t.Fatalf("resolve should be explicitly terminal via Terminal flag: %+v", byID["resolve"])
	}
}

func TestBuildWorkflowGraph_OnMaxVisitsEdge(t *testing.T) {
	cfg := &arenaconfig.Config{Workflow: map[string]any{
		"version": 2, "entry": "intake",
		"states": map[string]any{
			"intake": map[string]any{
				"on_event":      map[string]any{"classified": "triage"},
				"max_visits":    3,
				"on_max_visits": "escalate",
			},
			"triage":   map[string]any{"on_event": map[string]any{"done": "escalate"}},
			"escalate": map[string]any{},
		},
	}}
	g, err := BuildWorkflowGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var maxVisitsEdges []WorkflowGraphEdge
	for _, e := range g.Edges {
		if e.Dashed {
			maxVisitsEdges = append(maxVisitsEdges, e)
		}
	}
	if len(maxVisitsEdges) != 1 {
		t.Fatalf("want exactly 1 dashed max-visits edge, got %+v", g.Edges)
	}
	edge := maxVisitsEdges[0]
	if edge.From != "intake" || edge.To != "escalate" || edge.Label != "max-visits" {
		t.Fatalf("want intake->escalate[max-visits] dashed, got %+v", edge)
	}
}

func TestBuildWorkflowGraph_DeterministicOrdering(t *testing.T) {
	cfg := &arenaconfig.Config{Workflow: map[string]any{
		"version": 2, "entry": "b",
		"states": map[string]any{
			"b": map[string]any{"on_event": map[string]any{"z": "a", "y": "c"}},
			"a": map[string]any{},
			"c": map[string]any{},
		},
	}}

	var first WorkflowGraph
	for i := range 10 {
		g, err := BuildWorkflowGraph(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = g
			continue
		}
		if len(g.Nodes) != len(first.Nodes) {
			t.Fatalf("node count changed across runs")
		}
		for j := range g.Nodes {
			if g.Nodes[j] != first.Nodes[j] {
				t.Fatalf("node ordering not deterministic: run %d got %+v, want %+v", i, g.Nodes, first.Nodes)
			}
		}
		for j := range g.Edges {
			if g.Edges[j] != first.Edges[j] {
				t.Fatalf("edge ordering not deterministic: run %d got %+v, want %+v", i, g.Edges, first.Edges)
			}
		}
	}

	// Nodes sorted by name: a, b, c.
	wantNodeIDs := []string{"a", "b", "c"}
	for i, n := range first.Nodes {
		if n.ID != wantNodeIDs[i] {
			t.Fatalf("nodes not sorted by name: got %+v", first.Nodes)
		}
	}
	// Edges from "b" sorted by event name: y before z.
	if len(first.Edges) != 2 || first.Edges[0].Label != "y" || first.Edges[1].Label != "z" {
		t.Fatalf("edges not sorted by event name: got %+v", first.Edges)
	}
}

func TestBuildWorkflowGraph_ParseError(t *testing.T) {
	cfg := &arenaconfig.Config{Workflow: func() {}} // not JSON-marshalable
	if _, err := BuildWorkflowGraph(cfg); err == nil {
		t.Fatal("want error for unparsable workflow config, got nil")
	}
}
