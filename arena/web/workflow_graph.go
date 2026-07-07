package web

import (
	"fmt"
	"sort"

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
		}, nil
	}

	spec, err := workflow.ParseConfig(cfg.Workflow)
	if err != nil {
		return WorkflowGraph{}, fmt.Errorf("parsing workflow config: %w", err)
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
	}

	return graph, nil
}
