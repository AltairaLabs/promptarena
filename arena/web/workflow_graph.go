package web

import (
	"fmt"
	"sort"

	"github.com/AltairaLabs/PromptKit/runtime/composition"
	"github.com/AltairaLabs/PromptKit/runtime/workflow"
	"github.com/AltairaLabs/promptarena/arena/arenaconfig"
)

// WorkflowGraphNode is a single state in the workflow topology graph.
type WorkflowGraphNode struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Kind     string `json:"kind"` // entry|output|agent (composition kinds added in a later task)
	Entry    bool   `json:"entry"`
	Terminal bool   `json:"terminal"`
}

// WorkflowGraphEdge is a single transition between two states in the
// workflow topology graph.
type WorkflowGraphEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Label  string `json:"label,omitempty"`
	Dashed bool   `json:"dashed,omitempty"`
}

// WorkflowGraph is the full workflow topology: its states (nodes) and
// transitions (edges).
type WorkflowGraph struct {
	Nodes []WorkflowGraphNode `json:"nodes"`
	Edges []WorkflowGraphEdge `json:"edges"`
}

// BuildWorkflowGraph turns a config's workflow spec into a topology graph.
// cfg == nil or cfg.Workflow == nil produces a single "default" node with no
// edges (Arena's implicit single-state workflow). Node and edge ordering is
// deterministic (sorted by state name, then event name).
func BuildWorkflowGraph(cfg *arenaconfig.Config) (WorkflowGraph, error) {
	if cfg == nil || cfg.Workflow == nil {
		return WorkflowGraph{
			Nodes: []WorkflowGraphNode{
				{ID: "default", Label: "default", Kind: "entry", Terminal: true},
			},
			Edges: []WorkflowGraphEdge{},
		}, nil
	}

	spec, err := workflow.ParseConfig(cfg.Workflow)
	if err != nil {
		return WorkflowGraph{}, fmt.Errorf("parsing workflow config: %w", err)
	}

	comps, err := composition.ParseConfig(cfg.Compositions)
	if err != nil {
		return WorkflowGraph{}, fmt.Errorf("parsing compositions config: %w", err)
	}

	names := make([]string, 0, len(spec.States))
	for name := range spec.States {
		names = append(names, name)
	}
	sort.Strings(names)

	graph := WorkflowGraph{
		Nodes: make([]WorkflowGraphNode, 0, len(names)),
		Edges: []WorkflowGraphEdge{},
	}

	for _, name := range names {
		state := spec.States[name]
		terminal := state.Terminal || len(state.OnEvent) == 0
		kind := "agent"
		switch {
		case name == spec.Entry:
			kind = "entry"
		case terminal:
			kind = "output"
		}

		graph.Nodes = append(graph.Nodes, WorkflowGraphNode{
			ID:       name,
			Label:    name,
			Kind:     kind,
			Entry:    name == spec.Entry,
			Terminal: terminal,
		})

		events := make([]string, 0, len(state.OnEvent))
		for event := range state.OnEvent {
			events = append(events, event)
		}
		sort.Strings(events)

		for _, event := range events {
			graph.Edges = append(graph.Edges, WorkflowGraphEdge{
				From:  name,
				To:    state.OnEvent[event],
				Label: event,
			})
		}

		if state.OnMaxVisits != "" {
			graph.Edges = append(graph.Edges, WorkflowGraphEdge{
				From:   name,
				To:     state.OnMaxVisits,
				Label:  "max-visits",
				Dashed: true,
			})
		}

		if state.Orchestration == workflow.OrchestrationComposition && state.Composition != "" {
			if comp := comps[state.Composition]; comp != nil {
				stepNodes, stepEdges := expandComposition(name, comp)
				graph.Nodes = append(graph.Nodes, stepNodes...)
				graph.Edges = append(graph.Edges, stepEdges...)
			}
		}
	}

	graph.Edges = dedupeEdges(graph.Edges)

	return graph, nil
}

// dedupeEdges removes duplicate edges (same From, To, and Dashed) from the
// whole graph's edge set, keeping the first occurrence and preserving order.
// Duplicates can arise when a state-machine or composition edge is derivable
// two ways — e.g. a branch step's "then" target that also declares a
// depends_on back to the branch step (the runtime's recommended join-hint
// pattern) produces the same edge from both the Then/Else loop and the
// DependsOn loop in expandComposition.
func dedupeEdges(edges []WorkflowGraphEdge) []WorkflowGraphEdge {
	type key struct {
		from, to string
		dashed   bool
	}
	seen := make(map[key]bool, len(edges))
	out := make([]WorkflowGraphEdge, 0, len(edges))
	for _, e := range edges {
		k := key{e.From, e.To, e.Dashed}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out
}

// expandComposition inlines a composition's step graph as nodes/edges scoped
// under stateName. Step ids are prefixed "<stateName>/<stepID>" to avoid
// collisions with workflow state ids and other states' composition steps
// (composition.Validate enforces step id uniqueness within a composition,
// including nested parallel.branches, so the prefix alone is enough).
//
// Top-level steps with no DependsOn are treated as the composition's entry
// points and get an edge from the state node itself. Nested branch steps
// inside a parallel step's Branches never get a state-entry edge — they are
// reached only via the parallel step's fan-out edge.
func expandComposition(stateName string, comp *composition.Composition) ([]WorkflowGraphNode, []WorkflowGraphEdge) {
	prefix := func(id string) string { return stateName + "/" + id }

	var nodes []WorkflowGraphNode
	var edges []WorkflowGraphEdge

	var walk func(steps []*composition.Step, topLevel bool)
	walk = func(steps []*composition.Step, topLevel bool) {
		for _, step := range steps {
			if step == nil {
				continue
			}
			nodes = append(nodes, WorkflowGraphNode{
				ID:    prefix(step.ID),
				Label: step.ID,
				Kind:  compositionStepKind(step.Kind),
			})

			if topLevel && len(step.DependsOn) == 0 {
				edges = append(edges, WorkflowGraphEdge{From: stateName, To: prefix(step.ID)})
			}

			deps := append([]string(nil), step.DependsOn...)
			sort.Strings(deps)
			for _, dep := range deps {
				edges = append(edges, WorkflowGraphEdge{From: prefix(dep), To: prefix(step.ID)})
			}

			switch step.Kind {
			case composition.KindBranch:
				if step.Then != "" {
					edges = append(edges, WorkflowGraphEdge{From: prefix(step.ID), To: prefix(step.Then)})
				}
				if step.Else != "" {
					edges = append(edges, WorkflowGraphEdge{From: prefix(step.ID), To: prefix(step.Else), Dashed: true})
				}
			case composition.KindParallel:
				branches := make([]*composition.Step, 0, len(step.Branches))
				for _, b := range step.Branches {
					if b == nil {
						continue
					}
					branches = append(branches, b)
				}
				sort.Slice(branches, func(i, j int) bool { return branches[i].ID < branches[j].ID })
				for _, b := range branches {
					edges = append(edges, WorkflowGraphEdge{From: prefix(step.ID), To: prefix(b.ID)})
				}
				walk(step.Branches, false)
			}
		}
	}

	walk(comp.Steps, true)

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return !edges[i].Dashed && edges[j].Dashed
	})

	return nodes, edges
}

// compositionStepKind maps a composition.StepKind to the WorkflowGraphNode
// Kind used by the frontend. parallel collapses into "branch" alongside
// branch since both are non-leaf control-flow steps.
func compositionStepKind(kind composition.StepKind) string {
	switch kind {
	case composition.KindPrompt:
		return "prompt"
	case composition.KindAgent:
		return "agent"
	case composition.KindTool:
		return "tool"
	case composition.KindBranch, composition.KindParallel:
		return "branch"
	default:
		return string(kind)
	}
}
