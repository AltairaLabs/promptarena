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

func TestBuildWorkflowGraph_CompositionExpansion(t *testing.T) {
	cfg := &arenaconfig.Config{
		Workflow: map[string]any{
			"version": 2, "entry": "intake",
			"states": map[string]any{
				"intake": map[string]any{"on_event": map[string]any{"go": "process"}},
				"process": map[string]any{
					"orchestration": "composition",
					"composition":   "flow",
					"terminal":      true,
				},
			},
		},
		Compositions: map[string]any{
			"flow": map[string]any{
				"version": 1,
				"steps": []map[string]any{
					{"id": "fetch", "kind": "agent", "termination": map[string]any{"max_steps": 3}},
					{"id": "check", "kind": "tool", "tool": "echo", "depends_on": []string{"fetch"}},
					{
						"id": "route", "kind": "branch", "depends_on": []string{"check"},
						"predicate": map[string]any{"path": "check.ok", "op": "equals", "value": true},
						"then":      "approve", "else": "reject",
					},
					{"id": "approve", "kind": "prompt", "depends_on": []string{"route"}},
					{"id": "reject", "kind": "prompt", "depends_on": []string{"route"}},
				},
			},
		},
	}

	g, err := BuildWorkflowGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}

	nodeByID := map[string]WorkflowGraphNode{}
	for _, n := range g.Nodes {
		nodeByID[n.ID] = n
	}
	wantKinds := map[string]string{
		"process/fetch":   "agent",
		"process/check":   "tool",
		"process/route":   "branch",
		"process/approve": "prompt",
		"process/reject":  "prompt",
	}
	for id, kind := range wantKinds {
		n, ok := nodeByID[id]
		if !ok {
			t.Fatalf("missing composition step node %q, got nodes %+v", id, g.Nodes)
		}
		if n.Kind != kind {
			t.Fatalf("node %q: want kind %q, got %q", id, kind, n.Kind)
		}
	}

	hasEdge := func(from, to string, dashed bool) bool {
		for _, e := range g.Edges {
			if e.From == from && e.To == to && e.Dashed == dashed {
				return true
			}
		}
		return false
	}

	if !hasEdge("process", "process/fetch", false) {
		t.Fatalf("want edge from state node process to entry step process/fetch, got %+v", g.Edges)
	}
	if !hasEdge("process/fetch", "process/check", false) {
		t.Fatalf("want depends_on edge process/fetch->process/check, got %+v", g.Edges)
	}
	if !hasEdge("process/check", "process/route", false) {
		t.Fatalf("want depends_on edge process/check->process/route, got %+v", g.Edges)
	}
	if !hasEdge("process/route", "process/approve", false) {
		t.Fatalf("want solid then edge process/route->process/approve, got %+v", g.Edges)
	}
	if !hasEdge("process/route", "process/reject", true) {
		t.Fatalf("want dashed else edge process/route->process/reject, got %+v", g.Edges)
	}
	if hasEdge("process", "process/approve", false) || hasEdge("process", "process/reject", false) {
		t.Fatalf("approve/reject are not entry steps (they depend_on route), want no state->step edge, got %+v", g.Edges)
	}
}

func TestBuildWorkflowGraph_CompositionExpansion_JoinHintDedupesEdge(t *testing.T) {
	// A branch step's "then" target that also declares depends_on back to
	// the branch step is the runtime's recommended join-hint pattern (see
	// validateJoinHints). It must not produce two stacked edges in the
	// graph: one from the branch's Then loop and one from the DependsOn
	// loop.
	cfg := &arenaconfig.Config{
		Workflow: map[string]any{
			"version": 2, "entry": "intake",
			"states": map[string]any{
				"intake": map[string]any{"on_event": map[string]any{"go": "process"}},
				"process": map[string]any{
					"orchestration": "composition",
					"composition":   "flow",
					"terminal":      true,
				},
			},
		},
		Compositions: map[string]any{
			"flow": map[string]any{
				"version": 1,
				"steps": []map[string]any{
					{"id": "route", "kind": "branch",
						"predicate": map[string]any{"path": "x", "op": "equals", "value": true},
						"then":      "approve", "else": "reject",
					},
					// approve declares depends_on ["route"] in addition to
					// being the branch's "then" target -- the join-hint
					// pattern that previously produced a duplicate edge.
					{"id": "approve", "kind": "prompt", "depends_on": []string{"route"}},
					{"id": "reject", "kind": "prompt", "depends_on": []string{"route"}},
				},
			},
		},
	}

	g, err := BuildWorkflowGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var matches int
	for _, e := range g.Edges {
		if e.From == "process/route" && e.To == "process/approve" && !e.Dashed {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("want exactly 1 edge process/route->process/approve, got %d in %+v", matches, g.Edges)
	}
}

func TestBuildWorkflowGraph_CompositionExpansion_ParallelFanOut(t *testing.T) {
	cfg := &arenaconfig.Config{
		Workflow: map[string]any{
			"version": 2, "entry": "run",
			"states": map[string]any{
				"run": map[string]any{
					"orchestration": "composition",
					"composition":   "par",
					"terminal":      true,
				},
			},
		},
		Compositions: map[string]any{
			"par": map[string]any{
				"version": 1,
				"steps": []map[string]any{
					{"id": "start", "kind": "agent", "termination": map[string]any{"max_steps": 1}},
					{
						"id": "fan", "kind": "parallel", "depends_on": []string{"start"},
						"reduce": map[string]any{"strategy": "append", "into": "results"},
						"branches": []map[string]any{
							{"id": "b1", "kind": "tool", "tool": "echo"},
							{"id": "b2", "kind": "tool", "tool": "echo"},
						},
					},
				},
			},
		},
	}

	g, err := BuildWorkflowGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}

	nodeByID := map[string]WorkflowGraphNode{}
	for _, n := range g.Nodes {
		nodeByID[n.ID] = n
	}
	if nodeByID["run/fan"].Kind != "branch" {
		t.Fatalf("parallel step should map to kind branch, got %+v", nodeByID["run/fan"])
	}
	if nodeByID["run/b1"].Kind != "tool" || nodeByID["run/b2"].Kind != "tool" {
		t.Fatalf("want branch steps b1/b2 as tool nodes, got %+v %+v", nodeByID["run/b1"], nodeByID["run/b2"])
	}

	hasEdge := func(from, to string) bool {
		for _, e := range g.Edges {
			if e.From == from && e.To == to {
				return true
			}
		}
		return false
	}
	if !hasEdge("run", "run/start") {
		t.Fatalf("want state->entry edge run->run/start, got %+v", g.Edges)
	}
	if !hasEdge("run/start", "run/fan") {
		t.Fatalf("want depends_on edge run/start->run/fan, got %+v", g.Edges)
	}
	if !hasEdge("run/fan", "run/b1") || !hasEdge("run/fan", "run/b2") {
		t.Fatalf("want fan-out edges from run/fan to both branches, got %+v", g.Edges)
	}
}

func TestBuildWorkflowGraph_CompositionExpansion_ImplicitSequencing(t *testing.T) {
	// document-analysis shape: classify(prompt) -> route(branch: then
	// extract_paper / else extract_general) -> extract_paper(prompt) ->
	// extract_general(prompt) -> meta(parallel: branches meta_summary,
	// meta_keywords) -> synthesize(agent). None of the top-level steps
	// declare depends_on -- the graph must be derived entirely from list
	// order + branch/parallel structure, matching how the runtime's
	// composition engine actually executes the step list.
	cfg := &arenaconfig.Config{
		Workflow: map[string]any{
			"version": 2, "entry": "intake",
			"states": map[string]any{
				"intake": map[string]any{"on_event": map[string]any{"go": "analyzing"}},
				"analyzing": map[string]any{
					"orchestration": "composition",
					"composition":   "document-analysis",
					"terminal":      true,
				},
			},
		},
		Compositions: map[string]any{
			"document-analysis": map[string]any{
				"version": 1,
				"steps": []map[string]any{
					{"id": "classify", "kind": "prompt"},
					{
						"id": "route", "kind": "branch",
						"predicate": map[string]any{"path": "classify.output.type", "op": "equals", "value": "paper"},
						"then":      "extract_paper", "else": "extract_general",
					},
					{"id": "extract_paper", "kind": "prompt"},
					{"id": "extract_general", "kind": "prompt"},
					{
						"id": "meta", "kind": "parallel",
						"reduce": map[string]any{"strategy": "append", "into": "results"},
						"branches": []map[string]any{
							{"id": "meta_summary", "kind": "tool", "tool": "echo"},
							{"id": "meta_keywords", "kind": "tool", "tool": "echo"},
						},
					},
					{"id": "synthesize", "kind": "agent", "termination": map[string]any{"max_steps": 3}},
				},
			},
		},
	}

	g, err := BuildWorkflowGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}

	hasEdge := func(from, to string, dashed bool) bool {
		for _, e := range g.Edges {
			if e.From == from && e.To == to && e.Dashed == dashed {
				return true
			}
		}
		return false
	}
	countEdgesFrom := func(from string) int {
		n := 0
		for _, e := range g.Edges {
			if e.From == from {
				n++
			}
		}
		return n
	}

	// State entry edge: only the composition's first step, not a star to
	// every step.
	if countEdgesFrom("analyzing") != 1 {
		t.Fatalf("want exactly 1 out-edge from analyzing, got %d in %+v", countEdgesFrom("analyzing"), g.Edges)
	}
	if !hasEdge("analyzing", "analyzing/classify", false) {
		t.Fatalf("want analyzing->analyzing/classify entry edge, got %+v", g.Edges)
	}

	// Implicit sequential edge.
	if !hasEdge("analyzing/classify", "analyzing/route", false) {
		t.Fatalf("want implicit sequential edge classify->route, got %+v", g.Edges)
	}

	// Branch then/else edges.
	if !hasEdge("analyzing/route", "analyzing/extract_paper", false) {
		t.Fatalf("want solid then edge route->extract_paper, got %+v", g.Edges)
	}
	if !hasEdge("analyzing/route", "analyzing/extract_general", true) {
		t.Fatalf("want dashed else edge route->extract_general, got %+v", g.Edges)
	}

	// Branch join: both arms converge on the next step.
	if !hasEdge("analyzing/extract_paper", "analyzing/meta", false) {
		t.Fatalf("want branch-join edge extract_paper->meta, got %+v", g.Edges)
	}
	if !hasEdge("analyzing/extract_general", "analyzing/meta", false) {
		t.Fatalf("want branch-join edge extract_general->meta, got %+v", g.Edges)
	}

	// Naive list-order chaining across the branch's arms must NOT appear.
	if hasEdge("analyzing/extract_paper", "analyzing/extract_general", false) ||
		hasEdge("analyzing/extract_paper", "analyzing/extract_general", true) {
		t.Fatalf("must not have a naive edge extract_paper->extract_general, got %+v", g.Edges)
	}

	// Parallel fan-out.
	if !hasEdge("analyzing/meta", "analyzing/meta_summary", false) {
		t.Fatalf("want fan-out edge meta->meta_summary, got %+v", g.Edges)
	}
	if !hasEdge("analyzing/meta", "analyzing/meta_keywords", false) {
		t.Fatalf("want fan-out edge meta->meta_keywords, got %+v", g.Edges)
	}

	// Parallel join to the next step: modeled from the parallel step itself
	// (the barrier/join), matching the runtime's own top-level "prev"
	// sequencing -- nested parallel branches never advance it.
	if !hasEdge("analyzing/meta", "analyzing/synthesize", false) {
		t.Fatalf("want parallel-join edge meta->synthesize, got %+v", g.Edges)
	}
}

func TestBuildWorkflowGraph_CompositionExpansion_MissingCompositions(t *testing.T) {
	cfg := &arenaconfig.Config{
		Workflow: map[string]any{
			"version": 2, "entry": "process",
			"states": map[string]any{
				"process": map[string]any{
					"orchestration": "composition",
					"composition":   "flow",
					"terminal":      true,
				},
			},
		},
		// Compositions intentionally left nil.
	}

	g, err := BuildWorkflowGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 1 || g.Nodes[0].ID != "process" {
		t.Fatalf("want only the state node when cfg.Compositions is nil, got %+v", g.Nodes)
	}
	if len(g.Edges) != 0 {
		t.Fatalf("want no edges when cfg.Compositions is nil, got %+v", g.Edges)
	}
}

func TestBuildWorkflowGraph_CompositionExpansion_UnknownCompositionName(t *testing.T) {
	cfg := &arenaconfig.Config{
		Workflow: map[string]any{
			"version": 2, "entry": "process",
			"states": map[string]any{
				"process": map[string]any{
					"orchestration": "composition",
					"composition":   "missing",
					"terminal":      true,
				},
			},
		},
		Compositions: map[string]any{
			"other": map[string]any{
				"version": 1,
				"steps": []map[string]any{
					{"id": "s", "kind": "tool", "tool": "echo"},
				},
			},
		},
	}

	g, err := BuildWorkflowGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 1 || g.Nodes[0].ID != "process" {
		t.Fatalf("want only the state node when the named composition is not found, got %+v", g.Nodes)
	}
	if len(g.Edges) != 0 {
		t.Fatalf("want no edges when the named composition is not found, got %+v", g.Edges)
	}
}
